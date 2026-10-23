package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"google.golang.org/genai"
)

// GeminiPrices are list prices per million tokens (Gemini API paid tier,
// checked 2026-10-19). Override with VISION_PRICE_* when they change.
var GeminiPrices = map[string]Price{
	"gemini-2.5-flash-lite":  {InputPerM: 0.10, OutputPerM: 0.40},
	"gemini-3.1-flash-lite":  {InputPerM: 0.25, OutputPerM: 1.50},
	"gemini-2.5-flash":       {InputPerM: 0.30, OutputPerM: 2.50},
	"gemini-3-flash-preview": {InputPerM: 0.50, OutputPerM: 3.00},
}

type Gemini struct {
	client *genai.Client
	model  string
	price  Price
}

func NewGemini(ctx context.Context, apiKey, model string, price Price) (*Gemini, error) {
	if apiKey == "" {
		return nil, errors.New("gemini: API key is empty")
	}
	c, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: apiKey, Backend: genai.BackendGeminiAPI})
	if err != nil {
		return nil, fmt.Errorf("gemini client: %w", err)
	}
	return &Gemini{client: c, model: model, price: price}, nil
}

func (g *Gemini) Provider() string { return "gemini" }
func (g *Gemini) Model() string    { return g.model }

func (g *Gemini) Structured(ctx context.Context, req StructuredRequest) (json.RawMessage, Usage, error) {
	parts := make([]*genai.Part, 0, len(req.Images)+1)
	for _, img := range req.Images {
		parts = append(parts, genai.NewPartFromBytes(img.Data, img.MIMEType))
	}
	parts = append(parts, genai.NewPartFromText(req.Prompt))

	temp := float32(0)
	cfg := &genai.GenerateContentConfig{
		Temperature:        &temp,
		ResponseMIMEType:   "application/json",
		ResponseJsonSchema: req.Schema,
		MaxOutputTokens:    int32(req.MaxOutputTokens),
	}
	if req.Instructions != "" {
		cfg.SystemInstruction = &genai.Content{Parts: []*genai.Part{genai.NewPartFromText(req.Instructions)}}
	}

	start := time.Now()
	resp, err := g.client.Models.GenerateContent(ctx, g.model, []*genai.Content{{Role: genai.RoleUser, Parts: parts}}, cfg)
	usage := Usage{Latency: time.Since(start)}
	if err != nil {
		return nil, usage, err
	}
	if m := resp.UsageMetadata; m != nil {
		usage.InputTokens = int(m.PromptTokenCount)
		// Thinking tokens are billed as output.
		usage.OutputTokens = int(m.CandidatesTokenCount + m.ThoughtsTokenCount)
		usage.CostUSD = g.price.Cost(usage.InputTokens, usage.OutputTokens)
	}
	text := resp.Text()
	if text == "" {
		return nil, usage, errors.New("gemini: empty response")
	}
	return json.RawMessage(text), usage, nil
}

// GeminiEmbedder embeds images for similarity search.
type GeminiEmbedder struct {
	client      *genai.Client
	model       string
	dims        int
	costPerItem float64
}

func NewGeminiEmbedder(ctx context.Context, apiKey, model string, dims int, costPerImage float64) (*GeminiEmbedder, error) {
	if apiKey == "" {
		return nil, errors.New("gemini: API key is empty")
	}
	c, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: apiKey, Backend: genai.BackendGeminiAPI})
	if err != nil {
		return nil, fmt.Errorf("gemini client: %w", err)
	}
	return &GeminiEmbedder{client: c, model: model, dims: dims, costPerItem: costPerImage}, nil
}

func (e *GeminiEmbedder) Dimensions() int { return e.dims }

func (e *GeminiEmbedder) EmbedImage(ctx context.Context, img Image) ([]float32, Usage, error) {
	dims := int32(e.dims)
	start := time.Now()
	resp, err := e.client.Models.EmbedContent(ctx, e.model,
		[]*genai.Content{{Parts: []*genai.Part{genai.NewPartFromBytes(img.Data, img.MIMEType)}}},
		&genai.EmbedContentConfig{OutputDimensionality: &dims})
	usage := Usage{Provider: "gemini", Model: e.model, Latency: time.Since(start)}
	if err != nil {
		return nil, usage, err
	}
	if len(resp.Embeddings) == 0 || len(resp.Embeddings[0].Values) != e.dims {
		return nil, usage, fmt.Errorf("gemini: unexpected embedding shape")
	}
	usage.CostUSD = e.costPerItem
	return resp.Embeddings[0].Values, usage, nil
}
