package container

import (
	"net/http"

	"github.com/DongShull/DTool/internal/handler/handlerutil"
	"github.com/DongShull/DTool/internal/logic/container"
	"github.com/DongShull/DTool/internal/svc"
	"github.com/DongShull/DTool/internal/types"
)

func RestoreHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 与同目录 renamehandler 等统一为 handlerutil.ParseAndRun 形态。
		// 注意 Restore logic 存在 err!=nil 但 resp 已填充（types.Fail 400）的
		// 业务失败路径（非法文件名/仅支持 config 备份）：ParseAndRun 会把
		// resp.Code=400 写成 HTTP 400，与 docs/API.md 的文档约定一致
		//（旧行为经 SetErrorHandler 输出 HTTP 200 + code=50000，为漏网点）。
		handlerutil.ParseAndRun(w, r, func(req *types.ContainerRestoreReq) (*types.Resp, error) {
			l := container.NewRestoreLogic(r.Context(), svcCtx)
			return l.Restore(req)
		})
	}
}
