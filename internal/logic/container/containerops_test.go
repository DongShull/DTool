package container

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/DongShull/DTool/internal/svc"
	"github.com/DongShull/DTool/internal/types"
	"github.com/docker/docker/client"
)

// fakeDockerServer 是一个最小化的 Docker Engine API mock，只覆盖
// inspect / start / stop / restart / unpause，并记录每次调用，
// 用于验证启停重启链路的幂等行为（不依赖真实 Docker）。
type fakeDockerServer struct {
	mu      sync.Mutex
	server  *httptest.Server
	calls   []string // 记录 "METHOD path"，不含版本前缀
	running bool
	paused  bool
}

var versionPrefixRe = regexp.MustCompile("^/v[0-9.]+/")

func newFakeDockerServer(t *testing.T, running, paused bool) *fakeDockerServer {
	t.Helper()
	f := &fakeDockerServer{running: running, paused: paused}
	mux := http.NewServeMux()
	mux.HandleFunc("/_ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := versionPrefixRe.ReplaceAllString(r.URL.Path, "/")
		f.record(r.Method + " " + path)
		switch {
		case r.Method == http.MethodGet && path == "/containers/demo/json":
			w.Header().Set("Content-Type", "application/json")
			state := "created"
			if f.running {
				state = "running"
			} else if f.paused {
				state = "paused"
			}
			fmt.Fprintf(w, "{\"Id\":\"demo0123456789abcdef\",\"Name\":\"/demo\",\"Image\":\"sha256:oldimage\",\"State\":{\"Status\":%q,\"Running\":%t,\"Paused\":%t},\"Config\":{\"Image\":\"old:latest\",\"Labels\":{}},\"HostConfig\":{}}", state, f.running, f.paused)
		case r.Method == http.MethodPost && path == "/containers/demo/start":
			f.running = true
			f.paused = false
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && path == "/containers/demo/stop":
			f.running = false
			f.paused = false
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && path == "/containers/demo/restart":
			f.running = true
			f.paused = false
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && path == "/containers/demo/unpause":
			f.paused = false
			f.running = true
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeDockerServer) record(call string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
}

func (f *fakeDockerServer) count(prefix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

// newTestServiceContext 构造指向 mock Docker 的 ServiceContext（测试专用）。
func newTestServiceContext(t *testing.T, f *fakeDockerServer) *svc.ServiceContext {
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

func TestStartContainerIdempotent(t *testing.T) {
	// 已在运行的容器：幂等成功，且不向 daemon 发送 start。
	f := newFakeDockerServer(t, true, false)
	svcCtx := newTestServiceContext(t, f)
	resp, err := NewStartLogic(context.Background(), svcCtx).Start(&types.IdReq{Id: "demo"})
	if err != nil || resp.Code != 200 {
		t.Fatalf("start running container should succeed idempotently, got resp=%+v err=%v", resp, err)
	}
	if n := f.count("POST /containers/demo/start"); n != 0 {
		t.Fatalf("running container should not be started again, start calls=%d", n)
	}

	// 已停止的容器：正常启动一次。
	f2 := newFakeDockerServer(t, false, false)
	svcCtx2 := newTestServiceContext(t, f2)
	if resp, err := NewStartLogic(context.Background(), svcCtx2).Start(&types.IdReq{Id: "demo"}); err != nil || resp.Code != 200 {
		t.Fatalf("start stopped container should succeed, got resp=%+v err=%v", resp, err)
	}
	if n := f2.count("POST /containers/demo/start"); n != 1 {
		t.Fatalf("stopped container should be started exactly once, start calls=%d", n)
	}

	// paused 容器：不能直接 start，应返回明确错误而不是 daemon 的模糊报错。
	f3 := newFakeDockerServer(t, false, true)
	svcCtx3 := newTestServiceContext(t, f3)
	resp3, err3 := NewStartLogic(context.Background(), svcCtx3).Start(&types.IdReq{Id: "demo"})
	if err3 == nil || !strings.Contains(err3.Error(), "暂停") {
		t.Fatalf("start paused container should fail with clear error, got err=%v", err3)
	}
	if resp3.Code != 400 {
		t.Fatalf("paused start should return code 400, got %d", resp3.Code)
	}
}

func TestStopContainerIdempotent(t *testing.T) {
	// 已停止的容器：幂等成功，且不向 daemon 发送 stop。
	f := newFakeDockerServer(t, false, false)
	svcCtx := newTestServiceContext(t, f)
	resp, err := NewStopLogic(context.Background(), svcCtx).Stop(&types.IdReq{Id: "demo"})
	if err != nil || resp.Code != 200 {
		t.Fatalf("stop stopped container should succeed idempotently, got resp=%+v err=%v", resp, err)
	}
	if n := f.count("POST /containers/demo/stop"); n != 0 {
		t.Fatalf("stopped container should not be stopped again, stop calls=%d", n)
	}

	// 运行中的容器：正常停止一次。
	f2 := newFakeDockerServer(t, true, false)
	svcCtx2 := newTestServiceContext(t, f2)
	if resp, err := NewStopLogic(context.Background(), svcCtx2).Stop(&types.IdReq{Id: "demo"}); err != nil || resp.Code != 200 {
		t.Fatalf("stop running container should succeed, got resp=%+v err=%v", resp, err)
	}
	if n := f2.count("POST /containers/demo/stop"); n != 1 {
		t.Fatalf("running container should be stopped exactly once, stop calls=%d", n)
	}

	// paused 容器：先 unpause 再 stop。
	f3 := newFakeDockerServer(t, true, true)
	svcCtx3 := newTestServiceContext(t, f3)
	if resp, err := NewStopLogic(context.Background(), svcCtx3).Stop(&types.IdReq{Id: "demo"}); err != nil || resp.Code != 200 {
		t.Fatalf("stop paused container should succeed, got resp=%+v err=%v", resp, err)
	}
	if n := f3.count("POST /containers/demo/unpause"); n != 1 {
		t.Fatalf("paused container should be unpaused before stop, unpause calls=%d", n)
	}
	if n := f3.count("POST /containers/demo/stop"); n != 1 {
		t.Fatalf("paused container should be stopped once, stop calls=%d", n)
	}
}

func TestRestartContainer(t *testing.T) {
	// 重启：运行中容器每次都真正执行 restart（重启不是幂等操作，语义明确）。
	f := newFakeDockerServer(t, true, false)
	svcCtx := newTestServiceContext(t, f)
	for i := 0; i < 2; i++ {
		if resp, err := NewRestartLogic(context.Background(), svcCtx).Restart(&types.IdReq{Id: "demo"}); err != nil || resp.Code != 200 {
			t.Fatalf("restart should succeed (round %d), got resp=%+v err=%v", i, resp, err)
		}
	}
	if n := f.count("POST /containers/demo/restart"); n != 2 {
		t.Fatalf("restart should hit daemon twice, restart calls=%d", n)
	}

	// 已停止的容器：Docker restart 语义是直接启动它，同样应成功。
	f2 := newFakeDockerServer(t, false, false)
	svcCtx2 := newTestServiceContext(t, f2)
	if resp, err := NewRestartLogic(context.Background(), svcCtx2).Restart(&types.IdReq{Id: "demo"}); err != nil || resp.Code != 200 {
		t.Fatalf("restart stopped container should succeed, got resp=%+v err=%v", resp, err)
	}
	if n := f2.count("POST /containers/demo/restart"); n != 1 {
		t.Fatalf("restart stopped container should hit daemon once, restart calls=%d", n)
	}
}
