package image

import (
	"context"
	"github.com/DongShull/DTool/internal/utiles"
	"strings"

	"github.com/DongShull/DTool/internal/svc"
	"github.com/DongShull/DTool/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type RemoveLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewRemoveLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RemoveLogic {
	return &RemoveLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *RemoveLogic) Remove(req *types.RemoveImageReq) (resp *types.Resp, err error) {
	resp = &types.Resp{}
	var imageId = req.Id
	imageId = strings.TrimPrefix(imageId, "sha256:")
	err = utiles.RemoveImage(l.svcCtx, imageId, req.Force)
	if err != nil {
		resp = types.Fail(types.CodeConflict, err.Error(), map[string]interface{}{})
		return resp, nil
	}
	utiles.AuditLog("image.remove", req.Id, "")
	resp = types.OK(map[string]interface{}{})
	return resp, nil
}
