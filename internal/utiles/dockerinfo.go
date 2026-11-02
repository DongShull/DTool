package utiles

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/DongShull/DTool/internal/svc"
	MyType "github.com/DongShull/DTool/internal/types"
	dockerTypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/system"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/zeromicro/go-zero/core/logx"
)

// 类型别名：把 docker 的结构体在包内换个短名字，读起来不至于到处 dockerTypes.XXX。
type (
	dockerContainerJSON   = dockerTypes.ContainerJSON
	dockerNetworkSettings = dockerTypes.NetworkSettings
	dockerMountPoint      = dockerTypes.MountPoint
	dockerDiskUsage       = dockerTypes.DiskUsage
)

// requireDocker 统一做 Docker 客户端可用性检查，避免每个入口重复三段 nil 判断。
func requireDocker(serviceContext *svc.ServiceContext) error {
	if serviceContext == nil || serviceContext.DockerClient == nil {
		return fmt.Errorf("Docker 客户端未初始化，请检查 DOCKER_HOST 配置")
	}
	return nil
}

func diskUsageOptions() dockerTypes.DiskUsageOptions {
	// 空 Types 表示全部对象（镜像/容器/卷/构建缓存）。
	return dockerTypes.DiskUsageOptions{}
}

func jsonDecodeReader(r io.Reader, v interface{}) error {
	return json.NewDecoder(r).Decode(v)
}

func pidsLimit(hostConfig *container.HostConfig) int64 {
	if hostConfig == nil || hostConfig.Resources.PidsLimit == nil {
		return 0
	}
	return *hostConfig.Resources.PidsLimit
}

// ---- 系统概览 ----

// SystemOverview 汇总 Docker 版本、主机信息与磁盘占用。
func SystemOverview(ctx context.Context, serviceContext *svc.ServiceContext) (MyType.SystemOverview, error) {
	var out MyType.SystemOverview
	if err := requireDocker(serviceContext); err != nil {
		return out, err
	}
	callCtx, cancel := dockerTimeout(ctx, dockerListTimeout)
	defer cancel()

	version, err := serviceContext.DockerClient.ServerVersion(callCtx)
	if err != nil {
		return out, fmt.Errorf("获取 Docker 版本失败: %w", err)
	}
	info, err := serviceContext.DockerClient.Info(callCtx)
	if err != nil {
		return out, fmt.Errorf("获取 Docker 信息失败: %w", err)
	}
	containers, listErr := serviceContext.DockerClient.ContainerList(callCtx, container.ListOptions{All: true})
	if listErr != nil {
		logx.Errorf("获取容器列表失败（磁盘占用里的未使用镜像统计将不准确）: %v", listErr)
	}
	disk, diskErr := serviceContext.DockerClient.DiskUsage(callCtx, diskUsageOptions())
	if diskErr != nil {
		// 磁盘占用失败不影响整体：个别驱动/旧 daemon 不支持 df。
		logx.Errorf("获取磁盘占用失败: %v", diskErr)
	}

	out.Docker = MyType.DockerVersionInfo{
		Version:       version.Version,
		APIVersion:    version.APIVersion,
		MinAPIVersion: version.MinAPIVersion,
		Os:            version.Os,
		Arch:          version.Arch,
		KernelVersion: version.KernelVersion,
	}
	out.Info = hostInfoFrom(info)
	out.Disk = diskUsageFrom(disk, containers)
	return out, nil
}

func hostInfoFrom(info system.Info) MyType.HostInfo {
	return MyType.HostInfo{
		Name:              info.Name,
		ServerVersion:     info.ServerVersion,
		OperatingSystem:   info.OperatingSystem,
		OSType:            info.OSType,
		Architecture:      info.Architecture,
		NCPU:              info.NCPU,
		MemTotal:          info.MemTotal,
		DockerRootDir:     info.DockerRootDir,
		StorageDriver:     info.Driver,
		Containers:        info.Containers,
		ContainersRunning: info.ContainersRunning,
		ContainersPaused:  info.ContainersPaused,
		ContainersStopped: info.ContainersStopped,
		Images:            info.Images,
		Warnings:          len(info.Warnings),
	}
}

