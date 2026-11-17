package utiles

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/DongShull/DTool/internal/svc"
	dockerTypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	dockerMsgType "github.com/docker/docker/pkg/jsonmessage"
	"github.com/zeromicro/go-zero/core/logx"
)

// 同一个容器只允许一个更新流程，避免手动更新与自动更新并发改名/创建。
var containerUpdateLocks sync.Map

func lockContainerUpdate(id string) func() {
	value, _ := containerUpdateLocks.LoadOrStore(id, &sync.Mutex{})
	mutex := value.(*sync.Mutex)
	mutex.Lock()
	return mutex.Unlock
}

// containerUpdateState 记录重建链路中旧/新容器的中间态，供回滚判断恢复到哪一步。
type containerUpdateState struct {
	oldName    string
	backupName string
	renamed    bool
	newCreated bool
	wasRunning bool
	wasPaused  bool
}

// containerUpdateProgress 封装任务进度上报与取消检查。
type containerUpdateProgress struct {
	serviceContext *svc.ServiceContext
	taskID         string
	name           string
}

func (p *containerUpdateProgress) report(percentage int, message, detailMsg string, isDone bool) {
	p.serviceContext.UpdateProgress(p.taskID, svc.TaskProgress{
		TaskID:     p.taskID,
		Percentage: percentage,
		Name:       p.name,
		Message:    message,
		DetailMsg:  detailMsg,
		IsDone:     isDone,
	})
}

func (p *containerUpdateProgress) cancelled(ctx context.Context) bool {
	if ctx != nil && ctx.Err() != nil {
		p.report(100, "任务已取消", "任务已取消", true)
		return true
	}
	return false
}

// UpdateContainer 执行一次容器更新：拉取新镜像 → 停止并改名旧容器 → 克隆配置创建新容器
// → 启动新容器 → 删除旧容器 → 清理旧镜像。任一步失败都会尝试把旧容器恢复到原名并重新启动。
// ctx 支持取消：任务被取消时在步骤边界退出。
func UpdateContainer(serviceContext *svc.ServiceContext, ctx context.Context, id string, name string, imageNameAndTag string, delOldContainer bool, taskID string) error {
	unlock := lockContainerUpdate(id)
	defer unlock()

	// 允许调用方不传 ctx（如测试或内部调用）：回退到 Background，避免空指针。
	if ctx == nil {
		ctx = context.Background()
	}

	progress := &containerUpdateProgress{serviceContext: serviceContext, taskID: taskID, name: name}

	// P0 自身保护：更新链路会 stop→rename→remove 目标容器，若目标是 DTool
	// 自己，进程会随容器一起死在半路，回滚代码永远执行不到（旧容器会以
	// 备份名遗留在停止状态）。自身更新请走程序自更新（PUT /api/program）
	// 或宿主侧 compose，二进制替换不依赖容器重建。
	// 自身保护用 IsSelfTarget：先 ID 匹配，再用 Inspect 把目标（可能是容器名）
	// 解析成真实容器 ID 比对，防止用户传 DTool 容器名绕过保护。
	// 注意自身检查必须在 requireDocker 之前：ID/短 ID 命中不依赖 Docker 客户端。
	if IsSelfTarget(serviceContext, id) {
		selfID := DetectSelfContainerID()
		logx.Infof("[自身保护] 拒绝更新自身容器：self=%s target=%s(%s)", selfID, name, id)
		progress.report(100, "拒绝更新自身容器", "目标是 DTool 自身容器；请使用程序自更新接口（PUT /api/program）或宿主侧 compose 更新", true)
		return fmt.Errorf("拒绝更新自身容器：请使用程序自更新接口")
	}

	// nil client 前置检查：与 RestoreContainer 一致的明确错误，避免后续 panic。
	if err := requireDocker(serviceContext); err != nil {
		progress.report(100, "更新失败", err.Error(), true)
		return err
	}

	progress.report(0, "正在连接Docker", "正在连接Docker", false)
	if progress.cancelled(ctx) {
		return ctx.Err()
	}

	state, inspectedContainer, warnings, err := validateAndInspect(serviceContext, ctx, id, name, imageNameAndTag, progress)
	if err != nil {
		return err
	}
	if strings.TrimSpace(imageNameAndTag) == "" {
		imageNameAndTag = inspectedContainer.Config.Image
	}

	// ---- Compose 链路由：标签齐全且可用时优先走 compose pull/up，
	// 失败或不可用时回退到下方的容器链路兑底 ----
	completed, composeOut, composeErr := tryComposePath(serviceContext, ctx, id, taskID, state, inspectedContainer, progress)
	if composeErr != nil {
		return composeErr
	}
	if completed {
		return nil
	}
	warnings = append(warnings, composeOut)

	// 容器链路兑底：拉取 → 停止 → 改名 → 重建 → 启动 → 清理。
	return containerRecreatePath(serviceContext, ctx, id, taskID, imageNameAndTag, delOldContainer, state, inspectedContainer, warnings, progress)
}

