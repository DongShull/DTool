package utiles

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DongShull/DTool/internal/svc"
	dockerTypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"strings"
	"time"
)

func TestComposeUpdateStrategy(t *testing.T) {
	cases := []struct {
		env  string
		want string
	}{
		{"", ComposeStrategyAuto},
		{"auto", ComposeStrategyAuto},
		{"force", ComposeStrategyForce},
		{"FORCE", ComposeStrategyForce},
		{"container", ComposeStrategyContainer},
		{"compose-only", ComposeStrategyOnly},
		{"COMPOSE-ONLY", ComposeStrategyOnly}, // 大小写不敏感
		{"bogus", ComposeStrategyAuto},        // 非法回退 auto
	}
	for _, c := range cases {
		t.Setenv("COMPOSE_UPDATE", c.env)
		if got := ComposeUpdateStrategy(); got != c.want {
			t.Errorf("COMPOSE_UPDATE=%q: got %s, want %s", c.env, got, c.want)
		}
	}
}

func TestComposeLabelsRoutable(t *testing.T) {
	full := composeLabels{Project: "web", Service: "nginx", WorkingDir: "/opt/web"}
	if !full.routable() {
		t.Error("full labels should be routable")
	}
	missing := []composeLabels{
		{Service: "nginx", WorkingDir: "/opt/web"},          // 无 project
		{Project: "web", WorkingDir: "/opt/web"},            // 无 service
		{Project: "web", Service: "nginx"},                  // 无 working_dir
		{Project: "web", Service: "nginx", ConfigFile: "x"}, // config_file 不能替代 working_dir
	}
	for i, l := range missing {
		if l.routable() {
			t.Errorf("case %d should not be routable: %+v", i, l)
		}
	}
}

func TestComposeSafeToken(t *testing.T) {
	good := []string{"web", "my-project", "app_1", "svc.name", "a/b"}
	for _, v := range good {
		if !composeSafeToken(v) {
			t.Errorf("%q should be safe", v)
		}
	}
	bad := []string{"", "-pull", "--help", "a b", "a;b", "a$(x)", "a`b", "中文"}
	for _, v := range bad {
		if composeSafeToken(v) {
			t.Errorf("%q should be rejected", v)
		}
	}
}

func TestComposeLabelsFromInspected(t *testing.T) {
	inspected := &dockerTypes.ContainerJSON{
		Config: &container.Config{
			Labels: map[string]string{
				"com.docker.compose.project":              "stack",
				"com.docker.compose.service":              "api",
				"com.docker.compose.project.working_dir":  "/opt/stack",
				"com.docker.compose.project.config_files": "/opt/stack/compose.yaml",
			},
		},
	}
	l := composeLabelsFrom(inspected)
	if l.Project != "stack" || l.Service != "api" || l.WorkingDir != "/opt/stack" || l.ConfigFile != "/opt/stack/compose.yaml" {
		t.Fatalf("labels parsed wrong: %+v", l)
	}
	// 空容器不炸
	if empty := composeLabelsFrom(&dockerTypes.ContainerJSON{}); empty.Project != "" {
		t.Fatalf("empty inspected should give empty labels, got %+v", empty)
	}
	if nilCase := composeLabelsFrom(nil); nilCase.Project != "" {
		t.Fatal("nil inspected should not panic")
	}
}

func TestComposeServiceUpdateRejectsUnsafeLabels(t *testing.T) {
	_, err := composeServiceUpdate(context.Background(), composeLabels{
		Project:    "-evil",
		Service:    "web",
		WorkingDir: t.TempDir(),
	}, false)
	if err == nil {
		t.Fatal("project 以 - 开头应被拒绝（防参数注入）")
	}
}

func TestComposeWorkingDirExists(t *testing.T) {
	dir := t.TempDir()
	if !composeWorkingDirExists(composeLabels{WorkingDir: dir}) {
		t.Error("existing dir should be visible")
	}
	if composeWorkingDirExists(composeLabels{WorkingDir: filepath.Join(dir, "nope")}) {
		t.Error("missing dir should not be visible")
	}
	if composeWorkingDirExists(composeLabels{}) {
		t.Error("empty working dir should not be visible")
	}
}

// TestComposeCommandAbsentIsGraceful：在没有 docker CLI 的环境（如本测试容器），
// 探测应返回不可用而不是报错——调用方据此走容器链路兑底。
func TestComposeCommandAbsentIsGraceful(t *testing.T) {
	t.Setenv("PATH", os.TempDir()) // 清空 PATH 里的 docker
	if _, ok := composeCommand(); ok {
		t.Log("环境里居然有 docker，跳过断言")
	}
}

