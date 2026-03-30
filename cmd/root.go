// Package cmd defines the CLI commands for racellm.
package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/khang/racellm/internal/config"
	"github.com/khang/racellm/internal/coordinator"
	"github.com/khang/racellm/internal/race"
)

var (
	cfgFile  string
	modeFlag string
	version  = "0.1.0"
)

var rootCmd = &cobra.Command{
	Use:   "racellm [prompt]",
	Short: "Race multiple AI models simultaneously and get the fastest response",
	Long: `RaceLLM 🏁 — Race your LLMs!

Fire your prompt to every configured model at once, stream results
in parallel, and let the fastest or best response win.

Examples:
  racellm "Explain goroutines in Go"
  racellm "Write a regex for email" --mode fastest
  racellm --config myconfig.yaml "What is the meaning of life?"`,
	Args: cobra.MinimumNArgs(1),
	RunE: runRace,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("racellm v%s\n", version)
	},
}

var modelsCmd = &cobra.Command{
	Use:   "models",
	Short: "List all configured providers and models",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return err
		}

		entrants, err := race.BuildEntrants(cfg)
		if err != nil {
			return err
		}

		fmt.Println("Configured racers:")
		for _, e := range entrants {
			fmt.Printf("  • %s / %s\n", e.Provider.Name(), e.Model)
		}
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ~/.racellm.yaml)")
	rootCmd.PersistentFlags().StringVar(&modeFlag, "mode", "", "race mode: 'fastest' (cancel losers) or 'all' (wait for everyone)")
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(modelsCmd)
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runRace(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("config error: %w", err)
	}

	prompt := strings.Join(args, " ")
	if prompt == "" {
		return fmt.Errorf("prompt cannot be empty")
	}

	// Determine race mode.
	var mode coordinator.RaceMode
	modeStr := cfg.DefaultMode
	if modeFlag != "" {
		modeStr = modeFlag
	}
	switch modeStr {
	case "fastest":
		mode = coordinator.ModeFastest
	case "all", "":
		mode = coordinator.ModeAll
	default:
		return fmt.Errorf("unknown mode %q; use 'fastest' or 'all'", modeStr)
	}

	// Set up context with OS signal handling for graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	return race.Run(ctx, cfg, prompt, mode)
}
