package scanner

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestFileContentHash(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		name    string
		content string
	}{
		{"empty", ""},
		{"ascii", "hello world"},
		{"multiline", "line one\nline two\nline three\n"},
		{"binary-ish", "\x00\x01\x02\x03 some text"},
		{"unicode", "café résumé 日本語"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(dir, tt.name+".txt")
			if err := os.WriteFile(path, []byte(tt.content), 0o644); err != nil {
				t.Fatalf("write fixture: %v", err)
			}

			got, err := FileContentHash(path)
			if err != nil {
				t.Fatalf("FileContentHash returned error: %v", err)
			}

			want := fmt.Sprintf("%x", sha256.Sum256([]byte(tt.content)))
			if got != want {
				t.Errorf("FileContentHash(%q) = %q, want %q", path, got, want)
			}
		})
	}
}

func TestFileContentHash_MissingFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist.txt")
	if _, err := FileContentHash(missing); err == nil {
		t.Fatalf("FileContentHash on missing file: expected error, got nil")
	}
}

func TestFileContentHash_Stable(t *testing.T) {
	// Same content must always hash to the same value across reads.
	dir := t.TempDir()
	path := filepath.Join(dir, "stable.txt")
	if err := os.WriteFile(path, []byte("stable content"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	first, err := FileContentHash(path)
	if err != nil {
		t.Fatalf("first hash: %v", err)
	}
	second, err := FileContentHash(path)
	if err != nil {
		t.Fatalf("second hash: %v", err)
	}
	if first != second {
		t.Errorf("hash not stable: first=%q second=%q", first, second)
	}
}
