package logs

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

const (
	maxEventTypeLength = 128
	maxTextFieldLength = 512
	maxMessageLength   = 4096
	maxMetadataBytes   = 64 << 10
)

const (
	ReviewStatusNormal     = "normal"
	ReviewStatusSuspicious = "suspicious"
	ReviewStatusViolation  = "violation"
)

const (
	ArchiveActiveOnly   = "active"
	ArchiveInclude      = "include_archived"
	ArchiveArchivedOnly = "archived_only"
)

type Event struct {
	ID           int64           `json:"id"`
	ServerID     int64           `json:"server_id"`
	ServerName   string          `json:"server_name"`
	EventType    string          `json:"event_type"`
	Severity     string          `json:"severity"`
	PlayerSource *int            `json:"player_source,omitempty"`
	PlayerName   string          `json:"player_name,omitempty"`
	License      string          `json:"license,omitempty"`
	Discord      string          `json:"discord,omitempty"`
	Steam        string          `json:"steam,omitempty"`
	CitizenID    string          `json:"citizenid,omitempty"`
	Resource     string          `json:"resource,omitempty"`
	Message      string          `json:"message"`
	CoordsX      *float64        `json:"coords_x,omitempty"`
	CoordsY      *float64        `json:"coords_y,omitempty"`
	CoordsZ      *float64        `json:"coords_z,omitempty"`
	Metadata     json.RawMessage `json:"metadata"`
	OccurredAt   time.Time       `json:"occurred_at"`
	CreatedAt    time.Time       `json:"created_at"`
	Review       EventReview     `json:"review"`
}

type EventReview struct {
	Status         string     `json:"status"`
	Note           string     `json:"note,omitempty"`
	ArchivedAt     *time.Time `json:"archived_at,omitempty"`
	ArchivedBy     *int64     `json:"archived_by,omitempty"`
	UpdatedBy      *int64     `json:"updated_by,omitempty"`
	UpdatedAt      *time.Time `json:"updated_at,omitempty"`
	ArchivedByName string     `json:"archived_by_name,omitempty"`
	UpdatedByName  string     `json:"updated_by_name,omitempty"`
}

type Coords struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

type IngestEvent struct {
	EventType      string         `json:"event_type"`
	Type           string         `json:"type"`
	Event          string         `json:"event"`
	Severity       string         `json:"severity"`
	Level          string         `json:"level"`
	Source         *int           `json:"source"`
	PlayerSource   *int           `json:"player_source"`
	PlayerName     string         `json:"player_name"`
	License        string         `json:"license"`
	Discord        string         `json:"discord"`
	Steam          string         `json:"steam"`
	CitizenID      string         `json:"citizenid"`
	CharacterName  string         `json:"character_name"`
	Job            string         `json:"job"`
	Gang           string         `json:"gang"`
	Resource       string         `json:"resource"`
	Plugin         string         `json:"plugin"`
	PluginResource string         `json:"plugin_resource"`
	Message        string         `json:"message"`
	Coords         *Coords        `json:"coords"`
	Metadata       map[string]any `json:"metadata"`
	Data           any            `json:"data"`
	OccurredAt     *time.Time     `json:"occurred_at"`
}

