package progress

import (
	"net/http"

	"github.com/DongShull/DTool/internal/logic/progress"
	"github.com/DongShull/DTool/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func ListProgressHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := progress.NewListProgressLogic(r.Context(), svcCtx)
		resp, err := l.ListProgress()
		if err != nil {
			httpx.WriteJson(w, resp.Code, resp)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
