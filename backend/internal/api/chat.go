package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	pidexv1 "pidex/gen/pidex/v1"
	"pidex/internal/search"
	"pidex/internal/store"
	"pidex/internal/store/dbq"
	"pidex/internal/teamconfig"
	"pidex/internal/tracing"
)

type chatService struct{ *Server }

const (
	// historyTurns is how many earlier exchanges a message is read with.
	historyTurns = 4
	titleRunes   = 40
)

func (s *chatService) ListConversations(ctx context.Context, req *connect.Request[pidexv1.ListConversationsRequest]) (*connect.Response[pidexv1.ListConversationsResponse], error) {
	pr := principalFrom(ctx)
	limit := req.Msg.PageSize
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.Q.ListConversations(ctx, dbq.ListConversationsParams{TeamID: pr.TeamID, UserID: pr.UserID, PageLimit: limit})
	if err != nil {
		return nil, internal(err)
	}
	out := &pidexv1.ListConversationsResponse{}
	for _, r := range rows {
		out.Conversations = append(out.Conversations, &pidexv1.Conversation{
			Id: r.ID.String(), Title: r.Title, CreatedAt: timestamppb.New(r.CreatedAt), UpdatedAt: timestamppb.New(r.UpdatedAt),
			MessageCount: r.MessageCount,
		})
	}
	return connect.NewResponse(out), nil
}

func (s *chatService) GetConversation(ctx context.Context, req *connect.Request[pidexv1.GetConversationRequest]) (*connect.Response[pidexv1.GetConversationResponse], error) {
	pr := principalFrom(ctx)
	conv, err := s.conversation(ctx, pr, req.Msg.Id)
	if err != nil {
		return nil, err
	}
	rows, err := s.Q.ListConversationMessages(ctx, conv.ID)
	if err != nil {
		return nil, internal(err)
	}
	out := &pidexv1.GetConversationResponse{Conversation: conversationOut(conv, int32(len(rows)))}
	for _, r := range rows {
		m, err := s.messageOut(ctx, pr.TeamID, r)
		if err != nil {
			return nil, err
		}
		out.Messages = append(out.Messages, m)
	}
	return connect.NewResponse(out), nil
}

func (s *chatService) DeleteConversation(ctx context.Context, req *connect.Request[pidexv1.DeleteConversationRequest]) (*connect.Response[pidexv1.DeleteConversationResponse], error) {
	pr := principalFrom(ctx)
	id, err := parseID("id", req.Msg.Id)
	if err != nil {
		return nil, err
	}
	n, err := s.Q.DeleteConversation(ctx, dbq.DeleteConversationParams{ID: id, TeamID: pr.TeamID, UserID: pr.UserID})
	if err != nil {
		return nil, internal(err)
	}
	if n == 0 {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("conversation not found"))
	}
	return connect.NewResponse(&pidexv1.DeleteConversationResponse{}), nil
}

// SendMessage reads the message in the light of the conversation so far:
// a follow-up adjusts the last search, a question about "these" is about
// the photos just shown, and earlier answers inform the next one.
//
// Each turn is a trace: the message, how it was read, what was searched,
// checked and answered, and every model call on the way.
func (s *chatService) SendMessage(ctx context.Context, req *connect.Request[pidexv1.SendMessageRequest]) (*connect.Response[pidexv1.SendMessageResponse], error) {
	return traced(ctx, "chat turn", func(ctx context.Context, sp *tracing.Span) (*connect.Response[pidexv1.SendMessageResponse], error) {
		return s.sendMessage(ctx, sp, req)
	})
}