// diskUsageFrom 把 daemon 的 df 结果折算成"可回收空间"视角。
// 注意：DiskUsage.Images 不带 InUse 字段，需要自己用容器引用的 ImageID 反推。
func diskUsageFrom(disk dockerDiskUsage, containers []dockerTypes.Container) MyType.DiskUsageInfo {
	var out MyType.DiskUsageInfo
	out.LayersSize = disk.LayersSize
	out.Images = len(disk.Images)
	out.Containers = len(disk.Containers)
	out.Volumes = len(disk.Volumes)
	out.BuildCache = len(disk.BuildCache)

	inUse := imageInUseSet(containers)
	var unusedImages int64
	for _, img := range disk.Images {
		if img == nil {
			continue
		}
		if _, ok := inUse[img.ID]; !ok && img.Size > 0 {
			unusedImages += img.Size
		}
	}
	var stoppedSize int64
	for _, c := range disk.Containers {
		if c == nil {
			continue
		}
		if c.State != "running" {
			stoppedSize += c.SizeRw
		}
	}
	var volumesSize int64
	for _, v := range disk.Volumes {
		if v != nil && v.UsageData != nil {
			volumesSize += v.UsageData.Size
		}
	}
	out.Detail.UnusedImages = unusedImages
	out.Detail.StoppedLogs = stoppedSize
	out.Detail.Volumes = volumesSize
	out.Reclaimable = unusedImages + stoppedSize
	return out
}

func imageInUseSet(containers []dockerTypes.Container) map[string]struct{} {
	set := make(map[string]struct{}, len(containers))
	for _, c := range containers {
		set[c.ImageID] = struct{}{}
	}
	return set
}

// ---- 卷 ----

// ListVolumes 列出全部数据卷，并标出被哪些容器引用（含已停止容器）。
func ListVolumes(ctx context.Context, serviceContext *svc.ServiceContext) ([]MyType.VolumeInfo, error) {
	if err := requireDocker(serviceContext); err != nil {
		return nil, err
	}
	callCtx, cancel := dockerTimeout(ctx, dockerListTimeout)
	defer cancel()

	resp, err := serviceContext.DockerClient.VolumeList(callCtx, volume.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("获取卷列表失败: %w", err)
	}
	containers, err := serviceContext.DockerClient.ContainerList(callCtx, container.ListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("获取容器列表失败: %w", err)
	}
	return buildVolumeInfos(resp.Volumes, containers), nil
}

