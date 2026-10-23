package ai

import (
	"context"
	"log/slog"

	"pidex/internal/store/dbq"
)

// DBRecorder writes every model call to ai_calls for cost monitoring.
type DBRecorder struct {
	Q   *dbq.Queries
	Log *slog.Logger
}

func (r *DBRecorder) Record(ctx context.Context, rec CallRecord) {
	errText := ""
	if rec.Err != nil {
		errText = rec.Err.Error()
	}
	err := r.Q.InsertAICall(context.WithoutCancel(ctx), dbq.InsertAICallParams{
		TeamID:       rec.TeamID,
		PhotoID:      rec.PhotoID,
		Purpose:      rec.Purpose,
		Provider:     rec.Usage.Provider,
		Model:        rec.Usage.Model,
		InputTokens:  int32(rec.Usage.InputTokens),
		OutputTokens: int32(rec.Usage.OutputTokens),
		CostUsd:      rec.Usage.CostUSD,
		LatencyMs:    int32(rec.Usage.Latency.Milliseconds()),
		Ok:           rec.Err == nil,
		Error:        errText,
	})
	if err != nil {
		r.Log.Warn("record ai call", "err", err)
	}
}
