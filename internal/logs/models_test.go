package logs

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestIngestEventNormalize(t *testing.T) {
	event := IngestEvent{
		EventType: "player death",
		Message:   "died",
	}
	if err := event.Normalize(time.Unix(10, 0)); err != nil {
		t.Fatal(err)
	}
	if event.EventType != "player_death" {
		t.Fatalf("event type = %s", event.EventType)
	}
	if event.Severity != "info" {
		t.Fatalf("severity = %s", event.Severity)
	}
	if event.OccurredAt == nil {
		t.Fatal("occurred_at not set")
	}
}

func TestIngestEventNormalizeSeverity(t *testing.T) {
	event := IngestEvent{
		EventType: "admin_action",
		Severity:  "critical",
		Message:   "bad severity should fall back",
	}
	if err := event.Normalize(time.Unix(10, 0)); err != nil {
		t.Fatal(err)
	}
	if event.Severity != "info" {
		t.Fatalf("severity = %s", event.Severity)
	}
}

func TestIngestEventNormalizePluginAliases(t *testing.T) {
	source := 42
	event := IngestEvent{
		Type:           "door lock",
		Level:          "warning",
		PlayerSource:   &source,
		PluginResource: "doorlocks",
		Data: map[string]any{
			"door_id": "bank-front",
		},
		Message: "door forced open",
	}
	if err := event.Normalize(time.Unix(10, 0)); err != nil {
		t.Fatal(err)
	}
	if event.EventType != "door_lock" {
		t.Fatalf("event type = %s", event.EventType)
	}
	if event.Severity != "warning" {
		t.Fatalf("severity = %s", event.Severity)
	}
	if event.Source == nil || *event.Source != source {
		t.Fatalf("source = %v", event.Source)
	}
	if event.Resource != "doorlocks" {
		t.Fatalf("resource = %s", event.Resource)
	}
	if event.Metadata["door_id"] != "bank-front" {
		t.Fatalf("metadata door_id = %v", event.Metadata["door_id"])
	}
	if event.Metadata["plugin_resource"] != "doorlocks" {
		t.Fatalf("metadata plugin_resource = %v", event.Metadata["plugin_resource"])
	}
}

func TestIngestEventNormalizePromotesPlayerContextMetadata(t *testing.T) {
	event := IngestEvent{
		EventType: "money_change",
		Metadata: map[string]any{
			"characterName": "Wei Chen",
			"job":           "police:sergeant",
			"gang":          "none",
		},
	}
	if err := event.Normalize(time.Unix(10, 0)); err != nil {
		t.Fatal(err)
	}
	if event.CharacterName != "Wei Chen" {
		t.Fatalf("character name = %q", event.CharacterName)
	}
	if event.Job != "police:sergeant" {
		t.Fatalf("job = %q", event.Job)
	}
	if event.Gang != "none" {
		t.Fatalf("gang = %q", event.Gang)
	}
	if event.Metadata["character_name"] != "Wei Chen" {
		t.Fatalf("metadata character_name = %v", event.Metadata["character_name"])
	}
}

func TestIngestEventUnmarshalToleratesLooseOptionalFields(t *testing.T) {
	var event IngestEvent
	if err := json.Unmarshal([]byte(`{
		"event_type": "coords string",
		"source": "42",
		"player_name": 123,
		"character_name": "role name",
		"job": "mechanic:2",
		"coords": {"x": "1.5", "y": 2, "z": "3.25"},
		"occurred_at": "not a timestamp",
		"metadata": "raw"
	}`), &event); err != nil {
		t.Fatal(err)
	}
	if event.Source == nil || *event.Source != 42 {
		t.Fatalf("source = %v", event.Source)
	}
	if event.PlayerName != "123" {
		t.Fatalf("player name = %q", event.PlayerName)
	}
	if event.CharacterName != "role name" || event.Job != "mechanic:2" {
		t.Fatalf("context fields = %q %q", event.CharacterName, event.Job)
	}
	if event.Coords == nil || event.Coords.X != 1.5 || event.Coords.Y != 2 || event.Coords.Z != 3.25 {
		t.Fatalf("coords = %+v", event.Coords)
	}
	if event.OccurredAt != nil {
		t.Fatalf("occurred at = %v", event.OccurredAt)
	}
	if event.Metadata["data"] != "raw" {
		t.Fatalf("metadata = %+v", event.Metadata)
	}
}

