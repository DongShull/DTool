package version

import (
	"context"
	"time"

	"github.com/DongShull/DTool/internal/svc"
	"github.com/DongShull/DTool/internal/types"
	"github.com/DongShull/DTool/internal/utiles"
	"github.com/zeromicro/go-zero/core/logx"
)

type UpdateProgramLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateProgramLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateProgramLogic {
	return &UpdateProgramLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UpdateProgramLogic) UpdateProgram(req *types.UpdateProgramReq) (resp *types.Resp, err error) {
	resp = &types.Resp{}
	if utiles.UpdateProgramDisabled() {
		resp.Code = 403
		resp.Msg = utiles.ErrUpdateProgramDisabled.Error()
		resp.Data = map[string]interface{}{}
		return resp, nil
	}
	target := ""
	if req != nil {
		target = req.Target
	}
	if target != "" {
		if _, chkErr := utiles.SanitizeUpdateTarget(target); chkErr != nil {
			resp.Code = 400
			resp.Msg = chkErr.Error()
			resp.Data = map[string]interface{}{}
			return resp, nil
		}
	}
	err = utiles.UpdateProgram(l.svcCtx, utiles.UpdateOptions{
		Target: target,
		Manual: true,
	})
	if err != nil {
		resp.Code = 500
		resp.Msg = err.Error()
		resp.Data = map[string]interface{}{}
		return resp, err
	}
	resp.Code = 200
	resp.Msg = "success"
	// 与 MCP 通道保持一致：自更新成功也要留审计与通知（进程 10 秒后退出，
	// 必须在退出前完成落盘）。
	utiles.AuditLog("program.update", "self", "program update triggered via HTTP API")
	utiles.NotifyTaskDone("program", "self", "", true, "程序自更新完成，服务即将重启换入新版本")
	// 退出码 0：start.sh 只把「非 0 且运行不足 30 秒」的退出计为启动失败，
	// 原来用 os.Exit(1) 时，容器刚起来就点几次更新会触发无谓回退。
	utiles.ScheduleProcessExit(10 * time.Second)
	resp.Data = map[string]interface{}{}
	return resp, nil
}
