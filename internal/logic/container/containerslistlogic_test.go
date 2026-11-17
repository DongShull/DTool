package container

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DongShull/DTool/internal/module"
	"github.com/DongShull/DTool/internal/svc"
	"github.com/docker/docker/client"
)

// newFakeListDockerServer 模拟 Docker Engine API 的 /containers/json 与
// /containers/{id}/json，供容器列表接口测试使用（不依赖真实 Docker）。
func newFakeListDockerServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/_ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := versionPrefixRe.ReplaceAllString(r.URL.Path, "/")
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && path == "/containers/json" {
			fmt.Fprint(w, "[{\"Id\":\"demo0123456789abcdef\",\"Names\":[\"/demo\"],\"Image\":\"old:latest\",\"ImageID\":\"sha256:demo\",\"State\":\"running\",\"Status\":\"Up 2 minutes\",\"Created\":1700000000,\"Labels\":{\"com.docker.compose.project\":\"demo\"},\"Ports\":[]}]")
			return
		}
		fmt.Fprint(w, "{\"Id\":\"demo0123456789abcdef\",\"Name\":\"/demo\",\"Config\":{\"Image\":\"old:latest\",\"Labels\":{}},\"State\":{\"Status\":\"running\",\"Running\":true},\"HostConfig\":{}}")
	})
	return httptest.NewServer(mux)
}

func newTestServiceContextForList(t *testing.T, serverURL string) *svc.ServiceContext {
	t.Helper()
	cli, err := client.NewClientWithOpts(
		client.WithHost("tcp://"+strings.TrimPrefix(serverURL, "http://")),
		client.WithHTTPClient(http.DefaultClient),
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		t.Fatalf("构造 mock docker client 失败: %v", err)
	}
	return &svc.ServiceContext{
		DockerClient:  cli,
		HubImageInfo:  &module.ImageUpdateData{Data: map[string]module.ImageCheckList{}},
		ProgressStore: make(svc.ProgressStoreType),
	}
}

// TestContainersListSuccessCode200 保证 GET /api/containers 成功响应的
// code 与其他接口一致，统一为 200（此前该接口遗漏赋值，序列化成 0）。
func TestContainersListSuccessCode200(t *testing.T) {
	server := newFakeListDockerServer(t)
	defer server.Close()
	svcCtx := newTestServiceContextForList(t, server.URL)

	resp, err := NewContainersListLogic(context.Background(), svcCtx).ContainersList()
	if err != nil {
		t.Fatalf("ContainersList 返回错误: %v", err)
	}
	if resp.Code != 200 {
		t.Fatalf("成功响应 code 应为 200，实际为 %d", resp.Code)
	}

	body, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("序列化响应失败: %v", err)
	}
	if !strings.Contains(string(body), "\"code\":200") {
		t.Fatalf("JSON 响应应包含 \"code\":200，实际为 %s", body)
	}
}
