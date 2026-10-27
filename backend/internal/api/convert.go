package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/timestamppb"

	pidexv1 "pidex/gen/pidex/v1"
	"pidex/internal/analysis"
	"pidex/internal/pipeline"
	"pidex/internal/store/dbq"
)

const viewURLTTL = 15 * time.Minute

var roleToProto = map[string]pidexv1.PhotoRole{
	"identity": pidexv1.PhotoRole_PHOTO_ROLE_IDENTITY,
	"document": pidexv1.PhotoRole_PHOTO_ROLE_DOCUMENT,
	"content":  pidexv1.PhotoRole_PHOTO_ROLE_CONTENT,
	"junk":     pidexv1.PhotoRole_PHOTO_ROLE_JUNK,
}

var statusToProto = map[string]pidexv1.PhotoStatus{
	"awaiting_upload": pidexv1.PhotoStatus_PHOTO_STATUS_AWAITING_UPLOAD,
	"uploaded":        pidexv1.PhotoStatus_PHOTO_STATUS_UPLOADED,
	"processing":      pidexv1.PhotoStatus_PHOTO_STATUS_PROCESSING,
	"processed":       pidexv1.PhotoStatus_PHOTO_STATUS_PROCESSED,
	"failed":          pidexv1.PhotoStatus_PHOTO_STATUS_FAILED,
}

var assignmentToProto = map[string]pidexv1.AssignmentState{
	"auto":       pidexv1.AssignmentState_ASSIGNMENT_STATE_AUTO,
	"suggested":  pidexv1.AssignmentState_ASSIGNMENT_STATE_SUGGESTED,
	"manual":     pidexv1.AssignmentState_ASSIGNMENT_STATE_MANUAL,
	"unassigned": pidexv1.AssignmentState_ASSIGNMENT_STATE_UNASSIGNED,
}

func reverse[K comparable, V comparable](m map[K]V) map[V]K {
	out := make(map[V]K, len(m))
	for k, v := range m {
		out[v] = k
	}
	return out
}

var (
	roleFromProto       = reverse(roleToProto)
	assignmentFromProto = reverse(assignmentToProto)
)

func idString(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}

func ts(t *time.Time) *timestamppb.Timestamp {
	if t == nil {
		return nil
	}
	return timestamppb.New(*t)
}

// placeJSON keeps the parts of a place that say something, trimmed; nil
// when none does.
func placeJSON(p *pidexv1.Place) []byte {
	if p == nil {
		return nil
	}
	parts := map[string]string{}
	for k, v := range map[string]string{"country": p.Country, "region": p.Region, "city": p.City, "district": p.District, "name": p.Name} {
		if v = strings.TrimSpace(v); v != "" {
			if len(v) > 200 {
				v = v[:200]
			}
			parts[k] = v
		}
	}
	if len(parts) == 0 {
		return nil
	}
	b, _ := json.Marshal(parts)
	return b
}

func placeProto(raw []byte) *pidexv1.Place {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]string
	if json.Unmarshal(raw, &m) != nil {
		return nil
	}
	return &pidexv1.Place{Country: m["country"], Region: m["region"], City: m["city"], District: m["district"], Name: m["name"]}
}

// described is a photo's description, with its subject called by name: "Tutu
// crouches by the sink", not "a tuxedo cat". Only once it is filed under
// that subject; a suggestion may be wrong.
func described(ph dbq.Photo, ex pipeline.Extracted, names map[uuid.UUID]string) string {
	if ph.SubjectID == nil || ex.NamedDescription == "" || (ph.Assignment != "auto" && ph.Assignment != "manual") {
		return ph.Description
	}
	name := names[*ph.SubjectID]
	if name == "" {
		return ph.Description
	}
	return strings.ReplaceAll(ex.NamedDescription, analysis.SubjectPlaceholder, name)
}

// photoProto converts a photo with links to its images; names are the
// display names of the subjects photos are filed under, for their
// descriptions.
func (s *Server) photoProto(ctx context.Context, ph dbq.Photo, tags []dbq.PhotoTag, names map[uuid.UUID]string) *pidexv1.Photo {
	out := photoData(ph, tags, names)
	out.ViewUrl, out.ThumbnailUrl = s.photoLinks(ctx, ph.Status, ph.ObjectKey, ph.ThumbKey)
	return out
}

// photoLinks are short-lived links to a photo's original and thumbnail,
// each empty until there is one.
func (s *Server) photoLinks(ctx context.Context, status, objectKey string, thumbKey *string) (view, thumb string) {
	if status != "awaiting_upload" {
		view, _ = s.Storage.PresignGet(ctx, objectKey, viewURLTTL)
	}
	if thumbKey != nil {
		thumb, _ = s.Storage.PresignGet(ctx, *thumbKey, viewURLTTL)
	}
	return view, thumb
}

// fileLinks are short-lived links to a photo's other files, each empty
// unless it is kept.
func (s *Server) fileLinks(ctx context.Context, originalKey, liveKey *string) (original, live string) {
	if originalKey != nil {
		original, _ = s.Storage.PresignGet(ctx, *originalKey, viewURLTTL)
	}
	if liveKey != nil {
		live, _ = s.Storage.PresignGet(ctx, *liveKey, viewURLTTL)
	}
	return original, live
}

