package version

import (
	"net/http"

	"github.com/DongShull/DTool/internal/logic/version"
	"github.com/DongShull/DTool/internal/svc"
	"github.com/DongShull/DTool/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// 不套 handlerutil 模板：Version logic 存在 err==nil 但业务码 400/500 的路径
//（type 参数错误 / 远端版本获取失败），当前把它们写成对应 HTTP 状态码；
// 统一形态会把这些变成 HTTP 200，为保持对外语义保留原样。
func VersionHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.VersionReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := version.NewVersionLogic(r.Context(), svcCtx)
		resp, err := l.Version(&req)
		if err != nil {
			httpx.WriteJson(w, resp.Code, resp)
		} else {
			httpx.WriteJson(w, resp.Code, resp)
		}
	}
}
