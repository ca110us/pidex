package api

import (
	"context"
	"errors"
	"unicode/utf8"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	pidexv1 "pidex/gen/pidex/v1"
	"pidex/internal/store"
	"pidex/internal/store/dbq"
)

const (
	// maxSetupBatch bounds the messages one call adds: a new collection's
	// conversation so far, or one exchange.
	maxSetupBatch = 100
	maxSetupText  = 8000
)

var setupRoles = map[pidexv1.SetupRole]string{
	pidexv1.SetupRole_SETUP_ROLE_USER:      "user",
	pidexv1.SetupRole_SETUP_ROLE_ASSISTANT: "assistant",
	pidexv1.SetupRole_SETUP_ROLE_EVENT:     "event",
}

func (s *configService) GetSetupConversation(ctx context.Context, _ *connect.Request[pidexv1.GetSetupConversationRequest]) (*connect.Response[pidexv1.GetSetupConversationResponse], error) {
	pr := principalFrom(ctx)
	rows, err := s.Q.ListSetupMessages(ctx, pr.TeamID)
	if err != nil {
		return nil, internal(err)
	}
	out := &pidexv1.GetSetupConversationResponse{}
	for _, r := range rows {
		out.Messages = append(out.Messages, setupMessageOut(r))
	}
	return connect.NewResponse(out), nil
}

// AddSetupMessages records the conversation as the app holds it. Only
// those who may change the config add to it.
func (s *configService) AddSetupMessages(ctx context.Context, req *connect.Request[pidexv1.AddSetupMessagesRequest]) (*connect.Response[pidexv1.AddSetupMessagesResponse], error) {
	pr := principalFrom(ctx)
	if !pr.CanAdmin() {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("only owners and admins can change the config"))
	}
	msgs := req.Msg.Messages
	if len(msgs) == 0 || len(msgs) > maxSetupBatch {
		return nil, invalid("messages must have 1 to %d entries", maxSetupBatch)
	}
	for _, m := range msgs {
		if setupRoles[m.Role] == "" {
			return nil, invalid("message role is required")
		}
		if utf8.RuneCountInString(m.Text) > maxSetupText {
			return nil, invalid("message text is longer than %d characters", maxSetupText)
		}
	}
	out := &pidexv1.AddSetupMessagesResponse{}
	err := store.WithTx(ctx, s.Pool, func(tx pgx.Tx) error {
		q := s.Q.WithTx(tx)
		for _, m := range msgs {
			p := dbq.AddSetupMessageParams{TeamID: pr.TeamID, UserID: &pr.UserID, Role: setupRoles[m.Role], Text: m.Text}
			if m.Draft != nil {
				b, err := protojson.Marshal(m.Draft)
				if err != nil {
					return err
				}
				p.Draft = b
			}
			if m.ConfigVersion > 0 {
				v := m.ConfigVersion
				p.ConfigVersion = &v
			}
			row, err := q.AddSetupMessage(ctx, p)
			if err != nil {
				return err
			}
			out.Messages = append(out.Messages, setupMessageOut(row))
		}
		return nil
	})
	if err != nil {
		return nil, internal(err)
	}
	return connect.NewResponse(out), nil
}

func setupMessageOut(r dbq.SetupMessage) *pidexv1.SetupMessage {
	out := &pidexv1.SetupMessage{Id: r.ID.String(), Text: r.Text, CreatedAt: timestamppb.New(r.CreatedAt)}
	for role, name := range setupRoles {
		if name == r.Role {
			out.Role = role
		}
	}
	if len(r.Draft) > 0 {
		var d pidexv1.DraftConfigResponse
		if protojson.Unmarshal(r.Draft, &d) == nil {
			out.Draft = &d
		}
	}
	if r.ConfigVersion != nil {
		out.ConfigVersion = *r.ConfigVersion
	}
	return out
}
