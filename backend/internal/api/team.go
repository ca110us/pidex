package api

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	pidexv1 "pidex/gen/pidex/v1"
	"pidex/internal/audit"
	"pidex/internal/configstore"
	"pidex/internal/search"
	"pidex/internal/store"
	"pidex/internal/store/dbq"
	"pidex/internal/teamconfig"
)

type teamService struct{ *Server }

func (s *teamService) GetTeam(ctx context.Context, _ *connect.Request[pidexv1.GetTeamRequest]) (*connect.Response[pidexv1.GetTeamResponse], error) {
	pr := principalFrom(ctx)
	t, err := s.Q.GetTeam(ctx, pr.TeamID)
	if err != nil {
		return nil, notFoundOr(err, "team")
	}
	return connect.NewResponse(&pidexv1.GetTeamResponse{Team: teamOut(t, pr.Role)}), nil
}

// CreateTeam is open to any member of any team: the caller becomes owner of
// a team of their own and gets a token for it.
func (s *teamService) CreateTeam(ctx context.Context, req *connect.Request[pidexv1.CreateTeamRequest]) (*connect.Response[pidexv1.CreateTeamResponse], error) {
	pr := principalFrom(ctx)
	cfg, err := teamconfig.Parse([]byte(req.Msg.ConfigJson))
	if err != nil {
		return nil, invalid("config: %v", err)
	}
	name := strings.TrimSpace(req.Msg.Name)
	if name == "" {
		name = cfg.Name
	}
	if len(name) > 200 {
		return nil, invalid("name is longer than 200 characters")
	}

	var (
		team  dbq.Team
		token string
	)
	err = store.WithTx(ctx, s.Pool, func(tx pgx.Tx) error {
		q := dbq.New(tx)
		var err error
		if team, err = q.CreateTeam(ctx, dbq.CreateTeamParams{Name: name, DataRegion: "us"}); err != nil {
			return err
		}
		if err := q.AddTeamMember(ctx, dbq.AddTeamMemberParams{TeamID: team.ID, UserID: pr.UserID, Role: "owner"}); err != nil {
			return err
		}
		var hash []byte
		token, hash = NewToken()
		if err := q.CreateAPIToken(ctx, dbq.CreateAPITokenParams{TokenSha256: hash, TeamID: team.ID, UserID: pr.UserID, Label: "app"}); err != nil {
			return err
		}
		row, err := configstore.PublishTx(ctx, tx, team.ID, &pr.UserID, cfg, "create_team")
		if err != nil {
			return err
		}
		team.ActiveConfigVersion = &row.Version
		return audit.Log(ctx, q, audit.Entry{
			TeamID: team.ID, ActorID: &pr.UserID, Action: "team.created",
			TargetType: "team", TargetID: ptr(team.ID),
			Details: map[string]any{"from_team": pr.TeamID},
		})
	})
	if err != nil {
		return nil, internal(err)
	}
	return connect.NewResponse(&pidexv1.CreateTeamResponse{Team: teamOut(team, "owner"), Token: token}), nil
}

func teamOut(t dbq.Team, role string) *pidexv1.Team {
	out := &pidexv1.Team{Id: t.ID.String(), Name: t.Name, Role: role}
	if t.ActiveConfigVersion != nil {
		out.ConfigVersion = *t.ActiveConfigVersion
	}
	return out
}

func ptr(id uuid.UUID) *uuid.UUID { return &id }

const (
	defaultRecent = 4
	maxRecent     = 24
	overviewTags  = 12
)

func (s *teamService) GetOverview(ctx context.Context, req *connect.Request[pidexv1.GetOverviewRequest]) (*connect.Response[pidexv1.GetOverviewResponse], error) {
	pr := principalFrom(ctx)
	t, err := s.Q.GetTeam(ctx, pr.TeamID)
	if err != nil {
		return nil, notFoundOr(err, "team")
	}
	counts, err := s.Q.TeamOverviewCounts(ctx, pr.TeamID)
	if err != nil {
		return nil, internal(err)
	}
	tags, err := s.Q.TeamTagCounts(ctx, dbq.TeamTagCountsParams{TeamID: pr.TeamID, PageLimit: overviewTags})
	if err != nil {
		return nil, internal(err)
	}
	n := int(req.Msg.RecentCount)
	if n <= 0 {
		n = defaultRecent
	}
	// The same listing as a search without filters: newest, no junk.
	res, err := s.Search.Run(ctx, pr.TeamID, nil, search.Filters{Mode: search.ModePhotos}, min(n, maxRecent))
	if err != nil {
		return nil, internal(err)
	}
	photos, err := s.photosInOrder(ctx, pr.TeamID, res.PhotoIDs)
	if err != nil {
		return nil, err
	}
	out := &pidexv1.GetOverviewResponse{
		Team: teamOut(t, pr.Role), PhotoCount: counts.PhotoCount, SubjectCount: counts.SubjectCount,
	}
	if out.Recent, err = s.photosProto(ctx, s.Q, photos); err != nil {
		return nil, err
	}
	for _, tc := range tags {
		out.Tags = append(out.Tags, &pidexv1.TagCount{Key: tc.TagKey, Count: tc.PhotoCount})
	}
	return connect.NewResponse(out), nil
}

// DeleteTeam deletes a team and everything in it, for good. The rows go
// first, in one transaction, so nothing half-deleted is ever shown; the
// photo files follow. A file that fails to delete is logged, and is
// unreachable either way.
func (s *teamService) DeleteTeam(ctx context.Context, req *connect.Request[pidexv1.DeleteTeamRequest]) (*connect.Response[pidexv1.DeleteTeamResponse], error) {
	pr := principalFrom(ctx)
	if pr.Role != "owner" {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("only the owner can delete a collection"))
	}
	team, err := s.Q.GetTeam(ctx, pr.TeamID)
	if err != nil {
		return nil, notFoundOr(err, "team")
	}
	if strings.TrimSpace(req.Msg.ConfirmName) != team.Name {
		return nil, invalid("confirm_name must be the collection's name exactly")
	}
	var (
		keys   []string
		photos int
	)
	err = store.WithTx(ctx, s.Pool, func(tx pgx.Tx) error {
		q := s.Q.WithTx(tx)
		rows, err := q.TeamObjectKeys(ctx, pr.TeamID)
		if err != nil {
			return err
		}
		photos = len(rows)
		for _, r := range rows {
			keys = append(keys, r.ObjectKey)
			for _, k := range []*string{r.ThumbKey, r.OriginalKey, r.LiveKey} {
				if k != nil {
					keys = append(keys, *k)
				}
			}
		}
		_, err = q.DeleteTeam(ctx, pr.TeamID)
		return err
	})
	if err != nil {
		return nil, internal(err)
	}
	if err := s.Storage.Delete(context.WithoutCancel(ctx), keys); err != nil {
		s.Log.Error("photo files of a deleted team remain", "team", pr.TeamID, "files", len(keys), "err", err)
	}
	s.Log.Info("team deleted", "team", pr.TeamID, "by", pr.UserID, "files", len(keys))
	return connect.NewResponse(&pidexv1.DeleteTeamResponse{DeletedPhotos: int32(photos)}), nil
}
