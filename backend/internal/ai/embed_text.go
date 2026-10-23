package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// TextEmbedder turns text into vectors for semantic search. Photos are
// searched by their description, tags and subject, so text vectors carry
// what was seen without sending images again.
type TextEmbedder interface {
	Provider() string
	Model() string
	Dimensions() int
	EmbedTexts(ctx context.Context, texts []string) ([][]float32, Usage, error)
}

// EmbeddingDimensions, when set, asks the model for vectors of that size
// (models trained for it, such as Qwen3 and OpenAI text-embedding-3, keep
// most of their quality when shortened).
func (o *OpenAICompatible) Dimensions() int { return o.EmbeddingDimensions }

// EmbedTexts returns one vector per text, in order.
func (o *OpenAICompatible) EmbedTexts(ctx context.Context, texts []string) ([][]float32, Usage, error) {
	body := map[string]any{"model": o.ModelID, "input": texts}
	if o.EmbeddingDimensions > 0 {
		body["dimensions"] = o.EmbeddingDimensions
	}
	raw, status, latency, err := o.post(ctx, "/embeddings", body)
	usage := Usage{Provider: o.Name, Model: o.ModelID, Latency: latency}
	if err != nil {
		return nil, usage, err
	}
	var out struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
		Usage *struct {
			PromptTokens int      `json:"prompt_tokens"`
			Cost         *float64 `json:"cost"`
		} `json:"usage"`
		Error *apiError `json:"error"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, usage, fmt.Errorf("%s: HTTP %d: %s", o.Name, status, snippet(raw))
	}
	if out.Usage != nil {
		usage.InputTokens = out.Usage.PromptTokens
		if out.Usage.Cost != nil {
			usage.CostUSD = *out.Usage.Cost
		} else {
			usage.CostUSD = o.Price.Cost(usage.InputTokens, 0)
		}
	}
	switch {
	case out.Error != nil:
		return nil, usage, o.apiErr(out.Error)
	case status != http.StatusOK:
		return nil, usage, fmt.Errorf("%s: HTTP %d: %s", o.Name, status, snippet(raw))
	case len(out.Data) != len(texts):
		return nil, usage, fmt.Errorf("%s: %d vectors for %d texts", o.Name, len(out.Data), len(texts))
	}
	vecs := make([][]float32, len(texts))
	for _, d := range out.Data {
		if d.Index < 0 || d.Index >= len(texts) || vecs[d.Index] != nil {
			return nil, usage, fmt.Errorf("%s: bad vector index %d", o.Name, d.Index)
		}
		if o.EmbeddingDimensions > 0 && len(d.Embedding) != o.EmbeddingDimensions {
			return nil, usage, fmt.Errorf("%s: got %d dimensions, want %d", o.Name, len(d.Embedding), o.EmbeddingDimensions)
		}
		vecs[d.Index] = d.Embedding
	}
	return vecs, usage, nil
}
