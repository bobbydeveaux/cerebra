package scanner

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/bobbydeveaux/cerebra/internal/config"
)

// writeFile creates path with content, creating parent dirs as needed.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestIsBinaryFile(t *testing.T) {
	dir := t.TempDir()

	textPath := filepath.Join(dir, "text.txt")
	writeFile(t, textPath, "this is a plain text file\nwith multiple lines\n")

	binPath := filepath.Join(dir, "bin.dat")
	writeFile(t, binPath, "preamble\x00binary payload")

	emptyPath := filepath.Join(dir, "empty.txt")
	writeFile(t, emptyPath, "")

	tests := []struct {
		name string
		path string
		want bool
	}{
		{"plain text", textPath, false},
		{"binary with null byte", binPath, true},
		{"empty file", emptyPath, true},
		{"missing file", filepath.Join(dir, "nope.bin"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isBinaryFile(tt.path); got != tt.want {
				t.Errorf("isBinaryFile(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestFindGitRepo(t *testing.T) {
	root := t.TempDir()

	// Create a fake repo layout:
	// root/
	//   repo/
	//     .git/
	//     src/
	//       main.go
	repoDir := filepath.Join(root, "repo")
	if err := os.MkdirAll(filepath.Join(repoDir, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	srcFile := filepath.Join(repoDir, "src", "main.go")
	writeFile(t, srcFile, "package main\n")

	// And a non-repo file under root:
	loosePath := filepath.Join(root, "loose.txt")
	writeFile(t, loosePath, "loose")

	t.Run("inside repo", func(t *testing.T) {
		name, rootPath := findGitRepo(srcFile)
		if name != "repo" {
			t.Errorf("repo name = %q, want %q", name, "repo")
		}
		if rootPath != repoDir {
			t.Errorf("repo root = %q, want %q", rootPath, repoDir)
		}
	})

	t.Run("outside repo", func(t *testing.T) {
		name, rootPath := findGitRepo(loosePath)
		// May walk up to the system root and find nothing, or find a parent
		// .git. The contract is: if it returns empty, both must be empty.
		if (name == "") != (rootPath == "") {
			t.Errorf("partial result: name=%q root=%q", name, rootPath)
		}
	})
}

func newTestScanner(t *testing.T) *Scanner {
	t.Helper()
	cfg := &config.Config{
		Ignore: []string{".git", "node_modules", "*.exe", "vendor"},
	}
	return New(cfg)
}

func TestScanner_Scan_BasicTree(t *testing.T) {
	root := t.TempDir()

	// Populate a small tree.
	writeFile(t, filepath.Join(root, "main.go"), "package main\nfunc main(){}\n")
	writeFile(t, filepath.Join(root, "README.md"), "# project\n")
	writeFile(t, filepath.Join(root, "internal", "pkg", "lib.go"), "package pkg\n")

	// Files that should be skipped.
	writeFile(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/main")
	writeFile(t, filepath.Join(root, "node_modules", "x", "index.js"), "module.exports = {}")
	writeFile(t, filepath.Join(root, "app.exe"), "should be ignored")
	writeFile(t, filepath.Join(root, "vendor", "v.go"), "package vendor\n")
	writeFile(t, filepath.Join(root, "binary.dat"), "preamble\x00null byte makes this binary")

	scanner := newTestScanner(t)
	docs, errs := scanner.Scan(context.Background(), root)

	collected := drainScan(t, docs, errs)

	// Build set of relative paths returned.
	relPaths := make([]string, 0, len(collected))
	for _, d := range collected {
		relPaths = append(relPaths, filepath.ToSlash(d.RelPath))
	}
	sort.Strings(relPaths)

	want := []string{
		"README.md",
		"internal/pkg/lib.go",
		"main.go",
	}
	if !equalStringSlices(relPaths, want) {
		t.Fatalf("Scan returned %v, want %v", relPaths, want)
	}

	// Check that document fields are populated for one entry.
	var main Document
	for _, d := range collected {
		if filepath.ToSlash(d.RelPath) == "main.go" {
			main = d
			break
		}
	}
	if main.ID == "" {
		t.Errorf("document ID empty")
	}
	if main.ContentHash == "" {
		t.Errorf("content hash empty")
	}
	if main.Language != "go" {
		t.Errorf("language = %q, want %q", main.Language, "go")
	}
	if main.FileType != FileTypeCode {
		t.Errorf("file type = %q, want %q", main.FileType, FileTypeCode)
	}
	if main.SourceType != SourceTypeFilesystem {
		t.Errorf("source type = %q, want %q", main.SourceType, SourceTypeFilesystem)
	}
	if !strings.Contains(main.Content, "package main") {
		t.Errorf("content missing expected token: %q", main.Content)
	}
	if main.ModTime.IsZero() {
		t.Errorf("ModTime not populated")
	}
	if main.Metadata == nil {
		t.Errorf("Metadata not initialised")
	}
}

func TestScanner_Scan_EmptyDirectory(t *testing.T) {
	root := t.TempDir()
	scanner := newTestScanner(t)
	docs, errs := scanner.Scan(context.Background(), root)
	collected := drainScan(t, docs, errs)
	if len(collected) != 0 {
		t.Fatalf("expected 0 documents from empty dir, got %d", len(collected))
	}
}

func TestScanner_Scan_NonExistentRoot(t *testing.T) {
	scanner := newTestScanner(t)
	root := filepath.Join(t.TempDir(), "does-not-exist")
	docs, errs := scanner.Scan(context.Background(), root)

	// Collect both channels; we expect at least one error or zero docs.
	var (
		gotDocs []Document
		gotErr  bool
	)
	docsClosed, errsClosed := false, false
	for !docsClosed || !errsClosed {
		select {
		case d, ok := <-docs:
			if !ok {
				docsClosed = true
				docs = nil
				continue
			}
			gotDocs = append(gotDocs, d)
		case e, ok := <-errs:
			if !ok {
				errsClosed = true
				errs = nil
				continue
			}
			if e != nil {
				gotErr = true
			}
		}
	}

	if len(gotDocs) != 0 {
		t.Errorf("expected 0 docs for non-existent root, got %d", len(gotDocs))
	}
	if !gotErr {
		t.Errorf("expected at least one error for non-existent root, got none")
	}
}

func TestScanner_Scan_ContextCancelled(t *testing.T) {
	root := t.TempDir()
	// Create a number of files so cancellation has a chance to bite.
	for i := 0; i < 20; i++ {
		writeFile(t, filepath.Join(root, "f", "file"+itoa(i)+".go"), "package f\n")
	}

	scanner := newTestScanner(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before scan begins
	docs, errs := scanner.Scan(ctx, root)

	// Drain channels — should close cleanly even when cancelled.
	for range docs {
	}
	for range errs {
	}
	// Reaching here without deadlock is the assertion.
}

func TestScanner_ScanFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "pkg", "thing.go")
	writeFile(t, path, "package pkg\n// hello\n")

	scanner := newTestScanner(t)
	// First run Scan briefly to set s.root; alternatively call Scan with cancelled ctx.
	scanner.root = root

	doc, err := scanner.ScanFile(path)
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}
	if doc.RelPath != filepath.Join("pkg", "thing.go") {
		t.Errorf("RelPath = %q", doc.RelPath)
	}
	if doc.Language != "go" {
		t.Errorf("Language = %q, want %q", doc.Language, "go")
	}
	if doc.ContentHash == "" {
		t.Errorf("ContentHash empty")
	}
}

func TestScanner_ScanFile_Missing(t *testing.T) {
	scanner := newTestScanner(t)
	scanner.root = t.TempDir()

	_, err := scanner.ScanFile(filepath.Join(scanner.root, "missing.go"))
	if err == nil {
		t.Fatalf("expected error for missing file, got nil")
	}
}

// --- helpers ---

func drainScan(t *testing.T, docs <-chan Document, errs <-chan error) []Document {
	t.Helper()
	var (
		collected []Document
		gotErrs   []error
	)
	docsClosed, errsClosed := false, false
	for !docsClosed || !errsClosed {
		select {
		case d, ok := <-docs:
			if !ok {
				docsClosed = true
				docs = nil
				continue
			}
			collected = append(collected, d)
		case e, ok := <-errs:
			if !ok {
				errsClosed = true
				errs = nil
				continue
			}
			if e != nil {
				gotErrs = append(gotErrs, e)
			}
		}
	}
	if len(gotErrs) > 0 {
		t.Logf("scan emitted %d non-fatal errors: %v", len(gotErrs), gotErrs)
	}
	return collected
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}
