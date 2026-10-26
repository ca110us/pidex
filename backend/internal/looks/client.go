// Package looks asks the look service (vision/) what the subject of a photo
// looks like, as a vector. Photos of one individual come out close together;
// two look-alikes come out further apart than words can put them.
package looks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Look is a photo's vector and the model that made it: vectors from
// different models are not comparable.
type Look struct {
	Model  string
	Vector []float32
}

// Client calls the look service at URL.
type Client struct {
	URL  string
	HTTP *http.Client
}

// New is a client with a timeout fit for a CPU service: a photo takes a
// few hundred milliseconds, queued behind others.
func New(url string) *Client {
	return &Client{URL: strings.TrimRight(url, "/"), HTTP: &http.Client{Timeout: 20 * time.Second}}
}

// Embed returns the look of an image, already cut to its subject.
func (c *Client) Embed(ctx context.Context, jpeg []byte) (Look, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL+"/v1/embed", bytes.NewReader(jpeg))
	if err != nil {
		return Look{}, err
	}
	req.Header.Set("Content-Type", "image/jpeg")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Look{}, fmt.Errorf("look service: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Look{}, fmt.Errorf("look service: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return Look{}, fmt.Errorf("look service: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var out struct {
		Model  string    `json:"model"`
		Dims   int       `json:"dims"`
		Vector []float32 `json:"vector"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return Look{}, fmt.Errorf("look service: %w", err)
	}
	if out.Model == "" || len(out.Vector) == 0 || len(out.Vector) != out.Dims {
		return Look{}, fmt.Errorf("look service: an answer without a vector")
	}
	return Look{Model: out.Model, Vector: out.Vector}, nil
}
