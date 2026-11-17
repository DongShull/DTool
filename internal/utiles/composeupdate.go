package utiles

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"os/exec"
	"regexp"
	"strings"
	"time"

	dockerTypes "github.com/docker/docker/api/types"
	"github.com/DongShull/DTool/internal/svc"
	"github.com/zeromicro/go-zero/core/logx"
)

// ---- Compose 更新策略 ----
//
// 更新策略由环境变量 COMPOSE_UPDATE 控制：
//   - auto（默认）：容器带 compose 标签且宿主 compose 可用时走 compose 链路，
//     否则或失败时回退容器链路（兜底）。是否重建由 compose 按镜像 ID 对比决定。
//   - force：同 auto，但 up 时加 --force-recreate：不依赖 digest 检查的准确性，
//     每次触发必重建容器（latest 标签场景的保险档；代价是服务会闪断一次）。
//   - container：永远走容器链路（维持旧行为）。
//   - compose-only：compose 链路失败不兜底，直接报错（严格 IaC 场景）。

const (
	ComposeStrategyAuto      = "auto"
	ComposeStrategyForce     = "force"
	ComposeStrategyContainer = "container"
	ComposeStrategyOnly      = "compose-only"
)

// ComposeUpdateStrategy 读取并校验策略配置，非法值回退 auto。
func ComposeUpdateStrategy() string {
	switch strings.TrimSpace(strings.ToLower(os.Getenv("COMPOSE_UPDATE"))) {
	case ComposeStrategyForce:
		return ComposeStrategyForce
	case ComposeStrategyContainer:
		return ComposeStrategyContainer
	case ComposeStrategyOnly:
		return ComposeStrategyOnly
	default:
		return ComposeStrategyAuto
	}
}

// composeLabels 从容器 inspect 结果提取 compose 路由所需标签。
type composeLabels struct {
	Project    string
	Service    string
	WorkingDir string
	ConfigFile string
}

func composeLabelsFrom(inspected *dockerTypes.ContainerJSON) composeLabels {
	if inspected == nil || inspected.Config == nil {
		return composeLabels{}
	}
	return ComposeLabelsOf(inspected.Config.Labels)
}

// ComposeLabelsOf 从 compose 标签 map 提取路由所需标签。
// compose 标签的键名字符串全项目只在这一处定义；容器列表（composeproject.go）
// 与容器 inspect（composeLabelsFrom）共用它，避免三处重复。
func ComposeLabelsOf(labels map[string]string) composeLabels {
	return composeLabels{
		Project:    labels["com.docker.compose.project"],
		Service:    labels["com.docker.compose.service"],
		WorkingDir: labels["com.docker.compose.project.working_dir"],
		ConfigFile: labels["com.docker.compose.project.config_files"],
	}
}

// routable 判断标签是否足以支撑 compose 更新：项目、服务、工作目录缺一不可。
// config_files 缺失时 compose 会用 working_dir 下的默认文件名，可以接受。
func (l composeLabels) routable() bool {
	return l.Project != "" && l.Service != "" && l.WorkingDir != ""
}

// ---- compose 命令探测与执行 ----

// composeCommand 探测宿主可用的 compose 入口。
// 优先 docker compose（v2 插件），回退 docker-compose（v1 独立二进制）。
// 返回 (命令前缀, 是否可用)。
// composeProbeTimeout 是 compose 探测命令（compose version）的上限。
// docker CLI 在某些环境下会挂死（如 daemon 不可达时插件初始化卡住），
// 无超时的探测会拖住更新链路。
const composeProbeTimeout = 5 * time.Second

func composeCommand() ([]string, bool) {
	if _, err := exec.LookPath("docker"); err == nil {
		if finishProbe("docker", []string{"compose", "version", "--short"}) {
			return []string{"docker", "compose"}, true
		}
	}
	if _, err := exec.LookPath("docker-compose"); err == nil {
		if finishProbe("docker-compose", []string{"version", "--short"}) {
			return []string{"docker-compose"}, true
		}
	}
	return nil, false
}

