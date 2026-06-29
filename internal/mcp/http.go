package mcp

// http.go implements a multi-tenant MCP transport over HTTP using the MCP
// HTTP+SSE transport convention (protocol revision 2024-11-05):
//
//   - The client opens a GET to the SSE endpoint (default /mcp/sse). The server
//     replies with an SSE stream and immediately emits an `endpoint` event whose
//     data is the URL the client should POST JSON-RPC messages to. That URL
//     carries a per-connection session id.
//   - The client POSTs JSON-RPC requests to that messages endpoint (default
//     /mcp/message?session=...). The HTTP handler returns 202 Accepted and the
//     JSON-RPC response is delivered back over the matching SSE stream.
//
// Every request — both the SSE open and each POST — must carry a valid signed
// token (Authorization: Bearer <token>, or ?token=<token> for clients that
// cannot set headers). The token resolves to a user_id; the request is then
// dispatched against ONLY that user's *Server (and therefore that user's DB).
// No token, bad token, or unknown user => 401 and zero data. This is the
// isolation invariant: there is no unauthenticated code path and no shared
// store.

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

const (
	defaultSSEPath     = "/mcp/sse"
	defaultMessagePath = "/mcp/message"
)

// HTTPServer is the multi-tenant HTTP MCP transport. It owns the per-user
// Registry and the token secret, and routes authenticated requests to the
// correct user's dispatcher.
type HTTPServer struct {
	registry *Registry
	secret   string

	ssePath     string
	messagePath string

	mu       sync.Mutex
	sessions map[string]*sseSession
}

// sseSession is one live SSE connection. It is pinned to the user id that was
// authenticated when the stream opened; POSTs that reuse this session must
// authenticate as the same user, so a session can never be used to reach
// another tenant's data.
type sseSession struct {
	userID string
	events chan []byte
}

// NewHTTPServer constructs the transport. secret MUST be non-empty; callers
// (cmd/serve) are responsible for refusing to start when CEREBRA_TOKEN_SECRET
// is unset, so this is a defensive second check.
func NewHTTPServer(registry *Registry, secret string) (*HTTPServer, error) {
	if secret == "" {
		return nil, ErrEmptySecret
	}
	return &HTTPServer{
		registry:    registry,
		secret:      secret,
		ssePath:     defaultSSEPath,
		messagePath: defaultMessagePath,
		sessions:    make(map[string]*sseSession),
	}, nil
}

// SetSSEPath overrides the SSE endpoint path. Call before Handler().
func (h *HTTPServer) SetSSEPath(path string) {
	if path != "" {
		h.ssePath = path
	}
}

// Handler returns the http.Handler for the transport. Mount it on a mux/server
// of your choosing.
func (h *HTTPServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(h.ssePath, h.handleSSE)
	mux.HandleFunc(h.messagePath, h.handleMessage)
	return mux
}

// authenticate extracts and verifies the token from the request, returning the
// resolved user id. Bearer header is preferred; ?token= is accepted as a
// fallback for SSE clients that cannot set headers. Any failure returns an
// error and the caller must respond 401 with no data.
func (h *HTTPServer) authenticate(r *http.Request) (string, error) {
	token := bearerToken(r.Header.Get("Authorization"))
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	if token == "" {
		return "", ErrMalformedToken
	}
	return VerifyToken(token, h.secret)
}

func bearerToken(header string) string {
	const prefix = "Bearer "
	if len(header) >= len(prefix) && strings.EqualFold(header[:len(prefix)], prefix) {
		return strings.TrimSpace(header[len(prefix):])
	}
	return ""
}

// handleSSE opens an authenticated SSE stream and registers a session bound to
// the authenticated user. It emits the `endpoint` event then streams JSON-RPC
// responses until the client disconnects.
func (h *HTTPServer) handleSSE(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID, err := h.authenticate(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	sessionID := newSessionID()
	sess := &sseSession{userID: userID, events: make(chan []byte, 16)}

	h.mu.Lock()
	h.sessions[sessionID] = sess
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.sessions, sessionID)
		h.mu.Unlock()
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	// Tell the client where to POST. Preserve the token in the endpoint URL so
	// clients that authenticate via ?token= on the SSE open keep working on the
	// POST without needing to set a header.
	endpoint := h.messagePath + "?session=" + url.QueryEscape(sessionID)
	if tok := r.URL.Query().Get("token"); tok != "" {
		endpoint += "&token=" + url.QueryEscape(tok)
	}
	fmt.Fprintf(w, "event: endpoint\ndata: %s\n\n", endpoint)
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-sess.events:
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", msg)
			flusher.Flush()
		}
	}
}

// handleMessage accepts a JSON-RPC request, dispatches it against the
// authenticated user's store, and delivers the response over the matching SSE
// stream. It re-authenticates every POST (stateless auth) and additionally
// verifies that the authenticated user matches the session's user, so a leaked
// or guessed session id cannot be paired with another user's token.
func (h *HTTPServer) handleMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID, err := h.authenticate(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	sessionID := r.URL.Query().Get("session")

	h.mu.Lock()
	sess := h.sessions[sessionID]
	h.mu.Unlock()
	if sess == nil {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	// A session is pinned to the user who opened it. The token presented on the
	// POST must resolve to that same user — otherwise this is a cross-tenant
	// attempt and is refused.
	if sess.userID != userID {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req jsonRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json-rpc", http.StatusBadRequest)
		return
	}

	srv, err := h.registry.ServerFor(userID)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Notifications (no id) get no response, mirroring the stdio loop.
	if req.ID == nil || string(req.ID) == "null" {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	resp := srv.handle(r.Context(), req)
	payload, err := json.Marshal(resp)
	if err != nil {
		http.Error(w, "encoding response", http.StatusInternalServerError)
		return
	}

	select {
	case sess.events <- payload:
		w.WriteHeader(http.StatusAccepted)
	case <-r.Context().Done():
		http.Error(w, "client gone", http.StatusGone)
	}
}

// HandleMessageSync is a non-SSE convenience: it authenticates, dispatches, and
// writes the JSON-RPC response directly in the HTTP response body. It is not
// part of the SSE transport but is useful for simple clients and for testing
// the auth+routing path without managing an SSE stream. Same isolation rules
// apply: bad/missing token => 401, zero data.
func (h *HTTPServer) HandleMessageSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID, err := h.authenticate(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req jsonRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json-rpc", http.StatusBadRequest)
		return
	}

	srv, err := h.registry.ServerFor(userID)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	resp := srv.handle(r.Context(), req)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		// Response is already partially written; nothing more we can do.
		return
	}
}

func newSessionID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand should not fail; if it does, fall back to a context-free
		// value rather than panicking the whole server.
		return "session-fallback"
	}
	return hex.EncodeToString(b[:])
}
