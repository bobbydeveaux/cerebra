package chunker

import (
	"strings"
	"testing"

	"github.com/bobbydeveaux/cerebra/internal/scanner"
)

func TestDispatcher_ChunkCode(t *testing.T) {
	d := NewDispatcher(512)

	doc := scanner.Document{
		ID:       "test-doc-1",
		RelPath:  "main.go",
		FileType: scanner.FileTypeCode,
		Language: "go",
		Content: `package main

import "fmt"

func hello() {
	fmt.Println("hello")
}

func world() {
	fmt.Println("world")
}
`,
	}

	chunks, err := d.Chunk(doc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(chunks) == 0 {
		t.Fatal("expected at least one chunk")
	}

	// Should have preamble + hello + world
	foundHello := false
	foundWorld := false
	for _, c := range chunks {
		if strings.Contains(c.Content, "func hello()") {
			foundHello = true
		}
		if strings.Contains(c.Content, "func world()") {
			foundWorld = true
		}
	}

	if !foundHello {
		t.Error("expected chunk containing func hello()")
	}
	if !foundWorld {
		t.Error("expected chunk containing func world()")
	}

	// Check metadata
	for _, c := range chunks {
		if c.DocumentID != "test-doc-1" {
			t.Errorf("expected DocumentID test-doc-1, got %s", c.DocumentID)
		}
		if c.Metadata.Path != "main.go" {
			t.Errorf("expected path main.go, got %s", c.Metadata.Path)
		}
		if c.Metadata.Language != "go" {
			t.Errorf("expected language go, got %s", c.Metadata.Language)
		}
	}
}

func TestDispatcher_ChunkMarkdown(t *testing.T) {
	d := NewDispatcher(512)

	doc := scanner.Document{
		ID:       "test-doc-2",
		RelPath:  "README.md",
		FileType: scanner.FileTypeMarkdown,
		Content: `# Title

This is the intro paragraph.

## Section One

Content of section one.

## Section Two

Content of section two.
`,
	}

	chunks, err := d.Chunk(doc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d", len(chunks))
	}

	foundS1 := false
	foundS2 := false
	for _, c := range chunks {
		if strings.Contains(c.Content, "Section One") {
			foundS1 = true
		}
		if strings.Contains(c.Content, "Section Two") {
			foundS2 = true
		}
	}

	if !foundS1 {
		t.Error("expected chunk containing Section One")
	}
	if !foundS2 {
		t.Error("expected chunk containing Section Two")
	}
}

func TestDispatcher_ChunkConfig(t *testing.T) {
	d := NewDispatcher(512)

	doc := scanner.Document{
		ID:       "test-doc-3",
		RelPath:  "config.yaml",
		FileType: scanner.FileTypeConfig,
		Content:  "key: value\nother: stuff\n",
	}

	chunks, err := d.Chunk(doc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk for config, got %d", len(chunks))
	}

	if chunks[0].Content != "key: value\nother: stuff\n" {
		t.Errorf("expected full config content, got %q", chunks[0].Content)
	}
}

func TestDispatcher_EmptyContent(t *testing.T) {
	d := NewDispatcher(512)

	doc := scanner.Document{
		ID:       "test-doc-4",
		RelPath:  "empty.go",
		FileType: scanner.FileTypeCode,
		Language: "go",
		Content:  "",
	}

	chunks, err := d.Chunk(doc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(chunks) != 0 {
		t.Fatalf("expected 0 chunks for empty content, got %d", len(chunks))
	}
}
