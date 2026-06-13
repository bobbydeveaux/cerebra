// Package rag tests exercise the orchestration in pipeline.go without
// touching real model APIs or a real SQLite store. Embedder and Store are
// interface-driven so we substitute lightweight in-memory stubs; the four
// streaming LLM clients post to either a config-driven URL (Ollama) or a
// hard-coded vendor URL (OpenAI / MiniMax / Claude) which we redirect via
// a rewriteTransport — the same pattern used in internal/embedder/openai_test.go.
package rag

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bobbydeveaux/cerebra/internal/chunker"
	"github.com/bobbydeveaux/cerebra/internal/config"
	"github.com/bobbydeveaux/cerebra/internal/scanner"
	"github.com/bobbydeveaux/cerebra/internal/store"
)

// requireNetwork skips the calling test when the sandbox cannot bind a
// local TCP port. httptest.NewServer panics (rather than returning an
// error) when port binding is denied, which aborts the whole package and
// masks unrelated results. Probing 127.0.0.1 here turns that panic into a
// clean skip; under full networking (CI / make test) it is a no-op.
func requireNetwork(t *testing.T) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("rag test requires network port binding (httptest): " + err.Error())
	}
	_ = ln.Close()
}

// ---------- fakes ----------

type stubEmbedder struct {
	vec [][]float32
	err error
}

func (s *stubEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.vec != nil {
		return s.vec, nil
	}
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = []float32{0.1, 0.2, 0.3}
	}
	return out, nil
}

func (s *stubEmbedder) Dimensions() int { return 3 }

// stubStore implements store.Store with no-op defaults; only Search is
// meaningful for the RAG pipeline tests. Methods that the pipeline never
// calls return zero values; this keeps the surface minimal but satisfies
// the wide interface.
type stubStore struct {
	results   []store.SearchResult
	searchErr error
}

func (s *stubStore) UpsertDocument(_ context.Context, _ scanner.Document, _ []chunker.Chunk) error {
	return nil
}
func (s *stubStore) DeleteDocument(_ context.Context, _ string) error            { return nil }
func (s *stubStore) UpdateScanState(_ context.Context, _ store.ScanState) error  { return nil }
func (s *stubStore) Search(_ context.Context, _ []float32, _ int) ([]store.SearchResult, error) {
	return s.results, s.searchErr
}
func (s *stubStore) SearchFTS(_ context.Context, _ string, _ int) ([]store.SearchResult, error) {
	return nil, nil
}
func (s *stubStore) GetDocument(_ context.Context, _ string) (*scanner.Document, []chunker.Chunk, error) {
	return nil, nil, nil
}
func (s *stubStore) GetScanState(_ context.Context, _ string) (*store.ScanState, error) {
	return nil, nil
}
func (s *stubStore) GetStats(_ context.Context) (store.Stats, error)             { return store.Stats{}, nil }
func (s *stubStore) ListCategories(_ context.Context) ([]store.CategorySummary, error) {
	return nil, nil
}
func (s *stubStore) GetContentHash(_ context.Context, _ string) (string, error) { return "", nil }
func (s *stubStore) ListFilesByCategory(_ context.Context, _ string, _ int) ([]store.FileSummary, error) {
	return nil, nil
}
func (s *stubStore) ListAllFiles(_ context.Context) ([]store.FileSummary, error) {
	return nil, nil
}
func (s *stubStore) UpsertBrain(_ context.Context, _ store.Brain) error { return nil }
func (s *stubStore) GetBrain(_ context.Context, _ string) (*store.Brain, error) {
	return nil, nil
}
func (s *stubStore) ListBrains(_ context.Context, _ string, _ string, _ int) ([]store.Brain, error) {
	return nil, nil
}
func (s *stubStore) GetBrainStats(_ context.Context) (store.BrainStats, error) {
	return store.BrainStats{}, nil
}
func (s *stubStore) DeleteBrainActivity(_ context.Context, _ string) error { return nil }
func (s *stubStore) UpsertActivity(_ context.Context, _ store.HourlyActivity) error {
	return nil
}
func (s *stubStore) ListActivity(_ context.Context, _ string, _ string) ([]store.HourlyActivity, error) {
	return nil, nil
}
func (s *stubStore) UpsertAgentMessage(_ context.Context, _ store.AgentMessage) error {
	return nil
}
func (s *stubStore) SearchAgentMessages(_ context.Context, _ string, _ string, _ int) ([]store.AgentMessage, error) {
	return nil, nil
}
func (s *stubStore) ListAgentActivity(_ context.Context, _ string, _ string, _ string, _ int) ([]store.AgentMessage, error) {
	return nil, nil
}
func (s *stubStore) Close() error { return nil }

