package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLoginLimiterBlocksAfterFails(t *testing.T) {
	l := newLoginLimiter()
	ip := "203.0.113.10"

	for i := 0; i < loginMaxFails; i++ {
		if !l.allow(ip) {
			t.Fatalf("第 %d 次失败后不应被锁定", i)
		}
		l.recordFail(ip)
	}
	if l.allow(ip) {
		t.Fatalf("连续 %d 次失败后应被锁定", loginMaxFails)
	}
}

func TestLoginLimiterResetOnSuccess(t *testing.T) {
	l := newLoginLimiter()
	ip := "203.0.113.11"

	for i := 0; i < loginMaxFails-1; i++ {
		l.recordFail(ip)
	}
	l.reset(ip)
	if !l.allow(ip) {
		t.Fatalf("成功登录后应清除计数")
	}
	l.recordFail(ip)
	if !l.allow(ip) {
		t.Fatalf("清零后单次失败不应触发锁定")
	}
}

func TestLoginLimiterIsolatedPerIP(t *testing.T) {
	l := newLoginLimiter()
	bad := "203.0.113.12"
	good := "198.51.100.7"

	for i := 0; i < loginMaxFails; i++ {
		l.recordFail(bad)
	}
	if !l.allow(good) {
		t.Fatalf("其它 IP 不应被连带锁定")
	}
}

func TestClientIP(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/api/auth", nil)
	r.RemoteAddr = "10.0.0.8:54321"
	if got := clientIP(r); got != "10.0.0.8" {
		t.Fatalf("clientIP = %q, want 10.0.0.8", got)
	}
}
