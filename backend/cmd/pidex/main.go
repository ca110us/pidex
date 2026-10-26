// Command pidex runs the API server and its workers, migrates the
// database, and seeds development teams.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"pidex/internal/ai"
	"pidex/internal/api"
	"pidex/internal/appconfig"
	"pidex/internal/configstore"
	"pidex/internal/jobs"
	"pidex/internal/looks"
	"pidex/internal/pipeline"
	"pidex/internal/search"
	"pidex/internal/storage"
	"pidex/internal/store"
	"pidex/internal/store/dbq"
	"pidex/internal/teamconfig"
)

const usage = `usage:
  pidex serve
  pidex migrate up|down|status
  pidex seed -team NAME -email EMAIL [-name NAME] [-template KEY]
  pidex reindex   (queue processed photos that need thumbnails or search vectors)
  pidex embed-looks [-all]   (queue photos of subjects told apart by look for the look service; -all also those embedded by another model)
  pidex name-descriptions   (queue photos of pets and the like described before descriptions used their names)
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := appconfig.FromEnv()
	if err != nil {
		log.Error("configuration", "err", err)
		os.Exit(2)
	}
	switch os.Args[1] {
	case "serve":
		err = serve(ctx, cfg, log)
	case "migrate":
		err = migrate(ctx, cfg, os.Args[2:])
	case "seed":
		err = seed(ctx, cfg, os.Args[2:])
	case "reindex":
		err = reindex(ctx, cfg, log)
	case "name-descriptions":
		err = nameDescriptions(ctx, cfg, log)
	case "embed-looks":
		err = embedLooks(ctx, cfg, log, slices.Contains(os.Args[2:], "-all"))
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		log.Error(os.Args[1]+" failed", "err", err)
		os.Exit(1)
	}
}

func migrate(ctx context.Context, cfg appconfig.Config, args []string) error {
	if len(args) != 1 {
		return errors.New("migrate needs one of: up, down, status")
	}
	switch args[0] {
	case "up":
		return store.MigrateUp(ctx, cfg.DatabaseURL, os.Stdout)
	case "down":
		return store.MigrateDown(ctx, cfg.DatabaseURL, os.Stdout)
	case "status":
		return store.MigrateStatus(ctx, cfg.DatabaseURL, os.Stdout)
	}
	return fmt.Errorf("unknown migrate command %q", args[0])
}

func seed(ctx context.Context, cfg appconfig.Config, args []string) error {
	fs := flag.NewFlagSet("seed", flag.ContinueOnError)
	team := fs.String("team", "", "team name")
	email := fs.String("email", "", "owner email")
	name := fs.String("name", "", "owner display name")
	template := fs.String("template", "", "config template key")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *team == "" || *email == "" || *template == "" {
		tpls, _ := teamconfig.Templates()
		var keys []string
		for _, t := range tpls {
			keys = append(keys, t.Key)
		}
		return fmt.Errorf("-team, -email and -template are required (templates: %s); a team can also start from a description in the app", strings.Join(keys, ", "))
	}
	if *name == "" {
		*name = *email
	}
	tcfg, err := teamconfig.LoadTemplate(*template)
	if err != nil {
		return err
	}
	pool, err := store.OpenPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	q := dbq.New(pool)

	t, err := q.CreateTeam(ctx, dbq.CreateTeamParams{Name: *team, DataRegion: "us"})
	if err != nil {
		return err
	}
	u, err := q.UpsertUser(ctx, dbq.UpsertUserParams{Email: *email, DisplayName: *name})
	if err != nil {
		return err
	}
	if err := q.AddTeamMember(ctx, dbq.AddTeamMemberParams{TeamID: t.ID, UserID: u.ID, Role: "owner"}); err != nil {
		return err
	}
	token, hash := api.NewToken()
	if err := q.CreateAPIToken(ctx, dbq.CreateAPITokenParams{TokenSha256: hash, TeamID: t.ID, UserID: u.ID, Label: "seed"}); err != nil {
		return err
	}
	row, err := configstore.Publish(ctx, pool, t.ID, &u.ID, tcfg, "template:"+*template)
	if err != nil {
		return err
	}
	fmt.Printf("team:     %s (%s)\nconfig:   %s v%d\ntoken:    %s\n", t.Name, t.ID, *template, row.Version, token)
	return nil
}

// reindex queues every processed photo that was never indexed, or was
// embedded by another model than the configured one. A running server
// works the queue.
func reindex(ctx context.Context, cfg appconfig.Config, log *slog.Logger) error {
	pool, err := store.OpenPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	model := ""
	if cfg.AIProvider == "openrouter" && cfg.TextEmbeddingModel != "" {
		model = "openrouter/" + cfg.TextEmbeddingModel
	}
	ids, err := dbq.New(pool).ListPhotosToIndex(ctx, model)
	if err != nil {
		return err
	}
	queue, err := jobs.New(pool, nil, nil, 0, log)
	if err != nil {
		return err
	}
	for start := 0; start < len(ids); start += 500 {
		if err := queue.EnqueueIndexMany(ctx, ids[start:min(start+500, len(ids))]); err != nil {
			return err
		}
	}
	fmt.Printf("queued %d photos for indexing\n", len(ids))
	return nil
}

// embedLooks queues photos of subjects told apart by look that have no
// look yet (all of them with all) for the server to send to the look
// service: photos filed before it ran, or embedded by another model.
func embedLooks(ctx context.Context, cfg appconfig.Config, log *slog.Logger, all bool) error {
	pool, err := store.OpenPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	ids, err := dbq.New(pool).ListPhotosToEmbedLook(ctx, all)
	if err != nil {
		return err
	}
	queue, err := jobs.New(pool, nil, nil, 0, log)
	if err != nil {
		return err
	}
	for start := 0; start < len(ids); start += 500 {
		if err := queue.EnqueueEmbedLooks(ctx, ids[start:min(start+500, len(ids))]); err != nil {
			return err
		}
	}
	fmt.Printf("queued %d photos for the look service\n", len(ids))
	return nil
}

// nameDescriptions queues photos described before descriptions named their
// subject; the server describes them again, about $0.001 each.
func nameDescriptions(ctx context.Context, cfg appconfig.Config, log *slog.Logger) error {
	pool, err := store.OpenPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	ids, err := dbq.New(pool).ListPhotosToName(ctx)
	if err != nil {
		return err
	}
	queue, err := jobs.New(pool, nil, nil, 0, log)
	if err != nil {
		return err
	}
	for start := 0; start < len(ids); start += 500 {
		if err := queue.EnqueueNameDescriptions(ctx, ids[start:min(start+500, len(ids))]); err != nil {
			return err
		}
	}
	fmt.Printf("queued %d photos to be described by name\n", len(ids))
	return nil
}

func serve(ctx context.Context, cfg appconfig.Config, log *slog.Logger) error {
	s3cfg := storage.Config{
		Endpoint: cfg.S3Endpoint, PublicEndpoint: cfg.S3PublicEndpoint, Region: cfg.S3Region,
		Bucket: cfg.S3Bucket, AccessKey: cfg.S3AccessKey, SecretKey: cfg.S3SecretKey, PathStyle: cfg.S3PathStyle,
	}
	devS3 := cfg.S3Endpoint == ""
	if devS3 {
		fake, err := storage.StartDevS3(cfg.DevS3Addr, cfg.DevS3Dir)
		if err != nil {
			return err
		}
		defer fake.Close()
		log.Warn("using in-process dev S3; not for production", "url", fake.URL, "dir", cfg.DevS3Dir)
		s3cfg.Endpoint, s3cfg.AccessKey, s3cfg.SecretKey, s3cfg.PathStyle = fake.URL, "dev", "dev-secret", true
		if s3cfg.PublicEndpoint == "" {
			s3cfg.PublicEndpoint = fake.URL
		}
	}
	objects, err := storage.NewS3(ctx, s3cfg)
	if err != nil {
		return err
	}
	if devS3 {
		if err := objects.EnsureBucket(ctx); err != nil {
			return err
		}
	}

	pool, err := store.OpenPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	q := dbq.New(pool)
	configs := configstore.New(q)
	rec := &ai.DBRecorder{Q: q, Log: log}

	ms, err := models(ctx, cfg, rec, log)
	if err != nil {
		return err
	}
	proc := &pipeline.Processor{
		Pool: pool, Configs: configs, Storage: objects, Vision: ms.vision, Strong: ms.strong, Match: ms.match,
		Embedder: ms.embedder, TextEmbedder: ms.textEmbedder, Recorder: rec, Log: log,
		MaxImageEdge: cfg.MaxImageEdge, MaxObjectBytes: cfg.MaxUploadBytes,
		LookAutoSimilarity: cfg.LookAutoSimilarity, LookMargin: cfg.LookMargin,
	}
	if cfg.LookServiceURL != "" {
		proc.Looks = looks.New(cfg.LookServiceURL)
		log.Info("telling subjects apart by look", "service", cfg.LookServiceURL)
	}
	ix := &search.Indexer{
		Pool: pool, Configs: configs, Storage: objects, Embedder: ms.textEmbedder, Recorder: rec, Log: log,
		MaxObjectBytes: cfg.MaxUploadBytes,
	}
	queue, err := jobs.New(pool, proc, ix, cfg.Workers, log)
	if err != nil {
		return err
	}
	if err := queue.Start(ctx); err != nil {
		return err
	}

	srv := &api.Server{
		Pool: pool, Q: q, Configs: configs, Storage: objects, Jobs: queue, Processor: proc, Log: log,
		ConfigModel: ms.config, SearchModel: ms.search,
		Search: &search.Searcher{Pool: pool, Embedder: ms.textEmbedder, Recorder: rec, Cutoffs: search.Cutoffs{
			Max: cfg.SearchMaxDistance, Relative: cfg.SearchRelativeDistance, Missing: cfg.SearchMissingDistance,
		}},
		MaxUploadBytes: cfg.MaxUploadBytes, UploadURLTTL: 15 * time.Minute,
	}
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)
	httpSrv := &http.Server{
		Addr: cfg.HTTPAddr, Handler: srv.Handler(), Protocols: protocols,
		ReadHeaderTimeout: 10 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() { errCh <- httpSrv.ListenAndServe() }()
	log.Info("serving", "addr", cfg.HTTPAddr, "vision", ms.vision.Primary(), "config", ms.config.Primary())

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutdownCtx)
	return queue.Stop(shutdownCtx)
}

type modelSet struct {
	vision, strong, match, config, search *ai.Router
	embedder                              ai.ImageEmbedder
	textEmbedder                          ai.TextEmbedder
}

func models(ctx context.Context, cfg appconfig.Config, rec ai.Recorder, log *slog.Logger) (ms modelSet, err error) {
	price := func(model string) ai.Price {
		if cfg.VisionPriceIn > 0 || cfg.VisionPriceOut > 0 {
			return ai.Price{InputPerM: cfg.VisionPriceIn, OutputPerM: cfg.VisionPriceOut}
		}
		return ai.GeminiPrices[strings.TrimPrefix(model, "google/")]
	}
	newModel := func(model string) (ai.VisionModel, error) {
		switch cfg.AIProvider {
		case "openrouter":
			// OpenRouter reports the billed cost; the price table is a fallback.
			return &ai.OpenAICompatible{
				Name: "openrouter", BaseURL: ai.OpenRouterBaseURL, APIKey: cfg.OpenRouterAPIKey,
				ModelID: model, Price: price(model), ReportCost: true,
				Headers: map[string]string{"X-Title": "Pidex"},
			}, nil
		default:
			p := price(model)
			if p == (ai.Price{}) {
				log.Warn("no price known for model; cost will be recorded as 0", "model", model)
			}
			return ai.NewGemini(ctx, cfg.GeminiAPIKey, model, p)
		}
	}

	if cfg.AIProvider == "" {
		log.Warn("no AI provider key set: photos are filed by barcode anchors only, with no tags")
		ms.vision = ai.NewRouter(rec, log)
		ms.config = ai.NewRouter(rec, log)
		ms.search = ai.NewRouter(rec, log)
	} else {
		var chain []ai.VisionModel
		for _, m := range append([]string{cfg.VisionModel}, cfg.VisionFallbackModels...) {
			vm, err := newModel(m)
			if err != nil {
				return ms, err
			}
			chain = append(chain, vm)
		}
		ms.vision = ai.NewRouter(rec, log, chain...)
		if cfg.StrongModel != "" {
			sm, err := newModel(cfg.StrongModel)
			if err != nil {
				return ms, err
			}
			ms.strong = ai.NewRouter(rec, log, sm)
		}
		if cfg.MatchModel != "" {
			// The vision fallbacks stand in when the match model is
			// rate-limited or down, so a comparison is not simply lost.
			var chain []ai.VisionModel
			for _, m := range append([]string{cfg.MatchModel}, cfg.VisionFallbackModels...) {
				vm, err := newModel(m)
				if err != nil {
					return ms, err
				}
				chain = append(chain, vm)
			}
			ms.match = ai.NewRouter(rec, log, chain...)
		}
		cm, err := newModel(cfg.ConfigModel)
		if err != nil {
			return ms, err
		}
		ms.config = ai.NewRouter(rec, log, cm)
		sm, err := newModel(cfg.SearchModel)
		if err != nil {
			return ms, err
		}
		ms.search = ai.NewRouter(rec, log, sm)
	}
	if cfg.AIProvider == "openrouter" && cfg.TextEmbeddingModel != "" {
		ms.textEmbedder = &ai.OpenAICompatible{
			Name: "openrouter", BaseURL: ai.OpenRouterBaseURL, APIKey: cfg.OpenRouterAPIKey,
			ModelID: cfg.TextEmbeddingModel, EmbeddingDimensions: search.EmbeddingDimensions,
			Headers: map[string]string{"X-Title": "Pidex"},
		}
	} else {
		log.Warn("no text embedding model: search matches words only")
	}
	if cfg.EmbeddingModel != "" {
		if cfg.GeminiAPIKey == "" {
			return ms, errors.New("EMBEDDING_MODEL needs GEMINI_API_KEY")
		}
		// Gemini Embedding 2 list price per image, checked 2026-10-19.
		if ms.embedder, err = ai.NewGeminiEmbedder(ctx, cfg.GeminiAPIKey, cfg.EmbeddingModel, 768, 0.00012); err != nil {
			return ms, err
		}
	}
	return ms, nil
}
