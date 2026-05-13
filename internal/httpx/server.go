package httpx

import (
	"context"
	"crypto/subtle"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/vancehuds/VanceFiveMLog/internal/aijson"
	"github.com/vancehuds/VanceFiveMLog/internal/auth"
	"github.com/vancehuds/VanceFiveMLog/internal/i18n"
	"github.com/vancehuds/VanceFiveMLog/internal/logs"
	"github.com/vancehuds/VanceFiveMLog/internal/serverkeys"
	"github.com/vancehuds/VanceFiveMLog/internal/settings"
	"github.com/vancehuds/VanceFiveMLog/internal/timezone"
)

type Server struct {
	authStore    *auth.Store
	sessions     *auth.SessionManager
	serverStore  *serverkeys.Store
	logStore     *logs.Store
	aiJSONStore  *aijson.Store
	aiJSONConfig settings.AIProviderConfig
	settings     *settings.Store
	hub          *logs.Hub
	loginLimiter *loginRateLimiter
	turnstile    *turnstileVerifier
	templatesDir string
	staticDir    string
	retention    int
	timeZone     string
	geoMap       GeoMapConfig
	log          *slog.Logger
}

type GeoMapConfig struct {
	ImageURL string  `json:"image_url"`
	MinX     float64 `json:"min_x"`
	MaxX     float64 `json:"max_x"`
	MinY     float64 `json:"min_y"`
	MaxY     float64 `json:"max_y"`
}

type Deps struct {
	AuthStore    *auth.Store
	Sessions     *auth.SessionManager
	ServerStore  *serverkeys.Store
	LogStore     *logs.Store
	AIJSONStore  *aijson.Store
	AIJSONConfig settings.AIProviderConfig
	Settings     *settings.Store
	Hub          *logs.Hub
	TemplatesDir string
	StaticDir    string
	Retention    int
	TimeZone     string
	GeoMap       GeoMapConfig
	Turnstile    TurnstileConfig
	Logger       *slog.Logger
}

func NewServer(deps Deps) *Server {
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	zone, err := timezone.Normalize(deps.TimeZone)
	if err != nil {
		zone = timezone.Default
	}
	return &Server{
		authStore:    deps.AuthStore,
		sessions:     deps.Sessions,
		serverStore:  deps.ServerStore,
		logStore:     deps.LogStore,
		aiJSONStore:  deps.AIJSONStore,
		aiJSONConfig: settings.NormalizeAIProviderConfigLenient(deps.AIJSONConfig),
		settings:     deps.Settings,
		hub:          deps.Hub,
		loginLimiter: newLoginRateLimiter(),
		turnstile:    newTurnstileVerifier(deps.Turnstile, nil),
		templatesDir: deps.TemplatesDir,
		staticDir:    deps.StaticDir,
		retention:    deps.Retention,
		timeZone:     zone,
		geoMap:       normalizeGeoMapConfig(deps.GeoMap),
		log:          deps.Logger,
	}
}

func normalizeGeoMapConfig(cfg GeoMapConfig) GeoMapConfig {
	if cfg.ImageURL == "" {
		cfg.ImageURL = "/static/maps/los-santos.jpg"
	}
	if cfg.MinX == 0 && cfg.MaxX == 0 {
		cfg.MinX = -5610
		cfg.MaxX = 6730
	}
	if cfg.MinY == 0 && cfg.MaxY == 0 {
		cfg.MinY = -3850
		cfg.MaxY = 8350
	}
	if cfg.MinX >= cfg.MaxX {
		cfg.MinX = -5610
		cfg.MaxX = 6730
	}
	if cfg.MinY >= cfg.MaxY {
		cfg.MinY = -3850
		cfg.MaxY = 8350
	}
	return cfg
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir(s.staticDir))))

	mux.HandleFunc("GET /login", s.loginPage)
	mux.HandleFunc("POST /login", s.login)
	mux.HandleFunc("POST /logout", s.requireAuth(s.requireCSRF(s.logout)))
	mux.HandleFunc("POST /language", s.setLanguage)

	mux.HandleFunc("GET /", s.requireAuth(s.dashboard))
	mux.HandleFunc("GET /logs", s.requireAuth(s.logsPage))
	mux.HandleFunc("GET /logs/{id}", s.requireAuth(s.logDetail))
	mux.HandleFunc("GET /logs/export.csv", s.requireAuth(s.exportLogsCSV))
	mux.HandleFunc("GET /logs/stream", s.requireAuth(s.streamLogs))
	mux.HandleFunc("POST /logs/{id}/review", s.requireAuth(s.requireRole(canManageLogs)(s.requireCSRF(s.reviewLog))))
	mux.HandleFunc("POST /logs/{id}/archive", s.requireAuth(s.requireRole(canManageLogs)(s.requireCSRF(s.archiveLog))))
	mux.HandleFunc("POST /logs/bulk/archive", s.requireAuth(s.requireRole(canManageLogs)(s.requireCSRF(s.bulkArchiveLogs))))
	mux.HandleFunc("GET /players/{id}", s.requireAuth(s.playerTimeline))
	mux.HandleFunc("GET /accounts", s.requireAuth(s.accounts))
	mux.HandleFunc("GET /geo", s.requireAuth(s.geo))
	mux.HandleFunc("GET /ai-json", s.requireAuth(s.requireRole(canManageAIJSON)(s.aiJSONPage)))
	mux.HandleFunc("POST /ai-json", s.requireAuth(s.requireRole(canManageAIJSON)(s.requireCSRF(s.aiJSONPage))))
	mux.HandleFunc("POST /ai-json/suggest", s.requireAuth(s.requireRole(canManageAIJSON)(s.requireCSRF(s.suggestAIJSONMethod))))
	mux.HandleFunc("POST /ai-json/methods", s.requireAuth(s.requireRole(canManageAIJSON)(s.requireCSRF(s.createAIJSONMethod))))
	mux.HandleFunc("POST /ai-json/methods/{id}", s.requireAuth(s.requireRole(canManageAIJSON)(s.requireCSRF(s.updateAIJSONMethod))))
	mux.HandleFunc("POST /ai-json/methods/{id}/toggle", s.requireAuth(s.requireRole(canManageAIJSON)(s.requireCSRF(s.toggleAIJSONMethod))))
	mux.HandleFunc("GET /settings", s.requireAuth(s.requireRole(canManageServers)(s.settingsPage)))
	mux.HandleFunc("POST /settings", s.requireAuth(s.requireRole(canManageSettings)(s.requireCSRF(s.updateSettings))))
	mux.HandleFunc("POST /servers", s.requireAuth(s.requireRole(canManageServers)(s.requireCSRF(s.createServer))))
	mux.HandleFunc("POST /servers/{id}/rotate-key", s.requireAuth(s.requireRole(canManageServers)(s.requireCSRF(s.rotateKey))))
	mux.HandleFunc("POST /servers/{id}/toggle", s.requireAuth(s.requireRole(canManageServers)(s.requireCSRF(s.toggleServer))))
	mux.HandleFunc("POST /servers/{id}/test-event", s.requireAuth(s.requireRole(canManageServers)(s.requireCSRF(s.createTestEvent))))
	mux.HandleFunc("POST /admins", s.requireAuth(s.requireRole(canManageAdmins)(s.requireCSRF(s.createAdmin))))
	mux.HandleFunc("POST /admins/{id}/toggle", s.requireAuth(s.requireRole(canManageAdmins)(s.requireCSRF(s.toggleAdmin))))
	mux.HandleFunc("POST /admins/{id}/reset-password", s.requireAuth(s.requireRole(canManageAdmins)(s.requireCSRF(s.resetAdminPassword))))

	mux.HandleFunc("POST /api/v1/events", requireJSON(s.ingestEvents))
	mux.HandleFunc("POST /api/v1/heartbeat", requireJSON(s.heartbeat))
	mux.HandleFunc("GET /api/v1/logs", s.requireAuth(s.logsJSON))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	return securityHeaders(mux)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		scriptSrc := "script-src 'self'"
		frameSrc := "frame-src 'none'"
		connectSrc := "connect-src 'self'"
		if r.URL.Path == "/login" {
			scriptSrc = "script-src 'self' https://challenges.cloudflare.com"
			frameSrc = "frame-src https://challenges.cloudflare.com"
			connectSrc = "connect-src 'self' https://challenges.cloudflare.com"
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		w.Header().Set("Content-Security-Policy", strings.Join([]string{
			"default-src 'self'",
			scriptSrc,
			"style-src 'self' 'unsafe-inline'",
			"img-src 'self' data:",
			"font-src 'self'",
			connectSrc,
			frameSrc,
			"object-src 'none'",
			"base-uri 'self'",
			"form-action 'self'",
			"frame-ancestors 'none'",
		}, "; "))
		if isHTTPS(r) {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		if !strings.HasPrefix(r.URL.Path, "/static/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

func isHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func requireJSON(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		contentType := r.Header.Get("Content-Type")
		if contentType == "" {
			http.Error(w, "content-type must be application/json", http.StatusUnsupportedMediaType)
			return
		}
		mediaType := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
		if mediaType != "application/json" {
			http.Error(w, "content-type must be application/json", http.StatusUnsupportedMediaType)
			return
		}
		next(w, r)
	}
}

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session, err := s.sessions.Read(r)
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		admin, err := s.authStore.Get(r.Context(), session.AdminID)
		if err != nil {
			s.sessions.Clear(w)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		ctx := context.WithValue(r.Context(), sessionContextKey{}, session)
		ctx = context.WithValue(ctx, adminContextKey{}, admin)
		next(w, r.WithContext(ctx))
	}
}

type roleCheck func(auth.Admin) bool

func canManageServers(admin auth.Admin) bool {
	return admin.CanManageServers()
}

func canManageAdmins(admin auth.Admin) bool {
	return admin.CanManageAdmins()
}

func canManageSettings(admin auth.Admin) bool {
	return admin.CanManageSettings()
}

func canManageLogs(admin auth.Admin) bool {
	return admin.CanManageLogs()
}

func canManageAIJSON(admin auth.Admin) bool {
	return admin.IsOwner()
}

func (s *Server) requireRole(check roleCheck) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if !check(adminFromContext(r.Context())) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next(w, r)
		}
	}
}

