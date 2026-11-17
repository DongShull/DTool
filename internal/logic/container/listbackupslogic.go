package container

import (
	"context"
	"github.com/DongShull/DTool/internal/utiles"

	"github.com/DongShull/DTool/internal/svc"
	"github.com/DongShull/DTool/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListBackupsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListBackupsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListBackupsLogic {
	return &ListBackupsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListBackupsLogic) ListBackups() (resp *types.Resp, err error) {
	resp = &types.Resp{}
	backupList, err := utiles.BackupList(l.svcCtx)
	if err != nil {
		resp = types.Fail(types.CodeServerError, err.Error(), map[string]interface{}{})
		return resp, err
	}
	resp = types.OK(backupList)
	return resp, nil
}
