package rag

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bobbydeveaux/cerebra/internal/config"
	"github.com/bobbydeveaux/cerebra/internal/embedder"
	"github.com/bobbydeveaux/cerebra/internal/store"
)

type Pipeline struct {
	embedder embedder.Embedder
	store    store.Store
	cfg      *config.Config
	client   *http.Client
}

func NewPipeline(emb embedder.Embedder, s store.Store, cfg *config.Config) *Pipeline {
	return &Pipeline{
		embedder: emb,
		store:    s,
		cfg:      cfg,
		client:   &http.Client{Timeout: 120 * time.Second},
	}
}

func (p *Pipeline) Answer(ctx context.Context, question string) (<-chan string, error) {
	return p.AnswerWithHistory(ctx, question, nil)
}

func (p *Pipeline) AnswerWithHistory(ctx context.Context, question string, history []map[string]string) (<-chan string, error) {
	// Step 1: embed the question
	vecs, err := p.embedder.Embed(ctx, []string{question})
	if err != nil {
		return nil, fmt.Errorf("embedding question: %w", err)
	}

	// Step 2: retrieve top-k chunks
	results, err := p.store.Search(ctx, vecs[0], 8)
	if err != nil {
		return nil, fmt.Errorf("searching: %w", err)
	}

	// Step 3: build prompt
	var contextBuf strings.Builder
	for i, r := range results {
		fmt.Fprintf(&contextBuf, "[%d] File: %s (lines %d-%d)\n%s\n\n",
			i+1, r.Chunk.Metadata.Path, r.Chunk.StartLine, r.Chunk.EndLine, r.Chunk.Content)
	}
	prompt := buildPrompt(question, contextBuf.String(), history)

	// Step 4: stream from LLM
	switch p.cfg.ChatLLM {
	case "claude":
		return p.streamClaude(ctx, prompt)
	case "minimax":
		return p.streamMiniMax(ctx, prompt)
	case "openai":
		return p.streamOpenAI(ctx, prompt)
	default:
		return p.streamOllama(ctx, prompt)
	}
}

func buildPrompt(question, codeContext string, history []map[string]string) string {
	var historyBuf strings.Builder
	if len(history) > 0 {
		historyBuf.WriteString("\nConversation so far:\n")
		// Keep last 6 messages max to avoid token bloat
		start := 0
		if len(history) > 6 {
			start = len(history) - 6
		}
		for _, msg := range history[start:] {
			role := msg["role"]
			content := msg["content"]
			if len(content) > 500 {
				content = content[:500] + "..."
			}
			historyBuf.WriteString(fmt.Sprintf("%s: %s\n", role, content))
		}
		historyBuf.WriteString("\n")
	}

	return fmt.Sprintf(`You are Jor-El, a codebase knowledge assistant. Rules:
1. Answer ONLY from the provided context. If it's not there, say "I don't have enough context for that."
2. Be concise and structured. Use bullet points for lists.
3. Cite sources as [N] matching the context block numbers.
4. Do NOT include <think> tags or internal reasoning in your response.
5. Do NOT repeat the question. Jump straight to the answer.
6. Use the conversation history to understand follow-up questions (e.g. "what does it look like?" refers to the previous topic).

%sContext:
%s

Question: %s`, historyBuf.String(), codeContext, question)
}

func (p *Pipeline) streamOllama(ctx context.Context, prompt string) (<-chan string, error) {
	body := map[string]interface{}{
		"model":  p.cfg.Ollama.ChatModel,
		"prompt": prompt,
		"stream": true,
	}
	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", p.cfg.Ollama.URL+"/api/generate", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama request: %w", err)
	}

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("ollama error %d: %s", resp.StatusCode, string(body))
	}

	ch := make(chan string, 100)
	go func() {
		defer resp.Body.Close()
		defer close(ch)

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			var chunk struct {
				Response string `json:"response"`
				Done     bool   `json:"done"`
			}
			if err := json.Unmarshal(scanner.Bytes(), &chunk); err != nil {
				continue
			}
			if chunk.Response != "" {
				select {
				case ch <- chunk.Response:
				case <-ctx.Done():
					return
				}
			}
			if chunk.Done {
				return
			}
		}
	}()

	return ch, nil
}

func (p *Pipeline) streamOpenAI(ctx context.Context, prompt string) (<-chan string, error) {
	body := map[string]interface{}{
		"model":  p.cfg.OpenAI.ChatModel,
		"stream": true,
		"messages": []map[string]string{
			{"role": "system", "content": "You are Jor-El, a codebase knowledge assistant. Be concise, use bullet points, cite sources as [N]."},
			{"role": "user", "content": prompt},
		},
	}
	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.cfg.OpenAI.APIKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai request: %w", err)
	}

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("openai error %d: %s", resp.StatusCode, string(respBody))
	}

	ch := make(chan string, 100)
	go func() {
		defer resp.Body.Close()
		defer close(ch)

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				return
			}

			var chunk struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
			}
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}
			if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
				select {
				case ch <- chunk.Choices[0].Delta.Content:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return ch, nil
}

func (p *Pipeline) streamMiniMax(ctx context.Context, prompt string) (<-chan string, error) {
	body := map[string]interface{}{
		"model":                p.cfg.MiniMax.Model,
		"stream":               true,
		"max_completion_tokens": 4096,
		"temperature":          0.3,
		"messages": []map[string]string{
			{"role": "system", "content": "You are Jor-El, an expert codebase assistant. Answer concisely and cite sources using [N] notation."},
			{"role": "user", "content": prompt},
		},
	}
	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.minimax.io/v1/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.cfg.MiniMax.APIKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("minimax request: %w", err)
	}

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("minimax error %d: %s", resp.StatusCode, string(respBody))
	}

	ch := make(chan string, 100)
	go func() {
		defer resp.Body.Close()
		defer close(ch)

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				return
			}

			var chunk struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
			}
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}
			if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
				select {
				case ch <- chunk.Choices[0].Delta.Content:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return ch, nil
}

func (p *Pipeline) streamClaude(ctx context.Context, prompt string) (<-chan string, error) {
	body := map[string]interface{}{
		"model":      p.cfg.Claude.Model,
		"max_tokens": 4096,
		"stream":     true,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.cfg.Claude.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("claude request: %w", err)
	}

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("claude error %d: %s", resp.StatusCode, string(body))
	}

	ch := make(chan string, 100)
	go func() {
		defer resp.Body.Close()
		defer close(ch)

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				return
			}

			var event struct {
				Type  string `json:"type"`
				Delta struct {
					Text string `json:"text"`
				} `json:"delta"`
			}
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}
			if event.Type == "content_block_delta" && event.Delta.Text != "" {
				select {
				case ch <- event.Delta.Text:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return ch, nil
}
