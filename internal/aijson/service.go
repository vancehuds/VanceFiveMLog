package aijson

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var ErrAIUnavailable = errors.New("ai json client is not configured")

const defaultBaseURL = "https://api.openai.com/v1"

type AIClient struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
	model      string
}

type SuggestedMethod struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Source      string          `json:"source"`
	EventType   string          `json:"event_type"`
	Resource    string          `json:"resource"`
	Spec        json.RawMessage `json:"spec"`
}

func NewAIClient(baseURL, apiKey, model string) *AIClient {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &AIClient{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    baseURL,
		apiKey:     strings.TrimSpace(apiKey),
		model:      strings.TrimSpace(model),
	}
}

func (c *AIClient) Configured() bool {
	return c != nil && c.apiKey != "" && c.model != ""
}

func (c *AIClient) SuggestMethod(ctx context.Context, sample json.RawMessage, prompt string) (SuggestedMethod, error) {
	if !c.Configured() {
		return SuggestedMethod{}, ErrAIUnavailable
	}
	sample = bytes.TrimSpace(sample)
	if len(sample) == 0 || !json.Valid(sample) {
		return SuggestedMethod{}, ErrInvalidMethod
	}
	if len(sample) > 64<<10 {
		return SuggestedMethod{}, fmt.Errorf("%w: sample exceeds 65536 bytes", ErrInvalidMethod)
	}

	messages := []map[string]string{
		{
			"role": "system",
			"content": strings.Join([]string{
				"You design reusable JSON display methods for a FiveM/Qbox log console.",
				"Return one compact JSON object only.",
				"The object shape is {\"name\":\"...\",\"description\":\"...\",\"source\":\"metadata|event\",\"event_type\":\"optional\",\"resource\":\"optional\",\"spec\":{\"title\":\"...\",\"description\":\"...\",\"summary_path\":\"...\",\"summary_template\":\"... {path} ...\",\"badges\":[{\"label\":\"...\",\"path\":\"...\",\"paths\":[\"fallback.path\"],\"format\":\"text|number|currency|delta|time|date|clock|percent|duration|duration_s|coords|boolean|list|json\",\"tone\":\"info|success|warning|error|muted|auto\"}],\"metrics\":[{\"label\":\"...\",\"path\":\"...\",\"format\":\"number|currency|delta|percent|duration\",\"tone\":\"auto\"}],\"fields\":[{\"label\":\"...\",\"path\":\"...\",\"paths\":[\"fallback.path\"],\"format\":\"text|number|currency|delta|time|date|clock|percent|duration|coords|boolean|list|json\",\"span\":\"wide|full\",\"prefix\":\"...\",\"suffix\":\"...\",\"max_length\":120}],\"sections\":[{\"title\":\"...\",\"fields\":[...],\"badges\":[...]}],\"lists\":[{\"title\":\"...\",\"path\":\"array_or_object\",\"title_path\":\"label\",\"subtitle_path\":\"name\",\"badges\":[...],\"fields\":[...],\"limit\":20}],\"tables\":[{\"title\":\"...\",\"description\":\"...\",\"path\":\"array_or_object\",\"limit\":80,\"columns\":[{\"label\":\"...\",\"path\":\"...\",\"sub_path\":\"...\",\"format\":\"text|number|delta|json\",\"align\":\"right|center\",\"tone\":\"auto\"}]}],\"json_blocks\":[{\"title\":\"...\",\"path\":\"...\"}]}}.",
				"Use dot paths. [] is optional and treated as array traversal. For object maps, tables/lists automatically expose key and value columns. Prefer summary_template for human-readable summaries and use metrics/sections/lists/tables/json_blocks when the sample is rich. Keep labels concise Chinese when possible.",
			}, " "),
		},
		{
			"role":    "user",
			"content": strings.TrimSpace(prompt) + "\n\nSample JSON:\n" + string(sample),
		},
	}
	reqBody := map[string]any{
		"model":       c.model,
		"messages":    messages,
		"temperature": 0.1,
		"response_format": map[string]string{
			"type": "json_object",
		},
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return SuggestedMethod{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return SuggestedMethod{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return SuggestedMethod{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return SuggestedMethod{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return SuggestedMethod{}, fmt.Errorf("ai request failed: %s", strings.TrimSpace(string(raw)))
	}

	var payload struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return SuggestedMethod{}, err
	}
	if len(payload.Choices) == 0 || strings.TrimSpace(payload.Choices[0].Message.Content) == "" {
		return SuggestedMethod{}, errors.New("ai response is empty")
	}
	return parseSuggestion(payload.Choices[0].Message.Content)
}

func parseSuggestion(raw string) (SuggestedMethod, error) {
	raw = strings.TrimSpace(raw)
	var suggestion SuggestedMethod
	if err := json.Unmarshal([]byte(raw), &suggestion); err != nil {
		return SuggestedMethod{}, err
	}
	normalized, err := NormalizeInput(MethodInput{
		Name:        suggestion.Name,
		Description: suggestion.Description,
		Source:      suggestion.Source,
		EventType:   suggestion.EventType,
		Resource:    suggestion.Resource,
		Spec:        suggestion.Spec,
	})
	if err != nil {
		return SuggestedMethod{}, err
	}
	return SuggestedMethod{
		Name:        normalized.Name,
		Description: normalized.Description,
		Source:      normalized.Source,
		EventType:   normalized.EventType,
		Resource:    normalized.Resource,
		Spec:        normalized.Spec,
	}, nil
}
