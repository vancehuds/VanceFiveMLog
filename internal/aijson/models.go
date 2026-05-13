package aijson

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var ErrInvalidMethod = errors.New("invalid ai json method")

const (
	SourceMetadata = "metadata"
	SourceEvent    = "event"
)

const (
	maxNameBytes        = 80
	maxDescriptionBytes = 512
	maxScopeBytes       = 128
	maxPromptBytes      = 8192
	maxSpecBytes        = 64 << 10
)

type Method struct {
	ID            int64           `json:"id"`
	Name          string          `json:"name"`
	Description   string          `json:"description,omitempty"`
	Source        string          `json:"source"`
	EventType     string          `json:"event_type,omitempty"`
	Resource      string          `json:"resource,omitempty"`
	Prompt        string          `json:"-"`
	Spec          json.RawMessage `json:"spec"`
	Active        bool            `json:"active"`
	CreatedBy     *int64          `json:"-"`
	CreatedByName string          `json:"created_by_name,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

type MethodInput struct {
	Name        string
	Description string
	Source      string
	EventType   string
	Resource    string
	Prompt      string
	Spec        json.RawMessage
}

func NormalizeSource(source string) string {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case SourceEvent:
		return SourceEvent
	default:
		return SourceMetadata
	}
}

func NormalizeInput(input MethodInput) (MethodInput, error) {
	input.Name = limitBytes(strings.TrimSpace(input.Name), maxNameBytes)
	input.Description = limitBytes(strings.TrimSpace(input.Description), maxDescriptionBytes)
	input.Source = NormalizeSource(input.Source)
	input.EventType = limitBytes(strings.TrimSpace(input.EventType), maxScopeBytes)
	input.Resource = limitBytes(strings.TrimSpace(input.Resource), maxScopeBytes)
	input.Prompt = limitBytes(strings.TrimSpace(input.Prompt), maxPromptBytes)
	input.Spec = compactJSONObject(input.Spec)
	if input.Name == "" || len(input.Spec) == 0 {
		return MethodInput{}, ErrInvalidMethod
	}
	return input, nil
}

func compactJSONObject(raw json.RawMessage) json.RawMessage {
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || len(raw) > maxSpecBytes {
		return nil
	}
	var value map[string]any
	decErr := json.Unmarshal(raw, &value)
	if decErr != nil || value == nil {
		return nil
	}
	out, err := json.Marshal(value)
	if err != nil || len(out) > maxSpecBytes {
		return nil
	}
	return out
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
