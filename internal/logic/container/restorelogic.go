package container

import (
	"context"
	"fmt"
	"github.com/DongShull/DTool/internal/utiles"
	"path/filepath"

	"github.com/DongShull/DTool/internal/svc"
	"github.com/DongShull/DTool/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type RestoreLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewRestoreLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RestoreLogic {
	return &RestoreLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *RestoreLogic) Restore(req *types.ContainerRestoreReq) (resp *types.Resp, err error) {
	resp = &types.Resp{}
	var taskID string
	fileName := req.Filename
	fullPath, pathErr := utiles.ResolveBackupPath(fileName, ".json")
	if pathErr != nil {
		resp = types.Fail(types.CodeBadRequest, pathErr.Error(), map[string]interface{}{})
		return resp, pathErr
	}
	fileName = filepath.Base(fullPath)
	if filepath.Ext(fileName) != ".json" {
		err = fmt.Errorf("目前仅支持config备份恢复")
		resp = types.Fail(types.CodeBadRequest, err.Error(), map[string]interface{}{})
		return resp, err
	}
	// RunManagedTask 统一封装：占位进度（恢复全局串行，排队期间可查可取消）、
	// panic recover、通知、审计与收尾。HTTP 链路异步语义：立即返回 taskID。
	taskID, _ = utiles.RunManagedTask(l.svcCtx, l.Logger, utiles.ManagedTaskMeta{
		Name:        "恢复容器",
		NotifyKind:  "restore",
		AuditAction: "container.restore",
		Placeholder: svc.TaskProgress{
			Name:      "恢复容器",
			Message:   "任务已排队",
			DetailMsg: "等待上一轮恢复结束",
		},
		PanicMessage:   "恢复失败",
		PanicDetailFmt: "恢复任务异常中断: %v",
		DoneFallback:   "恢复任务已结束",
	}, nil, func(taskCtx context.Context, taskID string) error {
		return utiles.RestoreContainer(taskCtx, l.svcCtx, fileName, taskID, utiles.RestoreOptions{
			StartOverride: req.Start,
			Force:         req.Force,
			NameMap:       req.NameMap,
		})
	})
	resp = types.OK(map[string]interface{}{
		"taskID": taskID,
	})
	return resp, nil
}