// rewriteTransport rewrites every outbound URL to the given target host,
// preserving the original request path. This lets us redirect the
// hard-coded OpenAI / MiniMax / Claude URLs to a local httptest server.
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

// pipelineForTest builds a Pipeline whose HTTP client is intercepted by
// the given test server, and whose Ollama URL points directly at it.
func pipelineForTest(t *testing.T, emb *stubEmbedder, st *stubStore, llm string, srv *httptest.Server) *Pipeline {
	t.Helper()
	cfg := &config.Config{
		ChatLLM: llm,
		Ollama: config.OllamaConfig{
			URL:       srv.URL,
			ChatModel: "test-ollama",
		},
		OpenAI:  config.OpenAIConfig{APIKey: "sk-test", ChatModel: "gpt-test"},
		Claude:  config.ClaudeConfig{APIKey: "anth-test", Model: "claude-test"},
		MiniMax: config.MiniMaxConfig{APIKey: "mm-test", Model: "minimax-test"},
	}
	p := NewPipeline(emb, st, cfg)
	p.client = &http.Client{
		Transport: &rewriteTransport{target: srv.URL},
		Timeout:   5 * time.Second,
	}
	return p
}

func seedResults() []store.SearchResult {
	return []store.SearchResult{
		{
			Chunk: chunker.Chunk{
				Content:   "func Foo() {}",
				StartLine: 1,
				EndLine:   1,
				Metadata:  chunker.ChunkMeta{Path: "foo.go"},
			},
			Score: 0.9,
		},
	}
}

// ---------- constructor + helpers ----------

func TestNewPipeline_Wiring(t *testing.T) {
	emb := &stubEmbedder{}
	st := &stubStore{}
	cfg := &config.Config{ChatLLM: "ollama"}
	p := NewPipeline(emb, st, cfg)
	if p == nil {
		t.Fatal("NewPipeline returned nil")
	}
	if p.embedder != emb {
		t.Error("embedder not wired")
	}
	if p.store != st {
		t.Error("store not wired")
	}
	if p.cfg != cfg {
		t.Error("cfg not wired")
	}
	if p.client == nil {
		t.Error("client not initialised")
	}
}

func TestBuildPrompt_NoHistory(t *testing.T) {
	got := buildPrompt("what is foo?", "[1] code here\n", nil)
	if !strings.Contains(got, "what is foo?") {
		t.Error("question missing")
	}
	if !strings.Contains(got, "[1] code here") {
		t.Error("context missing")
	}
	if strings.Contains(got, "Conversation so far:") {
		t.Error("history block should be absent")
	}
}

func TestBuildPrompt_WithShortHistory(t *testing.T) {
	hist := []map[string]string{
		{"role": "user", "content": "hi"},
		{"role": "assistant", "content": "hello"},
	}
	got := buildPrompt("what next?", "ctx", hist)
	if !strings.Contains(got, "Conversation so far:") {
		t.Error("history header missing")
	}
	if !strings.Contains(got, "user: hi") {
		t.Error("user line missing")
	}
	if !strings.Contains(got, "assistant: hello") {
		t.Error("assistant line missing")
	}
}

func TestBuildPrompt_HistoryTruncatesToLastSix(t *testing.T) {
	// Build 8 messages — only the last 6 should appear.
	hist := make([]map[string]string, 8)
	for i := range hist {
		hist[i] = map[string]string{
			"role":    "user",
			"content": fmt.Sprintf("msg-%d", i),
		}
	}
	got := buildPrompt("q", "ctx", hist)
	if strings.Contains(got, "msg-0") {
		t.Error("msg-0 should be dropped (only last 6 kept)")
	}
	if strings.Contains(got, "msg-1") {
		t.Error("msg-1 should be dropped (only last 6 kept)")
	}
	if !strings.Contains(got, "msg-2") {
		t.Error("msg-2 should be retained")
	}
	if !strings.Contains(got, "msg-7") {
		t.Error("msg-7 should be retained")
	}
}

func TestBuildPrompt_LongContentTruncates(t *testing.T) {
	long := strings.Repeat("x", 600)
	hist := []map[string]string{{"role": "user", "content": long}}
	got := buildPrompt("q", "ctx", hist)
	if !strings.Contains(got, strings.Repeat("x", 500)+"...") {
		t.Error("long message should be truncated to 500 chars + ellipsis")
	}
	if strings.Contains(got, strings.Repeat("x", 600)) {
		t.Error("full untruncated message should not appear")
	}
}

