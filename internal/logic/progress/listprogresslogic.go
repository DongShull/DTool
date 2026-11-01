package progress

import (
	"context"

	"github.com/DongShull/DTool/internal/svc"
	"github.com/DongShull/DTool/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListProgressLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListProgressLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListProgressLogic {
	return &ListProgressLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListProgressLogic) ListProgress(req *types.ListProgressReq) (resp *types.Resp, err error) {
	resp = &types.Resp{}
	resp.Code = 200
	resp.Msg = "success"
	resp.Data = l.svcCtx.FilterProgress(req.Status, req.Name)
	return resp, nil
}
