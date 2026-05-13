package settings

import "testing"

func TestNormalizeAIProviderConfigDefaultsOpenAIBaseURL(t *testing.T) {
	cfg, err := NormalizeAIProviderConfig(AIProviderConfig{
		Provider: "openai",
		APIKey:   " sk-test ",
		Model:    " gpt-test ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Provider != AIProviderOpenAI {
		t.Fatalf("provider = %s", cfg.Provider)
	}
	if cfg.BaseURL != "https://api.openai.com/v1" {
		t.Fatalf("base url = %s", cfg.BaseURL)
	}
	if cfg.APIKey != "sk-test" || cfg.Model != "gpt-test" {
		t.Fatalf("cfg = %+v", cfg)
	}
}

func TestNormalizeAIProviderConfigAllowsIncompleteCredentials(t *testing.T) {
	cfg, err := NormalizeAIProviderConfig(AIProviderConfig{
		Provider: "openai",
		BaseURL:  "https://example.test/v1/",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Configured() {
		t.Fatalf("expected incomplete config to be not ready: %+v", cfg)
	}
	if cfg.BaseURL != "https://example.test/v1" {
		t.Fatalf("base url = %s", cfg.BaseURL)
	}
}

func TestNormalizeAIProviderConfigRejectsInvalidBaseURL(t *testing.T) {
	if _, err := NormalizeAIProviderConfig(AIProviderConfig{
		Provider: "custom",
		BaseURL:  "ftp://example.test/v1",
		Model:    "model",
	}); err == nil {
		t.Fatal("expected invalid base url")
	}
}

func TestNormalizeAIProviderConfigDisabledClearsRuntimeFields(t *testing.T) {
	cfg, err := NormalizeAIProviderConfig(AIProviderConfig{
		Provider: "disabled",
		BaseURL:  "https://example.test/v1",
		APIKey:   "sk-test",
		Model:    "model",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Provider != AIProviderDisabled || cfg.BaseURL != "" || cfg.APIKey != "" || cfg.Model != "" {
		t.Fatalf("cfg = %+v", cfg)
	}
}
