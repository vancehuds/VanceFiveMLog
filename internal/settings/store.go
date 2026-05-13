package settings

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vancehuds/VanceFiveMLog/internal/timezone"
)

type Store struct {
	db *pgxpool.Pool
}

var ErrInvalidAIProviderConfig = errors.New("invalid ai provider config")

const (
	AIProviderDisabled = "disabled"
	AIProviderOpenAI   = "openai"
	AIProviderCustom   = "custom"
)

const (
	maxAIProviderBytes = 32
	maxAIBaseURLBytes  = 512
	maxAIAPIKeyBytes   = 4096
	maxAIModelBytes    = 128
)

type AIProviderConfig struct {
	Provider string
	BaseURL  string
	APIKey   string
	Model    string
}

func NewStore(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

func (s *Store) RetentionDays(ctx context.Context, fallback int) int {
	var raw string
	err := s.db.QueryRow(ctx, `SELECT value FROM settings WHERE key = 'retention_days'`).Scan(&raw)
	if err != nil {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return fallback
	}
	return n
}

func (s *Store) SetRetentionDays(ctx context.Context, days int) error {
	if days < 1 {
		days = 1
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO settings (key, value, updated_at)
		VALUES ('retention_days', $1, now())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()
	`, strconv.Itoa(days))
	return err
}

func (s *Store) TimeZone(ctx context.Context, fallback string) string {
	fallback, err := timezone.Normalize(fallback)
	if err != nil {
		fallback = timezone.Default
	}
	var raw string
	err = s.db.QueryRow(ctx, `SELECT value FROM settings WHERE key = 'time_zone'`).Scan(&raw)
	if err != nil {
		return fallback
	}
	zone, err := timezone.Normalize(raw)
	if err != nil {
		return fallback
	}
	return zone
}

func (s *Store) SetTimeZone(ctx context.Context, name string) error {
	zone, err := timezone.Normalize(strings.TrimSpace(name))
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO settings (key, value, updated_at)
		VALUES ('time_zone', $1, now())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()
	`, zone)
	return err
}

func (s *Store) AIProviderConfig(ctx context.Context, fallback AIProviderConfig) AIProviderConfig {
	cfg := NormalizeAIProviderConfigLenient(fallback)
	values, err := s.values(ctx, []string{"ai_json_provider", "ai_json_base_url", "ai_json_api_key", "ai_json_model"})
	if err != nil {
		return cfg
	}
	if value, ok := values["ai_json_provider"]; ok {
		cfg.Provider = NormalizeAIProvider(value)
	}
	if cfg.Provider == AIProviderDisabled {
		return AIProviderConfig{Provider: AIProviderDisabled}
	}
	if value, ok := values["ai_json_base_url"]; ok {
		cfg.BaseURL = value
	}
	if value, ok := values["ai_json_api_key"]; ok {
		cfg.APIKey = value
	}
	if value, ok := values["ai_json_model"]; ok {
		cfg.Model = value
	}
	return NormalizeAIProviderConfigLenient(cfg)
}

func (s *Store) SetAIProviderConfig(ctx context.Context, cfg AIProviderConfig) error {
	normalized, err := NormalizeAIProviderConfig(cfg)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	values := map[string]string{
		"ai_json_provider": normalized.Provider,
		"ai_json_base_url": normalized.BaseURL,
		"ai_json_api_key":  normalized.APIKey,
		"ai_json_model":    normalized.Model,
	}
	for key, value := range values {
		if _, err := tx.Exec(ctx, `
			INSERT INTO settings (key, value, updated_at)
			VALUES ($1, $2, now())
			ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()
		`, key, value); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) values(ctx context.Context, keys []string) (map[string]string, error) {
	rows, err := s.db.Query(ctx, `SELECT key, value FROM settings WHERE key = ANY($1)`, keys)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	values := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		values[key] = value
	}
	return values, rows.Err()
}

func NormalizeAIProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case AIProviderDisabled:
		return AIProviderDisabled
	case AIProviderCustom:
		return AIProviderCustom
	default:
		return AIProviderOpenAI
	}
}

func NormalizeAIProviderConfig(input AIProviderConfig) (AIProviderConfig, error) {
	cfg := AIProviderConfig{
		Provider: NormalizeAIProvider(limitBytes(input.Provider, maxAIProviderBytes)),
		BaseURL:  strings.TrimSpace(limitBytes(input.BaseURL, maxAIBaseURLBytes)),
		APIKey:   strings.TrimSpace(limitBytes(input.APIKey, maxAIAPIKeyBytes)),
		Model:    strings.TrimSpace(limitBytes(input.Model, maxAIModelBytes)),
	}
	if cfg.Provider == AIProviderDisabled {
		return AIProviderConfig{Provider: AIProviderDisabled}, nil
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultAIBaseURL(cfg.Provider)
	}
	normalizedURL, err := normalizeHTTPURL(cfg.BaseURL)
	if err != nil {
		return AIProviderConfig{}, err
	}
	cfg.BaseURL = normalizedURL
	return cfg, nil
}

func NormalizeAIProviderConfigLenient(input AIProviderConfig) AIProviderConfig {
	cfg := AIProviderConfig{
		Provider: NormalizeAIProvider(limitBytes(input.Provider, maxAIProviderBytes)),
		BaseURL:  strings.TrimSpace(limitBytes(input.BaseURL, maxAIBaseURLBytes)),
		APIKey:   strings.TrimSpace(limitBytes(input.APIKey, maxAIAPIKeyBytes)),
		Model:    strings.TrimSpace(limitBytes(input.Model, maxAIModelBytes)),
	}
	if cfg.Provider == AIProviderDisabled {
		return AIProviderConfig{Provider: AIProviderDisabled}
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultAIBaseURL(cfg.Provider)
	}
	if normalized, err := normalizeHTTPURL(cfg.BaseURL); err == nil {
		cfg.BaseURL = normalized
	}
	return cfg
}

func DefaultAIBaseURL(provider string) string {
	switch NormalizeAIProvider(provider) {
	default:
		return "https://api.openai.com/v1"
	}
}

func (c AIProviderConfig) Configured() bool {
	cfg := NormalizeAIProviderConfigLenient(c)
	return cfg.Provider != AIProviderDisabled && cfg.APIKey != "" && cfg.Model != ""
}

func normalizeHTTPURL(value string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed == nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", ErrInvalidAIProviderConfig
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", ErrInvalidAIProviderConfig
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func limitBytes(value string, max int) string {
	if max <= 0 || len(value) <= max {
		return value
	}
	last := 0
	for i := range value {
		if i == max {
			return value[:i]
		}
		if i > max {
			return value[:last]
		}
		last = i
	}
	return value[:last]
}
