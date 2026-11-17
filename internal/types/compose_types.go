package types

// 本文件集中定义 Compose 工程级控制（/api/compose/*）的请求/响应类型，
// 独立于共享 types.go，避免与并行改动冲突。

// ComposeProjectNameReq 是 POST /api/compose/{project}/{up,down} 的路径参数。
type ComposeProjectNameReq struct {
	Project string `path:"project"`
}

// ComposeLogsReq 是 GET /api/compose/{project}/logs 的参数。
// service 为空时聚合项目内全部服务；tail 为 0 时按默认行数取。
type ComposeLogsReq struct {
	Project string `path:"project"`
	Service string `form:"service,optional"`
	Tail    int    `form:"tail,optional"`
}

// ComposeContainerBrief 是 Compose 工程里单个服务的容器摘要。
type ComposeContainerBrief struct {
	Name    string `json:"name"`
	Service string `json:"service"`
	State   string `json:"state"`
	Image   string `json:"image"`
}

// ComposeProjectInfo 是 GET /api/compose/projects 的单个工程条目。
type ComposeProjectInfo struct {
	Project     string                  `json:"project"`
	Workdir     string                  `json:"workdir"`
	ConfigFiles string                  `json:"configFiles,omitempty"`
	Services    int                     `json:"services"`
	Running     int                     `json:"running"`
	Stopped     int                     `json:"stopped"`
	Status      string                  `json:"status"`
	Containers  []ComposeContainerBrief `json:"containers"`
}

// ComposeServiceLogs 是 GET /api/compose/{project}/logs 的单个服务日志块。
type ComposeServiceLogs struct {
	Service   string   `json:"service"`
	Container string   `json:"container"`
	Lines     []string `json:"lines"`
}
