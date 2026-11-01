package version

import (
	"net/http"

	"github.com/DongShull/DTool/internal/config"
	"github.com/DongShull/DTool/internal/svc"
	"github.com/DongShull/DTool/internal/utiles"
	"strconv"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// UpdateHistoryHandler 返回程序自更新历史（最新在前）。
// limit 可选；默认 20，最大 50。
func UpdateHistoryHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := 20
		if v := r.URL.Query().Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				limit = n
			}
		}
		if limit > 50 {
			limit = 50
		}
		httpx.OkJsonCtx(r.Context(), w, map[string]interface{}{
			"code": 200,
			"msg":  "success",
			"data": map[string]interface{}{
				"currentVersion": config.Version,
				"buildDate":      config.BuildDate,
				"history":        utiles.UpdateHistory(limit),
			},
		})
	}
}
