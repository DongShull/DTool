package system

import (
	"net/http"

	"github.com/DongShull/DTool/internal/svc"
	"github.com/DongShull/DTool/internal/types"
	"github.com/DongShull/DTool/internal/utiles"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// OverviewHandler 返回 Docker 版本、主机信息与磁盘占用（GET /api/system）。
func OverviewHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		overview, err := utiles.SystemOverview(r.Context(), svcCtx)
		if err != nil {
			httpx.WriteJson(w, http.StatusInternalServerError, types.Resp{
				Code: 500,
				Msg:  err.Error(),
			})
			return
		}
		httpx.OkJsonCtx(r.Context(), w, types.Resp{
			Code: 200,
			Msg:  "success",
			Data: overview,
		})
	}
}
