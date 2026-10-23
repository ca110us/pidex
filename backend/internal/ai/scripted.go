package ai

import (
	"context"
	"encoding/json"
)

// Scripted is a VisionModel driven by a function, for tests and for running
// the system locally without a provider key.
type Scripted struct {
	Name string
	Fn   func(ctx context.Context, req StructuredRequest) (json.RawMessage, error)
}

func (s *Scripted) Provider() string { return "scripted" }
func (s *Scripted) Model() string    { return s.Name }

func (s *Scripted) Structured(ctx context.Context, req StructuredRequest) (json.RawMessage, Usage, error) {
	out, err := s.Fn(ctx, req)
	return out, Usage{}, err
}
