package auth

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// loginLimiter 对 /api/auth 做进程内限流：同一来源 IP 在窗口内连续失败
// loginMaxFails 次后锁定 loginBlockWindow。目的：在 CORS 全域放开、口令
// 兼作 JWT 密钥的现状下，抬高在线爆破成本（P0 最小修复；令牌体系另行改造）。
type loginLimiter struct {
	mu       sync.Mutex
	attempts map[string]*loginAttempt
}

type loginAttempt struct {
	fails        int
	blockedUntil time.Time
}

const (
	loginMaxFails    = 5
	loginBlockWindow = 15 * time.Second
	// attempts 表超过该规模时先清理过期项，防止恶意来源撑大内存。
	loginPruneSize = 4096
)

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{attempts: make(map[string]*loginAttempt)}
}

// clientIP 取 RemoteAddr 的 host 部分（不信任 X-Forwarded-For，可被伪造）。
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (l *loginLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	a := l.attempts[ip]
	if a == nil {
		return true
	}
	return time.Now().After(a.blockedUntil)
}

func (l *loginLimiter) recordFail(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.attempts) >= loginPruneSize {
		now := time.Now()
		for k, v := range l.attempts {
			if v.blockedUntil.Before(now) {
				delete(l.attempts, k)
			}
		}
	}
	a := l.attempts[ip]
	if a == nil {
		a = &loginAttempt{}
		l.attempts[ip] = a
	}
	if time.Now().Before(a.blockedUntil) {
		return
	}
	a.fails++
	if a.fails >= loginMaxFails {
		a.blockedUntil = time.Now().Add(loginBlockWindow)
		a.fails = 0
	}
}

func (l *loginLimiter) reset(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, ip)
}
