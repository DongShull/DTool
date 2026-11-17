package module

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetDigestReturnsContentDigestHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Errorf("GetDigest 应发 HEAD 请求，实际 %s", r.Method)
		}
		if r.Header.Get("Accept") == "" {
			t.Error("GetDigest 应携带 Accept 头")
		}
		w.Header().Set("Docker-Content-Digest", "sha256:abc123")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	digest, err := GetDigest(server.URL, "")
	if err != nil {
		t.Fatalf("GetDigest() error = %v", err)
	}
	if digest != "sha256:abc123" {
		t.Fatalf("GetDigest() = %q, want sha256:abc123", digest)
	}
}

func TestGetDigestRejectsNon200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="https://auth.example.com"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()

	_, err := GetDigest(server.URL, "")
	if err == nil {
		t.Fatal("GetDigest() expected non-200 error")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Fatalf("GetDigest() error = %v, 应包含状态码 401", err)
	}
}
