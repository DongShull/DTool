package container

import (
	"net/http"

	"github.com/DongShull/DTool/internal/logic/container"
	"github.com/DongShull/DTool/internal/svc"
	"github.com/DongShull/DTool/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func Backup2composeHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := container.NewBackup2composeLogic(r.Context(), svcCtx)
		resp, err := l.Backup2compose()
		if err != nil {
			// Backup2compose logic 失败时 resp 恒为 nil（见 backup2composelogic.go），
			// 无法像 handlerutil.ParseAndRun 那样取 resp.Code 写状态码，故在此
			// 兜底为统一信封（HTTP 500 / code=500），不再走 httpx.ErrorCtx 的
			// SetErrorHandler（那会输出 code=50000 的非契约码）。此分支为前两轮
			// 统一收口的漏网点，对外行为由 200/50000 变为 500/500。
			httpx.WriteJson(w, http.StatusInternalServerError, types.Resp{
				Code: types.CodeServerError,
				Msg:  err.Error(),
				Data: map[string]interface{}{},
			})
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}
