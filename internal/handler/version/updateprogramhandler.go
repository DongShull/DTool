package version

import (
	"net/http"

	"github.com/DongShull/DTool/internal/logic/version"
	"github.com/DongShull/DTool/internal/svc"
	"github.com/DongShull/DTool/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func UpdateProgramHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.UpdateProgramReq
		_ = httpx.Parse(r, &req)
		l := version.NewUpdateProgramLogic(r.Context(), svcCtx)
		resp, err := l.UpdateProgram(&req)
		if err != nil {
			httpx.WriteJson(w, resp.Code, resp)
		} else {
			httpx.WriteJson(w, resp.Code, resp)
		}
	}
}
