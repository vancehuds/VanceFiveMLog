package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTurnstileVerifierSkipsWhenDisabled(t *testing.T) {
	verifier := newTurnstileVerifier(TurnstileConfig{}, nil)
	if verifier.Enabled() {
		t.Fatal("expected verifier to be disabled")
	}
	if err := verifier.Verify(context.Background(), "", ""); err != nil {
		t.Fatal(err)
	}
}

func TestTurnstileVerifierPostsSiteverifyForm(t *testing.T) {
	var gotSecret, gotResponse, gotRemoteIP string
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
			t.Fatalf("content-type = %s", got)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		gotSecret = r.FormValue("secret")
		gotResponse = r.FormValue("response")
		gotRemoteIP = r.FormValue("remoteip")
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer endpoint.Close()

	verifier := newTurnstileVerifier(TurnstileConfig{
		SiteKey:   "0xsite",
		SecretKey: "0xsecret",
		VerifyURL: endpoint.URL,
	}, endpoint.Client())
	if err := verifier.Verify(context.Background(), "token", "203.0.113.10:4555"); err != nil {
		t.Fatal(err)
	}
	if gotSecret != "0xsecret" || gotResponse != "token" || gotRemoteIP != "203.0.113.10" {
		t.Fatalf("posted secret=%q response=%q remoteip=%q", gotSecret, gotResponse, gotRemoteIP)
	}
}

func TestTurnstileVerifierRejectsFailedChallenge(t *testing.T) {
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":false,"error-codes":["invalid-input-response"]}`))
	}))
	defer endpoint.Close()

	verifier := newTurnstileVerifier(TurnstileConfig{
		SiteKey:   "0xsite",
		SecretKey: "0xsecret",
		VerifyURL: endpoint.URL,
	}, endpoint.Client())
	if err := verifier.Verify(context.Background(), "bad-token", "203.0.113.10"); err == nil {
		t.Fatal("expected failed challenge to be rejected")
	}
}

func TestNormalizeIPIgnoresInvalidValues(t *testing.T) {
	for _, value := range []string{"", "not-ip", strings.Repeat("1", 128)} {
		if got := normalizeIP(value); got != "" {
			t.Fatalf("normalizeIP(%q) = %q", value, got)
		}
	}
}
