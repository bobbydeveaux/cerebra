package embedder

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewOllama(t *testing.T) {
	emb := NewOllama("http://localhost:11434", "nomic-embed-text")
	if emb == nil {
		t.Fatal("NewOllama returned nil")
	}
	if emb.BaseURL != "http://localhost:11434" {
		t.Errorf("BaseURL = %q, want http://localhost:11434", emb.BaseURL)
	}
	if emb.Model != "nomic-embed-text" {
		t.Errorf("Model = %q, want nomic-embed-text", emb.Model)
	}
	if emb.client == nil {
		t.Fatal("client is nil")
	}
}

func TestOllama_Dimensions(t *testing.T) {
	emb := NewOllama("http://localhost:11434", "nomic-embed-text")
	if got := emb.Dimensions(); got != 768 {
		t.Errorf("Dimensions() = %d, want 768", got)
	}
}

func TestOllama_Embed_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embed" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("decoding request body: %v", err)
		}
		if req.Model != "nomic-embed-text" {
			t.Errorf("request model = %q, want nomic-embed-text", req.Model)
		}
		if len(req.Input) != 2 {
			t.Errorf("request input len = %d, want 2", len(req.Input))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"embeddings":[[0.1,0.2,0.3],[0.4,0.5,0.6]]}`))
	}))
	defer srv.Close()

	emb := NewOllama(srv.URL, "nomic-embed-text")
	vecs, err := emb.Embed(context.Background(), []string{"hello", "world"})
	if err != nil {
		t.Fatalf("Embed returned error: %v", err)
	}
	if len(vecs) != 2 {
		t.Fatalf("len(vecs) = %d, want 2", len(vecs))
	}
	if vecs[0][0] != 0.1 || vecs[1][2] != 0.6 {
		t.Errorf("unexpected embeddings: %v", vecs)
	}
}

func TestOllama_Embed_NonRetryableError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer srv.Close()

	emb := NewOllama(srv.URL, "nomic-embed-text")
	_, err := emb.Embed(context.Background(), []string{"hello"})
	if err == nil {
		t.Fatal("expected error for 400, got nil")
	}
	if !strings.Contains(err.Error(), "ollama API error 400") {
		t.Errorf("error = %v, want ollama API error 400", err)
	}
}

func TestOllama_Embed_RetryThenSucceed(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"embeddings":[[0.7,0.8]]}`))
	}))
	defer srv.Close()

	emb := NewOllama(srv.URL, "nomic-embed-text")
	vecs, err := emb.Embed(context.Background(), []string{"hi"})
	if err != nil {
		t.Fatalf("Embed returned error: %v", err)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Errorf("calls = %d, want 2", atomic.LoadInt32(&calls))
	}
	if len(vecs) != 1 || vecs[0][0] != 0.7 {
		t.Errorf("unexpected vecs: %v", vecs)
	}
}

func TestOllama_Embed_503Retry(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"embeddings":[[0.9]]}`))
	}))
	defer srv.Close()

	emb := NewOllama(srv.URL, "nomic-embed-text")
	_, err := emb.Embed(context.Background(), []string{"hi"})
	if err != nil {
		t.Fatalf("Embed returned error: %v", err)
	}
}

func TestOllama_Embed_AllRetriesExhausted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	emb := NewOllama(srv.URL, "nomic-embed-text")
	_, err := emb.Embed(context.Background(), []string{"hi"})
	if err == nil {
		t.Fatal("expected error after exhausted retries, got nil")
	}
	if !strings.Contains(err.Error(), "after retries") {
		t.Errorf("error = %v, want 'after retries'", err)
	}
}

func TestOllama_Embed_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{not json"))
	}))
	defer srv.Close()

	emb := NewOllama(srv.URL, "nomic-embed-text")
	_, err := emb.Embed(context.Background(), []string{"hi"})
	if err == nil {
		t.Fatal("expected error on malformed JSON, got nil")
	}
	if !strings.Contains(err.Error(), "parsing response") {
		t.Errorf("error = %v, want parsing response", err)
	}
}

func TestOllama_Embed_MismatchedCount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// One embedding returned for two inputs.
		_, _ = w.Write([]byte(`{"embeddings":[[0.1,0.2]]}`))
	}))
	defer srv.Close()

	emb := NewOllama(srv.URL, "nomic-embed-text")
	_, err := emb.Embed(context.Background(), []string{"a", "b"})
	if err == nil {
		t.Fatal("expected error on mismatched count, got nil")
	}
	if !strings.Contains(err.Error(), "expected 2 embeddings, got 1") {
		t.Errorf("error = %v, want mismatched count", err)
	}
}

func TestOllama_Embed_TransportError(t *testing.T) {
	// Server that closes connections without responding to force a transport error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			return
		}
		conn, _, _ := hj.Hijack()
		_ = conn.Close()
	}))
	defer srv.Close()

	emb := NewOllama(srv.URL, "nomic-embed-text")
	_, err := emb.Embed(context.Background(), []string{"hi"})
	if err == nil {
		t.Fatal("expected transport error, got nil")
	}
}

func TestOllama_Embed_ContextCancelledBeforeFirstCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	emb := NewOllama(srv.URL, "nomic-embed-text")
	ctx, cancel := context.WithCancel(context.Background())
	// Cancel quickly so the post-first-attempt backoff select hits ctx.Done.
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	_, err := emb.Embed(ctx, []string{"hi"})
	if err == nil {
		t.Fatal("expected context-cancelled error, got nil")
	}
}
