// Package appconfig reads process configuration from the environment.
package appconfig

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	DatabaseURL string
	HTTPAddr    string

	S3Endpoint       string
	S3PublicEndpoint string
	S3Region         string
	S3Bucket         string
	S3AccessKey      string
	S3SecretKey      string
	S3PathStyle      bool
	// With no S3 endpoint, an in-process dev S3 is started here.
	DevS3Addr string
	DevS3Dir  string

	// AIProvider is "gemini" or "openrouter". Empty picks whichever key is
	// set, preferring OpenRouter.
	AIProvider       string
	GeminiAPIKey     string
	OpenRouterAPIKey string
	VisionModel      string
	// VisionFallbackModels are tried in order when VisionModel fails.
	VisionFallbackModels []string
	StrongModel          string
	// MatchModel tells apart subjects that have no code, by comparing
	// photos of them; VisionModel does it when unset.
	MatchModel string
	// ConfigModel drafts team configs from descriptions: rare calls where
	// quality matters more than price.
	ConfigModel string
	// SearchModel turns a search request into filters: frequent, small
	// text-only calls where speed matters.
	SearchModel string
	// TextEmbeddingModel embeds each photo's search text (OpenRouter only).
	TextEmbeddingModel string
	// SearchCutoffs override the vector cutoffs measured for the default
	// embedding model; set them when changing TextEmbeddingModel.
	SearchMaxDistance, SearchRelativeDistance, SearchMissingDistance float64
	// EmbeddingModel uses the Gemini API directly and needs GEMINI_API_KEY.
	EmbeddingModel string
	// LookServiceURL is the look service (vision/), which turns a photo of
	// a subject told apart by appearance into a vector; without it those
	// subjects are matched by words and a vision model alone.
	LookServiceURL string
	// LookAutoSimilarity and LookMargin decide when a photo is filed by
	// its look alone: the closest subject at least this similar, and this
	// much more similar than the next. Zero takes the defaults measured
	// for the default model (pipeline.DefaultLook*).
	LookAutoSimilarity, LookMargin float64
	// Optional price overrides, USD per million tokens.
	VisionPriceIn  float64
	VisionPriceOut float64

	// Braintrust receives a trace of each conversation turn, search and
	// photo processed, with every model call in it. No key, no traces.
	BraintrustAPIKey    string
	BraintrustAPIURL    string
	BraintrustProject   string
	BraintrustProjectID string
	// BraintrustContent sends what people wrote and models answered, for
	// teams whose privacy is not high; off, only timings, tokens and ids.
	BraintrustContent bool

	Workers        int
	MaxImageEdge   int
	MaxUploadBytes int64
}

