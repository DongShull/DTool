package system

import (
	"net/http"
	"strconv"

	"github.com/DongShull/DTool/internal/svc"
	"github.com/DongShull/DTool/internal/types"
	"github.com/DongShull/DTool/internal/utiles"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// RemoveVolumeHandler 删除一个数据卷（DELETE /api/volumes/{name}?force=true）。
// 卷里的数据不可恢复，前端应二次确认；这里把审计写进 utiles.RemoveVolume。
func RemoveVolumeHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.VolumeNameReq
		if err := httpx.Parse(r, &req); err != nil || req.Name == "" {
			httpx.WriteJson(w, http.StatusBadRequest, types.Resp{Code: 400, Msg: "卷名不能为空"})
			return
		}
		name := req.Name
		force := false
		if v := r.URL.Query().Get("force"); v != "" {
			if parsed, err := strconv.ParseBool(v); err == nil {
				force = parsed
			}
		}
		if err := utiles.RemoveVolume(r.Context(), svcCtx, name, force); err != nil {
			httpx.WriteJson(w, http.StatusBadRequest, types.Resp{Code: 400, Msg: err.Error()})
			return
		}
		httpx.OkJsonCtx(r.Context(), w, types.Resp{
			Code: 200,
			Msg:  "success",
			Data: map[string]interface{}{"name": name, "force": force},
		})
	}
}

// ContainerDetailHandler 返回容器 inspect 摘要（GET /api/container/{id}/detail）。
func ContainerDetailHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.IdReq
		if err := httpx.Parse(r, &req); err != nil || req.Id == "" {
			httpx.WriteJson(w, http.StatusBadRequest, types.Resp{Code: 400, Msg: "容器 ID 不能为空"})
			return
		}
		id := req.Id
		detail, err := utiles.ContainerDetail(r.Context(), svcCtx, id)
		if err != nil {
			httpx.WriteJson(w, http.StatusNotFound, types.Resp{Code: 404, Msg: err.Error()})
			return
		}
		httpx.OkJsonCtx(r.Context(), w, types.Resp{Code: 200, Msg: "success", Data: detail})
	}
}

// ContainerLogsHandler 读取容器最近 N 行日志（GET /api/container/{id}/logs?tail=200&timestamps=false&since=）。
func ContainerLogsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.IdReq
		if err := httpx.Parse(r, &req); err != nil || req.Id == "" {
			httpx.WriteJson(w, http.StatusBadRequest, types.Resp{Code: 400, Msg: "容器 ID 不能为空"})
			return
		}
		id := req.Id
		tail := 200
		if v := r.URL.Query().Get("tail"); v != "" {
			if parsed, err := strconv.Atoi(v); err == nil {
				tail = parsed
			}
		}
		timestamps := false
		if v := r.URL.Query().Get("timestamps"); v != "" {
			if parsed, err := strconv.ParseBool(v); err == nil {
				timestamps = parsed
			}
		}
		lines, err := utiles.ContainerLogs(r.Context(), svcCtx, id, tail, timestamps, r.URL.Query().Get("since"))
		if err != nil {
			httpx.WriteJson(w, http.StatusBadRequest, types.Resp{Code: 400, Msg: err.Error()})
			return
		}
		httpx.OkJsonCtx(r.Context(), w, types.Resp{
			Code: 200,
			Msg:  "success",
			Data: map[string]interface{}{"containerId": id, "tail": tail, "lines": lines},
		})
	}
}

// ContainerStatsHandler 取一次资源采样（GET /api/container/{id}/stats）。
func ContainerStatsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.IdReq
		if err := httpx.Parse(r, &req); err != nil || req.Id == "" {
			httpx.WriteJson(w, http.StatusBadRequest, types.Resp{Code: 400, Msg: "容器 ID 不能为空"})
			return
		}
		id := req.Id
		stat, err := utiles.ContainerStatOnce(r.Context(), svcCtx, id)
		if err != nil {
			httpx.WriteJson(w, http.StatusBadRequest, types.Resp{Code: 400, Msg: err.Error()})
			return
		}
		httpx.OkJsonCtx(r.Context(), w, types.Resp{Code: 200, Msg: "success", Data: stat})
	}
}
