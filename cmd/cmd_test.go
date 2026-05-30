package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/bobbydeveaux/cerebra/internal/config"
	"github.com/spf13/cobra"
)

// TestRootCommandMetadata locks the user-visible identity of the CLI binary so
// renames go through a deliberate code change.
func TestRootCommandMetadata(t *testing.T) {
	if rootCmd.Use != "cerebra" {
		t.Errorf("rootCmd.Use = %q, want %q", rootCmd.Use, "cerebra")
	}
	if rootCmd.Short == "" {
		t.Error("rootCmd.Short is empty — every command must have a short description")
	}
	if rootCmd.Long == "" {
		t.Error("rootCmd.Long is empty — the root binary should ship a long description")
	}
	if !strings.Contains(rootCmd.Long, "Jor-El") {
		t.Errorf("rootCmd.Long should mention Jor-El (the SQLite vector store); got %q", rootCmd.Long)
	}
}

// TestRootCommandPersistentFlags ensures the cross-cutting --config and
// --db-path persistent flags remain on the root command, so every subcommand
// inherits them.
func TestRootCommandPersistentFlags(t *testing.T) {
	flags := []string{"config", "db-path"}
	for _, name := range flags {
		t.Run(name, func(t *testing.T) {
			f := rootCmd.PersistentFlags().Lookup(name)
			if f == nil {
				t.Fatalf("persistent flag %q not registered on rootCmd", name)
			}
			if f.DefValue != "" {
				t.Errorf("persistent flag %q default = %q, want empty string", name, f.DefValue)
			}
		})
	}
}

// TestRootCommandSubcommandRegistry asserts the top-level command tree. Adding
// or removing a subcommand without updating this list is a deliberate decision
// that must update the test in the same PR.
func TestRootCommandSubcommandRegistry(t *testing.T) {
	want := map[string]bool{
		"brains":         false,
		"forget":         false,
		"scan":           false,
		"search":         false,
		"serve":          false,
		"stats":          false,
		"watch":          false,
		"help":           false, // cobra auto-registers
		"completion":     false, // cobra auto-registers
	}
	for _, c := range rootCmd.Commands() {
		if _, expected := want[c.Name()]; expected {
			want[c.Name()] = true
		}
	}
	for name, found := range want {
		// help and completion are cobra-injected; tolerate either presence.
		if name == "help" || name == "completion" {
			continue
		}
		if !found {
			t.Errorf("subcommand %q not registered on rootCmd", name)
		}
	}
}

// TestBrainsSubcommandTree locks the `brains` command's three subcommands.
func TestBrainsSubcommandTree(t *testing.T) {
	subs := map[string]bool{"watch": false, "list": false, "index": false}
	for _, c := range brainsCmd.Commands() {
		if _, ok := subs[c.Name()]; ok {
			subs[c.Name()] = true
		}
	}
	for name, found := range subs {
		if !found {
			t.Errorf("brains subcommand %q not registered", name)
		}
	}
	if brainsCmd.Short == "" {
		t.Error("brainsCmd.Short is empty")
	}
}

// TestScanCommandFlags covers the four scan flags and their defaults.
func TestScanCommandFlags(t *testing.T) {
	cases := []struct {
		flag       string
		wantDef    string
		wantUsage  string // substring expected in the usage text
	}{
		{flag: "dry-run", wantDef: "false", wantUsage: "Preview"},
		{flag: "full", wantDef: "false", wantUsage: "Force"},
		{flag: "upload", wantDef: "", wantUsage: "Upload"},
		{flag: "confluence", wantDef: "false", wantUsage: "Confluence"},
		{flag: "ci", wantDef: "false", wantUsage: "CI"},
	}
	for _, tc := range cases {
		t.Run(tc.flag, func(t *testing.T) {
			f := scanCmd.Flags().Lookup(tc.flag)
			if f == nil {
				t.Fatalf("scan flag %q not registered", tc.flag)
			}
			if f.DefValue != tc.wantDef {
				t.Errorf("scan --%s default = %q, want %q", tc.flag, f.DefValue, tc.wantDef)
			}
			if !strings.Contains(f.Usage, tc.wantUsage) {
				t.Errorf("scan --%s usage = %q, want substring %q", tc.flag, f.Usage, tc.wantUsage)
			}
		})
	}
}

