package types

// VolumeNameReq 是 DELETE /api/volumes/{name} 的路径参数。
type VolumeNameReq struct {
	Name string `path:"name"`
}

// SystemOverview 是 GET /api/system 的响应数据：Docker 版本 + 主机信息 + 磁盘占用。
type SystemOverview struct {
	Docker DockerVersionInfo `json:"docker"`
	Info   HostInfo          `json:"info"`
	Disk   DiskUsageInfo     `json:"disk"`
}

type DockerVersionInfo struct {
	Version       string `json:"version"`
	APIVersion    string `json:"apiVersion"`
	MinAPIVersion string `json:"minApiVersion"`
	Os            string `json:"os"`
	Arch          string `json:"arch"`
	KernelVersion string `json:"kernelVersion"`
}

type HostInfo struct {
	Name              string `json:"name"`
	ServerVersion     string `json:"serverVersion"`
	OperatingSystem   string `json:"operatingSystem"`
	OSType            string `json:"osType"`
	Architecture      string `json:"architecture"`
	NCPU              int    `json:"nCPU"`
	MemTotal          int64  `json:"memTotal"`
	DockerRootDir     string `json:"dockerRootDir"`
	StorageDriver     string `json:"storageDriver"`
	Containers        int    `json:"containers"`
	ContainersRunning int    `json:"containersRunning"`
	ContainersPaused  int    `json:"containersPaused"`
	ContainersStopped int    `json:"containersStopped"`
	Images            int    `json:"images"`
	Warnings          int    `json:"warnings"`
}

type DiskUsageInfo struct {
	LayersSize  int64          `json:"layersSize"`
	Images      int            `json:"images"`
	Containers  int            `json:"containers"`
	Volumes     int            `json:"volumes"`
	BuildCache  int            `json:"buildCache"`
	Reclaimable int64          `json:"reclaimable"`
	Detail      DiskUsageBreak `json:"detail"`
}

// DiskUsageBreak 给出各类型可回收空间的粗略拆分（未使用镜像/停止容器日志/悬空卷）。
type DiskUsageBreak struct {
	UnusedImages int64 `json:"unusedImages"`
	StoppedLogs  int64 `json:"stoppedContainersSize"`
	Volumes      int64 `json:"volumesSize"`
}

// VolumeInfo 是 GET /api/volumes 的条目。
type VolumeInfo struct {
	Name       string            `json:"name"`
	Driver     string            `json:"driver"`
	Mountpoint string            `json:"mountpoint"`
	Scope      string            `json:"scope"`
	CreatedAt  string            `json:"createdAt"`
	Labels     map[string]string `json:"labels,omitempty"`
	InUse      bool              `json:"inUse"`
	UsedBy     []string          `json:"usedBy,omitempty"`
}

// NetworkInfo 是 GET /api/networks 的条目。
type NetworkInfo struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Driver     string            `json:"driver"`
	Scope      string            `json:"scope"`
	Internal   bool              `json:"internal"`
	Attachable bool              `json:"attachable"`
	Created    string            `json:"created"`
	Subnet     string            `json:"subnet,omitempty"`
	Gateway    string            `json:"gateway,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
	Containers int               `json:"containers"`
}

// ContainerDetail 是 GET /api/container/{id} 的响应：面向前端的 inspect 摘要。
type ContainerDetail struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Image         string            `json:"image"`
	ImageID       string            `json:"imageId"`
	State         string            `json:"state"`
	Status        string            `json:"status"`
	Health        string            `json:"health,omitempty"`
	Created       string            `json:"created"`
	StartedAt     string            `json:"startedAt,omitempty"`
	RestartCount  int               `json:"restartCount"`
	RestartPolicy string            `json:"restartPolicy"`
	Platform      string            `json:"platform,omitempty"`
	PID           int               `json:"pid"`
	ExitCode      int               `json:"exitCode"`
	Command       []string          `json:"command,omitempty"`
	Entrypoint    []string          `json:"entrypoint,omitempty"`
	Env           []string          `json:"env,omitempty"`
	Ports         []PortMapping     `json:"ports,omitempty"`
	Mounts        []MountInfo       `json:"mounts,omitempty"`
	Networks      []ContainerNet    `json:"networks,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
	Compose       ComposeInfo       `json:"compose"`
	Resources     ResourceInfo      `json:"resources"`
	User          string            `json:"user,omitempty"`
	WorkingDir    string            `json:"workingDir,omitempty"`
	LogPath       string            `json:"logPath,omitempty"`
}

type PortMapping struct {
	ContainerPort string `json:"containerPort"`
	Protocol      string `json:"protocol"`
	HostIP        string `json:"hostIp,omitempty"`
	HostPort      string `json:"hostPort,omitempty"`
}

type MountInfo struct {
	Type        string `json:"type"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Mode        string `json:"mode,omitempty"`
	RW          bool   `json:"rw"`
	Name        string `json:"name,omitempty"`
}

type ContainerNet struct {
	Name       string   `json:"name"`
	IPAddress  string   `json:"ipAddress,omitempty"`
	Gateway    string   `json:"gateway,omitempty"`
	MacAddress string   `json:"macAddress,omitempty"`
	Aliases    []string `json:"aliases,omitempty"`
}

type ComposeInfo struct {
	Project     string `json:"project,omitempty"`
	Service     string `json:"service,omitempty"`
	WorkingDir  string `json:"workingDir,omitempty"`
	ConfigFiles string `json:"configFiles,omitempty"`
}

type ResourceInfo struct {
	Memory     int64 `json:"memory"`
	NanoCPUs   int64 `json:"nanoCpus"`
	CPUShares  int64 `json:"cpuShares"`
	PidsLimit  int64 `json:"pidsLimit"`
	Privileged bool  `json:"privileged"`
	ReadOnly   bool  `json:"readonlyRootfs"`
}

// ContainerStat 是 GET /api/container/{id}/stats 的一次性采样结果。
type ContainerStat struct {
	CPUPercent    float64 `json:"cpuPercent"`
	MemUsage      int64   `json:"memUsage"`
	MemLimit      int64   `json:"memLimit"`
	MemPercent    float64 `json:"memPercent"`
	NetRx         int64   `json:"netRx"`
	NetTx         int64   `json:"netTx"`
	BlockRead     int64   `json:"blockRead"`
	BlockWrite    int64   `json:"blockWrite"`
	Pids          int64   `json:"pids"`
	SampledAtUnix int64   `json:"sampledAt"`
}