func FromEnv() (Config, error) {
	c := Config{
		DatabaseURL:        env("DATABASE_URL", "postgres://pidex:pidex@localhost:55432/pidex?sslmode=disable"),
		HTTPAddr:           env("HTTP_ADDR", ":8080"),
		S3Endpoint:         os.Getenv("S3_ENDPOINT"),
		S3PublicEndpoint:   os.Getenv("S3_PUBLIC_ENDPOINT"),
		S3Region:           env("S3_REGION", "us-east-1"),
		S3Bucket:           env("S3_BUCKET", "pidex"),
		S3AccessKey:        os.Getenv("S3_ACCESS_KEY"),
		S3SecretKey:        os.Getenv("S3_SECRET_KEY"),
		DevS3Addr:          env("DEV_S3_ADDR", "127.0.0.1:59000"),
		DevS3Dir:           env("DEV_S3_DIR", ".data/s3"),
		AIProvider:         os.Getenv("AI_PROVIDER"),
		GeminiAPIKey:       os.Getenv("GEMINI_API_KEY"),
		OpenRouterAPIKey:   os.Getenv("OPENROUTER_API_KEY"),
		VisionModel:        os.Getenv("VISION_MODEL"),
		StrongModel:        os.Getenv("STRONG_VISION_MODEL"),
		MatchModel:         os.Getenv("MATCH_MODEL"),
		ConfigModel:        os.Getenv("CONFIG_MODEL"),
		LookServiceURL:     os.Getenv("LOOK_SERVICE_URL"),
		SearchModel:        os.Getenv("SEARCH_MODEL"),
		TextEmbeddingModel: os.Getenv("TEXT_EMBEDDING_MODEL"),
		EmbeddingModel:     os.Getenv("EMBEDDING_MODEL"),

		BraintrustAPIKey:    os.Getenv("BRAINTRUST_API_KEY"),
		BraintrustAPIURL:    env("BRAINTRUST_API_URL", "https://api.braintrust.dev"),
		BraintrustProject:   env("BRAINTRUST_PROJECT", "pidex"),
		BraintrustProjectID: os.Getenv("BRAINTRUST_PROJECT_ID"),
	}
	for _, m := range strings.Split(os.Getenv("VISION_FALLBACK_MODELS"), ",") {
		if m = strings.TrimSpace(m); m != "" {
			c.VisionFallbackModels = append(c.VisionFallbackModels, m)
		}
	}
	if c.AIProvider == "" {
		switch {
		case c.OpenRouterAPIKey != "":
			c.AIProvider = "openrouter"
		case c.GeminiAPIKey != "":
			c.AIProvider = "gemini"
		}
	}
	switch c.AIProvider {
	case "":
	case "openrouter":
		if c.OpenRouterAPIKey == "" {
			return c, fmt.Errorf("AI_PROVIDER=openrouter needs OPENROUTER_API_KEY")
		}
		// Chosen by cmd/evalvision on public photos: the most accurate of the
		// cheap models, with a fallback from another vendor for when Google
		// rate-limits or blocks a photo.
		if c.VisionModel == "" {
			c.VisionModel = "google/gemini-3.1-flash-lite"
		}
		if _, set := os.LookupEnv("VISION_FALLBACK_MODELS"); !set {
			c.VisionFallbackModels = []string{"anthropic/claude-haiku-5.5"}
		}
		if c.ConfigModel == "" {
			c.ConfigModel = "anthropic/claude-sonnet-5.5"
		}
		if c.SearchModel == "" {
			c.SearchModel = "google/gemini-3.1-flash-lite"
		}
		// Chosen by cmd/evalappearance on photos of look-alike pets: few
		// wrong filings, honest when unsure, and cheap with many images.
		// anthropic/claude-sonnet-5.5 made none, at twenty times the price.
		if c.MatchModel == "" {
			c.MatchModel = "openai/gpt-6-luna"
		}
		// Chosen by cmd/evalembed: best on English and Chinese queries.
		if c.TextEmbeddingModel == "" {
			c.TextEmbeddingModel = "voyageai/voyage-4"
		}
	case "gemini":
		if c.GeminiAPIKey == "" {
			return c, fmt.Errorf("AI_PROVIDER=gemini needs GEMINI_API_KEY")
		}
		if c.VisionModel == "" {
			c.VisionModel = "gemini-3.1-flash-lite"
		}
		if c.ConfigModel == "" {
			c.ConfigModel = "gemini-3.5-flash"
		}
		if c.SearchModel == "" {
			c.SearchModel = "gemini-3.1-flash-lite"
		}
	default:
		return c, fmt.Errorf("AI_PROVIDER must be gemini or openrouter, got %q", c.AIProvider)
	}
	var err error
	if c.S3PathStyle, err = boolEnv("S3_PATH_STYLE", true); err != nil {
		return c, err
	}
	if c.BraintrustContent, err = boolEnv("BRAINTRUST_LOG_CONTENT", true); err != nil {
		return c, err
	}
	if c.Workers, err = intEnv("WORKERS", 8); err != nil {
		return c, err
	}
	if c.MaxImageEdge, err = intEnv("MAX_IMAGE_EDGE", 1280); err != nil {
		return c, err
	}
	maxUpload, err := intEnv("MAX_UPLOAD_MB", 30)
	if err != nil {
		return c, err
	}
	c.MaxUploadBytes = int64(maxUpload) << 20
	for key, dst := range map[string]*float64{
		"SEARCH_MAX_DISTANCE": &c.SearchMaxDistance, "SEARCH_RELATIVE_DISTANCE": &c.SearchRelativeDistance,
		"SEARCH_MISSING_DISTANCE": &c.SearchMissingDistance,
		"LOOK_AUTO_SIMILARITY":    &c.LookAutoSimilarity, "LOOK_MARGIN": &c.LookMargin,
	} {
		if *dst, err = floatEnv(key); err != nil {
			return c, err
		}
	}
	if c.VisionPriceIn, err = floatEnv("VISION_PRICE_IN"); err != nil {
		return c, err
	}
	if c.VisionPriceOut, err = floatEnv("VISION_PRICE_OUT"); err != nil {
		return c, err
	}
	return c, nil
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func intEnv(key string, def int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return n, nil
}

func floatEnv(key string) (float64, error) {
	v := os.Getenv(key)
	if v == "" {
		return 0, nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f < 0 {
		return 0, fmt.Errorf("%s must be a non-negative number", key)
	}
	return f, nil
}

func boolEnv(key string, def bool) (bool, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("%s must be true or false", key)
	}
	return b, nil
}
