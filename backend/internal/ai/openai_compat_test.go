package ai

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func fakeChatServer(t *testing.T, status int, reply string, inspect func(map[string]any, *http.Request)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("request is not JSON: %v", err)
		}
		if inspect != nil {
			inspect(body, r)
		}
		w.WriteHeader(status)
		io.WriteString(w, reply)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func request() StructuredRequest {
	return StructuredRequest{
		Instructions:    "You file photos.",
		Prompt:          "Fill in the form.",
		Images:          []Image{{Data: []byte{0xff, 0xd8, 0xff}, MIMEType: "image/jpeg"}},
		Schema:          map[string]any{"type": "object", "additionalProperties": false},
		MaxOutputTokens: 512,
	}
}

func TestOpenAICompatibleRequestAndCost(t *testing.T) {
	srv := fakeChatServer(t, 200, `{"choices":[{"message":{"content":"{\"role\":\"content\"}"},"finish_reason":"stop"}],
		"usage":{"prompt_tokens":1200,"completion_tokens":80,"cost":0.000151}}`,
		func(body map[string]any, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer sk-test" {
				t.Errorf("auth header %q", r.Header.Get("Authorization"))
			}
			if r.Header.Get("X-Title") != "Pidex" {
				t.Errorf("missing attribution header")
			}
			if body["model"] != "google/gemini-2.5-flash-lite" {
				t.Errorf("model %v", body["model"])
			}
			rf := body["response_format"].(map[string]any)
			js := rf["json_schema"].(map[string]any)
			if rf["type"] != "json_schema" || js["strict"] != true || js["schema"] == nil {
				t.Errorf("response_format %v", rf)
			}
			if body["usage"].(map[string]any)["include"] != true {
				t.Error("cost reporting not requested")
			}
			msgs := body["messages"].([]any)
			if msgs[0].(map[string]any)["role"] != "system" {
				t.Error("instructions must be the system message")
			}
			parts := msgs[1].(map[string]any)["content"].([]any)
			url := parts[0].(map[string]any)["image_url"].(map[string]any)["url"].(string)
			if !strings.HasPrefix(url, "data:image/jpeg;base64,/9j/") {
				t.Errorf("image not sent as a data URL: %.40s", url)
			}
		})

	m := &OpenAICompatible{Name: "openrouter", BaseURL: srv.URL, APIKey: "sk-test", ModelID: "google/gemini-2.5-flash-lite",
		ReportCost: true, Headers: map[string]string{"X-Title": "Pidex"}}
	out, usage, err := m.Structured(context.Background(), request())
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `{"role":"content"}` {
		t.Errorf("content %s", out)
	}
	if usage.InputTokens != 1200 || usage.OutputTokens != 80 || usage.CostUSD != 0.000151 {
		t.Errorf("usage %+v", usage)
	}
}

func TestOpenAICompatibleFallsBackToPriceTable(t *testing.T) {
	srv := fakeChatServer(t, 200, `{"choices":[{"message":{"content":"{}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1000000,"completion_tokens":0}}`, nil)
	m := &OpenAICompatible{Name: "x", BaseURL: srv.URL, ModelID: "m", Price: Price{InputPerM: 0.10}}
	_, usage, err := m.Structured(context.Background(), request())
	if err != nil {
		t.Fatal(err)
	}
	if usage.CostUSD != 0.10 {
		t.Errorf("cost %v, want 0.10 from the price table", usage.CostUSD)
	}
}

func TestOpenAICompatibleErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		reply  string
		want   string
	}{
		"http error":   {402, `{"error":{"message":"Insufficient credits","code":402}}`, "Insufficient credits"},
		"upstream":     {429, `{"error":{"message":"Provider returned error","code":429,"metadata":{"provider_name":"Google AI Studio","raw":"rate-limited upstream"}}}`, "from Google AI Studio: \"rate-limited upstream\""},
		"not json":     {502, `<html>bad gateway</html>`, "HTTP 502"},
		"refusal":      {200, `{"choices":[{"message":{"content":"","refusal":"cannot help"},"finish_reason":"stop"}]}`, "refused"},
		"truncated":    {200, `{"choices":[{"message":{"content":"{\"role\":"},"finish_reason":"length"}]}`, "truncated"},
		"empty answer": {200, `{"choices":[{"message":{"content":"  "},"finish_reason":"stop"}]}`, "empty"},
	} {
		t.Run(name, func(t *testing.T) {
			srv := fakeChatServer(t, tc.status, tc.reply, nil)
			m := &OpenAICompatible{Name: "openrouter", BaseURL: srv.URL, ModelID: "m"}
			_, _, err := m.Structured(context.Background(), request())
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

// TestRouterFallsThrough checks that a failing first model hands over to
// the next, and that both attempts are metered.
func TestRouterFallsThrough(t *testing.T) {
	bad := fakeChatServer(t, 503, `{"error":{"message":"overloaded"}}`, nil)
	good := fakeChatServer(t, 200, `{"choices":[{"message":{"content":"{\"ok\":true}"},"finish_reason":"stop"}]}`, nil)
	var recorded []CallRecord
	r := NewRouter(RecorderFunc(func(_ context.Context, rec CallRecord) { recorded = append(recorded, rec) }),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		&OpenAICompatible{Name: "openrouter", BaseURL: bad.URL, ModelID: "primary"},
		&OpenAICompatible{Name: "openrouter", BaseURL: good.URL, ModelID: "backup"},
	)
	out, usage, err := r.Structured(context.Background(), CallMeta{Purpose: "test"}, request())
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `{"ok":true}` || usage.Model != "backup" {
		t.Errorf("out=%s model=%s", out, usage.Model)
	}
	if len(recorded) != 2 || recorded[0].Err == nil || recorded[1].Err != nil {
		t.Errorf("want a failed and a successful call recorded, got %+v", recorded)
	}
}

func TestEmbedTextsKeepsOrderAndChecksSize(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if r.URL.Path != "/embeddings" || body["dimensions"] == nil || len(body["input"].([]any)) != 2 {
			t.Errorf("request %s %v", r.URL.Path, body)
		}
		io.WriteString(w, `{"data":[{"index":1,"embedding":[0,1,0]},{"index":0,"embedding":[1,0,0]}],"usage":{"prompt_tokens":7,"cost":0.0000001}}`)
	}))
	defer srv.Close()
	m := &OpenAICompatible{Name: "openrouter", BaseURL: srv.URL, ModelID: "e", EmbeddingDimensions: 3}
	vecs, usage, err := m.EmbedTexts(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	if vecs[0][0] != 1 || vecs[1][1] != 1 || usage.InputTokens != 7 || usage.CostUSD != 0.0000001 {
		t.Errorf("vecs %v usage %+v", vecs, usage)
	}
	m.EmbeddingDimensions = 4
	if _, _, err := m.EmbedTexts(context.Background(), []string{"a", "b"}); err == nil || !strings.Contains(err.Error(), "want 4") {
		t.Errorf("a wrong vector size must fail, got %v", err)
	}
}
