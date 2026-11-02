package handler

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/DongShull/DTool/internal/config"
	"github.com/DongShull/DTool/internal/svc"
	"github.com/golang-jwt/jwt"
	"github.com/zeromicro/go-zero/rest"
)

func TestProgressRouteRequiresJWT(t *testing.T) {
	port := getFreePort(t)
	cfg := config.Config{}
	cfg.Host = "127.0.0.1"
	cfg.Port = port
	cfg.Auth.AccessSecret = "test-secret"

	server := rest.MustNewServer(cfg.RestConf)
	RegisterHandlers(server, &svc.ServiceContext{Config: cfg})
	defer server.Stop()

	go server.Start()
	waitForServer(t, port)

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/api/progress/test-task", port))
	if err != nil {
		t.Fatalf("failed to call progress route: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without JWT, got %d", resp.StatusCode)
	}
}

func TestAuthRouteDoesNotRequireJWT(t *testing.T) {
	port := getFreePort(t)
	cfg := config.Config{}
	cfg.Host = "127.0.0.1"
	cfg.Port = port
	cfg.Auth.AccessSecret = "test-secret"

	server := rest.MustNewServer(cfg.RestConf)
	RegisterHandlers(server, &svc.ServiceContext{Config: cfg})
	defer server.Stop()

	go server.Start()
	waitForServer(t, port)

	req, err := http.NewRequest(
		http.MethodPost,
		fmt.Sprintf("http://127.0.0.1:%d/api/auth", port),
		bytes.NewBufferString("secretKey=test-secret"),
	)
	if err != nil {
		t.Fatalf("failed to create auth request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to call auth route: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read auth response body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from auth route without JWT, got %d with body %s", resp.StatusCode, string(body))
	}
}

// TestAuthRouteAcceptsJSONBody 回归：LoginReq 此前只有 form tag，
// JSON body 里的 secretKey 不会被解析，登录恒返回 401"无效的secretKey"。
func TestAuthRouteAcceptsJSONBody(t *testing.T) {
	port := getFreePort(t)
	cfg := config.Config{}
	cfg.Host = "127.0.0.1"
	cfg.Port = port
	cfg.Auth.AccessSecret = "test-secret"

	server := rest.MustNewServer(cfg.RestConf)
	RegisterHandlers(server, &svc.ServiceContext{Config: cfg})
	defer server.Stop()

	go server.Start()
	waitForServer(t, port)

	req, err := http.NewRequest(
		http.MethodPost,
		fmt.Sprintf("http://127.0.0.1:%d/api/auth", port),
		bytes.NewBufferString(`{"secretKey":"test-secret"}`),
	)
	if err != nil {
		t.Fatalf("failed to create auth request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to call auth route: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read auth response body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for JSON login, got %d with body %s", resp.StatusCode, string(body))
	}
	if !bytes.Contains(body, []byte(`"jwt":"`)) {
		t.Fatalf("expected jwt in JSON login response, got %s", string(body))
	}
}

// TestProgressExportRouteNotShadowed 回归：/api/progress/export 必须命中
// 导出逻辑（返回 exportedAt），而不是被 /api/progress/:taskid 抢走
// （那会返回"taskID 未找到"）。go-zero 的 patrouter 按注册顺序取首个
// 匹配，导出路由注册在参数路由之前。
func TestProgressExportRouteNotShadowed(t *testing.T) {
	port := getFreePort(t)
	cfg := config.Config{}
	cfg.Host = "127.0.0.1"
	cfg.Port = port
	cfg.Auth.AccessSecret = "test-secret"

	server := rest.MustNewServer(cfg.RestConf)
	RegisterHandlers(server, &svc.ServiceContext{Config: cfg})
	defer server.Stop()

	go server.Start()
	waitForServer(t, port)

	token := testJWT(t, "test-secret")
	req, err := http.NewRequest(
		http.MethodGet,
		fmt.Sprintf("http://127.0.0.1:%d/api/progress/export", port),
		nil,
	)
	if err != nil {
		t.Fatalf("failed to create export request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to call export route: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read export response body: %v", err)
	}
	if bytes.Contains(body, []byte("taskID 未找到")) {
		t.Fatalf("export route shadowed by /progress/:taskid, body: %s", string(body))
	}
	if !bytes.Contains(body, []byte("exportedAt")) {
		t.Fatalf("expected export payload with exportedAt, got %s", string(body))
	}
}

// testJWT 用与后端一致的 HS256 签发测试令牌（claims 仅 iat/exp，与 loginlogic 一致）。
func testJWT(t *testing.T, secret string) string {
	t.Helper()
	claims := jwt.MapClaims{
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	token := jwt.New(jwt.SigningMethodHS256)
	token.Claims = claims
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("failed to sign test jwt: %v", err)
	}
	return signed
}

func getFreePort(t *testing.T) int {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to get free port: %v", err)
	}
	defer listener.Close()

	return listener.Addr().(*net.TCPAddr).Port
}

func waitForServer(t *testing.T, port int) {
	t.Helper()

	client := &http.Client{Timeout: 200 * time.Millisecond}
	url := fmt.Sprintf("http://127.0.0.1:%d/", port)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("server on port %d did not become ready", port)
}
