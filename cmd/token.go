package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/bobbydeveaux/cerebra/internal/mcp"
)

// tokenCmd mints a multi-tenant MCP token for a user, using the shared
// CEREBRA_TOKEN_SECRET. This is a dev/operator convenience for the HTTP
// transport (serve --http); the operator normally mints these out-of-band.
//
// NOTE (future ref): the production token scheme should adopt the
// aisecurityposture/backend approach — DPoP (RFC 9449) sender-constrained
// tokens, ed25519 signing, JWKS key rotation — rather than this simple
// shared-secret HMAC. See docs in agentops-operator.
var tokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Mint a multi-tenant MCP token for a user (uses CEREBRA_TOKEN_SECRET)",
	RunE: func(cmd *cobra.Command, args []string) error {
		user, _ := cmd.Flags().GetString("user")
		if user == "" {
			return fmt.Errorf("--user is required")
		}
		secret := os.Getenv("CEREBRA_TOKEN_SECRET")
		if secret == "" {
			return fmt.Errorf("CEREBRA_TOKEN_SECRET is not set")
		}
		tok, err := mcp.SignToken(user, secret)
		if err != nil {
			return err
		}
		fmt.Println(tok)
		return nil
	},
}

func init() {
	tokenCmd.Flags().String("user", "", "user id to mint a token for")
	rootCmd.AddCommand(tokenCmd)
}
