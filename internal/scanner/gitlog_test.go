package scanner

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitOrSkip skips the calling test if the git binary is not on PATH. The
// gitlog functions shell out to git directly, so without a binary the
// tests cannot meaningfully exercise them.
func gitOrSkip(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not on PATH — skipping git-backed test")
	}
}

// initRepo creates a minimal git repo at dir with one commit and an
// origin remote URL stub. Returns the commit SHA.
func initRepo(t *testing.T, dir, remoteURL string) string {
	t.Helper()

	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		// Avoid the user's global git config leaking author/identity rules
		// (e.g. signing) into the fixture repo.
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=gopher-test",
			"GIT_AUTHOR_EMAIL=gopher@example.invalid",
			"GIT_COMMITTER_NAME=gopher-test",
			"GIT_COMMITTER_EMAIL=gopher@example.invalid",
			"GIT_CONFIG_GLOBAL=/dev/null",
			"GIT_CONFIG_NOSYSTEM=1",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}

	run("init", "-b", "main")
	run("config", "commit.gpgsign", "false")
	run("config", "user.name", "gopher-test")
	run("config", "user.email", "gopher@example.invalid")
	if remoteURL != "" {
		run("remote", "add", "origin", remoteURL)
	}

	readme := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readme, []byte("# initial\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	run("add", "README.md")
	run("-c", "commit.gpgsign=false", "commit", "-m", "initial commit")

	return run("rev-parse", "HEAD")
}

func TestExtractGitInfo(t *testing.T) {
	gitOrSkip(t)

	dir := t.TempDir()
	wantRemote := "https://git.example.invalid/test/repo.git"
	wantSHA := initRepo(t, dir, wantRemote)

	info, err := ExtractGitInfo(dir, 10)
	if err != nil {
		t.Fatalf("ExtractGitInfo: %v", err)
	}
	if info.RemoteURL != wantRemote {
		t.Errorf("RemoteURL = %q, want %q", info.RemoteURL, wantRemote)
	}
	if info.LastCommitSHA != wantSHA {
		t.Errorf("LastCommitSHA = %q, want %q", info.LastCommitSHA, wantSHA)
	}
	if !strings.Contains(info.Log, "initial commit") {
		t.Errorf("Log missing initial commit subject; got %q", info.Log)
	}
}

func TestExtractGitInfo_NoRemote(t *testing.T) {
	gitOrSkip(t)

	dir := t.TempDir()
	initRepo(t, dir, "") // no remote

	info, err := ExtractGitInfo(dir, 5)
	if err != nil {
		t.Fatalf("ExtractGitInfo: %v", err)
	}
	if info.RemoteURL != "" {
		t.Errorf("RemoteURL = %q, want empty", info.RemoteURL)
	}
	if info.LastCommitSHA == "" {
		t.Errorf("LastCommitSHA empty even though repo has a commit")
	}
}

func TestExtractGitInfo_DefaultMaxCommits(t *testing.T) {
	gitOrSkip(t)

	dir := t.TempDir()
	initRepo(t, dir, "")

	info, err := ExtractGitInfo(dir, 0) // 0 should default to 200
	if err != nil {
		t.Fatalf("ExtractGitInfo: %v", err)
	}
	if info.Log == "" {
		t.Errorf("Log empty for repo with at least one commit")
	}
}

func TestExtractGitInfo_NotARepo(t *testing.T) {
	gitOrSkip(t)

	dir := t.TempDir() // empty dir, no git init
	info, err := ExtractGitInfo(dir, 5)
	if err != nil {
		t.Fatalf("ExtractGitInfo on non-repo returned error: %v", err)
	}
	// All git invocations fail silently — fields stay empty.
	if info.RemoteURL != "" || info.LastCommitSHA != "" || info.Log != "" {
		t.Errorf("expected empty info for non-repo, got %+v", info)
	}
}

func TestGetChangedFiles(t *testing.T) {
	gitOrSkip(t)

	dir := t.TempDir()
	firstSHA := initRepo(t, dir, "")

	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=gopher-test",
			"GIT_AUTHOR_EMAIL=gopher@example.invalid",
			"GIT_COMMITTER_NAME=gopher-test",
			"GIT_COMMITTER_EMAIL=gopher@example.invalid",
			"GIT_CONFIG_GLOBAL=/dev/null",
			"GIT_CONFIG_NOSYSTEM=1",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	// Add a new file.
	if err := os.WriteFile(filepath.Join(dir, "added.txt"), []byte("a"), 0o644); err != nil {
		t.Fatalf("write added.txt: %v", err)
	}
	run("add", "added.txt")
	run("-c", "commit.gpgsign=false", "commit", "-m", "add file")

	// Delete README.
	if err := os.Remove(filepath.Join(dir, "README.md")); err != nil {
		t.Fatalf("remove README.md: %v", err)
	}
	run("add", "-A")
	run("-c", "commit.gpgsign=false", "commit", "-m", "delete README")

	changed, deleted, err := GetChangedFiles(dir, firstSHA)
	if err != nil {
		t.Fatalf("GetChangedFiles: %v", err)
	}

	if !containsString(changed, "added.txt") {
		t.Errorf("changed missing added.txt; got %v", changed)
	}
	if !containsString(deleted, "README.md") {
		t.Errorf("deleted missing README.md; got %v", deleted)
	}
}

func TestGetChangedFiles_NoChanges(t *testing.T) {
	gitOrSkip(t)

	dir := t.TempDir()
	sha := initRepo(t, dir, "")

	changed, deleted, err := GetChangedFiles(dir, sha)
	if err != nil {
		t.Fatalf("GetChangedFiles: %v", err)
	}
	if len(changed) != 0 {
		t.Errorf("expected no changed, got %v", changed)
	}
	if len(deleted) != 0 {
		t.Errorf("expected no deleted, got %v", deleted)
	}
}

func TestGetChangedFiles_InvalidSHA(t *testing.T) {
	gitOrSkip(t)

	dir := t.TempDir()
	initRepo(t, dir, "")

	_, _, err := GetChangedFiles(dir, "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	if err == nil {
		t.Fatalf("expected error for invalid SHA, got nil")
	}
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