// ---------- error propagation in AnswerWithHistory ----------

func TestAnswerWithHistory_EmbedderError(t *testing.T) {
	requireNetwork(t)
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	p := pipelineForTest(t, &stubEmbedder{err: errors.New("boom")}, &stubStore{}, "ollama", srv)
	_, err := p.Answer(context.Background(), "q")
	if err == nil {
		t.Fatal("expected embedder error, got nil")
	}
	if !strings.Contains(err.Error(), "embedding question") {
		t.Errorf("error = %v, want 'embedding question'", err)
	}
}

func TestAnswerWithHistory_SearchError(t *testing.T) {
	requireNetwork(t)
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	st := &stubStore{searchErr: errors.New("db gone")}
	p := pipelineForTest(t, &stubEmbedder{}, st, "ollama", srv)
	_, err := p.Answer(context.Background(), "q")
	if err == nil {
		t.Fatal("expected search error, got nil")
	}
	if !strings.Contains(err.Error(), "searching") {
		t.Errorf("error = %v, want 'searching'", err)
	}
}

// ---------- Ollama streaming ----------

func TestStreamOllama_HappyPath(t *testing.T) {
	requireNetwork(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			t.Errorf("path = %q, want /api/generate", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write([]byte(`{"response":"hello","done":false}` + "\n"))
		_, _ = w.Write([]byte(`{"response":" world","done":false}` + "\n"))
		_, _ = w.Write([]byte(`not-json` + "\n")) // exercises the unmarshal-error skip path
		_, _ = w.Write([]byte(`{"response":"","done":true}` + "\n"))
	}))
	defer srv.Close()

	p := pipelineForTest(t, &stubEmbedder{}, &stubStore{results: seedResults()}, "ollama", srv)
	ch, err := p.Answer(context.Background(), "say hi")
	if err != nil {
		t.Fatalf("Answer returned error: %v", err)
	}
	got := drain(ch)
	if got != "hello world" {
		t.Errorf("stream = %q, want %q", got, "hello world")
	}
}

func TestStreamOllama_ErrorStatus(t *testing.T) {
	requireNetwork(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "down", http.StatusInternalServerError)
	}))
	defer srv.Close()
	p := pipelineForTest(t, &stubEmbedder{}, &stubStore{results: seedResults()}, "ollama", srv)
	_, err := p.Answer(context.Background(), "q")
	if err == nil {
		t.Fatal("expected ollama error, got nil")
	}
	if !strings.Contains(err.Error(), "ollama error 500") {
		t.Errorf("error = %v, want 'ollama error 500'", err)
	}
}

func TestStreamOllama_ContextCancelled(t *testing.T) {
	requireNetwork(t)
	// Server sends a single chunk then hangs forever — context cancel must
	// shut the goroutine down so the channel closes.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		flusher, _ := w.(http.Flusher)
		_, _ = w.Write([]byte(`{"response":"first","done":false}` + "\n"))
		if flusher != nil {
			flusher.Flush()
		}
		<-r.Context().Done()
	}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	p := pipelineForTest(t, &stubEmbedder{}, &stubStore{results: seedResults()}, "ollama", srv)
	ch, err := p.Answer(ctx, "q")
	if err != nil {
		t.Fatalf("Answer returned error: %v", err)
	}
	// Read the first chunk, then cancel; the channel must drain and close.
	first := <-ch
	if first != "first" {
		t.Errorf("first chunk = %q", first)
	}
	cancel()
	// Drain until close — must terminate.
	for range ch {
	}
}

// ---------- OpenAI streaming ----------

func TestStreamOpenAI_HappyPath(t *testing.T) {
	requireNetwork(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Errorf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"foo"}}]}` + "\n"))
		_, _ = w.Write([]byte(`data: garbage` + "\n")) // exercises malformed-data skip
		_, _ = w.Write([]byte(`unrelated line` + "\n")) // exercises non-data prefix skip
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":" bar"}}]}` + "\n"))
		_, _ = w.Write([]byte(`data: [DONE]` + "\n"))
	}))
	defer srv.Close()
	p := pipelineForTest(t, &stubEmbedder{}, &stubStore{results: seedResults()}, "openai", srv)
	ch, err := p.Answer(context.Background(), "hi")
	if err != nil {
		t.Fatalf("Answer returned error: %v", err)
	}
	if got := drain(ch); got != "foo bar" {
		t.Errorf("stream = %q, want %q", got, "foo bar")
	}
}

