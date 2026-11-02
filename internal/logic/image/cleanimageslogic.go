package image

import (
	"context"
	"strconv"

	"github.com/DongShull/DTool/internal/svc"
	"github.com/DongShull/DTool/internal/types"
	"github.com/DongShull/DTool/internal/utiles"

	"github.com/zeromicro/go-zero/core/logx"
)

type CleanImagesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCleanImagesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CleanImagesLogic {
	return &CleanImagesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// CleanImages 批量清理镜像：filter=untagged 删除无 TAG 镜像，
// filter=unused 删除未被任何容器引用的镜像。单个失败不影响其它。
func (l *CleanImagesLogic) CleanImages(req *types.RemoveImagesReq) (resp *types.Resp, err error) {
	resp = &types.Resp{}
	if req.Filter != utiles.CleanupFilterUntagged && req.Filter != utiles.CleanupFilterUnused {
		resp.Code = 400
		resp.Msg = "filter 参数错误，只支持 untagged（无 TAG 镜像）或 unused（未使用镜像）"
		resp.Data = map[string]interface{}{}
		return resp, nil
	}
	removed, failed, err := utiles.RemoveImagesFiltered(l.svcCtx, req.Filter)
	if err != nil {
		resp.Code = 500
		resp.Msg = err.Error()
		resp.Data = map[string]interface{}{}
		return resp, err
	}
	// 已删除/失败清单都进审计，删除操作不可恢复，必须留痕。
	utiles.AuditLog("image.cleanup", req.Filter,
		"removed="+strconv.Itoa(len(removed))+", failed="+strconv.Itoa(len(failed)))
	// 部分失败用 207 明确区分：镜像已被真删除、无法回滚，
	// 把"部分失败"报成 200/success 会让调用方误判清理结果。
	resp.Code = 200
	resp.Msg = "success"
	if len(failed) > 0 {
		resp.Code = 207
		resp.Msg = "清理部分失败：" + strconv.Itoa(len(failed)) + " 个镜像未删除（详见 failed 字段）"
	}
	resp.Data = map[string]interface{}{
		"filter":  req.Filter,
		"removed": removed,
		"failed":  failed,
	}
	return resp, nil
}
