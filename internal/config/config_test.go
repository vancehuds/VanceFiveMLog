package config

import "testing"

func TestLoadConfig(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost/db")
	t.Setenv("SESSION_SECRET", "0123456789abcdef")
	t.Setenv("RETENTION_DAYS", "90")
	t.Setenv("GEO_MAP_IMAGE_URL", "/static/custom-map.jpg")
	t.Setenv("GEO_MAP_MIN_X", "-100")
	t.Setenv("GEO_MAP_MAX_X", "200")
	t.Setenv("GEO_MAP_MIN_Y", "-300")
	t.Setenv("GEO_MAP_MAX_Y", "400")
	t.Setenv("AI_JSON_BASE_URL", "https://example.test/v1/")
	t.Setenv("AI_JSON_API_KEY", "sk-test")
	t.Setenv("AI_JSON_MODEL", "test-model")
	t.Setenv("TURNSTILE_SITE_KEY", "0xsite")
	t.Setenv("TURNSTILE_SECRET_KEY", "0xsecret")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RetentionDays != 90 {
		t.Fatalf("retention = %d", cfg.RetentionDays)
	}
	if cfg.TimeZone != "Asia/Shanghai" {
		t.Fatalf("time zone = %s", cfg.TimeZone)
	}
	if cfg.Addr != ":8080" {
		t.Fatalf("addr = %s", cfg.Addr)
	}
	if cfg.GeoMapImageURL != "/static/custom-map.jpg" {
		t.Fatalf("geo image = %s", cfg.GeoMapImageURL)
	}
	if cfg.GeoMapMinX != -100 || cfg.GeoMapMaxX != 200 || cfg.GeoMapMinY != -300 || cfg.GeoMapMaxY != 400 {
		t.Fatalf("geo bounds = %+v", cfg)
	}
	if cfg.AIJSONBaseURL != "https://example.test/v1" || cfg.AIJSONAPIKey != "sk-test" || cfg.AIJSONModel != "test-model" {
		t.Fatalf("ai json config = %+v", cfg)
	}
	if cfg.TurnstileSiteKey != "0xsite" || cfg.TurnstileSecretKey != "0xsecret" {
		t.Fatalf("turnstile config = %+v", cfg)
	}
}

func TestLoadConfigTimeZoneOverride(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost/db")
	t.Setenv("SESSION_SECRET", "0123456789abcdef")
	t.Setenv("APP_TIME_ZONE", "America/New_York")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TimeZone != "America/New_York" {
		t.Fatalf("time zone = %s", cfg.TimeZone)
	}
}

func TestLoadConfigRejectsInvalidTimeZone(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost/db")
	t.Setenv("SESSION_SECRET", "0123456789abcdef")
	t.Setenv("APP_TIME_ZONE", "not-a-zone")

	if _, err := Load(); err == nil {
		t.Fatal("expected invalid time zone to fail")
	}
}

func TestLoadConfigUsesPort(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost/db")
	t.Setenv("SESSION_SECRET", "0123456789abcdef")
	t.Setenv("PORT", "3000")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != ":3000" {
		t.Fatalf("addr = %s", cfg.Addr)
	}
}

func TestLoadConfigAppAddrOverridesPort(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost/db")
	t.Setenv("SESSION_SECRET", "0123456789abcdef")
	t.Setenv("APP_ADDR", ":9090")
	t.Setenv("PORT", "3000")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != ":9090" {
		t.Fatalf("addr = %s", cfg.Addr)
	}
}

func TestLoadConfigUsesZeaburPostgresURL(t *testing.T) {
	t.Setenv("POSTGRES_CONNECTION_STRING", "postgres://user:pass@localhost/db")
	t.Setenv("SESSION_SECRET", "0123456789abcdef")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseURL != "postgres://user:pass@localhost/db" {
		t.Fatalf("database url = %s", cfg.DatabaseURL)
	}
}

func TestProductionRequiresStrongSessionSecret(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost/db")
	t.Setenv("APP_ENV", "production")
	t.Setenv("SESSION_SECRET", "0123456789abcdef")

	if _, err := Load(); err == nil {
		t.Fatal("expected weak production session secret to fail")
	}
}

func TestProductionEnablesSecureCookies(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost/db")
	t.Setenv("APP_ENV", "production")
	t.Setenv("SESSION_SECRET", "0123456789abcdef0123456789abcdef")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.SessionCookieSecure {
		t.Fatal("expected secure cookies in production")
	}
}

func TestSessionCookieSecureOverride(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost/db")
	t.Setenv("SESSION_SECRET", "0123456789abcdef")
	t.Setenv("SESSION_COOKIE_SECURE", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.SessionCookieSecure {
		t.Fatal("expected secure cookie override")
	}
}

func TestLoadConfigRejectsInvalidGeoBounds(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost/db")
	t.Setenv("SESSION_SECRET", "0123456789abcdef")
	t.Setenv("GEO_MAP_MIN_X", "10")
	t.Setenv("GEO_MAP_MAX_X", "10")

	if _, err := Load(); err == nil {
		t.Fatal("expected invalid geo bounds to fail")
	}
}

func TestLoadConfigRejectsPartialTurnstileConfig(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost/db")
	t.Setenv("SESSION_SECRET", "0123456789abcdef")
	t.Setenv("TURNSTILE_SITE_KEY", "0xsite")

	if _, err := Load(); err == nil {
		t.Fatal("expected partial turnstile config to fail")
	}
}
