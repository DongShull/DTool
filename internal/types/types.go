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
	// imageNameAndTag 可选：留空时按容器当前镜像更新（"一键更新"的主路径）。
	// 此前为必填 form 字段，缺少它会在解析阶段直接返回
	// "field \"imageNameAndTag\" is not set"，而 UpdateContainer 内部本来
	// 就支持为空（回落到 inspect 到的 Config.Image）。
	ImageNameAndTag string `form:"imageNameAndTag,optional" json:"imageNameAndTag,optional"`
	// json tag 必须带 optional：go-zero 的 Parse 对非 JSON 请求会走
	// mapping.UnmarshalJsonMap(nil, v)，此时 json tag 为必填就会误报
	// "field ... is not set"（本字段就踩过：form 请求也被判成缺字段）。
	ContainerName string `form:"containerName" json:"containerName,optional"`
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
	// json tag 与 form tag 并存：JSON 客户端（axios/fetch 默认发 JSON）与
	// form/query 客户端都能登录。此前只有 form tag 时，JSON body 里的
	// secretKey 不会被解析，登录恒返回"无效的secretKey"。
	SecretKey string `form:"secretKey,optional" json:"secretKey,optional"`
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
	// type 可选，缺省按 local 处理：此前 form:"type" 为必填，
	// 裸 GET /api/version 会得到 code:50000 "field \"type\" is not set"。
	Type string `form:"type,optional" json:"type,optional"`
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
	// 必须带 json tag：此前只有 form tag，JSON body 里的 target 会被静默丢弃，
	// "更新到指定版本"实际变成"更新到最新版"（破坏性且不可撤销）。
	Target string `form:"target,optional" json:"target,optional"`
}

// ── 统一响应信封契约（全项目唯一，前端请以本文档为准）──────────────
//
// 所有 HTTP 接口的响应体都是同一个信封：
//
//	{"code": <int 业务码>, "msg": <string>, "data": <T>}
//
// 规则：
//   - 成功恒为 code=200（CodeOK）且 msg="success"；错误码一律使用下方
//     CodeXxx 常量，禁止在任何业务代码里散落裸数字。
//   - OK(data) / Fail(code, msg, data...) 是构造信封的唯一入口。
//   - 错误响应的 data 通常为 {}，可能携带补充字段（如 failures）。
//   - 枚举含义与默认文案的自描述元数据由 /api/meta（logic/meta）返回，
//     前端不得硬编码任何码值含义或文案。
//
// 详细规范见 docs/API-RESPONSE.md。
// 业务状态码：resp.Code 会被 handler 直接当作 HTTP 状态码写入，
// 因此不能超过 999（见 internal/logic/version/versionlogic.go 中的注释）。
const (
	// CodeOK 成功。
	CodeOK = 200
	// CodePartial 部分成功：多目标任务中存在失败项（备份/清理部分失败）。
	CodePartial = 207
	// CodeBadRequest 请求参数错误或目标资源不存在。
	CodeBadRequest = 400
	// CodeUnauthorized 鉴权失败（如 secretKey 无效）。
	CodeUnauthorized = 401
	// CodeForbidden 无权执行该操作（如程序自更新被禁用）。
	CodeForbidden = 403
	// CodeConflict 资源状态冲突（如镜像仍被容器引用而无法删除）。
	CodeConflict = 409
	// CodeServerError 服务端内部错误。
	CodeServerError = 500
)

// OK 构造成功响应，Msg 默认为 "success"；需要自定义 Msg 时通过 msg 传入。
func OK(data interface{}, msg ...string) *Resp {
	m := "success"
	if len(msg) > 0 {
		m = msg[0]
	}
	return &Resp{Code: CodeOK, Msg: m, Data: data}
}

// Fail 构造失败响应；需要携带 Data 时通过 data 传入（不传则为 nil）。
func Fail(code int, msg string, data ...interface{}) *Resp {
	var d interface{}
	if len(data) > 0 {
		d = data[0]
	}
	return &Resp{Code: code, Msg: msg, Data: d}
}

// CodeInfo 单个业务码的自描述信息，随 /api/meta 下发给前端展示。
type CodeInfo struct {
	Code       int    `json:"code"`       // 业务码数值
	Meaning    string `json:"meaning"`    // 中文含义
	DefaultMsg string `json:"defaultMsg"` // 该码的默认 msg 文案
}

// MetaInfo 后端自描述元数据：版本信息 + 全部业务码枚举。
// 前端不得硬编码码值含义或文案，一切以此接口返回为准。
type MetaInfo struct {
	Version   string     `json:"version"`   // 后端版本号
	BuildDate string     `json:"buildDate"` // 构建时间
	Codes     []CodeInfo `json:"codes"`    // 全部业务码枚举
}