type adminContextKey struct{}
type sessionContextKey struct{}

func adminFromContext(ctx context.Context) auth.Admin {
	admin, _ := ctx.Value(adminContextKey{}).(auth.Admin)
	return admin
}

func sessionFromContext(ctx context.Context) auth.Session {
	session, _ := ctx.Value(sessionContextKey{}).(auth.Session)
	return session
}

func (s *Server) requireCSRF(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := parseLimitedForm(w, r); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		if !s.sessions.VerifyCSRFToken(sessionFromContext(r.Context()), r.FormValue("csrf_token")) {
			http.Error(w, "invalid csrf token", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

const maxFormBytes = 64 << 10

const (
	maxQueryTextBytes       = 512
	maxMetadataQueryBytes   = 4096
	maxQueryLimit           = 5000
	maxQueryOffset          = 100000
	defaultLogPageLimit     = 100
	maxLogPageLimit         = 500
	defaultGeoPageLimit     = 200
	maxGeoPageLimit         = 500
	defaultPlayerPageLimit  = 100
	maxPlayerPageLimit      = 200
	defaultAccountPageLimit = 50
	maxAccountPageLimit     = 100
)

func parseLimitedForm(w http.ResponseWriter, r *http.Request) error {
	if r.Form != nil {
		return nil
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	return r.ParseForm()
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, name string, data any) {
	lang := i18n.FromRequest(r)
	tmpl, err := s.parseTemplatesIn(s.currentLocation(r.Context()), lang)
	if err != nil {
		s.log.Error("parse templates", "error", err)
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		s.log.Error("render template", "name", name, "error", err)
		http.Error(w, "render error", http.StatusInternalServerError)
	}
}

func (s *Server) parseTemplates() (*template.Template, error) {
	return s.parseTemplatesIn(timezone.Load(s.timeZone), i18n.DefaultLanguage)
}

func (s *Server) parseTemplatesIn(loc *time.Location, lang string) (*template.Template, error) {
	if loc == nil {
		loc = timezone.Load(s.timeZone)
	}
	lang = i18n.Normalize(lang)
	funcs := template.FuncMap{
		"t": func(key string) string {
			return i18n.T(lang, key)
		},
		"i18nJSON": func() template.JS {
			return template.JS(i18n.ClientCatalogJSON(lang))
		},
		"htmlLang": func(code string) string {
			return i18n.LanguageInfo(code).HTML
		},
		"pageLangURL": pageLangURL,
		"formatPageOf": func(current, total int) string {
			text := i18n.T(lang, "pagination.page_of")
			text = strings.ReplaceAll(text, "{current}", strconv.Itoa(current))
			text = strings.ReplaceAll(text, "{total}", strconv.Itoa(total))
			return text
		},
		"aiJSONMethods": func(methods []aijson.Method) string {
			if methods == nil {
				methods = []aijson.Method{}
			}
			out, err := json.Marshal(methods)
			if err != nil {
				return "[]"
			}
			return string(out)
		},
		"aiJSONMethodData": func(method aijson.Method) string {
			out, err := json.Marshal(map[string]any{
				"id":          method.ID,
				"name":        method.Name,
				"description": method.Description,
				"source":      method.Source,
				"event_type":  method.EventType,
				"resource":    method.Resource,
				"prompt":      method.Prompt,
				"spec":        json.RawMessage(method.Spec),
			})
			if err != nil {
				return "{}"
			}
			return string(out)
		},
		"formatTime": func(t time.Time) string {
			if t.IsZero() {
				return ""
			}
			return t.In(loc).Format("2006-01-02 15:04:05")
		},
		"formatClock": func(t time.Time) string {
			if t.IsZero() {
				return ""
			}
			return t.In(loc).Format("15:04:05")
		},
		"shortID": func(v string) string {
			if len(v) <= 18 {
				return v
			}
			return v[:10] + "..." + v[len(v)-5:]
		},
		"jsonPretty": func(raw json.RawMessage) string {
			if len(raw) == 0 {
				return "{}"
			}
			var v any
			if err := json.Unmarshal(raw, &v); err != nil {
				return string(raw)
			}
			out, err := json.MarshalIndent(v, "", "  ")
			if err != nil {
				return string(raw)
			}
			return string(out)
		},
		"intPtr":         intPtr,
		"metaString":     metaString,
		"eventActor":     eventActor,
		"eventSummary":   eventSummary,
		"eventMetaChips": eventMetaChips,
		"eventJSON": func(e logs.Event) string {
			out, err := json.Marshal(e)
			if err != nil {
				return "{}"
			}
			return string(out)
		},
		"accountJSON": func(link logs.AccountLink) string {
			out, err := json.Marshal(map[string]any{
				"license":    link.License,
				"names":      link.Names,
				"discords":   link.Discords,
				"steams":     link.Steams,
				"citizenids": link.CitizenIDs,
				"events":     link.Events,
				"last_seen":  link.LastSeen,
				"identifier": accountIdentifier(link),
			})
			if err != nil {
				return "{}"
			}
			return string(out)
		},
		"coords": func(e logs.Event) string {
			if e.CoordsX == nil || e.CoordsY == nil || e.CoordsZ == nil {
				return ""
			}
			return fmt.Sprintf("%.1f, %.1f, %.1f", *e.CoordsX, *e.CoordsY, *e.CoordsZ)
		},
		"geoEventJSON": func(e logs.Event) string {
			if e.CoordsX == nil || e.CoordsY == nil {
				return "{}"
			}
			out, err := json.Marshal(map[string]any{
				"id":          e.ID,
				"event_type":  e.EventType,
				"severity":    e.Severity,
				"message":     e.Message,
				"coords_x":    *e.CoordsX,
				"coords_y":    *e.CoordsY,
				"coords_z":    e.CoordsZ,
				"occurred_at": e.OccurredAt,
				"server_name": e.ServerName,
				"player":      eventActor(e),
			})
			if err != nil {
				return "{}"
			}
			return string(out)
		},
		"geoMapJSON": func(cfg GeoMapConfig) string {
			out, err := json.Marshal(normalizeGeoMapConfig(cfg))
			if err != nil {
				return "{}"
			}
			return string(out)
		},
		"playerIdentifier": playerIdentifier,
		"accountIdentifier": func(link logs.AccountLink) string {
			return accountIdentifier(link)
		},
		"urlPath": func(v string) template.URL {
			return template.URL(url.PathEscape(v))
		},
		"join":      strings.Join,
		"hasSuffix": strings.HasSuffix,
		"lenMany": func(values []string) bool {
			return len(values) > 1
		},
		"reviewStatusLabel": func(status string) string {
			switch logs.NormalizeReviewStatus(status) {
			case logs.ReviewStatusSuspicious:
				return i18n.T(lang, "review.status.suspicious")
			case logs.ReviewStatusViolation:
				return i18n.T(lang, "review.status.violation")
			default:
				return i18n.T(lang, "review.status.normal")
			}
		},
		"reviewStatusClass": func(status string) string {
			switch logs.NormalizeReviewStatus(status) {
			case logs.ReviewStatusSuspicious:
				return "warning"
			case logs.ReviewStatusViolation:
				return "error"
			default:
				return "success"
			}
		},
		"percent": func(value, max int64) template.CSS {
			if max <= 0 || value <= 0 {
				return template.CSS("0%")
			}
			pct := (float64(value) / float64(max)) * 100
			if pct < 3 {
				pct = 3
			}
			if pct > 100 {
				pct = 100
			}
			return template.CSS(fmt.Sprintf("%.2f%%", pct))
		},
		"serverHealth": func(server serverkeys.Server) string {
			if !server.Active {
				return "DISABLED"
			}
			if server.LastSeenAt == nil {
				return "NEVER SEEN"
			}
			if time.Since(*server.LastSeenAt) <= 2*time.Minute {
				return "ONLINE"
			}
			if time.Since(*server.LastSeenAt) <= 15*time.Minute {
				return "STALE"
			}
			return "OFFLINE"
		},
		"serverHealthClass": func(server serverkeys.Server) string {
			if !server.Active {
				return "error"
			}
			if server.LastSeenAt == nil {
				return "warning"
			}
			if time.Since(*server.LastSeenAt) <= 2*time.Minute {
				return "success"
			}
			if time.Since(*server.LastSeenAt) <= 15*time.Minute {
				return "warning"
			}
			return "error"
		},
		"formatTimePtr": func(t *time.Time) string {
			if t == nil || t.IsZero() {
				return ""
			}
			return t.In(loc).Format("2006-01-02 15:04:05")
		},
		"queryWithOffset": func(query map[string]string, offset int) string {
			values := make(url.Values)
			for key, value := range query {
				if value != "" && key != "offset" && !strings.HasSuffix(key, "_local") {
					values.Add(key, value)
				}
			}
			if offset > 0 {
				values.Add("offset", strconv.Itoa(offset))
			}
			encoded := values.Encode()
			if encoded == "" {
				return "?"
			}
			return "?" + encoded
		},
		"queryWithPage": func(query map[string]string, page logs.Page, item logs.PageItem) string {
			values := make(url.Values)
			for key, value := range query {
				if value != "" && key != "offset" && !strings.HasSuffix(key, "_local") {
					values.Add(key, value)
				}
			}
			offset := item.Offset
			if item.Gap {
				offset = page.Offset
			}
			if offset > 0 {
				values.Add("offset", strconv.Itoa(offset))
			}
			encoded := values.Encode()
			if encoded == "" {
				return "?"
			}
			return "?" + encoded
		},
		"queryURL": func(path string, pairs ...string) template.URL {
			values := make(url.Values)
			for i := 0; i+1 < len(pairs); i += 2 {
				if pairs[i+1] != "" {
					values.Add(pairs[i], pairs[i+1])
				}
			}
			encoded := values.Encode()
			if encoded == "" {
				return template.URL(path)
			}
			return template.URL(path + "?" + encoded)
		},
	}
	pattern := filepath.Join(s.templatesDir, "*.html")
	return template.New("").Funcs(funcs).ParseGlob(pattern)
}

func pageLangURL(currentPath, rawQuery, lang string) string {
	if currentPath == "" {
		currentPath = "/"
	}
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		values = url.Values{}
	}
	values.Set("lang", i18n.Normalize(lang))
	encoded := values.Encode()
	if encoded == "" {
		return currentPath
	}
	return currentPath + "?" + encoded
}

func (s *Server) currentTimeZone(ctx context.Context) string {
	if s.settings == nil {
		if zone, err := timezone.Normalize(s.timeZone); err == nil {
			return zone
		}
		return timezone.Default
	}
	return s.settings.TimeZone(ctx, s.timeZone)
}

func (s *Server) currentLocation(ctx context.Context) *time.Location {
	return timezone.Load(s.currentTimeZone(ctx))
}

func (s *Server) currentAIProviderConfig(ctx context.Context) settings.AIProviderConfig {
	fallback := settings.NormalizeAIProviderConfigLenient(s.aiJSONConfig)
	if fallback.Provider == "" {
		fallback.Provider = settings.AIProviderOpenAI
	}
	if s.settings == nil {
		return fallback
	}
	return s.settings.AIProviderConfig(ctx, fallback)
}

func (s *Server) loginPage(w http.ResponseWriter, r *http.Request) {
	s.renderLogin(w, r, "")
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	identity := clientIdentity(r)
	if !s.loginLimiter.Allow(identity) {
		http.Error(w, "too many login attempts", http.StatusTooManyRequests)
		return
	}
	if err := parseLimitedForm(w, r); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	if err := s.turnstile.Verify(r.Context(), r.FormValue("cf-turnstile-response"), identity); err != nil {
		s.loginLimiter.Fail(identity)
		s.log.Warn("turnstile verification failed", "error", err)
		s.renderLogin(w, r, i18n.T(i18n.FromRequest(r), "login.turnstile_failed"))
		return
	}
	admin, err := s.authStore.Authenticate(r.Context(), r.FormValue("username"), r.FormValue("password"))
	if err != nil {
		s.loginLimiter.Fail(identity)
		s.renderLogin(w, r, i18n.T(i18n.FromRequest(r), "login.invalid_credentials"))
		return
	}
	s.loginLimiter.Success(identity)
	if err := s.sessions.Set(w, admin.ID); err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) renderLogin(w http.ResponseWriter, r *http.Request, message string) {
	lang := i18n.FromRequest(r)
	data := map[string]any{
		"Title":            i18n.T(lang, "login.title"),
		"Error":            message,
		"TurnstileSiteKey": s.turnstile.SiteKey(),
		"TurnstileEnabled": s.turnstile.Enabled(),
		"Lang":             lang,
		"HTMLLang":         i18n.LanguageInfo(lang).HTML,
		"Languages":        i18n.SupportedLanguages(),
		"CurrentPath":      r.URL.Path,
		"CurrentQuery":     r.URL.RawQuery,
	}
	s.render(w, r, "login.html", data)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	s.sessions.Clear(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) setLanguage(w http.ResponseWriter, r *http.Request) {
	if err := parseLimitedForm(w, r); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	lang := i18n.Normalize(r.FormValue("lang"))
	http.SetCookie(w, i18n.Cookie(lang, isHTTPS(r)))
	http.Redirect(w, r, returnTo(r, "/"), http.StatusSeeOther)
}

type pageData struct {
	Title         string
	Active        string
	Lang          string
	HTMLLang      string
	Languages     []i18n.Language
	CurrentPath   string
	CurrentQuery  string
	Admin         auth.Admin
	Admins        []auth.Admin
	CSRFToken     string
	Events        []logs.Event
	HourBuckets   []logs.HourBucket
	TopEvents     []logs.EventTypeCount
	MaxBucket     int64
	MaxTopEvent   int64
	Stats         logs.Stats
	Servers       []serverkeys.Server
	AIJSONMethods []aijson.Method
	AIJSONReady   bool
	AIJSONDraft   aiJSONDraft
	AIProvider    settings.AIProviderConfig
	AIProviderKey bool
	Retention     int
	TimeZone      string
	Page          logs.Page
	Query         map[string]string
	Player        string
	PlayerStats   logs.PlayerSummary
	AccountLinks  []logs.AccountLink
	GeoMap        GeoMapConfig
	Event         logs.Event
	NewAPIKey     string
	Error         string
	Notice        string
}

type aiJSONDraft struct {
	ID          int64
	Name        string
	Description string
	Source      string
	EventType   string
	Resource    string
	Prompt      string
	Spec        string
	Sample      string
}

func (s *Server) baseData(ctx context.Context, title, active string) pageData {
	servers, _ := s.serverStore.List(ctx)
	retention := s.settings.RetentionDays(ctx, s.retention)
	zone := s.settings.TimeZone(ctx, s.timeZone)
	aiProvider := s.currentAIProviderConfig(ctx)
	aiProviderForDisplay := aiProvider
	aiProviderForDisplay.APIKey = ""
	return pageData{
		Title:         title,
		Active:        active,
		Lang:          i18n.DefaultLanguage,
		HTMLLang:      i18n.LanguageInfo(i18n.DefaultLanguage).HTML,
		Languages:     i18n.SupportedLanguages(),
		Admin:         adminFromContext(ctx),
		CSRFToken:     s.sessions.CSRFToken(sessionFromContext(ctx)),
		Servers:       servers,
		AIJSONReady:   aiProvider.Configured(),
		AIProvider:    aiProviderForDisplay,
		AIProviderKey: strings.TrimSpace(aiProvider.APIKey) != "",
		Retention:     retention,
		TimeZone:      zone,
		GeoMap:        s.geoMap,
		Query:         map[string]string{},
	}
}

func (s *Server) baseDataForRequest(r *http.Request, titleKey, active string) pageData {
	lang := i18n.FromRequest(r)
	data := s.baseData(r.Context(), i18n.T(lang, titleKey), active)
	data.Lang = lang
	data.HTMLLang = i18n.LanguageInfo(lang).HTML
	data.Languages = i18n.SupportedLanguages()
	data.CurrentPath = r.URL.Path
	data.CurrentQuery = r.URL.RawQuery
	return data
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	data := s.baseDataForRequest(r, "dashboard.title", "dashboard")
	loc := s.currentLocation(r.Context())
	data.Stats, _ = s.logStore.DashboardStats(r.Context(), loc)
	data.HourBuckets, _ = s.logStore.HourlyBuckets(r.Context(), 24, loc)
	data.TopEvents, _ = s.logStore.TopEventTypes(r.Context(), 8)
	for _, bucket := range data.HourBuckets {
		if bucket.Total > data.MaxBucket {
			data.MaxBucket = bucket.Total
		}
	}
	for _, item := range data.TopEvents {
		if item.Total > data.MaxTopEvent {
			data.MaxTopEvent = item.Total
		}
	}
	data.Events, _ = s.logStore.Query(r.Context(), logs.Query{Limit: 60})
	data.AIJSONMethods = s.activeAIJSONMethods(r.Context())
	s.render(w, r, "dashboard.html", data)
}

func (s *Server) logsPage(w http.ResponseWriter, r *http.Request) {
	data := s.baseDataForRequest(r, "logs.title", "logs")
	loc := s.currentLocation(r.Context())
	query := queryFromRequest(r, loc)
	if query.Limit > maxLogPageLimit {
		query.Limit = maxLogPageLimit
	}
	total, _ := s.logStore.Count(r.Context(), query)
	data.Page = logs.NewPage(query.Limit, query.Offset, total)
	query.Limit = data.Page.Limit
	query.Offset = data.Page.Offset
	data.Events, _ = s.logStore.Query(r.Context(), query)
	data.Query = queryMap(r, loc)
	data.AIJSONMethods = s.activeAIJSONMethods(r.Context())
	applyPageQuery(data.Query, data.Page)
	s.render(w, r, "logs.html", data)
}

func (s *Server) logDetail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	event, err := s.logStore.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "query error", http.StatusInternalServerError)
		return
	}
	data := s.baseDataForRequest(r, "logs.detail_title", "logs")
	data.Event = event
	data.AIJSONMethods = s.activeAIJSONMethods(r.Context())
	s.render(w, r, "log_detail.html", data)
}

func (s *Server) exportLogsCSV(w http.ResponseWriter, r *http.Request) {
	loc := s.currentLocation(r.Context())
	query := queryFromRequest(r, loc)
	if r.URL.Query().Get("limit") == "" || query.Limit <= 0 || query.Limit > 5000 {
		query.Limit = 5000
	}
	events, err := s.logStore.Query(r.Context(), query)
	if err != nil {
		http.Error(w, "query error", http.StatusInternalServerError)
		return
	}

	filename := "vancefivemlog-" + time.Now().In(loc).Format("20060102-150405") + ".csv"
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)

	writer := csv.NewWriter(w)
	_ = writer.Write([]string{
		"id", "server", "event_type", "severity", "occurred_at", "player_source",
		"player_name", "license", "discord", "steam", "citizenid", "resource",
		"message", "coords_x", "coords_y", "coords_z", "review_status",
		"review_note", "archived_at", "metadata",
	})
	for _, event := range events {
		archivedAt := ""
		if event.Review.ArchivedAt != nil {
			archivedAt = event.Review.ArchivedAt.In(loc).Format(time.RFC3339)
		}
		_ = writer.Write([]string{
			strconv.FormatInt(event.ID, 10),
			event.ServerName,
			event.EventType,
			event.Severity,
			event.OccurredAt.In(loc).Format(time.RFC3339),
			intPtr(event.PlayerSource),
			event.PlayerName,
			event.License,
			event.Discord,
			event.Steam,
			event.CitizenID,
			event.Resource,
			event.Message,
			floatPtr(event.CoordsX),
			floatPtr(event.CoordsY),
			floatPtr(event.CoordsZ),
			event.Review.Status,
			event.Review.Note,
			archivedAt,
			string(event.Metadata),
		})
	}
	writer.Flush()
}

