package container

import (
	"context"
	"github.com/DongShull/DTool/internal/svc"
	"github.com/DongShull/DTool/internal/types"
	"github.com/DongShull/DTool/internal/utiles"

	"github.com/zeromicro/go-zero/core/logx"
)

type StartLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewStartLogic(ctx context.Context, svcCtx *svc.ServiceContext) *StartLogic {
	return &StartLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *StartLogic) Start(req *types.IdReq) (resp *types.Resp, err error) {
	resp = &types.Resp{}
	// 幂等启动：运行中直接成功，paused 报明确错误，避免 Docker 304 类错误被当作失败。
	err = utiles.EnsureContainerStarted(l.svcCtx, req.Id)
	if err != nil {
		resp = types.Fail(types.CodeBadRequest, err.Error(), map[string]interface{}{})
		return resp, err
	}
	utiles.AuditLog("container.start", req.Id, "")
	resp = types.OK(map[string]interface{}{})
	return resp, nil
}
