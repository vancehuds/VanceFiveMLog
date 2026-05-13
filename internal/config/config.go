package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/vancehuds/VanceFiveMLog/internal/timezone"
)

type Config struct {
	Addr                 string
	DatabaseURL          string
	Env                  string
	SessionSecret        string
	SessionCookieSecure  bool
	InitialAdminUsername string
	InitialAdminPassword string
	RetentionDays        int
	RetentionSweep       time.Duration
	TimeZone             string
	GeoMapImageURL       string
	GeoMapMinX           float64
	GeoMapMaxX           float64
	GeoMapMinY           float64
	GeoMapMaxY           float64
	AIJSONBaseURL        string
	AIJSONAPIKey         string
	AIJSONModel          string
	TurnstileSiteKey     string
	TurnstileSecretKey   string
}

func Load() (Config, error) {
	cfg := Config{
		Addr:                listenAddr(),
		DatabaseURL:         databaseURL(),
		Env:                 strings.ToLower(strings.TrimSpace(getenv("APP_ENV", "development"))),
		SessionSecret:       getenv("SESSION_SECRET", "dev-secret-change-me"),
		SessionCookieSecure: false,
		RetentionDays:       180,
		RetentionSweep:      24 * time.Hour,
		TimeZone:            timezone.Default,
		GeoMapImageURL:      strings.TrimSpace(getenv("GEO_MAP_IMAGE_URL", "/static/maps/los-santos.jpg")),
		GeoMapMinX:          -5610,
		GeoMapMaxX:          6730,
		GeoMapMinY:          -3850,
		GeoMapMaxY:          8350,
		AIJSONBaseURL:       strings.TrimRight(strings.TrimSpace(getenv("AI_JSON_BASE_URL", "https://api.openai.com/v1")), "/"),
		AIJSONAPIKey:        strings.TrimSpace(os.Getenv("AI_JSON_API_KEY")),
		AIJSONModel:         strings.TrimSpace(os.Getenv("AI_JSON_MODEL")),
		TurnstileSiteKey:    strings.TrimSpace(os.Getenv("TURNSTILE_SITE_KEY")),
		TurnstileSecretKey:  strings.TrimSpace(os.Getenv("TURNSTILE_SECRET_KEY")),
	}
	cfg.SessionCookieSecure = cfg.Env == "production"

	cfg.InitialAdminUsername = strings.TrimSpace(os.Getenv("INITIAL_ADMIN_USERNAME"))
	cfg.InitialAdminPassword = os.Getenv("INITIAL_ADMIN_PASSWORD")

	if raw := strings.TrimSpace(os.Getenv("APP_TIME_ZONE")); raw != "" {
		zone, err := timezone.Normalize(raw)
		if err != nil {
			return Config{}, fmt.Errorf("APP_TIME_ZONE must be a valid IANA time zone")
		}
		cfg.TimeZone = zone
	}

	if raw := strings.TrimSpace(os.Getenv("SESSION_COOKIE_SECURE")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("SESSION_COOKIE_SECURE must be a boolean")
		}
		cfg.SessionCookieSecure = value
	}

	if raw := strings.TrimSpace(os.Getenv("RETENTION_DAYS")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			return Config{}, fmt.Errorf("RETENTION_DAYS must be a positive integer")
		}
		cfg.RetentionDays = n
	}
	for key, target := range map[string]*float64{
		"GEO_MAP_MIN_X": &cfg.GeoMapMinX,
		"GEO_MAP_MAX_X": &cfg.GeoMapMaxX,
		"GEO_MAP_MIN_Y": &cfg.GeoMapMinY,
		"GEO_MAP_MAX_Y": &cfg.GeoMapMaxY,
	} {
		if raw := strings.TrimSpace(os.Getenv(key)); raw != "" {
			n, err := strconv.ParseFloat(raw, 64)
			if err != nil {
				return Config{}, fmt.Errorf("%s must be a number", key)
			}
			*target = n
		}
	}

	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL or a compatible PostgreSQL connection variable is required")
	}
	if len(cfg.SessionSecret) < 16 {
		return Config{}, errors.New("SESSION_SECRET must be at least 16 characters")
	}
	if cfg.Env == "production" {
		if cfg.SessionSecret == "dev-secret-change-me" || len(cfg.SessionSecret) < 32 {
			return Config{}, errors.New("SESSION_SECRET must be a unique value of at least 32 characters in production")
		}
	}
	if (cfg.InitialAdminUsername == "") != (cfg.InitialAdminPassword == "") {
		return Config{}, errors.New("INITIAL_ADMIN_USERNAME and INITIAL_ADMIN_PASSWORD must be set together")
	}
	if (cfg.TurnstileSiteKey == "") != (cfg.TurnstileSecretKey == "") {
		return Config{}, errors.New("TURNSTILE_SITE_KEY and TURNSTILE_SECRET_KEY must be set together")
	}
	if cfg.GeoMapMinX >= cfg.GeoMapMaxX {
		return Config{}, errors.New("GEO_MAP_MIN_X must be less than GEO_MAP_MAX_X")
	}
	if cfg.GeoMapMinY >= cfg.GeoMapMaxY {
		return Config{}, errors.New("GEO_MAP_MIN_Y must be less than GEO_MAP_MAX_Y")
	}

	return cfg, nil
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func databaseURL() string {
	for _, key := range []string{"DATABASE_URL", "POSTGRES_CONNECTION_STRING", "POSTGRES_URI"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func listenAddr() string {
	if addr := strings.TrimSpace(os.Getenv("APP_ADDR")); addr != "" {
		return addr
	}
	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		return ":8080"
	}
	if strings.HasPrefix(port, ":") || strings.Contains(port, ":") {
		return port
	}
	return ":" + port
}
