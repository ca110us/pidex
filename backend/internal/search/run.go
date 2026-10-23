package search

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"

	"pidex/internal/ai"
	"pidex/internal/teamconfig"
)

// Searcher applies filters in the database and ranks by meaning.
type Searcher struct {
	Pool *pgxpool.Pool
	// Embedder is optional; without it text is matched by words only.
	Embedder ai.TextEmbedder
	Recorder ai.Recorder
	// Cutoffs depend on the embedding model; zero values take the defaults.
	Cutoffs Cutoffs
}

// Cutoffs are cosine distances that decide which vector matches count.
// They belong to an embedding model, not to a team or an industry: measure
// them for a new model with cmd/evalembed -gaps and cmd/evalsearch.
//
// Matches are kept relative to the best one, since similarity is not
// comparable across requests (a Chinese request scores lower against
// English descriptions) while the order within one request is reliable.
type Cutoffs struct {
	// Max is the distance beyond which nothing matches.
	Max float64
	// Relative keeps matches within this distance of the best match.
	Relative float64
	// Missing is stricter: in mode MISSING a match proves a subject is
	// covered.
	Missing float64
}

// DefaultCutoffs were measured for voyageai/voyage-4 at 1024 dimensions:
// relative cuts from 0.08 to 0.15 find the same photos, tighter ones
// return fewer stray photos.
var DefaultCutoffs = Cutoffs{Max: 0.55, Relative: 0.10, Missing: 0.45}

func (s *Searcher) cutoffs() Cutoffs {
	c := s.Cutoffs
	if c.Max <= 0 {
		c.Max = DefaultCutoffs.Max
	}
	if c.Relative <= 0 {
		c.Relative = DefaultCutoffs.Relative
	}
	if c.Missing <= 0 {
		c.Missing = DefaultCutoffs.Missing
	}
	return c
}

type Result struct {
	// SubjectIDs are the subjects filters.SubjectQuery resolved to.
	SubjectIDs      []uuid.UUID
	SubjectNotFound bool
	PhotoIDs        []uuid.UUID
	MissingIDs      []uuid.UUID
	// Total is how many photos (or, for missing, subjects) match in all,
	// of which the IDs are one page; 0 for a page past the end.
	Total int
	// TagCounts, when asked for, are how many matches have each tag.
	TagCounts []TagCount
}

type TagCount struct {
	Key   string
	Count int
}

// Page is which matches a search returns, and what else it counts.
type Page struct {
	Limit, Offset int
	// CountTags counts matches by tag, for narrowing by another tag. Only
	// searches without words count: theirs are all the matches there are.
	CountTags bool
}

// Rank fusion. Lists here are short, so a small k lets a clear first
// place count; tag matches are weighted by the share of the request's tags
// a photo has, and bare word matches count half (a common word matches
// many photos alike).
const (
	rrfK     = 10
	kwWeight = 0.5
)

// AsContent is the filters with the subject's words matched as what photos
// show: for words that name no subject on file ("iPad" where devices are
// filed by serial number, "yellow" for a cat).
func AsContent(f Filters) Filters {
	if f.SubjectQuery == "" {
		return f
	}
	f.Text = strings.TrimSpace(f.SubjectQuery + " " + f.Text)
	if f.Request != "" {
		f.Request = strings.TrimSpace(f.SubjectQuery + " " + f.Request)
	}
	return f
}

// RunPlanned runs a search a model planned from someone's words. AI tags
// miss things: when photos must show every tag the model asked for and
// none does, the tags only rank instead, so the words can still find the
// photo ("in the bag" where no photo is tagged as a carrier). The filters
// returned are those that found the photos, for paging on. Tags a person
// picks are not relaxed: they get what they picked.
func (s *Searcher) RunPlanned(ctx context.Context, teamID uuid.UUID, cfg *teamconfig.Config, f Filters, p Page) (Filters, Result, error) {
	res, err := s.RunPage(ctx, teamID, cfg, f, p)
	if err != nil || len(res.PhotoIDs) > 0 || p.Offset > 0 || !f.TagsRequired || len(f.Tags) == 0 || f.Mode != ModePhotos {
		return f, res, err
	}
	f.TagsRequired = false
	res, err = s.RunPage(ctx, teamID, cfg, f, p)
	return f, res, err
}

func (s *Searcher) Run(ctx context.Context, teamID uuid.UUID, cfg *teamconfig.Config, f Filters, limit int) (Result, error) {
	return s.RunPage(ctx, teamID, cfg, f, Page{Limit: limit})
}