// validateAndInspect 拉取并校验旧容器信息，确定镜像名、备份名与初始回滚状态。
func validateAndInspect(serviceContext *svc.ServiceContext, ctx context.Context, id string, name string, imageNameAndTag string, progress *containerUpdateProgress) (*containerUpdateState, *dockerTypes.ContainerJSON, []string, error) {
	negotiateCtx, negotiateCancel := dockerTimeout(ctx, dockerNegotiateTimeout)
	serviceContext.DockerClient.NegotiateAPIVersion(negotiateCtx)
	negotiateCancel()

	// 先记录旧容器信息：旧镜像 ID 用于更新后的清理，旧容器名用于失败回滚
	inspectedContainer, err := serviceContext.DockerClient.ContainerInspect(ctx, id)
	if err != nil {
		progress.report(0, "获取容器信息失败", err.Error(), true)
		logx.Errorf("获取容器信息失败: %v", err)
		return nil, nil, nil, fmt.Errorf("inspect container %s: %w", id, err)
	}
	if inspectedContainer.Config == nil {
		err = fmt.Errorf("旧容器配置为空，无法更新")
		progress.report(0, "获取容器配置失败", err.Error(), true)
		return nil, nil, nil, err
	}
	if inspectedContainer.HostConfig == nil {
		err = fmt.Errorf("旧容器 HostConfig 为空，无法克隆配置")
		progress.report(0, "获取容器配置失败", err.Error(), true)
		return nil, nil, nil, err
	}
	if strings.TrimSpace(imageNameAndTag) == "" && strings.TrimSpace(inspectedContainer.Config.Image) == "" {
		err = fmt.Errorf("镜像名称为空")
		progress.report(0, "更新失败", err.Error(), true)
		return nil, nil, nil, err
	}
	oldContainerName := strings.TrimPrefix(inspectedContainer.Name, "/")
	if oldContainerName == "" {
		oldContainerName = name
	}
	state := &containerUpdateState{
		oldName:    oldContainerName,
		backupName: oldContainerName + "-" + time.Now().Format("2006-01-02-15-04-05.000000000"),
		wasRunning: inspectedContainer.State != nil && inspectedContainer.State.Running,
		wasPaused:  inspectedContainer.State != nil && inspectedContainer.State.Paused,
	}
	var warnings []string
	isCompose := inspectedContainer.Config.Labels["com.docker.compose.project"] != ""
	if isCompose {
		warnings = append(warnings, "该容器由 Compose 管理，本次按容器方式重建；建议优先使用 compose 更新链路")
	}
	return state, &inspectedContainer, warnings, nil
}

