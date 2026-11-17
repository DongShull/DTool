package container

import (
	"net/http"

	"github.com/DongShull/DTool/internal/handler/handlerutil"
	"github.com/DongShull/DTool/internal/logic/container"
	"github.com/DongShull/DTool/internal/svc"
	"github.com/DongShull/DTool/internal/types"
)

func RenameHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		handlerutil.ParseAndRun(w, r, func(req *types.ContainerRenameReq) (*types.Resp, error) {
			l := container.NewRenameLogic(r.Context(), svcCtx)
			return l.Rename(req)
		})
	}
}
