// Package api serves the Connect/gRPC API used by the mobile apps.
package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"pidex/gen/pidex/v1/pidexv1connect"
	"pidex/internal/ai"
	"pidex/internal/configstore"
	"pidex/internal/pipeline"
	"pidex/internal/search"
	"pidex/internal/storage"
	"pidex/internal/store/dbq"
)

// Enqueuer schedules work inside the caller's transaction.
type Enqueuer interface {
	EnqueueProcessPhoto(ctx context.Context, tx pgx.Tx, photoID uuid.UUID) error
	EnqueueIndexPhoto(ctx context.Context, tx pgx.Tx, photoID uuid.UUID) error
}

type Server struct {
	Pool      *pgxpool.Pool
	Q         *dbq.Queries
	Configs   *configstore.Store
	Storage   *storage.S3
	Jobs      Enqueuer
	Processor *pipeline.Processor
	// ConfigModel drafts configs from descriptions; it may be empty.
	ConfigModel *ai.Router
	// SearchModel turns search requests into filters; it may be empty.
	SearchModel *ai.Router
	Search      *search.Searcher
	Log         *slog.Logger

	MaxUploadBytes int64
	UploadURLTTL   time.Duration
}

// Handler mounts every service behind the auth interceptor.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	opts := connect.WithInterceptors(AuthInterceptor(s.Q))
	mux.Handle(pidexv1connect.NewCaptureServiceHandler(&captureService{s}, opts))
	mux.Handle(pidexv1connect.NewLibraryServiceHandler(&libraryService{s}, opts))
	mux.Handle(pidexv1connect.NewConfigServiceHandler(&configService{s}, opts))
	mux.Handle(pidexv1connect.NewTeamServiceHandler(&teamService{s}, opts))
	mux.Handle(pidexv1connect.NewSearchServiceHandler(&searchService{s}, opts))
	mux.Handle(pidexv1connect.NewChatServiceHandler(&chatService{s}, opts))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := s.Pool.Ping(r.Context()); err != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte("ok")) //nolint:errcheck
	})
	return mux
}
