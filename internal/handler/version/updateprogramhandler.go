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
		// 这是破坏性接口（换二进制 + 退出进程），解析失败绝不能继续执行：
		// 之前的 `_ = httpx.Parse` 会把"更新到指定版本"静默降级成"更新到最新版"。
		if err := httpx.Parse(r, &req); err != nil {
			httpx.WriteJson(w, http.StatusBadRequest, types.Resp{
				Code: 400,
				Msg:  "请求参数解析失败: " + err.Error(),
			})
			return
		}
		l := version.NewUpdateProgramLogic(r.Context(), svcCtx)
		resp, err := l.UpdateProgram(&req)
		if err != nil {
			httpx.WriteJson(w, resp.Code, resp)
		} else {
			httpx.WriteJson(w, resp.Code, resp)
		}
	}
}
