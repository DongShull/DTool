package version

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DongShull/DTool/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// 证明 UpdateProgramReq 对 JSON body 的解析行为（当前：target 被忽略）。
// PUT /api/program 是破坏性接口（会换二进制并退出进程），解析层不能"静默降级"。
func TestUpdateProgramReqJSONTarget(t *testing.T) {
	r := httptest.NewRequest("PUT", "/api/program", strings.NewReader(`{"target":"v2.2.0"}`))
	r.Header.Set("Content-Type", "application/json")

	var req types.UpdateProgramReq
	err := httpx.Parse(r, &req)
	t.Logf("err=%v target=%q", err, req.Target)
	if strings.TrimSpace(req.Target) != "v2.2.0" {
		t.Fatalf("JSON body 里的 target 没有被解析：target=%q err=%v", req.Target, err)
	}
}

// 证明非法 JSON body 时 Parse 会报错（handler 目前丢弃该错误）。
func TestUpdateProgramReqMalformedJSON(t *testing.T) {
	r := httptest.NewRequest("PUT", "/api/program", strings.NewReader(`{"target":`))
	r.Header.Set("Content-Type", "application/json")

	var req types.UpdateProgramReq
	err := httpx.Parse(r, &req)
	t.Logf("err=%v target=%q", err, req.Target)
	if err == nil {
		t.Fatalf("非法 JSON 应当返回解析错误")
	}
}

// 表单形式的 target 目前是唯一可用的传递方式，保持可用。
func TestUpdateProgramReqFormTarget(t *testing.T) {
	r := httptest.NewRequest("PUT", "/api/program", strings.NewReader(`target=v2.2.0`))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var req types.UpdateProgramReq
	if err := httpx.Parse(r, &req); err != nil {
		t.Fatalf("form 解析失败: %v", err)
	}
	if req.Target != "v2.2.0" {
		t.Fatalf("form target = %q", req.Target)
	}
}
