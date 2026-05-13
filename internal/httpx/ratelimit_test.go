package httpx

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestLoginRateLimiterLocksAfterFailures(t *testing.T) {
	now := time.Unix(100, 0)
	limiter := newLoginRateLimiter()
	limiter.now = func() time.Time { return now }

	for range loginFailureLimit {
		if !limiter.Allow("127.0.0.1") {
			t.Fatal("unexpected early lockout")
		}
		limiter.Fail("127.0.0.1")
	}
	if limiter.Allow("127.0.0.1") {
		t.Fatal("expected lockout")
	}

	now = now.Add(loginLockout + time.Second)
	if !limiter.Allow("127.0.0.1") {
		t.Fatal("expected lockout to expire")
	}
}

func TestLoginRateLimiterSuccessClearsFailures(t *testing.T) {
	limiter := newLoginRateLimiter()
	limiter.Fail("127.0.0.1")
	limiter.Success("127.0.0.1")

	for range loginFailureLimit - 1 {
		if !limiter.Allow("127.0.0.1") {
			t.Fatal("unexpected lockout after success")
		}
		limiter.Fail("127.0.0.1")
	}
	if !limiter.Allow("127.0.0.1") {
		t.Fatal("expected one remaining attempt")
	}
}

func TestClientIdentityPrefersForwardedHeaders(t *testing.T) {
	req := httptest.NewRequest("POST", "/login", nil)
	req.RemoteAddr = "10.0.0.2:1234"
	req.Header.Set("X-Forwarded-For", "203.0.113.10, 10.0.0.2")

	if got := clientIdentity(req); got != "203.0.113.10" {
		t.Fatalf("identity = %s", got)
	}
}
