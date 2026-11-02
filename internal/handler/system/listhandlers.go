package system

import (
	"net/http"

	"github.com/DongShull/DTool/internal/svc"
	"github.com/DongShull/DTool/internal/types"
	"github.com/DongShull/DTool/internal/utiles"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// VolumesHandler 列出数据卷（GET /api/volumes）。
func VolumesHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		volumes, err := utiles.ListVolumes(r.Context(), svcCtx)
		if err != nil {
			httpx.WriteJson(w, http.StatusInternalServerError, types.Resp{Code: 500, Msg: err.Error()})
			return
		}
		httpx.OkJsonCtx(r.Context(), w, types.Resp{Code: 200, Msg: "success", Data: volumes})
	}
}

// NetworksHandler 列出网络（GET /api/networks）。
func NetworksHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		networks, err := utiles.ListNetworks(r.Context(), svcCtx)
		if err != nil {
			httpx.WriteJson(w, http.StatusInternalServerError, types.Resp{Code: 500, Msg: err.Error()})
			return
		}
		httpx.OkJsonCtx(r.Context(), w, types.Resp{Code: 200, Msg: "success", Data: networks})
	}
}
