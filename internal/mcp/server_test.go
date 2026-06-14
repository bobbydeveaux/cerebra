package mcp

// server_test.go exercises every JSON-RPC method and tool handler in
// internal/mcp/server.go. The package is a pure dispatch layer over
// store.Store and embedder.Embedder, so we use in-memory fakes for both
// rather than spinning up a SQLite database or an Ollama backend.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/bobbydeveaux/cerebra/internal/chunker"
	"github.com/bobbydeveaux/cerebra/internal/scanner"
	"github.com/bobbydeveaux/cerebra/internal/store"
)

// ---------------------------------------------------------------------------
// Test doubles
// ---------------------------------------------------------------------------

// fakeStore implements store.Store with toggleable return values per call.
// Only the methods reachable from the MCP server are exercised; the rest
// return zero values so the surface stays minimal.
type fakeStore struct {
	categories []store.CategorySummary
	searchRes  []store.SearchResult
	ftsRes     []store.SearchResult
	doc        *scanner.Document
	docChunks  []chunker.Chunk
	stats      store.Stats
	brains     []store.Brain
	brain      *store.Brain
	activity   []store.HourlyActivity
	agentMsgs  []store.AgentMessage
	agentList  []store.AgentMessage

	listCatsErr     error
	getDocErr       error
	getStatsErr     error
	listBrainsErr   error
	getBrainErr     error
	listActivityErr error
	searchAgentErr  error
	listAgentActErr error
}

func (f *fakeStore) UpsertDocument(_ context.Context, _ scanner.Document, _ []chunker.Chunk) error {
	return nil
}
func (f *fakeStore) DeleteDocument(_ context.Context, _ string) error           { return nil }
func (f *fakeStore) UpdateScanState(_ context.Context, _ store.ScanState) error { return nil }

func (f *fakeStore) Search(_ context.Context, _ []float32, _ int) ([]store.SearchResult, error) {
	return f.searchRes, nil
}

func (f *fakeStore) SearchFTS(_ context.Context, _ string, _ int) ([]store.SearchResult, error) {
	return f.ftsRes, nil
}

func (f *fakeStore) GetDocument(_ context.Context, _ string) (*scanner.Document, []chunker.Chunk, error) {
	if f.getDocErr != nil {
		return nil, nil, f.getDocErr
	}
	return f.doc, f.docChunks, nil
}

func (f *fakeStore) GetScanState(_ context.Context, _ string) (*store.ScanState, error) {
	return nil, nil
}

func (f *fakeStore) GetStats(_ context.Context) (store.Stats, error) {
	if f.getStatsErr != nil {
		return store.Stats{}, f.getStatsErr
	}
	return f.stats, nil
}

func (f *fakeStore) ListCategories(_ context.Context) ([]store.CategorySummary, error) {
	if f.listCatsErr != nil {
		return nil, f.listCatsErr
	}
	return f.categories, nil
}

func (f *fakeStore) GetContentHash(_ context.Context, _ string) (string, error) {
	return "", nil
}

func (f *fakeStore) ListFilesByCategory(_ context.Context, _ string, _ int) ([]store.FileSummary, error) {
	return nil, nil
}

func (f *fakeStore) ListAllFiles(_ context.Context) ([]store.FileSummary, error) {
	return nil, nil
}

func (f *fakeStore) UpsertBrain(_ context.Context, _ store.Brain) error { return nil }

func (f *fakeStore) GetBrain(_ context.Context, _ string) (*store.Brain, error) {
	if f.getBrainErr != nil {
		return nil, f.getBrainErr
	}
	return f.brain, nil
}

func (f *fakeStore) ListBrains(_ context.Context, _ string, _ string, _ int) ([]store.Brain, error) {
	if f.listBrainsErr != nil {
		return nil, f.listBrainsErr
	}
	return f.brains, nil
}

func (f *fakeStore) GetBrainStats(_ context.Context) (store.BrainStats, error) {
	return store.BrainStats{}, nil
}

func (f *fakeStore) DeleteBrainActivity(_ context.Context, _ string) error          { return nil }
func (f *fakeStore) UpsertActivity(_ context.Context, _ store.HourlyActivity) error { return nil }

func (f *fakeStore) ListActivity(_ context.Context, _ string, _ string) ([]store.HourlyActivity, error) {
	if f.listActivityErr != nil {
		return nil, f.listActivityErr
	}
	return f.activity, nil
}

