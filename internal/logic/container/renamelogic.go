package container

import (
	"context"
	"strings"

	"github.com/DongShull/DTool/internal/utiles"

	"github.com/DongShull/DTool/internal/svc"
	"github.com/DongShull/DTool/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type RenameLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewRenameLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RenameLogic {
	return &RenameLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *RenameLogic) Rename(req *types.ContainerRenameReq) (resp *types.Resp, err error) {
	resp = &types.Resp{}
	newName := strings.TrimSpace(req.NewName)
	if newName == "" {
		resp = types.Fail(types.CodeBadRequest, "新容器名不能为空", map[string]interface{}{})
		return resp, nil
	}
	err = utiles.RenameContainer(l.svcCtx, req.Id, newName)
	if err != nil {
		resp = types.Fail(types.CodeBadRequest, err.Error(), map[string]interface{}{})
		return resp, err
	}
	utiles.AuditLog("container.rename", req.Id, "newName="+newName)
	resp = types.OK(map[string]interface{}{})
	return resp, nil
}
