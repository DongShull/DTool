package utiles

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateHistoryAppendAndLimit(t *testing.T) {
	t.Setenv("UPDATE_HISTORY", filepath.Join(t.TempDir(), "history.json"))

	for i := 0; i < 8; i++ {
		AppendUpdateHistory(UpdateHistoryEntry{
			FromVersion: "v1",
			ToVersion:   "v2",
			OK:          i%2 == 0,
			Manual:      i%3 == 0,
			Message:     "msg",
		})
	}
	all := UpdateHistory(0)
	if len(all) != 8 {
		t.Fatalf("history = %d entries, want 8", len(all))
	}
	// 最新在前：全部条目时间相同（秒级），用截断验证 limit 生效即可。
	if got := UpdateHistory(3); len(got) != 3 {
		t.Fatalf("limit=3 got %d entries", len(got))
	}
}

func TestUpdateHistorySurvivesCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	t.Setenv("UPDATE_HISTORY", path)
	if err := os.WriteFile(path, []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}
	AppendUpdateHistory(UpdateHistoryEntry{FromVersion: "a", ToVersion: "b", OK: true})
	got := UpdateHistory(0)
	if len(got) != 1 {
		t.Fatalf("corrupt file should be rebuilt, got %d entries", len(got))
	}
}

func TestUpdateHistoryPruneToMax(t *testing.T) {
	t.Setenv("UPDATE_HISTORY", filepath.Join(t.TempDir(), "history.json"))
	for i := 0; i < updateHistoryMax+10; i++ {
		AppendUpdateHistory(UpdateHistoryEntry{Message: strings.Repeat("x", 1)})
	}
	got := UpdateHistory(0)
	if len(got) > updateHistoryMax {
		t.Fatalf("history not pruned: %d > %d", len(got), updateHistoryMax)
	}
}

func TestRemoveLastUpdateHistory(t *testing.T) {
	t.Setenv("UPDATE_HISTORY", filepath.Join(t.TempDir(), "history.json"))
	AppendUpdateHistory(UpdateHistoryEntry{Message: "first"})
	AppendUpdateHistory(UpdateHistoryEntry{Message: "second"})
	RemoveLastUpdateHistory()
	got := UpdateHistory(0)
	if len(got) != 1 || got[0].Message != "first" {
		t.Fatalf("after remove: %+v, want only [first]", got)
	}
	// 空历史时删除不报错
	RemoveLastUpdateHistory()
	RemoveLastUpdateHistory()
}

func TestSanitizeUpdateTarget(t *testing.T) {
	ok := []string{"v2.2.1", "v2.2.1-FNOS", "1.0.0+build.5", "latest_dev-1"}
	for _, v := range ok {
		if _, err := SanitizeUpdateTarget(v); err != nil {
			t.Errorf("%q should pass, got %v", v, err)
		}
	}
	bad := []string{"", "  ", "../../etc", "a/b", "a?b", "v1 v2", "%00"}
	for _, v := range bad {
		if _, err := SanitizeUpdateTarget(v); err == nil {
			t.Errorf("%q should be rejected", v)
		}
	}
}

func TestUpdateProgramDisabled(t *testing.T) {
	t.Setenv("UPDATE_PROGRAM_ENABLED", "")
	if UpdateProgramDisabled() {
		t.Error("unset env should mean enabled")
	}
	t.Setenv("UPDATE_PROGRAM_ENABLED", "true")
	if UpdateProgramDisabled() {
		t.Error("true should mean enabled")
	}
	t.Setenv("UPDATE_PROGRAM_ENABLED", "false")
	if !UpdateProgramDisabled() {
		t.Error("false should mean disabled")
	}
	t.Setenv("UPDATE_PROGRAM_ENABLED", " FALSE ")
	if !UpdateProgramDisabled() {
		t.Error("case/space-insensitive false should mean disabled")
	}
}

// TestUpdateHistoryEntryJSON 保证字段名稳定：历史文件要跨版本可读。
func TestUpdateHistoryEntryJSON(t *testing.T) {
	data, err := json.Marshal(UpdateHistoryEntry{FromVersion: "a", ToVersion: "b", OK: true, Manual: true, Message: "m"})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"fromVersion"`, `"toVersion"`, `"ok"`, `"manual"`, `"message"`, `"time"`} {
		if !strings.Contains(string(data), key) {
			t.Errorf("json missing %s: %s", key, data)
		}
	}
}
