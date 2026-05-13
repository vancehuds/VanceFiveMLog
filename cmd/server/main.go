package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/vancehuds/VanceFiveMLog/internal/aijson"
	"github.com/vancehuds/VanceFiveMLog/internal/auth"
	"github.com/vancehuds/VanceFiveMLog/internal/config"
	"github.com/vancehuds/VanceFiveMLog/internal/db"
	"github.com/vancehuds/VanceFiveMLog/internal/httpx"
	"github.com/vancehuds/VanceFiveMLog/internal/logs"
	"github.com/vancehuds/VanceFiveMLog/internal/serverkeys"
	"github.com/vancehuds/VanceFiveMLog/internal/settings"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("load config", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("open database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		logger.Error("migrate database", "error", err)
		os.Exit(1)
	}

	authStore := auth.NewStore(pool)
	if err := authStore.EnsureInitial(ctx, cfg.InitialAdminUsername, cfg.InitialAdminPassword); err != nil {
		logger.Error("ensure initial admin", "error", err)
		os.Exit(1)
	}

	logStore := logs.NewStore(pool)
	settingsStore := settings.NewStore(pool)
	aiJSONStore := aijson.NewStore(pool)
	retentionCtx, cancelRetention := context.WithCancel(ctx)
	var retentionWG sync.WaitGroup
	retentionWG.Add(1)
	go func() {
		defer retentionWG.Done()
		retentionLoop(retentionCtx, logger, logStore, settingsStore, cfg.RetentionDays, cfg.RetentionSweep)
	}()
	defer func() {
		cancelRetention()
		retentionWG.Wait()
	}()

	app := httpx.NewServer(httpx.Deps{
		AuthStore:   authStore,
		Sessions:    auth.NewSessionManager(cfg.SessionSecret).WithSecureCookie(cfg.SessionCookieSecure),
		ServerStore: serverkeys.NewStore(pool),
		LogStore:    logStore,
		AIJSONStore: aiJSONStore,
		AIJSONConfig: settings.AIProviderConfig{
			Provider: settings.AIProviderOpenAI,
			BaseURL:  cfg.AIJSONBaseURL,
			APIKey:   cfg.AIJSONAPIKey,
			Model:    cfg.AIJSONModel,
		},
		Settings:     settingsStore,
		Hub:          logs.NewHub(),
		TemplatesDir: "web/templates",
		StaticDir:    "web/static",
		Retention:    cfg.RetentionDays,
		TimeZone:     cfg.TimeZone,
		GeoMap: httpx.GeoMapConfig{
			ImageURL: cfg.GeoMapImageURL,
			MinX:     cfg.GeoMapMinX,
			MaxX:     cfg.GeoMapMaxX,
			MinY:     cfg.GeoMapMinY,
			MaxY:     cfg.GeoMapMaxY,
		},
		Turnstile: httpx.TurnstileConfig{
			SiteKey:   cfg.TurnstileSiteKey,
			SecretKey: cfg.TurnstileSecretKey,
		},
		Logger: logger,
	})

	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           app.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		logger.Info("server listening", "addr", cfg.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server stopped", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown", "error", err)
	}
}

func retentionLoop(ctx context.Context, logger *slog.Logger, store *logs.Store, settingsStore *settings.Store, fallbackDays int, interval time.Duration) {
	run := func() {
		if ctx.Err() != nil {
			return
		}
		days := settingsStore.RetentionDays(ctx, fallbackDays)
		if ctx.Err() != nil {
			return
		}
		deleted, err := store.DeleteOlderThan(ctx, days)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			logger.Error("retention cleanup", "error", err)
			return
		}
		if deleted > 0 {
			logger.Info("retention cleanup", "deleted", deleted, "days", days)
		}
	}

	run()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			run()
		case <-ctx.Done():
			return
		}
	}
}
