package progress

import (
	"context"

	"github.com/DongShull/DTool/internal/svc"
	"github.com/DongShull/DTool/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CancelProgressLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCancelProgressLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CancelProgressLogic {
	return &CancelProgressLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CancelProgressLogic) CancelProgress(req *types.GetProgressReq) (resp *types.Resp, err error) {
	resp = &types.Resp{}
	ok, msg := l.svcCtx.CancelActiveTask(req.TaskId)
	if !ok {
		resp = types.Fail(types.CodeBadRequest, msg, map[string]interface{}{})
		return resp, nil
	}
	resp = types.OK(map[string]interface{}{"taskID": req.TaskId})
	return resp, nil
}
