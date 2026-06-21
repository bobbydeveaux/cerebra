package store

// activity_test.go covers the error-return branches of activity.go
// (UpsertActivity, ListActivity, DeleteBrainActivity). The happy paths and
// accumulation semantics are already exercised by
// TestActivity_UpsertAccumulatesAndDeletes in brains_test.go; these tests fill
// the remaining error-wrap branches by driving each call against a closed DB,
// the same convention used by TestAgentMessages_ClosedDBErrors.

import (
	"context"
	"strings"
	"testing"
)

func TestActivity_ClosedDBErrors(t *testing.T) {
	// Closing the store before each call forces the driver to return
	// ErrConnDone, exercising every fmt.Errorf wrap in activity.go in one place.
	s := testDB(t)
	ctx := context.Background()

	// Seed a brain + one activity row so the queries have a real target before
	// the close trips them.
	if err := s.UpsertBrain(ctx, Brain{
		BrainID: "act-closed", ProjectKey: "actproj", ProjectPath: "/a", SessionFile: "f",
	}); err != nil {
		t.Fatalf("seed brain: %v", err)
	}
	if err := s.UpsertActivity(ctx, HourlyActivity{
		BrainID: "act-closed", Hour: "2026-06-21T09", ProjectKey: "actproj",
		UserMsgs: 1, AsstMsgs: 1, ToolUses: 1, Tokens: 10,
	}); err != nil {
		t.Fatalf("seed activity: %v", err)
	}

	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// UpsertActivity on closed DB -> "upserting activity" wrap.
	if err := s.UpsertActivity(ctx, HourlyActivity{
		BrainID: "act-closed", Hour: "2026-06-21T10", ProjectKey: "actproj",
	}); err == nil {
		t.Error("expected UpsertActivity to error after Close")
	} else if !strings.Contains(err.Error(), "upserting activity") {
		t.Errorf("expected upsert wrap, got %v", err)
	}

	// ListActivity on closed DB -> "listing activity" wrap.
	if _, err := s.ListActivity(ctx, "actproj", "2026-06-21"); err == nil {
		t.Error("expected ListActivity to error after Close")
	} else if !strings.Contains(err.Error(), "listing activity") {
		t.Errorf("expected list wrap, got %v", err)
	}

	// DeleteBrainActivity on closed DB -> "deleting brain activity" wrap.
	if err := s.DeleteBrainActivity(ctx, "act-closed"); err == nil {
		t.Error("expected DeleteBrainActivity to error after Close")
	} else if !strings.Contains(err.Error(), "deleting brain activity") {
		t.Errorf("expected delete wrap, got %v", err)
	}
}

func TestListActivity_NoFilters(t *testing.T) {
	// The 1=1 base query with both filters empty returns every row ordered by
	// hour. This covers the no-filter branch (neither projectKey nor date set)
	// which the filtered cases in brains_test.go do not exercise directly.
	s := testDB(t)
	ctx := context.Background()

	if err := s.UpsertBrain(ctx, Brain{
		BrainID: "nf-brain", ProjectKey: "nfproj", ProjectPath: "/n", SessionFile: "f",
	}); err != nil {
		t.Fatalf("seed brain: %v", err)
	}

	// Insert out of hour order to confirm the ORDER BY hour ASC.
	for _, hour := range []string{"2026-06-21T12", "2026-06-21T08", "2026-06-21T10"} {
		if err := s.UpsertActivity(ctx, HourlyActivity{
			BrainID: "nf-brain", Hour: hour, ProjectKey: "nfproj", UserMsgs: 1,
		}); err != nil {
			t.Fatalf("UpsertActivity %s: %v", hour, err)
		}
	}

	rows, err := s.ListActivity(ctx, "", "")
	if err != nil {
		t.Fatalf("ListActivity no filters: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	if rows[0].Hour != "2026-06-21T08" || rows[2].Hour != "2026-06-21T12" {
		t.Errorf("rows not ordered by hour ASC: %s ... %s", rows[0].Hour, rows[2].Hour)
	}
}