// tryComposePath 在容器由 Compose 管理且策略可用时尝试 compose pull/up 更新。
// completed 表示已走完 compose 链路（无论成败）；未走完时 composeOut 为原因说明。
func tryComposePath(serviceContext *svc.ServiceContext, ctx context.Context, id string, taskID string, state *containerUpdateState, inspectedContainer *dockerTypes.ContainerJSON, progress *containerUpdateProgress) (completed bool, composeOut string, composeErr error) {
	isCompose := inspectedContainer.Config.Labels["com.docker.compose.project"] != ""
	if strategy := ComposeUpdateStrategy(); !isCompose || strategy == ComposeStrategyContainer {
		return false, "", nil
	}
	progress.report(10, "正在检查 Compose 可用性", "尝试以 compose 方式更新", false)
	// compose pull+up 是分钟级操作且全程持全局变更锁：给整个 compose 操作加
	// 5 分钟上限（与 runComposeProject 的 composeActionTimeout 对齐），避免
	// 一次卡死的 compose 拖死所有其它容器的启停/更新。
	// 取舍说明：compose 的 pull 与 up 在同一条子命令链里（compose pull 后
	// compose up -d 内部自行判断重建），无法像容器链路那样把镜像拉取移出
	// 锁外，只能以超时兜底，故保持整段持锁 + 超时。
	composeCtx, composeCancel := context.WithTimeout(ctx, composeActionTimeout)
	defer composeCancel()
	unlockCompose := LockContainerMutation()
	completed, composeOut, composeErr = tryComposeUpdate(composeCtx, serviceContext, inspectedContainer, state.oldName, taskID)
	unlockCompose()
	if completed {
		if composeErr != nil {
			progress.report(100, "Compose 更新失败", composeErr.Error(), true)
			return completed, composeOut, composeErr
		}
		// compose up -d 成功后观察新容器是否稳定/健康，复用容器链路的健康等待。
		if state.wasRunning && !state.wasPaused {
			progress.report(92, "正在等待容器健康", "正在等待新容器通过健康检查", false)
			if healthErr := waitForContainerHealthy(ctx, serviceContext, state.oldName, inspectedContainer.Config.Healthcheck); healthErr != nil {
				detail := composeOut + "；健康检查失败: " + healthErr.Error()
				progress.report(100, "Compose 更新完成但容器未通过健康检查", detail, true)
				return completed, composeOut, healthErr
			}
		}
		detail := "已由 compose 更新"
		if composeOut != "" {
			detail += "\n" + composeOut
		}
		progress.report(100, "更新成功（compose）", detail, true)
		return completed, composeOut, nil
	}
	// 未走完 compose 链路：原因记入警告，继续容器链路兑底。
	return completed, composeOut, nil
}

