package image

import (
	"net/http"

	"github.com/DongShull/DTool/internal/logic/image"
	"github.com/DongShull/DTool/internal/svc"
	"github.com/DongShull/DTool/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// 不套 handlerutil 模板：CleanImages 的部分失败用 207（types.CodePartial）+
// filter 参数错误用 400（err==nil 的业务码）作为 HTTP 状态码区分，
// 统一到「err==nil 一律 200」会丢掉该语义，故保留原样。
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
