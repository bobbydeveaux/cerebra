package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

// resetViper clears the package-global viper state between subtests so they
// do not leak settings into one another. Load() reads from this singleton.
func resetViper() {
	viper.Reset()
}

func TestLoadDefaults(t *testing.T) {
	resetViper()

	// Ensure none of the env vars pre-populate fields under test.
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("MINIMAX_API_KEY", "")
	t.Setenv("MEALFIT_MINIMAX", "")
	t.Setenv("TT_RES_CONFLUENCE", "")
	t.Setenv("CONFLUENCE_API_TOKEN", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("Load() returned nil config")
	}

	if cfg.Embedder != "ollama" {
		t.Errorf("Embedder default = %q, want %q", cfg.Embedder, "ollama")
	}
	if cfg.ChatLLM != "ollama" {
		t.Errorf("ChatLLM default = %q, want %q", cfg.ChatLLM, "ollama")
	}
	if cfg.Ollama.URL != "http://localhost:11434" {
		t.Errorf("Ollama.URL default = %q", cfg.Ollama.URL)
	}
	if cfg.Ollama.EmbedModel != "nomic-embed-text" {
		t.Errorf("Ollama.EmbedModel default = %q", cfg.Ollama.EmbedModel)
	}
	if cfg.Ollama.ChatModel != "llama3.2" {
		t.Errorf("Ollama.ChatModel default = %q", cfg.Ollama.ChatModel)
	}
	if cfg.OpenAI.EmbedModel != "text-embedding-3-small" {
		t.Errorf("OpenAI.EmbedModel default = %q", cfg.OpenAI.EmbedModel)
	}
	if cfg.OpenAI.ChatModel != "gpt-4o" {
		t.Errorf("OpenAI.ChatModel default = %q", cfg.OpenAI.ChatModel)
	}
	if cfg.Claude.Model != "claude-sonnet-4-6" {
		t.Errorf("Claude.Model default = %q", cfg.Claude.Model)
	}
	if cfg.MiniMax.Model != "MiniMax-M2.7-highspeed" {
		t.Errorf("MiniMax.Model default = %q", cfg.MiniMax.Model)
	}

	if cfg.ChunkSize != 512 {
		t.Errorf("ChunkSize default = %d, want 512", cfg.ChunkSize)
	}
	if cfg.ChunkOverlap != 64 {
		t.Errorf("ChunkOverlap default = %d, want 64", cfg.ChunkOverlap)
	}
	if cfg.DBPath != ".cerebra/jor-el.db" {
		t.Errorf("DBPath default = %q", cfg.DBPath)
	}
	if cfg.DocsPath != ".cerebra/docs/" {
		t.Errorf("DocsPath default = %q", cfg.DocsPath)
	}
	if cfg.UIPort != 8080 {
		t.Errorf("UIPort default = %d, want 8080", cfg.UIPort)
	}
	if cfg.UIBind != "127.0.0.1" {
		t.Errorf("UIBind default = %q", cfg.UIBind)
	}
	if cfg.EmbedWorkers != 2 {
		t.Errorf("EmbedWorkers default = %d, want 2", cfg.EmbedWorkers)
	}
	if cfg.EmbedBatchSize != 32 {
		t.Errorf("EmbedBatchSize default = %d, want 32", cfg.EmbedBatchSize)
	}

	// Ignore list — verify presence of a known entry rather than the full list.
	wantIgnore := map[string]bool{
		".git":         false,
		"node_modules": false,
		"vendor":       false,
		".cerebra":     false,
	}
	for _, ig := range cfg.Ignore {
		if _, ok := wantIgnore[ig]; ok {
			wantIgnore[ig] = true
		}
	}
	for k, seen := range wantIgnore {
		if !seen {
			t.Errorf("Ignore default missing %q", k)
		}
	}

	// BrainWatchPath is derived from $HOME when available. We can't force
	// UserHomeDir to fail portably, so just assert it is non-empty in the
	// happy path (CI always has a HOME).
	if home, err := os.UserHomeDir(); err == nil {
		want := filepath.Join(home, ".claude", "projects")
		if cfg.BrainWatchPath != want {
			t.Errorf("BrainWatchPath = %q, want %q", cfg.BrainWatchPath, want)
		}
	}

	// Env-isolated keys: must be empty since we cleared them above.
	if cfg.OpenAI.APIKey != "" {
		t.Errorf("OpenAI.APIKey expected empty, got %q", cfg.OpenAI.APIKey)
	}
	if cfg.Claude.APIKey != "" {
		t.Errorf("Claude.APIKey expected empty, got %q", cfg.Claude.APIKey)
	}
	if cfg.MiniMax.APIKey != "" {
		t.Errorf("MiniMax.APIKey expected empty, got %q", cfg.MiniMax.APIKey)
	}
	if cfg.Confluence.APIToken != "" {
		t.Errorf("Confluence.APIToken expected empty, got %q", cfg.Confluence.APIToken)
	}
}

