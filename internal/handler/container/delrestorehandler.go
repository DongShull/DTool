package container

import (
	"net/http"

	"github.com/DongShull/DTool/internal/logic/container"
	"github.com/DongShull/DTool/internal/svc"
	"github.com/DongShull/DTool/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// 不套 handlerutil 模板：DelRestore 存在 err==nil 但业务码 400 的路径
//（非法文件名/删除失败），删除分支会把这些写成 HTTP 400；
// 统一形态会把它们变成 HTTP 200，为保持对外语义保留原样。
func DelRestoreHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.DelContainerBackupReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := container.NewDelRestoreLogic(r.Context(), svcCtx)
		resp, err := l.DelRestore(&req)
		if err != nil {
			httpx.WriteJson(w, resp.Code, resp)
		} else {
			httpx.WriteJson(w, resp.Code, resp)
		}
	}
}