func (f *fakeStore) UpsertAgentMessage(_ context.Context, _ store.AgentMessage) error {
	return nil
}

func (f *fakeStore) SearchAgentMessages(_ context.Context, _ string, _ string, _ int) ([]store.AgentMessage, error) {
	if f.searchAgentErr != nil {
		return nil, f.searchAgentErr
	}
	return f.agentMsgs, nil
}

func (f *fakeStore) ListAgentActivity(_ context.Context, _ string, _ string, _ string, _ int) ([]store.AgentMessage, error) {
	if f.listAgentActErr != nil {
		return nil, f.listAgentActErr
	}
	return f.agentList, nil
}

func (f *fakeStore) Close() error { return nil }

// fakeEmbedder implements embedder.Embedder. embedErr toggles the failure
// branch so we can exercise the FTS fallback path in toolSearch.
type fakeEmbedder struct {
	vec      []float32
	embedErr error
}

func (e *fakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	if e.embedErr != nil {
		return nil, e.embedErr
	}
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = e.vec
	}
	return out, nil
}

func (e *fakeEmbedder) Dimensions() int { return 4 }

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newReq(id int, method string, params interface{}) jsonRPCRequest {
	idBytes, _ := json.Marshal(id)
	var paramsRaw json.RawMessage
	if params != nil {
		paramsRaw, _ = json.Marshal(params)
	}
	return jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      idBytes,
		Method:  method,
		Params:  paramsRaw,
	}
}

func toolCallReq(id int, name string, args interface{}) jsonRPCRequest {
	params := map[string]interface{}{"name": name}
	if args != nil {
		params["arguments"] = args
	}
	return newReq(id, "tools/call", params)
}

// decodeTextContent extracts the text payload from a tool response's
// {"content":[{"type":"text","text":"..."}]} envelope.
func decodeTextContent(t *testing.T, resp jsonRPCResponse) string {
	t.Helper()
	m, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("Result is not a map: %T", resp.Result)
	}
	content, ok := m["content"].([]map[string]interface{})
	if !ok {
		t.Fatalf("content not a []map[string]interface{}: %T", m["content"])
	}
	if len(content) == 0 {
		t.Fatalf("content slice empty")
	}
	text, _ := content[0]["text"].(string)
	return text
}

// ---------------------------------------------------------------------------
// Constructor + dispatch
// ---------------------------------------------------------------------------

func TestNewServer(t *testing.T) {
	st := &fakeStore{}
	emb := &fakeEmbedder{}
	srv := NewServer(st, emb)
	if srv == nil {
		t.Fatal("NewServer returned nil")
	}
	if srv.store == nil || srv.embedder == nil {
		t.Fatalf("dependencies not stored: store=%v embedder=%v", srv.store, srv.embedder)
	}
}

func TestHandleInitialize(t *testing.T) {
	srv := NewServer(&fakeStore{}, &fakeEmbedder{})
	req := newReq(1, "initialize", nil)
	resp := srv.handle(context.Background(), req)

	if resp.JSONRPC != "2.0" {
		t.Errorf("jsonrpc = %q, want 2.0", resp.JSONRPC)
	}
	if string(resp.ID) != "1" {
		t.Errorf("ID echo = %s, want 1", string(resp.ID))
	}
	m, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("Result not a map: %T", resp.Result)
	}
	if m["protocolVersion"] != "2024-11-05" {
		t.Errorf("protocolVersion = %v, want 2024-11-05", m["protocolVersion"])
	}
	info, ok := m["serverInfo"].(map[string]interface{})
	if !ok {
		t.Fatal("serverInfo missing or wrong type")
	}
	if info["name"] != "cerebra" {
		t.Errorf("serverInfo.name = %v, want cerebra", info["name"])
	}
	if _, ok := m["capabilities"]; !ok {
		t.Error("capabilities missing")
	}
}

func TestHandleToolsList(t *testing.T) {
	srv := NewServer(&fakeStore{}, &fakeEmbedder{})
	req := newReq(2, "tools/list", nil)
	resp := srv.handle(context.Background(), req)

	m, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("Result not a map: %T", resp.Result)
	}
	tools, ok := m["tools"].([]toolDef)
	if !ok {
		t.Fatalf("tools not a []toolDef: %T", m["tools"])
	}

	expected := []string{
		"search", "list_categories", "get_document", "get_stats",
		"list_brains", "get_brain", "get_activity", "search_agent",
		"list_agent_activity",
	}
	if len(tools) != len(expected) {
		t.Fatalf("tool count = %d, want %d", len(tools), len(expected))
	}
	got := make(map[string]bool, len(tools))
	for _, td := range tools {
		got[td.Name] = true
		if td.Description == "" {
			t.Errorf("tool %q has empty description", td.Name)
		}
		if td.InputSchema == nil {
			t.Errorf("tool %q has nil InputSchema", td.Name)
		}
	}
	for _, name := range expected {
		if !got[name] {
			t.Errorf("missing tool %q in tools/list", name)
		}
	}
}

