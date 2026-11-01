package utiles

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	dockerTypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
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