func TestIngestEventUnmarshalIgnoresInvalidOptionalFields(t *testing.T) {
	var event IngestEvent
	if err := json.Unmarshal([]byte(`{
		"event_type": "loose",
		"source": {"bad": true},
		"coords": {"x": {}, "y": 2, "z": 3},
		"metadata": {"ok": true}
	}`), &event); err != nil {
		t.Fatal(err)
	}
	if event.Source != nil {
		t.Fatalf("source = %v", event.Source)
	}
	if event.Coords != nil {
		t.Fatalf("coords = %+v", event.Coords)
	}
	if event.Metadata["ok"] != true {
		t.Fatalf("metadata = %+v", event.Metadata)
	}
}

func TestIngestEventNormalizeScalarData(t *testing.T) {
	event := IngestEvent{
		EventType: "plugin_value",
		Data:      "bank-front",
	}
	if err := event.Normalize(time.Unix(10, 0)); err != nil {
		t.Fatal(err)
	}
	if event.Metadata["data"] != "bank-front" {
		t.Fatalf("metadata data = %v", event.Metadata["data"])
	}
}

func TestIngestEventNormalizeLimitsFieldSizes(t *testing.T) {
	event := IngestEvent{
		EventType:  strings.Repeat("a", maxEventTypeLength+10),
		PlayerName: strings.Repeat("玩", maxTextFieldLength),
		Message:    strings.Repeat("m", maxMessageLength+10),
	}
	if err := event.Normalize(time.Unix(10, 0)); err != nil {
		t.Fatal(err)
	}
	if len(event.EventType) != maxEventTypeLength {
		t.Fatalf("event type length = %d", len(event.EventType))
	}
	if len(event.Message) != maxMessageLength {
		t.Fatalf("message length = %d", len(event.Message))
	}
	if !utf8.ValidString(event.PlayerName) {
		t.Fatal("player name is not valid utf-8")
	}
	if len(event.PlayerName) > maxTextFieldLength {
		t.Fatalf("player name bytes = %d", len(event.PlayerName))
	}
}

func TestIngestEventNormalizeRejectsHugeMetadata(t *testing.T) {
	event := IngestEvent{
		EventType: "large_metadata",
		Metadata: map[string]any{
			"payload": strings.Repeat("x", maxMetadataBytes),
		},
	}
	if err := event.Normalize(time.Unix(10, 0)); err == nil {
		t.Fatal("expected large metadata to fail")
	}
}

func TestIngestEventNormalizeRejectsMissingEventType(t *testing.T) {
	event := IngestEvent{
		Message: "missing event type",
	}
	if err := event.Normalize(time.Unix(10, 0)); err == nil {
		t.Fatal("expected missing event type to fail")
	}
}

func TestPrepareInsertEventRowsSkipsInvalidEvents(t *testing.T) {
	source := 7
	rows, err := prepareInsertEventRows(42, []IngestEvent{
		{Message: "missing event type"},
		{
			EventType:  "door forced",
			Severity:   "warning",
			Source:     &source,
			PlayerName: "  Mei Lin  ",
			Coords:     &Coords{X: 1, Y: 2, Z: 3},
			Metadata:   map[string]any{"door_id": "bank-front"},
		},
	}, time.Unix(10, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows length = %d", len(rows))
	}
	row := rows[0]
	if row.ServerID != 42 || row.EventType != "door_forced" || row.Severity != "warning" {
		t.Fatalf("row = %+v", row)
	}
	if row.PlayerSource == nil || *row.PlayerSource != source {
		t.Fatalf("source = %v", row.PlayerSource)
	}
	if row.PlayerName == nil || *row.PlayerName != "Mei Lin" {
		t.Fatalf("player name = %v", row.PlayerName)
	}
	if row.CoordsX == nil || *row.CoordsX != 1 || row.CoordsY == nil || *row.CoordsY != 2 || row.CoordsZ == nil || *row.CoordsZ != 3 {
		t.Fatalf("coords = %v %v %v", row.CoordsX, row.CoordsY, row.CoordsZ)
	}
	if !json.Valid(row.Metadata) {
		t.Fatalf("metadata json = %s", row.Metadata)
	}
}

func TestPrepareInsertEventRowsRejectsAllInvalidEvents(t *testing.T) {
	_, err := prepareInsertEventRows(42, []IngestEvent{{Message: "missing event type"}}, time.Unix(10, 0).UTC())
	if !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("err = %v", err)
	}
}