func TestHandleUnknownMethod(t *testing.T) {
	srv := NewServer(&fakeStore{}, &fakeEmbedder{})
	req := newReq(3, "no/such/method", nil)
	resp := srv.handle(context.Background(), req)

	if resp.Error != nil {
		t.Errorf("expected no error for unknown method, got %+v", resp.Error)
	}
	m, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("Result not a map: %T", resp.Result)
	}
	if len(m) != 0 {
		t.Errorf("expected empty result map, got %v", m)
	}
}

func TestHandleToolsCallUnknownTool(t *testing.T) {
	srv := NewServer(&fakeStore{}, &fakeEmbedder{})
	req := toolCallReq(4, "definitely_not_a_real_tool", map[string]interface{}{})
	resp := srv.handle(context.Background(), req)

	if resp.Error == nil {
		t.Fatal("expected error for unknown tool, got nil")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("error code = %d, want -32601", resp.Error.Code)
	}
	if !strings.Contains(resp.Error.Message, "Unknown tool") {
		t.Errorf("error message = %q, want substring 'Unknown tool'", resp.Error.Message)
	}
}

func TestHandleToolsCallMalformedParams(t *testing.T) {
	srv := NewServer(&fakeStore{}, &fakeEmbedder{})
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`5`),
		Method:  "tools/call",
		// raw bytes that are not valid JSON for the params struct
		Params: json.RawMessage(`"not an object"`),
	}
	resp := srv.handle(context.Background(), req)

	if resp.Error == nil {
		t.Fatal("expected error for malformed params, got nil")
	}
	if resp.Error.Code != -32602 {
		t.Errorf("error code = %d, want -32602", resp.Error.Code)
	}
}

// ---------------------------------------------------------------------------
// toolSearch
// ---------------------------------------------------------------------------

func TestToolSearchHappyPathVector(t *testing.T) {
	st := &fakeStore{
		searchRes: []store.SearchResult{
			{
				Chunk: chunker.Chunk{
					Content:   "hello world",
					StartLine: 10,
					EndLine:   20,
					Metadata: chunker.ChunkMeta{
						Path:     "main.go",
						Repo:     "cerebra",
						Category: scanner.CategoryAPI,
					},
				},
				Score: 0.95,
			},
		},
	}
	emb := &fakeEmbedder{vec: []float32{0.1, 0.2, 0.3, 0.4}}
	srv := NewServer(st, emb)

	req := toolCallReq(10, "search", map[string]interface{}{"query": "hello", "limit": 5})
	resp := srv.handle(context.Background(), req)

	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	text := decodeTextContent(t, resp)
	if !strings.Contains(text, "hello world") {
		t.Errorf("response text missing content: %s", text)
	}
	if !strings.Contains(text, "main.go") {
		t.Errorf("response text missing file path: %s", text)
	}
	if !strings.Contains(text, `"category":"api"`) {
		t.Errorf("response text missing category: %s", text)
	}
}

func TestToolSearchFallsBackToFTSWhenEmbedFails(t *testing.T) {
	st := &fakeStore{
		ftsRes: []store.SearchResult{
			{
				Chunk: chunker.Chunk{
					Content:   "fts hit",
					StartLine: 1,
					EndLine:   2,
					Metadata:  chunker.ChunkMeta{Path: "fts.go", Repo: "cerebra"},
				},
				Score: 0.5,
			},
		},
	}
	emb := &fakeEmbedder{embedErr: errors.New("ollama down")}
	srv := NewServer(st, emb)

	req := toolCallReq(11, "search", map[string]interface{}{"query": "needle"})
	resp := srv.handle(context.Background(), req)

	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	text := decodeTextContent(t, resp)
	if !strings.Contains(text, "fts hit") {
		t.Errorf("expected FTS fallback content in response, got: %s", text)
	}
}

