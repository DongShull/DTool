package utiles

// Compose 工程级控制的纯函数单测：cli 参数构造、token 校验、状态推导、聚合。
// 全部为 mock 纯函数测试，不依赖真实 docker。

import (
	"testing"

	MyType "github.com/DongShull/DTool/internal/types"
)

// 与 composeupdate_test.go 的 TestComposeSafeToken 互补：这里覆盖边界与拒绝集。
func TestComposeSafeTokenBoundary(t *testing.T) {
	valid := []string{"myproj", "web_app", "app.web", "a1-b2_c3.d4", "org/app", "name+edge", "foo@bar"}
	for _, v := range valid {
		if !ComposeSafeToken(v) {
			t.Errorf("ComposeSafeToken(%q) = false, want true", v)
		}
	}
	invalid := []string{
		"",
		"-proj",         // 以 - 开头：会被 compose 当成自己的选项
		"pro j",         // 空格
		"pro;j",         // shell 元字符（防御性拒绝）
		"pro&j",         // shell 元字符
		"pro|j",         // shell 元字符
		"pro$j",         // 变量展开
		"pro" + string(rune(92)) + "j", // 反斜杠
		"pro*j",         // 通配符
		"换名字",          // 非 ASCII
	}
	for _, v := range invalid {
		if ComposeSafeToken(v) {
			t.Errorf("ComposeSafeToken(%q) = true, want false", v)
		}
	}
}

func TestComposeLifecycleArgs(t *testing.T) {
	upArgs, err := ComposeLifecycleArgs("up", "myproj")
	if err != nil {
		t.Fatalf("up: unexpected error: %v", err)
	}
	if len(upArgs) != 2 || upArgs[0] != "up" || upArgs[1] != "-d" {
		t.Errorf("up args = %v, want [up -d]", upArgs)
	}

	downArgs, err := ComposeLifecycleArgs("down", "myproj")
	if err != nil {
		t.Fatalf("down: unexpected error: %v", err)
	}
	if len(downArgs) != 1 || downArgs[0] != "down" {
		t.Errorf("down args = %v, want [down]", downArgs)
	}

	// 大小写与空白容忍
	if _, err = ComposeLifecycleArgs("  UP ", "myproj"); err != nil {
		t.Errorf("uppercase up: unexpected error: %v", err)
	}

	// 非法动作
	if _, err = ComposeLifecycleArgs("rm", "myproj"); err == nil {
		t.Error("action rm should be rejected")
	}

	// 注入式项目名必须被拒绝
	for _, bad := range []string{"-p evil", "", "-v", "a;b"} {
		if _, err = ComposeLifecycleArgs("down", bad); err == nil {
			t.Errorf("project %q should be rejected", bad)
		}
	}
}

func TestComposeProjectStatus(t *testing.T) {
	cases := []struct {
		running, stopped int
		want             string
	}{
		{2, 0, "running"},
		{1, 1, "partial"},
		{0, 2, "stopped"},
		{0, 0, "empty"},
	}
	for _, c := range cases {
		if got := composeProjectStatus(c.running, c.stopped); got != c.want {
			t.Errorf("composeProjectStatus(%d,%d) = %q, want %q", c.running, c.stopped, got, c.want)
		}
	}
}

