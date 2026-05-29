package web

import (
	"context"
	"net/http"
	"net/http/httptest"
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
