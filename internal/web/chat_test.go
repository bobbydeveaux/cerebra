package web

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	stripe "github.com/stripe/stripe-go/v76"
)

// TestLoggingStripeHandlerLogsAndReturnsNil exercises the default fallback
// StripeEventHandler — it must not return an error and must not panic on
// either dispatch type. This is the "no DB wired yet" code path that runs
// in the moment between agentops-011 ship and agentops-012 wiring.
func TestLoggingStripeHandlerLogsAndReturnsNil(t *testing.T) {
	h := loggingStripeHandler{}
	if err := h.OnCheckoutComplete(context.Background(), stripe.Event{ID: "evt_log_1"}); err != nil {
		t.Errorf("OnCheckoutComplete returned err: %v", err)
	}
	if err := h.OnSubscriptionDeleted(context.Background(), stripe.Event{ID: "evt_log_2"}); err != nil {
		t.Errorf("OnSubscriptionDeleted returned err: %v", err)
	}
}

func TestHandleChatPageRenders(t *testing.T) {
	st := newFakeStore()
	srv := newWikiServer(t, st, &fakeEmbedder{})

	req := httptest.NewRequest(http.MethodGet, "/chat", nil)
	w := httptest.NewRecorder()

	srv.handleChatPage(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Chat with Jor-El") {
		t.Errorf("expected chat title in body, got %q", w.Body.String())
	}
}

func TestHandleChatStreamMissingQuery(t *testing.T) {
	st := newFakeStore()
	srv := newWikiServer(t, st, &fakeEmbedder{})

	req := httptest.NewRequest(http.MethodGet, "/api/chat/stream", nil)
	w := httptest.NewRecorder()

	srv.handleChatStream(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Missing query") {
		t.Errorf("expected Missing query error, got %q", w.Body.String())
	}
}

// nonFlusherRecorder wraps an httptest.ResponseRecorder so it does NOT
// satisfy http.Flusher, which lets us cover the "Streaming not supported"
// branch of handleChatStream without needing a real pipeline.
type nonFlusherRecorder struct {
	rec *httptest.ResponseRecorder
}

func (n *nonFlusherRecorder) Header() http.Header         { return n.rec.Header() }
func (n *nonFlusherRecorder) Write(b []byte) (int, error) { return n.rec.Write(b) }
func (n *nonFlusherRecorder) WriteHeader(code int)        { n.rec.WriteHeader(code) }

func TestHandleChatStreamNonFlusher(t *testing.T) {
	st := newFakeStore()
	srv := newWikiServer(t, st, &fakeEmbedder{})

	req := httptest.NewRequest(http.MethodGet, "/api/chat/stream?q=hello", nil)
	rec := httptest.NewRecorder()
	w := &nonFlusherRecorder{rec: rec}

	srv.handleChatStream(w, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Streaming not supported") {
		t.Errorf("expected Streaming not supported, got %q", rec.Body.String())
	}
}

// TestHandleChatStreamPipelineError covers the branch where
// pipeline.AnswerWithHistory returns an error: the handler must emit a
// single `data: Error: <msg>` SSE frame followed by `data: [DONE]` and
// flush both. Locks in the contract that downstream SSE consumers see a
// terminator even on failure (otherwise the browser stalls).
func TestHandleChatStreamPipelineError(t *testing.T) {
	st := newFakeStore()
	srv := newWikiServer(t, st, &fakeEmbedder{})
	srv.pipeline = &fakeChatPipeline{answerErr: errors.New("embedder offline")}

	ts := httptest.NewServer(http.HandlerFunc(srv.handleChatStream))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "?q=hello")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200 (SSE streams 200 even on error)", resp.StatusCode)
	}
	if got, want := resp.Header.Get("Content-Type"), "text/event-stream"; got != want {
		t.Errorf("Content-Type = %q, want %q", got, want)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	bodyStr := string(body)

	if !strings.Contains(bodyStr, "data: Error: embedder offline\n\n") {
		t.Errorf("body missing Error SSE frame: %q", bodyStr)
	}
	if !strings.Contains(bodyStr, "data: [DONE]\n\n") {
		t.Errorf("body missing [DONE] terminator: %q", bodyStr)
	}
}

// TestHandleChatStreamPlainTokens covers the streaming happy path with no
// <think> tags: each token from the pipeline channel must surface as its
// own `data: <tok>\n\n` SSE frame, in order, terminated by `[DONE]`. This
// exercises the inThink=false branch of the token-handling loop.
func TestHandleChatStreamPlainTokens(t *testing.T) {
	st := newFakeStore()
	srv := newWikiServer(t, st, &fakeEmbedder{})
	srv.pipeline = &fakeChatPipeline{tokens: []string{"hello ", "world"}}

	ts := httptest.NewServer(http.HandlerFunc(srv.handleChatStream))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "?q=greet")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	bodyStr := string(body)

	wantFrames := []string{
		"data: hello \n\n",
		"data: world\n\n",
		"data: [DONE]\n\n",
	}
	for _, frame := range wantFrames {
		if !strings.Contains(bodyStr, frame) {
			t.Errorf("body missing frame %q; got %q", frame, bodyStr)
		}
	}

	// Order check: hello must come before world, world before [DONE].
	helloIdx := strings.Index(bodyStr, "data: hello")
	worldIdx := strings.Index(bodyStr, "data: world")
	doneIdx := strings.Index(bodyStr, "data: [DONE]")
	if !(helloIdx < worldIdx && worldIdx < doneIdx) {
		t.Errorf("frame ordering wrong: hello=%d world=%d done=%d body=%q",
			helloIdx, worldIdx, doneIdx, bodyStr)
	}
}

