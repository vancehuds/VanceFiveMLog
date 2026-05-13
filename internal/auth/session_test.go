package auth

import (
	"net/http/httptest"
	"testing"
)

func TestSessionRoundTrip(t *testing.T) {
	manager := NewSessionManager("0123456789abcdef0123456789abcdef")
	rec := httptest.NewRecorder()
	if err := manager.Set(rec, 42); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	for _, cookie := range rec.Result().Cookies() {
		req.AddCookie(cookie)
	}

	session, err := manager.Read(req)
	if err != nil {
		t.Fatal(err)
	}
	if session.AdminID != 42 {
		t.Fatalf("admin id = %d", session.AdminID)
	}
}

func TestCSRFTokenRoundTrip(t *testing.T) {
	manager := NewSessionManager("0123456789abcdef0123456789abcdef")
	rec := httptest.NewRecorder()
	if err := manager.Set(rec, 7); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	for _, cookie := range rec.Result().Cookies() {
		req.AddCookie(cookie)
	}
	session, err := manager.Read(req)
	if err != nil {
		t.Fatal(err)
	}

	token := manager.CSRFToken(session)
	if token == "" {
		t.Fatal("expected csrf token")
	}
	if !manager.VerifyCSRFToken(session, token) {
		t.Fatal("expected csrf token to verify")
	}
	if manager.VerifyCSRFToken(session, token+"x") {
		t.Fatal("expected tampered csrf token to fail")
	}
}

func TestSecureCookieOption(t *testing.T) {
	manager := NewSessionManager("0123456789abcdef0123456789abcdef").WithSecureCookie(true)
	rec := httptest.NewRecorder()
	if err := manager.Set(rec, 42); err != nil {
		t.Fatal(err)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d", len(cookies))
	}
	if !cookies[0].Secure {
		t.Fatal("expected secure cookie")
	}
}
