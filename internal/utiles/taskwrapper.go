package utiles

import (
	"context"
	"fmt"

	"github.com/DongShull/DTool/internal/svc"
	"github.com/google/uuid"
	"github.com/zeromicro/go-zero/core/logx"
)

// ManagedTaskHooks 允许测试注入 Notify/Audit 替身；字段为 nil 时使用真实实现。
type ManagedTaskHooks struct {
	Notify func(kind, name, taskID string, ok bool, message string)
	Audit  func(action, target, detail string)
}

// ManagedTaskMeta 描述一个受管任务的展示名、文案与审计元数据。
type ManagedTaskMeta struct {
	// Name 展示名：用于进度、通知与审计主体。
	Name string
	// NotifyKind 通知类型，如 "update" / "restore"。
	NotifyKind string
	// AuditAction 审计动作，如 "container.update"。
	AuditAction string
	// Placeholder 排队占位进度；TaskID 由 RunManagedTask 填入。
	Placeholder svc.TaskProgress
	// PanicMessage 任务 panic 时写入的进度 Message，如 "更新失败"。
	PanicMessage string
	// PanicDetailFmt 任务 panic 时写入的 DetailMsg 模板，如 "更新任务异常中断: %v"。
	PanicDetailFmt string
	// DoneFallback fn 结束后进度 Message 为空时的兜底文案，如 "更新任务已结束"。
	DoneFallback string
}

// RunManagedTask 统一封装异步任务的完整生命周期：
//
//	注册任务（RegisterTask）→ 写入占位进度 → 执行 fn（panic recover）→
//	读取最终进度并兜底 message/detail → NotifyTaskDone → AuditLog → FinishTask。
//
// fn 在独立 goroutine 中运行；返回的 taskID 在 fn 启动前就已确定，HTTP 链路
// 可直接返回给前端（异步语义）；doneCh 收到 fn 的返回值（panic 时为合成错误），
// MCP 等同步链路等待 doneCh 后再用 GetProgress 读取最终进度。
//
// logger 允许为 nil，此时使用 logx.WithContext(taskCtx) 记录。
func RunManagedTask(svcCtx *svc.ServiceContext, logger logx.Logger, meta ManagedTaskMeta, hooks *ManagedTaskHooks, fn func(taskCtx context.Context, taskID string) error) (string, <-chan error) {
	taskID := uuid.New().String()
	taskCtx := svcCtx.RegisterTask(taskID)

	// 先落一条占位进度：RegisterTask 只登记取消句柄，不写 ProgressStore。
	// 没有它，任务被锁排在后面时，前端拿到 taskID 立刻查进度会得到
	// "taskID 未找到"，取消接口也会 400。
	placeholder := meta.Placeholder
	placeholder.TaskID = taskID
	placeholder.IsDone = false
	svcCtx.UpdateProgress(taskID, placeholder)

	notify := NotifyTaskDone
	if hooks != nil && hooks.Notify != nil {
		notify = hooks.Notify
	}
	audit := AuditLog
	if hooks != nil && hooks.Audit != nil {
		audit = hooks.Audit
	}

	// 缓冲为 1：保证发送结果不会阻塞在 FinishTask 之前。
	doneCh := make(chan error, 1)
	go func() {
		var runErr error
		log := logger
		if log == nil {
			log = logx.WithContext(taskCtx)
		}
		defer func() {
			if r := recover(); r != nil {
				log.Errorf("%s 任务 panic 已恢复: %v", meta.Name, r)
				msg := fmt.Sprintf(meta.PanicDetailFmt, r)
				svcCtx.UpdateProgress(taskID, svc.TaskProgress{
					TaskID:     taskID,
					Percentage: 100,
					Name:       meta.Name,
					Message:    meta.PanicMessage,
					DetailMsg:  msg,
					IsDone:     true,
				})
				notify(meta.NotifyKind, meta.Name, taskID, false, meta.PanicMessage)
				audit(meta.AuditAction, meta.Name, msg)
				doneCh <- fmt.Errorf("%s", msg)
			}
			svcCtx.FinishTask(taskID)
		}()

		runErr = fn(taskCtx, taskID)
		if runErr != nil {
			log.Errorf("%s出错: %v", meta.Name, runErr)
		}
		final, _ := svcCtx.GetProgress(taskID)
		message := final.Message
		if message == "" {
			message = meta.DoneFallback
		}
		detail := final.DetailMsg
		if detail == "" {
			detail = message
		}
		notify(meta.NotifyKind, meta.Name, taskID, runErr == nil, message)
		audit(meta.AuditAction, meta.Name, detail)
		doneCh <- runErr
	}()
	return taskID, doneCh
}
