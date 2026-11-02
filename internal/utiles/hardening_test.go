package utiles

import "testing"

func TestSanitizeUpdateTargetRejectsDotOnly(t *testing.T) {
	bad := []string{".", "..", "...", " .. "}
	for _, v := range bad {
		if _, err := SanitizeUpdateTarget(v); err == nil {
			t.Errorf("SanitizeUpdateTarget(%q) 应该被拒绝（路径上跳风险）", v)
		}
	}
	good := []string{"v2.2.0", "2.2.0-UGREEN", "v1.0.0-rc.1+meta"}
	for _, v := range good {
		if _, err := SanitizeUpdateTarget(v); err != nil {
			t.Errorf("SanitizeUpdateTarget(%q) = %v, want nil", v, err)
		}
	}
}

func TestManagedUpdatePlatform(t *testing.T) {
	cases := map[string]string{
		"v2.2.0-FNOS":   "FnOS",
		"v2.2.0-fnos":   "FnOS",
		"v2.1.3-UGREEN": "UGREEN",
		"v2.2.0-ugreen": "UGREEN",
		"v2.2.0":        "",
		"":              "",
	}
	for version, want := range cases {
		got, _ := ManagedUpdatePlatform(version)
		if got != want {
			t.Errorf("ManagedUpdatePlatform(%q) = %q, want %q", version, got, want)
		}
	}
}

func TestUpdateBranchDefaultsToMain(t *testing.T) {
	t.Setenv("UPDATE_BRANCH", "")
	if got := UpdateBranch(); got != "main" {
		t.Fatalf("UpdateBranch() 默认 = %q, want main（远程已无 latest 分支，曾导致版本检查 404）", got)
	}
	t.Setenv("UPDATE_BRANCH", "latest")
	if got := UpdateBranch(); got != "latest" {
		t.Fatalf("UpdateBranch() 覆盖失效: %q", got)
	}
}
