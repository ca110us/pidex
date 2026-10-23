package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenRouterBaseURL is OpenRouter's OpenAI-compatible endpoint.
const OpenRouterBaseURL = "https://openrouter.ai/api/v1"

// OpenAICompatible calls any chat-completions API in OpenAI's format:
// OpenRouter, OpenAI itself, or a self-hosted gateway. Through OpenRouter
// one key reaches Gemini, Claude, GPT and others, which suits development
// and model comparisons. For regulated data, use a provider endpoint covered
// by the required agreements (e.g. a HIPAA BAA) instead.
type OpenAICompatible struct {
	Name    string // provider label for metering, e.g. "openrouter"
	BaseURL string
	APIKey  string
	ModelID string
	// Price is used when the response carries no cost of its own.
	Price Price
	// ReportCost asks OpenRouter to include the billed cost in usage.
	ReportCost bool
	// Headers are sent with every request (OpenRouter reads HTTP-Referer
	// and X-Title for attribution).
	Headers map[string]string
	HTTP    *http.Client
	// EmbeddingDimensions is the vector size asked of embedding models; 0
	// keeps the model's own.
	EmbeddingDimensions int
}

func (o *OpenAICompatible) Provider() string { return o.Name }
func (o *OpenAICompatible) Model() string    { return o.ModelID }

type chatContent struct {
	Type     string        `json:"type"`
	Text     string        `json:"text,omitempty"`
	ImageURL *chatImageURL `json:"image_url,omitempty"`
}

type chatImageURL struct {
	URL string `json:"url"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type chatRequest struct {
	Model          string         `json:"model"`
	Messages       []chatMessage  `json:"messages"`
	Temperature    float64        `json:"temperature"`
	MaxTokens      int            `json:"max_tokens,omitempty"`
	ResponseFormat map[string]any `json:"response_format"`
	Usage          map[string]any `json:"usage,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
			Refusal string `json:"refusal"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int      `json:"prompt_tokens"`
		CompletionTokens int      `json:"completion_tokens"`
		Cost             *float64 `json:"cost"`
	} `json:"usage"`
	Error *apiError `json:"error"`
}

type apiError struct {
	Message string `json:"message"`
	Code    any    `json:"code"`
	// OpenRouter wraps upstream failures as "Provider returned error" and
	// puts the upstream's own answer here.
	Metadata struct {
		ProviderName string          `json:"provider_name"`
		Raw          json.RawMessage `json:"raw"`
	} `json:"metadata"`
}

func (o *OpenAICompatible) apiErr(e *apiError) error {
	msg := fmt.Sprintf("%s: %s (code %v)", o.Name, e.Message, e.Code)
	if e.Metadata.ProviderName != "" {
		msg += " from " + e.Metadata.ProviderName
	}
	if len(e.Metadata.Raw) > 0 {
		msg += ": " + snippet(e.Metadata.Raw)
	}
	return errors.New(msg)
}

// post sends a JSON request and reads the whole answer. Latency covers the
// body: OpenRouter sends headers at once and keeps the connection alive
// with whitespace while the model works.
func (o *OpenAICompatible) post(ctx context.Context, path string, body any) (raw []byte, status int, latency time.Duration, err error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, 0, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(o.BaseURL, "/")+path, bytes.NewReader(payload))
	if err != nil {
		return nil, 0, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.APIKey)
	for k, v := range o.Headers {
		req.Header.Set(k, v)
	}
	client := o.HTTP
	if client == nil {
		client = &http.Client{Timeout: 90 * time.Second}
	}
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, time.Since(start), err
	}
	defer resp.Body.Close()
	raw, err = io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	return raw, resp.StatusCode, time.Since(start), err
}

func (o *OpenAICompatible) Structured(ctx context.Context, req StructuredRequest) (json.RawMessage, Usage, error) {
	user := make([]chatContent, 0, len(req.Images)+1)
	for _, img := range req.Images {
		url := "data:" + img.MIMEType + ";base64," + base64.StdEncoding.EncodeToString(img.Data)
		user = append(user, chatContent{Type: "image_url", ImageURL: &chatImageURL{URL: url}})
	}
	user = append(user, chatContent{Type: "text", Text: req.Prompt})
	var msgs []chatMessage
	if req.Instructions != "" {
		msgs = append(msgs, chatMessage{Role: "system", Content: req.Instructions})
	}
	msgs = append(msgs, chatMessage{Role: "user", Content: user})

	body := chatRequest{
		Model:       o.ModelID,
		Messages:    msgs,
		Temperature: 0,
		MaxTokens:   req.MaxOutputTokens,
		ResponseFormat: map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   "photo_analysis",
				"strict": true,
				"schema": req.Schema,
			},
		},
	}
	if o.ReportCost {
		body.Usage = map[string]any{"include": true}
	}
	raw, status, latency, err := o.post(ctx, "/chat/completions", body)
	usage := Usage{Latency: latency}
	if err != nil {
		return nil, usage, err
	}
	var out chatResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, usage, fmt.Errorf("%s: HTTP %d: %s", o.Name, status, snippet(raw))
	}
	if out.Usage != nil {
		usage.InputTokens, usage.OutputTokens = out.Usage.PromptTokens, out.Usage.CompletionTokens
		if out.Usage.Cost != nil {
			usage.CostUSD = *out.Usage.Cost
		} else {
			usage.CostUSD = o.Price.Cost(usage.InputTokens, usage.OutputTokens)
		}
	}
	switch {
	case out.Error != nil:
		return nil, usage, o.apiErr(out.Error)
	case status != http.StatusOK:
		return nil, usage, fmt.Errorf("%s: HTTP %d: %s", o.Name, status, snippet(raw))
	case len(out.Choices) == 0:
		return nil, usage, fmt.Errorf("%s: no choices in response", o.Name)
	}
	msg := out.Choices[0].Message
	if msg.Refusal != "" {
		return nil, usage, fmt.Errorf("%s: model refused: %s", o.Name, msg.Refusal)
	}
	if out.Choices[0].FinishReason == "length" {
		return nil, usage, fmt.Errorf("%s: response truncated at max tokens", o.Name)
	}
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return nil, usage, errors.New(o.Name + ": empty response")
	}
	return json.RawMessage(content), usage, nil
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 300 {
		s = s[:300] + "…"
	}
	return s
}
