package container

import (
	"context"
	"fmt"
	"github.com/DongShull/DTool/internal/utiles"

	"github.com/DongShull/DTool/internal/svc"
	"github.com/DongShull/DTool/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type BackupLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewBackupLogic(ctx context.Context, svcCtx *svc.ServiceContext) *BackupLogic {
	return &BackupLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *BackupLogic) Backup(req *types.BackupContainersReq) (resp *types.Resp, err error) {
	resp = &types.Resp{}
	failures, err := utiles.BackupContainer(l.svcCtx, req.Include...)
	if err != nil {
		resp.Code = 500
		resp.Msg = err.Error()
		resp.Data = map[string]interface{}{"failures": failures}
		return resp, err
	}
	// 部分失败必须与"全部成功"在协议层可区分：HTTP 207 + code 207。
	// 旧实现把部分失败也报成 200/success，前端与脚本会认为整份备份可用。
	resp.Code = 200
	resp.Msg = "success"
	if len(failures) > 0 {
		resp.Code = 207
		resp.Msg = fmt.Sprintf("备份部分失败：%d 个容器未写入备份文件", len(failures))
	}
	resp.Data = map[string]interface{}{"failures": failures}
	return resp, nil
}
