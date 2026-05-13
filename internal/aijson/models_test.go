package aijson

import (
	"encoding/json"
	"testing"
)

func TestNormalizeInputCompactsSpec(t *testing.T) {
	input, err := NormalizeInput(MethodInput{
		Name:   "  inventory  ",
		Source: "event",
		Spec:   json.RawMessage(`{ "title": "Inventory", "fields": [] }`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if input.Name != "inventory" || input.Source != SourceEvent {
		t.Fatalf("input = %+v", input)
	}
	if string(input.Spec) != `{"fields":[],"title":"Inventory"}` {
		t.Fatalf("spec = %s", input.Spec)
	}
}

func TestNormalizeInputRejectsInvalidSpec(t *testing.T) {
	if _, err := NormalizeInput(MethodInput{Name: "bad", Spec: json.RawMessage(`[]`)}); err == nil {
		t.Fatal("expected invalid spec")
	}
}

func TestNormalizeInputAllowsDetailedSpec(t *testing.T) {
	raw := json.RawMessage(`{
		"title": "Inventory",
		"summary_template": "{action} {item}",
		"badges": [{"label": "状态", "path": "status", "tone": "auto"}],
		"metrics": [{"label": "变化", "path": "delta", "format": "delta"}],
		"fields": [{"label": "原因", "path": "reason", "span": "wide"}],
		"sections": [{"title": "上下文", "fields": [{"label": "职业", "path": "job"}]}],
		"lists": [{"title": "变化列表", "path": "changes", "title_path": "label"}],
		"tables": [{"title": "明细", "path": "changes", "columns": [{"label": "Delta", "path": "delta", "format": "delta"}]}],
		"json_blocks": [{"title": "原始", "path": ""}]
	}`)
	input, err := NormalizeInput(MethodInput{Name: "detailed", Spec: raw})
	if err != nil {
		t.Fatal(err)
	}
	var spec map[string]any
	if err := json.Unmarshal(input.Spec, &spec); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"metrics", "sections", "lists", "json_blocks", "summary_template"} {
		if _, ok := spec[key]; !ok {
			t.Fatalf("missing %s in compacted spec: %s", key, input.Spec)
		}
	}
}

func TestParseSuggestionNormalizesSource(t *testing.T) {
	suggestion, err := parseSuggestion(`{
		"name": "money",
		"source": "full",
		"spec": {"title": "Money", "fields": [{"label": "金额", "path": "amount"}]}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if suggestion.Source != SourceMetadata {
		t.Fatalf("source = %s", suggestion.Source)
	}
}
