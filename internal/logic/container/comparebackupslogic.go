package container

import (
	"context"

	"github.com/DongShull/DTool/internal/svc"
	"github.com/DongShull/DTool/internal/types"
	"github.com/DongShull/DTool/internal/utiles"

	"github.com/zeromicro/go-zero/core/logx"
)

type CompareBackupsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCompareBackupsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CompareBackupsLogic {
	return &CompareBackupsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CompareBackupsLogic) CompareBackups(req *types.BackupCompareReq) (resp *types.Resp, err error) {
	resp = &types.Resp{}
	result, err := utiles.CompareBackups(req.Old, req.New)
	if err != nil {
		resp.Code = 400
		resp.Msg = err.Error()
		resp.Data = map[string]interface{}{}
		return resp, nil
	}
	utiles.AuditLog("container.backup.compare", req.Old+" → "+req.New, "diff computed")
	resp.Code = 200
	resp.Msg = "success"
	resp.Data = result
	return resp, nil
}
