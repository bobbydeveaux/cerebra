package mcp

// http_test.go is the isolation proof for the multi-tenant HTTP transport.
//
// It seeds TWO real per-user SQLite databases under a temp data-dir with
// distinguishable data, starts the HTTP transport via httptest, and asserts:
//
//   - user A's token returns ONLY user A's data,
//   - user B's data is never visible to user A (and vice versa),
//   - a missing or invalid token gets 401 with no data.
//
// If a cross-tenant leak ever appears, the assertions below fail loudly.
//
// These tests build a real store (CGO + sqlite_fts5), so run with:
//
//	CGO_ENABLED=1 go test -tags sqlite_fts5 ./internal/mcp/

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bobbydeveaux/cerebra/internal/store"
)

// httpTestEmbedder is a stateless embedder with small fixed-dimension vectors.
// The tools exercised here (list_brains, search_agent, get_stats) do not depend
// on embeddings, but the registry needs a Dimensions() to size the vec table.
type httpTestEmbedder struct{}

func (httpTestEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = []float32{0.1, 0.2, 0.3, 0.4}
	}
	return out, nil
}
func (httpTestEmbedder) Dimensions() int { return 4 }

// seedUser creates a per-user store at <dataDir>/<userID>/jor-el.db and writes
// one brain and one agent message tagged with the user so cross-tenant leaks
// are obvious. The store is closed afterwards so the registry reopens it.
func seedUser(t *testing.T, dataDir, userID string) {
	t.Helper()
	dbPath := filepath.Join(dataDir, userID, "jor-el.db")
	st, err := store.New(dbPath, 4)
	if err != nil {
		t.Fatalf("seed store for %s: %v", userID, err)
	}
	defer st.Close()

	ctx := context.Background()
	if err := st.UpsertBrain(ctx, store.Brain{
		BrainID:    "brain-" + userID,
		ProjectKey: "proj-" + userID,
		Status:     "active",
	}); err != nil {
		t.Fatalf("seed brain for %s: %v", userID, err)
	}
	if err := st.UpsertAgentMessage(ctx, store.AgentMessage{
		ID:        "msg-" + userID,
		AgentName: "agent-" + userID,
		Prompt:    "secret-prompt-of-" + userID,
		Response:  "secret-response-of-" + userID,
		Timestamp: "2026-06-01T10:00:00Z",
	}); err != nil {
		t.Fatalf("seed agent message for %s: %v", userID, err)
	}
}

// callTool drives the synchronous message handler with a tools/call request
// using httptest.ResponseRecorder. Driving the handler in-process (rather than
// over a real socket) exercises the identical authenticate -> ServerFor ->
// handle path while staying inside the sandbox, which forbids binding sockets.
// authHeader empty means no Authorization header; queryToken non-empty appends
// ?token= to the URL. Returns the HTTP status and the response body.
func callTool(t *testing.T, h *HTTPServer, authHeader, queryToken, tool string, args map[string]interface{}) (int, string) {
	t.Helper()

	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params":  map[string]interface{}{"name": tool, "arguments": args},
	}
	body, _ := json.Marshal(reqBody)

	target := "/sync"
	if queryToken != "" {
		target += "?token=" + queryToken
	}
	httpReq := httptest.NewRequest(http.MethodPost, target, strings.NewReader(string(body)))
	if authHeader != "" {
		httpReq.Header.Set("Authorization", authHeader)
	}

	rec := httptest.NewRecorder()
	h.HandleMessageSync(rec, httpReq)

	res := rec.Result()
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(raw)
}

// newTestHandler wires a registry over dataDir and returns the HTTPServer. The
// synchronous message endpoint exercises the exact same authenticate +
// ServerFor + handle path used by the SSE POST handler.
func newTestHandler(t *testing.T, dataDir, secret string) *HTTPServer {
	t.Helper()
	reg := NewRegistry(dataDir, httpTestEmbedder{})
	t.Cleanup(func() { _ = reg.Close() })

	h, err := NewHTTPServer(reg, secret)
	if err != nil {
		t.Fatalf("NewHTTPServer: %v", err)
	}
	return h
}

