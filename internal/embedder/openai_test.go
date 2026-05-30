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

// openAIWithBaseURL builds an OpenAIEmbedder pointed at a custom base URL for tests.
// The production NewOpenAI hard-codes the upstream API URL, so we construct the
// struct directly to redirect requests at our httptest server. The request path
// stays the same because we control the test server's handler.
//
// Note: the production code path posts to "https://api.openai.com/v1/embeddings"
// — to exercise the same Embed function we need to substitute the host. The
// struct fields APIKey/Model/client are exported via the same package.
func openAIWithBaseURL(apiKey, model string, client *http.Client) *OpenAIEmbedder {
	return &OpenAIEmbedder{
		APIKey: apiKey,
		Model:  model,
		client: client,
	}
}

// rewriteTransport intercepts every request and rewrites the URL to point at
// the given target. This lets us redirect "https://api.openai.com/v1/embeddings"
// to a local httptest server without touching production code.
type rewriteTransport struct {
	target string
}

func (r *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	targetReq, err := http.NewRequestWithContext(req.Context(), req.Method, r.target+req.URL.Path, req.Body)
	if err != nil {
		return nil, err
	}
	targetReq.Header = req.Header
	return http.DefaultTransport.RoundTrip(targetReq)
}

func TestNewOpenAI(t *testing.T) {
	emb := NewOpenAI("sk-test-key", "text-embedding-3-small")
	if emb == nil {
		t.Fatal("NewOpenAI returned nil")
	}
	if emb.APIKey != "sk-test-key" {
		t.Errorf("APIKey = %q, want sk-test-key", emb.APIKey)
	}
	if emb.Model != "text-embedding-3-small" {
		t.Errorf("Model = %q, want text-embedding-3-small", emb.Model)
	}
	if emb.client == nil {
		t.Fatal("client is nil")
	}
}

func TestOpenAI_Dimensions(t *testing.T) {
	emb := NewOpenAI("sk-test", "text-embedding-3-small")
	if got := emb.Dimensions(); got != 1536 {
		t.Errorf("Dimensions() = %d, want 1536", got)
	}
}

func TestOpenAI_Embed_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Errorf("path = %q, want /v1/embeddings", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer sk-test-key" {
			t.Errorf("Authorization = %q, want Bearer sk-test-key", auth)
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
		if req.Model != "text-embedding-3-small" {
			t.Errorf("request model = %q", req.Model)
		}
		if len(req.Input) != 2 {
			t.Errorf("request input len = %d", len(req.Input))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1,0.2]},{"embedding":[0.3,0.4]}]}`))
	}))
	defer srv.Close()

	client := &http.Client{Transport: &rewriteTransport{target: srv.URL}}
	emb := openAIWithBaseURL("sk-test-key", "text-embedding-3-small", client)
	vecs, err := emb.Embed(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("Embed returned error: %v", err)
	}
	if len(vecs) != 2 {
		t.Fatalf("len(vecs) = %d, want 2", len(vecs))
	}
	if vecs[0][1] != 0.2 || vecs[1][0] != 0.3 {
		t.Errorf("unexpected vecs: %v", vecs)
	}
}

func TestOpenAI_Embed_NonRetryableError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	client := &http.Client{Transport: &rewriteTransport{target: srv.URL}}
	emb := openAIWithBaseURL("sk-bad", "text-embedding-3-small", client)
	_, err := emb.Embed(context.Background(), []string{"hi"})
	if err == nil {
		t.Fatal("expected error for 401, got nil")
	}
	if !strings.Contains(err.Error(), "openai API error 401") {
		t.Errorf("error = %v, want openai API error 401", err)
	}
}

func TestOpenAI_Embed_RetryThenSucceed(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.5]}]}`))
	}))
	defer srv.Close()

	client := &http.Client{Transport: &rewriteTransport{target: srv.URL}}
	emb := openAIWithBaseURL("sk-test", "m", client)
	vecs, err := emb.Embed(context.Background(), []string{"x"})
	if err != nil {
		t.Fatalf("Embed returned error: %v", err)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Errorf("calls = %d, want 2", atomic.LoadInt32(&calls))
	}
	if len(vecs) != 1 || vecs[0][0] != 0.5 {
		t.Errorf("unexpected vecs: %v", vecs)
	}
}

func TestOpenAI_Embed_503Retry(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.9]}]}`))
	}))
	defer srv.Close()

	client := &http.Client{Transport: &rewriteTransport{target: srv.URL}}
	emb := openAIWithBaseURL("sk-test", "m", client)
	_, err := emb.Embed(context.Background(), []string{"x"})
	if err != nil {
		t.Fatalf("Embed returned error: %v", err)
	}
}

func TestOpenAI_Embed_AllRetriesExhausted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	client := &http.Client{Transport: &rewriteTransport{target: srv.URL}}
	emb := openAIWithBaseURL("sk-test", "m", client)
	_, err := emb.Embed(context.Background(), []string{"x"})
	if err == nil {
		t.Fatal("expected error after exhausted retries, got nil")
	}
	if !strings.Contains(err.Error(), "after retries") {
		t.Errorf("error = %v, want 'after retries'", err)
	}
}

func TestOpenAI_Embed_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not json at all"))
	}))
	defer srv.Close()

	client := &http.Client{Transport: &rewriteTransport{target: srv.URL}}
	emb := openAIWithBaseURL("sk-test", "m", client)
	_, err := emb.Embed(context.Background(), []string{"x"})
	if err == nil {
		t.Fatal("expected error on malformed JSON, got nil")
	}
	if !strings.Contains(err.Error(), "parsing response") {
		t.Errorf("error = %v, want parsing response", err)
	}
}

func TestOpenAI_Embed_TransportError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			return
		}
		conn, _, _ := hj.Hijack()
		_ = conn.Close()
	}))
	defer srv.Close()

	client := &http.Client{Transport: &rewriteTransport{target: srv.URL}}
	emb := openAIWithBaseURL("sk-test", "m", client)
	_, err := emb.Embed(context.Background(), []string{"x"})
	if err == nil {
		t.Fatal("expected transport error, got nil")
	}
}

func TestOpenAI_Embed_ContextCancelledDuringBackoff(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	client := &http.Client{Transport: &rewriteTransport{target: srv.URL}}
	emb := openAIWithBaseURL("sk-test", "m", client)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	_, err := emb.Embed(ctx, []string{"x"})
	if err == nil {
		t.Fatal("expected context-cancelled error, got nil")
	}
}
