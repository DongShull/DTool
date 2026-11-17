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
	selfContainerIDShort = regexp.MustCompile(`^[0-9a-f]{12}$`)
	// docker 会把 /etc/hostname、/etc/hosts、/etc/resolv.conf 从
	// /var/lib/docker/containers/<64hex>/ 绑定进容器；这条路径里的 ID 就是容器 ID。
	// 注意不能直接抓 mountinfo 里第一个 64 位 hex：overlay2 的 upperdir/cache-id
	// 也是 64 位 hex，但那是层缓存目录名，跟容器 ID 无关。
	selfContainerMountID = regexp.MustCompile(`containers/([0-9a-f]{64})`)
	// cgroup v1：/docker/<64hex>；cgroup v2（systemd 驱动）：/docker-<64hex>.scope
	selfContainerCgroupID = regexp.MustCompile(`docker[-/]([0-9a-f]{64})`)
)

// DetectSelfContainerID 推断自身进程所在容器的 ID。
// 顺序：mountinfo 的 containers/<id> 绑定 → cgroup 的 docker[-/]<id> → HOSTNAME 短 ID。
// 宿主上裸跑（非容器部署）时返回空字符串，此时自身保护判断自动放行。
func DetectSelfContainerID() string {
	var mountinfo, cgroup, hostname string
	if data, err := os.ReadFile("/proc/self/mountinfo"); err == nil {
		mountinfo = string(data)
	}
	if data, err := os.ReadFile("/proc/self/cgroup"); err == nil {
		cgroup = string(data)
	}
	hostname = os.Getenv("HOSTNAME")
	return detectSelfContainerID(mountinfo, cgroup, hostname)
}

// detectSelfContainerID 是 DetectSelfContainerID 的纯函数内核，便于单测。
func detectSelfContainerID(mountinfo, cgroup, hostname string) string {
	if m := selfContainerMountID.FindStringSubmatch(mountinfo); m != nil {
		return m[1]
	}
	if m := selfContainerCgroupID.FindStringSubmatch(cgroup); m != nil {
		return m[1]
	}
	if h := strings.TrimSpace(hostname); selfContainerIDShort.MatchString(h) {
		return h
	}
	return ""
}

// InContainer 判断当前进程是否运行在容器内，用于 ID 探测失败时的告警。
func InContainer() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	if _, err := os.Stat("/run/.containerenv"); err == nil {
		return true
	}
	if data, err := os.ReadFile("/proc/self/mountinfo"); err == nil {
		// 容器根文件系统是 overlay；宿主上 /proc/self/mountinfo 的根通常是真实块设备。
		return strings.Contains(string(data), " - overlay overlay ")
	}
	return false
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

// IsSelfTarget 判断目标（容器 ID、短 ID 或容器名）是否指向自身容器。
// 先做 ID 双向前缀匹配（不依赖 Docker 可用）；未命中时用 Docker Inspect
// 把目标解析成真实容器 ID 再比对——这样用户传 DTool 容器名（如 "dtool"）
// 也无法绕过自身保护。inspect 失败（容器不存在、Docker 不可用等）时保守
// 兜底：用 SelfContainerName 的名字比对，宁可误拒也不放行自杀操作。
func IsSelfTarget(serviceContext *svc.ServiceContext, idOrName string) bool {
	selfID := DetectSelfContainerID()
	if selfID == "" {
		// 宿主裸跑：无自身容器可言，放行（与 DetectSelfContainerID 语义一致）。
		return false
	}
	target := strings.ToLower(strings.TrimSpace(idOrName))
	if target == "" {
		return false
	}
	if IsSelfContainer(selfID, target) {
		return true
	}
	if serviceContext == nil || serviceContext.DockerClient == nil {
		// 没有 Docker 客户端时只能靠 ID 匹配；ID 不匹配则放行。
		return false
	}
	inspectCtx, cancel := dockerTimeout(context.Background(), dockerInspectTimeout)
	defer cancel()
	inspected, err := serviceContext.DockerClient.ContainerInspect(inspectCtx, target)
	if err == nil && strings.TrimSpace(inspected.ID) != "" {
		// 成功解析出真实容器 ID：以 ID 比对为准（容器名可能被复用，ID 才是本体）。
		return IsSelfContainer(selfID, inspected.ID)
	}
	// inspect 失败，解析不出 ID：按名字兜底比对（restorecontainer 同款先例）。
	selfName := SelfContainerName(serviceContext)
	return selfName != "" && selfName == strings.TrimPrefix(target, "/")
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
