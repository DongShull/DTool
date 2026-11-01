package image

import (
	"net/http"

	"github.com/DongShull/DTool/internal/logic/image"
	"github.com/DongShull/DTool/internal/svc"
	"github.com/DongShull/DTool/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func CleanImagesHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.RemoveImagesReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := image.NewCleanImagesLogic(r.Context(), svcCtx)
		resp, err := l.CleanImages(&req)
		if err != nil {
			httpx.WriteJson(w, resp.Code, resp)
		} else {
			httpx.WriteJson(w, resp.Code, resp)
		}
	}
}