// TestSearchCommandFlags covers --limit default and metadata.
func TestSearchCommandFlags(t *testing.T) {
	f := searchCmd.Flags().Lookup("limit")
	if f == nil {
		t.Fatal("search --limit flag not registered")
	}
	if f.DefValue != "5" {
		t.Errorf("search --limit default = %q, want %q", f.DefValue, "5")
	}
	if searchCmd.Use != "search [query]" {
		t.Errorf("searchCmd.Use = %q, want %q", searchCmd.Use, "search [query]")
	}
}

// TestServeCommandFlags locks all three serve flags.
func TestServeCommandFlags(t *testing.T) {
	cases := []struct {
		flag    string
		wantDef string
	}{
		{flag: "ui", wantDef: "false"},
		{flag: "port", wantDef: "0"},
		{flag: "db", wantDef: ""},
	}
	for _, tc := range cases {
		t.Run(tc.flag, func(t *testing.T) {
			f := serveCmd.Flags().Lookup(tc.flag)
			if f == nil {
				t.Fatalf("serve flag %q not registered", tc.flag)
			}
			if f.DefValue != tc.wantDef {
				t.Errorf("serve --%s default = %q, want %q", tc.flag, f.DefValue, tc.wantDef)
			}
		})
	}
}

// TestForgetCommandArgs locks ExactArgs(1) — the cobra Args validator runs
// before RunE, so this exercises arg validation without touching the store.
func TestForgetCommandArgs(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		wantError bool
	}{
		{name: "zero args", args: []string{}, wantError: true},
		{name: "one arg", args: []string{"some/path"}, wantError: false},
		{name: "two args", args: []string{"a", "b"}, wantError: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := forgetCmd.Args(forgetCmd, tc.args)
			if tc.wantError && err == nil {
				t.Errorf("forget Args(%v) = nil, want error", tc.args)
			}
			if !tc.wantError && err != nil {
				t.Errorf("forget Args(%v) = %v, want nil", tc.args, err)
			}
		})
	}
}

// TestSearchCommandArgs locks ExactArgs(1).
func TestSearchCommandArgs(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		wantError bool
	}{
		{name: "zero args", args: []string{}, wantError: true},
		{name: "one arg", args: []string{"query string"}, wantError: false},
		{name: "two args", args: []string{"a", "b"}, wantError: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := searchCmd.Args(searchCmd, tc.args)
			if tc.wantError && err == nil {
				t.Errorf("search Args(%v) = nil, want error", tc.args)
			}
			if !tc.wantError && err != nil {
				t.Errorf("search Args(%v) = %v, want nil", tc.args, err)
			}
		})
	}
}

// TestScanAndWatchAcceptVariableArgs both use MaximumNArgs(1).
func TestScanAndWatchAcceptVariableArgs(t *testing.T) {
	for _, c := range []*cobra.Command{scanCmd, watchCmd} {
		t.Run(c.Name(), func(t *testing.T) {
			if err := c.Args(c, []string{}); err != nil {
				t.Errorf("%s Args([]) = %v, want nil", c.Name(), err)
			}
			if err := c.Args(c, []string{"./path"}); err != nil {
				t.Errorf("%s Args([path]) = %v, want nil", c.Name(), err)
			}
			if err := c.Args(c, []string{"a", "b"}); err == nil {
				t.Errorf("%s Args([a,b]) = nil, want error", c.Name())
			}
		})
	}
}

// TestRootHelpOutputMentionsAllSubcommands drives cobra's help machinery — the
// closest thing to a smoke test of Execute() that doesn't open a real database.
func TestRootHelpOutputMentionsAllSubcommands(t *testing.T) {
	buf := &bytes.Buffer{}
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"--help"})
	defer rootCmd.SetArgs(nil)

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("rootCmd.Execute(--help) = %v, want nil", err)
	}
	out := buf.String()
	for _, name := range []string{"brains", "forget", "scan", "search", "serve", "stats", "watch"} {
		if !strings.Contains(out, name) {
			t.Errorf("help output missing subcommand %q.\nGot:\n%s", name, out)
		}
	}
	if !strings.Contains(out, "Cerebra") {
		t.Errorf("help output missing 'Cerebra' header; got:\n%s", out)
	}
}