// containerRecreatePath 容器重建链路：拉取新镜像 → 停止并改名旧容器 → 克隆配置创建
// 新容器 → 启动新容器 → 删除旧容器 → 清理旧镜像。失败时通过 rollback 恢复旧容器。
func containerRecreatePath(serviceContext *svc.ServiceContext, ctx context.Context, id string, taskID string, imageNameAndTag string, delOldContainer bool, state *containerUpdateState, inspectedContainer *dockerTypes.ContainerJSON, warnings []string, progress *containerUpdateProgress) error {
	timeout := 10
	signal := "SIGTERM"
	oldContainerName := state.oldName
	oldImageID := inspectedContainer.Image

	// rollback 只恢复本流程真正改过名的旧容器；原本停止的容器不会被误启动。
	rollback := func(reason string) {
		state.rollback(serviceContext, id, oldContainerName, reason, progress)
	}

	// 1. 拉取新镜像（流式进度 10% → 25%）
	progress.report(10, "正在拉取新镜像", "正在拉取新镜像", false)
	reader, err := serviceContext.DockerClient.ImagePull(ctx, imageNameAndTag, image.PullOptions{})
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			progress.report(100, "任务已取消", "任务已在拉取镜像阶段取消", true)
			return ctx.Err()
		}
		progress.report(100, "拉取镜像失败", err.Error(), true)
		logx.Errorf("拉取镜像失败: %s", err)
		return fmt.Errorf("pull image %s: %w", imageNameAndTag, err)
	}
	defer func() {
		if closeErr := reader.Close(); closeErr != nil {
			logx.Errorf("关闭镜像拉取流失败: %v", closeErr)
		}
	}()
	if err = decodePullResp(reader, serviceContext, ctx, taskID, pullProgressBase); err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			// 取消发生在拉取阶段时，错误本质是 context.Canceled，
			// 如实报“任务已取消”而不是误导性的“拉取镜像失败”。
			progress.report(100, "任务已取消", "任务已在拉取镜像阶段取消", true)
			return ctx.Err()
		}
		progress.report(100, "拉取镜像失败", err.Error(), true)
		logx.Errorf("拉取镜像失败: %s", err)
		return err
	}
	if progress.cancelled(ctx) {
		return ctx.Err()
	}

	// 2. 停止旧容器（原本停止的容器跳过，避免更新后把它误启动）
	// 从这里开始进入"名字中间态"（旧容器改名→新容器用原名创建），必须持有
	// 全局变更锁，否则并发的 rename/start/stop（或另一次恢复）会让 create
	// 因名字被占用而失败，并把用户看到的错误误导成"更新失败"。
	// 镜像拉取是分钟级操作，故意放在取锁之前。
	unlockMutation := LockContainerMutation()
	defer unlockMutation()
	progress.report(30, "正在停止容器", "正在停止容器", false)
	stopOptions := container.StopOptions{
		Signal:  signal,
		Timeout: &timeout,
	}
	if state.wasRunning {
		// Docker 不允许直接 stop paused 容器，先取消暂停；回滚/新容器启动后会恢复。
		if state.wasPaused {
			if err = serviceContext.DockerClient.ContainerUnpause(ctx, id); err != nil {
				rollback(fmt.Sprintf("取消旧容器暂停失败: %v", err))
				return fmt.Errorf("unpause container %s: %w", id, err)
			}
		}
		if err = serviceContext.DockerClient.ContainerStop(ctx, id, stopOptions); err != nil {
			rollback(fmt.Sprintf("停止容器失败: %v", err))
			return fmt.Errorf("stop container %s: %w", id, err)
		}
	}

	// 3. 重命名旧容器（腾出原名）
	progress.report(40, "正在重命名旧容器", "正在重命名旧容器", false)
	if err = serviceContext.DockerClient.ContainerRename(ctx, id, state.backupName); err != nil {
		// 还没改名成功，旧容器仍是原名，只恢复它原来的运行状态
		rollback(fmt.Sprintf("重命名旧容器失败: %v", err))
		return fmt.Errorf("rename container %s: %w", id, err)
	}
	state.renamed = true

	// 4. 克隆旧容器配置创建新容器（只替换镜像）
	progress.report(60, "正在创建新容器", "正在创建新容器", false)
	inspectedContainer.Config.Hostname = ""
	inspectedContainer.Config.Image = imageNameAndTag
	inspectedContainer.Image = imageNameAndTag
	config := inspectedContainer.Config
	hostConfig := inspectedContainer.HostConfig
	var endpoints map[string]*network.EndpointSettings
	if inspectedContainer.NetworkSettings != nil {
		endpoints = sanitizeEndpointSettings(inspectedContainer.NetworkSettings.Networks)
	}
	networkingConfig := &network.NetworkingConfig{
		EndpointsConfig: endpoints,
	}
	if _, err = serviceContext.DockerClient.ContainerCreate(ctx, config, hostConfig, networkingConfig, nil, oldContainerName); err != nil {
		rollback(fmt.Sprintf("创建新容器失败: %v", err))
		return fmt.Errorf("create container %s: %w", oldContainerName, err)
	}
	state.newCreated = true

	// 5. 启动新容器并等待健康检查；原 paused 容器启动后再暂停。
	if state.wasRunning {
		progress.report(80, "正在启动新容器", "正在启动新容器", false)
		if err = serviceContext.DockerClient.ContainerStart(ctx, oldContainerName, container.StartOptions{}); err != nil {
			// 新容器已创建但起不来：rollback 会移除它并恢复旧容器。
			rollback(fmt.Sprintf("启动新容器失败: %v", err))
			return fmt.Errorf("start container %s: %w", oldContainerName, err)
		}
		if !state.wasPaused {
			progress.report(85, "正在等待容器健康", "正在等待新容器通过健康检查", false)
			if err = waitForContainerHealthy(ctx, serviceContext, oldContainerName, inspectedContainer.Config.Healthcheck); err != nil {
				rollback(fmt.Sprintf("新容器健康检查失败: %v", err))
				return fmt.Errorf("wait container %s healthy: %w", oldContainerName, err)
			}
		}
		if state.wasPaused {
			if err = serviceContext.DockerClient.ContainerPause(ctx, oldContainerName); err != nil {
				rollback(fmt.Sprintf("恢复新容器暂停状态失败: %v", err))
				return fmt.Errorf("pause container %s: %w", oldContainerName, err)
			}
		}
	}

	// 6. 删除旧容器（默认保留开关可关闭）
	if delOldContainer {
		if err = serviceContext.DockerClient.ContainerRemove(ctx, id, container.RemoveOptions{}); err != nil {
			// 旧容器删除失败不算更新失败，但需要告知
			logx.Errorf("删除旧容器失败: %v", err)
			warning := fmt.Sprintf("旧容器删除失败，请手动清理 %s: %v", state.backupName, err)
			warnings = append(warnings, warning)
			progress.report(90, "更新成功（旧容器删除失败）", warning, false)
		}
	}

	// 7. 清理旧镜像：仅当旧容器已删除、且旧镜像不再被任何容器引用时删除
	if delOldContainer && os.Getenv("CLEANUP_OLD_IMAGE") != "false" {
		progress.report(95, "正在清理旧镜像", "正在清理旧镜像", false)
		if cleanupMsg := cleanupOldImage(serviceContext, oldImageID); cleanupMsg != "" {
			warnings = append(warnings, cleanupMsg)
			progress.report(95, "更新成功（旧镜像未清理）", cleanupMsg, false)
		}
	}

	detail := "更新成功"
	if len(warnings) > 0 {
		detail = strings.Join(warnings, "；\n")
	}
	progress.report(100, "更新成功", detail, true)
	return nil
}