func TestLoadEnvVarFallbacks(t *testing.T) {
	cases := []struct {
		name    string
		envKey  string
		value   string
		extract func(*Config) string
	}{
		{"openai", "OPENAI_API_KEY", "sk-openai-xyz", func(c *Config) string { return c.OpenAI.APIKey }},
		{"anthropic", "ANTHROPIC_API_KEY", "sk-ant-xyz", func(c *Config) string { return c.Claude.APIKey }},
		{"minimax", "MINIMAX_API_KEY", "mm-prod-xyz", func(c *Config) string { return c.MiniMax.APIKey }},
		{"mealfit_minimax", "MEALFIT_MINIMAX", "mm-mealfit-xyz", func(c *Config) string { return c.MiniMax.APIKey }},
		{"tt_res_confluence", "TT_RES_CONFLUENCE", "tt-confluence-xyz", func(c *Config) string { return c.Confluence.APIToken }},
		{"confluence_api_token", "CONFLUENCE_API_TOKEN", "atlassian-xyz", func(c *Config) string { return c.Confluence.APIToken }},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			resetViper()
			// Clear all known env vars first so a stray host value
			// can't satisfy the assertion by accident.
			t.Setenv("OPENAI_API_KEY", "")
			t.Setenv("ANTHROPIC_API_KEY", "")
			t.Setenv("MINIMAX_API_KEY", "")
			t.Setenv("MEALFIT_MINIMAX", "")
			t.Setenv("TT_RES_CONFLUENCE", "")
			t.Setenv("CONFLUENCE_API_TOKEN", "")

			t.Setenv(tc.envKey, tc.value)

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() error: %v", err)
			}

			if got := tc.extract(cfg); got != tc.value {
				t.Errorf("%s: extracted value = %q, want %q", tc.envKey, got, tc.value)
			}
		})
	}
}

func TestLoadEnvVarDoesNotOverrideExistingValue(t *testing.T) {
	// Each sub-config: prove that when viper already has a value, the env-var
	// fallback does NOT override it.
	cases := []struct {
		name     string
		viperKey string
		preset   string
		envKey   string
		envVal   string
		extract  func(*Config) string
	}{
		{"openai", "openai.api_key", "already-set-openai", "OPENAI_API_KEY", "env-openai", func(c *Config) string { return c.OpenAI.APIKey }},
		{"anthropic", "claude.api_key", "already-set-claude", "ANTHROPIC_API_KEY", "env-claude", func(c *Config) string { return c.Claude.APIKey }},
		{"minimax", "minimax.api_key", "already-set-mm", "MINIMAX_API_KEY", "env-mm", func(c *Config) string { return c.MiniMax.APIKey }},
		{"confluence", "confluence.api_token", "already-set-conf", "CONFLUENCE_API_TOKEN", "env-conf", func(c *Config) string { return c.Confluence.APIToken }},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			resetViper()
			viper.Set(tc.viperKey, tc.preset)
			t.Setenv("OPENAI_API_KEY", "")
			t.Setenv("ANTHROPIC_API_KEY", "")
			t.Setenv("MINIMAX_API_KEY", "")
			t.Setenv("MEALFIT_MINIMAX", "")
			t.Setenv("TT_RES_CONFLUENCE", "")
			t.Setenv("CONFLUENCE_API_TOKEN", "")
			t.Setenv(tc.envKey, tc.envVal)

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() error: %v", err)
			}

			if got := tc.extract(cfg); got != tc.preset {
				t.Errorf("%s: env var overrode preset; got %q want %q", tc.envKey, got, tc.preset)
			}
		})
	}
}

func TestLoadConfluenceTokenFallbackChain(t *testing.T) {
	// When both TT_RES_CONFLUENCE and CONFLUENCE_API_TOKEN are set and viper
	// has no value, the first guard (TT_RES_CONFLUENCE) wins because the
	// second guard sees an already-populated APIToken and skips.
	resetViper()
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("MINIMAX_API_KEY", "")
	t.Setenv("MEALFIT_MINIMAX", "")
	t.Setenv("TT_RES_CONFLUENCE", "tt-wins")
	t.Setenv("CONFLUENCE_API_TOKEN", "atlassian-loses")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Confluence.APIToken != "tt-wins" {
		t.Errorf("Confluence.APIToken = %q, want %q (TT_RES_CONFLUENCE should win)", cfg.Confluence.APIToken, "tt-wins")
	}

	// And when only CONFLUENCE_API_TOKEN is set, it should be picked up.
	resetViper()
	t.Setenv("TT_RES_CONFLUENCE", "")
	t.Setenv("CONFLUENCE_API_TOKEN", "atlassian-only")

	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Confluence.APIToken != "atlassian-only" {
		t.Errorf("Confluence.APIToken = %q, want %q", cfg.Confluence.APIToken, "atlassian-only")
	}
}

