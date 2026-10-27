package api

import (
	"context"
	"errors"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	pidexv1 "pidex/gen/pidex/v1"
	"pidex/internal/store/dbq"
)

// A photo's other files are kept beside its working copy, under its key.
var fileKinds = map[pidexv1.PhotoFileKind]struct {
	suffix string
	types  map[string]bool
}{
	pidexv1.PhotoFileKind_PHOTO_FILE_KIND_ORIGINAL: {".original", map[string]bool{"image/heic": true, "image/heif": true, "image/jpeg": true, "image/png": true}},
	pidexv1.PhotoFileKind_PHOTO_FILE_KIND_LIVE:     {".live", map[string]bool{"video/quicktime": true}},
}

// photoFile finds the photo a file is for, and checks the collection keeps
// files of its kind.
func (s *captureService) photoFile(ctx context.Context, pr Principal, rawID string, kind pidexv1.PhotoFileKind) (dbq.Photo, string, error) {
	k, ok := fileKinds[kind]
	if !ok {
		return dbq.Photo{}, "", invalid("kind must be original or live")
	}
	id, err := parseID("photo_id", rawID)
	if err != nil {
		return dbq.Photo{}, "", err
	}
	photo, err := s.Q.GetPhoto(ctx, dbq.GetPhotoParams{TeamID: pr.TeamID, ID: id})
	if err != nil {
		return dbq.Photo{}, "", notFoundOr(err, "photo")
	}
	if kind == pidexv1.PhotoFileKind_PHOTO_FILE_KIND_LIVE {
		cfg, _, err := s.Configs.Active(ctx, pr.TeamID)
		if err != nil {
			return dbq.Photo{}, "", connect.NewError(connect.CodeFailedPrecondition, err)
		}
		if !cfg.LivePhotos {
			return dbq.Photo{}, "", connect.NewError(connect.CodeFailedPrecondition, errors.New("this collection keeps no Live Photos"))
		}
	}
	return photo, photo.ObjectKey + k.suffix, nil
}

func (s *captureService) CreateFileUpload(ctx context.Context, req *connect.Request[pidexv1.CreateFileUploadRequest]) (*connect.Response[pidexv1.CreateFileUploadResponse], error) {
	pr := principalFrom(ctx)
	if err := requireWrite(pr); err != nil {
		return nil, err
	}
	m := req.Msg
	photo, key, err := s.photoFile(ctx, pr, m.PhotoId, m.Kind)
	if err != nil {
		return nil, err
	}
	switch {
	case !fileKinds[m.Kind].types[m.ContentType]:
		return nil, invalid("content_type %q is not kept for this kind of file", m.ContentType)
	case m.SizeBytes <= 0 || m.SizeBytes > s.MaxUploadBytes:
		return nil, invalid("size_bytes must be between 1 and %d", s.MaxUploadBytes)
	}
	live := m.Kind == pidexv1.PhotoFileKind_PHOTO_FILE_KIND_LIVE
	resp := &pidexv1.CreateFileUploadResponse{}
	if live && photo.LiveKey != nil || !live && photo.OriginalKey != nil {
		resp.AlreadyUploaded = true
		return connect.NewResponse(resp), nil
	}
	if !live {
		if err := s.Q.SetOriginalType(ctx, dbq.SetOriginalTypeParams{TeamID: pr.TeamID, ID: photo.ID, ContentType: &m.ContentType}); err != nil {
			return nil, internal(err)
		}
	}
	url, headers, err := s.Storage.PresignPut(ctx, key, m.ContentType, m.SizeBytes, s.UploadURLTTL)
	if err != nil {
		return nil, internal(err)
	}
	resp.UploadUrl, resp.UploadHeaders = url, headers
	resp.ExpiresAt = timestamppb.New(time.Now().Add(s.UploadURLTTL))
	return connect.NewResponse(resp), nil
}

func (s *captureService) CompleteFileUpload(ctx context.Context, req *connect.Request[pidexv1.CompleteFileUploadRequest]) (*connect.Response[pidexv1.CompleteFileUploadResponse], error) {
	pr := principalFrom(ctx)
	if err := requireWrite(pr); err != nil {
		return nil, err
	}
	m := req.Msg
	photo, key, err := s.photoFile(ctx, pr, m.PhotoId, m.Kind)
	if err != nil {
		return nil, err
	}
	size, exists, err := s.Storage.Head(ctx, key)
	if err != nil {
		return nil, internal(err)
	}
	if !exists {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("upload not received"))
	}
	var n int64
	if m.Kind == pidexv1.PhotoFileKind_PHOTO_FILE_KIND_LIVE {
		n, err = s.Q.ReceiveLive(ctx, dbq.ReceiveLiveParams{TeamID: pr.TeamID, ID: photo.ID, ObjectKey: &key, SizeBytes: &size})
	} else {
		n, err = s.Q.ReceiveOriginal(ctx, dbq.ReceiveOriginalParams{TeamID: pr.TeamID, ID: photo.ID, ObjectKey: &key, SizeBytes: &size})
	}
	if err != nil {
		return nil, internal(err)
	}
	if n == 0 {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("upload not received"))
	}
	return connect.NewResponse(&pidexv1.CompleteFileUploadResponse{}), nil
}
