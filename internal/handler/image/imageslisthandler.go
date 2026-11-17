package image

import (
	"net/http"

	"github.com/DongShull/DTool/internal/handler/handlerutil"
	"github.com/DongShull/DTool/internal/logic/image"
	"github.com/DongShull/DTool/internal/svc"
	"github.com/DongShull/DTool/internal/types"
)

func ImagesListHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		handlerutil.RunNoReq(w, r, func() (*types.Resp, error) {
			l := image.NewImagesListLogic(r.Context(), svcCtx)
			return l.ImagesList()
		})
	}
}
