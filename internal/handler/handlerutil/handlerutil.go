// Package handlerutil 收敛 HTTP handler 的「解析请求 → 调 logic → 按结果写响应」样板。
// 放在独立子包而不是 internal/handler 根包：routes.go（package handler）import 各
// 子包 handler，子包反向 import 根包会形成环。
//
// 全项目统一响应写出口（信封 {"code","msg","data"}）：
//   - logic 返回 err != nil：httpx.WriteJson(w, resp.Code, resp)，HTTP 状态码 = resp.Code；
//   - logic 返回 err == nil：httpx.OkJsonCtx（HTTP 200），业务码在 body.code；
// 与项目主模板（container/stophandler.go 等逐字模板）逐字节一致。
package handlerutil

import (
	"net/http"

	"github.com/DongShull/DTool/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// ParseAndRun 解析请求 → 调 logic → 按统一形态写响应。
func ParseAndRun[REQ any](w http.ResponseWriter, r *http.Request, run func(req *REQ) (*types.Resp, error)) {
	var req REQ
	if err := httpx.Parse(r, &req); err != nil {
		httpx.ErrorCtx(r.Context(), w, err)
		return
	}
	resp, err := run(&req)
	if err != nil {
		httpx.WriteJson(w, resp.Code, resp)
	} else {
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

// RunNoReq 无请求参数版本的 ParseAndRun。
func RunNoReq(w http.ResponseWriter, r *http.Request, run func() (*types.Resp, error)) {
	resp, err := run()
	if err != nil {
		httpx.WriteJson(w, resp.Code, resp)
	} else {
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}
