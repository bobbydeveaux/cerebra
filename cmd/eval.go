package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bobbydeveaux/cerebra/internal/eval"
	"github.com/bobbydeveaux/cerebra/internal/store"
	"github.com/spf13/cobra"
)

var (
	evalCI        bool
	evalThreshold float64
	evalTopN      int
)

var evalCmd = &cobra.Command{
	Use:   "eval",
	Short: "Run the retrieval eval suite (FTS-only, no API keys)",
	Long: "Run a deterministic, API-free retrieval evaluation of the search " +
		"pipeline. With --ci, the suite seeds an ephemeral database from an " +
		"embedded fixture corpus, runs FTS retrieval for each question, prints " +
		"the pass/fail count, and exits non-zero when the pass rate falls below " +
		"the threshold. Intended as a CI quality gate; the live LLM ablation " +
		"benchmark still lives in evals/run.sh.",
	RunE: runEval,
}

func init() {
	evalCmd.Flags().BoolVar(&evalCI, "ci", false, "CI mode: ephemeral DB seeded from embedded fixtures, exit non-zero below --threshold")
	evalCmd.Flags().Float64Var(&evalThreshold, "threshold", 0.70, "Minimum pass rate (0..1) required to exit 0")
	evalCmd.Flags().IntVar(&evalTopN, "top-n", 5, "Number of FTS results inspected per question")
	rootCmd.AddCommand(evalCmd)
}

func runEval(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	if !evalCI {
		return fmt.Errorf("only --ci mode is currently supported; pass --ci")
	}

	dir, err := os.MkdirTemp("", "cerebra-eval-")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(dir)

	db, err := store.New(filepath.Join(dir, "eval.db"), 768)
	if err != nil {
		return fmt.Errorf("opening ephemeral database: %w", err)
	}
	defer db.Close()

	if err := eval.Seed(ctx, db); err != nil {
		return fmt.Errorf("seeding fixtures: %w", err)
	}

	rep, err := eval.Run(ctx, db, eval.Questions(), evalTopN)
	if err != nil {
		return fmt.Errorf("running eval suite: %w", err)
	}

	for _, r := range rep.Results {
		status := "PASS"
		if !r.Pass {
			status = "FAIL"
		}
		fmt.Printf("  [%s] %-3s %s\n", status, r.Question.ID, r.Question.Query)
		if !r.Pass {
			fmt.Printf("         missing term: %q (hits=%d)\n", r.Missing, r.Hits)
		}
	}

	fmt.Printf("eval: %d/%d passed (%.0f%%), threshold %.0f%%\n",
		rep.Pass, rep.Total, rep.PassRate*100, evalThreshold*100)

	if !rep.Meets(evalThreshold) {
		return fmt.Errorf("pass rate %.0f%% below threshold %.0f%%", rep.PassRate*100, evalThreshold*100)
	}
	return nil
}