func (e *IngestEvent) UnmarshalJSON(data []byte) error {
	var payload struct {
		EventType      json.RawMessage `json:"event_type"`
		Type           json.RawMessage `json:"type"`
		Event          json.RawMessage `json:"event"`
		Severity       json.RawMessage `json:"severity"`
		Level          json.RawMessage `json:"level"`
		Source         json.RawMessage `json:"source"`
		PlayerSource   json.RawMessage `json:"player_source"`
		PlayerName     json.RawMessage `json:"player_name"`
		License        json.RawMessage `json:"license"`
		Discord        json.RawMessage `json:"discord"`
		Steam          json.RawMessage `json:"steam"`
		CitizenID      json.RawMessage `json:"citizenid"`
		CharacterName  json.RawMessage `json:"character_name"`
		CharName       json.RawMessage `json:"char_name"`
		Job            json.RawMessage `json:"job"`
		Gang           json.RawMessage `json:"gang"`
		Resource       json.RawMessage `json:"resource"`
		Plugin         json.RawMessage `json:"plugin"`
		PluginResource json.RawMessage `json:"plugin_resource"`
		Message        json.RawMessage `json:"message"`
		Coords         json.RawMessage `json:"coords"`
		Metadata       json.RawMessage `json:"metadata"`
		Data           any             `json:"data"`
		OccurredAt     json.RawMessage `json:"occurred_at"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}

	*e = IngestEvent{
		EventType:      optionalString(payload.EventType),
		Type:           optionalString(payload.Type),
		Event:          optionalString(payload.Event),
		Severity:       optionalString(payload.Severity),
		Level:          optionalString(payload.Level),
		Source:         optionalInt(payload.Source),
		PlayerSource:   optionalInt(payload.PlayerSource),
		PlayerName:     optionalString(payload.PlayerName),
		License:        optionalString(payload.License),
		Discord:        optionalString(payload.Discord),
		Steam:          optionalString(payload.Steam),
		CitizenID:      optionalString(payload.CitizenID),
		CharacterName:  firstNonEmpty(optionalString(payload.CharacterName), optionalString(payload.CharName)),
		Job:            optionalString(payload.Job),
		Gang:           optionalString(payload.Gang),
		Resource:       optionalString(payload.Resource),
		Plugin:         optionalString(payload.Plugin),
		PluginResource: optionalString(payload.PluginResource),
		Message:        optionalString(payload.Message),
		Coords:         optionalCoords(payload.Coords),
		Metadata:       optionalMetadata(payload.Metadata),
		Data:           payload.Data,
		OccurredAt:     optionalTime(payload.OccurredAt),
	}
	return nil
}

type Query struct {
	ServerID     int64
	EventType    string
	Severity     string
	Player       string
	Resource     string
	Keyword      string
	Metadata     string
	ReviewStatus string
	ArchiveMode  string
	Since        *time.Time
	Until        *time.Time
	WithCoords   bool
	Limit        int
	Offset       int
}

type Page struct {
	Limit      int
	Offset     int
	Total      int64
	Current    int
	TotalPages int
	Items      []PageItem
	PrevOffset int
	NextOffset int
	HasPrev    bool
	HasNext    bool
	From       int64
	To         int64
}

type PageItem struct {
	Number int
	Offset int
	Active bool
	Gap    bool
}

type HourBucket struct {
	Hour   time.Time
	Total  int64
	Errors int64
}

type EventTypeCount struct {
	EventType string
	Severity  string
	Total     int64
}

type AdminAuditEntry struct {
	AdminID       int64
	AdminUsername string
	Action        string
	EventID       int64
	EventIDs      []int64
	Query         map[string]string
	Details       map[string]any
}

func (e *IngestEvent) Normalize(now time.Time) error {
	if e.EventType == "" {
		e.EventType = e.Type
	}
	if e.EventType == "" {
		e.EventType = e.Event
	}
	if e.Severity == "" {
		e.Severity = e.Level
	}
	if e.Source == nil {
		e.Source = e.PlayerSource
	}
	if e.Resource == "" {
		if e.PluginResource != "" {
			e.Resource = e.PluginResource
		} else {
			e.Resource = e.Plugin
		}
	}
	e.EventType = clean(e.EventType)
	e.Severity = strings.ToLower(clean(e.Severity))
	if e.Severity == "" {
		e.Severity = "info"
	}
	switch e.Severity {
	case "info", "success", "warning", "error":
	default:
		e.Severity = "info"
	}
	if e.Message == "" {
		e.Message = e.EventType
	}
	e.EventType = truncateString(e.EventType, maxEventTypeLength)
	e.PlayerName = truncateString(strings.TrimSpace(e.PlayerName), maxTextFieldLength)
	e.License = truncateString(strings.TrimSpace(e.License), maxTextFieldLength)
	e.Discord = truncateString(strings.TrimSpace(e.Discord), maxTextFieldLength)
	e.Steam = truncateString(strings.TrimSpace(e.Steam), maxTextFieldLength)
	e.CitizenID = truncateString(strings.TrimSpace(e.CitizenID), maxTextFieldLength)
	e.CharacterName = truncateString(strings.TrimSpace(e.CharacterName), maxTextFieldLength)
	e.Job = truncateString(strings.TrimSpace(e.Job), maxTextFieldLength)
	e.Gang = truncateString(strings.TrimSpace(e.Gang), maxTextFieldLength)
	e.Resource = truncateString(strings.TrimSpace(e.Resource), maxTextFieldLength)
	e.Message = truncateString(strings.TrimSpace(e.Message), maxMessageLength)
	if e.Metadata == nil {
		e.Metadata = map[string]any{}
	}
	if e.CharacterName == "" {
		e.CharacterName = metadataString(e.Metadata, "character_name", "characterName", "char_name", "charName")
	}
	if e.Job == "" {
		e.Job = metadataString(e.Metadata, "job")
	}
	if e.Gang == "" {
		e.Gang = metadataString(e.Metadata, "gang")
	}
	e.CharacterName = truncateString(strings.TrimSpace(e.CharacterName), maxTextFieldLength)
	e.Job = truncateString(strings.TrimSpace(e.Job), maxTextFieldLength)
	e.Gang = truncateString(strings.TrimSpace(e.Gang), maxTextFieldLength)
	if e.CharacterName != "" {
		if _, exists := e.Metadata["character_name"]; !exists {
			e.Metadata["character_name"] = e.CharacterName
		}
	}
	if e.Job != "" {
		if _, exists := e.Metadata["job"]; !exists {
			e.Metadata["job"] = e.Job
		}
	}
	if e.Gang != "" {
		if _, exists := e.Metadata["gang"]; !exists {
			e.Metadata["gang"] = e.Gang
		}
	}
	switch data := e.Data.(type) {
	case nil:
	case map[string]any:
		for key, value := range data {
			if _, exists := e.Metadata[key]; !exists {
				e.Metadata[key] = value
			}
		}
	default:
		if _, exists := e.Metadata["data"]; !exists {
			e.Metadata["data"] = data
		}
	}
	if e.PluginResource != "" {
		if _, exists := e.Metadata["plugin_resource"]; !exists {
			e.Metadata["plugin_resource"] = strings.TrimSpace(e.PluginResource)
		}
	} else if e.Plugin != "" {
		if _, exists := e.Metadata["plugin_resource"]; !exists {
			e.Metadata["plugin_resource"] = strings.TrimSpace(e.Plugin)
		}
	}
	if e.OccurredAt == nil {
		t := now
		e.OccurredAt = &t
	}
	if e.EventType == "" {
		return ErrInvalidEvent
	}
	if meta, err := json.Marshal(e.Metadata); err != nil {
		return err
	} else if len(meta) > maxMetadataBytes {
		return fmt.Errorf("%w: metadata exceeds %d bytes", ErrInvalidEvent, maxMetadataBytes)
	}
	return nil
}

func clean(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, " ", "_")
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func metadataString(metadata map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := metadata[key]; ok {
			switch typed := value.(type) {
			case string:
				if strings.TrimSpace(typed) != "" {
					return typed
				}
			case json.Number:
				return typed.String()
			case float64:
				if !math.IsNaN(typed) && !math.IsInf(typed, 0) {
					return strconv.FormatFloat(typed, 'f', -1, 64)
				}
			case bool:
				return strconv.FormatBool(typed)
			}
		}
	}
	return ""
}

func truncateString(value string, max int) string {
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

func optionalString(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err == nil {
		return value
	}
	var number json.Number
	if err := json.Unmarshal(raw, &number); err == nil {
		return number.String()
	}
	var boolean bool
	if err := json.Unmarshal(raw, &boolean); err == nil {
		return strconv.FormatBool(boolean)
	}
	return ""
}

func optionalInt(raw json.RawMessage) *int {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var number json.Number
	if err := json.Unmarshal(raw, &number); err == nil {
		if value, ok := intFromString(number.String()); ok {
			return &value
		}
	}
	var value string
	if err := json.Unmarshal(raw, &value); err == nil {
		if parsed, ok := intFromString(strings.TrimSpace(value)); ok {
			return &parsed
		}
	}
	return nil
}

func optionalCoords(raw json.RawMessage) *Coords {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var object struct {
		X json.RawMessage `json:"x"`
		Y json.RawMessage `json:"y"`
		Z json.RawMessage `json:"z"`
	}
	if err := json.Unmarshal(raw, &object); err == nil {
		if x, ok := optionalFloat(object.X); ok {
			if y, ok := optionalFloat(object.Y); ok {
				if z, ok := optionalFloat(object.Z); ok {
					return &Coords{X: x, Y: y, Z: z}
				}
			}
		}
	}
	var array []json.RawMessage
	if err := json.Unmarshal(raw, &array); err == nil && len(array) >= 3 {
		if x, ok := optionalFloat(array[0]); ok {
			if y, ok := optionalFloat(array[1]); ok {
				if z, ok := optionalFloat(array[2]); ok {
					return &Coords{X: x, Y: y, Z: z}
				}
			}
		}
	}
	return nil
}

func optionalFloat(raw json.RawMessage) (float64, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, false
	}
	var value float64
	if err := json.Unmarshal(raw, &value); err == nil && !math.IsNaN(value) && !math.IsInf(value, 0) {
		return value, true
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		value, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
		if err == nil && !math.IsNaN(value) && !math.IsInf(value, 0) {
			return value, true
		}
	}
	return 0, false
}

func optionalMetadata(raw json.RawMessage) map[string]any {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var metadata map[string]any
	if err := json.Unmarshal(raw, &metadata); err == nil {
		return metadata
	}
	var value any
	if err := json.Unmarshal(raw, &value); err == nil && value != nil {
		return map[string]any{"data": value}
	}
	return nil
}

func optionalTime(raw json.RawMessage) *time.Time {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err == nil {
		parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(value))
		if err == nil {
			parsed = parsed.UTC()
			return &parsed
		}
		return nil
	}
	var number json.Number
	if err := json.Unmarshal(raw, &number); err == nil {
		seconds, err := strconv.ParseFloat(number.String(), 64)
		if err == nil && !math.IsNaN(seconds) && !math.IsInf(seconds, 0) {
			whole, fraction := math.Modf(seconds)
			parsed := time.Unix(int64(whole), int64(fraction*1e9)).UTC()
			return &parsed
		}
	}
	return nil
}

func intFromString(value string) (int, bool) {
	if value == "" {
		return 0, false
	}
	parsed, err := strconv.ParseInt(value, 10, 0)
	if err == nil {
		return int(parsed), true
	}
	floatValue, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(floatValue) || math.IsInf(floatValue, 0) || math.Trunc(floatValue) != floatValue {
		return 0, false
	}
	if floatValue > float64(maxInt()) || floatValue < float64(minInt()) {
		return 0, false
	}
	return int(floatValue), true
}

func maxInt() int {
	return int(^uint(0) >> 1)
}

func minInt() int {
	return -maxInt() - 1
}
