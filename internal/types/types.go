// 基于 goctl 生成的类型定义，包含手工扩展（Nil、RemoveImagesReq、
// ContainerRestoreReq 等）。若用 goctl 重新生成，请对照 git 历史补回手工部分。

package types

// Nil 用作 BaseResponse 泛型的占位类型，表示“无数据”。
// 不要再用 go/types.Nil 之类的第三方类型充当占位符。
type Nil struct{}

type ContainerRenameReq struct {
	IdReq
	NewName string `form:"newName"`
}

type ContainerRestoreReq struct {
	Filename string `json:"filename"`
	// Start 为空时按备份记录的状态恢复；true/false 强制启动或不启动。
	Start *bool `json:"start,optional"`
	// Force 为 true 时，同名容器会被删除后重建；默认跳过同名容器。
	Force bool `json:"force,optional"`
	// NameMap 为空时按备份里的容器名恢复；非空时把备份里的容器名重定向到
	// 指定名字恢复（不改原名），键为备份中的容器名，值为恢复后的容器名。
	NameMap map[string]string `json:"nameMap,optional"`
}

type ContainerUpdateReq struct {
	IdReq
	ImageNameAndTag string `form:"imageNameAndTag"`
	ContainerName   string `form:"containerName"`
}

type CreateContainerReq struct {
	OldName         string `json:"old_name"`
	NewName         string `json:"new_name"`
	ImageNameAndTag string `json:"image_name_and_tag"`
}

type DelContainerBackupReq struct {
	Filename string `form:"filename"`
}

// BackupCompareReq 对比两个 JSON 配置备份的差异。
type BackupCompareReq struct {
	Old string `form:"old"`
	New string `form:"new"`
}

type DoLoginReq struct {
	SecretKey string `form:"secret_key,optional"`
}

type GetProgressReq struct {
	TaskId string `path:"taskid"`
}

// ListProgressReq 任务进度列表的过滤条件。
// status：running / cancelled / done，为空返回全部；
// name：按任务名子串匹配（不区分大小写）。
type ListProgressReq struct {
	Status string `form:"status,optional"`
	Name   string `form:"name,optional"`
}

// ExportProgressReq 进度导出条件：与列表过滤一致，另支持时间范围（Unix 秒）。
type ExportProgressReq struct {
	Status string `form:"status,optional"`
	Name   string `form:"name,optional"`
	From   int64  `form:"from,optional"`
	To     int64  `form:"to,optional"`
}

type IdReq struct {
	Id string `path:"id"`
}

type LoginReq struct {
	SecretKey string `form:"secretKey,optional"`
}

type MsgResp struct {
	Status string `json:"status"`
	Msg    string `json:"msg"`
}

type RemoveContainerReq struct {
	Name string `json:"name"`
}

type RemoveImageReq struct {
	IdReq
	Force bool `form:"force,default=false"`
}

// RemoveImagesReq 批量镜像清理请求：
// filter=untagged 删除无 TAG 镜像，filter=unused 删除未被任何容器引用的镜像。
type RemoveImagesReq struct {
	Filter string `form:"filter"`
}

// BackupContainersReq 备份请求：include 为空备份全部容器，
// 非空时只备份名称（或 ID）命中的容器。
type BackupContainersReq struct {
	Include []string `form:"include,optional"`
}

type RenameContainerReq struct {
	OldName string `json:"oldName"`
	NewName string `json:"newName"`
}

type Resp struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data"`
}

type StartContainerReq struct {
	Name string `json:"name"`
}

type StopContainerReq struct {
	Name string `json:"name"`
}

type VerifyJwtReq struct {
	Jwt string `form:"jwt,optional"`
}

type VersionMsgResp struct {
	Version   string `json:"version"`
	BuildDate string `json:"build_date"`
}

type VersionReq struct {
	Type string `form:"type"`
}

type GetNewImageReq struct {
	ImageNameAndTag string `json:"image_name_and_tag"`
}

type UpdateSettingsReq struct {
	BackupDir string `json:"backupDir"`
}

// UpdateProgramReq 程序自更新请求。
// Target 为空更新到最新版；非空时强制更新到指定版本（允许降级）。
type UpdateProgramReq struct {
	Target string `form:"target,optional"`
}