func TestStreamOpenAI_ErrorStatus(t *testing.T) {
	requireNetwork(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusUnauthorized)
	}))
	defer srv.Close()
	p := pipelineForTest(t, &stubEmbedder{}, &stubStore{results: seedResults()}, "openai", srv)
	_, err := p.Answer(context.Background(), "q")
	if err == nil {
		t.Fatal("expected openai error, got nil")
	}
	if !strings.Contains(err.Error(), "openai error 401") {
		t.Errorf("error = %v, want 'openai error 401'", err)
	}
}

// ---------- MiniMax streaming ----------

func TestStreamMiniMax_HappyPath(t *testing.T) {
	requireNetwork(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/v1/chat/completions") {
			t.Errorf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer mm-test" {
			t.Errorf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"alpha"}}]}` + "\n"))
		_, _ = w.Write([]byte(`data: [DONE]` + "\n"))
	}))
	defer srv.Close()
	p := pipelineForTest(t, &stubEmbedder{}, &stubStore{results: seedResults()}, "minimax", srv)
	ch, err := p.Answer(context.Background(), "q")
	if err != nil {
		t.Fatalf("Answer returned error: %v", err)
	}
	if got := drain(ch); got != "alpha" {
		t.Errorf("stream = %q", got)
	}
}

func TestStreamMiniMax_ErrorStatus(t *testing.T) {
	requireNetwork(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "bad", http.StatusBadRequest)
	}))
	defer srv.Close()
	p := pipelineForTest(t, &stubEmbedder{}, &stubStore{results: seedResults()}, "minimax", srv)
	_, err := p.Answer(context.Background(), "q")
	if err == nil {
		t.Fatal("expected minimax error, got nil")
	}
	if !strings.Contains(err.Error(), "minimax error 400") {
		t.Errorf("error = %v, want 'minimax error 400'", err)
	}
}

// ---------- Claude streaming ----------

func TestStreamClaude_HappyPath(t *testing.T) {
	requireNetwork(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-api-key"); got != "anth-test" {
			t.Errorf("x-api-key = %q", got)
		}
		if got := r.Header.Get("anthropic-version"); got != "2023-06-01" {
			t.Errorf("anthropic-version = %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`data: {"type":"content_block_delta","delta":{"text":"AAA"}}` + "\n"))
		_, _ = w.Write([]byte(`data: not-json` + "\n")) // exercises decode skip
		_, _ = w.Write([]byte(`data: {"type":"message_start","delta":{"text":"ignored"}}` + "\n")) // wrong type, ignored
		_, _ = w.Write([]byte(`data: {"type":"content_block_delta","delta":{"text":"BBB"}}` + "\n"))
		_, _ = w.Write([]byte(`data: [DONE]` + "\n"))
	}))
	defer srv.Close()
	p := pipelineForTest(t, &stubEmbedder{}, &stubStore{results: seedResults()}, "claude", srv)
	ch, err := p.Answer(context.Background(), "q")
	if err != nil {
		t.Fatalf("Answer returned error: %v", err)
	}
	if got := drain(ch); got != "AAABBB" {
		t.Errorf("stream = %q, want %q", got, "AAABBB")
	}
}

func TestStreamClaude_ErrorStatus(t *testing.T) {
	requireNetwork(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "rate", http.StatusTooManyRequests)
	}))
	defer srv.Close()
	p := pipelineForTest(t, &stubEmbedder{}, &stubStore{results: seedResults()}, "claude", srv)
	_, err := p.Answer(context.Background(), "q")
	if err == nil {
		t.Fatal("expected claude error, got nil")
	}
	if !strings.Contains(err.Error(), "claude error 429") {
		t.Errorf("error = %v, want 'claude error 429'", err)
	}
}

// ---------- default switch (unknown ChatLLM → Ollama) ----------

func TestAnswer_DefaultSwitchFallsBackToOllama(t *testing.T) {
	requireNetwork(t)
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Path != "/api/generate" {
			t.Errorf("default branch should hit Ollama path, got %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write([]byte(`{"response":"ok","done":true}` + "\n"))
	}))
	defer srv.Close()
	p := pipelineForTest(t, &stubEmbedder{}, &stubStore{results: seedResults()}, "something-unknown", srv)
	ch, err := p.Answer(context.Background(), "q")
	if err != nil {
		t.Fatalf("Answer returned error: %v", err)
	}
	_ = drain(ch)
	if hits == 0 {
		t.Error("default branch did not call Ollama path")
	}
}

// ---------- helpers ----------

func drain(ch <-chan string) string {
	var b strings.Builder
	for s := range ch {
		b.WriteString(s)
	}
	return b.String()
}
