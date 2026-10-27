// Package tracing records what the server does with models as traces: one
// for each conversation turn, search, question, config draft and photo
// processed, with every model call in it (what was asked, what came back,
// tokens, cost and time). Traces go to Braintrust over OpenTelemetry, or to
// any OTLP collector; with no key set nothing is recorded.
//
// What people wrote and what models answered is sent only for traces whose
// team allows it (see Allow); otherwise a trace keeps its shape, timings,
// tokens and ids. Images are never sent, only their type and size.
package tracing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// Config says where traces go.
type Config struct {
	// APIKey is a Braintrust API key; empty turns tracing off.
	APIKey string
	// APIURL is Braintrust's API, or a self-hosted data plane.
	APIURL string
	// Project names the Braintrust project traces are logged to; it is
	// created on first use. ProjectID, when set, is used instead.
	Project   string
	ProjectID string
	// Content sends what people wrote and what models answered, for teams
	// that allow it. Off, traces keep only their shape and numbers.
	Content bool
	// Service names this program in the traces.
	Service string
}

var (
	tracer  trace.Tracer = noopProvider().Tracer("")
	content atomic.Bool
)

// Setup starts sending traces. The returned function flushes what is still
// buffered; call it before the process exits.
func Setup(ctx context.Context, cfg Config) (func(context.Context) error, error) {
	if cfg.APIKey == "" {
		return func(context.Context) error { return nil }, nil
	}
	parent := "project_name:" + cfg.Project
	if cfg.ProjectID != "" {
		parent = "project_id:" + cfg.ProjectID
	} else if cfg.Project == "" {
		return nil, errors.New("tracing: a Braintrust project name or id is needed")
	}
	exp, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpointURL(strings.TrimRight(cfg.APIURL, "/")+"/otel/v1/traces"),
		otlptracehttp.WithHeaders(map[string]string{
			"Authorization": "Bearer " + cfg.APIKey,
			"x-bt-parent":   parent,
		}))
	if err != nil {
		return nil, fmt.Errorf("tracing: %w", err)
	}
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(resource.NewSchemaless(semconv.ServiceName(cfg.Service))),
	)
	tracer = provider.Tracer("pidex")
	content.Store(cfg.Content)
	return provider.Shutdown, nil
}

func noopProvider() trace.TracerProvider { return noop.NewTracerProvider() }

// Use sends traces to the given provider instead, as tests do.
func Use(provider trace.TracerProvider, withContent bool) {
	tracer = provider.Tracer("pidex")
	content.Store(withContent)
}

// Kind is what a span is, as Braintrust shows it.
type Kind string

const (
	// Task is a unit of work: a conversation turn, a search, a photo.
	Task Kind = "task"
	// LLM is one call to a model.
	LLM Kind = "llm"
	// Tool is a step a turn takes without a model, such as a search of the
	// database.
	Tool Kind = "tool"
)

// Span is one step of a trace. Its methods do nothing on a nil Span, so
// callers need not check whether tracing is on.
type Span struct {
	span  trace.Span
	share *shared

	mu      sync.Mutex
	input   any
	output  any
	meta    map[string]any
	metrics map[string]float64
	tags    []string
}

// shared is what every span of one trace knows.
type shared struct {
	// 0: not said; 1: content allowed; 2: not allowed.
	content atomic.Int32
}

type spanKey struct{}

// StartOption adjusts a span as it starts.
type StartOption func(*[]trace.SpanStartOption)

// At starts the span at an earlier time, for a step timed by its caller.
func At(t time.Time) StartOption {
	return func(o *[]trace.SpanStartOption) { *o = append(*o, trace.WithTimestamp(t)) }
}

// Start begins a span, a child of the one in ctx or the root of a new
// trace.
func Start(ctx context.Context, name string, kind Kind, opts ...StartOption) (context.Context, *Span) {
	var so []trace.SpanStartOption
	for _, o := range opts {
		o(&so)
	}
	ctx, span := tracer.Start(ctx, name, so...)
	if !span.IsRecording() {
		return ctx, nil
	}
	share := &shared{}
	if parent := From(ctx); parent != nil {
		share = parent.share
	}
	span.SetAttributes(attribute.String("braintrust.span_attributes", mustJSON(map[string]string{"type": string(kind)})))
	s := &Span{span: span, share: share, meta: map[string]any{}}
	return context.WithValue(ctx, spanKey{}, s), s
}

// From is the span in ctx, or nil.
func From(ctx context.Context) *Span {
	s, _ := ctx.Value(spanKey{}).(*Span)
	return s
}

// Allow says whether the trace in ctx may carry what people wrote and what
// models answered: a team's privacy decides. Until said, it may not.
func Allow(ctx context.Context, ok bool) {
	if s := From(ctx); s != nil {
		v := int32(2)
		if ok {
			v = 1
		}
		s.share.content.Store(v)
	}
}

// Input is what the step was given: a person's words, a model's prompt.
// It is sent only where content is allowed.
func (s *Span) Input(v any) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.input = v
	s.mu.Unlock()
}

// Output is what the step gave back; like Input, it is content.
func (s *Span) Output(v any) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.output = v
	s.mu.Unlock()
}

// Set notes a fact about the step: ids, counts, choices made. Facts are
// sent whatever the team's privacy, so they must not be content.
func (s *Span) Set(key string, v any) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.meta[key] = v
	s.mu.Unlock()
}

// Metric records a number Braintrust charts, such as tokens.
func (s *Span) Metric(key string, v float64) {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.metrics == nil {
		s.metrics = map[string]float64{}
	}
	s.metrics[key] = v
	s.mu.Unlock()
}

// Tag labels the trace, for filtering.
func (s *Span) Tag(tags ...string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.tags = append(s.tags, tags...)
	s.mu.Unlock()
}

// Model names the model an LLM span called.
func (s *Span) Model(provider, model string) {
	if s == nil {
		return
	}
	s.span.SetAttributes(attribute.String("gen_ai.system", provider), attribute.String("gen_ai.request.model", model))
}

// End finishes the span; a non-nil err marks it failed.
func (s *Span) End(err error) {
	s.end(err)
}

// EndAt finishes the span at a given time, for a step timed by its caller.
func (s *Span) EndAt(t time.Time, err error) {
	s.end(err, trace.WithTimestamp(t))
}

func (s *Span) end(err error, opts ...trace.SpanEndOption) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	attrs := []attribute.KeyValue{attribute.String("braintrust.metadata", mustJSON(s.meta))}
	if content.Load() && s.share.content.Load() == 1 {
		if s.input != nil {
			attrs = append(attrs, attribute.String("braintrust.input_json", mustJSON(s.input)))
		}
		if s.output != nil {
			attrs = append(attrs, attribute.String("braintrust.output_json", mustJSON(s.output)))
		}
	}
	if len(s.metrics) > 0 {
		attrs = append(attrs, attribute.String("braintrust.metrics", mustJSON(s.metrics)))
	}
	if len(s.tags) > 0 {
		attrs = append(attrs, attribute.StringSlice("braintrust.tags", s.tags))
	}
	s.span.SetAttributes(attrs...)
	if err != nil {
		s.span.RecordError(err)
		s.span.SetStatus(codes.Error, err.Error())
	}
	s.span.End(opts...)
}

// mustJSON is v as JSON; a value that cannot be is said so instead.
func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf(`{"unserializable":%q}`, err.Error())
	}
	return string(b)
}
