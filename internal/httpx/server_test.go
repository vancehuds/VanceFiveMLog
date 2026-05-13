package httpx

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vancehuds/VanceFiveMLog/internal/aijson"
	"github.com/vancehuds/VanceFiveMLog/internal/auth"
	"github.com/vancehuds/VanceFiveMLog/internal/logs"
	"github.com/vancehuds/VanceFiveMLog/internal/serverkeys"
	"github.com/vancehuds/VanceFiveMLog/internal/settings"
)

func TestQueryFromRequestParsesFilters(t *testing.T) {
	req := httptest.NewRequest("GET", "/logs?server_id=12&limit=250&offset=500&since=2026-05-05T08:30&until=2026-05-05T09:45&player=license:abc", nil)
	loc := time.FixedZone("test", 8*60*60)
	q := queryFromRequest(req, loc)

	if q.ServerID != 12 {
		t.Fatalf("server id = %d", q.ServerID)
	}
	if q.Limit != 250 || q.Offset != 500 {
		t.Fatalf("limit/offset = %d/%d", q.Limit, q.Offset)
	}
	if q.Since == nil || q.Until == nil {
		t.Fatal("expected local datetime filters")
	}
	if got := q.Since.In(time.UTC).Format(time.RFC3339); got != "2026-05-05T00:30:00Z" {
		t.Fatalf("since UTC = %s", got)
	}
	if q.Player != "license:abc" {
		t.Fatalf("player = %s", q.Player)
	}
}

func TestQueryMapAddsDatetimeLocalValues(t *testing.T) {
	req := httptest.NewRequest("GET", "/logs?since=2026-05-05T08:30:00Z", nil)
	loc := time.FixedZone("test", 8*60*60)
	values := queryMap(req, loc)
	if values["since_local"] != "2026-05-05T16:30" {
		t.Fatalf("since_local = %s", values["since_local"])
	}
}

func TestPageParamsClampLimitAndOffset(t *testing.T) {
	req := httptest.NewRequest("GET", "/accounts?limit=999&offset=-5", nil)
	limit, offset := pageParams(req, defaultAccountPageLimit, maxAccountPageLimit)
	if limit != maxAccountPageLimit || offset != 0 {
		t.Fatalf("limit/offset = %d/%d", limit, offset)
	}

	req = httptest.NewRequest("GET", "/accounts?limit=0&offset=200000", nil)
	limit, offset = pageParams(req, defaultAccountPageLimit, maxAccountPageLimit)
	if limit != defaultAccountPageLimit || offset != maxQueryOffset {
		t.Fatalf("limit/offset = %d/%d", limit, offset)
	}
}

func TestApplyPageQueryNormalizesLimitAndOffset(t *testing.T) {
	query := map[string]string{"limit": "5000", "offset": "900"}
	applyPageQuery(query, logs.NewPage(100, 0, 20))
	if query["limit"] != "100" {
		t.Fatalf("limit = %s", query["limit"])
	}
	if _, ok := query["offset"]; ok {
		t.Fatalf("offset should be removed: %#v", query)
	}
}

