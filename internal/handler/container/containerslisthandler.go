package container

import (
	"net/http"

	"github.com/DongShull/DTool/internal/handler/handlerutil"
	"github.com/DongShull/DTool/internal/logic/container"
	"github.com/DongShull/DTool/internal/svc"
	"github.com/DongShull/DTool/internal/types"
)

func ContainersListHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		handlerutil.RunNoReq(w, r, func() (*types.Resp, error) {
			l := container.NewContainersListLogic(r.Context(), svcCtx)
			return l.ContainersList()
		})
	}
}
