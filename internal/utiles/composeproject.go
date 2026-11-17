package utiles

// Compose 工程级控制（list / up / down / logs）的核心实现。
// 与 composeupdate.go 同包：复用 composeCommand（cli 探测）、composeSafeToken
// （参数防注入）与 composeLabels 的标签约定。
//
// 自护栏：down/up 拒绝作用于 DTool 自身所在 compose 工程（参考 selfid.go 的
// DetectSelfContainerID / IsSelfContainer 语义）；宿主裸跑时 selfID 为空，
// 与现有自身保护一致，自动放行。

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	MyType "github.com/DongShull/DTool/internal/types"
	"github.com/DongShull/DTool/internal/svc"
	"github.com/docker/docker/api/types/container"
	"github.com/zeromicro/go-zero/core/logx"
)

// composeActionTimeout 是单次 compose up/down 的执行上限。
const composeActionTimeout = 5 * time.Minute

// ComposeSafeToken 对外暴露标签值/项目名的参数安全校验（测试与 handler 复用）。
// 唯一实现在 composeupdate.go 的 composeSafeToken，这里只是导出薄包装，不要重复写规则。
func ComposeSafeToken(value string) bool {
	return composeSafeToken(value)
}

// composeContainerName 从容器列表条目取容器名：优先 Names[0]（去掉开头的 /），
// Names 为空时回退容器 ID 前 12 位。注意先判空再取下标，避免越界 panic。
func composeContainerName(id string, names []string) string {
	if len(names) == 0 || names[0] == "" {
		if len(id) >= 12 {
			return id[:12]
		}
		return id
	}
	return strings.TrimPrefix(names[0], "/")
}

// ComposeLifecycleArgs 构造 compose 工程级 up/down 的 cli 参数。
// 纯函数便于单测：action 只接受 up/down，project 走 composeSafeToken 防注入。
func ComposeLifecycleArgs(action, project string) ([]string, error) {
	action = strings.TrimSpace(strings.ToLower(action))
	switch action {
	case "up":
		if !composeSafeToken(project) {
			return nil, fmt.Errorf("project 名含非法字符或以 - 开头，拒绝执行（project=%q）", project)
		}
		return []string{"up", "-d"}, nil
	case "down":
		if !composeSafeToken(project) {
			return nil, fmt.Errorf("project 名含非法字符或以 - 开头，拒绝执行（project=%q）", project)
		}
		return []string{"down"}, nil
	default:
		return nil, fmt.Errorf("不支持的 compose 动作: %q（仅支持 up/down）", action)
	}
}

// composeProjectStatus 由 running/stopped 计数推导工程状态（纯函数，便于单测）。
func composeProjectStatus(running, stopped int) string {
	switch {
	case running > 0 && stopped == 0:
		return "running"
	case running > 0:
		return "partial"
	case running == 0 && stopped > 0:
		return "stopped"
	default:
		return "empty"
	}
}

// composeProjectBrief 是聚合工程信息时的容器原始条目。
type composeProjectBrief struct {
	Project     string
	Service     string
	Workdir     string
	ConfigFiles string
	Name        string
	State       string
	Image       string
}

// listComposeBriefs 拉取全部容器并提取 compose 相关标签（便于 mock/测试的收口）。
func listComposeBriefs(ctx context.Context, serviceContext *svc.ServiceContext) ([]composeProjectBrief, error) {
	if err := requireDocker(serviceContext); err != nil {
		return nil, err
	}
	callCtx, cancel := dockerTimeout(ctx, dockerListTimeout)
	defer cancel()
	items, err := serviceContext.DockerClient.ContainerList(callCtx, container.ListOptions{All: true})
	if err != nil {
		logx.Errorf("获取容器列表失败（compose projects）: %v", err)
		return nil, fmt.Errorf("获取容器列表失败: %w", err)
	}
	out := make([]composeProjectBrief, 0, len(items))
	for _, it := range items {
		labels := ComposeLabelsOf(it.Labels)
		if labels.Project == "" {
			continue
		}
		out = append(out, composeProjectBrief{
			Project:     labels.Project,
			Service:     labels.Service,
			Workdir:     labels.WorkingDir,
			ConfigFiles: labels.ConfigFile,
			Name:        composeContainerName(it.ID, it.Names),
			State:       it.State,
			Image:       it.Image,
		})
	}
	return out, nil
}

