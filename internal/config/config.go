package config

import (
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

type Config struct {
	Embedder string       `mapstructure:"embedder"`
	Ollama   OllamaConfig `mapstructure:"ollama"`
	OpenAI   OpenAIConfig `mapstructure:"openai"`
	ChatLLM  string       `mapstructure:"chat_llm"`
	Claude   ClaudeConfig  `mapstructure:"claude"`
	MiniMax    MiniMaxConfig    `mapstructure:"minimax"`
	Confluence ConfluenceConfig `mapstructure:"confluence"`

	Ignore []string `mapstructure:"ignore"`

	ChunkSize    int `mapstructure:"chunk_size"`
	ChunkOverlap int `mapstructure:"chunk_overlap"`

	DBPath   string `mapstructure:"db_path"`
	DocsPath string `mapstructure:"docs_path"`

	CloudStorage string `mapstructure:"cloud_storage"`

	UIPort int    `mapstructure:"ui_port"`
	UIBind string `mapstructure:"ui_bind"`

	EmbedWorkers   int `mapstructure:"embed_workers"`
	EmbedBatchSize int `mapstructure:"embed_batch_size"`

	BrainWatchPath string `mapstructure:"brain_watch_path"`
}

type OllamaConfig struct {
	URL        string `mapstructure:"url"`
	EmbedModel string `mapstructure:"embed_model"`
	ChatModel  string `mapstructure:"chat_model"`
}

type OpenAIConfig struct {
	APIKey     string `mapstructure:"api_key"`
	EmbedModel string `mapstructure:"embed_model"`
	ChatModel  string `mapstructure:"chat_model"`
}

type ClaudeConfig struct {
	APIKey string `mapstructure:"api_key"`
	Model  string `mapstructure:"model"`
}

type MiniMaxConfig struct {
	APIKey string `mapstructure:"api_key"`
	Model  string `mapstructure:"model"`
}

type ConfluenceConfig struct {
	BaseURL   string   `mapstructure:"base_url"`
	Email     string   `mapstructure:"email"`
	APIToken  string   `mapstructure:"api_token"`
	SpaceKeys []string `mapstructure:"space_keys"`
}

func Load() (*Config, error) {
	cfg := &Config{
		Embedder: "ollama",
		Ollama: OllamaConfig{
			URL:        "http://localhost:11434",
			EmbedModel: "nomic-embed-text",
			ChatModel:  "llama3.2",
		},
		OpenAI: OpenAIConfig{
			EmbedModel: "text-embedding-3-small",
			ChatModel:  "gpt-4o",
		},
		ChatLLM: "ollama",
		Claude: ClaudeConfig{
			Model: "claude-sonnet-4-6",
		},
		MiniMax: MiniMaxConfig{
			Model: "MiniMax-M2.7-highspeed",
		},
		Ignore: []string{
			".git", "node_modules", "vendor", ".venv", "__pycache__",
			"*.bin", "*.exe", "*.so", "*.dylib",
			"dist", "build", "coverage", ".cerebra",
		},
		ChunkSize:      512,
		ChunkOverlap:   64,
		DBPath:         ".cerebra/jor-el.db",
		DocsPath:       ".cerebra/docs/",
		UIPort:         8080,
		UIBind:         "127.0.0.1",
		EmbedWorkers:   2,
		EmbedBatchSize: 32,
	}

	if home, err := os.UserHomeDir(); err == nil {
		cfg.BrainWatchPath = filepath.Join(home, ".claude", "projects")
	}

	if err := viper.Unmarshal(cfg); err != nil {
		return nil, err
	}

	if key := os.Getenv("OPENAI_API_KEY"); key != "" && cfg.OpenAI.APIKey == "" {
		cfg.OpenAI.APIKey = key
	}
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" && cfg.Claude.APIKey == "" {
		cfg.Claude.APIKey = key
	}
	if key := os.Getenv("MINIMAX_API_KEY"); key != "" && cfg.MiniMax.APIKey == "" {
		cfg.MiniMax.APIKey = key
	}
	if key := os.Getenv("MEALFIT_MINIMAX"); key != "" && cfg.MiniMax.APIKey == "" {
		cfg.MiniMax.APIKey = key
	}
	if token := os.Getenv("TT_RES_CONFLUENCE"); token != "" && cfg.Confluence.APIToken == "" {
		cfg.Confluence.APIToken = token
	}
	if token := os.Getenv("CONFLUENCE_API_TOKEN"); token != "" && cfg.Confluence.APIToken == "" {
		cfg.Confluence.APIToken = token
	}

	return cfg, nil
}

func (c *Config) EmbedDimensions() int {
	switch c.Embedder {
	case "openai":
		return 1536
	default:
		return 768
	}
}
