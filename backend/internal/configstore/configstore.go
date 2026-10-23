// Package configstore loads team configs. Published versions are immutable,
// so they are cached forever; only "which version is active" is looked up.
package configstore

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"pidex/internal/audit"
	"pidex/internal/store"
	"pidex/internal/store/dbq"
	"pidex/internal/teamconfig"
)

type Store struct {
	q     *dbq.Queries
	cache sync.Map // key{team, version} -> *teamconfig.Config
}

type key struct {
	team    uuid.UUID
	version int32
}

func New(q *dbq.Queries) *Store { return &Store{q: q} }

func (s *Store) Version(ctx context.Context, teamID uuid.UUID, version int32) (*teamconfig.Config, error) {
	k := key{teamID, version}
	if c, ok := s.cache.Load(k); ok {
		return c.(*teamconfig.Config), nil
	}
	row, err := s.q.GetTeamConfig(ctx, dbq.GetTeamConfigParams{TeamID: teamID, Version: version})
	if err != nil {
		return nil, fmt.Errorf("load config v%d: %w", version, err)
	}
	cfg, err := teamconfig.Parse(row.Config)
	if err != nil {
		return nil, fmt.Errorf("parse stored config v%d: %w", version, err)
	}
	s.cache.Store(k, cfg)
	return cfg, nil
}

func (s *Store) Active(ctx context.Context, teamID uuid.UUID) (*teamconfig.Config, int32, error) {
	team, err := s.q.GetTeam(ctx, teamID)
	if err != nil {
		return nil, 0, fmt.Errorf("load team: %w", err)
	}
	if team.ActiveConfigVersion == nil {
		return nil, 0, fmt.Errorf("team %s has no published config", teamID)
	}
	cfg, err := s.Version(ctx, teamID, *team.ActiveConfigVersion)
	return cfg, *team.ActiveConfigVersion, err
}

// Publish stores cfg as the team's next version and makes it active.
// Earlier versions are kept; photos keep the version they were filed with.
func Publish(ctx context.Context, pool *pgxpool.Pool, teamID uuid.UUID, actor *uuid.UUID, cfg *teamconfig.Config, source string) (dbq.TeamConfig, error) {
	var row dbq.TeamConfig
	err := store.WithTx(ctx, pool, func(tx pgx.Tx) error {
		var err error
		row, err = PublishTx(ctx, tx, teamID, actor, cfg, source)
		return err
	})
	return row, err
}

// PublishTx is Publish inside the caller's transaction.
func PublishTx(ctx context.Context, tx pgx.Tx, teamID uuid.UUID, actor *uuid.UUID, cfg *teamconfig.Config, source string) (dbq.TeamConfig, error) {
	data, err := cfg.Marshal()
	if err != nil {
		return dbq.TeamConfig{}, err
	}
	// Serialize publishers so version numbers never collide.
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", "config:"+teamID.String()); err != nil {
		return dbq.TeamConfig{}, err
	}
	q := dbq.New(tx)
	row, err := q.InsertTeamConfig(ctx, dbq.InsertTeamConfigParams{TeamID: teamID, Config: data, CreatedBy: actor})
	if err != nil {
		return dbq.TeamConfig{}, err
	}
	if err := q.SetActiveConfigVersion(ctx, dbq.SetActiveConfigVersionParams{ID: teamID, Version: &row.Version}); err != nil {
		return dbq.TeamConfig{}, err
	}
	return row, audit.Log(ctx, q, audit.Entry{
		TeamID: teamID, ActorID: actor, Action: "config.published",
		Details: map[string]any{"version": row.Version, "source": source},
	})
}
