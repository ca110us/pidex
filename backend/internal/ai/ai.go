// Package ai is the only place that talks to model providers. Everything
// else asks for structured JSON or embeddings and gets usage back for
// metering, so providers can be swapped or stacked without touching callers.
package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"pidex/internal/tracing"
)

type Image struct {
	Data     []byte
	MIMEType string
}

type Usage struct {
	Provider     string
	Model        string
	InputTokens  int
	OutputTokens int
	CostUSD      float64
	Latency      time.Duration
}

// StructuredRequest asks for one JSON object conforming to Schema.
type StructuredRequest struct {
	// Instructions are stable across calls for a team, which lets providers
	// cache them.
	Instructions    string
	Prompt          string
	Images          []Image
	Schema          map[string]any
	MaxOutputTokens int
}

type VisionModel interface {
	Provider() string
	Model() string
	Structured(ctx context.Context, req StructuredRequest) (json.RawMessage, Usage, error)
}

type ImageEmbedder interface {
	Dimensions() int
	EmbedImage(ctx context.Context, img Image) ([]float32, Usage, error)
}

// Price is USD per million tokens.
type Price struct {
	InputPerM  float64
	OutputPerM float64
}

func (p Price) Cost(in, out int) float64 {
	return (float64(in)*p.InputPerM + float64(out)*p.OutputPerM) / 1e6
}

// CallMeta attributes a call for metering.
type CallMeta struct {
	TeamID  uuid.UUID
	PhotoID *uuid.UUID
	Purpose string
}

type CallRecord struct {
	CallMeta
	Usage Usage
	Err   error
	// traced is set by a Router, which traces its calls itself.
	traced bool
}

type Recorder interface {
	Record(ctx context.Context, rec CallRecord)
}

type RecorderFunc func(ctx context.Context, rec CallRecord)

func (f RecorderFunc) Record(ctx context.Context, rec CallRecord) { f(ctx, rec) }

// Router tries each model in order until one returns valid JSON. Every
// attempt, failed or not, is recorded, and traced as a model call.
type Router struct {
	models []VisionModel
	rec    Recorder
	log    *slog.Logger
}

func NewRouter(rec Recorder, log *slog.Logger, models ...VisionModel) *Router {
	return &Router{models: models, rec: rec, log: log}
}

func (r *Router) Empty() bool { return r == nil || len(r.models) == 0 }

// Primary names the first model, for provenance.
func (r *Router) Primary() string {
	if r.Empty() {
		return ""
	}
	return r.models[0].Provider() + "/" + r.models[0].Model()
}

var ErrNoModel = errors.New("ai: no model configured")

func (r *Router) Structured(ctx context.Context, meta CallMeta, req StructuredRequest) (json.RawMessage, Usage, error) {
	if r.Empty() {
		return nil, Usage{}, ErrNoModel
	}
	var errs []error
	for i, m := range r.models {
		callCtx, sp := tracing.Start(ctx, meta.Purpose, tracing.LLM)
		sp.Model(m.Provider(), m.Model())
		noteCall(sp, meta)
		sp.Set("attempt", i+1)
		sp.Set("images", len(req.Images))
		sp.Input(prompt(req))
		start := time.Now()
		out, usage, err := m.Structured(callCtx, req)
		usage.Provider, usage.Model = m.Provider(), m.Model()
		if usage.Latency == 0 {
			usage.Latency = time.Since(start)
		}
		if err == nil && !json.Valid(out) {
			err = fmt.Errorf("model returned invalid JSON")
		}
		if out != nil {
			sp.Output(answer(out))
		}
		noteUsage(sp, usage)
		sp.End(err)
		if r.rec != nil {
			r.rec.Record(ctx, CallRecord{CallMeta: meta, Usage: usage, Err: err, traced: true})
		}
		if err == nil {
			return out, usage, nil
		}
		if ctx.Err() != nil {
			return nil, usage, ctx.Err()
		}
		r.log.Warn("model call failed, trying next", "provider", m.Provider(), "model", m.Model(), "err", err)
		errs = append(errs, fmt.Errorf("%s/%s: %w", m.Provider(), m.Model(), err))
	}
	return nil, Usage{}, errors.Join(errs...)
}