func (s *chatService) sendMessage(ctx context.Context, sp *tracing.Span, req *connect.Request[pidexv1.SendMessageRequest]) (*connect.Response[pidexv1.SendMessageResponse], error) {
	pr := principalFrom(ctx)
	text := strings.TrimSpace(req.Msg.Text)
	sp.Input(map[string]any{"text": text})
	if text == "" || len(text) > maxQueryLen {
		return nil, invalid("a message must be 1 to %d characters", maxQueryLen)
	}
	cfg, _, err := s.Configs.Active(ctx, pr.TeamID)
	if err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}
	tracing.Allow(ctx, cfg.Privacy != teamconfig.PrivacyHigh)

	var conv dbq.Conversation
	var earlier []dbq.ConversationMessage
	if req.Msg.ConversationId == "" {
		conv, err = s.Q.CreateConversation(ctx, dbq.CreateConversationParams{TeamID: pr.TeamID, UserID: pr.UserID, Title: title(text)})
		if err != nil {
			return nil, internal(err)
		}
	} else {
		if conv, err = s.conversation(ctx, pr, req.Msg.ConversationId); err != nil {
			return nil, err
		}
		if earlier, err = s.Q.ListConversationMessages(ctx, conv.ID); err != nil {
			return nil, internal(err)
		}
	}
	sp.Set("conversation_id", conv.ID)
	sp.Set("turn", len(earlier)/2+1)
	history, previousPhotos := s.history(ctx, pr.TeamID, cfg, earlier)

	loc, err := time.LoadLocation(req.Msg.TimeZone)
	if err != nil || req.Msg.TimeZone == "" {
		loc = time.UTC
	}
	turn := s.planTurn(ctx, pr.TeamID, cfg, text, time.Now().In(loc), history)
	canAnswer := cfg.Privacy != teamconfig.PrivacyHigh && !s.Processor.Vision.Empty()
	sp.Set("intent", turn.Intent)
	sp.Set("about_previous", turn.AboutPrevious)

	var (
		f        = turn.Filters
		res      search.Result
		photoIDs []uuid.UUID
		// Matches in all; an answer or a reply about earlier photos has
		// only those it shows.
		total int
		kind  = "photos"
	)
	switch {
	case turn.Intent == search.IntentUnrelated:
		// Not about the photos: nothing to look up or answer.
		kind = "unrelated"
	case turn.AboutPrevious && len(previousPhotos) > 0 && turn.Intent == search.IntentPhotos:
		// "Only the back ones": the photos just shown, narrowed.
		f.Within = previousPhotos
		f, res, err = s.Search.RunPlanned(ctx, pr.TeamID, cfg, f, search.Page{Limit: defaultResults})
		photoIDs, total = res.PhotoIDs, res.Total
	case turn.AboutPrevious && len(previousPhotos) > 0:
		// "The second one", "which of these": the photos just shown.
		photoIDs = previousPhotos[:min(len(previousPhotos), askPhotos)]
	case turn.Intent == search.IntentAnswer && canAnswer:
		f, res, err = s.findForAnswer(ctx, pr.TeamID, cfg, f)
		photoIDs = res.PhotoIDs
	default:
		f, res, err = s.Search.RunPlanned(ctx, pr.TeamID, cfg, f, search.Page{Limit: defaultResults})
		photoIDs, total = res.PhotoIDs, res.Total
	}
	if err != nil {
		return nil, internal(err)
	}
	if f.Mode == search.ModeMissing && kind != "unrelated" {
		kind = "missing"
	}
	photos, err := s.photosInOrder(ctx, pr.TeamID, photoIDs)
	if err != nil {
		return nil, err
	}
	if kind == "photos" && !(turn.Intent == search.IntentAnswer && canAnswer) {
		// Shown as found: only those whose words show what was asked. An
		// answer looks at the photos themselves instead.
		checked := s.checkFound(ctx, pr.TeamID, cfg, text, f, res.SubjectNotFound, photos)
		total = max(total-(len(photos)-len(checked)), len(checked))
		photos, photoIDs = checked, make([]uuid.UUID, 0, len(checked))
		for _, p := range photos {
			photoIDs = append(photoIDs, p.ID)
		}
	}

	var answer string
	if kind != "missing" && turn.Intent == search.IntentAnswer && canAnswer && len(photos) > 0 {
		subjects, _, err := s.resultSubjects(ctx, pr.TeamID, res, photos)
		if err != nil {
			return nil, err
		}
		if answer, err = s.answer(ctx, pr.TeamID, cfg, text, photos, subjects, history.Turns); err != nil {
			return nil, connect.NewError(connect.CodeUnavailable, err)
		}
		kind = "answer"
		// Only the photos the answer rests on are shown with it.
		answer, photos = cited(answer, photos)
		photoIDs, total = make([]uuid.UUID, 0, len(photos)), len(photos)
		for _, p := range photos {
			photoIDs = append(photoIDs, p.ID)
		}
	}

	filtersJSON, err := protojson.Marshal(filtersProto(f, res.SubjectIDs))
	if err != nil {
		return nil, internal(err)
	}
	sp.Set("reply", kind)
	sp.Set("photos", len(photoIDs))
	sp.Set("total", max(total, len(photoIDs)))
	sp.Output(map[string]any{"kind": kind, "answer": answer, "photo_ids": photoIDs, "filters": json.RawMessage(filtersJSON)})
	var userRow, replyRow dbq.ConversationMessage
	err = store.WithTx(ctx, s.Pool, func(tx pgx.Tx) error {
		q := s.Q.WithTx(tx)
		var err error
		if userRow, err = q.AddConversationMessage(ctx, dbq.AddConversationMessageParams{
			ConversationID: conv.ID, Role: "user", Text: text, PhotoIds: []uuid.UUID{}, MissingSubjectIds: []uuid.UUID{},
		}); err != nil {
			return err
		}
		if replyRow, err = q.AddConversationMessage(ctx, dbq.AddConversationMessageParams{
			ConversationID: conv.ID, Role: "assistant", Text: answer, Intent: &kind, Filters: filtersJSON,
			PhotoIds: orEmpty(photoIDs), MissingSubjectIds: orEmpty(res.MissingIDs), SubjectNotFound: res.SubjectNotFound,
			PhotoTotal: int32(max(total, len(photoIDs))),
		}); err != nil {
			return err
		}
		return q.TouchConversation(ctx, conv.ID)
	})
	if err != nil {
		return nil, internal(err)
	}

	out := &pidexv1.SendMessageResponse{Conversation: conversationOut(conv, int32(len(earlier)+2))}
	if out.Message, err = s.messageOut(ctx, pr.TeamID, userRow); err != nil {
		return nil, err
	}
	if out.Reply, err = s.messageOut(ctx, pr.TeamID, replyRow); err != nil {
		return nil, err
	}
	return connect.NewResponse(out), nil
}