// RunPage is Run from the offset-th match on, for paging through more
// matches than a page shows. Each page searches again: matches are not kept.
func (s *Searcher) RunPage(ctx context.Context, teamID uuid.UUID, cfg *teamconfig.Config, f Filters, p Page) (Result, error) {
	limit, offset := p.Limit, p.Offset
	var res Result
	subjects := f.SubjectIDs
	if len(subjects) == 0 && f.SubjectQuery != "" {
		ids, err := s.findSubjects(ctx, teamID, f.SubjectQuery, f.SubjectType)
		if err != nil {
			return res, err
		}
		switch {
		case len(ids) > 0:
			subjects = ids
		case f.Mode == ModeMissing:
			res.SubjectNotFound = true
			return res, nil
		default:
			// The words were not an identity on file ("iPad" where devices
			// are filed by serial number): say so, and match them as
			// content instead of returning nothing.
			res.SubjectNotFound = true
			f = AsContent(f)
		}
	}
	res.SubjectIDs = subjects

	// Rank by the request's own words; else by the text; else, with tags
	// only, by their names, so photos whose AI tags missed still match.
	meaning := f.Request
	if meaning == "" {
		meaning = f.Text
	}
	if meaning == "" && f.Mode == ModePhotos && !f.TagsRequired && cfg != nil {
		var names []string
		for _, k := range f.Tags {
			for _, t := range cfg.Tags {
				if t.Key == k {
					names = append(names, t.Name)
				}
			}
		}
		meaning = strings.Join(names, ", ")
	}
	var qvec *pgvector.Vector
	if meaning != "" && s.Embedder != nil {
		vecs, usage, err := s.Embedder.EmbedTexts(ctx, []string{meaning})
		if s.Recorder != nil {
			s.Recorder.Record(ctx, ai.CallRecord{CallMeta: ai.CallMeta{TeamID: teamID, Purpose: "search_embed"}, Usage: usage, Err: err})
		}
		if err == nil {
			v := pgvector.NewVector(vecs[0])
			qvec = &v
		}
		// Without a vector the words still match.
	}

	b := &sqlBuilder{args: []any{teamID}}
	scope := ""
	if len(subjects) > 0 {
		scope = fmt.Sprintf(`WITH RECURSIVE scope AS (
	SELECT id FROM subjects WHERE team_id = $1 AND id = ANY(%s)
	UNION SELECT s.id FROM subjects s JOIN scope ON s.parent_id = scope.id
)`, b.arg(subjects))
	}
	cut := s.cutoffs()
	photoConds := b.photoConditions(f, qvec, cut.Missing)

	if f.Mode == ModeMissing {
		var where []string
		if f.SubjectType != "" {
			where = append(where, "s.type_key = "+b.arg(f.SubjectType))
		}
		if scope != "" {
			where = append(where, "s.id IN (SELECT id FROM scope)")
		}
		sql := scope + `
SELECT s.id, count(*) OVER () FROM subjects s
WHERE s.team_id = $1 AND s.merged_into_id IS NULL` + and(where) + `
  AND NOT EXISTS (SELECT 1 FROM photos p WHERE p.subject_id = s.id AND p.team_id = $1` + and(photoConds) + `)
ORDER BY s.display_name
LIMIT ` + b.arg(max(limit, 200)) + ` OFFSET ` + b.arg(offset)
		ids, total, err := s.page(ctx, sql, b.args)
		res.MissingIDs, res.Total = ids, total
		return res, err
	}

	if scope != "" {
		photoConds = append(photoConds, "p.subject_id IN (SELECT id FROM scope)")
	}
	filtered := `filtered AS (
	SELECT p.id, p.subject_id, p.search_text, p.text_embedding, coalesce(p.captured_at, p.created_at) AS at
	FROM photos p
	WHERE p.team_id = $1` + and(photoConds) + `
)`
	with := "WITH " + filtered
	if scope != "" {
		with = scope + ", " + filtered
	}
	// Tags the user picked are conditions (photoConditions); only a
	// request's text is left to rank by.
	rankTags := f.Tags
	if f.TagsRequired {
		rankTags = nil
	}
	var sql string
	if f.Text == "" && len(rankTags) == 0 {
		if p.CountTags {
			counts, err := s.tagCounts(ctx, with+`
SELECT pt.tag_key, count(*) FROM filtered p
JOIN photo_tags pt ON pt.photo_id = p.id AND NOT pt.rejected
GROUP BY pt.tag_key ORDER BY count(*) DESC, pt.tag_key`, slices.Clone(b.args))
			if err != nil {
				return res, err
			}
			res.TagCounts = counts
		}
		sql = with + "\nSELECT id, count(*) OVER () FROM filtered ORDER BY at DESC LIMIT " + b.arg(limit) + " OFFSET " + b.arg(offset)
	} else {
		// Three ranked lists, fused: photos with the most of the tags,
		// photos whose text means the same, photos containing the words.
		// A photo with all the tags always matches; one with some of them
		// only when its text matches too.
		none := "SELECT NULL::uuid AS id, NULL::bigint AS r WHERE false"
		nearCTE := ""
		tag, vec, kw, look := "SELECT NULL::uuid AS id, NULL::bigint AS r, NULL::float AS share WHERE false", none, none, none
		if len(rankTags) > 0 {
			tag = `SELECT p.id, dense_rank() OVER (ORDER BY count(*) DESC) AS r, count(*)::float / ` + fmt.Sprint(len(rankTags)) + ` AS share
	FROM filtered p JOIN photo_tags pt ON pt.photo_id = p.id AND NOT pt.rejected AND pt.tag_key = ANY(` + b.arg(rankTags) + `)
	GROUP BY p.id ORDER BY r LIMIT 200`
		}
		if qvec != nil {
			q := b.arg(qvec)
			// near is the 200 closest, so its first row is the best match:
			// distances are computed once, not again for the cutoff.
			vec = fmt.Sprintf(`SELECT id, row_number() OVER (ORDER BY d) AS r FROM near
	WHERE d <= least(%[2]s, (SELECT min(d) FROM near) + %[3]s)`,
				q, b.arg(cut.Max), b.arg(cut.Relative))
			nearCTE = fmt.Sprintf(`near AS (
	SELECT id, text_embedding <=> %s AS d FROM filtered WHERE text_embedding IS NOT NULL ORDER BY d LIMIT 200
),
`, q)
			// What a subject looks like is in its profile, not in every
			// photo's words ("the cat with the black nose"): subjects whose
			// profile means the same bring their photos.
			nearCTE += fmt.Sprintf(`looks AS (
	SELECT id, profile_embedding <=> %[1]s AS d FROM subjects
	WHERE team_id = $1 AND merged_into_id IS NULL AND profile_embedding IS NOT NULL
	ORDER BY d LIMIT 20
),
`, q)
			look = fmt.Sprintf(`SELECT p.id, dense_rank() OVER (ORDER BY l.d) AS r FROM filtered p JOIN looks l ON l.id = p.subject_id
	WHERE l.d <= least(%[1]s, (SELECT min(d) FROM looks) + %[2]s)`, b.arg(cut.Max), b.arg(cut.Relative))
		}
		if f.Text != "" {
			text := b.arg(f.Text)
			kw = `SELECT id, row_number() OVER (ORDER BY word_similarity(` + text + `, search_text) DESC) AS r
	FROM filtered WHERE search_text ILIKE ` + b.arg("%"+escapeLike(f.Text)+"%") + ` OR ` + text + ` <% search_text
	ORDER BY r LIMIT 200`
		}
		k := fmt.Sprint(rrfK)
		sql = with + `,
` + nearCTE + `tag AS (` + tag + `),
vec AS (` + vec + `),
kw AS (` + kw + `),
look AS (` + look + `)
SELECT id, count(*) OVER () FROM tag FULL JOIN vec USING (id) FULL JOIN kw USING (id) FULL JOIN look USING (id)
-- A photo with only some of the tags needs the text to agree, so one
-- shared tag does not bring in every photo that has it.
WHERE tag.share >= 1 OR vec.r IS NOT NULL OR kw.r IS NOT NULL OR look.r IS NOT NULL
ORDER BY coalesce(tag.share / (` + k + ` + tag.r), 0) + coalesce(1.0 / (` + k + ` + vec.r), 0) + coalesce(` + fmt.Sprint(kwWeight) + ` / (` + k + ` + kw.r), 0)
    + coalesce(1.0 / (` + k + ` + look.r), 0) DESC
LIMIT ` + b.arg(limit) + ` OFFSET ` + b.arg(offset)
	}
	ids, total, err := s.page(ctx, sql, b.args)
	res.PhotoIDs, res.Total = ids, total
	return res, err
}