// rollback 只恢复本流程真正改过名的旧容器；原本停止的容器不会被误启动。
func (s *containerUpdateState) rollback(serviceContext *svc.ServiceContext, id string, oldContainerName string, reason string, progress *containerUpdateProgress) {
	bg, bgCancel := dockerTimeout(context.Background(), dockerMutationTimeout)
	defer bgCancel()
	// 新容器还占着原名时先移除，否则旧容器改不回去。
	if s.newCreated {
		if removeErr := serviceContext.DockerClient.ContainerRemove(bg, oldContainerName, container.RemoveOptions{Force: true}); removeErr != nil {
			logx.Errorf("回滚移除新容器失败: %v", removeErr)
		}
		s.newCreated = false
	}
	if s.renamed {
		if renameErr := serviceContext.DockerClient.ContainerRename(bg, id, oldContainerName); renameErr != nil {
			logx.Errorf("回滚重命名失败: %v", renameErr)
			progress.report(100, "更新失败且回滚失败，请手动处理旧容器 "+s.backupName, reason, true)
			return
		}
		s.renamed = false
	}
	if s.wasRunning {
		// 原本是 paused 的容器，先取消暂停再确保运行，最后恢复暂停状态。
		if s.wasPaused {
			_ = serviceContext.DockerClient.ContainerUnpause(bg, id)
		}
		current, inspectErr := serviceContext.DockerClient.ContainerInspect(bg, id)
		if inspectErr != nil || current.State == nil || !current.State.Running {
			if startErr := serviceContext.DockerClient.ContainerStart(bg, id, container.StartOptions{}); startErr != nil {
				logx.Errorf("回滚启动失败: %v", startErr)
				progress.report(100, "更新失败，旧容器已改回原名但启动失败", reason+"；启动错误: "+startErr.Error(), true)
				return
			}
		}
		if s.wasPaused {
			if pauseErr := serviceContext.DockerClient.ContainerPause(bg, id); pauseErr != nil {
				logx.Errorf("回滚恢复暂停状态失败: %v", pauseErr)
				progress.report(100, "更新失败，旧容器已恢复但暂停状态恢复失败", reason+"；暂停错误: "+pauseErr.Error(), true)
				return
			}
		}
	}
	progress.report(100, "更新失败，已回滚旧容器", reason, true)
}

