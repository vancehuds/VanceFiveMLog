package httpx

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	loginFailureLimit = 5
	loginWindow       = 10 * time.Minute
	loginLockout      = 10 * time.Minute
)

type loginRateLimiter struct {
	mu       sync.Mutex
	now      func() time.Time
	attempts map[string]loginAttempt
}

type loginAttempt struct {
	Failures  int
	FirstSeen time.Time
	LockedTil time.Time
}

func newLoginRateLimiter() *loginRateLimiter {
	return &loginRateLimiter{
		now:      time.Now,
		attempts: map[string]loginAttempt{},
	}
}

func (l *loginRateLimiter) Allow(identity string) bool {
	if identity == "" {
		identity = "unknown"
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.cleanup(now)
	attempt := l.attempts[identity]
	return attempt.LockedTil.IsZero() || !attempt.LockedTil.After(now)
}

func (l *loginRateLimiter) Fail(identity string) {
	if identity == "" {
		identity = "unknown"
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.cleanup(now)
	attempt := l.attempts[identity]
	if attempt.FirstSeen.IsZero() || now.Sub(attempt.FirstSeen) > loginWindow {
		attempt = loginAttempt{FirstSeen: now}
	}
	attempt.Failures++
	if attempt.Failures >= loginFailureLimit {
		attempt.LockedTil = now.Add(loginLockout)
	}
	l.attempts[identity] = attempt
}

func (l *loginRateLimiter) Success(identity string) {
	if identity == "" {
		identity = "unknown"
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, identity)
}

func (l *loginRateLimiter) cleanup(now time.Time) {
	for identity, attempt := range l.attempts {
		if !attempt.LockedTil.IsZero() && attempt.LockedTil.After(now) {
			continue
		}
		if now.Sub(attempt.FirstSeen) > loginWindow {
			delete(l.attempts, identity)
		}
	}
}

func clientIdentity(r *http.Request) string {
	for _, header := range []string{"CF-Connecting-IP", "X-Real-IP"} {
		if ip := net.ParseIP(strings.TrimSpace(r.Header.Get(header))); ip != nil {
			return ip.String()
		}
	}
	forwarded := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	if len(forwarded) > 0 {
		if ip := net.ParseIP(strings.TrimSpace(forwarded[0])); ip != nil {
			return ip.String()
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		if ip := net.ParseIP(host); ip != nil {
			return ip.String()
		}
	}
	if ip := net.ParseIP(strings.TrimSpace(r.RemoteAddr)); ip != nil {
		return ip.String()
	}
	return strings.TrimSpace(r.RemoteAddr)
}
