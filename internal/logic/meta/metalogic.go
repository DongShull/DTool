package meta

import (
	"context"

	"github.com/DongShull/DTool/internal/config"
	"github.com/DongShull/DTool/internal/svc"
	"github.com/DongShull/DTool/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// MetaLogic 返回后端自描述元数据（版本 + 业务码枚举），
// 供前端动态渲染错误文案与码值含义，禁止前端硬编码。
type MetaLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewMetaLogic(ctx context.Context, svcCtx *svc.ServiceContext) *MetaLogic {
	return &MetaLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// Meta 业务码的自描述默认文案；各接口实际 msg 以响应体为准（可能更具体）。
func (l *MetaLogic) Meta() (resp *types.Resp, err error) {
	meta := types.MetaInfo{
		Version:   config.Version,
		BuildDate: config.BuildDate,
		Codes: []types.CodeInfo{
			{Code: types.CodeOK, Meaning: "成功", DefaultMsg: "success"},
			{Code: types.CodePartial, Meaning: "部分成功：多目标任务中存在失败项，详见 data 中的失败清单", DefaultMsg: "部分成功"},
			{Code: types.CodeBadRequest, Meaning: "请求参数错误或目标资源不存在", DefaultMsg: "请求参数错误"},
			// 401 有两种来源：登录接口返回 {"code":401,"msg":"无效的secretKey",...}，
			// JWT 中间件返回 {"code":401,"msg":"未授权","data":{}}（见 dtool.go
			// WithUnauthorizedCallback）。defaultMsg 取中间件文案「未授权」，
			// 登录接口的实际 msg 以响应体为准。
			{Code: types.CodeUnauthorized, Meaning: "鉴权失败（登录 secretKey 不匹配，或 JWT 缺失/无效）", DefaultMsg: "未授权"},
			{Code: types.CodeForbidden, Meaning: "无权执行该操作（如程序自更新被禁用）", DefaultMsg: "禁止执行该操作"},
			{Code: types.CodeConflict, Meaning: "资源状态冲突（如镜像仍被容器引用而无法删除）", DefaultMsg: "资源状态冲突"},
			{Code: types.CodeServerError, Meaning: "服务端内部错误", DefaultMsg: "服务端内部错误"},
			// 429 仅登录限流产生（loginhandler 的进程内限流器）。
			{Code: 429, Meaning: "登录尝试过于频繁", DefaultMsg: "尝试过于频繁，请稍后再试"},
		},
	}
	return types.OK(meta), nil
}
