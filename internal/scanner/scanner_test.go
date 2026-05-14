package scanner

import (
	"testing"
)

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"main.go", "go"},
		{"app.py", "python"},
		{"index.js", "javascript"},
		{"app.ts", "typescript"},
		{"lib.rs", "rust"},
		{"App.java", "java"},
		{"Dockerfile", "dockerfile"},
		{"Makefile", "makefile"},
		{"unknown.xyz", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := DetectLanguage(tt.path)
			if got != tt.want {
				t.Errorf("DetectLanguage(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestDetectFileType(t *testing.T) {
	tests := []struct {
		path string
		lang string
		want FileType
	}{
		{"README.md", "", FileTypeMarkdown},
		{"main.go", "go", FileTypeCode},
		{"config.yaml", "", FileTypeConfig},
		{"Dockerfile", "dockerfile", FileTypeConfig},
		{"data.bin", "", FileTypeData},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := DetectFileType(tt.path, tt.lang)
			if got != tt.want {
				t.Errorf("DetectFileType(%q, %q) = %q, want %q", tt.path, tt.lang, got, tt.want)
			}
		})
	}
}

func TestDetectCategory(t *testing.T) {
	tests := []struct {
		path     string
		language string
		ft       FileType
		want     Category
	}{
		{"api/handler.go", "go", FileTypeCode, CategoryAPI},
		{"terraform/main.tf", "terraform", FileTypeCode, CategoryInfra},
		{"src/components/Button.tsx", "typescript", FileTypeCode, CategoryFrontend},
		{"tests/unit_test.go", "go", FileTypeCode, CategoryTesting},
		{"docs/README.md", "", FileTypeMarkdown, CategoryDocs},
		{"cmd/main.go", "go", FileTypeCode, CategoryUnknown},
		{"handler_test.go", "go", FileTypeCode, CategoryTesting},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := DetectCategory(tt.path, tt.language, tt.ft)
			if got != tt.want {
				t.Errorf("DetectCategory(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestIgnorer(t *testing.T) {
	ig := NewIgnorer([]string{".git", "node_modules", "*.exe", "vendor"})

	tests := []struct {
		path   string
		isDir  bool
		ignore bool
	}{
		{".git", true, true},
		{"node_modules", true, true},
		{"vendor", true, true},
		{"src/main.go", false, false},
		{"app.exe", false, true},
		{"src/app.go", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := ig.ShouldIgnore(tt.path, tt.isDir)
			if got != tt.ignore {
				t.Errorf("ShouldIgnore(%q, %v) = %v, want %v", tt.path, tt.isDir, got, tt.ignore)
			}
		})
	}
}