// ListComposeProjects 按 compose project 聚合容器，返回工程列表（项目名、服务数、workdir、状态）。
func ListComposeProjects(ctx context.Context, serviceContext *svc.ServiceContext) ([]MyType.ComposeProjectInfo, error) {
	briefs, err := listComposeBriefs(ctx, serviceContext)
	if err != nil {
		return nil, err
	}
	order := make([]string, 0)
	byProject := map[string]*MyType.ComposeProjectInfo{}
	for _, b := range briefs {
		info, ok := byProject[b.Project]
		if !ok {
			info = &MyType.ComposeProjectInfo{
				Project:     b.Project,
				Workdir:     b.Workdir,
				ConfigFiles: b.ConfigFiles,
				Containers:  []MyType.ComposeContainerBrief{},
			}
			byProject[b.Project] = info
			order = append(order, b.Project)
		}
		if info.Workdir == "" {
			info.Workdir = b.Workdir
		}
		if info.ConfigFiles == "" {
			info.ConfigFiles = b.ConfigFiles
		}
		info.Containers = append(info.Containers, MyType.ComposeContainerBrief{
			Name:    b.Name,
			Service: b.Service,
			State:   b.State,
			Image:   b.Image,
		})
		if b.State == "running" {
			info.Running++
		} else {
			info.Stopped++
		}
	}
	out := make([]MyType.ComposeProjectInfo, 0, len(order))
	for _, p := range order {
		info := byProject[p]
		// 服务数按去重的 service 名统计（一个服务可有多个容器副本）。
		seen := map[string]bool{}
		for _, c := range info.Containers {
			if c.Service != "" && !seen[c.Service] {
				seen[c.Service] = true
			}
		}
		info.Services = len(seen)
		info.Status = composeProjectStatus(info.Running, info.Stopped)
		out = append(out, *info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Project < out[j].Project })
	return out, nil
}

// SelfComposeProject 返回自身所在 compose 工程名；不在容器内或无 compose 标签时为空。
func SelfComposeProject(ctx context.Context, serviceContext *svc.ServiceContext) string {
	selfID := DetectSelfContainerID()
	if selfID == "" || serviceContext == nil || serviceContext.DockerClient == nil {
		return ""
	}
	inspectCtx, cancel := dockerTimeout(ctx, dockerListTimeout)
	defer cancel()
	inspected, err := serviceContext.DockerClient.ContainerInspect(inspectCtx, selfID)
	if err != nil || inspected.Config == nil {
		return ""
	}
	return ComposeLabelsOf(inspected.Config.Labels).Project
}

// ComposeProjectAction 对指定 compose 工程执行 up -d / down。
// 返回 compose cli 的合并输出。写操作，内部走审计。
func ComposeProjectAction(ctx context.Context, serviceContext *svc.ServiceContext, project, action string) (string, error) {
	project = strings.TrimSpace(project)
	args, err := ComposeLifecycleArgs(action, project)
	if err != nil {
		return "", err
	}
	// 自护栏：DTool 自身所在工程不允许 up/down，避免把自己下线。
	if selfProject := SelfComposeProject(ctx, serviceContext); selfProject != "" && IsSelfContainer(selfProject, project) {
		AuditLog("compose."+action, project, "rejected: 自身所在 compose 工程禁止操作")
		return "", fmt.Errorf("project %q 是 DTool 自身所在 compose 工程，禁止 %s 操作", project, action)
	}

	briefs, err := listComposeBriefs(ctx, serviceContext)
	if err != nil {
		return "", err
	}
	workdir := ""
	found := false
	for _, b := range briefs {
		if b.Project == project {
			found = true
			if workdir == "" {
				workdir = b.Workdir
			}
		}
	}
	if !found {
		return "", fmt.Errorf("没有找到 compose 工程 %q 的容器（确认项目名是否正确）", project)
	}
	if workdir == "" {
		return "", fmt.Errorf("工程 %q 的容器缺少 working_dir 标签，无法定位 compose 文件目录", project)
	}
	if err = EnsureComposeCli(); err != nil {
		return "", err
	}

	output, execErr := runComposeProject(ctx, workdir, project, args...)
	AuditLog("compose."+action, project, mapResultDetail(output, execErr))
	if execErr != nil {
		return output, fmt.Errorf("compose %s 失败: %w", action, execErr)
	}
	logx.Infof("compose %s 完成（project=%s）: %s", action, project, strings.ReplaceAll(output, "\n", " "))
	return output, nil
}

