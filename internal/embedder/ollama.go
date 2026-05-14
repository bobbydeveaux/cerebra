package embedder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type OllamaEmbedder struct {
	BaseURL string
	Model   string
	client  *http.Client
}

func NewOllama(baseURL, model string) *OllamaEmbedder {
	return &OllamaEmbedder{
		BaseURL: baseURL,
		Model:   model,
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func (o *OllamaEmbedder) Dimensions() int {
	return 768
}

// Embed uses Ollama's batch /api/embed endpoint to embed multiple texts in one request.
func (o *OllamaEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	body := map[string]interface{}{
		"model": o.Model,
		"input": texts,
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			delay := time.Duration(500*(1<<attempt)) * time.Millisecond
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		req, err := http.NewRequestWithContext(ctx, "POST", o.BaseURL+"/api/embed", bytes.NewReader(jsonBody))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := o.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == 429 || resp.StatusCode == 503 {
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
			continue
		}

		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("ollama API error %d: %s", resp.StatusCode, string(respBody))
		}

		if err != nil {
			return nil, err
		}

		var parsed struct {
			Embeddings [][]float32 `json:"embeddings"`
		}
		if err := json.Unmarshal(respBody, &parsed); err != nil {
			return nil, fmt.Errorf("parsing response: %w", err)
		}

		if len(parsed.Embeddings) != len(texts) {
			return nil, fmt.Errorf("expected %d embeddings, got %d", len(texts), len(parsed.Embeddings))
		}

		return parsed.Embeddings, nil
	}

	return nil, fmt.Errorf("after retries: %w", lastErr)
}