// buildVolumeInfos 是纯函数，便于单测：把卷与被引用它的容器名对上。
func buildVolumeInfos(volumes []*volume.Volume, containers []dockerTypes.Container) []MyType.VolumeInfo {
	used := make(map[string][]string, len(volumes))
	for _, c := range containers {
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		for _, m := range c.Mounts {
			if m.Type == "volume" && m.Name != "" {
				used[m.Name] = append(used[m.Name], name)
			}
		}
	}
	out := make([]MyType.VolumeInfo, 0, len(volumes))
	for _, v := range volumes {
		if v == nil {
			continue
		}
		info := MyType.VolumeInfo{
			Name:       v.Name,
			Driver:     v.Driver,
			Mountpoint: v.Mountpoint,
			Scope:      v.Scope,
			CreatedAt:  v.CreatedAt,
			Labels:     v.Labels,
		}
		if users, ok := used[v.Name]; ok {
			info.InUse = true
			info.UsedBy = users
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].InUse != out[j].InUse {
			return !out[i].InUse // 未被引用的（可清理候选）排前面
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// RemoveVolume 删除一个卷；force=true 由 daemon 强删。
func RemoveVolume(ctx context.Context, serviceContext *svc.ServiceContext, name string, force bool) error {
	if err := requireDocker(serviceContext); err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("卷名不能为空")
	}
	callCtx, cancel := dockerTimeout(ctx, dockerMutationTimeout)
	defer cancel()
	if err := serviceContext.DockerClient.VolumeRemove(callCtx, name, force); err != nil {
		return fmt.Errorf("删除卷 %s 失败: %w", name, err)
	}
	AuditLog("volume.remove", name, fmt.Sprintf("force=%v", force))
	return nil
}

// ---- 网络 ----

// ListNetworks 列出网络，并统计每个网络上的容器数量。
func ListNetworks(ctx context.Context, serviceContext *svc.ServiceContext) ([]MyType.NetworkInfo, error) {
	if err := requireDocker(serviceContext); err != nil {
		return nil, err
	}
	callCtx, cancel := dockerTimeout(ctx, dockerListTimeout)
	defer cancel()

	networks, err := serviceContext.DockerClient.NetworkList(callCtx, network.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("获取网络列表失败: %w", err)
	}
	containers, err := serviceContext.DockerClient.ContainerList(callCtx, container.ListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("获取容器列表失败: %w", err)
	}
	return buildNetworkInfos(networks, containers), nil
}

// buildNetworkInfos 是纯函数，便于单测。
func buildNetworkInfos(networks []network.Summary, containers []dockerTypes.Container) []MyType.NetworkInfo {
	counts := make(map[string]int, len(networks))
	for _, c := range containers {
		for name := range c.NetworkSettings.Networks {
			counts[name]++
		}
	}
	out := make([]MyType.NetworkInfo, 0, len(networks))
	for _, n := range networks {
		info := MyType.NetworkInfo{
			ID:         n.ID,
			Name:       n.Name,
			Driver:     n.Driver,
			Scope:      n.Scope,
			Internal:   n.Internal,
			Attachable: n.Attachable,
			Created:    n.Created.Format(time.RFC3339),
			Labels:     n.Labels,
			Containers: counts[n.Name],
		}
		if len(n.IPAM.Config) > 0 {
			info.Subnet = n.IPAM.Config[0].Subnet
			info.Gateway = n.IPAM.Config[0].Gateway
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ---- 容器详情 ----

// ContainerDetail 返回面向前端的容器 inspect 摘要（凭据类环境变量会打码）。
func ContainerDetail(ctx context.Context, serviceContext *svc.ServiceContext, id string) (MyType.ContainerDetail, error) {
	var out MyType.ContainerDetail
	if err := requireDocker(serviceContext); err != nil {
		return out, err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return out, fmt.Errorf("容器 ID 不能为空")
	}
	callCtx, cancel := dockerTimeout(ctx, dockerInspectTimeout)
	defer cancel()
	inspected, err := serviceContext.DockerClient.ContainerInspect(callCtx, id)
	if err != nil {
		return out, fmt.Errorf("获取容器信息失败: %w", err)
	}
	return containerDetailFrom(inspected), nil
}

func containerDetailFrom(inspected dockerContainerJSON) MyType.ContainerDetail {
	out := MyType.ContainerDetail{
		ID:   inspected.ID,
		Name: strings.TrimPrefix(inspected.Name, "/"),
	}
	if inspected.Config != nil {
		out.Image = inspected.Config.Image
		out.Command = inspected.Config.Cmd
		out.Entrypoint = inspected.Config.Entrypoint
		out.Env = maskEnv(inspected.Config.Env)
		out.User = inspected.Config.User
		out.WorkingDir = inspected.Config.WorkingDir
		out.Labels = inspected.Config.Labels
		out.Ports = portMappings(inspected.NetworkSettings)
		out.Compose = composeInfoFrom(inspected.Config.Labels)
	}
	out.ImageID = inspected.Image
	out.Platform = inspected.Platform
	out.Mounts = mountInfos(inspected.Mounts)
	out.Networks = containerNets(inspected.NetworkSettings)
	out.LogPath = inspected.LogPath
	if inspected.State != nil {
		out.State = inspected.State.Status
		out.PID = inspected.State.Pid
		out.ExitCode = inspected.State.ExitCode
		out.StartedAt = inspected.State.StartedAt
		if inspected.State.Health != nil {
			out.Health = inspected.State.Health.Status
		}
	}
	out.Created = inspected.Created
	out.RestartCount = inspected.RestartCount
	if inspected.HostConfig != nil {
		out.RestartPolicy = string(inspected.HostConfig.RestartPolicy.Name)
		out.Resources = MyType.ResourceInfo{
			Memory:     inspected.HostConfig.Memory,
			NanoCPUs:   inspected.HostConfig.NanoCPUs,
			CPUShares:  inspected.HostConfig.CPUShares,
			PidsLimit:  pidsLimit(inspected.HostConfig),
			Privileged: inspected.HostConfig.Privileged,
			ReadOnly:   inspected.HostConfig.ReadonlyRootfs,
		}
	}
	out.Status = out.State
	if out.Health != "" {
		out.Status = out.State + " (" + out.Health + ")"
	}
	return out
}

// portMappings 从 NetworkSettings.Ports 生成端口映射列表（宿主机侧可能为空）。
func portMappings(settings *dockerNetworkSettings) []MyType.PortMapping {
	if settings == nil || len(settings.Ports) == 0 {
		return nil
	}
	out := make([]MyType.PortMapping, 0, len(settings.Ports))
	for port, bindings := range settings.Ports {
		if len(bindings) == 0 {
			out = append(out, MyType.PortMapping{
				ContainerPort: string(port),
				Protocol:      string(port.Proto()),
			})
			continue
		}
		for _, b := range bindings {
			out = append(out, MyType.PortMapping{
				ContainerPort: string(port),
				Protocol:      string(port.Proto()),
				HostIP:        b.HostIP,
				HostPort:      b.HostPort,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ContainerPort < out[j].ContainerPort })
	return out
}

// maskEnv 把形如 PASSWORD/TOKEN/SECRET/KEY 的环境变量值打码，避免接口回显泄露凭据。
func maskEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		key, _, found := strings.Cut(kv, "=")
		if found && isSensitiveEnvKey(key) {
			out = append(out, key+"=******")
			continue
		}
		out = append(out, kv)
	}
	return out
}

func isSensitiveEnvKey(key string) bool {
	upper := strings.ToUpper(strings.TrimSpace(key))
	for _, marker := range []string{"PASSWORD", "PASSWD", "SECRET", "TOKEN", "APIKEY", "API_KEY", "PRIVATE_KEY", "ACCESS_KEY", "CREDENTIAL"} {
		if strings.Contains(upper, marker) {
			return true
		}
	}
	return false
}

func composeInfoFrom(labels map[string]string) MyType.ComposeInfo {
	if len(labels) == 0 {
		return MyType.ComposeInfo{}
	}
	return MyType.ComposeInfo{
		Project:     labels["com.docker.compose.project"],
		Service:     labels["com.docker.compose.service"],
		WorkingDir:  labels["com.docker.compose.project.working_dir"],
		ConfigFiles: labels["com.docker.compose.project.config_files"],
	}
}

func mountInfos(mounts []dockerMountPoint) []MyType.MountInfo {
	out := make([]MyType.MountInfo, 0, len(mounts))
	for _, m := range mounts {
		out = append(out, MyType.MountInfo{
			Type:        string(m.Type),
			Source:      m.Source,
			Destination: m.Destination,
			Mode:        m.Mode,
			RW:          m.RW,
			Name:        m.Name,
		})
	}
	return out
}

func containerNets(settings *dockerNetworkSettings) []MyType.ContainerNet {
	if settings == nil || len(settings.Networks) == 0 {
		return nil
	}
	out := make([]MyType.ContainerNet, 0, len(settings.Networks))
	for name, ep := range settings.Networks {
		if ep == nil {
			continue
		}
		out = append(out, MyType.ContainerNet{
			Name:       name,
			IPAddress:  ep.IPAddress,
			Gateway:    ep.Gateway,
			MacAddress: ep.MacAddress,
			Aliases:    ep.Aliases,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ---- 日志 ----

// ContainerLogs 读取容器最近 N 行日志（不跟随）。tail<=0 默认 200，上限 5000。
func ContainerLogs(ctx context.Context, serviceContext *svc.ServiceContext, id string, tail int, timestamps bool, since string) ([]string, error) {
	if err := requireDocker(serviceContext); err != nil {
		return nil, err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("容器 ID 不能为空")
	}
	if tail <= 0 {
		tail = 200
	}
	if tail > 5000 {
		tail = 5000
	}
	callCtx, cancel := dockerTimeout(ctx, dockerListTimeout)
	defer cancel()

	reader, err := serviceContext.DockerClient.ContainerLogs(callCtx, id, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       strconv.Itoa(tail),
		Timestamps: timestamps,
		Since:      since,
	})
	if err != nil {
		return nil, fmt.Errorf("获取容器日志失败: %w", err)
	}
	defer func() { _ = reader.Close() }()

	// 容器的日志流是 docker 的多路复用格式（每帧 8 字节头：stream + 长度），
	// 直接按行读会把帧头当成正文（表现为行首出现 \x01/\x02 与乱码）。
	// 必须用 stdcopy 解复用：stdout/stderr 都汇到同一个 buffer。
	var buf bytes.Buffer
	if _, err = stdcopy.StdCopy(&buf, &buf, reader); err != nil {
		return nil, fmt.Errorf("读取容器日志失败: %w", err)
	}
	raw := strings.TrimRight(buf.String(), "\n")
	if raw == "" {
		return []string{}, nil
	}
	all := strings.Split(raw, "\n")
	if len(all) > tail {
		all = all[len(all)-tail:]
	}
	lines := make([]string, 0, len(all))
	for _, line := range all {
		lines = append(lines, cleanLogLine(line))
	}
	return lines, nil
}

// cleanLogLine 去掉 docker 多路复用流留下的 0x01/0x02 流标记与行尾 \r。
func cleanLogLine(line string) string {
	line = strings.TrimRight(line, "\r")
	return strings.TrimPrefix(strings.TrimPrefix(line, "\x01"), "\x02")
}

// ---- 资源采样 ----

// ContainerStatOnce 取一次资源采样（daemon 侧 stream=false）。
func ContainerStatOnce(ctx context.Context, serviceContext *svc.ServiceContext, id string) (MyType.ContainerStat, error) {
	var out MyType.ContainerStat
	if err := requireDocker(serviceContext); err != nil {
		return out, err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return out, fmt.Errorf("容器 ID 不能为空")
	}
	callCtx, cancel := dockerTimeout(ctx, dockerInspectTimeout)
	defer cancel()

	resp, err := serviceContext.DockerClient.ContainerStatsOneShot(callCtx, id)
	if err != nil {
		return out, fmt.Errorf("获取容器资源采样失败: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var raw container.StatsResponse
	if err = jsonDecodeReader(resp.Body, &raw); err != nil {
		return out, fmt.Errorf("解析资源采样失败: %w", err)
	}
	return statFromRaw(raw), nil
}

// statFromRaw 把 daemon 的 stats 结构折算成百分比与字节数（纯函数，便于单测）。
func statFromRaw(raw container.StatsResponse) MyType.ContainerStat {
	out := MyType.ContainerStat{
		SampledAtUnix: time.Now().Unix(),
		CPUPercent:    cpuPercentFrom(raw),
		MemUsage:      int64(raw.MemoryStats.Usage),
		MemLimit:      int64(raw.MemoryStats.Limit),
		Pids:          int64(raw.PidsStats.Current),
	}
	// docker 的 mem usage 含 page cache，减掉 inactive_file 才是常说的"实际占用"。
	if inactive, ok := raw.MemoryStats.Stats["inactive_file"]; ok {
		if adjusted := int64(raw.MemoryStats.Usage) - int64(inactive); adjusted > 0 {
			out.MemUsage = adjusted
		}
	}
	if out.MemLimit > 0 {
		out.MemPercent = float64(out.MemUsage) / float64(out.MemLimit) * 100
	}
	for _, nw := range raw.Networks {
		out.NetRx += int64(nw.RxBytes)
		out.NetTx += int64(nw.TxBytes)
	}
	for _, b := range raw.BlkioStats.IoServiceBytesRecursive {
		switch strings.ToLower(b.Op) {
		case "read":
			out.BlockRead += int64(b.Value)
		case "write":
			out.BlockWrite += int64(b.Value)
		}
	}
	return out
}

// cpuPercentFrom 用 docker CLI 的公式：容器 CPU 增量 / 系统增量 * 在线核数 * 100。
func cpuPercentFrom(raw container.StatsResponse) float64 {
	cpuDelta := float64(raw.CPUStats.CPUUsage.TotalUsage) - float64(raw.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(raw.CPUStats.SystemUsage) - float64(raw.PreCPUStats.SystemUsage)
	online := float64(raw.CPUStats.OnlineCPUs)
	if online == 0 {
		online = float64(len(raw.CPUStats.CPUUsage.PercpuUsage))
	}
	if systemDelta <= 0 || cpuDelta <= 0 || online <= 0 {
		return 0
	}
	return cpuDelta / systemDelta * online * 100
}

// ---- 事件流 ----

// StreamDockerEvents 把 daemon 的事件流暴露给上层（SSE 桥接）。
func StreamDockerEvents(ctx context.Context, serviceContext *svc.ServiceContext, since string) (<-chan events.Message, <-chan error, error) {
	if err := requireDocker(serviceContext); err != nil {
		return nil, nil, err
	}
	opts := events.ListOptions{}
	if since != "" {
		opts.Since = since
	}
	msgCh, errCh := serviceContext.DockerClient.Events(ctx, opts)
	return msgCh, errCh, nil
}

// EventSummary 把 docker 事件压成 SSE 载荷的精简结构（纯函数，便于单测）。
func EventSummary(msg events.Message) map[string]interface{} {
	out := map[string]interface{}{
		"type":   string(msg.Type),
		"action": string(msg.Action),
		"time":   msg.Time,
	}
	if msg.Actor.ID != "" {
		out["id"] = msg.Actor.ID
		if len(msg.Actor.ID) > 12 {
			out["shortId"] = msg.Actor.ID[:12]
		}
	}
	if name, ok := msg.Actor.Attributes["name"]; ok {
		out["name"] = name
	}
	if image, ok := msg.Actor.Attributes["image"]; ok {
		out["image"] = image
	}
	return out
}
