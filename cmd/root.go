package cmd

import (
	"fmt"
	"os"

	"github.com/bobbydeveaux/cerebra/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfg *config.Config

var rootCmd = &cobra.Command{
	Use:   "cerebra",
	Short: "Cerebra — AI agent memory and codebase knowledge base powered by embeddings",
	Long:  "Cerebra scans codebases, indexes AI agent conversations, generates embeddings, and stores them in a local SQLite vector database (Jor-El). Queryable via MCP server, CLI, and web UI.",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().String("config", "", "config file (default cerebra.yaml)")
	rootCmd.PersistentFlags().String("db-path", "", "database path (default .cerebra/jor-el.db)")
}

func initConfig() {
	cfgFile, _ := rootCmd.Flags().GetString("config")
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.SetConfigName("cerebra")
		viper.SetConfigType("yaml")
		viper.AddConfigPath(".")
		if home, err := os.UserHomeDir(); err == nil {
			viper.AddConfigPath(home)
		}
	}

	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			fmt.Fprintf(os.Stderr, "Warning: error reading config: %v\n", err)
		}
	}

	var err error
	cfg, err = config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	if dbPath, _ := rootCmd.Flags().GetString("db-path"); dbPath != "" {
		cfg.DBPath = dbPath
	}
}