func TestRoutesServeStaticFiles(t *testing.T) {
	staticDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(staticDir, "app.css"), []byte("body{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := &Server{staticDir: staticDir}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/static/app.css", nil)
	server.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "body{}" {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestSecurityHeaders(t *testing.T) {
	handler := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Content-Security-Policy"); !strings.Contains(got, "frame-ancestors 'none'") {
		t.Fatalf("csp = %q", got)
	}
	if got := rec.Header().Get("Strict-Transport-Security"); got == "" {
		t.Fatal("expected hsts header")
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("cache-control = %q", got)
	}
}

func TestSecurityHeadersAllowTurnstileOnLogin(t *testing.T) {
	handler := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	handler.ServeHTTP(rec, req)

	csp := rec.Header().Get("Content-Security-Policy")
	for _, want := range []string{
		"script-src 'self' https://challenges.cloudflare.com",
		"connect-src 'self' https://challenges.cloudflare.com",
		"frame-src https://challenges.cloudflare.com",
	} {
		if !strings.Contains(csp, want) {
			t.Fatalf("missing %q in csp = %q", want, csp)
		}
	}
}

func TestRequireJSONRejectsWrongContentType(t *testing.T) {
	handler := requireJSON(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/events", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "text/plain")
	handler(rec, req)

	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestRequireCSRFLimitsFormBody(t *testing.T) {
	server := &Server{sessions: auth.NewSessionManager("0123456789abcdef0123456789abcdef")}
	handler := server.requireCSRF(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/settings", strings.NewReader("csrf_token="+strings.Repeat("a", maxFormBytes+1)))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestRequireRoleForbidsViewer(t *testing.T) {
	server := &Server{}
	handler := server.requireRole(canManageServers)(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodPost, "/servers", nil)
	ctx := context.WithValue(req.Context(), adminContextKey{}, auth.Admin{Role: auth.RoleViewer})
	rec := httptest.NewRecorder()

	handler(rec, req.WithContext(ctx))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestRequireRoleAllowsAdminForServerManagement(t *testing.T) {
	server := &Server{}
	handler := server.requireRole(canManageServers)(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodPost, "/servers", nil)
	ctx := context.WithValue(req.Context(), adminContextKey{}, auth.Admin{Role: auth.RoleAdmin})
	rec := httptest.NewRecorder()

	handler(rec, req.WithContext(ctx))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestRequireRoleRestrictsAdminManagementToOwner(t *testing.T) {
	server := &Server{}
	handler := server.requireRole(canManageAdmins)(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	for _, tt := range []struct {
		name string
		role string
		want int
	}{
		{name: "admin", role: auth.RoleAdmin, want: http.StatusForbidden},
		{name: "viewer", role: auth.RoleViewer, want: http.StatusForbidden},
		{name: "owner", role: auth.RoleOwner, want: http.StatusNoContent},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/admins", nil)
			ctx := context.WithValue(req.Context(), adminContextKey{}, auth.Admin{Role: tt.role})
			rec := httptest.NewRecorder()

			handler(rec, req.WithContext(ctx))

			if rec.Code != tt.want {
				t.Fatalf("status = %d", rec.Code)
			}
		})
	}
}

func TestRequireRoleRestrictsLogManagement(t *testing.T) {
	server := &Server{}
	manageHandler := server.requireRole(canManageLogs)(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	for _, tt := range []struct {
		name       string
		role       string
		manageWant int
	}{
		{name: "owner", role: auth.RoleOwner, manageWant: http.StatusNoContent},
		{name: "admin", role: auth.RoleAdmin, manageWant: http.StatusNoContent},
		{name: "viewer", role: auth.RoleViewer, manageWant: http.StatusForbidden},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/logs/1/review", nil)
			ctx := context.WithValue(req.Context(), adminContextKey{}, auth.Admin{Role: tt.role})
			rec := httptest.NewRecorder()
			manageHandler(rec, req.WithContext(ctx))
			if rec.Code != tt.manageWant {
				t.Fatalf("manage status = %d", rec.Code)
			}
		})
	}
}

func TestRequireRoleRestrictsAIJSONManagementToOwner(t *testing.T) {
	server := &Server{}
	handler := server.requireRole(canManageAIJSON)(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	for _, tt := range []struct {
		name string
		role string
		want int
	}{
		{name: "owner", role: auth.RoleOwner, want: http.StatusNoContent},
		{name: "admin", role: auth.RoleAdmin, want: http.StatusForbidden},
		{name: "viewer", role: auth.RoleViewer, want: http.StatusForbidden},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/ai-json", nil)
			ctx := context.WithValue(req.Context(), adminContextKey{}, auth.Admin{Role: tt.role})
			rec := httptest.NewRecorder()
			handler(rec, req.WithContext(ctx))
			if rec.Code != tt.want {
				t.Fatalf("status = %d", rec.Code)
			}
		})
	}
}

func TestDecodeIngestEventsAcceptsSinglePluginEvent(t *testing.T) {
	events, err := decodeIngestEvents(strings.NewReader(`{
		"event": "door forced",
		"level": "warning",
		"player_source": 7,
		"plugin_resource": "doorlocks",
		"message": "door forced open",
		"data": {"door_id": "bank-front"}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("events length = %d", len(events))
	}
	if events[0].Event != "door forced" || events[0].PluginResource != "doorlocks" {
		t.Fatalf("event = %+v", events[0])
	}
	if events[0].PlayerSource == nil || *events[0].PlayerSource != 7 {
		t.Fatalf("player source = %v", events[0].PlayerSource)
	}
}

func TestDecodeIngestEventsAcceptsArray(t *testing.T) {
	events, err := decodeIngestEvents(strings.NewReader(`[
		{"type": "money add", "plugin": "banking"},
		{"type": "money remove", "plugin": "banking"}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("events length = %d", len(events))
	}
	if events[1].Type != "money remove" {
		t.Fatalf("second event = %+v", events[1])
	}
}

func TestDecodeIngestEventsAcceptsWrappedEvent(t *testing.T) {
	events, err := decodeIngestEvents(strings.NewReader(`{
		"event": {"event_type": "admin_warn", "message": "warned"}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].EventType != "admin_warn" {
		t.Fatalf("events = %+v", events)
	}
}

func TestTemplatesParse(t *testing.T) {
	server := &Server{templatesDir: filepath.Join("..", "..", "web", "templates")}
	if _, err := server.parseTemplates(); err != nil {
		t.Fatal(err)
	}
}

func TestLoginTemplateRendersTurnstileWhenEnabled(t *testing.T) {
	server := &Server{templatesDir: filepath.Join("..", "..", "web", "templates")}
	tmpl, err := server.parseTemplates()
	if err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	err = tmpl.ExecuteTemplate(&out, "login.html", map[string]any{
		"Title":            "登录",
		"TurnstileEnabled": true,
		"TurnstileSiteKey": "0xsite",
	})
	if err != nil {
		t.Fatal(err)
	}
	rendered := out.String()
	for _, want := range []string{
		`https://challenges.cloudflare.com/turnstile/v0/api.js`,
		`class="cf-turnstile"`,
		`data-sitekey="0xsite"`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("missing %q in %s", want, rendered)
		}
	}
}

func TestLoginTemplateOmitsTurnstileWhenDisabled(t *testing.T) {
	server := &Server{templatesDir: filepath.Join("..", "..", "web", "templates")}
	tmpl, err := server.parseTemplates()
	if err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	err = tmpl.ExecuteTemplate(&out, "login.html", map[string]any{"Title": "登录"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "cf-turnstile") {
		t.Fatalf("turnstile should be omitted: %s", out.String())
	}
}

func TestRenderLoginUsesRequestedLanguage(t *testing.T) {
	server := &Server{templatesDir: filepath.Join("..", "..", "web", "templates")}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/login?lang=en", nil)

	server.renderLogin(rec, req, "")

	rendered := rec.Body.String()
	for _, want := range []string{`<html lang="en">`, `FiveM / Qbox server audit log console`, `Sign in`} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("missing %q: %s", want, rendered)
		}
	}
}

func TestSetLanguageStoresCookieAndRedirects(t *testing.T) {
	server := &Server{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/language", strings.NewReader("lang=en&return_to=%2Flogs%3Fseverity%3Dwarning"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	server.setLanguage(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "/logs?severity=warning" {
		t.Fatalf("location = %q", got)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "vfl_lang" || cookies[0].Value != "en" {
		t.Fatalf("cookies = %+v", cookies)
	}
}

func TestLogRowRendersFullEventDataAndEncodedLinks(t *testing.T) {
	server := &Server{templatesDir: filepath.Join("..", "..", "web", "templates"), timeZone: "Asia/Shanghai"}
	tmpl, err := server.parseTemplates()
	if err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	err = tmpl.ExecuteTemplate(&out, "log-row", logs.Event{
		ID:         7,
		ServerName: "city",
		EventType:  "door forced",
		Severity:   "warning",
		PlayerName: "Vance",
		License:    "license:abc",
		Resource:   "doorlocks",
		Message:    "door forced open",
		Metadata:   json.RawMessage(`{"door_id":"bank-front"}`),
		OccurredAt: time.Unix(10, 0).UTC(),
		CreatedAt:  time.Unix(11, 0).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}

	rendered := out.String()
	if !strings.Contains(rendered, `data-event=`) || !strings.Contains(rendered, `door_id`) {
		t.Fatalf("missing event data: %s", rendered)
	}
	if !strings.Contains(rendered, `/logs?event_type=door&#43;forced`) {
		t.Fatalf("missing encoded event link: %s", rendered)
	}
	if !strings.Contains(rendered, `/players/license:abc`) {
		t.Fatalf("missing player link: %s", rendered)
	}
	if !strings.Contains(rendered, `08:00:10`) {
		t.Fatalf("missing Beijing clock: %s", rendered)
	}
}

func TestLogDetailTemplateRendersEventAndReviewControls(t *testing.T) {
	server := &Server{templatesDir: filepath.Join("..", "..", "web", "templates"), timeZone: "Asia/Shanghai"}
	tmpl, err := server.parseTemplates()
	if err != nil {
		t.Fatal(err)
	}

	source := 42
	var out strings.Builder
	err = tmpl.ExecuteTemplate(&out, "log_detail.html", pageData{
		Title:     "日志详情",
		Active:    "logs",
		Admin:     auth.Admin{Username: "admin", Role: auth.RoleAdmin},
		CSRFToken: "csrf",
		Event: logs.Event{
			ID:           7,
			ServerName:   "city",
			EventType:    "money_change",
			Severity:     "warning",
			PlayerSource: &source,
			PlayerName:   "Vance",
			License:      "license:abc",
			Resource:     "qb-core",
			Message:      "cash changed",
			Metadata:     json.RawMessage(`{"amount":100,"money_type":"cash"}`),
			OccurredAt:   time.Unix(10, 0).UTC(),
			CreatedAt:    time.Unix(11, 0).UTC(),
			Review: logs.EventReview{
				Status: logs.ReviewStatusSuspicious,
				Note:   "watch",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	rendered := out.String()
	for _, want := range []string{
		`#7`,
		`/logs/7/review`,
		`/logs/7/archive`,
		`/players/license:abc`,
		`cash changed`,
		`&#34;amount&#34;: 100`,
		`watch`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("missing %q: %s", want, rendered)
		}
	}
}

func TestLogsTemplateRendersManagementControlsByRole(t *testing.T) {
	server := &Server{templatesDir: filepath.Join("..", "..", "web", "templates")}
	tmpl, err := server.parseTemplates()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		role     string
		mustHave []string
		mustOmit []string
	}{
		{
			name:     "owner",
			role:     auth.RoleOwner,
			mustHave: []string{`/logs/bulk/archive`, `data-review-form`, `data-review-archive`},
			mustOmit: []string{`/logs/bulk/delete`, `data-review-delete`},
		},
		{
			name:     "admin",
			role:     auth.RoleAdmin,
			mustHave: []string{`/logs/bulk/archive`, `data-review-form`, `data-review-archive`},
			mustOmit: []string{`/logs/bulk/delete`, `data-review-delete`},
		},
		{
			name:     "viewer",
			role:     auth.RoleViewer,
			mustOmit: []string{`/logs/bulk/archive`, `/logs/bulk/delete`, `data-review-form`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out strings.Builder
			err = tmpl.ExecuteTemplate(&out, "logs.html", pageData{
				Title:  "日志检索",
				Active: "logs",
				Admin:  auth.Admin{Username: tt.name, Role: tt.role},
				Query:  map[string]string{},
				Events: []logs.Event{{
					ID:         7,
					ServerName: "city",
					EventType:  "money_change",
					Severity:   "warning",
					PlayerName: "Vance",
					Message:    "cash changed",
					Metadata:   json.RawMessage(`{}`),
					OccurredAt: time.Unix(10, 0).UTC(),
				}},
			})
			if err != nil {
				t.Fatal(err)
			}
			rendered := out.String()
			for _, want := range tt.mustHave {
				if !strings.Contains(rendered, want) {
					t.Fatalf("missing %q: %s", want, rendered)
				}
			}
			for _, text := range tt.mustOmit {
				if strings.Contains(rendered, text) {
					t.Fatalf("unexpected %q: %s", text, rendered)
				}
			}
		})
	}
}

func TestLogsTemplateEmbedsAIJSONMethods(t *testing.T) {
	server := &Server{templatesDir: filepath.Join("..", "..", "web", "templates")}
	tmpl, err := server.parseTemplates()
	if err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	err = tmpl.ExecuteTemplate(&out, "logs.html", pageData{
		Title:  "日志检索",
		Active: "logs",
		Admin:  auth.Admin{Username: "owner", Role: auth.RoleOwner},
		Query:  map[string]string{},
		AIJSONMethods: []aijson.Method{{
			ID:     1,
			Name:   "Inventory",
			Source: "metadata",
			Spec:   json.RawMessage(`{"title":"Inventory","fields":[{"label":"Item","path":"item"}]}`),
			Active: true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	rendered := out.String()
	for _, want := range []string{`id="ai-json-methods"`, `Inventory`, `data-ai-json-method`, `data-ai-json-render`, `data-ai-json-import-form`} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("missing %q: %s", want, rendered)
		}
	}
}

func TestDashboardTemplateEmbedsAIJSONMethods(t *testing.T) {
	server := &Server{templatesDir: filepath.Join("..", "..", "web", "templates")}
	tmpl, err := server.parseTemplates()
	if err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	err = tmpl.ExecuteTemplate(&out, "dashboard.html", pageData{
		Title:  "实时日志仪表盘",
		Active: "dashboard",
		Admin:  auth.Admin{Username: "owner", Role: auth.RoleOwner},
		AIJSONMethods: []aijson.Method{{
			ID:     2,
			Name:   "Inventory Prefix",
			Source: "metadata",
			Spec:   json.RawMessage(`{"title":"Inventory","fields":[{"label":"Item","path":"metadata.item"}]}`),
			Active: true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	rendered := out.String()
	for _, want := range []string{`id="ai-json-methods"`, `Inventory Prefix`, `metadata.item`, `data-ai-json-render`} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("missing %q: %s", want, rendered)
		}
	}
}

func TestEventSummaryHandlesInventoryDiffObjectChanges(t *testing.T) {
	event := logs.Event{
		EventType: "inventory_diff",
		Message:   "inventory changed",
		Metadata: json.RawMessage(`{
			"changes": {
				"2": {"name": "ammo-9", "label": "9mm 子弹", "delta": 717},
				"1": {"name": "WEAPON_PISTOL", "label": "手枪", "delta": 1}
			}
		}`),
	}

	if got := eventSummary(event); got != "手枪 +1, 9mm 子弹 +717" {
		t.Fatalf("summary = %s", got)
	}
}

func TestGeoTemplateRendersInteractiveMap(t *testing.T) {
	server := &Server{templatesDir: filepath.Join("..", "..", "web", "templates")}
	tmpl, err := server.parseTemplates()
	if err != nil {
		t.Fatal(err)
	}

	x, y, z := 219.2, -810.1, 30.7
	var out strings.Builder
	err = tmpl.ExecuteTemplate(&out, "geo.html", pageData{
		Title:  "geo",
		Active: "geo",
		GeoMap: GeoMapConfig{
			ImageURL: "/static/test-map.jpg",
			MinX:     -1000,
			MaxX:     1000,
			MinY:     -2000,
			MaxY:     2000,
		},
		Query: map[string]string{},
		Events: []logs.Event{{
			ID:         9,
			ServerName: "city",
			EventType:  "player_move",
			Severity:   "success",
			PlayerName: "Vance",
			Message:    "snapshot",
			CoordsX:    &x,
			CoordsY:    &y,
			CoordsZ:    &z,
			Metadata:   json.RawMessage(`{}`),
			OccurredAt: time.Unix(10, 0).UTC(),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	rendered := out.String()
	for _, want := range []string{
		`data-geo-map`,
		`data-geo-config=`,
		`/static/test-map.jpg`,
		`data-geo-point="9"`,
		`data-geo-event=`,
		`geo-marker success`,
		`data-geo-zoom="in"`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("missing %q in %s", want, rendered)
		}
	}
}

func TestDefaultLosSantosMapAssetExists(t *testing.T) {
	path := filepath.Join("..", "..", "web", "static", "maps", "los-santos.jpg")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("default geo map asset missing: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("default geo map asset is empty")
	}
}

func TestAccountIdentifierFallbacks(t *testing.T) {
	link := logs.AccountLink{
		License:  "unknown",
		Discords: []string{"discord:42"},
		Steams:   []string{"steam:1"},
	}
	if got := accountIdentifier(link); got != "discord:42" {
		t.Fatalf("identifier = %s", got)
	}

	link.License = "license:abc"
	if got := accountIdentifier(link); got != "license:abc" {
		t.Fatalf("identifier = %s", got)
	}
}

func TestAccountsTemplateRendersViewButtonWithFullAccountData(t *testing.T) {
	server := &Server{templatesDir: filepath.Join("..", "..", "web", "templates")}
	tmpl, err := server.parseTemplates()
	if err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	err = tmpl.ExecuteTemplate(&out, "accounts.html", pageData{
		Title:  "账户关联审计页",
		Active: "accounts",
		Query:  map[string]string{},
		AccountLinks: []logs.AccountLink{{
			License:    "license:abcdefghijklmnopqrstuvwxyz",
			Names:      []string{"Vance", "VanceFive"},
			Discords:   []string{"discord:123456789"},
			Steams:     []string{"steam:110000abcdef"},
			CitizenIDs: []string{"CID12345"},
			Events:     12,
			LastSeen:   time.Unix(10, 0).UTC(),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	rendered := out.String()
	for _, want := range []string{
		`data-open-account`,
		`完整玩家信息`,
		`license:abcdefghijklmnopqrstuvwxyz`,
		`discord:123456789`,
		`steam:110000abcdef`,
		`CID12345`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("missing %q in %s", want, rendered)
		}
	}
}

func TestServerHealthLabels(t *testing.T) {
	server := &Server{templatesDir: filepath.Join("..", "..", "web", "templates")}
	tmpl, err := server.parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	if tmpl == nil {
		t.Fatal("expected templates")
	}

	now := time.Now()
	data := pageData{
		Title:     "test",
		Active:    "dashboard",
		Retention: 180,
		Servers: []serverkeys.Server{{
			ID:         1,
			Name:       "city",
			Active:     true,
			LastSeenAt: &now,
		}},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	server.render(rec, req, "dashboard.html", data)
	if rec.Code != 200 {
		t.Fatalf("render status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestLayoutHidesSettingsForViewer(t *testing.T) {
	server := &Server{templatesDir: filepath.Join("..", "..", "web", "templates")}
	tmpl, err := server.parseTemplates()
	if err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	err = tmpl.ExecuteTemplate(&out, "shell-start", pageData{
		Title:     "test",
		Admin:     auth.Admin{Username: "readonly", Role: auth.RoleViewer},
		Retention: 180,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), `href="/settings"`) {
		t.Fatalf("viewer should not see settings link: %s", out.String())
	}
}

func TestLayoutShowsAIJSONOnlyForOwner(t *testing.T) {
	server := &Server{templatesDir: filepath.Join("..", "..", "web", "templates")}
	tmpl, err := server.parseTemplates()
	if err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		role string
		want bool
	}{
		{role: auth.RoleOwner, want: true},
		{role: auth.RoleAdmin, want: false},
		{role: auth.RoleViewer, want: false},
	} {
		t.Run(tt.role, func(t *testing.T) {
			var out strings.Builder
			err = tmpl.ExecuteTemplate(&out, "shell-start", pageData{
				Title:     "test",
				Admin:     auth.Admin{Username: tt.role, Role: tt.role},
				Retention: 180,
			})
			if err != nil {
				t.Fatal(err)
			}
			has := strings.Contains(out.String(), `href="/ai-json"`)
			if has != tt.want {
				t.Fatalf("ai-json link = %v body=%s", has, out.String())
			}
		})
	}
}

func TestAIJSONTemplateRendersOwnerPanel(t *testing.T) {
	server := &Server{templatesDir: filepath.Join("..", "..", "web", "templates")}
	tmpl, err := server.parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	err = tmpl.ExecuteTemplate(&out, "ai_json.html", pageData{
		Title:       "AI JSON 解析",
		Active:      "ai-json",
		Admin:       auth.Admin{Username: "owner", Role: auth.RoleOwner},
		Retention:   180,
		AIJSONReady: true,
		AIJSONDraft: aiJSONDraft{Source: "metadata", Spec: defaultAIJSONSpec()},
		AIJSONMethods: []aijson.Method{{
			ID:          5,
			Name:        "Money",
			Description: "资金变化",
			Source:      "metadata",
			EventType:   "money_change",
			Spec:        json.RawMessage(`{"title":"Money","fields":[]}`),
			Active:      true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	rendered := out.String()
	for _, want := range []string{`/ai-json/suggest`, `AI 解析一次`, `Money`, `data-ai-json-method-card`, `metrics`, `json_blocks`, `data-ai-json-insert-template`} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("missing %q: %s", want, rendered)
		}
	}
}

func TestAIJSONTemplatePrefillsImportedLogSample(t *testing.T) {
	server := &Server{templatesDir: filepath.Join("..", "..", "web", "templates")}
	tmpl, err := server.parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	err = tmpl.ExecuteTemplate(&out, "ai_json.html", pageData{
		Title:     "AI JSON 解析",
		Active:    "ai-json",
		Admin:     auth.Admin{Username: "owner", Role: auth.RoleOwner},
		Retention: 180,
		AIJSONDraft: aiJSONDraft{
			Source:    "metadata",
			EventType: "inventory_diff",
			Resource:  "ox_inventory",
			Sample:    `{"changes":[{"name":"cash","delta":5}]}`,
			Spec:      defaultAIJSONSpec(),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	rendered := out.String()
	for _, want := range []string{
		`value="inventory_diff"`,
		`value="ox_inventory"`,
		`{&#34;changes&#34;:[{&#34;name&#34;:&#34;cash&#34;,&#34;delta&#34;:5}]}`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("missing %q: %s", want, rendered)
		}
	}
}

func TestSettingsTemplateRespectsRolePanels(t *testing.T) {
	server := &Server{templatesDir: filepath.Join("..", "..", "web", "templates")}
	tmpl, err := server.parseTemplates()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		role     string
		mustHave string
		mustOmit []string
	}{
		{
			name:     "owner",
			role:     auth.RoleOwner,
			mustHave: "AI JSON 提供商",
		},
		{
			name:     "admin",
			role:     auth.RoleAdmin,
			mustHave: "服务器 API Key 管理",
			mustOmit: []string{"新增 / 重置管理员", "日志保留与时区"},
		},
		{
			name:     "viewer",
			role:     auth.RoleViewer,
			mustOmit: []string{"服务器 API Key 管理", "新增 / 重置管理员", "日志保留与时区"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out strings.Builder
			err = tmpl.ExecuteTemplate(&out, "settings.html", pageData{
				Title:     "系统设置",
				Active:    "settings",
				Admin:     auth.Admin{Username: tt.name, Role: tt.role},
				Retention: 180,
			})
			if err != nil {
				t.Fatal(err)
			}
			rendered := out.String()
			if tt.mustHave != "" && !strings.Contains(rendered, tt.mustHave) {
				t.Fatalf("missing %q: %s", tt.mustHave, rendered)
			}
			for _, text := range tt.mustOmit {
				if strings.Contains(rendered, text) {
					t.Fatalf("unexpected %q: %s", text, rendered)
				}
			}
		})
	}
}

func TestSettingsTemplateShowsTimeZone(t *testing.T) {
	server := &Server{templatesDir: filepath.Join("..", "..", "web", "templates"), timeZone: "Asia/Shanghai"}
	tmpl, err := server.parseTemplates()
	if err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	err = tmpl.ExecuteTemplate(&out, "settings.html", pageData{
		Title:     "系统设置",
		Active:    "settings",
		Admin:     auth.Admin{Username: "owner", Role: auth.RoleOwner},
		Retention: 180,
		TimeZone:  "Asia/Shanghai",
	})
	if err != nil {
		t.Fatal(err)
	}
	rendered := out.String()
	if !strings.Contains(rendered, `name="time_zone"`) || !strings.Contains(rendered, `value="Asia/Shanghai"`) {
		t.Fatalf("missing time zone setting: %s", rendered)
	}
}

func TestSettingsTemplateShowsAIProviderWithoutSecret(t *testing.T) {
	server := &Server{templatesDir: filepath.Join("..", "..", "web", "templates")}
	tmpl, err := server.parseTemplates()
	if err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	err = tmpl.ExecuteTemplate(&out, "settings.html", pageData{
		Title:         "系统设置",
		Active:        "settings",
		Admin:         auth.Admin{Username: "owner", Role: auth.RoleOwner},
		Retention:     180,
		TimeZone:      "Asia/Shanghai",
		AIJSONReady:   true,
		AIProviderKey: true,
		AIProvider: settings.AIProviderConfig{
			Provider: settings.AIProviderCustom,
			BaseURL:  "https://ai.example.test/v1",
			Model:    "audit-model",
			APIKey:   "",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	rendered := out.String()
	for _, want := range []string{`name="ai_json_provider"`, `value="https://ai.example.test/v1"`, `value="audit-model"`, `已保存，留空则保留当前密钥`} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("missing %q: %s", want, rendered)
		}
	}
	if strings.Contains(rendered, `sk-test`) {
		t.Fatalf("secret leaked in template: %s", rendered)
	}
}
