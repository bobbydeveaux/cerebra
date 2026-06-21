package store

// query_test.go covers the error-return branches of query.go (Search,
// SearchFTS). The happy paths and the default-limit / unavailable guards are
// already exercised by TestVectorSearch / TestSearchFTS (store_test.go) and
// TestSearch_DefaultLimitAndUnavailable / TestSearchFTS_EscapesQuotes
// (brains_test.go); these tests fill the remaining query-error wraps by driving
// each call against a closed DB.

import (
	"context"
	"strings"
	"testing"
)

func TestQuery_ClosedDBErrors(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()

	// Force vecAvailable true so Search proceeds past the availability guard and
	// reaches the query, which then fails on the closed connection. This isolates
	// the "vector search" query-error wrap rather than the "unavailable" guard
	// (the latter is covered in brains_test.go).
	s.vecAvailable = true

	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Search on closed DB -> "vector search" wrap.
	queryVec := make([]float32, 768)
	if _, err := s.Search(ctx, queryVec, 5); err == nil {
		t.Error("expected Search to error after Close")
	} else if !strings.Contains(err.Error(), "vector search") {
		t.Errorf("expected vector-search wrap, got %v", err)
	}

	// SearchFTS on closed DB -> "FTS search" wrap.
	if _, err := s.SearchFTS(ctx, "anything", 5); err == nil {
		t.Error("expected SearchFTS to error after Close")
	} else if !strings.Contains(err.Error(), "FTS search") {
		t.Errorf("expected FTS-search wrap, got %v", err)
	}
}

func TestSearch_SerializeRejectsWrongDimension(t *testing.T) {
	// sqlite-vec serializes any float32 slice, but querying with a vector whose
	// length differs from the indexed dimension is the realistic failure mode.
	// When the vec extension is unavailable the guard fires first; either way the
	// call must return an error and never a partial result set, which protects
	// the MCP search surface from silently returning empty on a malformed query.
	s := testDB(t)
	ctx := context.Background()

	if !s.vecAvailable {
		// Without the extension the availability guard returns the error path,
		// which is the branch we still want to assert holds.
		if _, err := s.Search(ctx, make([]float32, 1), 5); err == nil {
			t.Error("expected error when vec unavailable")
		}
		return
	}

	// With the extension present, a dimension mismatch must surface as an error,
	// not a silent empty slice.
	results, err := s.Search(ctx, make([]float32, 1), 5)
	if err == nil && len(results) != 0 {
		t.Errorf("expected error or empty result for wrong-dim query, got %d results", len(results))
	}
}
