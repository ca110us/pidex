package api

import (
	"context"

	"connectrpc.com/connect"

	"pidex/internal/tracing"
)

// traced runs a request as the root of a trace named for what it is, with
// who asked; the handler adds what it was asked and what it found.
func traced[Res any](ctx context.Context, name string, run func(context.Context, *tracing.Span) (*connect.Response[Res], error)) (*connect.Response[Res], error) {
	ctx, sp := tracing.Start(ctx, name, tracing.Task)
	pr := principalFrom(ctx)
	sp.Set("team_id", pr.TeamID)
	sp.Set("user_id", pr.UserID)
	sp.Tag(name)
	res, err := run(ctx, sp)
	sp.End(err)
	return res, err
}
