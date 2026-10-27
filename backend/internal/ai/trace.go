package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"pidex/internal/tracing"
)

// TracedRecorder records calls made outside a Router, such as embeddings,
// as model calls in the trace they belong to, then passes every record on.
// A Router traces its own calls, with what was asked and answered.
type TracedRecorder struct {
	Next Recorder
}

func (t TracedRecorder) Record(ctx context.Context, rec CallRecord) {
	if !rec.traced {
		end := time.Now()
		_, sp := tracing.Start(ctx, rec.Purpose, tracing.LLM, tracing.At(end.Add(-rec.Usage.Latency)))
		sp.Model(rec.Usage.Provider, rec.Usage.Model)
		noteCall(sp, rec.CallMeta)
		noteUsage(sp, rec.Usage)
		sp.EndAt(end, rec.Err)
	}
	if t.Next != nil {
		t.Next.Record(ctx, rec)
	}
}

func noteCall(sp *tracing.Span, meta CallMeta) {
	sp.Set("purpose", meta.Purpose)
	sp.Set("team_id", meta.TeamID)
	if meta.PhotoID != nil {
		sp.Set("photo_id", *meta.PhotoID)
	}
}

func noteUsage(sp *tracing.Span, u Usage) {
	sp.Metric("prompt_tokens", float64(u.InputTokens))
	sp.Metric("completion_tokens", float64(u.OutputTokens))
	sp.Metric("tokens", float64(u.InputTokens+u.OutputTokens))
	sp.Metric("cost_usd", u.CostUSD)
}

// prompt is a request as chat messages. Images are named, not sent: the
// trace says what the model was shown, not the photo.
func prompt(req StructuredRequest) []map[string]any {
	user := []map[string]any{{"type": "text", "text": req.Prompt}}
	for i, img := range req.Images {
		user = append(user, map[string]any{"type": "text", "text": fmt.Sprintf("[image %d: %s, %d KB]", i+1, img.MIMEType, len(img.Data)/1024)})
	}
	return []map[string]any{
		{"role": "system", "content": req.Instructions},
		{"role": "user", "content": user},
	}
}

// answer is what a model returned, as JSON when it is.
func answer(out json.RawMessage) any {
	if json.Valid(out) {
		return out
	}
	return string(out)
}