func (s *Server) reviewLog(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	admin := adminFromContext(r.Context())
	status := r.FormValue("status")
	note := r.FormValue("note")
	if err := s.logStore.ReviewEvent(r.Context(), id, admin.ID, status, note); err != nil {
		http.Error(w, "review error", http.StatusBadRequest)
		return
	}
	_ = s.auditLogAction(r, logs.AdminAuditEntry{
		Action:  "log.review",
		EventID: id,
		Details: map[string]any{
			"status": logs.NormalizeReviewStatus(status),
			"note":   strings.TrimSpace(note),
		},
	})
	http.Redirect(w, r, returnTo(r, "/logs"), http.StatusSeeOther)
}

func (s *Server) archiveLog(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	admin := adminFromContext(r.Context())
	if _, err := s.logStore.ArchiveEvents(r.Context(), []int64{id}, admin.ID); err != nil {
		http.Error(w, "archive error", http.StatusBadRequest)
		return
	}
	_ = s.auditLogAction(r, logs.AdminAuditEntry{
		Action:   "log.archive",
		EventID:  id,
		EventIDs: []int64{id},
	})
	http.Redirect(w, r, returnTo(r, "/logs"), http.StatusSeeOther)
}

func (s *Server) bulkArchiveLogs(w http.ResponseWriter, r *http.Request) {
	admin := adminFromContext(r.Context())
	ids := selectedEventIDs(r)
	var count int64
	var err error
	entry := logs.AdminAuditEntry{Action: "log.bulk_archive"}
	if len(ids) > 0 {
		count, err = s.logStore.ArchiveEvents(r.Context(), ids, admin.ID)
		entry.EventIDs = ids
	} else {
		query := queryFromForm(r, s.currentLocation(r.Context()))
		count, err = s.logStore.ArchiveQuery(r.Context(), query, admin.ID, maxQueryLimit)
		entry.Query = queryMapFromForm(r)
	}
	if err != nil {
		http.Error(w, "bulk archive error", http.StatusBadRequest)
		return
	}
	entry.Details = map[string]any{"affected": count}
	_ = s.auditLogAction(r, entry)
	http.Redirect(w, r, returnTo(r, "/logs"), http.StatusSeeOther)
}

