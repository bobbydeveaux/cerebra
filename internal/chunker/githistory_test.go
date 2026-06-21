package chunker

import (
	"strings"
	"testing"
)

// TestGitHistoryChunker_ExactBoundary verifies that an input whose line count
// is exactly commitsPerChunk (20) produces a single chunk and never spills a
// trailing empty batch.
func TestGitHistoryChunker_ExactBoundary(t *testing.T) {
	g := NewGitHistoryChunker(512)

	lines := make([]string, 20)
	for i := range lines {
		lines[i] = "commit deadbeef line " + string(rune('a'+(i%26)))
	}
	content := strings.Join(lines, "\n")

	chunks, err := g.chunk(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(chunks) != 1 {
		t.Fatalf("expected exactly 1 chunk for 20 lines at boundary, got %d", len(chunks))
	}
	if chunks[0].startLine != 1 {
		t.Errorf("expected startLine 1, got %d", chunks[0].startLine)
	}
	if chunks[0].endLine != 20 {
		t.Errorf("expected endLine 20, got %d", chunks[0].endLine)
	}
	if !strings.Contains(chunks[0].content, "[Git History] Commits 1-20") {
		t.Errorf("expected header 'Commits 1-20', got %q", chunks[0].content)
	}
}

// TestGitHistoryChunker_RemainderBatch verifies that input longer than one
// full batch produces a final remainder chunk sized to the leftover lines.
func TestGitHistoryChunker_RemainderBatch(t *testing.T) {
	g := NewGitHistoryChunker(512)

	// 25 lines -> batch of 20 + remainder of 5.
	lines := make([]string, 25)
	for i := range lines {
		lines[i] = "commit feedface line " + string(rune('a'+(i%26)))
	}
	content := strings.Join(lines, "\n")

	chunks, err := g.chunk(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks (20 + 5 remainder), got %d", len(chunks))
	}

	first, second := chunks[0], chunks[1]
	if first.startLine != 1 || first.endLine != 20 {
		t.Errorf("first chunk span = %d-%d, want 1-20", first.startLine, first.endLine)
	}
	if second.startLine != 21 || second.endLine != 25 {
		t.Errorf("remainder chunk span = %d-%d, want 21-25", second.startLine, second.endLine)
	}
	if !strings.Contains(second.content, "[Git History] Commits 21-25") {
		t.Errorf("expected remainder header 'Commits 21-25', got %q", second.content)
	}
}

// TestGitHistoryChunker_Empty verifies that empty input yields no chunks.
func TestGitHistoryChunker_Empty(t *testing.T) {
	g := NewGitHistoryChunker(512)

	chunks, err := g.chunk("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 0 {
		t.Fatalf("expected 0 chunks for empty input, got %d", len(chunks))
	}
}