func TestHTTPTenantIsolation(t *testing.T) {
	dataDir := t.TempDir()
	const secret = "isolation-secret"

	seedUser(t, dataDir, "userA")
	seedUser(t, dataDir, "userB")

	h := newTestHandler(t, dataDir, secret)

	tokenA, _ := SignToken("userA", secret)
	tokenB, _ := SignToken("userB", secret)

	// --- userA sees only userA via list_brains ---
	status, text := callTool(t, h, "Bearer "+tokenA, "", "list_brains", map[string]interface{}{})
	if status != http.StatusOK {
		t.Fatalf("userA list_brains status = %d, want 200 (body=%s)", status, text)
	}
	if !strings.Contains(text, "proj-userA") {
		t.Errorf("userA list_brains missing own data: %s", text)
	}
	if strings.Contains(text, "proj-userB") || strings.Contains(text, "brain-userB") {
		t.Fatalf("CROSS-TENANT LEAK: userA saw userB brain data: %s", text)
	}

	// --- userB sees only userB via list_brains ---
	status, text = callTool(t, h, "Bearer "+tokenB, "", "list_brains", map[string]interface{}{})
	if status != http.StatusOK {
		t.Fatalf("userB list_brains status = %d, want 200 (body=%s)", status, text)
	}
	if !strings.Contains(text, "proj-userB") {
		t.Errorf("userB list_brains missing own data: %s", text)
	}
	if strings.Contains(text, "proj-userA") || strings.Contains(text, "brain-userA") {
		t.Fatalf("CROSS-TENANT LEAK: userB saw userA brain data: %s", text)
	}

	// --- userA search_agent returns only userA agent data ---
	status, text = callTool(t, h, "Bearer "+tokenA, "", "search_agent", map[string]interface{}{})
	if status != http.StatusOK {
		t.Fatalf("userA search_agent status = %d, want 200 (body=%s)", status, text)
	}
	if !strings.Contains(text, "secret-prompt-of-userA") {
		t.Errorf("userA search_agent missing own data: %s", text)
	}
	if strings.Contains(text, "secret-prompt-of-userB") || strings.Contains(text, "agent-userB") {
		t.Fatalf("CROSS-TENANT LEAK: userA saw userB agent data: %s", text)
	}

	// --- userB search_agent returns only userB agent data ---
	status, text = callTool(t, h, "Bearer "+tokenB, "", "search_agent", map[string]interface{}{})
	if status != http.StatusOK {
		t.Fatalf("userB search_agent status = %d, want 200 (body=%s)", status, text)
	}
	if strings.Contains(text, "secret-prompt-of-userA") || strings.Contains(text, "agent-userA") {
		t.Fatalf("CROSS-TENANT LEAK: userB saw userA agent data: %s", text)
	}
}

func TestHTTPRejectsMissingToken(t *testing.T) {
	dataDir := t.TempDir()
	seedUser(t, dataDir, "userA")
	h := newTestHandler(t, dataDir, "s3cr3t")

	status, text := callTool(t, h, "", "", "list_brains", map[string]interface{}{})
	if status != http.StatusUnauthorized {
		t.Fatalf("no-token status = %d, want 401 (body=%s)", status, text)
	}
	if strings.Contains(text, "userA") {
		t.Fatalf("no-token response leaked data: %s", text)
	}
}

func TestHTTPRejectsInvalidToken(t *testing.T) {
	dataDir := t.TempDir()
	seedUser(t, dataDir, "userA")
	h := newTestHandler(t, dataDir, "s3cr3t")

	// Token signed with the wrong secret.
	bad, _ := SignToken("userA", "the-wrong-secret")

	status, text := callTool(t, h, "Bearer "+bad, "", "list_brains", map[string]interface{}{})
	if status != http.StatusUnauthorized {
		t.Fatalf("bad-token status = %d, want 401 (body=%s)", status, text)
	}
	if strings.Contains(text, "userA") {
		t.Fatalf("bad-token response leaked data: %s", text)
	}
}

func TestHTTPTokenViaQueryParam(t *testing.T) {
	dataDir := t.TempDir()
	const secret = "query-secret"
	seedUser(t, dataDir, "userA")
	h := newTestHandler(t, dataDir, secret)

	tokenA, _ := SignToken("userA", secret)

	status, text := callTool(t, h, "", tokenA, "list_brains", map[string]interface{}{})
	if status != http.StatusOK {
		t.Fatalf("query-param token status = %d, want 200 (body=%s)", status, text)
	}
	if !strings.Contains(text, "proj-userA") {
		t.Errorf("query-param token did not return userA data: %s", text)
	}
}

func TestNewHTTPServerRequiresSecret(t *testing.T) {
	reg := NewRegistry(t.TempDir(), httpTestEmbedder{})
	defer reg.Close()
	if _, err := NewHTTPServer(reg, ""); err != ErrEmptySecret {
		t.Errorf("NewHTTPServer empty secret err = %v, want ErrEmptySecret", err)
	}
}

func TestRegistryRejectsTraversalUserID(t *testing.T) {
	reg := NewRegistry(t.TempDir(), httpTestEmbedder{})
	defer reg.Close()
	for _, bad := range []string{"../escape", "a/b", "..", "", "with space", strings.Repeat("x", 200)} {
		if _, err := reg.ServerFor(bad); err == nil {
			t.Errorf("ServerFor(%q) succeeded; want rejection", bad)
		}
	}
}

func TestRegistryCachesServerPerUser(t *testing.T) {
	dataDir := t.TempDir()
	seedUser(t, dataDir, "userA")
	reg := NewRegistry(dataDir, httpTestEmbedder{})
	defer reg.Close()

	first, err := reg.ServerFor("userA")
	if err != nil {
		t.Fatalf("first ServerFor: %v", err)
	}
	second, err := reg.ServerFor("userA")
	if err != nil {
		t.Fatalf("second ServerFor: %v", err)
	}
	if first != second {
		t.Errorf("registry returned distinct *Server for same user; cache not working")
	}
}