func (s *Server) playerTimeline(w http.ResponseWriter, r *http.Request) {
	data := s.baseDataForRequest(r, "player.title", "players")
	player := r.PathValue("id")
	if q := strings.TrimSpace(r.URL.Query().Get("player")); q != "" {
		limit, _ := pageParams(r, defaultPlayerPageLimit, maxPlayerPageLimit)
		target := "/players/" + url.PathEscape(q)
		if limit != defaultPlayerPageLimit {
			target += "?limit=" + strconv.Itoa(limit)
		}
		http.Redirect(w, r, target, http.StatusSeeOther)
		return
	}
	if player == "all" {
		player = ""
	}
	data.Player = player
	data.PlayerStats, _ = s.logStore.PlayerSummary(r.Context(), player)
	limit, offset := pageParams(r, defaultPlayerPageLimit, maxPlayerPageLimit)
	query := logs.Query{Player: player, Limit: limit, Offset: offset}
	total, _ := s.logStore.Count(r.Context(), query)
	data.Page = logs.NewPage(query.Limit, query.Offset, total)
	query.Limit = data.Page.Limit
	query.Offset = data.Page.Offset
	data.Query = queryMap(r, s.currentLocation(r.Context()))
	applyPageQuery(data.Query, data.Page)
	data.Events, _ = s.logStore.Query(r.Context(), query)
	data.AIJSONMethods = s.activeAIJSONMethods(r.Context())
	s.render(w, r, "player.html", data)
}

