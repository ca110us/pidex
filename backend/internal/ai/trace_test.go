package ai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"pidex/internal/tracing"
)

func spans(t *testing.T, content bool) *tracetest.InMemoryExporter {
	t.Helper()
	exp := tracetest.NewInMemoryExporter()
	tracing.Use(sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp)), content)
	t.Cleanup(func() { tracing.Use(sdktrace.NewTracerProvider(), false) })
	return exp
}

func attrs(s tracetest.SpanStub) map[string]string {
	out := map[string]string{}
	for _, kv := range s.Attributes {
		out[string(kv.Key)] = kv.Value.Emit()
	}
	return out
}

func TestRouterTracesEachAttemptInItsTurn(t *testing.T) {
	exp := spans(t, true)
	failing := &Scripted{Name: "first", Fn: func(context.Context, StructuredRequest) (json.RawMessage, error) {
		return nil, errors.New("overloaded")
	}}
	working := &Scripted{Name: "second", Fn: func(context.Context, StructuredRequest) (json.RawMessage, error) {
		return json.RawMessage(`{"subject":"Tutu"}`), nil
	}}
	r := NewRouter(nil, slog.New(slog.NewTextHandler(io.Discard, nil)), failing, working)

	ctx, root := tracing.Start(context.Background(), "chat turn", tracing.Task)
	tracing.Allow(ctx, true)
	team := uuid.New()
	if _, _, err := r.Structured(ctx, CallMeta{TeamID: team, Purpose: "search_plan"}, request()); err != nil {
		t.Fatal(err)
	}
	root.End(nil)

	got := exp.GetSpans()
	if len(got) != 3 {
		t.Fatalf("got %d spans, want two attempts and the turn", len(got))
	}
	first, second, turn := got[0], got[1], got[2]
	for _, s := range []tracetest.SpanStub{first, second} {
		if s.Name != "search_plan" || s.Parent.SpanID() != turn.SpanContext.SpanID() {
			t.Errorf("span %q is not a call within the turn", s.Name)
		}
		a := attrs(s)
		if a["braintrust.span_attributes"] != `{"type":"llm"}` {
			t.Errorf("span type = %s", a["braintrust.span_attributes"])
		}
		if !strings.Contains(a["braintrust.metadata"], team.String()) {
			t.Errorf("metadata %s does not name the team", a["braintrust.metadata"])
		}
		// The photo is named, never sent.
		in := a["braintrust.input_json"]
		if !strings.Contains(in, "You file photos.") || !strings.Contains(in, "[image 1: image/jpeg, 0 KB]") {
			t.Errorf("input = %s", in)
		}
	}
	if first.Status.Description != "overloaded" || attrs(first)["gen_ai.request.model"] != "first" {
		t.Errorf("the failed attempt is not marked: %+v", first.Status)
	}
	if out := attrs(second)["braintrust.output_json"]; out != `{"subject":"Tutu"}` {
		t.Errorf("output = %s", out)
	}
}

func TestTracesKeepContentOutUnlessAllowed(t *testing.T) {
	model := &Scripted{Name: "m", Fn: func(context.Context, StructuredRequest) (json.RawMessage, error) {
		return json.RawMessage(`{"ok":true}`), nil
	}}
	r := NewRouter(nil, slog.New(slog.NewTextHandler(io.Discard, nil)), model)
	call := func(allow *bool) {
		ctx, root := tracing.Start(context.Background(), "turn", tracing.Task)
		if allow != nil {
			tracing.Allow(ctx, *allow)
		}
		if _, _, err := r.Structured(ctx, CallMeta{Purpose: "ask"}, request()); err != nil {
			t.Fatal(err)
		}
		root.End(nil)
	}
	no, yes := false, true
	for _, c := range []struct {
		name    string
		content bool
		allow   *bool
		want    bool
	}{
		{"a high-privacy team", true, &no, false},
		{"a team that never said", true, nil, false},
		{"content turned off", false, &yes, false},
		{"allowed", true, &yes, true},
	} {
		exp := spans(t, c.content)
		call(c.allow)
		a := attrs(exp.GetSpans()[0])
		_, has := a["braintrust.input_json"]
		if has != c.want {
			t.Errorf("%s: content sent = %v", c.name, has)
		}
		if a["braintrust.metrics"] == "" || a["gen_ai.request.model"] != "m" {
			t.Errorf("%s: the call's numbers are missing: %v", c.name, a)
		}
	}
}

func TestRecordedCallsAreTracedOnce(t *testing.T) {
	exp := spans(t, true)
	var recorded []CallRecord
	rec := TracedRecorder{Next: RecorderFunc(func(_ context.Context, r CallRecord) { recorded = append(recorded, r) })}
	model := &Scripted{Name: "m", Fn: func(context.Context, StructuredRequest) (json.RawMessage, error) {
		return json.RawMessage(`{}`), nil
	}}
	r := NewRouter(rec, slog.New(slog.NewTextHandler(io.Discard, nil)), model)
	if _, _, err := r.Structured(context.Background(), CallMeta{Purpose: "analyze"}, request()); err != nil {
		t.Fatal(err)
	}
	// An embedding is recorded after it is made, and traced as it took.
	rec.Record(context.Background(), CallRecord{CallMeta: CallMeta{Purpose: "embed_text"},
		Usage: Usage{Provider: "openrouter", Model: "qwen", InputTokens: 12, Latency: 300 * time.Millisecond}})

	got := exp.GetSpans()
	if len(got) != 2 || len(recorded) != 2 {
		t.Fatalf("%d spans and %d records, want 2 of each", len(got), len(recorded))
	}
	embed := got[1]
	if embed.Name != "embed_text" || embed.EndTime.Sub(embed.StartTime) < 300*time.Millisecond {
		t.Errorf("embedding span %q took %v", embed.Name, embed.EndTime.Sub(embed.StartTime))
	}
	if !strings.Contains(attrs(embed)["braintrust.metrics"], `"prompt_tokens":12`) {
		t.Errorf("metrics = %s", attrs(embed)["braintrust.metrics"])
	}
}
