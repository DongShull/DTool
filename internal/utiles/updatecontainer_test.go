package utiles

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DongShull/DTool/internal/svc"
	"github.com/docker/docker/client"
)

// fakeUpdateServer 是针对 UpdateContainer 全链路的最小 Docker Engine API mock：
// 覆盖 inspect / pull / create / start / stop / rename / remove / list / image remove，
// 并记录每次调用与关键参数，用于验证回滚路径与指定镜像 tag（不依赖真实 Docker）。
type fakeUpdateServer struct {
	mu        sync.Mutex
	server    *httptest.Server
	calls     []string // "METHOD path?rawquery"
	bodies    map[string]string
	paused    bool
	failStart bool
}

var updateVersionPrefixRe = regexp.MustCompile("^/v[0-9.]+/")

func newFakeUpdateServer(t *testing.T, failStart bool) *fakeUpdateServer {
	t.Helper()
	f := &fakeUpdateServer{failStart: failStart, bodies: make(map[string]string)}
	mux := http.NewServeMux()
	mux.HandleFunc("/_ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := updateVersionPrefixRe.ReplaceAllString(r.URL.Path, "/")
		f.record(r.Method + " " + path + "?" + r.URL.RawQuery)
		switch {
		case r.Method == http.MethodGet && path == "/containers/dtool/json":
			// 自身保护测试用：名为 dtool 的容器，ID 是"自身"ID（selfGuardShort 前缀）。
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, "{\"Id\":\"%s\",\"Name\":\"/dtool\",\"Image\":\"sha256:oldimage\",\"State\":{\"Status\":\"running\",\"Running\":true,\"Paused\":false,\"ExitCode\":0},\"Config\":{\"Image\":\"old:latest\",\"Labels\":{}},\"HostConfig\":{}}", selfGuardLong)
		case r.Method == http.MethodGet && (path == "/containers/cid123/json" || path == "/containers/demo/json"):
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, "{\"Id\":\"cid123\",\"Name\":\"/demo\",\"Image\":\"sha256:oldimage\",\"State\":{\"Status\":\"running\",\"Running\":true,\"Paused\":%t,\"ExitCode\":0},\"Config\":{\"Image\":\"old:latest\",\"Labels\":{}},\"HostConfig\":{}}", f.paused)
		case r.Method == http.MethodPost && path == "/images/create":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, "{\"status\":\"Download complete\"}\n")
		case r.Method == http.MethodPost && path == "/containers/create":
			body, _ := io.ReadAll(r.Body)
			f.setBody("create", string(body))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, "{\"Id\":\"newcontainerid1\",\"Warnings\":[]}")
		case r.Method == http.MethodPost && path == "/containers/demo/start":
			if f.failStart {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = io.WriteString(w, "{\"message\":\"mock start failure\"}")
				return
			}
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/start"):
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/stop"):
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/unpause"):
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/pause"):
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/rename"):
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodDelete && strings.HasPrefix(path, "/containers/"):
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && path == "/containers/json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, "[]")
		case r.Method == http.MethodDelete && strings.HasPrefix(path, "/images/"):
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeUpdateServer) record(call string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
}

func (f *fakeUpdateServer) setBody(key, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.bodies[key] = body
}

func (f *fakeUpdateServer) getBody(key string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.bodies[key]
}