func (s *Server) accounts(w http.ResponseWriter, r *http.Request) {
	data := s.baseDataForRequest(r, "account.audit.title", "accounts")
	data.Query = queryMap(r, s.currentLocation(r.Context()))
	keyword := strings.TrimSpace(r.URL.Query().Get("q"))
	limit, offset := pageParams(r, defaultAccountPageLimit, maxAccountPageLimit)
	total, _ := s.logStore.AccountLinksCount(r.Context(), keyword)
	data.Page = logs.NewPage(limit, offset, total)
	applyPageQuery(data.Query, data.Page)
	data.AccountLinks, _ = s.logStore.AccountLinksPage(r.Context(), keyword, data.Page.Limit, data.Page.Offset)
	s.render(w, r, "accounts.html", data)
}

func (s *Server) geo(w http.ResponseWriter, r *http.Request) {
	data := s.baseDataForRequest(r, "geo.title", "geo")
	loc := s.currentLocation(r.Context())
	query := queryFromRequest(r, loc)
	query.WithCoords = true
	if r.URL.Query().Get("limit") == "" || query.Limit <= 0 || query.Limit > maxGeoPageLimit {
		query.Limit = defaultGeoPageLimit
	}
	total, _ := s.logStore.Count(r.Context(), query)
	data.Page = logs.NewPage(query.Limit, query.Offset, total)
	query.Limit = data.Page.Limit
	query.Offset = data.Page.Offset
	data.Query = queryMap(r, loc)
	applyPageQuery(data.Query, data.Page)
	data.Events, _ = s.logStore.Query(r.Context(), query)
	data.AIJSONMethods = s.activeAIJSONMethods(r.Context())
	s.render(w, r, "geo.html", data)
}

func (s *Server) activeAIJSONMethods(ctx context.Context) []aijson.Method {
	if s.aiJSONStore == nil {
		return nil
	}
	methods, err := s.aiJSONStore.List(ctx, true)
	if err != nil {
		if s.log != nil {
			s.log.Warn("list active ai json methods", "error", err)
		}
		return nil
	}
	return methods
}

func (s *Server) settingsPage(w http.ResponseWriter, r *http.Request) {
	data := s.baseDataForRequest(r, "settings.title", "settings")
	data.Admins, _ = s.authStore.List(r.Context())
	s.render(w, r, "settings.html", data)
}

func (s *Server) aiJSONPage(w http.ResponseWriter, r *http.Request) {
	data := s.baseDataForRequest(r, "ai_json.title", "ai-json")
	if s.aiJSONStore != nil {
		data.AIJSONMethods, _ = s.aiJSONStore.List(r.Context(), false)
	}
	data.AIJSONDraft = aiJSONDraftFromRequest(r, i18n.FromRequest(r))
	s.render(w, r, "ai_json.html", data)
}

func (s *Server) suggestAIJSONMethod(w http.ResponseWriter, r *http.Request) {
	lang := i18n.FromRequest(r)
	data := s.baseDataForRequest(r, "ai_json.title", "ai-json")
	if s.aiJSONStore != nil {
		data.AIJSONMethods, _ = s.aiJSONStore.List(r.Context(), false)
	}
	data.AIJSONDraft = aiJSONDraftFromRequest(r, lang)
	cfg := s.currentAIProviderConfig(r.Context())
	client := aijson.NewAIClient(cfg.BaseURL, cfg.APIKey, cfg.Model)
	if !client.Configured() {
		data.Error = i18n.T(lang, "ai_json.not_configured")
		s.render(w, r, "ai_json.html", data)
		return
	}
	prompt := strings.TrimSpace(data.AIJSONDraft.Prompt)
	if prompt == "" {
		prompt = i18n.T(lang, "ai_json.prompt_default")
	}
	suggestion, err := client.SuggestMethod(r.Context(), json.RawMessage(data.AIJSONDraft.Sample), prompt)
	if err != nil {
		data.Error = i18n.T(lang, "ai_json.suggest_failed") + err.Error()
		s.render(w, r, "ai_json.html", data)
		return
	}
	data.AIJSONDraft.Name = suggestion.Name
	data.AIJSONDraft.Description = suggestion.Description
	data.AIJSONDraft.Source = suggestion.Source
	data.AIJSONDraft.EventType = firstNonBlank(data.AIJSONDraft.EventType, suggestion.EventType)
	data.AIJSONDraft.Resource = firstNonBlank(data.AIJSONDraft.Resource, suggestion.Resource)
	data.AIJSONDraft.Spec = prettyJSONString(suggestion.Spec)
	data.Notice = i18n.T(lang, "ai_json.generated_notice")
	s.render(w, r, "ai_json.html", data)
}

func (s *Server) createAIJSONMethod(w http.ResponseWriter, r *http.Request) {
	if s.aiJSONStore == nil {
		http.Error(w, "ai json store unavailable", http.StatusInternalServerError)
		return
	}
	input := aiJSONInputFromRequest(r)
	if _, err := s.aiJSONStore.Create(r.Context(), input, adminFromContext(r.Context()).ID); err != nil {
		data := s.baseDataForRequest(r, "ai_json.title", "ai-json")
		data.AIJSONDraft = aiJSONDraftFromRequest(r, i18n.FromRequest(r))
		data.AIJSONMethods, _ = s.aiJSONStore.List(r.Context(), false)
		data.Error = i18n.T(i18n.FromRequest(r), "ai_json.save_failed") + err.Error()
		s.render(w, r, "ai_json.html", data)
		return
	}
	_ = s.auditLogAction(r, logs.AdminAuditEntry{
		Action:  "ai_json.create",
		Details: map[string]any{"name": input.Name, "source": input.Source, "event_type": input.EventType, "resource": input.Resource},
	})
	http.Redirect(w, r, "/ai-json", http.StatusSeeOther)
}