// photoConditions are the filters on one photo p, shared by photo search
// and by "missing" (where they say what a matching photo would be).
func (b *sqlBuilder) photoConditions(f Filters, qvec *pgvector.Vector, missingDistance float64) []string {
	conds := []string{"p.status = 'processed'", "coalesce(p.role, 'content') <> 'junk'"}
	if len(f.Within) > 0 {
		conds = append(conds, "p.id = ANY("+b.arg(f.Within)+")")
	}
	if f.TagsRequired && len(f.Tags) > 0 && f.Mode == ModePhotos {
		conds = append(conds, `NOT EXISTS (SELECT 1 FROM unnest(`+b.arg(f.Tags)+`::text[]) t(k)
		WHERE NOT EXISTS (SELECT 1 FROM photo_tags pt WHERE pt.photo_id = p.id AND pt.tag_key = t.k AND NOT pt.rejected))`)
	}
	if f.From != nil {
		conds = append(conds, "coalesce(p.captured_at, p.created_at) >= "+b.arg(*f.From))
	}
	if f.To != nil {
		conds = append(conds, "coalesce(p.captured_at, p.created_at) < "+b.arg(*f.To))
	}
	if len(f.Roles) > 0 {
		conds = append(conds, "p.role = ANY("+b.arg(f.Roles)+")")
	}
	// For photo search, tags and text rank; for "missing" they decide
	// whether a covering photo exists: any of the tags, else the text.
	if f.Mode == ModeMissing {
		switch {
		case len(f.Tags) > 0:
			conds = append(conds, "EXISTS (SELECT 1 FROM photo_tags pt WHERE pt.photo_id = p.id AND pt.tag_key = ANY("+b.arg(f.Tags)+") AND NOT pt.rejected)")
		case f.Text != "":
			cond := "p.search_text ILIKE " + b.arg("%"+escapeLike(f.Text)+"%")
			if qvec != nil {
				cond += " OR p.text_embedding <=> " + b.arg(qvec) + " < " + b.arg(missingDistance)
			}
			conds = append(conds, "("+cond+")")
		}
	}
	return conds
}

