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
