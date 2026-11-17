package container

import (
	"net/http"

	"github.com/DongShull/DTool/internal/handler/handlerutil"
	"github.com/DongShull/DTool/internal/logic/container"
	"github.com/DongShull/DTool/internal/svc"
	"github.com/DongShull/DTool/internal/types"
)

func RestartHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		handlerutil.ParseAndRun(w, r, func(req *types.IdReq) (*types.Resp, error) {
			l := container.NewRestartLogic(r.Context(), svcCtx)
			return l.Restart(req)
		})
	}
}