// findSubjects resolves the words naming a subject: an identity value equal
// after normalization (a word of the request equals a stored code or
// number), or a name containing or resembling the words.
func (s *Searcher) findSubjects(ctx context.Context, teamID uuid.UUID, query, typeKey string) ([]uuid.UUID, error) {
	norm := normalizeToken(query)
	var tokens []string
	for _, w := range strings.FieldsFunc(query, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if t := normalizeToken(w); len([]rune(t)) >= 2 {
			tokens = append(tokens, t)
		}
	}
	const sql = `
SELECT id FROM (
	SELECT s.id,
		CASE
			WHEN EXISTS (SELECT 1 FROM jsonb_each_text(s.identity_norm) v WHERE lower(v.value) = $2 OR lower(v.value) = ANY($3)) THEN 3
			WHEN s.display_name ILIKE $4 OR s.identity::text ILIKE $4 THEN 2
			ELSE word_similarity($5, s.display_name)
		END AS score
	FROM subjects s
	WHERE s.team_id = $1 AND s.merged_into_id IS NULL AND ($6 = '' OR s.type_key = $6)
) m
WHERE score >= 0.5
ORDER BY score DESC
LIMIT 10`
	return s.ids(ctx, sql, []any{teamID, norm, tokens, "%" + escapeLike(strings.TrimSpace(query)) + "%", query, typeKey})
}

// normalizeToken is compared with lower(identity_norm values): normalized
// ids keep only letters and digits ("IT-004512" is stored as IT004512).
func normalizeToken(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (s *Searcher) ids(ctx context.Context, sql string, args []any) ([]uuid.UUID, error) {
	rows, err := s.Pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}

func (s *Searcher) tagCounts(ctx context.Context, sql string, args []any) ([]TagCount, error) {
	rows, err := s.Pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("search: tag counts: %w", err)
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (TagCount, error) {
		var c TagCount
		var n int64
		err := r.Scan(&c.Key, &n)
		c.Count = int(n)
		return c, err
	})
	if err != nil {
		return nil, fmt.Errorf("search: tag counts: %w", err)
	}
	return out, nil
}

// page reads rows of an ID and the count of all matches.
func (s *Searcher) page(ctx context.Context, sql string, args []any) ([]uuid.UUID, int, error) {
	rows, err := s.Pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("search: %w", err)
	}
	defer rows.Close()
	var ids []uuid.UUID
	var total int64
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id, &total); err != nil {
			return nil, 0, fmt.Errorf("search: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("search: %w", err)
	}
	return ids, int(total), nil
}

type sqlBuilder struct{ args []any }

func (b *sqlBuilder) arg(v any) string {
	b.args = append(b.args, v)
	return fmt.Sprintf("$%d", len(b.args))
}

func and(conds []string) string {
	if len(conds) == 0 {
		return ""
	}
	return "\n  AND " + strings.Join(conds, "\n  AND ")
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
