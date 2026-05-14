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

type OpenAIEmbedder struct {
	APIKey string
	Model  string
	client *http.Client
}

func NewOpenAI(apiKey, model string) *OpenAIEmbedder {
	return &OpenAIEmbedder{
		APIKey: apiKey,
		Model:  model,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (o *OpenAIEmbedder) Dimensions() int {
	return 1536
}

func (o *OpenAIEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
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

		req, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/embeddings", bytes.NewReader(jsonBody))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+o.APIKey)

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
			return nil, fmt.Errorf("openai API error %d: %s", resp.StatusCode, string(respBody))
		}

		if err != nil {
			return nil, err
		}

		var parsed struct {
			Data []struct {
				Embedding []float32 `json:"embedding"`
			} `json:"data"`
		}
		if err := json.Unmarshal(respBody, &parsed); err != nil {
			return nil, fmt.Errorf("parsing response: %w", err)
		}

		results := make([][]float32, len(parsed.Data))
		for i, d := range parsed.Data {
			results[i] = d.Embedding
		}
		return results, nil
	}

	return nil, fmt.Errorf("after retries: %w", lastErr)
}
