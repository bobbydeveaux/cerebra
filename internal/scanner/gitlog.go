package scanner

import (
	"fmt"
	"os/exec"
	"strings"
)

type GitInfo struct {
	RemoteURL     string
	LastCommitSHA string
	Log           string
}

func ExtractGitInfo(repoRoot string, maxCommits int) (*GitInfo, error) {
	info := &GitInfo{}

	if out, err := runGit(repoRoot, "remote", "get-url", "origin"); err == nil {
		info.RemoteURL = strings.TrimSpace(out)
	}

	if out, err := runGit(repoRoot, "rev-parse", "HEAD"); err == nil {
		info.LastCommitSHA = strings.TrimSpace(out)
	}

	if maxCommits <= 0 {
		maxCommits = 200
	}
	if out, err := runGit(repoRoot, "log",
		fmt.Sprintf("--max-count=%d", maxCommits),
		"--format=%H %ai %an: %s",
	); err == nil {
		info.Log = strings.TrimSpace(out)
	}

	return info, nil
}

func GetChangedFiles(repoRoot, lastSHA string) (changed []string, deleted []string, err error) {
	out, err := runGit(repoRoot, "diff", "--name-only", lastSHA+"..HEAD")
	if err != nil {
		return nil, nil, fmt.Errorf("git diff: %w", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line != "" {
			changed = append(changed, line)
		}
	}

	out, err = runGit(repoRoot, "diff", "--name-only", "--diff-filter=D", lastSHA+"..HEAD")
	if err != nil {
		return changed, nil, nil
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line != "" {
			deleted = append(deleted, line)
		}
	}

	return changed, deleted, nil
}

func runGit(repoRoot string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", repoRoot}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
