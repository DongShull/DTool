package utiles

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/DongShull/DTool/internal/svc"
	MyType "github.com/DongShull/DTool/internal/types"
	dockerTypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/volume"
)

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
