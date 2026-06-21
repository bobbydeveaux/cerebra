package chunker

import (
	"strings"
	"testing"
)

// TestMarkdownChunker_HeadingBoundarySplit verifies that a document with
// multiple headings is split into one chunk per heading section.
func TestMarkdownChunker_HeadingBoundarySplit(t *testing.T) {
	m := NewMarkdownChunker(512)

	content := "# Introduction\n\n" +
		"Intro body text that comfortably clears the minimum length filter.\n\n" +
		"## Installation\n\n" +
		"Run the installer and follow the prompts on screen carefully.\n\n" +
		"## Usage\n\n" +
		"Invoke the binary with the subcommand you need for the task.\n"

	chunks, err := m.chunk(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Three headings, no preamble before the first heading -> three chunks.
	if len(chunks) != 3 {
		t.Fatalf("expected 3 heading-section chunks, got %d", len(chunks))
	}

	wantHeads := []string{"# Introduction", "## Installation", "## Usage"}
	for i, want := range wantHeads {
		if !strings.HasPrefix(strings.TrimSpace(chunks[i].content), want) {
			t.Errorf("chunk %d: expected to start with %q, got %q", i, want, chunks[i].content)
		}
	}
	// Each section must own a contiguous, increasing line span.
	if chunks[0].startLine != 1 {
		t.Errorf("first section startLine = %d, want 1", chunks[0].startLine)
	}
	if chunks[2].startLine <= chunks[1].startLine {
		t.Errorf("section start lines not increasing: %d then %d", chunks[1].startLine, chunks[2].startLine)
	}
}

// TestMarkdownChunker_LargeSection verifies that a single oversized heading
// section is still returned as one chunk preserving its full body.
func TestMarkdownChunker_LargeSection(t *testing.T) {
	m := NewMarkdownChunker(512)

	var b strings.Builder
	b.WriteString("# Big Section\n\n")
	for i := 0; i < 200; i++ {
		b.WriteString("Line of body content number ")
		b.WriteString(string(rune('a' + (i % 26))))
		b.WriteString(" with enough words to be substantial.\n")
	}
	content := b.String()

	chunks, err := m.chunk(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk for a single large heading section, got %d", len(chunks))
	}
	if !strings.Contains(chunks[0].content, "# Big Section") {
		t.Errorf("expected chunk to retain the heading, got %q", chunks[0].content[:40])
	}
	if !strings.Contains(chunks[0].content, "number z") {
		t.Error("expected oversized section body to be preserved in full")
	}
}

// TestMarkdownChunker_Empty verifies empty input yields no chunks.
func TestMarkdownChunker_Empty(t *testing.T) {
	m := NewMarkdownChunker(512)

	chunks, err := m.chunk("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 0 {
		t.Fatalf("expected 0 chunks for empty input, got %d", len(chunks))
	}
}