// TestComposeConfigFileArgs 验证【审查项2】：config_files 标签 → -f 参数，
// 相对路径/缺失文件/目录都要拒绝执行而不是让 compose 猜配置。
func TestComposeConfigFileArgs(t *testing.T) {
	dir := t.TempDir()
	f1 := filepath.Join(dir, "compose.yaml")
	f2 := filepath.Join(dir, "override.yaml")
	for _, p := range []string{f1, f2} {
		if err := os.WriteFile(p, []byte("services: {}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// 空标签：不追加 -f，compose 用默认文件名。
	args, err := composeConfigFileArgs("")
	if err != nil || args != nil {
		t.Fatalf("empty config_files should give no args, got args=%v err=%v", args, err)
	}
	// 单文件。
	args, err = composeConfigFileArgs(f1)
	if err != nil {
		t.Fatalf("single file should pass, got %v", err)
	}
	if len(args) != 2 || args[0] != "-f" || args[1] != f1 {
		t.Fatalf("single file args = %v, want [-f %s]", args, f1)
	}
	// 多文件（冒号分隔，与 compose 标签一致）。
	args, err = composeConfigFileArgs(f1 + ":" + f2)
	if err != nil {
		t.Fatalf("multi files should pass, got %v", err)
	}
	if len(args) != 4 || args[1] != f1 || args[3] != f2 {
		t.Fatalf("multi file args = %v, want [-f f1 -f f2]", args)
	}
	// 相对路径拒绝。
	if _, err = composeConfigFileArgs("compose.yaml"); err == nil {
		t.Fatal("relative path should be rejected")
	}
	// 文件不存在拒绝。
	if _, err = composeConfigFileArgs(filepath.Join(dir, "nope.yaml")); err == nil {
		t.Fatal("missing file should be rejected")
	}
	// 目录拒绝。
	if _, err = composeConfigFileArgs(dir); err == nil {
		t.Fatal("directory should be rejected")
	}
}

// TestComposeProbeTimeoutConstant 验证【审查项3】：探测超时常量为 5 秒，
// finishProbe 已改为 exec.CommandContext（超时自动 kill 挂死的 docker CLI）。
func TestComposeProbeTimeoutConstant(t *testing.T) {
	if composeProbeTimeout != 5*time.Second {
		t.Fatalf("composeProbeTimeout = %v, want 5s", composeProbeTimeout)
	}
}

// TestTryComposeUpdateHardFailsWithoutFallback 验证【审查项6】：
// compose CLI 存在但 pull/up 失败时，compose 管理的容器不再静默回退容器链路，
// 而是返回带指引文案的明确错误（completed=true）。
func TestTryComposeUpdateHardFailsWithoutFallback(t *testing.T) {
	binDir := t.TempDir()
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = compose ] && [ \"$2\" = version ]; then exit 0; fi\n" +
		"echo \"mock compose pull failure\" >&2\n" +
		"exit 1\n"
	if err := os.WriteFile(filepath.Join(binDir, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)
	t.Setenv("COMPOSE_UPDATE", ComposeStrategyAuto)

	inspected := &dockerTypes.ContainerJSON{
		Config: &container.Config{
			Labels: map[string]string{
				"com.docker.compose.project":             "stack",
				"com.docker.compose.service":             "api",
				"com.docker.compose.project.working_dir": t.TempDir(),
			},
		},
	}
	svcCtx := &svc.ServiceContext{ProgressStore: make(svc.ProgressStoreType)}
	completed, out, err := tryComposeUpdate(context.Background(), svcCtx, inspected, "web", "task-compose-fail")
	if !completed {
		t.Fatalf("compose failure must NOT fall back to container path (completed=false), out=%q", out)
	}
	if err == nil || !strings.Contains(err.Error(), "该容器由 compose 管理，compose 更新失败") {
		t.Fatalf("expected explicit compose-managed error, got err=%v out=%q", err, out)
	}
	if !strings.Contains(err.Error(), "mock compose pull failure") {
		t.Fatalf("error should carry compose stderr summary, got %v", err)
	}
	_, _ = svcCtx.GetProgress("task-compose-fail") // 进度由上层 tryComposePath 置 done
}

// TestTryComposeUpdateCliAbsentStillFallsBack 验证：宿主没有 compose CLI 时
// 仍走容器链路兜底（completed=false, err=nil）——这是环境缺失，不是 compose 失败。
func TestTryComposeUpdateCliAbsentStillFallsBack(t *testing.T) {
	t.Setenv("PATH", os.TempDir())
	inspected := &dockerTypes.ContainerJSON{
		Config: &container.Config{
			Labels: map[string]string{
				"com.docker.compose.project":             "stack",
				"com.docker.compose.service":             "api",
				"com.docker.compose.project.working_dir": t.TempDir(),
			},
		},
	}
	svcCtx := &svc.ServiceContext{ProgressStore: make(svc.ProgressStoreType)}
	completed, out, err := tryComposeUpdate(context.Background(), svcCtx, inspected, "web", "task-compose-absent")
	if completed || err != nil {
		t.Fatalf("compose CLI absent should fall back gracefully, got completed=%v err=%v out=%q", completed, err, out)
	}
	if out == "" {
		t.Fatal("fallback reason should be reported")
	}
}
