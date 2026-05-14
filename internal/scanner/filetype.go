package scanner

import (
	"path/filepath"
	"strings"
)

var languageExtensions = map[string]string{
	".go":     "go",
	".py":     "python",
	".js":     "javascript",
	".ts":     "typescript",
	".tsx":    "typescript",
	".jsx":    "javascript",
	".rs":     "rust",
	".java":   "java",
	".rb":     "ruby",
	".php":    "php",
	".c":      "c",
	".cpp":    "cpp",
	".h":      "c",
	".hpp":    "cpp",
	".cs":     "csharp",
	".swift":  "swift",
	".kt":     "kotlin",
	".scala":  "scala",
	".lua":    "lua",
	".r":      "r",
	".pl":     "perl",
	".sh":     "shell",
	".bash":   "shell",
	".zsh":    "shell",
	".fish":   "shell",
	".ps1":    "powershell",
	".sql":    "sql",
	".html":   "html",
	".htm":    "html",
	".css":    "css",
	".scss":   "scss",
	".less":   "less",
	".vue":    "vue",
	".svelte": "svelte",
	".dart":   "dart",
	".ex":     "elixir",
	".exs":    "elixir",
	".erl":    "erlang",
	".hs":     "haskell",
	".ml":     "ocaml",
	".clj":    "clojure",
	".tf":     "terraform",
	".proto":  "protobuf",
	".graphql":"graphql",
	".gql":    "graphql",
}

var configExtensions = map[string]bool{
	".yaml": true, ".yml": true, ".toml": true, ".json": true,
	".env": true, ".ini": true, ".cfg": true, ".conf": true,
	".properties": true,
}

var configFilenames = map[string]bool{
	"Dockerfile": true, "Makefile": true, "Rakefile": true,
	"Gemfile": true, "Procfile": true, "Vagrantfile": true,
	".gitignore": true, ".dockerignore": true, ".editorconfig": true,
	".eslintrc": true, ".prettierrc": true, "tsconfig.json": true,
	"package.json": true, "go.mod": true, "go.sum": true,
	"Cargo.toml": true, "Cargo.lock": true, "requirements.txt": true,
	"pyproject.toml": true, "setup.py": true, "setup.cfg": true,
	"pom.xml": true, "build.gradle": true, "build.sbt": true,
	"CMakeLists.txt": true, "docker-compose.yml": true,
	"docker-compose.yaml": true, ".github": true,
	"Jenkinsfile": true, ".travis.yml": true, ".circleci": true,
}

func DetectLanguage(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if lang, ok := languageExtensions[ext]; ok {
		return lang
	}

	base := filepath.Base(path)
	switch base {
	case "Dockerfile":
		return "dockerfile"
	case "Makefile", "GNUmakefile":
		return "makefile"
	case "Jenkinsfile":
		return "groovy"
	}

	return ""
}

func DetectFileType(path string, language string) FileType {
	ext := strings.ToLower(filepath.Ext(path))
	base := filepath.Base(path)

	if ext == ".md" || ext == ".mdx" || ext == ".rst" || ext == ".txt" || ext == ".adoc" {
		return FileTypeMarkdown
	}

	if configExtensions[ext] || configFilenames[base] {
		return FileTypeConfig
	}

	if language != "" {
		return FileTypeCode
	}

	return FileTypeData
}

func DetectCategory(relPath string, language string, ft FileType) Category {
	lower := strings.ToLower(relPath)
	parts := strings.Split(lower, string(filepath.Separator))

	for _, part := range parts {
		switch {
		case part == "api" || part == "apis" || part == "handlers" || part == "routes" || part == "controllers":
			return CategoryAPI
		case part == "infra" || part == "infrastructure" || part == "terraform" || part == "deploy" || part == "k8s" || part == "kubernetes" || part == "helm":
			return CategoryInfra
		case part == "frontend" || part == "ui" || part == "web" || part == "client" || part == "components" || part == "pages":
			return CategoryFrontend
		case part == "test" || part == "tests" || part == "testing" || part == "spec" || part == "specs" || part == "__tests__":
			return CategoryTesting
		case part == "docs" || part == "documentation" || part == "doc":
			return CategoryDocs
		case part == "config" || part == "configs" || part == "configuration":
			return CategoryConfig
		case part == "migrations" || part == "migrate" || part == "schema":
			return CategoryMigrations
		}
	}

	if strings.HasSuffix(lower, "_test.go") || strings.HasSuffix(lower, ".test.js") ||
		strings.HasSuffix(lower, ".test.ts") || strings.HasSuffix(lower, ".spec.js") ||
		strings.HasSuffix(lower, ".spec.ts") || strings.HasSuffix(lower, "_spec.rb") {
		return CategoryTesting
	}

	if ft == FileTypeMarkdown {
		return CategoryDocs
	}
	if ft == FileTypeConfig {
		return CategoryConfig
	}

	return CategoryUnknown
}