func TestEventJSONUsesSnakeCase(t *testing.T) {
	event := Event{
		ID:         1,
		ServerID:   2,
		ServerName: "city",
		EventType:  "player_connecting",
		Severity:   "info",
		Message:    "joined",
		Metadata:   json.RawMessage(`{"ping":42}`),
		OccurredAt: time.Unix(10, 0).UTC(),
		CreatedAt:  time.Unix(11, 0).UTC(),
	}
	raw, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, ok := decoded["EventType"]; ok {
		t.Fatal("unexpected Go-style JSON key")
	}
	if decoded["event_type"] != "player_connecting" {
		t.Fatalf("event_type = %v", decoded["event_type"])
	}
	if decoded["server_name"] != "city" {
		t.Fatalf("server_name = %v", decoded["server_name"])
	}
}

func TestNewPage(t *testing.T) {
	page := NewPage(100, 200, 350)
	if !page.HasPrev || !page.HasNext {
		t.Fatalf("expected prev and next: %+v", page)
	}
	if page.Current != 3 || page.TotalPages != 4 {
		t.Fatalf("unexpected page numbers: %+v", page)
	}
	if page.PrevOffset != 100 || page.NextOffset != 300 {
		t.Fatalf("unexpected offsets: %+v", page)
	}
	if page.From != 201 || page.To != 300 {
		t.Fatalf("unexpected range: %+v", page)
	}

	page = NewPage(100, 300, 350)
	if !page.HasPrev || page.HasNext {
		t.Fatalf("expected last page: %+v", page)
	}
	if page.From != 301 || page.To != 350 {
		t.Fatalf("unexpected last range: %+v", page)
	}
}

func TestNewPageClampsPastEnd(t *testing.T) {
	page := NewPage(100, 900, 350)
	if page.Offset != 300 || page.Current != 4 || page.TotalPages != 4 {
		t.Fatalf("unexpected clamped page: %+v", page)
	}
	if page.From != 301 || page.To != 350 || page.HasNext {
		t.Fatalf("unexpected clamped range: %+v", page)
	}
}

func TestNewPageItems(t *testing.T) {
	page := NewPage(100, 500, 1200)
	var parts []string
	for _, item := range page.Items {
		if item.Gap {
			parts = append(parts, "...")
			continue
		}
		part := strconv.Itoa(item.Number)
		if item.Active {
			part += "*"
		}
		parts = append(parts, part)
	}
	if got := strings.Join(parts, ","); got != "1,...,4,5,6*,7,8,...,12" {
		t.Fatalf("items = %s", got)
	}
}

func TestNewPageDefaults(t *testing.T) {
	page := NewPage(0, -10, 0)
	if page.Limit != 100 || page.Offset != 0 {
		t.Fatalf("unexpected defaults: %+v", page)
	}
	if page.HasPrev || page.HasNext || page.From != 0 || page.To != 0 {
		t.Fatalf("unexpected empty page: %+v", page)
	}
}

func TestQueryWherePlayerClause(t *testing.T) {
	var args []any
	where := queryWhere(Query{Player: "license:abc", Limit: 50}, &args)
	if len(args) != 2 || args[0] != "%license:abc%" || args[1] != "license:abc" {
		t.Fatalf("args = %#v", args)
	}
	joined := strings.Join(where, " AND ")
	if !strings.Contains(joined, "(e.player_name ILIKE $1 OR e.license = $2 OR e.discord = $2 OR e.steam = $2 OR e.citizenid = $2)") {
		t.Fatalf("missing player clause: %s", joined)
	}
	if !strings.Contains(joined, "NOT EXISTS") || !strings.Contains(joined, "ar.archived_at IS NOT NULL") {
		t.Fatalf("missing default active archive filter: %s", joined)
	}
}

func TestQueryWhereKeywordSearchesMetadata(t *testing.T) {
	var args []any
	where := queryWhere(Query{Keyword: "markedbills"}, &args)
	joined := strings.Join(where, " AND ")
	if !strings.Contains(joined, "e.metadata::text ILIKE $1") {
		t.Fatalf("missing metadata keyword search: %s", joined)
	}
	if !strings.Contains(joined, "e.resource ILIKE $1") {
		t.Fatalf("missing resource keyword search: %s", joined)
	}
	if len(args) != 1 || args[0] != "%markedbills%" {
		t.Fatalf("args = %#v", args)
	}
}

