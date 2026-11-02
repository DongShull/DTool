package settings

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/DongShull/DTool/internal/svc"
	"github.com/DongShull/DTool/internal/types"
	"github.com/DongShull/DTool/internal/utiles"

	"github.com/zeromicro/go-zero/core/logx"
)

type SettingsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewSettingsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SettingsLogic {
	return &SettingsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *SettingsLogic) GetSettings() (resp *types.Resp, err error) {
	resp = &types.Resp{}
	s := utiles.LoadSettings()
	resp.Code = 200
	resp.Msg = "success"
	resp.Data = map[string]interface{}{
		"backupDir":          s.BackupDir,
		"effectiveBackupDir": utiles.EffectiveBackupDir(),
	}
	return resp, nil
}

func (l *SettingsLogic) UpdateSettings(req *types.UpdateSettingsReq) (resp *types.Resp, err error) {
	resp = &types.Resp{}
	dir := strings.TrimSpace(req.BackupDir)
	if dir == "" {
		resp.Code = 400
		resp.Msg = "备份目录不能为空"
		resp.Data = map[string]interface{}{}
		return resp, nil
	}
	if !filepath.IsAbs(dir) {
		resp.Code = 400
		resp.Msg = "备份目录必须是绝对路径"
		resp.Data = map[string]interface{}{}
		return resp, nil
	}
	// 备份目录会被"备份/恢复/删除备份"当作作用域：指到配置或日志目录后，
	// 删除备份的作用域就会覆盖到 progress.json / settings.json / 审计日志。
	if ok, reason := utiles.IsSafeBackupDir(dir); !ok {
		resp.Code = 400
		resp.Msg = reason
		resp.Data = map[string]interface{}{}
		return resp, nil
	}
	if mkErr := os.MkdirAll(dir, 0755); mkErr != nil {
		resp.Code = 400
		resp.Msg = "无法创建备份目录: " + mkErr.Error()
		resp.Data = map[string]interface{}{}
		return resp, nil
	}
	testFile, testErr := os.CreateTemp(dir, ".write-test-*")
	if testErr != nil {
		resp.Code = 400
		resp.Msg = "备份目录不可写: " + testErr.Error()
		resp.Data = map[string]interface{}{}
		return resp, nil
	}
	testName := testFile.Name()
	_ = testFile.Close()
	_ = os.Remove(testName)

	s := utiles.LoadSettings()
	s.BackupDir = dir
	if saveErr := utiles.SaveSettings(s); saveErr != nil {
		resp.Code = 500
		resp.Msg = "保存设置失败: " + saveErr.Error()
		resp.Data = map[string]interface{}{}
		return resp, nil
	}
	utiles.AuditLog("settings.update", "settings", "backupDir="+dir)
	resp.Code = 200
	resp.Msg = "success"
	resp.Data = map[string]interface{}{
		"backupDir":          s.BackupDir,
		"effectiveBackupDir": utiles.EffectiveBackupDir(),
	}
	return resp, nil
}