// runComposeProject 在工程 workdir 下执行一条 compose 命令并返回合并输出。
func runComposeProject(ctx context.Context, workdir, project string, args ...string) (string, error) {
	base, ok := composeCommand()
	if !ok {
		return "", fmt.Errorf("宿主环境没有可用的 docker compose（既无 v2 插件也无 docker-compose）")
	}
	actCtx, cancel := context.WithTimeout(ctx, composeActionTimeout)
	defer cancel()
	cmdArgs := append(append([]string{}, base...), args...)
	cmd := exec.CommandContext(actCtx, cmdArgs[0], cmdArgs[1:]...)
	cmd.Dir = workdir
	cmd.Env = append(os.Environ(), "COMPOSE_PROJECT_NAME="+project)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// mapResultDetail 把输出/错误压成审计用的单行摘要。
func mapResultDetail(output string, err error) string {
	line := strings.ReplaceAll(output, "\n", " ")
	if len(line) > 200 {
		line = line[:200]
	}
	if err != nil {
		return "failed: " + err.Error() + " | " + line
	}
	if line == "" {
		return "ok"
	}
	return "ok: " + line
}

// ComposeProjectLogs 聚合指定工程（可再按 service 过滤）各容器最近 N 行日志。
// tail<=0 默认 200，上限 5000（与 ContainerLogs 一致）。
func ComposeProjectLogs(ctx context.Context, serviceContext *svc.ServiceContext, project, service string, tail int) ([]MyType.ComposeServiceLogs, error) {
	project = strings.TrimSpace(project)
	service = strings.TrimSpace(service)
	if !composeSafeToken(project) {
		return nil, fmt.Errorf("project 名含非法字符，拒绝执行（project=%q）", project)
	}
	if service != "" && !composeSafeToken(service) {
		return nil, fmt.Errorf("service 名含非法字符，拒绝执行（service=%q）", service)
	}
	briefs, err := listComposeBriefs(ctx, serviceContext)
	if err != nil {
		return nil, err
	}
	var targetIDs []string
	var targetNames []string
	var targetServices []string
	found := false
	for _, b := range briefs {
		if b.Project != project {
			continue
		}
		found = true
		if service != "" && b.Service != service {
			continue
		}
		targetIDs = append(targetIDs, b.Name)
		targetNames = append(targetNames, b.Name)
		targetServices = append(targetServices, b.Service)
	}
	if !found {
		return nil, fmt.Errorf("没有找到 compose 工程 %q 的容器", project)
	}
	if len(targetIDs) == 0 && service != "" {
		return nil, fmt.Errorf("工程 %q 里没有名为 %q 的服务", project, service)
	}

	out := make([]MyType.ComposeServiceLogs, 0, len(targetIDs))
	for i, id := range targetIDs {
		lines, logErr := ContainerLogs(ctx, serviceContext, id, tail, false, "")
		if logErr != nil {
			logx.Errorf("读取 %s/%s 日志失败: %v", project, id, logErr)
			lines = []string{"<读取日志失败: " + logErr.Error() + ">"}
		}
		out = append(out, MyType.ComposeServiceLogs{
			Service:   targetServices[i],
			Container: targetNames[i],
			Lines:     lines,
		})
	}
	return out, nil
}