func (s *Server) updateAIJSONMethod(w http.ResponseWriter, r *http.Request) {
	if s.aiJSONStore == nil {
		http.Error(w, "ai json store unavailable", http.StatusInternalServerError)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	input := aiJSONInputFromRequest(r)
	if _, err := s.aiJSONStore.Update(r.Context(), id, input); err != nil {
		data := s.baseDataForRequest(r, "ai_json.title", "ai-json")
		data.AIJSONDraft = aiJSONDraftFromRequest(r, i18n.FromRequest(r))
		data.AIJSONDraft.ID = id
		data.AIJSONMethods, _ = s.aiJSONStore.List(r.Context(), false)
		data.Error = i18n.T(i18n.FromRequest(r), "ai_json.update_failed") + err.Error()
		s.render(w, r, "ai_json.html", data)
		return
	}
	_ = s.auditLogAction(r, logs.AdminAuditEntry{
		Action:  "ai_json.update",
		Details: map[string]any{"id": id, "name": input.Name, "source": input.Source, "event_type": input.EventType, "resource": input.Resource},
	})
	http.Redirect(w, r, "/ai-json", http.StatusSeeOther)
}

func (s *Server) toggleAIJSONMethod(w http.ResponseWriter, r *http.Request) {
	if s.aiJSONStore == nil {
		http.Error(w, "ai json store unavailable", http.StatusInternalServerError)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	active, err := s.aiJSONStore.ToggleActive(r.Context(), id)
	if err != nil {
		http.Error(w, "toggle error", http.StatusBadRequest)
		return
	}
	_ = s.auditLogAction(r, logs.AdminAuditEntry{
		Action:  "ai_json.toggle",
		Details: map[string]any{"id": id, "active": active},
	})
	http.Redirect(w, r, "/ai-json", http.StatusSeeOther)
}

func (s *Server) updateSettings(w http.ResponseWriter, r *http.Request) {
	if err := parseLimitedForm(w, r); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	days, err := strconv.Atoi(r.FormValue("retention_days"))
	if err != nil || days < 1 {
		data := s.baseDataForRequest(r, "settings.title", "settings")
		data.Error = i18n.T(i18n.FromRequest(r), "settings.invalid_retention")
		data.Admins, _ = s.authStore.List(r.Context())
		s.render(w, r, "settings.html", data)
		return
	}
	zone, err := timezone.Normalize(r.FormValue("time_zone"))
	if err != nil {
		data := s.baseDataForRequest(r, "settings.title", "settings")
		data.Error = i18n.T(i18n.FromRequest(r), "settings.invalid_timezone")
		data.Admins, _ = s.authStore.List(r.Context())
		s.render(w, r, "settings.html", data)
		return
	}
	if err := s.settings.SetRetentionDays(r.Context(), days); err != nil {
		http.Error(w, "settings error", http.StatusInternalServerError)
		return
	}
	if err := s.settings.SetTimeZone(r.Context(), zone); err != nil {
		http.Error(w, "settings error", http.StatusInternalServerError)
		return
	}
	if adminFromContext(r.Context()).IsOwner() {
		currentAI := s.currentAIProviderConfig(r.Context())
		aiCfg := settings.AIProviderConfig{
			Provider: r.FormValue("ai_json_provider"),
			BaseURL:  r.FormValue("ai_json_base_url"),
			APIKey:   r.FormValue("ai_json_api_key"),
			Model:    r.FormValue("ai_json_model"),
		}
		if strings.TrimSpace(aiCfg.APIKey) == "" && settings.NormalizeAIProvider(aiCfg.Provider) != settings.AIProviderDisabled {
			aiCfg.APIKey = currentAI.APIKey
		}
		if err := s.settings.SetAIProviderConfig(r.Context(), aiCfg); err != nil {
			data := s.baseDataForRequest(r, "settings.title", "settings")
			data.Error = i18n.T(i18n.FromRequest(r), "settings.invalid_ai_provider")
			data.Admins, _ = s.authStore.List(r.Context())
			s.render(w, r, "settings.html", data)
			return
		}
	}
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func (s *Server) createServer(w http.ResponseWriter, r *http.Request) {
	if err := parseLimitedForm(w, r); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	_, key, err := s.serverStore.Create(r.Context(), r.FormValue("name"))
	data := s.baseDataForRequest(r, "settings.title", "settings")
	if err != nil {
		data.Error = i18n.T(i18n.FromRequest(r), "settings.create_server_failed") + err.Error()
	} else {
		data.NewAPIKey = key
	}
	data.Servers, _ = s.serverStore.List(r.Context())
	data.Admins, _ = s.authStore.List(r.Context())
	s.render(w, r, "settings.html", data)
}

func (s *Server) rotateKey(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	key, err := s.serverStore.RotateKey(r.Context(), id)
	data := s.baseDataForRequest(r, "settings.title", "settings")
	if err != nil {
		data.Error = i18n.T(i18n.FromRequest(r), "settings.rotate_key_failed") + err.Error()
	} else {
		data.NewAPIKey = key
	}
	data.Servers, _ = s.serverStore.List(r.Context())
	data.Admins, _ = s.authStore.List(r.Context())
	s.render(w, r, "settings.html", data)
}

func (s *Server) toggleServer(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if _, err := s.serverStore.ToggleActive(r.Context(), id); err != nil {
		data := s.baseDataForRequest(r, "settings.title", "settings")
		data.Error = i18n.T(i18n.FromRequest(r), "settings.toggle_server_failed") + err.Error()
		data.Admins, _ = s.authStore.List(r.Context())
		s.render(w, r, "settings.html", data)
		return
	}
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func (s *Server) createTestEvent(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	admin := adminFromContext(r.Context())
	now := time.Now().UTC()
	inserted, err := s.logStore.InsertEvents(r.Context(), id, []logs.IngestEvent{{
		EventType: "backend_test_event",
		Severity:  "success",
		Resource:  "vancefivemlog",
		Message:   "manual backend test event",
		Metadata: map[string]any{
			"admin":  admin.Username,
			"source": "settings",
		},
		OccurredAt: &now,
	}})
	data := s.baseDataForRequest(r, "settings.title", "settings")
	if err != nil {
		data.Error = i18n.T(i18n.FromRequest(r), "settings.test_event_failed") + err.Error()
	} else {
		servers, _ := s.serverStore.List(r.Context())
		serverName := ""
		for _, server := range servers {
			if server.ID == id {
				serverName = server.Name
				break
			}
		}
		for _, event := range inserted {
			event.ServerName = serverName
			s.hub.Publish(event)
		}
		_ = s.serverStore.MarkEvent(r.Context(), id)
		data.Notice = i18n.T(i18n.FromRequest(r), "settings.test_event_written")
	}
	data.Servers, _ = s.serverStore.List(r.Context())
	data.Admins, _ = s.authStore.List(r.Context())
	s.render(w, r, "settings.html", data)
}

func (s *Server) createAdmin(w http.ResponseWriter, r *http.Request) {
	data := s.baseDataForRequest(r, "settings.title", "settings")
	username := r.FormValue("username")
	password := r.FormValue("password")
	role := r.FormValue("role")
	if _, err := s.authStore.Create(r.Context(), username, password, role); err != nil {
		data.Error = i18n.T(i18n.FromRequest(r), "settings.create_admin_failed") + err.Error()
	} else {
		data.Notice = i18n.T(i18n.FromRequest(r), "settings.admin_created")
	}
	data.Servers, _ = s.serverStore.List(r.Context())
	data.Admins, _ = s.authStore.List(r.Context())
	s.render(w, r, "settings.html", data)
}

func (s *Server) toggleAdmin(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	current := adminFromContext(r.Context())
	if _, err := s.authStore.ToggleActive(r.Context(), id, current.ID); err != nil {
		data := s.baseDataForRequest(r, "settings.title", "settings")
		data.Error = i18n.T(i18n.FromRequest(r), "settings.toggle_admin_failed") + err.Error()
		data.Admins, _ = s.authStore.List(r.Context())
		s.render(w, r, "settings.html", data)
		return
	}
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func (s *Server) resetAdminPassword(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	data := s.baseDataForRequest(r, "settings.title", "settings")
	if err := s.authStore.ResetPassword(r.Context(), id, r.FormValue("password")); err != nil {
		data.Error = i18n.T(i18n.FromRequest(r), "settings.reset_password_failed") + err.Error()
	} else {
		data.Notice = i18n.T(i18n.FromRequest(r), "settings.admin_password_reset")
	}
	data.Servers, _ = s.serverStore.List(r.Context())
	data.Admins, _ = s.authStore.List(r.Context())
	s.render(w, r, "settings.html", data)
}

func (s *Server) ingestEvents(w http.ResponseWriter, r *http.Request) {
	server, err := s.serverStore.Authenticate(r.Context(), r.Header.Get("Authorization"))
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	events, err := decodeIngestEvents(http.MaxBytesReader(w, r.Body, 2<<20))
	if err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	inserted, err := s.logStore.InsertEvents(r.Context(), server.ID, events)
	if errors.Is(err, logs.ErrInvalidEvent) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err != nil {
		s.log.Error("ingest events", "error", err)
		http.Error(w, "ingest error", http.StatusInternalServerError)
		return
	}
	for _, event := range inserted {
		event.ServerName = server.Name
		s.hub.Publish(event)
	}
	_ = s.serverStore.MarkEvent(r.Context(), server.ID)
	writeJSON(w, map[string]any{"ok": true, "inserted": len(inserted)})
}

func decodeIngestEvents(r io.Reader) ([]logs.IngestEvent, error) {
	dec := json.NewDecoder(r)
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return nil, err
	}
	if err := dec.Decode(&json.RawMessage{}); err != io.EOF {
		return nil, fmt.Errorf("multiple json values")
	}

	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err == nil {
		if _, ok := object["events"]; ok {
			var payload struct {
				Events []logs.IngestEvent `json:"events"`
				Event  *logs.IngestEvent  `json:"event"`
			}
			if err := json.Unmarshal(raw, &payload); err != nil {
				return nil, err
			}
			if payload.Event != nil {
				payload.Events = append(payload.Events, *payload.Event)
			}
			return payload.Events, nil
		}
		if rawEvent, ok := object["event"]; ok && len(rawEvent) > 0 && rawEvent[0] == '{' {
			var payload struct {
				Events []logs.IngestEvent `json:"events"`
				Event  *logs.IngestEvent  `json:"event"`
			}
			if err := json.Unmarshal(raw, &payload); err != nil {
				return nil, err
			}
			if payload.Event != nil {
				payload.Events = append(payload.Events, *payload.Event)
			}
			return payload.Events, nil
		}
	}

	var events []logs.IngestEvent
	if err := json.Unmarshal(raw, &events); err == nil {
		return events, nil
	}

	var event logs.IngestEvent
	if err := json.Unmarshal(raw, &event); err != nil {
		return nil, err
	}
	return []logs.IngestEvent{event}, nil
}

func (s *Server) heartbeat(w http.ResponseWriter, r *http.Request) {
	server, err := s.serverStore.Authenticate(r.Context(), r.Header.Get("Authorization"))
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	var payload map[string]any
	if r.Body != nil && r.ContentLength != 0 {
		if err := dec.Decode(&payload); err != nil && !errors.Is(err, io.EOF) {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
	}
	if err := s.serverStore.MarkSeen(r.Context(), server.ID); err != nil {
		s.log.Error("heartbeat", "server", server.ID, "error", err)
		http.Error(w, "heartbeat error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "server": server.Name})
}

func (s *Server) logsJSON(w http.ResponseWriter, r *http.Request) {
	events, err := s.logStore.Query(r.Context(), queryFromRequest(r, s.currentLocation(r.Context())))
	if err != nil {
		http.Error(w, "query error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, events)
}

func (s *Server) streamLogs(w http.ResponseWriter, r *http.Request) {
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("Accept")), []byte("text/event-stream")) != 1 &&
		!strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
		http.Error(w, "SSE requires Accept: text/event-stream", http.StatusBadRequest)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	id, ch := s.hub.Subscribe()
	defer s.hub.Unsubscribe(id)

	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case event, ok := <-ch:
			if !ok {
				return
			}
			raw, _ := json.Marshal(event)
			fmt.Fprintf(w, "event: log\ndata: %s\n\n", raw)
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprint(w, ": heartbeat\n\n")
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func queryFromRequest(r *http.Request, loc *time.Location) logs.Query {
	if loc == nil {
		loc = timezone.Load(timezone.Default)
	}
	v := r.URL.Query()
	return queryFromValues(v, loc)
}

func queryFromForm(r *http.Request, loc *time.Location) logs.Query {
	if loc == nil {
		loc = timezone.Load(timezone.Default)
	}
	return queryFromValues(r.Form, loc)
}

func queryFromValues(v url.Values, loc *time.Location) logs.Query {
	q := logs.Query{
		EventType:    limitedQueryValue(v.Get("event_type"), maxQueryTextBytes),
		Severity:     limitedQueryValue(v.Get("severity"), maxQueryTextBytes),
		Player:       limitedQueryValue(v.Get("player"), maxQueryTextBytes),
		Resource:     limitedQueryValue(v.Get("resource"), maxQueryTextBytes),
		Keyword:      limitedQueryValue(v.Get("q"), maxQueryTextBytes),
		Metadata:     limitedQueryValue(v.Get("metadata"), maxMetadataQueryBytes),
		ReviewStatus: logs.NormalizeReviewStatus(limitedQueryValue(v.Get("review_status"), maxQueryTextBytes)),
		ArchiveMode:  logs.NormalizeArchiveMode(limitedQueryValue(v.Get("archive"), maxQueryTextBytes)),
		Limit:        defaultLogPageLimit,
	}
	if n, err := strconv.ParseInt(v.Get("server_id"), 10, 64); err == nil {
		q.ServerID = n
	}
	if n, err := strconv.Atoi(v.Get("limit")); err == nil {
		q.Limit = n
	}
	if n, err := strconv.Atoi(v.Get("offset")); err == nil {
		q.Offset = n
	}
	if q.Limit <= 0 {
		q.Limit = defaultLogPageLimit
	}
	if q.Limit > maxQueryLimit {
		q.Limit = maxQueryLimit
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
	if q.Offset > maxQueryOffset {
		q.Offset = maxQueryOffset
	}
	if t, err := time.Parse(time.RFC3339, v.Get("since")); err == nil {
		q.Since = &t
	} else if t, err := time.ParseInLocation("2006-01-02T15:04", v.Get("since"), loc); err == nil {
		q.Since = &t
	}
	if t, err := time.Parse(time.RFC3339, v.Get("until")); err == nil {
		q.Until = &t
	} else if t, err := time.ParseInLocation("2006-01-02T15:04", v.Get("until"), loc); err == nil {
		q.Until = &t
	}
	return q
}

func pageParams(r *http.Request, defaultLimit, maxLimit int) (int, int) {
	if defaultLimit <= 0 {
		defaultLimit = defaultLogPageLimit
	}
	if maxLimit <= 0 {
		maxLimit = maxQueryLimit
	}
	v := r.URL.Query()
	limit := defaultLimit
	if n, err := strconv.Atoi(v.Get("limit")); err == nil {
		limit = n
	}
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	offset := 0
	if n, err := strconv.Atoi(v.Get("offset")); err == nil {
		offset = n
	}
	if offset < 0 {
		offset = 0
	}
	if offset > maxQueryOffset {
		offset = maxQueryOffset
	}
	return limit, offset
}

func applyPageQuery(query map[string]string, page logs.Page) {
	if query == nil {
		return
	}
	if page.Limit > 0 {
		query["limit"] = strconv.Itoa(page.Limit)
	}
	if page.Offset > 0 {
		query["offset"] = strconv.Itoa(page.Offset)
	} else {
		delete(query, "offset")
	}
}

func limitedQueryValue(value string, maxBytes int) string {
	value = strings.TrimSpace(value)
	if len(value) <= maxBytes {
		return value
	}
	last := 0
	for i := range value {
		if i == maxBytes {
			return value[:i]
		}
		if i > maxBytes {
			return value[:last]
		}
		last = i
	}
	return value[:last]
}

func queryMap(r *http.Request, loc *time.Location) map[string]string {
	if loc == nil {
		loc = timezone.Load(timezone.Default)
	}
	out := map[string]string{}
	for key, values := range r.URL.Query() {
		if len(values) > 0 {
			out[key] = values[0]
		}
	}
	if since := out["since"]; since != "" {
		out["since_local"] = datetimeLocalValue(since, loc)
	}
	if until := out["until"]; until != "" {
		out["until_local"] = datetimeLocalValue(until, loc)
	}
	return out
}

func queryMapFromForm(r *http.Request) map[string]string {
	out := map[string]string{}
	for _, key := range []string{
		"q", "server_id", "event_type", "severity", "player", "resource",
		"metadata", "review_status", "archive", "since", "until", "limit",
	} {
		if value := strings.TrimSpace(r.FormValue(key)); value != "" {
			out[key] = value
		}
	}
	return out
}

func aiJSONDraftFromRequest(r *http.Request, lang string) aiJSONDraft {
	id, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("id")), 10, 64)
	source := aijson.NormalizeSource(r.FormValue("source"))
	spec := strings.TrimSpace(r.FormValue("spec"))
	if spec == "" {
		spec = defaultAIJSONSpec(lang)
	}
	eventType := strings.TrimSpace(firstNonBlank(r.FormValue("event_type"), r.URL.Query().Get("event_type")))
	resource := strings.TrimSpace(firstNonBlank(r.FormValue("resource"), r.URL.Query().Get("resource")))
	sample := strings.TrimSpace(firstNonBlank(r.FormValue("sample"), r.URL.Query().Get("sample")))
	return aiJSONDraft{
		ID:          id,
		Name:        strings.TrimSpace(r.FormValue("name")),
		Description: strings.TrimSpace(r.FormValue("description")),
		Source:      source,
		EventType:   eventType,
		Resource:    resource,
		Prompt:      strings.TrimSpace(r.FormValue("prompt")),
		Spec:        spec,
		Sample:      sample,
	}
}

func aiJSONInputFromRequest(r *http.Request) aijson.MethodInput {
	return aijson.MethodInput{
		Name:        r.FormValue("name"),
		Description: r.FormValue("description"),
		Source:      r.FormValue("source"),
		EventType:   r.FormValue("event_type"),
		Resource:    r.FormValue("resource"),
		Prompt:      r.FormValue("prompt"),
		Spec:        json.RawMessage(r.FormValue("spec")),
	}
}

func defaultAIJSONSpec(lang ...string) string {
	code := i18n.DefaultLanguage
	if len(lang) > 0 {
		code = lang[0]
	}
	spec := map[string]any{
		"title":            i18n.T(code, "ai_json.default.title"),
		"description":      i18n.T(code, "ai_json.default.description"),
		"summary_path":     "",
		"summary_template": "",
		"badges":           []any{},
		"metrics":          []any{},
		"fields": []map[string]any{{
			"label":  i18n.T(code, "ai_json.default.field"),
			"path":   "",
			"format": "text",
			"span":   "wide",
		}},
		"sections":    []any{},
		"lists":       []any{},
		"tables":      []any{},
		"json_blocks": []any{},
	}
	out, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(out)
}

func prettyJSONString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "{}"
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return string(raw)
	}
	out, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return string(raw)
	}
	return string(out)
}

func selectedEventIDs(r *http.Request) []int64 {
	values := append([]string{}, r.Form["event_id"]...)
	values = append(values, r.Form["event_ids"]...)
	ids := make([]int64, 0, len(values))
	seen := map[int64]struct{}{}
	for _, raw := range values {
		for _, part := range strings.Split(raw, ",") {
			id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
			if err != nil || id <= 0 {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	return ids
}

func returnTo(r *http.Request, fallback string) string {
	value := strings.TrimSpace(r.FormValue("return_to"))
	if value == "" || !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") {
		return fallback
	}
	return value
}

func (s *Server) auditLogAction(r *http.Request, entry logs.AdminAuditEntry) error {
	return s.logStore.AuditAdminAction(r.Context(), auditEntry(r, entry))
}

func auditEntry(r *http.Request, entry logs.AdminAuditEntry) logs.AdminAuditEntry {
	admin := adminFromContext(r.Context())
	entry.AdminID = admin.ID
	entry.AdminUsername = admin.Username
	return entry
}

func datetimeLocalValue(raw string, loc *time.Location) string {
	if loc == nil {
		loc = timezone.Load(timezone.Default)
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.In(loc).Format("2006-01-02T15:04")
	}
	return raw
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

func intPtr(v *int) string {
	if v == nil {
		return ""
	}
	return strconv.Itoa(*v)
}

func floatPtr(v *float64) string {
	if v == nil {
		return ""
	}
	return strconv.FormatFloat(*v, 'f', 3, 64)
}

func eventActor(e logs.Event) string {
	if value := metaString(e.Metadata, "character_name", "characterName", "char_name", "charName"); value != "" {
		if e.PlayerName != "" && e.PlayerName != value {
			return value + " / " + e.PlayerName
		}
		return value
	}
	if e.PlayerName != "" {
		return e.PlayerName
	}
	return "system"
}

func eventSummary(e logs.Event) string {
	switch e.EventType {
	case "money_change":
		parts := []string{}
		if moneyType := metaString(e.Metadata, "money_type", "account"); moneyType != "" {
			parts = append(parts, moneyType)
		}
		if operation := metaString(e.Metadata, "operation"); operation != "" {
			parts = append(parts, operation)
		}
		if amount := metaString(e.Metadata, "amount"); amount != "" {
			parts = append(parts, amount)
		}
		if balance := metaString(e.Metadata, "balance"); balance != "" {
			parts = append(parts, "balance "+balance)
		}
		if reason := metaString(e.Metadata, "reason"); reason != "" {
			parts = append(parts, reason)
		}
		if len(parts) > 0 {
			return strings.Join(parts, " · ")
		}
	case "inventory_diff":
		if summary := inventoryDiffSummary(e.Metadata); summary != "" {
			return summary
		}
	case "inventory_add", "inventory_remove":
		parts := []string{}
		if item := metaString(e.Metadata, "item", "name", "itemName"); item != "" {
			parts = append(parts, item)
		}
		if count := metaString(e.Metadata, "count", "amount"); count != "" {
			parts = append(parts, "x"+count)
		}
		if reason := metaString(e.Metadata, "reason"); reason != "" {
			parts = append(parts, reason)
		}
		if len(parts) > 0 {
			return strings.Join(parts, " · ")
		}
	}
	return e.Message
}

func eventMetaChips(e logs.Event) []string {
	chips := []string{}
	for _, key := range []string{"job", "gang", "money_type", "operation", "reason", "context_text", "plate", "weapon"} {
		if value := metaString(e.Metadata, key); value != "" {
			chips = append(chips, key+"="+truncateLabel(value, 36))
		}
	}
	if changeCount := metaString(e.Metadata, "change_count"); changeCount != "" {
		chips = append(chips, "changes="+changeCount)
	}
	if len(chips) > 4 {
		return chips[:4]
	}
	return chips
}

func metaString(raw json.RawMessage, keys ...string) string {
	if len(raw) == 0 {
		return ""
	}
	var metadata map[string]any
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return ""
	}
	for _, key := range keys {
		value, ok := metadata[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return typed
			}
		case json.Number:
			return typed.String()
		case float64:
			return strconv.FormatFloat(typed, 'f', -1, 64)
		case bool:
			return strconv.FormatBool(typed)
		}
	}
	return ""
}

func inventoryDiffSummary(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var metadata struct {
		Changes json.RawMessage `json:"changes"`
	}
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return ""
	}
	changes := inventoryDiffChanges(metadata.Changes)
	parts := []string{}
	for i, change := range changes {
		if i >= 4 {
			parts = append(parts, fmt.Sprintf("+%d more", len(changes)-i))
			break
		}
		name := firstNonBlank(change.Label, change.Name)
		if name == "" {
			name = "item"
		}
		delta := strconv.FormatFloat(change.Delta, 'f', -1, 64)
		if change.Delta > 0 {
			delta = "+" + delta
		}
		parts = append(parts, name+" "+delta)
	}
	return strings.Join(parts, ", ")
}

type inventoryChangeSummary struct {
	Name  string  `json:"name"`
	Label string  `json:"label"`
	Delta float64 `json:"delta"`
}

func inventoryDiffChanges(raw json.RawMessage) []inventoryChangeSummary {
	if len(raw) == 0 {
		return nil
	}
	var array []inventoryChangeSummary
	if err := json.Unmarshal(raw, &array); err == nil {
		return array
	}
	var object map[string]inventoryChangeSummary
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil
	}
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		left, leftErr := strconv.Atoi(keys[i])
		right, rightErr := strconv.Atoi(keys[j])
		if leftErr == nil && rightErr == nil {
			return left < right
		}
		return keys[i] < keys[j]
	})
	changes := make([]inventoryChangeSummary, 0, len(keys))
	for _, key := range keys {
		changes = append(changes, object[key])
	}
	return changes
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func truncateLabel(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 || len(value) <= max {
		return value
	}
	last := 0
	for i := range value {
		if i == max {
			return value[:i] + "..."
		}
		if i > max {
			return value[:last] + "..."
		}
		last = i
	}
	return value[:last] + "..."
}

func playerIdentifier(e logs.Event) string {
	switch {
	case e.License != "":
		return e.License
	case e.CitizenID != "":
		return e.CitizenID
	case e.Discord != "":
		return e.Discord
	case e.Steam != "":
		return e.Steam
	case e.PlayerName != "":
		return e.PlayerName
	default:
		return ""
	}
}

func accountIdentifier(link logs.AccountLink) string {
	if link.License != "" && link.License != "unknown" {
		return link.License
	}
	if len(link.Discords) > 0 {
		return link.Discords[0]
	}
	if len(link.Steams) > 0 {
		return link.Steams[0]
	}
	if len(link.CitizenIDs) > 0 {
		return link.CitizenIDs[0]
	}
	if len(link.Names) > 0 {
		return link.Names[0]
	}
	return ""
}

func IsNotFound(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}