func (f *fakeUpdateServer) has(prefix string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

func newUpdateTestContext(t *testing.T, f *fakeUpdateServer) *svc.ServiceContext {
	t.Helper()
	t.Setenv("PROGRESS_STORE", filepath.Join(t.TempDir(), "progress.json"))
	t.Setenv("AUDIT_LOG", filepath.Join(t.TempDir(), "audit.jsonl"))
	cli, err := client.NewClientWithOpts(
		client.WithHost("tcp://"+strings.TrimPrefix(f.server.URL, "http://")),
		client.WithHTTPClient(f.server.Client()),
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		t.Fatalf("构造 mock docker client 失败: %v", err)
	}
	return &svc.ServiceContext{
		ProgressStore: make(svc.ProgressStoreType),
		DockerClient:  cli,
	}
}

// TestUpdateContainerRollbackOnStartFailure 验证更新链路的回滚路径：
// 新容器启动失败时，移除新容器、旧容器改回原名并保持运行，旧容器不被删除。
func TestUpdateContainerRollbackOnStartFailure(t *testing.T) {
	f := newFakeUpdateServer(t, true) // 新容器（名字 demo）start 失败
	svcCtx := newUpdateTestContext(t, f)

	err := UpdateContainer(svcCtx, nil, "cid123", "demo", "old:latest", true, "task-rollback")
	if err == nil {
		t.Fatalf("expected update to fail when new container cannot start")
	}
	final, ok := svcCtx.GetProgress("task-rollback")
	if !ok || !final.IsDone {
		t.Fatalf("progress should be done, got %+v ok=%v", final, ok)
	}
	if final.Message != "更新失败，已回滚旧容器" {
		t.Fatalf("rollback progress message = %q, want 更新失败，已回滚旧容器", final.Message)
	}
	// 旧容器曾改名为备份名（rename?name=demo-*）。
	if !f.has("POST /containers/cid123/rename?name=demo-") {
		t.Fatalf("old container should be renamed to backup, calls=%v", f.calls)
	}
	// 占用原名的新容器被回滚移除。
	if !f.has("DELETE /containers/demo") {
		t.Fatalf("failed new container should be removed during rollback, calls=%v", f.calls)
	}
	// 旧容器改回原名。
	if !f.has("POST /containers/cid123/rename?name=demo") && !f.has("POST /containers/cid123/rename?name=demo&") {
		t.Fatalf("old container should be renamed back, calls=%v", f.calls)
	}
	// 旧容器不被删除。
	for _, c := range f.calls {
		if strings.HasPrefix(c, "DELETE /containers/cid123") {
			t.Fatalf("old container must not be removed on rollback, calls=%v", f.calls)
		}
	}
}

// TestUpdateContainerWithImageTag 验证指定镜像名:tag 的更新：
// 拉取的就是该 ref，新容器克隆配置时镜像替换为该 ref，成功后删除旧容器并清理旧镜像。
func TestUpdateContainerWithImageTag(t *testing.T) {
	f := newFakeUpdateServer(t, false)
	svcCtx := newUpdateTestContext(t, f)

	err := UpdateContainer(svcCtx, nil, "cid123", "demo", "myrepo/myapp:v2", true, "task-tag")
	if err != nil {
		t.Fatalf("update with explicit tag should succeed, got err=%v, calls=%v", err, f.calls)
	}
	final, _ := svcCtx.GetProgress("task-tag")
	if final.Message != "更新成功" {
		t.Fatalf("progress message = %q, want 更新成功", final.Message)
	}
	// 拉取的就是指定的镜像与 tag。
	pulled := false
	for _, c := range f.calls {
		if strings.HasPrefix(c, "POST /images/create") &&
			strings.Contains(c, "fromImage=myrepo%2Fmyapp") && strings.Contains(c, "tag=v2") {
			pulled = true
		}
	}
	if !pulled {
		t.Fatalf("should pull myrepo/myapp:v2, calls=%v", f.calls)
	}
	// 新容器配置里的镜像替换为指定 ref。
	body := f.getBody("create")
	if !strings.Contains(body, "\"Image\":\"myrepo/myapp:v2\"") {
		t.Fatalf("new container should use myrepo/myapp:v2, body=%s", body)
	}
	// 旧容器被删除，旧镜像被清理。
	if !f.has("DELETE /containers/cid123") {
		t.Fatalf("old container should be removed, calls=%v", f.calls)
	}
	if !f.has("DELETE /images/sha256:oldimage") {
		t.Fatalf("old image should be cleaned up, calls=%v", f.calls)
	}
}

// TestUpdateContainerRejectsSelf 验证自身容器保护：目标是 DTool 自身时直接拒绝。
func TestUpdateContainerRejectsSelf(t *testing.T) {
	selfID := "abcdef012345" // 12 位短 ID，命中 HOSTNAME 兜底探测
	t.Setenv("HOSTNAME", selfID)
	svcCtx := &svc.ServiceContext{ProgressStore: make(svc.ProgressStoreType)}
	err := UpdateContainer(svcCtx, nil, selfID, "self", "old:latest", true, "task-self")
	if err == nil || !strings.Contains(err.Error(), "自身") {
		t.Fatalf("self container update should be rejected, got err=%v", err)
	}
}

// TestUpdateContainerEmptyImageKeepsCurrent 验证镜像留空时回退到当前镜像。
func TestUpdateContainerEmptyImageKeepsCurrent(t *testing.T) {
	f := newFakeUpdateServer(t, false)
	svcCtx := newUpdateTestContext(t, f)

	err := UpdateContainer(svcCtx, nil, "cid123", "demo", "", true, "task-empty")
	if err != nil {
		t.Fatalf("update without image should fall back to current image, got err=%v", err)
	}
	pulled := false
	for _, c := range f.calls {
		if strings.HasPrefix(c, "POST /images/create") &&
			strings.Contains(c, "fromImage=old") && strings.Contains(c, "tag=latest") {
			pulled = true
		}
	}
	if !pulled {
		t.Fatalf("should pull current image old:latest, calls=%v", f.calls)
	}
	body := f.getBody("create")
	if !strings.Contains(body, "\"Image\":\"old:latest\"") {
		t.Fatalf("new container should keep old:latest, body=%s", body)
	}
}

// 确保 json 与 os 导入不被误报未使用（inspect 文档由 Fprintf 输出）。
var _ = json.Marshal
var _ = os.Getenv
var _ = time.Now

// selfGuardShort 是自身保护测试用的“DTool 自身容器”短 ID。
const selfGuardShort = "abcdef012345"

// selfGuardLong 是对应的 64 位容器 ID。
var selfGuardLong = selfGuardShort + strings.Repeat("a", 52)

// TestUpdateContainerRejectsSelfByContainerName 验证【审查项1】：用户传 DTool
// 容器名（如 "dtool"）而不是 ID 时，也要被自身保护拦截——先 Inspect 解析出
// 真实容器 ID 再与 selfID 比对，不能只比 ID。
func TestUpdateContainerRejectsSelfByContainerName(t *testing.T) {
	t.Setenv("HOSTNAME", selfGuardShort)
	f := newFakeUpdateServer(t, false)
	svcCtx := newUpdateTestContext(t, f)

	err := UpdateContainer(svcCtx, nil, "dtool", "dtool", "old:latest", true, "task-self-name")
	if err == nil || !strings.Contains(err.Error(), "自身") {
		t.Fatalf("update by self container NAME should be rejected, got err=%v, calls=%v", err, f.calls)
	}
	// 绝不能走进更新链路（拉镜像/停止容器）。
	if f.has("POST /images/create") || f.has("POST /containers/dtool/stop") {
		t.Fatalf("self container must not enter update flow, calls=%v", f.calls)
	}
}

// TestUpdateContainerRejectsSelfShortID 验证短 ID 命中自身仍被拦截
//（旧 ID 匹配行为保持兼容，且该检查不依赖 Docker 客户端）。
func TestUpdateContainerRejectsSelfShortID(t *testing.T) {
	t.Setenv("HOSTNAME", selfGuardShort)
	f := newFakeUpdateServer(t, false)
	svcCtx := newUpdateTestContext(t, f)

	err := UpdateContainer(svcCtx, nil, selfGuardShort, "self", "old:latest", true, "task-self-short")
	if err == nil || !strings.Contains(err.Error(), "自身") {
		t.Fatalf("self short ID should be rejected, got err=%v", err)
	}
	if f.has("POST /images/create") {
		t.Fatalf("self container must not enter update flow, calls=%v", f.calls)
	}
}

// TestUpdateContainerAllowsOtherContainerWithSelfGuardPresent 验证普通容器放行：
// 自身探测环境就绪（HOSTNAME 是合法短 ID）时，非自身容器照常完成更新全链路。
func TestUpdateContainerAllowsOtherContainerWithSelfGuardPresent(t *testing.T) {
	t.Setenv("HOSTNAME", selfGuardShort)
	f := newFakeUpdateServer(t, false)
	svcCtx := newUpdateTestContext(t, f)

	err := UpdateContainer(svcCtx, nil, "cid123", "demo", "myrepo/myapp:v2", true, "task-other")
	if err != nil {
		t.Fatalf("normal container should pass self-guard and update, got err=%v, calls=%v", err, f.calls)
	}
	if !f.has("DELETE /containers/cid123") {
		t.Fatalf("normal update flow should complete, calls=%v", f.calls)
	}
}

// TestUpdateContainerNilClientOnlyAffectsNonSelf 验证【审查项5】：
// 非 self 目标 + nil DockerClient 时返回明确的 Docker 未初始化错误而不是 panic。
func TestUpdateContainerNilClientOnlyAffectsNonSelf(t *testing.T) {
	svcCtx := &svc.ServiceContext{ProgressStore: make(svc.ProgressStoreType)}
	err := UpdateContainer(svcCtx, nil, "notself", "demo", "img:1", true, "task-nil")
	if err == nil || !strings.Contains(err.Error(), "Docker 客户端未初始化") {
		t.Fatalf("nil DockerClient should give explicit error, got err=%v", err)
	}
}