// planTurn plans with the conversation; a failed or slow model call falls
// back to matching the message as text, like a search does.
func (s *chatService) planTurn(ctx context.Context, teamID uuid.UUID, cfg *teamconfig.Config, text string, now time.Time, history search.Context) search.PlannedTurn {
	fallback := search.PlannedTurn{Filters: search.Filters{Mode: search.ModePhotos, Text: text}, Intent: search.IntentPhotos}
	if s.SearchModel.Empty() {
		return fallback
	}
	planCtx, cancel := context.WithTimeout(ctx, planTimeout)
	defer cancel()
	t, err := search.PlanTurn(planCtx, s.SearchModel, teamID, cfg, text, now, history)
	if err != nil {
		s.Log.Warn("chat plan failed; matching the message as text", "err", err)
		return fallback
	}
	return t
}

// history is what the conversation established: the last search, the last
// few exchanges, and the photos the last reply showed.
func (s *chatService) history(ctx context.Context, teamID uuid.UUID, cfg *teamconfig.Config, msgs []dbq.ConversationMessage) (search.Context, []uuid.UUID) {
	var h search.Context
	var shown []uuid.UUID
	var request string
	for _, m := range msgs {
		if m.Role == "user" {
			request = m.Text
			continue
		}
		h.Turns = append(h.Turns, search.Turn{Request: request, Reply: replySummary(m)})
		shown = m.PhotoIds
		if len(m.Filters) > 0 {
			var pf pidexv1.SearchFilters
			if protojson.Unmarshal(m.Filters, &pf) == nil {
				// The config may have changed since; what no longer fits
				// is dropped rather than failing the conversation.
				pf.Tags = known(pf.Tags, cfg)
				if f, err := filtersFrom(&pf, cfg); err == nil {
					f.SubjectIDs = nil // re-resolved from subject_query
					h.Previous = &f
				}
			}
		}
	}
	if len(h.Turns) > historyTurns {
		h.Turns = h.Turns[len(h.Turns)-historyTurns:]
	}
	return h, shown
}

