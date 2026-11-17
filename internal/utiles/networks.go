package utiles

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/DongShull/DTool/internal/svc"
	MyType "github.com/DongShull/DTool/internal/types"
	dockerTypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
)

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
