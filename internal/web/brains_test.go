package web

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bobbydeveaux/cerebra/internal/store"
)

func TestFormatTokenCount(t *testing.T) {
	cases := []struct {
		tokens int
		want   string
	}{
		{0, "0"},
		{1, "1"},
		{999, "999"},
		{1_000, "1.0K"},
		{1_500, "1.5K"},
		{12_345, "12.3K"},
		{999_999, "1000.0K"},
		{1_000_000, "1.0M"},
		{2_500_000, "2.5M"},
		{1_000_000_000, "1.0B"},
		{3_750_000_000, "3.8B"},
	}
	for _, c := range cases {
		if got := formatTokenCount(c.tokens); got != c.want {
			t.Errorf("formatTokenCount(%d) = %q, want %q", c.tokens, got, c.want)
		}
	}
}

func TestHandleBrainsHappyPath(t *testing.T) {
	st := newFakeStore()
	st.brainStats = store.BrainStats{TotalBrains: 2, ActiveBrains: 1, TotalMessages: 50, TotalTokens: 12_345, Projects: 1}
	st.brains = []store.Brain{
		{
			BrainID:        "brain-a",
			ProjectPath:    "/repos/cerebra",
			ProjectKey:     "cerebra",
			AgentType:      "claude-code",
			Model:          "opus-4.7",
			GitBranch:      "main",
			Status:         "active",
			MessageCount:   8,
			TokenUsage:     5000,
			FirstMessageAt: "2026-05-29T10:00:00Z",
			LastMessageAt:  "2026-05-29T11:00:00Z",
			Summary:        "short summary",
		},
		{
			BrainID:      "brain-b",
			ProjectPath:  "", // exercises fallback to ProjectKey
			ProjectKey:   "guardian",
			Status:       "completed",
			MessageCount: 200,
			TokenUsage:   7345,
			Summary:      strings.Repeat("x", 120), // exercises truncation branch
		},
	}
	srv := newWikiServer(t, st, &fakeEmbedder{})

	req := httptest.NewRequest(http.MethodGet, "/brains", nil)
	w := httptest.NewRecorder()

	srv.handleBrains(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
}

func TestHandleBrainsStatsError(t *testing.T) {
	st := newFakeStore()
	st.getBrainStatsErr = errors.New("boom-stats")
	srv := newWikiServer(t, st, &fakeEmbedder{})

	req := httptest.NewRequest(http.MethodGet, "/brains", nil)
	w := httptest.NewRecorder()

	srv.handleBrains(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "boom-stats") {
		t.Errorf("expected error body, got %q", w.Body.String())
	}
}

func TestHandleBrainsListError(t *testing.T) {
	st := newFakeStore()
	st.listBrainsErr = errors.New("boom-list")
	srv := newWikiServer(t, st, &fakeEmbedder{})

	req := httptest.NewRequest(http.MethodGet, "/brains", nil)
	w := httptest.NewRecorder()

	srv.handleBrains(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "boom-list") {
		t.Errorf("expected error body, got %q", w.Body.String())
	}
}

func TestHandleBrainDetailFound(t *testing.T) {
	st := newFakeStore()
	st.brainsByID["brain-x"] = &store.Brain{
		BrainID:        "brain-x",
		ProjectPath:    "/repos/cerebra",
		ProjectKey:     "cerebra",
		Model:          "opus-4.7",
		GitBranch:      "main",
		Status:         "active",
		MessageCount:   42,
		TokenUsage:     1_500_000,
		Version:        "1.0",
		AgentType:      "claude-code",
		FirstMessageAt: "2026-05-29T10:00:00Z",
		LastMessageAt:  "2026-05-29T12:00:00Z",
		Summary:        "Working on coverage tests",
	}
	srv := newWikiServer(t, st, &fakeEmbedder{})

	req := httptest.NewRequest(http.MethodGet, "/api/brains/brain-x", nil)
	req.SetPathValue("id", "brain-x")
	w := httptest.NewRecorder()

	srv.handleBrainDetail(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{"brain-x", "Active", "/repos/cerebra", "opus-4.7", "1.5M", "Working on coverage tests"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q. body=%s", want, body)
		}
	}
}

func TestHandleBrainDetailCompletedStatus(t *testing.T) {
	st := newFakeStore()
	st.brainsByID["brain-c"] = &store.Brain{
		BrainID:     "brain-c",
		ProjectPath: ".", // exercises ProjectKey fallback
		ProjectKey:  "fallback-project",
		Status:      "completed",
		TokenUsage:  500,
	}
	srv := newWikiServer(t, st, &fakeEmbedder{})

	req := httptest.NewRequest(http.MethodGet, "/api/brains/brain-c", nil)
	req.SetPathValue("id", "brain-c")
	w := httptest.NewRecorder()

	srv.handleBrainDetail(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Completed") {
		t.Errorf("expected Completed status badge, got %q", body)
	}
	if !strings.Contains(body, "fallback-project") {
		t.Errorf("expected ProjectKey fallback in body, got %q", body)
	}
}

func TestHandleBrainDetailNotFound(t *testing.T) {
	st := newFakeStore()
	srv := newWikiServer(t, st, &fakeEmbedder{})

	req := httptest.NewRequest(http.MethodGet, "/api/brains/missing", nil)
	req.SetPathValue("id", "missing")
	w := httptest.NewRecorder()

	srv.handleBrainDetail(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

func TestHandleBrainDetailStoreError(t *testing.T) {
	st := newFakeStore()
	st.getBrainErr = errors.New("boom-brain")
	srv := newWikiServer(t, st, &fakeEmbedder{})

	req := httptest.NewRequest(http.MethodGet, "/api/brains/x", nil)
	req.SetPathValue("id", "x")
	w := httptest.NewRecorder()

	srv.handleBrainDetail(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", w.Code)
	}
}