// TestSubcommandHelpOutputs walks the tree and runs --help against each leaf
// subcommand to surface any panics in help generation and to lift coverage on
// flag-registration init() blocks.
func TestSubcommandHelpOutputs(t *testing.T) {
	leafs := []*cobra.Command{
		brainsCmd, brainsListCmd, brainsIndexCmd, brainsWatchCmd,
		forgetCmd, scanCmd, searchCmd, serveCmd, statsCmd, watchCmd,
	}
	for _, c := range leafs {
		t.Run(c.Name(), func(t *testing.T) {
			buf := &bytes.Buffer{}
			c.SetOut(buf)
			c.SetErr(buf)
			if err := c.Help(); err != nil {
				t.Errorf("%s.Help() = %v, want nil", c.Name(), err)
			}
			if buf.Len() == 0 {
				t.Errorf("%s.Help() produced empty output", c.Name())
			}
		})
	}
}

// TestHashPathIsDeterministic locks the helper that scan.go and watch.go both
// use to derive document IDs.
func TestHashPathIsDeterministic(t *testing.T) {
	a := hashPath("internal/store/store.go")
	b := hashPath("internal/store/store.go")
	c := hashPath("internal/store/other.go")

	if a == "" {
		t.Fatal("hashPath returned empty string")
	}
	if a != b {
		t.Errorf("hashPath not deterministic: %q vs %q", a, b)
	}
	if a == c {
		t.Errorf("hashPath collision: distinct inputs produced same hash %q", a)
	}
	if len(a) != 64 {
		t.Errorf("hashPath length = %d, want 64 (sha256 hex)", len(a))
	}
}

// TestCreateEmbedderSwitchArms exercises both branches of createEmbedder
// without making a network call — construction only.
func TestCreateEmbedderSwitchArms(t *testing.T) {
	prev := cfg
	defer func() { cfg = prev }()

	t.Run("ollama default", func(t *testing.T) {
		cfg = &config.Config{
			Embedder: "ollama",
			Ollama: config.OllamaConfig{
				URL:        "http://localhost:11434",
				EmbedModel: "nomic-embed-text",
			},
		}
		e := createEmbedder()
		if e == nil {
			t.Fatal("createEmbedder returned nil for ollama")
		}
		if got := e.Dimensions(); got != 768 {
			t.Errorf("ollama Dimensions = %d, want 768", got)
		}
	})

	t.Run("openai", func(t *testing.T) {
		cfg = &config.Config{
			Embedder: "openai",
			OpenAI: config.OpenAIConfig{
				APIKey:     "sk-test-not-used",
				EmbedModel: "text-embedding-3-small",
			},
		}
		e := createEmbedder()
		if e == nil {
			t.Fatal("createEmbedder returned nil for openai")
		}
		if got := e.Dimensions(); got != 1536 {
			t.Errorf("openai Dimensions = %d, want 1536", got)
		}
	})

	t.Run("unknown defaults to ollama", func(t *testing.T) {
		cfg = &config.Config{
			Embedder: "totally-made-up",
			Ollama: config.OllamaConfig{
				URL:        "http://localhost:11434",
				EmbedModel: "nomic-embed-text",
			},
		}
		e := createEmbedder()
		if e == nil {
			t.Fatal("createEmbedder returned nil for unknown embedder")
		}
		if got := e.Dimensions(); got != 768 {
			t.Errorf("unknown -> ollama Dimensions = %d, want 768", got)
		}
	})
}

// TestCommandsHaveShortDescriptions guards against new commands shipping with
// no help summary — cobra renders empty Short fields as blanks in the help
// list, which looks unfinished to users.
func TestCommandsHaveShortDescriptions(t *testing.T) {
	for _, c := range rootCmd.Commands() {
		t.Run(c.Name(), func(t *testing.T) {
			if c.Name() == "help" || c.Name() == "completion" {
				return
			}
			if c.Short == "" {
				t.Errorf("subcommand %q has empty Short description", c.Name())
			}
		})
	}
}
