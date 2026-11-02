package container

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/DongShull/DTool/internal/svc"
	"github.com/DongShull/DTool/internal/types"
	"github.com/DongShull/DTool/internal/utiles"
	"github.com/google/uuid"
	"github.com/zeromicro/go-zero/core/logx"
)

type UpdateLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateLogic {
	return &UpdateLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UpdateLogic) Update(req *types.ContainerUpdateReq) (resp *types.Resp, err error) {
	resp = &types.Resp{}
	// 名字用于进度列表过滤、通知与审计。客户端不传或传了改名前的旧名时，
	// 用容器 ID 从 daemon 反查真实名字，避免进度按名字查不到、审计主体为空。
	displayName := strings.TrimSpace(req.ContainerName)
	if displayName == "" {
		displayName = utiles.ResolveContainerName(l.svcCtx, req.Id)
	}
	taskID := uuid.New().String()
	taskCtx := l.svcCtx.RegisterTask(taskID)
	// 先落一条占位进度：RegisterTask 只登记取消句柄，不写 ProgressStore。
	// 没有它，任务被同一个容器的锁排在后面时，前端拿到 taskID 立刻查进度会
	// 得到 "taskID 未找到"，取消接口也会 400。
	l.svcCtx.UpdateProgress(taskID, svc.TaskProgress{
		TaskID:    taskID,
		Name:      displayName,
		Message:   "任务已排队",
		DetailMsg: "等待调度：同一容器的更新/恢复串行执行",
		IsDone:    false,
	})
	go func() {
		// Catch any panic and log the error
		defer func() {
			if r := recover(); r != nil {
				l.Errorf("更新容器任务 panic 已恢复: %v", r)
				msg := fmt.Sprintf("更新任务异常中断: %v", r)
				l.svcCtx.UpdateProgress(taskID, svc.TaskProgress{
					TaskID:     taskID,
					Percentage: 100,
					Name:       displayName,
					Message:    "更新失败",
					DetailMsg:  msg,
					IsDone:     true,
				})
				utiles.NotifyTaskDone("update", displayName, taskID, false, msg)
				utiles.AuditLog("container.update", displayName, msg)
			}
			l.svcCtx.FinishTask(taskID)
		}()
		imageNameAndTag := req.ImageNameAndTag
		delOldContainer := os.Getenv("DelOldContainer") != "false"
		err := utiles.UpdateContainer(l.svcCtx, taskCtx, req.Id, displayName, imageNameAndTag, delOldContainer, taskID)
		if err != nil {
			l.Errorf("更新容器出错: %v", err)
		}
		final, _ := l.svcCtx.GetProgress(taskID)
		detail := final.DetailMsg
		if detail == "" {
			detail = final.Message
		}
		utiles.NotifyTaskDone("update", displayName, taskID, err == nil, final.Message)
		utiles.AuditLog("container.update", displayName, detail)
	}()
	resp.Code = 200
	resp.Msg = "success"
	resp.Data = map[string]string{"taskID": taskID}
	return resp, nil
}