// finishProbe 带超时地运行探测命令（exec.CommandContext，超时自动 kill）。
func finishProbe(bin string, args []string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), composeProbeTimeout)
	defer cancel()
	return exec.CommandContext(ctx, bin, args...).Run() == nil
}

// composeSafeTokenRE 校验标签值可以安全地作为命令行参数传递。
// 只放行常见文件名/项目名字符；因为参数不进 shell，这里主要防
// 以 - 开头的参数注入（被 compose 当成自己的选项解析）。
var composeSafeTokenRE = regexp.MustCompile(`^[A-Za-z0-9._+/@-]+$`)

func composeSafeToken(value string) bool {
	return value != "" && !strings.HasPrefix(value, "-") && composeSafeTokenRE.MatchString(value)
}

// ComposeCommandPublic 对外暴露 compose 探测结果（dt_info 用），
// 返回 (命令前缀, 是否可用)。
func ComposeCommandPublic() ([]string, bool) {
	return composeCommand()
}

// composeConfigFileArgs 从 config_files 标签构造 -f 参数序列。
// 标签值按冒号分隔多文件；每个路径要求是绝对路径且当前进程可见（os.Stat），
// 任一校验失败即报错——宁可拒绝执行，也不让 compose 在错误的工作目录里
// 按默认文件名猜配置、更新到错误的项目。参数走 exec 数组，不进 shell。
func composeConfigFileArgs(configFiles string) ([]string, error) {
	if strings.TrimSpace(configFiles) == "" {
		return nil, nil
	}
	var args []string
	for _, f := range strings.Split(configFiles, ":") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		if !filepath.IsAbs(f) {
			return nil, fmt.Errorf("compose config_files 含相对路径 %q，拒绝执行（标签值: %s）", f, configFiles)
		}
		if info, err := os.Stat(f); err != nil || info.IsDir() {
			return nil, fmt.Errorf("compose 文件 %q 在 DTool 进程内不可见或不是普通文件，拒绝执行（标签值: %s）", f, configFiles)
		}
		args = append(args, "-f", f)
	}
	return args, nil
}

// composeServiceUpdate 对单个服务执行 compose pull + up -d。
// workingDir 即容器标签里的 project.working_dir；DTool 通常以宿主根目录
// 挂载或直接运行在宿主上，相对路径按进程工作目录解析由 compose 自行处理。
// forceRecreate=true 时 up 加 --force-recreate：不看镜像 ID 是否变化，
// 每次必重建容器（latest 场景保险档，代价是服务闪断一次）。
func composeServiceUpdate(ctx context.Context, l composeLabels, forceRecreate bool) (output string, err error) {
	base, ok := composeCommand()
	if !ok {
		return "", fmt.Errorf("宿主环境没有可用的 docker compose（既无 v2 插件也无 docker-compose）")
	}
	if !composeSafeToken(l.Project) || !composeSafeToken(l.Service) {
		return "", fmt.Errorf("compose 标签含非法字符，拒绝执行（project=%q service=%q）", l.Project, l.Service)
	}
	fileArgs, err := composeConfigFileArgs(l.ConfigFile)
	if err != nil {
		return "", err
	}

	run := func(args ...string) (string, error) {
		cmdArgs := append(append(append([]string{}, base...), fileArgs...), args...)
		cmd := exec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...)
		cmd.Dir = l.WorkingDir
		cmd.Env = append(os.Environ(),
			"COMPOSE_PROJECT_NAME="+l.Project,
		)
		out, execErr := cmd.CombinedOutput()
		return strings.TrimSpace(string(out)), execErr
	}

	pullOut, pullErr := run("pull", "--quiet", l.Service)
	if pullErr != nil {
		return pullOut, fmt.Errorf("compose pull 失败: %w", pullErr)
	}
	upArgs := []string{"up", "-d", "--no-deps"}
	if forceRecreate {
		upArgs = append(upArgs, "--force-recreate")
	}
	upOut, upErr := run(append(upArgs, l.Service)...)
	if upErr != nil {
		return pullOut + "\n" + upOut, fmt.Errorf("compose up 失败: %w", upErr)
	}
	return strings.TrimSpace(pullOut + "\n" + upOut), nil
}