func TestToolSearchNoResults(t *testing.T) {
	srv := NewServer(&fakeStore{}, &fakeEmbedder{vec: []float32{0.1, 0.2, 0.3, 0.4}})
	req := toolCallReq(12, "search", map[string]interface{}{"query": "nothing"})
	resp := srv.handle(context.Background(), req)

	text := decodeTextContent(t, resp)
	if !strings.Contains(text, "No results found") {
		t.Errorf("expected 'No results found' message, got: %s", text)
	}
	if !strings.Contains(text, "nothing") {
		t.Errorf("expected query echoed in no-results message, got: %s", text)
	}
}

func TestToolSearchDefaultsLimitWhenZero(t *testing.T) {
	// limit defaults to 10 when missing or <= 0. We assert the call still
	// succeeds and reaches the fakeStore (which doesn't care what limit is).
	st := &fakeStore{
		searchRes: []store.SearchResult{{Chunk: chunker.Chunk{Content: "x"}}},
	}
	srv := NewServer(st, &fakeEmbedder{vec: []float32{1, 2, 3, 4}})
	req := toolCallReq(13, "search", map[string]interface{}{"query": "q"}) // no limit
	resp := srv.handle(context.Background(), req)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
}

// ---------------------------------------------------------------------------
// toolListCategories
// ---------------------------------------------------------------------------

func TestToolListCategoriesHappyPath(t *testing.T) {
	st := &fakeStore{
		categories: []store.CategorySummary{
			{Name: "api", FileCount: 3, ChunkCount: 12},
			{Name: "docs", FileCount: 1, ChunkCount: 4},
		},
	}
	srv := NewServer(st, &fakeEmbedder{})
	req := toolCallReq(20, "list_categories", nil)
	resp := srv.handle(context.Background(), req)

	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	text := decodeTextContent(t, resp)
	if !strings.Contains(text, `"name":"api"`) || !strings.Contains(text, `"name":"docs"`) {
		t.Errorf("categories missing in response: %s", text)
	}
}

func TestToolListCategoriesError(t *testing.T) {
	st := &fakeStore{listCatsErr: errors.New("db gone")}
	srv := NewServer(st, &fakeEmbedder{})
	req := toolCallReq(21, "list_categories", nil)
	resp := srv.handle(context.Background(), req)

	if resp.Error == nil {
		t.Fatal("expected error response, got nil")
	}
	if resp.Error.Code != -32000 {
		t.Errorf("error code = %d, want -32000", resp.Error.Code)
	}
}

// ---------------------------------------------------------------------------
// toolGetDocument
// ---------------------------------------------------------------------------

func TestToolGetDocumentHappyPath(t *testing.T) {
	st := &fakeStore{
		doc: &scanner.Document{
			RelPath:  "pkg/foo.go",
			Content:  "package foo",
			Category: scanner.CategoryAPI,
			Language: "go",
		},
		docChunks: []chunker.Chunk{
			{Content: "package foo", StartLine: 1, EndLine: 1},
		},
	}
	srv := NewServer(st, &fakeEmbedder{})
	req := toolCallReq(30, "get_document", map[string]interface{}{"path": "pkg/foo.go"})
	resp := srv.handle(context.Background(), req)

	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	text := decodeTextContent(t, resp)
	if !strings.Contains(text, `"path":"pkg/foo.go"`) {
		t.Errorf("path missing in response: %s", text)
	}
	if !strings.Contains(text, `"language":"go"`) {
		t.Errorf("language missing in response: %s", text)
	}
	if !strings.Contains(text, `"category":"api"`) {
		t.Errorf("category missing in response: %s", text)
	}
}

func TestToolGetDocumentNotFound(t *testing.T) {
	st := &fakeStore{getDocErr: errors.New("not found in store")}
	srv := NewServer(st, &fakeEmbedder{})
	req := toolCallReq(31, "get_document", map[string]interface{}{"path": "missing.go"})
	resp := srv.handle(context.Background(), req)

	if resp.Error == nil {
		t.Fatal("expected error response, got nil")
	}
	if resp.Error.Code != -32000 {
		t.Errorf("error code = %d, want -32000", resp.Error.Code)
	}
	if !strings.Contains(resp.Error.Message, "Not found") {
		t.Errorf("message = %q, want substring 'Not found'", resp.Error.Message)
	}
}

