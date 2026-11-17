package container

import (
	"context"
	"github.com/DongShull/DTool/internal/utiles"

	"github.com/DongShull/DTool/internal/svc"
	"github.com/DongShull/DTool/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type RestartLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewRestartLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RestartLogic {
	return &RestartLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *RestartLogic) Restart(req *types.IdReq) (resp *types.Resp, err error) {
	resp = &types.Resp{}
	err = utiles.RestartContainer(l.svcCtx, req.Id)
	if err != nil {
		resp = types.Fail(types.CodeBadRequest, err.Error(), map[string]interface{}{})
		return resp, err
	}
	utiles.AuditLog("container.restart", req.Id, "")
	resp = types.OK(map[string]interface{}{})
	return resp, nil
}
