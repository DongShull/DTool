package container

import (
	"context"
	"os"
	"strings"

	"github.com/DongShull/DTool/internal/svc"
	"github.com/DongShull/DTool/internal/types"
	"github.com/DongShull/DTool/internal/utiles"
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
	// RunManagedTask 统一封装：占位进度、panic recover、通知、审计与收尾。
	// HTTP 链路异步语义：立即返回 taskID，不等待任务完成。
	taskID, _ := utiles.RunManagedTask(l.svcCtx, l.Logger, utiles.ManagedTaskMeta{
		Name:        displayName,
		NotifyKind:  "update",
		AuditAction: "container.update",
		// 先落一条占位进度：任务被同一个容器的锁排在后面时，前端拿到 taskID
		// 立刻查进度也能查到，取消接口也不会 400。
		Placeholder: svc.TaskProgress{
			Name:      displayName,
			Message:   "任务已排队",
			DetailMsg: "等待调度：同一容器的更新/恢复串行执行",
		},
		PanicMessage:   "更新失败",
		PanicDetailFmt: "更新任务异常中断: %v",
		DoneFallback:   "更新任务已结束",
	}, nil, func(taskCtx context.Context, taskID string) error {
		imageNameAndTag := req.ImageNameAndTag
		delOldContainer := os.Getenv("DelOldContainer") != "false"
		return utiles.UpdateContainer(l.svcCtx, taskCtx, req.Id, displayName, imageNameAndTag, delOldContainer, taskID)
	})
	resp = types.OK(map[string]string{"taskID": taskID})
	return resp, nil
}