func TestAggregateComposeProjects(t *testing.T) {
	// 直接测试聚合逻辑的输入形态：通过 listComposeBriefs 的输出类型手工构造。
	briefs := []composeProjectBrief{
		{Project: "web", Service: "nginx", Workdir: "/opt/web", Name: "web-nginx-1", State: "running", Image: "nginx:latest"},
		{Project: "web", Service: "nginx", Workdir: "/opt/web", Name: "web-nginx-2", State: "running", Image: "nginx:latest"},
		{Project: "web", Service: "redis", Workdir: "/opt/web", Name: "web-redis-1", State: "exited", Image: "redis:7"},
		{Project: "db", Service: "pg", Workdir: "/opt/db", Name: "db-pg-1", State: "exited", Image: "postgres:16"},
		{Project: "", Service: "", Name: "non-compose", State: "running"}, // 无标签的容器会被过滤
	}

	byProject := map[string]*MyType.ComposeProjectInfo{}
	order := []string{}
	for _, b := range briefs {
		if b.Project == "" {
			continue
		}
		info, ok := byProject[b.Project]
		if !ok {
			info = &MyType.ComposeProjectInfo{Project: b.Project, Workdir: b.Workdir, Containers: []MyType.ComposeContainerBrief{}}
			byProject[b.Project] = info
			order = append(order, b.Project)
		}
		if info.Workdir == "" {
			info.Workdir = b.Workdir
		}
		info.Containers = append(info.Containers, MyType.ComposeContainerBrief{Name: b.Name, Service: b.Service, State: b.State, Image: b.Image})
		if b.State == "running" {
			info.Running++
		} else {
			info.Stopped++
		}
	}
	if len(order) != 2 {
		t.Fatalf("aggregated %d projects, want 2 (non-compose filtered)", len(order))
	}
	web := byProject["web"]
	if web.Services = func() int { seen := map[string]bool{}; for _, c := range web.Containers { seen[c.Service] = true }; return len(seen) }(); web.Services != 2 {
		t.Errorf("web services = %d, want 2 (nginx 副本去重)", web.Services)
	}
	if web.Running != 2 || web.Stopped != 1 {
		t.Errorf("web running/stopped = %d/%d, want 2/1", web.Running, web.Stopped)
	}
	if web.Status = composeProjectStatus(web.Running, web.Stopped); web.Status != "partial" {
		t.Errorf("web status = %q, want partial", web.Status)
	}
}

func TestMapResultDetail(t *testing.T) {
	if got := mapResultDetail("done\nsecond\nthird", nil); got != "ok: done second third" {
		t.Errorf("mapResultDetail ok = %q", got)
	}
	if got := mapResultDetail("", nil); got != "ok" {
		t.Errorf("mapResultDetail empty = %q", got)
	}
	got := mapResultDetail("some output", errFake)
	if got == "" || len(got) > 300 {
		t.Errorf("mapResultDetail err = %q", got)
	}
}

var errFake = &fakeError{}

type fakeError struct{}

func (*fakeError) Error() string { return "fake failure" }

// TestComposeContainerName 覆盖 composeContainerName 的取名规则，
// 特别是 Names 为空时不得越界 panic（原实现先取 Names[0] 再判空的缺陷）。
func TestComposeContainerName(t *testing.T) {
	cases := []struct {
		id    string
		names []string
		want  string
	}{
		{"abcdef1234567890", []string{"/web-nginx-1"}, "web-nginx-1"},                     // 正常：去前导 /
		{"abcdef1234567890", nil, "abcdef123456"},                                        // 空 Names：回退 ID 前 12 位
		{"abcdef1234567890", []string{}, "abcdef123456"},                                 // 空 slice：同上
		{"short", nil, "short"},                                                          // ID 不足 12 位：原样返回
		{"abcdef1234567890", []string{""}, "abcdef123456"},                               // Names[0] 为空：按无名字处理
		{"abcdef1234567890", []string{"no-slash"}, "no-slash"},                           // 无前导 /：原样返回
	}
	for _, c := range cases {
		if got := composeContainerName(c.id, c.names); got != c.want {
			t.Errorf("composeContainerName(%q, %v) = %q, want %q", c.id, c.names, got, c.want)
		}
	}
}

// TestComposeLabelsOf：统一标签解析的键名提取（composeproject.go / composeupdate.go 共用）。
func TestComposeLabelsOf(t *testing.T) {
	labels := ComposeLabelsOf(map[string]string{
		"com.docker.compose.project":              "stack",
		"com.docker.compose.service":              "api",
		"com.docker.compose.project.working_dir":  "/opt/stack",
		"com.docker.compose.project.config_files": "/opt/stack/compose.yaml",
	})
	if labels.Project != "stack" || labels.Service != "api" ||
		labels.WorkingDir != "/opt/stack" || labels.ConfigFile != "/opt/stack/compose.yaml" {
		t.Fatalf("ComposeLabelsOf = %+v", labels)
	}
	// 缺键与非 compose 标签都取零值
	empty := ComposeLabelsOf(map[string]string{"other": "x"})
	if empty != (composeLabels{}) {
		t.Errorf("unrelated labels should give zero value, got %+v", empty)
	}
	if nilCase := ComposeLabelsOf(nil); nilCase != (composeLabels{}) {
		t.Errorf("nil map should give zero value, got %+v", nilCase)
	}
}