func TestLoadMiniMaxFallbackChain(t *testing.T) {
	// MINIMAX_API_KEY wins over MEALFIT_MINIMAX when both are set.
	resetViper()
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("MINIMAX_API_KEY", "primary")
	t.Setenv("MEALFIT_MINIMAX", "secondary")
	t.Setenv("TT_RES_CONFLUENCE", "")
	t.Setenv("CONFLUENCE_API_TOKEN", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.MiniMax.APIKey != "primary" {
		t.Errorf("MiniMax.APIKey = %q, want %q", cfg.MiniMax.APIKey, "primary")
	}

	// MEALFIT_MINIMAX wins when MINIMAX_API_KEY is unset.
	resetViper()
	t.Setenv("MINIMAX_API_KEY", "")
	t.Setenv("MEALFIT_MINIMAX", "mealfit-fallback")

	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.MiniMax.APIKey != "mealfit-fallback" {
		t.Errorf("MiniMax.APIKey = %q, want %q", cfg.MiniMax.APIKey, "mealfit-fallback")
	}
}

func TestLoadViperOverrides(t *testing.T) {
	resetViper()
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("MINIMAX_API_KEY", "")
	t.Setenv("MEALFIT_MINIMAX", "")
	t.Setenv("TT_RES_CONFLUENCE", "")
	t.Setenv("CONFLUENCE_API_TOKEN", "")

	viper.Set("embedder", "openai")
	viper.Set("chunk_size", 1024)
	viper.Set("chunk_overlap", 128)
	viper.Set("ui_port", 9090)
	viper.Set("ui_bind", "0.0.0.0")
	viper.Set("db_path", "/tmp/custom.db")
	viper.Set("docs_path", "/tmp/docs/")
	viper.Set("embed_workers", 8)
	viper.Set("embed_batch_size", 64)
	viper.Set("ollama.url", "http://ollama.internal:11434")
	viper.Set("openai.embed_model", "text-embedding-3-large")
	viper.Set("ignore", []string{"foo", "bar"})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Embedder != "openai" {
		t.Errorf("Embedder = %q, want %q", cfg.Embedder, "openai")
	}
	if cfg.ChunkSize != 1024 {
		t.Errorf("ChunkSize = %d, want 1024", cfg.ChunkSize)
	}
	if cfg.ChunkOverlap != 128 {
		t.Errorf("ChunkOverlap = %d, want 128", cfg.ChunkOverlap)
	}
	if cfg.UIPort != 9090 {
		t.Errorf("UIPort = %d, want 9090", cfg.UIPort)
	}
	if cfg.UIBind != "0.0.0.0" {
		t.Errorf("UIBind = %q, want 0.0.0.0", cfg.UIBind)
	}
	if cfg.DBPath != "/tmp/custom.db" {
		t.Errorf("DBPath = %q", cfg.DBPath)
	}
	if cfg.DocsPath != "/tmp/docs/" {
		t.Errorf("DocsPath = %q", cfg.DocsPath)
	}
	if cfg.EmbedWorkers != 8 {
		t.Errorf("EmbedWorkers = %d, want 8", cfg.EmbedWorkers)
	}
	if cfg.EmbedBatchSize != 64 {
		t.Errorf("EmbedBatchSize = %d, want 64", cfg.EmbedBatchSize)
	}
	if cfg.Ollama.URL != "http://ollama.internal:11434" {
		t.Errorf("Ollama.URL = %q", cfg.Ollama.URL)
	}
	if cfg.OpenAI.EmbedModel != "text-embedding-3-large" {
		t.Errorf("OpenAI.EmbedModel = %q", cfg.OpenAI.EmbedModel)
	}
	if len(cfg.Ignore) != 2 || cfg.Ignore[0] != "foo" || cfg.Ignore[1] != "bar" {
		t.Errorf("Ignore = %v, want [foo bar]", cfg.Ignore)
	}
}

func TestEmbedDimensions(t *testing.T) {
	cases := []struct {
		name     string
		embedder string
		want     int
	}{
		{"openai", "openai", 1536},
		{"ollama", "ollama", 768},
		{"empty defaults to 768", "", 768},
		{"unknown defaults to 768", "some-other-provider", 768},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := &Config{Embedder: tc.embedder}
			if got := c.EmbedDimensions(); got != tc.want {
				t.Errorf("EmbedDimensions(%q) = %d, want %d", tc.embedder, got, tc.want)
			}
		})
	}
}