// waitForContainerHealthy 在容器启动后确认它没有立刻退出。
// 有 HEALTHCHECK 时等待 healthy；没有时只观察 dockerStableWindow（2 秒），避免把“启动成功但马上崩”当成成功。
func waitForContainerHealthy(ctx context.Context, serviceContext *svc.ServiceContext, name string, healthcheck *container.HealthConfig) error {
	checkCtx, cancel := context.WithTimeout(ctx, dockerHealthWaitTimeout)
	defer cancel()

	hasHealthcheck := healthcheck != nil && len(healthcheck.Test) > 0 &&
		!(len(healthcheck.Test) == 1 && strings.EqualFold(healthcheck.Test[0], "NONE"))
	stableDeadline := time.Now().Add(dockerStableWindow)
	ticker := time.NewTicker(dockerHealthPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-checkCtx.Done():
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("等待容器健康超时")
		case <-ticker.C:
			inspected, err := serviceContext.DockerClient.ContainerInspect(checkCtx, name)
			if err != nil {
				return fmt.Errorf("检查新容器状态失败: %w", err)
			}
			if inspected.State == nil {
				return fmt.Errorf("新容器状态为空")
			}
			if !inspected.State.Running {
				return fmt.Errorf("新容器已退出，退出码 %d", inspected.State.ExitCode)
			}
			if hasHealthcheck && inspected.State.Health != nil {
				switch strings.ToLower(string(inspected.State.Health.Status)) {
				case "healthy":
					return nil
				case "unhealthy":
					return fmt.Errorf("新容器健康检查结果为 unhealthy")
				}
			} else if !hasHealthcheck && time.Now().After(stableDeadline) {
				return nil
			}
		}
	}
}

// cleanupOldImage 在没有任何容器引用 oldImageID 时删除该镜像。
// 返回补充信息（清理失败原因），失败不影响更新结果。
func cleanupOldImage(serviceContext *svc.ServiceContext, oldImageID string) string {
	if oldImageID == "" {
		return ""
	}
	listCtx, listCancel := dockerTimeout(context.Background(), dockerListTimeout)
	containers, err := serviceContext.DockerClient.ContainerList(listCtx, container.ListOptions{All: true})
	listCancel()
	if err != nil {
		logx.Errorf("清理旧镜像前获取容器列表失败: %v", err)
		return "旧镜像清理检查失败（未确认是否仍被引用）: " + err.Error()
	}
	for _, c := range containers {
		if c.ImageID == oldImageID {
			// 仍被引用，不删
			return ""
		}
	}
	removeCtx, removeCancel := dockerTimeout(context.Background(), dockerMutationTimeout)
	defer removeCancel()
	if _, err := serviceContext.DockerClient.ImageRemove(removeCtx, oldImageID, image.RemoveOptions{}); err != nil {
		logx.Errorf("清理旧镜像失败（不影响更新结果）: %v", err)
		return "旧镜像清理失败: " + err.Error()
	}
	return ""
}

func containerStateByIDOrName(serviceContext *svc.ServiceContext, id string) (*dockerTypes.ContainerState, error) {
	inspectCtx, cancel := dockerTimeout(context.Background(), dockerInspectTimeout)
	defer cancel()
	inspected, err := serviceContext.DockerClient.ContainerInspect(inspectCtx, id)
	if err != nil {
		return nil, err
	}
	if inspected.Config == nil {
		// 与 UpdateContainer 的前置校验保持一致：配置为空视为容器不可操作。
		return nil, fmt.Errorf("容器配置为空，无法操作")
	}
	return inspected.State, nil
}

