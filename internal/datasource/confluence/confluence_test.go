package confluence

import (
	"testing"
)

// Scaffold — populated in subsequent commit.
func TestNew_TrimsTrailingSlash(t *testing.T) {
	p := New("https://example.atlassian.net/wiki/", "u@example.com", "tok", nil)
	if p == nil {
		t.Fatal("New returned nil")
	}
	if p.baseURL != "https://example.atlassian.net/wiki" {
		t.Errorf("baseURL = %q, want trimmed", p.baseURL)
	}
}
