package search

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"

	"pidex/internal/ai"
	"pidex/internal/analysis"
	"pidex/internal/configstore"
	"pidex/internal/imaging"
	"pidex/internal/storage"
	"pidex/internal/store/dbq"
)

// EmbeddingDimensions is the size of photos.text_embedding.
const EmbeddingDimensions = 1024

// ThumbEdge is the long edge of thumbnails, enough for a three-column grid
// on a phone at 3x.
const ThumbEdge = 480

// Indexer makes a processed photo findable: a thumbnail for browsing, the
// text it is searched by, and a vector of that text.
type Indexer struct {
	Pool    *pgxpool.Pool
	Configs *configstore.Store
	Storage *storage.S3
	// Embedder is optional; without it search matches words only.
	Embedder ai.TextEmbedder
	Recorder ai.Recorder
	Log      *slog.Logger

	MaxObjectBytes int64
}

func (ix *Indexer) IndexPhoto(ctx context.Context, photoID uuid.UUID) error {
	q := dbq.New(ix.Pool)
	photo, err := q.GetPhotoByID(ctx, photoID)
	if errors.Is(err, pgx.ErrNoRows) {
		// Deleted with its collection while waiting: nothing to do.
		return nil
	}
	if err != nil {
		return fmt.Errorf("load photo: %w", err)
	}
	if photo.Status != "processed" || photo.ConfigVersion == nil {
		return nil
	}
	if photo.ThumbKey == nil {
		if err := ix.thumbnail(ctx, q, photo); err != nil {
			return err
		}
	}

	cfg, err := ix.Configs.Version(ctx, photo.TeamID, *photo.ConfigVersion)
	if err != nil {
		return err
	}
	tags, err := q.ListPhotoTags(ctx, []uuid.UUID{photo.ID})
	if err != nil {
		return err
	}
	in := DocInput{Description: photo.Description, DocumentFields: documentFields(photo.Extracted), Place: PlaceWords(photo.Place)}
	for _, t := range tags {
		in.TagKeys = append(in.TagKeys, t.TagKey)
	}
	text := Document(cfg, in)

	params := dbq.SetPhotoIndexParams{ID: photo.ID, SearchText: text}
	// Junk is not worth a vector: it is hidden from search anyway.
	if ix.Embedder != nil && text != "" && (photo.Role == nil || *photo.Role != string(analysis.RoleJunk)) {
		vecs, usage, err := ix.Embedder.EmbedTexts(ctx, []string{text})
		if ix.Recorder != nil {
			ix.Recorder.Record(ctx, ai.CallRecord{CallMeta: ai.CallMeta{TeamID: photo.TeamID, PhotoID: &photo.ID, Purpose: "embed_text"}, Usage: usage, Err: err})
		}
		if err != nil {
			return fmt.Errorf("embed: %w", err)
		}
		v := pgvector.NewVector(vecs[0])
		model := ix.Embedder.Provider() + "/" + ix.Embedder.Model()
		params.TextEmbedding, params.TextEmbeddingModel = &v, &model
	}
	return q.SetPhotoIndex(ctx, params)
}

func (ix *Indexer) thumbnail(ctx context.Context, q *dbq.Queries, photo dbq.Photo) error {
	data, err := ix.Storage.Get(ctx, photo.ObjectKey, ix.MaxObjectBytes)
	if errors.Is(err, storage.ErrNotFound) {
		// Nothing to show, but the photo's text is still worth indexing.
		ix.Log.Warn("original missing; no thumbnail", "photo", photo.ID)
		return nil
	}
	if err != nil {
		return err
	}
	thumb, err := imaging.Fit(data, ThumbEdge)
	if err != nil {
		// An undecodable original stays browsable by its full view URL.
		ix.Log.Warn("thumbnail failed", "photo", photo.ID, "err", err)
		return nil
	}
	key := "thumbs/" + photo.ObjectKey
	if err := ix.Storage.Put(ctx, key, "image/jpeg", thumb); err != nil {
		return err
	}
	return q.SetPhotoThumb(ctx, dbq.SetPhotoThumbParams{ID: photo.ID, ThumbKey: &key})
}

func documentFields(extracted []byte) map[string]string {
	var ex struct {
		DocumentFields []analysis.DocumentField `json:"document_fields"`
	}
	if json.Unmarshal(extracted, &ex) != nil || len(ex.DocumentFields) == 0 {
		return nil
	}
	out := map[string]string{}
	for _, d := range ex.DocumentFields {
		out[d.Name] = d.Value
	}
	return out
}

// PlaceWords is where a photo was taken, in words, widest first: "China ·
// Liaoning · Dalian · Shahekou · Xinghai Square". Empty without a place.
func PlaceWords(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]string
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	var parts []string
	for _, k := range []string{"country", "region", "city", "district", "name"} {
		if v := strings.TrimSpace(m[k]); v != "" && !slices.Contains(parts, v) {
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, " · ")
}
