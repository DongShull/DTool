package container

import (
	"context"
	"github.com/DongShull/DTool/internal/utiles"

	"github.com/DongShull/DTool/internal/svc"
	"github.com/DongShull/DTool/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type StopLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewStopLogic(ctx context.Context, svcCtx *svc.ServiceContext) *StopLogic {
	return &StopLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *StopLogic) Stop(req *types.IdReq) (resp *types.Resp, err error) {
	resp = &types.Resp{}
	// 幂等停止：已停止直接成功；paused 先取消暂停再停止。
	err = utiles.EnsureContainerStopped(l.svcCtx, req.Id)
	if err != nil {
		resp = types.Fail(types.CodeBadRequest, err.Error(), map[string]interface{}{})
		return resp, err
	}
	utiles.AuditLog("container.stop", req.Id, "")
	resp = types.OK(map[string]interface{}{})
	return resp, nil
}
