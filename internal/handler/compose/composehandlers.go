package compose

// Compose 工程级控制的 HTTP handler：list / up / down / logs。
// 与 MCP 工具共用 utiles 实现；up/down 的审计在 utiles.ComposeProjectAction 内统一落盘。

import (
	"net/http"

	"github.com/DongShull/DTool/internal/svc"
	"github.com/DongShull/DTool/internal/types"
	"github.com/DongShull/DTool/internal/utiles"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// ProjectsHandler 列出全部 compose 工程（GET /api/compose/projects）。
func ProjectsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projects, err := utiles.ListComposeProjects(r.Context(), svcCtx)
		if err != nil {
			httpx.WriteJson(w, http.StatusInternalServerError, types.Resp{Code: 500, Msg: err.Error()})
			return
		}
		httpx.OkJsonCtx(r.Context(), w, types.Resp{Code: 200, Msg: "success", Data: projects})
	}
}

// UpHandler 启动一个 compose 工程（POST /api/compose/:project/up，等价 compose up -d）。
func UpHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ComposeProjectNameReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.WriteJson(w, http.StatusBadRequest, types.Resp{Code: 400, Msg: err.Error()})
			return
		}
		output, err := utiles.ComposeProjectAction(r.Context(), svcCtx, req.Project, "up")
		if err != nil {
			httpx.WriteJson(w, http.StatusInternalServerError, types.Resp{Code: 500, Msg: err.Error(), Data: output})
			return
		}
		httpx.OkJsonCtx(r.Context(), w, types.Resp{Code: 200, Msg: "success", Data: map[string]string{"project": req.Project, "output": output}})
	}
}

// DownHandler 停止并移除一个 compose 工程（POST /api/compose/:project/down，等价 compose down）。
func DownHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ComposeProjectNameReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.WriteJson(w, http.StatusBadRequest, types.Resp{Code: 400, Msg: err.Error()})
			return
		}
		output, err := utiles.ComposeProjectAction(r.Context(), svcCtx, req.Project, "down")
		if err != nil {
			httpx.WriteJson(w, http.StatusInternalServerError, types.Resp{Code: 500, Msg: err.Error(), Data: output})
			return
		}
		httpx.OkJsonCtx(r.Context(), w, types.Resp{Code: 200, Msg: "success", Data: map[string]string{"project": req.Project, "output": output}})
	}
}

// LogsHandler 聚合工程日志（GET /api/compose/:project/logs?service=&tail=）。
func LogsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ComposeLogsReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.WriteJson(w, http.StatusBadRequest, types.Resp{Code: 400, Msg: err.Error()})
			return
		}
		logs, err := utiles.ComposeProjectLogs(r.Context(), svcCtx, req.Project, req.Service, req.Tail)
		if err != nil {
			httpx.WriteJson(w, http.StatusInternalServerError, types.Resp{Code: 500, Msg: err.Error()})
			return
		}
		httpx.OkJsonCtx(r.Context(), w, types.Resp{Code: 200, Msg: "success", Data: logs})
	}
}
