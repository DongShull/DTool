package utiles

import (
	"context"
	"os"
	"regexp"
	"strings"

	"github.com/DongShull/DTool/internal/svc"
	"github.com/zeromicro/go-zero/core/logx"
)

var (
	selfContainerIDLong  = regexp.MustCompile(`[0-9a-f]{64}`)
	selfContainerIDShort = regexp.MustCompile(`^[0-9a-f]{12}$`)
)

// DetectSelfContainerID 推断自身进程所在容器的 ID。
// 优先从 /proc/self/mountinfo 与 /proc/self/cgroup 里提取 docker/containerd
// 的 64 位长 ID；取不到时回退 HOSTNAME（容器内默认等于 12 位短 ID）。
// 宿主上裸跑（非容器部署）时返回空字符串，此时所有自身保护判断自动放行。
func DetectSelfContainerID() string {
	for _, procFile := range []string{"/proc/self/mountinfo", "/proc/self/cgroup"} {
		data, err := os.ReadFile(procFile)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			if !strings.Contains(line, "docker") && !strings.Contains(line, "containerd") {
				continue
			}
			// cgroup v2 的段形如 .../docker-<64hex>.scope，v1 形如 .../docker/<64hex>。
			if id := selfContainerIDLong.FindString(line); id != "" {
				return id
			}
		}
	}
	if host := strings.TrimSpace(os.Getenv("HOSTNAME")); selfContainerIDShort.MatchString(host) {
		return host
	}
	return ""
}

// IsSelfContainer 判断 id 是否指向自身容器。
// 两边都可能是长 ID（64 位）或短 ID（12 位），统一小写后做双向前缀匹配。
func IsSelfContainer(selfID, id string) bool {
	selfID = strings.ToLower(strings.TrimSpace(selfID))
	id = strings.ToLower(strings.TrimSpace(id))
	if selfID == "" || id == "" {
		return false
	}
	return selfID == id || strings.HasPrefix(selfID, id) || strings.HasPrefix(id, selfID)
}

// SelfContainerName 返回自身容器名（去掉前导 /）。
// 不在容器内、Docker 不可用或查询失败时返回空字符串（此时调用方按无保护处理）。
func SelfContainerName(serviceContext *svc.ServiceContext) string {
	selfID := DetectSelfContainerID()
	if selfID == "" || serviceContext == nil || serviceContext.DockerClient == nil {
		return ""
	}
	inspectCtx, cancel := dockerTimeout(context.Background(), dockerListTimeout)
	defer cancel()
	inspected, err := serviceContext.DockerClient.ContainerInspect(inspectCtx, selfID)
	if err != nil {
		logx.Errorf("获取自身容器信息失败（自身保护降级为仅镜像名判断）: %v", err)
		return ""
	}
	return strings.TrimPrefix(inspected.Name, "/")
}