// photoData converts a photo without links, as phones keep it.
func photoData(ph dbq.Photo, tags []dbq.PhotoTag, names map[uuid.UUID]string) *pidexv1.Photo {
	ex := pipeline.DecodeExtracted(ph.Extracted)
	out := &pidexv1.Photo{
		Id:                   ph.ID.String(),
		SessionId:            idString(ph.SessionID),
		SubjectId:            idString(ph.SubjectID),
		SuggestedSubjectId:   idString(ph.SuggestedSubjectID),
		Assignment:           assignmentToProto[ph.Assignment],
		AssignmentConfidence: ph.AssignmentConfidence,
		Status:               statusToProto[ph.Status],
		CapturedAt:           ts(ph.CapturedAt),
		UploadedBy:           ph.UploadedBy.String(),
		Description:          described(ph, ex, names),
		ExtractedFields:      ex.DisplayFields(),
		Place:                placeProto(ph.Place),
		CreatedAt:            timestamppb.New(ph.CreatedAt),
		OriginalKept:         ph.OriginalKey != nil,
		Live:                 ph.LiveKey != nil,
	}
	if ph.Role != nil {
		out.Role = roleToProto[*ph.Role]
	}
	if ph.Latitude != nil && ph.Longitude != nil {
		out.Location = &pidexv1.GeoPoint{Latitude: *ph.Latitude, Longitude: *ph.Longitude}
		if ph.LocationAccuracyM != nil {
			out.Location.AccuracyMeters = *ph.LocationAccuracyM
		}
	}
	for _, t := range tags {
		if t.PhotoID != ph.ID {
			continue
		}
		src := pidexv1.TagSource_TAG_SOURCE_AI
		if t.Source == "user" {
			src = pidexv1.TagSource_TAG_SOURCE_USER
		}
		out.Tags = append(out.Tags, &pidexv1.Tag{Key: t.TagKey, Confidence: t.Confidence, Source: src})
	}
	return out
}

func (s *Server) photosProto(ctx context.Context, q *dbq.Queries, photos []dbq.Photo) ([]*pidexv1.Photo, error) {
	ids := make([]uuid.UUID, len(photos))
	for i, p := range photos {
		ids[i] = p.ID
	}
	tags, err := q.ListPhotoTags(ctx, ids)
	if err != nil {
		return nil, internal(err)
	}
	names, err := subjectNames(ctx, q, photos)
	if err != nil {
		return nil, err
	}
	out := make([]*pidexv1.Photo, len(photos))
	for i, p := range photos {
		out[i] = s.photoProto(ctx, p, tags, names)
	}
	return out, nil
}

// subjectNames are the display names of the subjects photos are filed under.
func subjectNames(ctx context.Context, q *dbq.Queries, photos []dbq.Photo) (map[uuid.UUID]string, error) {
	subs, err := subjectsOf(ctx, q, photos)
	names := make(map[uuid.UUID]string, len(subs))
	for id, sub := range subs {
		names[id] = sub.DisplayName
	}
	return names, err
}

// subjectsOf are the subjects photos are filed under.
func subjectsOf(ctx context.Context, q *dbq.Queries, photos []dbq.Photo) (map[uuid.UUID]dbq.Subject, error) {
	var ids []uuid.UUID
	for _, p := range photos {
		if p.SubjectID != nil {
			ids = append(ids, *p.SubjectID)
		}
	}
	out := map[uuid.UUID]dbq.Subject{}
	if len(ids) == 0 {
		return out, nil
	}
	subs, err := q.GetSubjectsByIDs(ctx, dbq.GetSubjectsByIDsParams{TeamID: photos[0].TeamID, Ids: ids})
	if err != nil {
		return nil, internal(err)
	}
	for _, sub := range subs {
		out[sub.ID] = sub
	}
	return out, nil
}

func subjectProto(sub dbq.Subject, count int32) *pidexv1.Subject {
	var identity map[string]string
	_ = json.Unmarshal(sub.Identity, &identity)
	return &pidexv1.Subject{
		Id:              sub.ID.String(),
		TypeKey:         sub.TypeKey,
		DisplayName:     sub.DisplayName,
		Identity:        identity,
		ParentId:        idString(sub.ParentID),
		CreatedAt:       timestamppb.New(sub.CreatedAt),
		PhotoCount:      count,
		Profile:         sub.Profile,
		ProfileByPerson: sub.ProfileByPerson,
	}
}

func parseID(field, s string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%s is not a valid id", field))
	}
	return id, nil
}

func parseOptionalID(field, s string) (*uuid.UUID, error) {
	if s == "" {
		return nil, nil
	}
	id, err := parseID(field, s)
	return &id, err
}

func notFoundOr(err error, what string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return connect.NewError(connect.CodeNotFound, fmt.Errorf("%s not found", what))
	}
	return internal(err)
}

func internal(err error) error {
	return connect.NewError(connect.CodeInternal, err)
}

func invalid(format string, args ...any) error {
	return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf(format, args...))
}

// Page tokens are opaque to clients: a keyset position of created_at and id.
func encodePageToken(t time.Time, id uuid.UUID) string {
	return base64.RawURLEncoding.EncodeToString([]byte(t.UTC().Format(time.RFC3339Nano) + "|" + id.String()))
}

func decodePageToken(tok string) (*time.Time, *uuid.UUID, error) {
	if tok == "" {
		return nil, nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(tok)
	if err != nil {
		return nil, nil, invalid("bad page token")
	}
	tStr, idStr, ok := strings.Cut(string(raw), "|")
	if !ok {
		return nil, nil, invalid("bad page token")
	}
	t, err := time.Parse(time.RFC3339Nano, tStr)
	if err != nil {
		return nil, nil, invalid("bad page token")
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, nil, invalid("bad page token")
	}
	return &t, &id, nil
}

func pageSize(n int32) int32 {
	switch {
	case n <= 0:
		return 50
	case n > 200:
		return 200
	default:
		return n
	}
}
