package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultTurnstileVerifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

var errTurnstileFailed = errors.New("turnstile verification failed")

type TurnstileConfig struct {
	SiteKey   string
	SecretKey string
	VerifyURL string
}

func (c TurnstileConfig) Enabled() bool {
	return strings.TrimSpace(c.SiteKey) != "" && strings.TrimSpace(c.SecretKey) != ""
}

func normalizeTurnstileConfig(cfg TurnstileConfig) TurnstileConfig {
	cfg.SiteKey = strings.TrimSpace(cfg.SiteKey)
	cfg.SecretKey = strings.TrimSpace(cfg.SecretKey)
	cfg.VerifyURL = strings.TrimSpace(cfg.VerifyURL)
	if cfg.VerifyURL == "" {
		cfg.VerifyURL = defaultTurnstileVerifyURL
	}
	return cfg
}

type turnstileVerifier struct {
	config TurnstileConfig
	client *http.Client
}

func newTurnstileVerifier(cfg TurnstileConfig, client *http.Client) *turnstileVerifier {
	cfg = normalizeTurnstileConfig(cfg)
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	return &turnstileVerifier{config: cfg, client: client}
}

func (v *turnstileVerifier) Enabled() bool {
	return v != nil && v.config.Enabled()
}

func (v *turnstileVerifier) SiteKey() string {
	if v == nil {
		return ""
	}
	return v.config.SiteKey
}

func (v *turnstileVerifier) Verify(ctx context.Context, token, remoteIP string) error {
	if !v.Enabled() {
		return nil
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return errTurnstileFailed
	}

	form := url.Values{}
	form.Set("secret", v.config.SecretKey)
	form.Set("response", token)
	if ip := normalizeIP(remoteIP); ip != "" {
		form.Set("remoteip", ip)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.config.VerifyURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("create turnstile request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := v.client.Do(req)
	if err != nil {
		return fmt.Errorf("verify turnstile: %w", err)
	}
	defer resp.Body.Close()

	var out struct {
		Success    bool     `json:"success"`
		ErrorCodes []string `json:"error-codes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fmt.Errorf("decode turnstile response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("turnstile returned status %d", resp.StatusCode)
	}
	if !out.Success {
		return errTurnstileFailed
	}
	return nil
}

func normalizeIP(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	if ip := net.ParseIP(value); ip != nil {
		return ip.String()
	}
	return ""
}
