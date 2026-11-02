package utiles

import (
	"context"
	"strings"
	"sync"

	"github.com/DongShull/DTool/internal/svc"
)

// containerMutationMu 串行化所有"改容器状态"的动作：更新、恢复、改名、启停、重启。
//
// 为什么需要：更新链路的窗口是 stop → rename(旧容器) → create(原名) → start，
// 这段时间里名字处于中间态。此前更新只有 per-ID 锁、恢复有全局锁，而
// rename/start/stop 完全无锁——更新进行到一半被 rename 一次，create 就会
// 因为"name already in use"失败并触发回滚，用户看到的是"更新失败"，
// 真正原因却是一次无关的改名。
//
// 实现选择：一把全局锁而不是按名字细粒度锁。理由是可证明无死锁（单锁、
// 只在短窗口内持有），且这些操作本来就是人工触发的低频动作；代价是恢复
// 期间其它容器的启停会排队等待。
var containerMutationMu sync.Mutex

// LockContainerMutation 获取"改容器状态"的全局锁，返回解锁函数。
// 调用方必须成对使用（推荐 defer unlock()），并且只在真正会改动容器的
// 短窗口内持有——不要跨镜像拉取这种分钟级操作持锁。
func LockContainerMutation() func() {
	containerMutationMu.Lock()
	return containerMutationMu.Unlock
}

// ResolveContainerName 把容器 ID 或名字解析成容器名（去掉前导 /）。
// 用于把"按 ID 发起的操作"归一到同一个可读名字上（进度过滤、审计、日志）。
// 解析失败时回退原值，保证调用方永远拿到一个非空 key（除非入参为空）。
func ResolveContainerName(serviceContext *svc.ServiceContext, idOrName string) string {
	key := strings.TrimPrefix(strings.TrimSpace(idOrName), "/")
	if key == "" || serviceContext == nil || serviceContext.DockerClient == nil {
		return key
	}
	inspectCtx, cancel := dockerTimeout(context.Background(), dockerInspectTimeout)
	defer cancel()
	inspected, err := serviceContext.DockerClient.ContainerInspect(inspectCtx, key)
	if err != nil || strings.TrimSpace(inspected.Name) == "" {
		return key
	}
	return strings.TrimPrefix(inspected.Name, "/")
}
