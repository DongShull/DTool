package progress

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/DongShull/DTool/internal/svc"
	"github.com/DongShull/DTool/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ExportProgressLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewExportProgressLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ExportProgressLogic {
	return &ExportProgressLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// ExportProgress 按条件导出任务进度为 JSON（含汇总头）。
// 过滤条件与列表一致，另支持 from/to 时间范围（Unix 秒，按开始时间判断）。
func (l *ExportProgressLogic) ExportProgress(req *types.ExportProgressReq) (resp *types.Resp, err error) {
	resp = &types.Resp{}
	list := l.svcCtx.FilterProgress(req.Status, req.Name)
	filtered := make([]svc.TaskProgress, 0, len(list))
	for _, p := range list {
		if req.From > 0 && p.StartedAt < req.From {
			continue
		}
		if req.To > 0 && p.StartedAt > req.To {
			continue
		}
		filtered = append(filtered, p)
	}
	summary := map[string]int{"running": 0, "cancelled": 0, "done": 0}
	for _, p := range filtered {
		summary[p.Status()]++
	}
	export := map[string]interface{}{
		"exportedAt": time.Now().Format(time.RFC3339),
		"filter": map[string]interface{}{
			"status": strings.TrimSpace(req.Status),
			"name":   strings.TrimSpace(req.Name),
			"from":   req.From,
			"to":     req.To,
		},
		"summary": map[string]interface{}{
			"total":     len(filtered),
			"byStatus":  summary,
		},
		"tasks": filtered,
	}
	data, err := json.MarshalIndent(export, "", "  ")
	if err != nil {
		resp.Code = 500
		resp.Msg = fmt.Sprintf("导出序列化失败: %v", err)
		resp.Data = map[string]interface{}{}
		return resp, err
	}
	resp.Code = 200
	resp.Msg = "success"
	resp.Data = json.RawMessage(data)
	return resp, nil
}
