package utiles

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/DongShull/DTool/internal/svc"
	MyType "github.com/DongShull/DTool/internal/types"
	"github.com/docker/docker/api/types/container"
)

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
