// Package audit records who did what. Automatic decisions are logged too, so
// every filing can be explained and corrections can be learned from.
package audit

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"pidex/internal/store/dbq"
)

type Entry struct {
	TeamID     uuid.UUID
	ActorID    *uuid.UUID // nil for the system
	Action     string
	TargetType string
	TargetID   *uuid.UUID
	Details    map[string]any
}

func Log(ctx context.Context, q *dbq.Queries, e Entry) error {
	details, err := json.Marshal(e.Details)
	if err != nil {
		return err
	}
	if e.Details == nil {
		details = []byte("{}")
	}
	kind := "system"
	if e.ActorID != nil {
		kind = "user"
	}
	return q.InsertAudit(ctx, dbq.InsertAuditParams{
		TeamID:      e.TeamID,
		ActorUserID: e.ActorID,
		ActorKind:   kind,
		Action:      e.Action,
		TargetType:  e.TargetType,
		TargetID:    e.TargetID,
		Details:     details,
	})
}
