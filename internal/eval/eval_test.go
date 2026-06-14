package eval

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bobbydeveaux/cerebra/internal/store"
)

func seededDB(t *testing.T) *store.SQLiteStore {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "eval.db")
	db, err := store.New(dbPath, 768)
	if err != nil {
		t.Fatalf("creating test DB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := Seed(context.Background(), db); err != nil {
		t.Fatalf("seeding fixtures: %v", err)
	}
	return db
}

// closedDB returns a store handle whose underlying database/sql connection has
// been closed, so any query against it fails with sql.ErrConnDone. Used to
// exercise the error branches of Seed and Run without changing production code.
func closedDB(t *testing.T) *store.SQLiteStore {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "closed.db")
	db, err := store.New(dbPath, 768)
	if err != nil {
		t.Fatalf("creating test DB: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("closing test DB: %v", err)
	}
	return db
}

func TestQuestionsNonEmpty(t *testing.T) {
	if len(Questions()) == 0 {
		t.Fatal("expected a non-empty CI question set")
	}
}

func TestRunPassesAboveThreshold(t *testing.T) {
	db := seededDB(t)
	rep, err := Run(context.Background(), db, Questions(), 5)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rep.Total != len(Questions()) {
		t.Fatalf("expected Total %d, got %d", len(Questions()), rep.Total)
	}
	if rep.Pass+rep.Fail != rep.Total {
		t.Fatalf("pass+fail %d != total %d", rep.Pass+rep.Fail, rep.Total)
	}
	if !rep.Meets(0.70) {
		for _, r := range rep.Results {
			if !r.Pass {
				t.Logf("FAIL %s: missing %q (hits=%d)", r.Question.ID, r.Missing, r.Hits)
			}
		}
		t.Fatalf("expected pass rate >= 0.70 on the fixture corpus, got %.2f (%d/%d)", rep.PassRate, rep.Pass, rep.Total)
	}
}

func TestRunReportsFailureForUnsatisfiableTerm(t *testing.T) {
	db := seededDB(t)
	qs := []Question{
		{
			ID:    "X01",
			Query: "Cerebra fork Fortress",
			MustContain: []string{
				"this_phrase_is_not_in_any_fixture_xyz",
			},
		},
	}
	rep, err := Run(context.Background(), db, qs, 5)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rep.Fail != 1 {
		t.Fatalf("expected 1 failure, got %d", rep.Fail)
	}
	if rep.Meets(0.70) {
		t.Fatal("expected Meets(0.70) to be false when the only question fails")
	}
}

func TestSeedErrorPath(t *testing.T) {
	err := Seed(context.Background(), closedDB(t))
	if err == nil {
		t.Fatal("expected Seed to return an error when the store write fails, got nil")
	}
}

func TestRunEmptyQuestions(t *testing.T) {
	db := seededDB(t)
	rep, err := Run(context.Background(), db, []Question{}, 5)
	if err != nil {
		t.Fatalf("Run with empty questions: unexpected error %v", err)
	}
	if rep.Total != 0 {
		t.Fatalf("expected Total 0, got %d", rep.Total)
	}
	if rep.Pass != 0 || rep.Fail != 0 {
		t.Fatalf("expected zero pass/fail, got pass=%d fail=%d", rep.Pass, rep.Fail)
	}
	if rep.PassRate != 0 {
		t.Fatalf("expected PassRate 0 for empty question set, got %v", rep.PassRate)
	}
	if len(rep.Results) != 0 {
		t.Fatalf("expected no results, got %d", len(rep.Results))
	}
}

func TestReportMeetsBoundary(t *testing.T) {
	if !(Report{PassRate: 0.75}).Meets(0.75) {
		t.Fatal("expected Meets to be inclusive: PassRate 0.75 should meet threshold 0.75")
	}
	if (Report{PassRate: 0.75}).Meets(0.751) {
		t.Fatal("expected PassRate 0.75 to fall short of threshold 0.751")
	}
}

func TestRunContextCancelled(t *testing.T) {
	db := seededDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Run(ctx, db, Questions(), 5)
	if err == nil {
		t.Fatal("expected Run to return an error when the context is cancelled, got nil")
	}
}
