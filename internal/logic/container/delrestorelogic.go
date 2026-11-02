package container

import (
	"context"
	"os"

	"github.com/DongShull/DTool/internal/svc"
	"github.com/DongShull/DTool/internal/types"
	"github.com/DongShull/DTool/internal/utiles"

	"github.com/zeromicro/go-zero/core/logx"
)

type DelRestoreLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDelRestoreLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DelRestoreLogic {
	return &DelRestoreLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DelRestoreLogic) DelRestore(req *types.DelContainerBackupReq) (resp *types.Resp, err error) {
	resp = &types.Resp{}
	// 只允许删除本工具生成的 backup-*.json / backup-*.yaml：
	// 备份目录可由设置改成任意可写路径，仅按扩展名校验会允许删掉同目录的
	// progress.json / update-history.json / settings.json 等配置数据。
	if !utiles.IsBackupFileName(req.Filename) {
		resp.Code = 400
		resp.Msg = "非法文件名：只允许删除 backup-*.json / backup-*.yaml"
		resp.Data = map[string]interface{}{}
		return resp, nil
	}
	fullPath, err := utiles.ResolveBackupPath(req.Filename, ".json", ".yaml")
	if err != nil {
		resp.Code = 400
		resp.Msg = "非法文件名"
		resp.Data = map[string]interface{}{}
		return resp, nil
	}
	err = os.Remove(fullPath)
	if err != nil {
		utiles.AuditLog("backup.delete", req.Filename, "删除失败: "+err.Error())
		resp.Code = 400
		resp.Msg = "删除失败"
		resp.Data = map[string]interface{}{}
		return resp, nil
	}
	utiles.AuditLog("backup.delete", req.Filename, "备份文件已删除（不可恢复）")
	resp.Code = 200
	resp.Msg = "success"
	resp.Data = map[string]interface{}{}
	return resp, nil
}
