package progress

import (
	"net/http"

	"github.com/DongShull/DTool/internal/logic/progress"
	"github.com/DongShull/DTool/internal/svc"
	"github.com/DongShull/DTool/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func ExportProgressHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ExportProgressReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := progress.NewExportProgressLogic(r.Context(), svcCtx)
		resp, err := l.ExportProgress(&req)
		if err != nil {
			httpx.WriteJson(w, resp.Code, resp)
		} else {
			// Content-Disposition 提示浏览器下载为文件；filename 不含用户输入，无注入面。
			w.Header().Set("Content-Disposition", `attachment; filename="progress-export.json"`)
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
