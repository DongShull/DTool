package image

import (
	"net/http"

	"github.com/DongShull/DTool/internal/handler/handlerutil"
	"github.com/DongShull/DTool/internal/logic/image"
	"github.com/DongShull/DTool/internal/svc"
	"github.com/DongShull/DTool/internal/types"
)

func RemoveHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		handlerutil.ParseAndRun(w, r, func(req *types.RemoveImageReq) (*types.Resp, error) {
			l := image.NewRemoveLogic(r.Context(), svcCtx)
			return l.Remove(req)
		})
	}
}
