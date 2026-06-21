package store

// agent_messages_test.go covers agent_messages.go: the per-session agent
// message store (UpsertAgentMessage merge semantics, SearchAgentMessages FTS +
// LIKE fallback, ListAgentActivity date/limit filtering, scanAgentMessages
// errors). These tests were extracted verbatim from brains_test.go into a
// dedicated file matching the package convention (activity.go->activity_test.go,
// query.go->query_test.go). FTS-backed paths require the sqlite_fts5 build tag.

import (
	"context"
	"strings"
	"testing"
	"time"
)

// --- agent_messages.go -------------------------------------------------------

func TestAgentMessages_UpsertAndSearch(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()

	// Empty ID is invalid.
	if err := s.UpsertAgentMessage(ctx, AgentMessage{}); err == nil {
		t.Error("expected error for empty agent_message id")
	}

	// First half: tool_use with prompt, no response yet.
	if err := s.UpsertAgentMessage(ctx, AgentMessage{
		ID:         "tool-1",
		BrainID:    "brain-A",
		AgentName:  "gopher",
		Prompt:     "implement coverage tests",
		Timestamp:  "2026-05-30T10:00:00Z",
		ProjectKey: "cerebra",
	}); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	// Second half: tool_result with response — must merge, not clobber.
	if err := s.UpsertAgentMessage(ctx, AgentMessage{
		ID:       "tool-1",
		Response: "wrote tests for brains, activity, agent_messages",
	}); err != nil {
		t.Fatalf("merge upsert: %v", err)
	}

	listed, err := s.ListAgentActivity(ctx, "gopher", "", "", 10)
	if err != nil {
		t.Fatalf("ListAgentActivity: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("expected 1 merged row, got %d", len(listed))
	}
	if listed[0].Prompt == "" || listed[0].Response == "" {
		t.Errorf("merge dropped a field: %+v", listed[0])
	}
	if listed[0].AgentName != "gopher" {
		t.Errorf("merge dropped agent_name: %+v", listed[0])
	}

	// Date range filtering.
	if err := s.UpsertAgentMessage(ctx, AgentMessage{
		ID: "tool-2", BrainID: "brain-A", AgentName: "gopher",
		Prompt: "later one", Timestamp: "2026-06-01T10:00:00Z", ProjectKey: "cerebra",
	}); err != nil {
		t.Fatalf("seed second: %v", err)
	}

	mayOnly, err := s.ListAgentActivity(ctx, "gopher", "2026-05-01", "2026-05-31", 10)
	if err != nil {
		t.Fatalf("ListAgentActivity date filter: %v", err)
	}
	if len(mayOnly) != 1 {
		t.Errorf("expected 1 result for May only, got %d", len(mayOnly))
	}
	if len(mayOnly) > 0 && mayOnly[0].ID != "tool-1" {
		t.Errorf("expected tool-1 for May filter, got %s", mayOnly[0].ID)
	}

	// FTS search across prompt + response.
	hits, err := s.SearchAgentMessages(ctx, "gopher", "coverage", 10)
	if err != nil {
		t.Fatalf("SearchAgentMessages: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("expected at least one FTS hit for 'coverage'")
	}

	// Empty query falls back to recent listing.
	recent, err := s.SearchAgentMessages(ctx, "gopher", "   ", 10)
	if err != nil {
		t.Fatalf("SearchAgentMessages empty query: %v", err)
	}
	if len(recent) != 2 {
		t.Errorf("expected 2 recent results, got %d", len(recent))
	}

	// agentName="" matches across all agents.
	if err := s.UpsertAgentMessage(ctx, AgentMessage{
		ID: "tool-3", BrainID: "brain-B", AgentName: "iris",
		Prompt: "draft a coverage retrospective post", Timestamp: "2026-05-30T12:00:00Z", ProjectKey: "brand",
	}); err != nil {
		t.Fatalf("seed iris: %v", err)
	}
	across, err := s.SearchAgentMessages(ctx, "", "coverage", 10)
	if err != nil {
		t.Fatalf("SearchAgentMessages across agents: %v", err)
	}
	// Expect tool-1 (gopher, "coverage tests") + tool-3 (iris, "coverage retrospective")
	if len(across) < 2 {
		t.Errorf("expected >=2 cross-agent hits, got %d", len(across))
	}

	// Limit honoured.
	limited, err := s.ListAgentActivity(ctx, "", "", "", 1)
	if err != nil {
		t.Fatalf("ListAgentActivity limit=1: %v", err)
	}
	if len(limited) != 1 {
		t.Errorf("expected 1 result with limit=1, got %d", len(limited))
	}
}

func TestAgentMessages_DefaultLimits(t *testing.T) {
	// Covers the limit <= 0 default branches in SearchAgentMessages (default 20)
	// and ListAgentActivity (default 50). With limit=0 supplied, the function
	// should still return seeded rows rather than truncating to zero.
	s := testDB(t)
	ctx := context.Background()

	for i, id := range []string{"dl-1", "dl-2", "dl-3"} {
		if err := s.UpsertAgentMessage(ctx, AgentMessage{
			ID: id, BrainID: "dl-brain", AgentName: "gopher",
			Prompt: "default limit seed", Response: "ok",
			Timestamp:  time.Now().UTC().Add(-time.Duration(i) * time.Hour).Format(time.RFC3339),
			ProjectKey: "cerebra",
		}); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}

	// limit=0 → SearchAgentMessages applies default 20 via the empty-query path
	// which delegates to ListAgentActivity. Both default branches exercised.
	hits, err := s.SearchAgentMessages(ctx, "gopher", "", 0)
	if err != nil {
		t.Fatalf("SearchAgentMessages default limit: %v", err)
	}
	if len(hits) != 3 {
		t.Errorf("expected 3 rows with default limit, got %d", len(hits))
	}

	// limit=0 → ListAgentActivity applies default 50 directly.
	listed, err := s.ListAgentActivity(ctx, "gopher", "", "", 0)
	if err != nil {
		t.Fatalf("ListAgentActivity default limit: %v", err)
	}
	if len(listed) != 3 {
		t.Errorf("expected 3 rows with default limit, got %d", len(listed))
	}

	// Negative limit also routes through the default branch.
	negHits, err := s.SearchAgentMessages(ctx, "gopher", "limit", -5)
	if err != nil {
		t.Fatalf("SearchAgentMessages negative limit: %v", err)
	}
	if len(negHits) == 0 {
		t.Error("expected at least 1 hit with negative limit (FTS path)")
	}
}

func TestAgentMessages_ClosedDBErrors(t *testing.T) {
	// Each function wraps the underlying SQL error with a fmt.Errorf. Closing the
	// store before the call forces the driver to return ErrConnDone, exercising
	// every error-wrap branch in one place.
	s := testDB(t)
	ctx := context.Background()

	// Seed one row so the queries have something to attempt against (the close
	// will trip them before any rows come back, but it keeps the semantics
	// honest if the driver order ever changes).
	if err := s.UpsertAgentMessage(ctx, AgentMessage{
		ID: "closed-1", BrainID: "b", AgentName: "gopher",
		Prompt: "hello", Response: "world", Timestamp: "2026-05-30T10:00:00Z",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// UpsertAgentMessage on closed DB → "upsert agent_message" wrap.
	if err := s.UpsertAgentMessage(ctx, AgentMessage{ID: "after-close"}); err == nil {
		t.Error("expected UpsertAgentMessage to error after Close")
	} else if !strings.Contains(err.Error(), "upsert agent_message") {
		t.Errorf("expected upsert wrap, got %v", err)
	}

	// ListAgentActivity on closed DB → "list agent activity" wrap.
	if _, err := s.ListAgentActivity(ctx, "", "", "", 5); err == nil {
		t.Error("expected ListAgentActivity to error after Close")
	} else if !strings.Contains(err.Error(), "list agent activity") {
		t.Errorf("expected list-activity wrap, got %v", err)
	}

	// SearchAgentMessages with a real query on a closed DB. The primary FTS
	// query errors → it falls back to LIKE, which also errors → returns the
	// "agent_messages LIKE search" wrap. Both error paths covered by one call.
	if _, err := s.SearchAgentMessages(ctx, "gopher", "coverage", 5); err == nil {
		t.Error("expected SearchAgentMessages to error after Close")
	} else if !strings.Contains(err.Error(), "agent_messages LIKE search") {
		t.Errorf("expected LIKE fallback wrap, got %v", err)
	}

	// Direct call to searchAgentMessagesFallback on closed DB → same wrap.
	if _, err := s.searchAgentMessagesFallback(ctx, "gopher", "coverage", 5); err == nil {
		t.Error("expected fallback to error after Close")
	} else if !strings.Contains(err.Error(), "agent_messages LIKE search") {
		t.Errorf("expected LIKE wrap, got %v", err)
	}
}

// fakeAgentRows satisfies the small interface that scanAgentMessages accepts.
// It surfaces a configurable Scan error on the first iteration so we can cover
// the error-wrap branch without needing a misbehaving driver.
type fakeAgentRows struct {
	scanErr error
	called  bool
	err     error
}

func (f *fakeAgentRows) Next() bool {
	if f.called {
		return false
	}
	f.called = true
	return true
}

func (f *fakeAgentRows) Scan(dest ...interface{}) error { return f.scanErr }
func (f *fakeAgentRows) Err() error                     { return f.err }

func TestScanAgentMessages_ScanError(t *testing.T) {
	rows := &fakeAgentRows{scanErr: errFakeScan}
	out, err := scanAgentMessages(rows)
	if err == nil {
		t.Fatal("expected scan error, got nil")
	}
	if !strings.Contains(err.Error(), "scan agent_message") {
		t.Errorf("expected scan wrap, got %v", err)
	}
	if out != nil {
		t.Errorf("expected nil rows, got %+v", out)
	}
}

// errFakeScan keeps the fake rows lightweight without pulling in a new dep.
var errFakeScan = fakeScanErr("forced scan failure")

type fakeScanErr string

func (e fakeScanErr) Error() string { return string(e) }

func TestAgentMessages_FTSFallback_ViaSearch(t *testing.T) {
	// SearchAgentMessages catches a primary FTS query error and falls back to
	// LIKE. Drop the FTS virtual table mid-test to force the FTS join to fail,
	// then verify the seeded row still comes back via the fallback path.
	s := testDB(t)
	if !s.ftsAvailable {
		t.Skip("FTS5 not built in; run: make test (uses -tags sqlite_fts5)")
	}
	ctx := context.Background()

	if err := s.UpsertAgentMessage(ctx, AgentMessage{
		ID: "fts-fb-1", BrainID: "b", AgentName: "gopher",
		Prompt:    "fallback path coverage",
		Response:  "via LIKE",
		Timestamp: "2026-05-30T10:00:00Z",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Drop the FTS table — the primary FTS query in SearchAgentMessages now
	// errors, triggering the searchAgentMessagesFallback branch.
	if _, err := s.db.ExecContext(ctx, `DROP TABLE agent_messages_fts`); err != nil {
		t.Fatalf("drop fts table: %v", err)
	}

	hits, err := s.SearchAgentMessages(ctx, "gopher", "fallback", 5)
	if err != nil {
		t.Fatalf("SearchAgentMessages (fts dropped): %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit via LIKE fallback, got %d", len(hits))
	}
	if hits[0].ID != "fts-fb-1" {
		t.Errorf("expected fts-fb-1, got %s", hits[0].ID)
	}
}

func TestAgentMessages_SearchFallback(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()

	if err := s.UpsertAgentMessage(ctx, AgentMessage{
		ID: "fallback-1", BrainID: "b", AgentName: "atlas",
		Prompt: "draft school email", Response: "drafted", Timestamp: "2026-05-30T09:00:00Z",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// searchAgentMessagesFallback uses LIKE — exercise it directly so the
	// branch is covered even when FTS5 is available (which it is in this build).
	got, err := s.searchAgentMessagesFallback(ctx, "atlas", "school", 10)
	if err != nil {
		t.Fatalf("fallback search: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 fallback hit, got %d", len(got))
	}

	// Across all agents.
	any, err := s.searchAgentMessagesFallback(ctx, "", "drafted", 10)
	if err != nil {
		t.Fatalf("fallback all agents: %v", err)
	}
	if len(any) != 1 {
		t.Errorf("expected 1 cross-agent fallback hit, got %d", len(any))
	}

	// No match returns empty slice, not error.
	none, err := s.searchAgentMessagesFallback(ctx, "atlas", "nonexistent-token", 10)
	if err != nil {
		t.Fatalf("fallback no match: %v", err)
	}
	if len(none) != 0 {
		t.Errorf("expected 0 hits, got %d", len(none))
	}
}
