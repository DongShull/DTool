package utiles

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/DongShull/DTool/internal/svc"
	MyType "github.com/DongShull/DTool/internal/types"
)

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
