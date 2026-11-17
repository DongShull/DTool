package utiles

import (
	"context"
	"fmt"

	"github.com/DongShull/DTool/internal/svc"
	MyType "github.com/DongShull/DTool/internal/types"
	dockerTypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/system"
	"github.com/zeromicro/go-zero/core/logx"
)

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
