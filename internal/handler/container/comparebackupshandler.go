package container

import (
	"net/http"

	"github.com/DongShull/DTool/internal/handler/handlerutil"
	"github.com/DongShull/DTool/internal/logic/container"
	"github.com/DongShull/DTool/internal/svc"
	"github.com/DongShull/DTool/internal/types"
)

func CompareBackupsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		handlerutil.ParseAndRun(w, r, func(req *types.BackupCompareReq) (*types.Resp, error) {
			l := container.NewCompareBackupsLogic(r.Context(), svcCtx)
			return l.CompareBackups(req)
		})
	}
}