// TestToolGetDocumentNilDoc is a regression test for a nil-pointer panic: when
// the store returns (nil, nil, nil) -- no error but no document -- the handler
// must surface a structured -32000 rather than dereferencing the nil *Document.
// This mirrors the existing nil guard in toolGetBrain.
func TestToolGetDocumentNilDoc(t *testing.T) {
	st := &fakeStore{} // doc == nil, getDocErr == nil
	srv := NewServer(st, &fakeEmbedder{})
	req := toolCallReq(32, "get_document", map[string]interface{}{"path": "ghost.go"})
	resp := srv.handle(context.Background(), req)

	if resp.Error == nil {
		t.Fatal("expected -32000 for a nil document, got nil error (likely a panic was avoided only by this guard)")
	}
	if resp.Error.Code != -32000 {
		t.Errorf("error code = %d, want -32000", resp.Error.Code)
	}
}

// ---------------------------------------------------------------------------
// toolGetStats
// ---------------------------------------------------------------------------

func TestToolGetStatsHappyPath(t *testing.T) {
	st := &fakeStore{
		stats: store.Stats{Repos: 4, Files: 200, Chunks: 1200, Categories: 7},
	}
	srv := NewServer(st, &fakeEmbedder{})
	req := toolCallReq(40, "get_stats", nil)
	resp := srv.handle(context.Background(), req)

	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	text := decodeTextContent(t, resp)
	if !strings.Contains(text, `"repos":4`) || !strings.Contains(text, `"files":200`) {
		t.Errorf("stats missing in response: %s", text)
	}
}

func TestToolGetStatsError(t *testing.T) {
	st := &fakeStore{getStatsErr: errors.New("stats query failed")}
	srv := NewServer(st, &fakeEmbedder{})
	req := toolCallReq(41, "get_stats", nil)
	resp := srv.handle(context.Background(), req)

	if resp.Error == nil {
		t.Fatal("expected error, got nil")
	}
	if resp.Error.Code != -32000 {
		t.Errorf("error code = %d, want -32000", resp.Error.Code)
	}
}

// ---------------------------------------------------------------------------
// toolListBrains
// ---------------------------------------------------------------------------

func TestToolListBrainsHappyPath(t *testing.T) {
	st := &fakeStore{
		brains: []store.Brain{
			{BrainID: "abc", ProjectKey: "cerebra", Status: "active"},
			{BrainID: "def", ProjectKey: "cerebra", Status: "completed"},
		},
	}
	srv := NewServer(st, &fakeEmbedder{})
	req := toolCallReq(50, "list_brains", map[string]interface{}{"project": "cerebra"})
	resp := srv.handle(context.Background(), req)

	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	text := decodeTextContent(t, resp)
	if !strings.Contains(text, `"brain_id":"abc"`) {
		t.Errorf("brain abc missing in response: %s", text)
	}
}

func TestToolListBrainsDefaultsLimit(t *testing.T) {
	// limit=0 should default to 20. The fake doesn't care about the limit
	// itself; we assert the call still succeeds and serialises empty slice.
	srv := NewServer(&fakeStore{}, &fakeEmbedder{})
	req := toolCallReq(51, "list_brains", map[string]interface{}{})
	resp := srv.handle(context.Background(), req)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
}

