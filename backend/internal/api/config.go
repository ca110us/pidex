package api

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	pidexv1 "pidex/gen/pidex/v1"
	"pidex/internal/configgen"
	"pidex/internal/configstore"
	"pidex/internal/teamconfig"
)

type configService struct{ *Server }

func (s *configService) ListTemplates(ctx context.Context, _ *connect.Request[pidexv1.ListTemplatesRequest]) (*connect.Response[pidexv1.ListTemplatesResponse], error) {
	tpls, err := teamconfig.Templates()
	if err != nil {
		return nil, internal(err)
	}
	out := &pidexv1.ListTemplatesResponse{}
	for _, t := range tpls {
		out.Templates = append(out.Templates, &pidexv1.TemplateSummary{
			Key: t.Key, Name: t.Config.Name, Industry: t.Config.Industry, Description: t.Config.Description,
		})
	}
	return connect.NewResponse(out), nil
}

func (s *configService) GetActiveConfig(ctx context.Context, _ *connect.Request[pidexv1.GetActiveConfigRequest]) (*connect.Response[pidexv1.GetActiveConfigResponse], error) {
	pr := principalFrom(ctx)
	row, err := s.Q.GetActiveTeamConfig(ctx, pr.TeamID)
	if err != nil {
		return nil, notFoundOr(err, "active config")
	}
	return connect.NewResponse(&pidexv1.GetActiveConfigResponse{
		Version: row.Version, ConfigJson: string(row.Config), CreatedAt: timestamppb.New(row.CreatedAt),
	}), nil
}

func (s *configService) PublishConfig(ctx context.Context, req *connect.Request[pidexv1.PublishConfigRequest]) (*connect.Response[pidexv1.PublishConfigResponse], error) {
	pr := principalFrom(ctx)
	if !pr.CanAdmin() {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("only owners and admins can change the config"))
	}
	var (
		cfg    *teamconfig.Config
		err    error
		source string
	)
	switch src := req.Msg.Source.(type) {
	case *pidexv1.PublishConfigRequest_TemplateKey:
		cfg, err = teamconfig.LoadTemplate(src.TemplateKey)
		source = "template:" + src.TemplateKey
	case *pidexv1.PublishConfigRequest_ConfigJson:
		cfg, err = teamconfig.Parse([]byte(src.ConfigJson))
		source = "json"
	case *pidexv1.PublishConfigRequest_RestoreVersion:
		cfg, err = s.Configs.Version(ctx, pr.TeamID, src.RestoreVersion)
		if err != nil {
			return nil, notFoundOr(err, "config version")
		}
		source = fmt.Sprintf("restore:%d", src.RestoreVersion)
	default:
		return nil, invalid("template_key or config_json is required")
	}
	if err != nil {
		return nil, invalid("%v", err)
	}
	row, err := configstore.Publish(ctx, s.Pool, pr.TeamID, &pr.UserID, cfg, source)
	if err != nil {
		return nil, internal(err)
	}
	return connect.NewResponse(&pidexv1.PublishConfigResponse{Version: row.Version, ConfigJson: string(row.Config)}), nil
}

func (s *configService) DraftConfig(ctx context.Context, req *connect.Request[pidexv1.DraftConfigRequest]) (*connect.Response[pidexv1.DraftConfigResponse], error) {
	pr := principalFrom(ctx)
	m := req.Msg
	if !m.ForNewTeam && !pr.CanAdmin() {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("only owners and admins can change the config"))
	}
	if desc := strings.TrimSpace(m.Description); desc == "" || len(desc) > configgen.MaxDescription {
		return nil, invalid("description must be 1 to %d characters", configgen.MaxDescription)
	}
	if s.ConfigModel.Empty() {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("no model is configured for drafting configs"))
	}
	var active *teamconfig.Config
	if !m.ForNewTeam {
		var err error
		if active, _, err = s.Configs.Active(ctx, pr.TeamID); err != nil {
			return nil, internal(err)
		}
	}
	base := active
	if m.BaseConfigJson != "" {
		var err error
		if base, err = teamconfig.Parse([]byte(m.BaseConfigJson)); err != nil {
			return nil, invalid("base config: %v", err)
		}
	}
	by := teamconfig.IdentifyBy(m.IdentifyBy)
	if by != "" && by != teamconfig.IdentifyByCode && by != teamconfig.IdentifyByAppearance {
		return nil, invalid("identify_by must be code, appearance or empty")
	}
	draft, err := configgen.Generate(ctx, s.ConfigModel, configgen.Request{Description: m.Description, Base: base, TeamID: pr.TeamID, IdentifyBy: by})
	switch {
	case errors.Is(err, configgen.ErrInvalid):
		return nil, connect.NewError(connect.CodeAborted, err)
	case err != nil:
		return nil, connect.NewError(connect.CodeUnavailable, err)
	}
	data, err := draft.Config.Marshal()
	if err != nil {
		return nil, internal(err)
	}
	out := &pidexv1.DraftConfigResponse{ConfigJson: string(data), Summary: draft.Summary, TestTips: draft.TestTips}
	if active != nil {
		for _, c := range configgen.Compare(active, draft.Config) {
			out.Warnings = append(out.Warnings, &pidexv1.ConfigChange{
				Kind: pidexv1.ConfigChangeKind(c.Kind), SubjectType: c.SubjectType, Field: c.Field, Tag: c.Tag,
				From: c.From, To: c.To, Message: c.Message,
			})
		}
	}
	return connect.NewResponse(out), nil
}

func (s *configService) ListConfigVersions(ctx context.Context, _ *connect.Request[pidexv1.ListConfigVersionsRequest]) (*connect.Response[pidexv1.ListConfigVersionsResponse], error) {
	rows, err := s.Q.ListTeamConfigs(ctx, principalFrom(ctx).TeamID)
	if err != nil {
		return nil, internal(err)
	}
	out := &pidexv1.ListConfigVersionsResponse{}
	for _, r := range rows {
		out.Versions = append(out.Versions, &pidexv1.ConfigVersion{
			Version: r.Version, Name: r.Name, CreatedAt: timestamppb.New(r.CreatedAt), Active: r.Active,
		})
	}
	return connect.NewResponse(out), nil
}