func known(tags []string, cfg *teamconfig.Config) []string {
	var out []string
	for _, t := range tags {
		if cfg.HasTag(t) {
			out = append(out, t)
		}
	}
	return out
}

// replySummary is a reply as the next turn reads it.
func replySummary(m dbq.ConversationMessage) string {
	switch {
	case m.Text != "":
		return m.Text
	case m.Intent != nil && *m.Intent == "missing":
		return fmt.Sprintf("%d subjects without such a photo", len(m.MissingSubjectIds))
	case m.Intent != nil && *m.Intent == "unrelated":
		return "Not about the photos; nothing looked up"
	default:
		return fmt.Sprintf("%d photos shown", len(m.PhotoIds))
	}
}

func (s *chatService) conversation(ctx context.Context, pr Principal, rawID string) (dbq.Conversation, error) {
	id, err := parseID("conversation_id", rawID)
	if err != nil {
		return dbq.Conversation{}, err
	}
	conv, err := s.Q.GetConversation(ctx, dbq.GetConversationParams{ID: id, TeamID: pr.TeamID, UserID: pr.UserID})
	if err != nil {
		return conv, notFoundOr(err, "conversation")
	}
	return conv, nil
}

// messageOut resolves a stored message's references now: photos that were
// deleted are skipped, and links and tags are current.
func (s *chatService) messageOut(ctx context.Context, teamID uuid.UUID, m dbq.ConversationMessage) (*pidexv1.ChatMessage, error) {
	out := &pidexv1.ChatMessage{Id: m.ID.String(), Text: m.Text, CreatedAt: timestamppb.New(m.CreatedAt)}
	if m.Role == "user" {
		out.Role = pidexv1.ChatRole_CHAT_ROLE_USER
		return out, nil
	}
	out.Role = pidexv1.ChatRole_CHAT_ROLE_ASSISTANT
	out.SubjectNotFound = m.SubjectNotFound
	out.PhotoTotal = max(m.PhotoTotal, int32(len(m.PhotoIds)))
	switch {
	case m.Intent != nil && *m.Intent == "answer":
		out.Kind = pidexv1.ReplyKind_REPLY_KIND_ANSWER
	case m.Intent != nil && *m.Intent == "missing":
		out.Kind = pidexv1.ReplyKind_REPLY_KIND_MISSING
	case m.Intent != nil && *m.Intent == "unrelated":
		out.Kind = pidexv1.ReplyKind_REPLY_KIND_UNRELATED
	default:
		out.Kind = pidexv1.ReplyKind_REPLY_KIND_PHOTOS
	}
	var subjectIDs []uuid.UUID
	if len(m.Filters) > 0 {
		var f pidexv1.SearchFilters
		if protojson.Unmarshal(m.Filters, &f) == nil {
			out.Filters = &f
			for _, id := range f.SubjectIds {
				if u, err := uuid.Parse(id); err == nil {
					subjectIDs = append(subjectIDs, u)
				}
			}
		}
	}
	photos, err := s.photosInOrder(ctx, teamID, m.PhotoIds)
	if err != nil {
		return nil, err
	}
	if out.Photos, err = s.photosProto(ctx, s.Q, photos); err != nil {
		return nil, err
	}
	out.Subjects, out.Missing, err = s.resultSubjects(ctx, teamID, search.Result{SubjectIDs: subjectIDs, MissingIDs: m.MissingSubjectIds}, photos)
	return out, err
}

func conversationOut(c dbq.Conversation, messages int32) *pidexv1.Conversation {
	return &pidexv1.Conversation{
		Id: c.ID.String(), Title: c.Title, CreatedAt: timestamppb.New(c.CreatedAt), UpdatedAt: timestamppb.New(c.UpdatedAt),
		MessageCount: messages,
	}
}

// title is the first request, cut to a few dozen characters.
func title(text string) string {
	if utf8.RuneCountInString(text) <= titleRunes {
		return text
	}
	return string([]rune(text)[:titleRunes]) + "…"
}

func orEmpty(ids []uuid.UUID) []uuid.UUID {
	if ids == nil {
		return []uuid.UUID{}
	}
	return ids
}
