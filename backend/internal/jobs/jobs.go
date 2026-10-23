// Package jobs runs pipeline work on a Postgres-backed queue (River). Jobs
// are inserted in the same transaction as the row they refer to, so an
// upload is never recorded without its processing job, or the reverse.
package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"

	"pidex/internal/pipeline"
	"pidex/internal/search"
)

type ProcessPhotoArgs struct {
	PhotoID uuid.UUID `json:"photo_id"`
}

func (ProcessPhotoArgs) Kind() string { return "process_photo" }

func (ProcessPhotoArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 5, UniqueOpts: river.UniqueOpts{ByArgs: true}}
}

type processPhotoWorker struct {
	river.WorkerDefaults[ProcessPhotoArgs]
	p *pipeline.Processor
}

// IndexPhotoArgs makes a processed photo findable. It runs apart from
// processing so a failing embedding model never holds up filing.
type IndexPhotoArgs struct {
	PhotoID uuid.UUID `json:"photo_id"`
}

func (IndexPhotoArgs) Kind() string { return "index_photo" }

// Index jobs are not unique: indexing is idempotent and cheap, and a
// change made while a job runs must still get its own run.
func (IndexPhotoArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 8}
}

type indexPhotoWorker struct {
	river.WorkerDefaults[IndexPhotoArgs]
	ix *search.Indexer
}

// NameDescriptionArgs describes a photo again, by its subject's name.
type NameDescriptionArgs struct {
	PhotoID uuid.UUID `json:"photo_id"`
}

func (NameDescriptionArgs) Kind() string { return "name_description" }

func (NameDescriptionArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 3, UniqueOpts: river.UniqueOpts{ByArgs: true}}
}

type nameDescriptionWorker struct {
	river.WorkerDefaults[NameDescriptionArgs]
	p *pipeline.Processor
}

func (w *nameDescriptionWorker) Work(ctx context.Context, job *river.Job[NameDescriptionArgs]) error {
	return w.p.NameDescription(ctx, job.Args.PhotoID)
}

// ProfileSubjectsArgs writes the profiles of subjects told apart by look
// that have none yet or have doubled their photos since (see
// pipeline.ProfileSubjects). It runs every few minutes, not per photo: a
// shoot of twenty photos asks for one profile, not twenty.
type ProfileSubjectsArgs struct{}

func (ProfileSubjectsArgs) Kind() string { return "profile_subjects" }

func (ProfileSubjectsArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 1, UniqueOpts: river.UniqueOpts{ByState: []rivertype.JobState{
		rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRunning, rivertype.JobStateScheduled,
	}}}
}

// profileEvery is how often subjects are looked at for a profile.
const profileEvery = 2 * time.Minute

type profileSubjectsWorker struct {
	river.WorkerDefaults[ProfileSubjectsArgs]
	p *pipeline.Processor
}

func (w *profileSubjectsWorker) Timeout(*river.Job[ProfileSubjectsArgs]) time.Duration {
	return 5 * time.Minute
}

func (w *profileSubjectsWorker) Work(ctx context.Context, _ *river.Job[ProfileSubjectsArgs]) error {
	return w.p.ProfileSubjects(ctx)
}

func (w *indexPhotoWorker) Work(ctx context.Context, job *river.Job[IndexPhotoArgs]) error {
	return w.ix.IndexPhoto(ctx, job.Args.PhotoID)
}

func (w *processPhotoWorker) Timeout(*river.Job[ProcessPhotoArgs]) time.Duration {
	return 3 * time.Minute
}

func (w *processPhotoWorker) Work(ctx context.Context, job *river.Job[ProcessPhotoArgs]) error {
	err := w.p.ProcessPhoto(ctx, job.Args.PhotoID)
	if err != nil && job.Attempt >= job.MaxAttempts {
		if markErr := w.p.MarkFailed(ctx, job.Args.PhotoID, err); markErr != nil {
			w.p.Log.Error("mark photo failed", "photo", job.Args.PhotoID, "err", markErr)
		}
	}
	if err != nil {
		return err
	}
	_, err = river.ClientFromContext[pgx.Tx](ctx).Insert(ctx, IndexPhotoArgs{PhotoID: job.Args.PhotoID}, nil)
	return err
}

type Queue struct {
	client *river.Client[pgx.Tx]
}

// New builds the queue. With a processor it also works jobs; without one
// it can only insert them.
func New(pool *pgxpool.Pool, p *pipeline.Processor, ix *search.Indexer, workers int, log *slog.Logger) (*Queue, error) {
	cfg := &river.Config{Logger: log}
	if p != nil {
		w := river.NewWorkers()
		river.AddWorker(w, &processPhotoWorker{p: p})
		river.AddWorker(w, &indexPhotoWorker{ix: ix})
		river.AddWorker(w, &nameDescriptionWorker{p: p})
		river.AddWorker(w, &profileSubjectsWorker{p: p})
		cfg.Workers = w
		cfg.PeriodicJobs = []*river.PeriodicJob{river.NewPeriodicJob(
			river.PeriodicInterval(profileEvery),
			func() (river.JobArgs, *river.InsertOpts) { return ProfileSubjectsArgs{}, nil },
			&river.PeriodicJobOpts{RunOnStart: true},
		)}
		cfg.Queues = map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: workers}}
		cfg.FetchCooldown = 50 * time.Millisecond
	}
	c, err := river.NewClient(riverpgxv5.New(pool), cfg)
	if err != nil {
		return nil, err
	}
	return &Queue{client: c}, nil
}

func (q *Queue) Start(ctx context.Context) error { return q.client.Start(ctx) }
func (q *Queue) Stop(ctx context.Context) error  { return q.client.Stop(ctx) }

func (q *Queue) EnqueueProcessPhoto(ctx context.Context, tx pgx.Tx, photoID uuid.UUID) error {
	_, err := q.client.InsertTx(ctx, tx, ProcessPhotoArgs{PhotoID: photoID}, nil)
	return err
}

func (q *Queue) EnqueueIndexPhoto(ctx context.Context, tx pgx.Tx, photoID uuid.UUID) error {
	_, err := q.client.InsertTx(ctx, tx, IndexPhotoArgs{PhotoID: photoID}, nil)
	return err
}

// EnqueueNameDescriptions queues photos to be described by name.
func (q *Queue) EnqueueNameDescriptions(ctx context.Context, ids []uuid.UUID) error {
	params := make([]river.InsertManyParams, len(ids))
	for i, id := range ids {
		params[i] = river.InsertManyParams{Args: NameDescriptionArgs{PhotoID: id}}
	}
	_, err := q.client.InsertMany(ctx, params)
	return err
}

// EnqueueIndexMany queues photos for (re)indexing, outside a transaction.
func (q *Queue) EnqueueIndexMany(ctx context.Context, ids []uuid.UUID) error {
	params := make([]river.InsertManyParams, len(ids))
	for i, id := range ids {
		params[i] = river.InsertManyParams{Args: IndexPhotoArgs{PhotoID: id}}
	}
	_, err := q.client.InsertMany(ctx, params)
	return err
}
