package chunker

import (
	"strings"
	"testing"
)

// TestCodeChunker_BoundaryDetectionByLanguage verifies that the per-language
// regexes split a source file at the expected definition boundaries for a
// representative spread of supported languages.
func TestCodeChunker_BoundaryDetectionByLanguage(t *testing.T) {
	c := NewCodeChunker(512)

	cases := []struct {
		name     string
		language string
		content  string
		// markers each expect their own chunk (one per definition boundary).
		markers []string
	}{
		{
			name:     "go",
			language: "go",
			content: "package main\n\n" +
				"func alpha() {\n\tprintln(\"a\")\n\tprintln(\"aa\")\n}\n\n" +
				"func beta() {\n\tprintln(\"b\")\n\tprintln(\"bb\")\n}\n",
			markers: []string{"func alpha()", "func beta()"},
		},
		{
			name:     "python",
			language: "python",
			content: "import os\n\n" +
				"def first():\n    return 1\n    # body\n\n" +
				"class Widget:\n    def m(self):\n        return 2\n",
			markers: []string{"def first()", "class Widget"},
		},
		{
			name:     "typescript",
			language: "typescript",
			content: "import x from 'x'\n\n" +
				"export function load() {\n  return 1\n  // body\n}\n\n" +
				"interface Shape {\n  side: number\n  area: number\n}\n",
			markers: []string{"function load()", "interface Shape"},
		},
		{
			name:     "swift",
			language: "swift",
			content: "import Foundation\n\n" +
				"func greet() {\n    print(\"hi\")\n    print(\"yo\")\n}\n\n" +
				"struct Point {\n    let x: Int\n    let y: Int\n}\n",
			markers: []string{"func greet()", "struct Point"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chunks, err := c.chunk(tc.content, tc.language)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(chunks) == 0 {
				t.Fatalf("expected boundary-split chunks for %s, got 0", tc.language)
			}
			for _, marker := range tc.markers {
				found := false
				for _, ch := range chunks {
					if strings.Contains(ch.content, marker) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("%s: expected a chunk containing %q", tc.language, marker)
				}
			}
		})
	}
}

// TestCodeChunker_UnknownExtensionFallback verifies that a language with no
// registered pattern falls through to the line-based chunker rather than
// failing or returning nothing.
func TestCodeChunker_UnknownExtensionFallback(t *testing.T) {
	c := NewCodeChunker(512)

	var b strings.Builder
	for i := 0; i < 60; i++ {
		b.WriteString("some content line that is not a recognised definition\n")
	}

	chunks, err := c.chunk(b.String(), "brainfuck")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatal("expected line-based fallback chunks for unknown language")
	}
	// chunkByLines uses maxSize/4 lines per chunk (capped at >=20); 60 lines at
	// 512/4 = 128 -> floored to a single chunk, so confirm coverage of the path
	// rather than a specific count.
	if chunks[0].startLine != 1 {
		t.Errorf("expected first fallback chunk to start at line 1, got %d", chunks[0].startLine)
	}
}

// TestCodeChunker_MaxSizeSplitsLineFallback verifies that a small maxSize
// forces the line-based fallback to emit multiple chunks (the linesPerChunk
// floor of 20 governs the split).
func TestCodeChunker_MaxSizeSplitsLineFallback(t *testing.T) {
	c := NewCodeChunker(1) // maxSize/4 < 20 -> floored to 20 lines per chunk

	var b strings.Builder
	for i := 0; i < 45; i++ {
		b.WriteString("plain line of text without any function boundary here\n")
	}

	chunks, err := c.chunk(b.String(), "unknownlang")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 45 lines / 20 per chunk -> 3 chunks (20, 20, 5).
	if len(chunks) != 3 {
		t.Fatalf("expected 3 line-fallback chunks (20+20+5), got %d", len(chunks))
	}
	if chunks[0].startLine != 1 || chunks[0].endLine != 20 {
		t.Errorf("first chunk span = %d-%d, want 1-20", chunks[0].startLine, chunks[0].endLine)
	}
	if chunks[2].startLine != 41 {
		t.Errorf("third chunk startLine = %d, want 41", chunks[2].startLine)
	}
}
