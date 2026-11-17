package utiles

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/DongShull/DTool/internal/svc"
	dockerTypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
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
