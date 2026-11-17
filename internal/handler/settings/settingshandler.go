package settings

import (
	"net/http"

	"github.com/DongShull/DTool/internal/handler/handlerutil"
	"github.com/DongShull/DTool/internal/logic/settings"
	"github.com/DongShull/DTool/internal/svc"
	"github.com/DongShull/DTool/internal/types"
)

func GetSettingsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		handlerutil.RunNoReq(w, r, func() (*types.Resp, error) {
			l := settings.NewSettingsLogic(r.Context(), svcCtx)
			return l.GetSettings()
		})
	}
}

func UpdateSettingsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		handlerutil.ParseAndRun(w, r, func(req *types.UpdateSettingsReq) (*types.Resp, error) {
			l := settings.NewSettingsLogic(r.Context(), svcCtx)
			return l.UpdateSettings(req)
		})
	}
}