func TestQueryWherePlayerNameSearchIsFuzzy(t *testing.T) {
	var args []any
	where := queryWhere(Query{Player: "Vance"}, &args)
	if len(args) != 2 || args[0] != "%Vance%" || args[1] != "Vance" {
		t.Fatalf("args = %#v", args)
	}
	joined := strings.Join(where, " AND ")
	if !strings.Contains(joined, "e.player_name ILIKE $1") {
		t.Fatalf("missing fuzzy player name search: %s", joined)
	}
}

func TestQueryWhereReviewAndArchiveFilters(t *testing.T) {
	var args []any
	where := queryWhere(Query{ReviewStatus: "suspicious", ArchiveMode: ArchiveArchivedOnly}, &args)
	joined := strings.Join(where, " AND ")
	if !strings.Contains(joined, "coalesce((SELECT rs.status FROM log_event_reviews rs WHERE rs.event_id = e.id), 'normal') = $1") {
		t.Fatalf("missing review status filter: %s", joined)
	}
	if !strings.Contains(joined, "EXISTS") || !strings.Contains(joined, "ar.archived_at IS NOT NULL") {
		t.Fatalf("missing archived-only filter: %s", joined)
	}
	if len(args) != 1 || args[0] != ReviewStatusSuspicious {
		t.Fatalf("args = %#v", args)
	}

	args = nil
	where = queryWhere(Query{ArchiveMode: ArchiveInclude}, &args)
	joined = strings.Join(where, " AND ")
	if strings.Contains(joined, "archived_at") {
		t.Fatalf("include archived should not add archive filter: %s", joined)
	}
}

func TestNormalizeReviewAndArchiveValues(t *testing.T) {
	if got := NormalizeReviewStatus(" VIOLATION "); got != ReviewStatusViolation {
		t.Fatalf("review status = %s", got)
	}
	if got := NormalizeReviewStatus("bad"); got != "" {
		t.Fatalf("invalid review status = %s", got)
	}
	if got := NormalizeArchiveMode("archived_only"); got != ArchiveArchivedOnly {
		t.Fatalf("archive mode = %s", got)
	}
	if got := NormalizeArchiveMode("bad"); got != ArchiveActiveOnly {
		t.Fatalf("default archive mode = %s", got)
	}
}

func TestQueryWhereGeoFilters(t *testing.T) {
	var args []any
	where := queryWhere(Query{WithCoords: true, Severity: "warning", Player: "discord:1"}, &args)
	joined := strings.Join(where, " AND ")
	if !strings.Contains(joined, "e.coords_x IS NOT NULL AND e.coords_y IS NOT NULL") {
		t.Fatalf("missing coords filter: %s", joined)
	}
	if len(args) != 3 || args[0] != "warning" || args[1] != "%discord:1%" || args[2] != "discord:1" {
		t.Fatalf("args = %#v", args)
	}
}

func TestQuerySQLPagesIDsBeforeDetailJoins(t *testing.T) {
	sql, args := querySQL(Query{EventType: "money_change", Limit: 50, Offset: 100})
	if !strings.Contains(sql, "WITH selected AS") {
		t.Fatalf("missing selected CTE: %s", sql)
	}
	if !strings.Contains(sql, "FROM log_events e\n\t\t\tWHERE") {
		t.Fatalf("selected query should start from log_events: %s", sql)
	}
	if !strings.Contains(sql, "JOIN log_events e ON e.id = selected.id") {
		t.Fatalf("missing detail join after selected ids: %s", sql)
	}
	if !strings.Contains(sql, "LIMIT $2 OFFSET $3") {
		t.Fatalf("missing limit/offset placeholders: %s", sql)
	}
	if len(args) != 3 || args[0] != "money_change" || args[1] != 50 || args[2] != 100 {
		t.Fatalf("args = %#v", args)
	}
}

func TestCountSQLDoesNotJoinDetailsForDefaultQuery(t *testing.T) {
	sql, args := countSQL(Query{Limit: 100})
	if strings.Contains(sql, "JOIN servers") || strings.Contains(sql, "LEFT JOIN log_event_reviews") {
		t.Fatalf("count should avoid detail joins: %s", sql)
	}
	if !strings.Contains(sql, "NOT EXISTS") {
		t.Fatalf("missing default active archive filter: %s", sql)
	}
	if len(args) != 0 {
		t.Fatalf("args = %#v", args)
	}
}