// tryComposeUpdate 尝试 compose 链路更新；返回 (是否已由 compose 完成, 输出, 错误)。
// 标签不齐或宿主根本没有 compose CLI 时返回 completed=false 且 err=nil，
// 由调用方走容器链路兜底；compose CLI 存在但执行失败时返回明确错误，
// 不再静默降级（防止 compose 管理的容器脱离 compose 管控）。
func tryComposeUpdate(ctx context.Context, serviceContext *svc.ServiceContext, inspected *dockerTypes.ContainerJSON, containerName, taskID string) (completed bool, output string, err error) {
	labels := composeLabelsFrom(inspected)
	if !labels.routable() {
		return false, "容器缺少 compose 项目/服务标签", nil
	}
	if _, ok := composeCommand(); !ok {
		return false, "宿主环境没有可用的 docker compose，按容器方式更新兜底", nil
	}
	strategy := ComposeUpdateStrategy()
	if strategy == ComposeStrategyContainer {
		return false, "COMPOSE_UPDATE=container，按容器方式更新", nil
	}

	report := func(pct int, msg, detail string, done bool) {
		serviceContext.UpdateProgress(taskID, svc.TaskProgress{
			TaskID:     taskID,
			Percentage: pct,
			Name:       containerName,
			Message:    msg,
			DetailMsg:  detail,
			IsDone:     done,
		})
	}
	report(15, "Compose 更新：探测 compose 环境", "项目 "+labels.Project+"，服务 "+labels.Service, false)
	out, execErr := composeServiceUpdate(ctx, labels, ComposeUpdateStrategy() == ComposeStrategyForce)
	if execErr == nil {
		report(90, "Compose 更新完成", out, false)
		return true, out, nil
	}
	if strategy == ComposeStrategyOnly {
		return true, out, fmt.Errorf("该容器由 compose 管理，compose 更新失败，请检查 compose 配置（COMPOSE_UPDATE=compose-only 不回退）: %w；compose 输出摘要: %s", execErr, composeOutputSummary(out))
	}
	// compose 管理的容器不再静默降级到 stop→rename→create 的容器重建：
	// 那会让容器脱离 compose 管控、与 1Panel 等面板状态反复打架。
	// 失败时直接返回明确错误，并附上 compose 输出摘要方便排查。
	logx.Errorf("compose 更新失败: %v\n%s", execErr, out)
	return true, out, fmt.Errorf("该容器由 compose 管理，compose 更新失败，请检查 compose 配置: %w；compose 输出摘要: %s", execErr, composeOutputSummary(out))
}

// composeOutputSummary 把 compose 输出压成单行摘要（错误信息用，防止刷屏）。
func composeOutputSummary(out string) string {
	line := strings.Join(strings.Fields(strings.TrimSpace(out)), " ")
	if len(line) > 300 {
		line = line[:300] + "…"
	}
	return line
}

// EnsureComposeCli 在 PATH 里找不到 compose 时给出明确指引的报错文案。
func EnsureComposeCli() error {
	if _, ok := composeCommand(); !ok {
		return fmt.Errorf("宿主环境没有可用的 docker compose；请挂载 docker CLI 或安装 docker-compose 后重试")
	}
	return nil
}

// composeWorkingDirExists 调试用：确认 compose 文件目录在当前进程可见。
func composeWorkingDirExists(l composeLabels) bool {
	if l.WorkingDir == "" {
		return false
	}
	info, err := os.Stat(l.WorkingDir)
	return err == nil && info.IsDir()
}
