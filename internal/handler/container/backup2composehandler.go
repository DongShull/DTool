package container

import (
	"net/http"

	"github.com/DongShull/DTool/internal/logic/container"
	"github.com/DongShull/DTool/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func Backup2composeHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := container.NewBackup2composeLogic(r.Context(), svcCtx)
		resp, err := l.Backup2compose()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
