package version

import (
	"context"
	"testing"

	"github.com/DongShull/DTool/internal/svc"
	"github.com/DongShull/DTool/internal/types"
)

// TestVersionDefaultsToLocal 回归：type 曾为必填 query，裸 GET /api/version
// 会返回 code:50000 "field \"type\" is not set"。现在缺省按 local 处理。
func TestVersionDefaultsToLocal(t *testing.T) {
	l := NewVersionLogic(context.Background(), &svc.ServiceContext{})

	resp, err := l.Version(&types.VersionReq{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Code != 200 {
		t.Fatalf("code = %d, want 200 (msg=%q)", resp.Code, resp.Msg)
	}
	if _, ok := resp.Data.(map[string]string); !ok {
		t.Fatalf("data type = %T, want map[string]string", resp.Data)
	}
}

// TestVersionExplicitTypeStillWorks 确保显式 type=local 行为不变。
func TestVersionExplicitTypeStillWorks(t *testing.T) {
	l := NewVersionLogic(context.Background(), &svc.ServiceContext{})

	resp, err := l.Version(&types.VersionReq{Type: "local"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Code != 200 {
		t.Fatalf("code = %d, want 200", resp.Code)
	}
}