// EnsureContainerStarted 幂等启动：已在运行的容器直接成功（Docker API 对
// 重复 start 返回 304 类错误，这里先检查状态避免把幂等调用当失败报出）。
// paused 的容器无法直接启动，返回明确错误引导先恢复（unpause）。
func EnsureContainerStarted(serviceContext *svc.ServiceContext, id string) error {
	unlockMutation := LockContainerMutation()
	defer unlockMutation()
	state, err := containerStateByIDOrName(serviceContext, id)
	if err != nil {
		return err
	}
	if state != nil && state.Running {
		return nil
	}
	if state != nil && state.Paused {
		return fmt.Errorf("容器处于暂停状态，无法直接启动，请先恢复（unpause）")
	}
	requestCtx, cancel := dockerTimeout(context.Background(), dockerMutationTimeout)
	defer cancel()
	return serviceContext.DockerClient.ContainerStart(requestCtx, id, container.StartOptions{})
}

// EnsureContainerStopped 幂等停止：已停止的容器直接成功；paused 的容器
// Docker 不允许直接 stop，先取消暂停再停止（与更新链路的处理一致）。
func EnsureContainerStopped(serviceContext *svc.ServiceContext, id string) error {
	unlockMutation := LockContainerMutation()
	defer unlockMutation()
	state, err := containerStateByIDOrName(serviceContext, id)
	if err != nil {
		return err
	}
	if state == nil || (!state.Running && !state.Paused) {
		return nil
	}
	requestCtx, cancel := dockerTimeout(context.Background(), dockerMutationTimeout)
	defer cancel()
	if state.Paused {
		if err = serviceContext.DockerClient.ContainerUnpause(requestCtx, id); err != nil {
			return err
		}
	}
	timeout := 10
	signal := "SIGTERM"
	stopOptions := container.StopOptions{
		Signal:  signal,
		Timeout: &timeout,
	}
	return serviceContext.DockerClient.ContainerStop(requestCtx, id, stopOptions)
}

// pullProgressBase 是更新链路镜像拉取阶段展示的进度基准。
// decodePullResp 不再硬编码该值：多容器恢复等调用方可传入各自的基准，
// 避免进度被打回 25% 造成回跳。
const pullProgressBase = 25

// decodePullResp 解析镜像拉取流并刷新进度。basePercentage 为可选的进度基准，
// 不传时沿用 pullProgressBase（历史行为，保持 restorecontainer 等旧调用兼容）。
func decodePullResp(reader io.Reader, svcCtx *svc.ServiceContext, ctx context.Context, taskID string, basePercentage ...int) (err error) {
	base := pullProgressBase
	if len(basePercentage) > 0 {
		base = basePercentage[0]
	}
	decoder := json.NewDecoder(reader)
	var oldTaskProgress, result = svcCtx.GetProgress(taskID)
	if !result {
		oldTaskProgress = svc.TaskProgress{
			Percentage: 0,
			Name:       "",
			Message:    "",
			DetailMsg:  "",
			IsDone:     false,
		}
	}
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var msg dockerMsgType.JSONMessage
		if err = decoder.Decode(&msg); err != nil {
			if err == io.EOF {
				return nil
			}
			oldTaskProgress.Message = "拉取镜像失败"
			oldTaskProgress.DetailMsg = err.Error()
			oldTaskProgress.Percentage = base
			svcCtx.UpdateProgress(taskID, oldTaskProgress)
			logx.Errorf("解析镜像拉取响应失败: %s", err)
			return fmt.Errorf("拉取镜像失败: %w", err)
		}
		if msg.Error != nil {
			oldTaskProgress.Message = "拉取镜像失败"
			oldTaskProgress.DetailMsg = msg.Error.Error()
			oldTaskProgress.Percentage = base
			svcCtx.UpdateProgress(taskID, oldTaskProgress)
			logx.Errorf("registry 返回错误: %s", msg.Error)
			return fmt.Errorf("拉取镜像失败: %w", msg.Error)
		}
		var formattedMsg string
		if msg.Progress != nil {
			formattedMsg = fmt.Sprintf("进度%s: %s", msg.Status, msg.Progress.String())
		} else {
			formattedMsg = fmt.Sprintf("进度%s", msg.Status)
		}
		oldTaskProgress.DetailMsg = formattedMsg
		oldTaskProgress.Percentage = base
		svcCtx.UpdateProgress(taskID, oldTaskProgress)
		logx.Infof("拉取镜像进度\t %s: %s\n", msg.Status, msg.Progress)
	}
}
