package auth

import (
	"github.com/DongShull/DTool/internal/logic/auth"
	"github.com/DongShull/DTool/internal/svc"
	"github.com/DongShull/DTool/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
	"net/http"
)

// limiter 进程内登录限流：见 loginlimiter.go。
var limiter = newLoginLimiter()

func LoginHandler(ctx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if !limiter.allow(ip) {
			// 429 而不是 401：让客户端能区分“口令错”与“被限流”。
			httpx.WriteJson(w, http.StatusTooManyRequests, types.Resp{
				Code: 429,
				Msg:  "尝试过于频繁，请稍后再试",
			})
			return
		}
		var req types.LoginReq
		if err := httpx.Parse(r, &req); err != nil {
			var resp types.Resp
			resp.Code = 400
			resp.Msg = "错误的请求"
			httpx.WriteJson(w, 400, resp)
			return
		}
		l := auth.NewLoginLogic(r.Context(), ctx)
		resp, err := l.Login(&req)
		if err != nil {
			limiter.recordFail(ip)
			httpx.WriteJson(w, resp.Code, resp)
			return
		}
		limiter.reset(ip)
		httpx.OkJson(w, resp)
	}
}