func TestToolListBrainsError(t *testing.T) {
	st := &fakeStore{listBrainsErr: errors.New("brain query failed")}
	srv := NewServer(st, &fakeEmbedder{})
	req := toolCallReq(52, "list_brains", map[string]interface{}{})
	resp := srv.handle(context.Background(), req)
	if resp.Error == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// toolGetBrain
// ---------------------------------------------------------------------------

func TestToolGetBrainHappyPath(t *testing.T) {
	st := &fakeStore{
		brain: &store.Brain{BrainID: "abc", ProjectKey: "cerebra", Status: "active"},
	}
	srv := NewServer(st, &fakeEmbedder{})
	req := toolCallReq(60, "get_brain", map[string]interface{}{"brain_id": "abc"})
	resp := srv.handle(context.Background(), req)

	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	text := decodeTextContent(t, resp)
	if !strings.Contains(text, `"brain_id":"abc"`) {
		t.Errorf("brain missing in response: %s", text)
	}
}

func TestToolGetBrainNotFound(t *testing.T) {
	// fakeStore with brain=nil and no error simulates "no row returned".
	srv := NewServer(&fakeStore{}, &fakeEmbedder{})
	req := toolCallReq(61, "get_brain", map[string]interface{}{"brain_id": "missing"})
	resp := srv.handle(context.Background(), req)

	if resp.Error == nil {
		t.Fatal("expected error for missing brain, got nil")
	}
	if !strings.Contains(resp.Error.Message, "Brain not found") {
		t.Errorf("message = %q, want substring 'Brain not found'", resp.Error.Message)
	}
}

func TestToolGetBrainError(t *testing.T) {
	st := &fakeStore{getBrainErr: errors.New("query failed")}
	srv := NewServer(st, &fakeEmbedder{})
	req := toolCallReq(62, "get_brain", map[string]interface{}{"brain_id": "x"})
	resp := srv.handle(context.Background(), req)
	if resp.Error == nil {
		t.Fatal("expected error response, got nil")
	}
}

// ---------------------------------------------------------------------------
// toolGetActivity
// ---------------------------------------------------------------------------

func TestToolGetActivityHappyPath(t *testing.T) {
	st := &fakeStore{
		activity: []store.HourlyActivity{
			{BrainID: "abc", Hour: "2026-05-30T14", UserMsgs: 5, AsstMsgs: 5},
		},
	}
	srv := NewServer(st, &fakeEmbedder{})
	req := toolCallReq(70, "get_activity", map[string]interface{}{"project": "cerebra", "date": "2026-05-30"})
	resp := srv.handle(context.Background(), req)

	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	text := decodeTextContent(t, resp)
	if !strings.Contains(text, `"hour":"2026-05-30T14"`) {
		t.Errorf("activity hour missing: %s", text)
	}
}

func TestToolGetActivityError(t *testing.T) {
	st := &fakeStore{listActivityErr: errors.New("activity query failed")}
	srv := NewServer(st, &fakeEmbedder{})
	req := toolCallReq(71, "get_activity", map[string]interface{}{})
	resp := srv.handle(context.Background(), req)
	if resp.Error == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// toolSearchAgent
// ---------------------------------------------------------------------------

func TestToolSearchAgentHappyPath(t *testing.T) {
	st := &fakeStore{
		agentMsgs: []store.AgentMessage{
			{ID: "tu_1", AgentName: "marcus", Prompt: "what is drift", Response: "the gap"},
		},
	}
	srv := NewServer(st, &fakeEmbedder{})
	req := toolCallReq(80, "search_agent", map[string]interface{}{"agent_name": "marcus", "query": "drift"})
	resp := srv.handle(context.Background(), req)

	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	text := decodeTextContent(t, resp)
	if !strings.Contains(text, `"agent_name":"marcus"`) {
		t.Errorf("agent_name missing in response: %s", text)
	}
}

func TestToolSearchAgentDefaultsLimit(t *testing.T) {
	srv := NewServer(&fakeStore{}, &fakeEmbedder{})
	req := toolCallReq(81, "search_agent", map[string]interface{}{})
	resp := srv.handle(context.Background(), req)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
}

func TestToolSearchAgentError(t *testing.T) {
	st := &fakeStore{searchAgentErr: errors.New("fts5 query failed")}
	srv := NewServer(st, &fakeEmbedder{})
	req := toolCallReq(82, "search_agent", map[string]interface{}{})
	resp := srv.handle(context.Background(), req)
	if resp.Error == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// toolListAgentActivity
// ---------------------------------------------------------------------------

func TestToolListAgentActivityHappyPath(t *testing.T) {
	st := &fakeStore{
		agentList: []store.AgentMessage{
			{ID: "tu_2", AgentName: "iris", Timestamp: "2026-05-29T10:00:00Z"},
		},
	}
	srv := NewServer(st, &fakeEmbedder{})
	req := toolCallReq(90, "list_agent_activity", map[string]interface{}{
		"agent_name": "iris", "start_date": "2026-05-29", "end_date": "2026-05-30",
	})
	resp := srv.handle(context.Background(), req)

	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	text := decodeTextContent(t, resp)
	if !strings.Contains(text, `"agent_name":"iris"`) {
		t.Errorf("agent_name missing in response: %s", text)
	}
}

func TestToolListAgentActivityDefaultsLimit(t *testing.T) {
	srv := NewServer(&fakeStore{}, &fakeEmbedder{})
	req := toolCallReq(91, "list_agent_activity", map[string]interface{}{})
	resp := srv.handle(context.Background(), req)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
}

func TestToolListAgentActivityError(t *testing.T) {
	st := &fakeStore{listAgentActErr: errors.New("query failed")}
	srv := NewServer(st, &fakeEmbedder{})
	req := toolCallReq(92, "list_agent_activity", map[string]interface{}{})
	resp := srv.handle(context.Background(), req)
	if resp.Error == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// Serve — stdio loop
// ---------------------------------------------------------------------------

// withStdin replaces os.Stdin and os.Stdout for the duration of fn so we can
// drive Serve through pipes. The real Server.Serve hardcodes os.Stdin/os.Stdout
// because that is the MCP transport — there is no DI seam — so this is the
// idiomatic way to exercise the loop.
func withStdin(t *testing.T, in string, fn func()) string {
	t.Helper()
	origIn := os.Stdin
	origOut := os.Stdout

	rIn, wIn, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	rOut, wOut, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}

	os.Stdin = rIn
	os.Stdout = wOut

	// Drain stdout asynchronously into a buffer so the writer doesn't block.
	var captured bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(&captured, rOut)
	}()

	// Write the input, then close the write-end to signal EOF to Serve.
	if _, err := io.WriteString(wIn, in); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	_ = wIn.Close()

	fn()

	// Close stdout writer so the drain goroutine returns.
	_ = wOut.Close()
	wg.Wait()

	os.Stdin = origIn
	os.Stdout = origOut

	return captured.String()
}

func TestServeRespondsToRequest(t *testing.T) {
	srv := NewServer(&fakeStore{}, &fakeEmbedder{})
	input := `{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n"

	out := withStdin(t, input, func() {
		if err := srv.Serve(context.Background()); err != nil {
			t.Fatalf("Serve returned error: %v", err)
		}
	})

	if out == "" {
		t.Fatal("Serve wrote nothing to stdout")
	}
	// Should contain the protocol version from handleInitialize.
	if !strings.Contains(out, "2024-11-05") {
		t.Errorf("Serve output missing protocolVersion echo: %q", out)
	}
}

func TestServeSkipsMalformedAndNotifications(t *testing.T) {
	srv := NewServer(&fakeStore{}, &fakeEmbedder{})
	input := strings.Join([]string{
		`not valid json`, // skipped by Unmarshal-continue
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,    // no ID -> skipped
		`{"jsonrpc":"2.0","id":null,"method":"notifications/ping"}`, // explicit null ID -> skipped
		`{"jsonrpc":"2.0","id":7,"method":"initialize"}`,            // real request
	}, "\n") + "\n"

	out := withStdin(t, input, func() {
		if err := srv.Serve(context.Background()); err != nil {
			t.Fatalf("Serve returned error: %v", err)
		}
	})

	// Exactly one response should have been written (the initialize). Count
	// occurrences of the protocol version marker to confirm.
	count := strings.Count(out, "2024-11-05")
	if count != 1 {
		t.Errorf("expected exactly 1 response, got %d (out=%q)", count, out)
	}
	if !strings.Contains(out, `"id":7`) {
		t.Errorf("expected response for id=7, got %q", out)
	}
}

func TestServeReturnsOnEmptyStdin(t *testing.T) {
	srv := NewServer(&fakeStore{}, &fakeEmbedder{})
	out := withStdin(t, "", func() {
		if err := srv.Serve(context.Background()); err != nil {
			t.Fatalf("Serve returned error on empty stdin: %v", err)
		}
	})
	if out != "" {
		t.Errorf("expected no output for empty stdin, got %q", out)
	}
}

func TestServeReturnsScannerError(t *testing.T) {
	// Serve sizes its bufio.Scanner buffer at 1 MiB. A single line longer than
	// that overflows the buffer; bufio.Scanner stops and surfaces ErrTooLong
	// via sc.Err(), which Serve must return rather than swallow.
	//
	// The 2 MiB payload is far larger than a pipe's kernel buffer, so the write
	// must run concurrently with Serve reading; otherwise the writer would block
	// before Serve ever starts. We drive os.Stdin directly here rather than
	// through withStdin, which writes the whole input up front and would deadlock
	// on a payload this size.
	origIn := os.Stdin
	rIn, wIn, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdin = rIn
	t.Cleanup(func() {
		os.Stdin = origIn
	})

	go func() {
		// One line longer than the 1 MiB scanner buffer, then EOF.
		_, _ = io.WriteString(wIn, strings.Repeat("a", 2*1024*1024)+"\n")
		_ = wIn.Close()
	}()

	srv := NewServer(&fakeStore{}, &fakeEmbedder{})
	serveErr := srv.Serve(context.Background())

	if serveErr == nil {
		t.Fatal("expected Serve to return the scanner error, got nil")
	}
	if !errors.Is(serveErr, bufio.ErrTooLong) {
		t.Errorf("Serve error = %v, want bufio.ErrTooLong", serveErr)
	}
}

// ---------------------------------------------------------------------------
// Malformed tool arguments
//
// Every arg-taking handler unmarshals params.Arguments into a local struct and
// deliberately ignores the decode error, degrading to a zero-value input. This
// is the documented contract: a tool call with a syntactically valid envelope
// but garbage arguments must NOT surface a -32602 (that code is reserved for a
// malformed envelope, see TestHandleToolsCallMalformedParams). Instead it falls
// through to the store with empty inputs. These table-driven cases pin that
// behaviour so a future "tighten arg validation" refactor is a conscious choice
// rather than a silent regression.
// ---------------------------------------------------------------------------

func TestToolCallMalformedArgumentsDegradeToZeroValue(t *testing.T) {
	const garbageArgs = `"this is a string, not an arguments object"`

	tests := []struct {
		name string
		tool string
	}{
		{"search", "search"},
		{"get_document", "get_document"},
		{"list_brains", "list_brains"},
		{"get_brain", "get_brain"},
		{"get_activity", "get_activity"},
		{"search_agent", "search_agent"},
		{"list_agent_activity", "list_agent_activity"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &fakeStore{
				brain: &store.Brain{BrainID: "", ProjectKey: "p", Status: "active"},
			}
			srv := NewServer(st, &fakeEmbedder{vec: []float32{1, 2, 3, 4}})

			params, _ := json.Marshal(map[string]json.RawMessage{
				"name":      json.RawMessage(`"` + tt.tool + `"`),
				"arguments": json.RawMessage(garbageArgs),
			})
			req := newReq(100, "tools/call", nil)
			req.Params = params

			// The contract: garbage arguments must never panic and must never
			// produce -32602 (that code is reserved for a malformed envelope, see
			// TestHandleToolsCallMalformedParams). Returning a structured -32000
			// store/lookup error for a zero-value input (e.g. get_document with an
			// empty path) is acceptable; a nil-pointer panic is not.
			resp := srv.handle(context.Background(), req)
			if resp.Error != nil && resp.Error.Code == -32602 {
				t.Fatalf("tool %q returned -32602 for garbage arguments; that code is for a malformed envelope, not bad arguments", tt.tool)
			}
			if resp.Error == nil {
				if _, ok := resp.Result.(map[string]interface{}); !ok {
					t.Fatalf("tool %q success result is not a content envelope: %T", tt.tool, resp.Result)
				}
			}
		})
	}
}

// list_categories and get_stats take no arguments, so a garbage arguments blob
// can never reach them in a way that changes behaviour. This documents that
// they are intentionally excluded from the table above.
func TestNoArgToolsIgnoreArguments(t *testing.T) {
	st := &fakeStore{
		categories: []store.CategorySummary{{Name: "api", FileCount: 1, ChunkCount: 1}},
		stats:      store.Stats{Repos: 1},
	}
	srv := NewServer(st, &fakeEmbedder{})

	for _, tool := range []string{"list_categories", "get_stats"} {
		params, _ := json.Marshal(map[string]json.RawMessage{
			"name":      json.RawMessage(`"` + tool + `"`),
			"arguments": json.RawMessage(`12345`),
		})
		req := newReq(101, "tools/call", nil)
		req.Params = params

		resp := srv.handle(context.Background(), req)
		if resp.Error != nil {
			t.Errorf("no-arg tool %q returned error with junk arguments", tool)
		}
	}
}

// A tools/call with the name present but arguments entirely omitted must behave
// identically to empty arguments: the handlers default their limits and run.
func TestToolCallOmittedArguments(t *testing.T) {
	// Populate the lookups that have a not-found guard (get_document, get_brain)
	// so the defaulted zero-value input still reaches a success branch.
	st := &fakeStore{
		brain: &store.Brain{BrainID: "x", Status: "active"},
		doc:   &scanner.Document{RelPath: "x", Content: "x", Language: "go"},
	}
	srv := NewServer(st, &fakeEmbedder{vec: []float32{1, 2, 3, 4}})

	for _, tool := range []string{"search", "list_brains", "search_agent", "list_agent_activity", "get_activity", "get_document", "get_brain"} {
		req := toolCallReq(102, tool, nil)
		resp := srv.handle(context.Background(), req)
		if resp.Error != nil {
			t.Errorf("tool %q with omitted arguments returned error", tool)
		}
	}
}