// TestHandleChatStreamStripsThinkBlock covers the inThink state machine:
// tokens arriving as `<think>` / `internal` / `</think>` / `visible answer`
// must be emitted with the think segment stripped. This mirrors how an
// LLM streams (one token at a time) and is the central branch the
// coverage gap leaves uncovered.
func TestHandleChatStreamStripsThinkBlock(t *testing.T) {
	st := newFakeStore()
	srv := newWikiServer(t, st, &fakeEmbedder{})
	srv.pipeline = &fakeChatPipeline{
		tokens: []string{"<think>", "internal reasoning", "</think>", "visible answer"},
	}

	ts := httptest.NewServer(http.HandlerFunc(srv.handleChatStream))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "?q=q")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	bodyStr := string(body)

	if strings.Contains(bodyStr, "internal reasoning") {
		t.Errorf("think-block content leaked to client: %q", bodyStr)
	}
	if !strings.Contains(bodyStr, "data: visible answer\n\n") {
		t.Errorf("expected `data: visible answer` after stripped think block: %q", bodyStr)
	}
	if !strings.Contains(bodyStr, "data: [DONE]\n\n") {
		t.Errorf("missing [DONE] terminator: %q", bodyStr)
	}
}

// TestHandleChatStreamEmitsTextBeforeThinkTag covers the branch where
// a single streamed token contains a `<think>` opener mid-token with
// non-empty leading content: the leading content must be emitted, then
// everything from `<think>` onward is suppressed until `</think>` is
// seen in a later token. Without this branch the prefix would be silently
// dropped on the floor.
func TestHandleChatStreamEmitsTextBeforeThinkTag(t *testing.T) {
	st := newFakeStore()
	srv := newWikiServer(t, st, &fakeEmbedder{})
	srv.pipeline = &fakeChatPipeline{
		tokens: []string{"prefix text<think>", "hidden", "</think>", "suffix"},
	}

	ts := httptest.NewServer(http.HandlerFunc(srv.handleChatStream))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "?q=q")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	bodyStr := string(body)

	if !strings.Contains(bodyStr, "data: prefix text\n\n") {
		t.Errorf("expected `data: prefix text` before think block: %q", bodyStr)
	}
	if strings.Contains(bodyStr, "hidden") {
		t.Errorf("think-block content leaked: %q", bodyStr)
	}
	if !strings.Contains(bodyStr, "data: suffix\n\n") {
		t.Errorf("expected `data: suffix` after think block: %q", bodyStr)
	}
}

// TestHandleChatStreamForwardsHistory covers the `history` query param
// parsing branch (json.Unmarshal on `r.URL.Query().Get("history")`). The
// fake pipeline records the parsed history slice so we can assert it
// reached the pipeline call unmodified.
func TestHandleChatStreamForwardsHistory(t *testing.T) {
	st := newFakeStore()
	srv := newWikiServer(t, st, &fakeEmbedder{})
	pipe := &fakeChatPipeline{tokens: []string{"ok"}}
	srv.pipeline = pipe

	ts := httptest.NewServer(http.HandlerFunc(srv.handleChatStream))
	defer ts.Close()

	history := `[{"role":"user","content":"hi"},{"role":"assistant","content":"hello"}]`
	u := ts.URL + "?q=follow-up&history=" + url.QueryEscape(history)

	resp, err := http.Get(u)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	resp.Body.Close()

	if pipe.gotQuestion != "follow-up" {
		t.Errorf("pipeline got question %q, want %q", pipe.gotQuestion, "follow-up")
	}
	if len(pipe.gotHistory) != 2 {
		t.Fatalf("pipeline got history len %d, want 2 — body parse failed: %#v",
			len(pipe.gotHistory), pipe.gotHistory)
	}
	if pipe.gotHistory[0]["role"] != "user" || pipe.gotHistory[0]["content"] != "hi" {
		t.Errorf("history[0] = %#v, want role=user content=hi", pipe.gotHistory[0])
	}
	if pipe.gotHistory[1]["role"] != "assistant" || pipe.gotHistory[1]["content"] != "hello" {
		t.Errorf("history[1] = %#v, want role=assistant content=hello", pipe.gotHistory[1])
	}
}

// Compile-time guard that fakeChatPipeline still satisfies the chatPipeline
// interface. If the interface gains methods, this fails at build time.
var _ chatPipeline = (*fakeChatPipeline)(nil)
