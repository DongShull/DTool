package version

import (
	"net/http"

	"github.com/DongShull/DTool/internal/logic/version"
	"github.com/DongShull/DTool/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func UpdateProgramHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := version.NewUpdateProgramLogic(r.Context(), svcCtx)
		resp, err := l.UpdateProgram()
		if err != nil {
			httpx.WriteJson(w, resp.Code, resp)
		} else {
			httpx.WriteJson(w, resp.Code, resp)
		}
	}
}
