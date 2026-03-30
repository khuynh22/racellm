// Package race builds the set of provider-model entrants from the loaded configuration.
package race

import (
	"context"
	"fmt"
	"strings"

	"github.com/khang/racellm/internal/config"
	"github.com/khang/racellm/internal/coordinator"
	"github.com/khang/racellm/internal/provider"
)

// BuildEntrants reads the config and constructs all the provider+model entrants.
func BuildEntrants(cfg *config.Config) ([]coordinator.Entrant, error) {
	var entrants []coordinator.Entrant

	if e := cfg.Providers.OpenAI; e != nil && e.Enabled {
		p := provider.NewOpenAI(e.ProviderConfig)
		for _, model := range e.Models {
			entrants = append(entrants, coordinator.Entrant{Provider: p, Model: model})
		}
	}

	if e := cfg.Providers.Anthropic; e != nil && e.Enabled {
		p := provider.NewAnthropic(e.ProviderConfig)
		for _, model := range e.Models {
			entrants = append(entrants, coordinator.Entrant{Provider: p, Model: model})
		}
	}

	if e := cfg.Providers.Gemini; e != nil && e.Enabled {
		p := provider.NewGemini(e.ProviderConfig)
		for _, model := range e.Models {
			entrants = append(entrants, coordinator.Entrant{Provider: p, Model: model})
		}
	}

	if e := cfg.Providers.Ollama; e != nil && e.Enabled {
		p := provider.NewOllama(e.ProviderConfig)
		for _, model := range e.Models {
			entrants = append(entrants, coordinator.Entrant{Provider: p, Model: model})
		}
	}

	if len(entrants) == 0 {
		return nil, fmt.Errorf("no providers enabled; check your config file")
	}

	return entrants, nil
}

// Run executes the race and prints results to stdout.
func Run(ctx context.Context, cfg *config.Config, prompt string, mode coordinator.RaceMode) error {
	entrants, err := BuildEntrants(cfg)
	if err != nil {
		return err
	}

	fmt.Printf("🏁 Racing %d model(s) with prompt: %q\n\n", len(entrants), truncate(prompt, 80))

	coord := coordinator.New(entrants, mode)
	eventChan, resultsChan := coord.Race(ctx, prompt)

	// Print live events.
	for event := range eventChan {
		switch event.Type {
		case coordinator.EventFirst:
			// TTFT is available on the final Result; this just signals the first token arrived.
			fmt.Printf("⚡ [%s] First token!\n", event.Entrant)
		case coordinator.EventToken:
			// In a TUI this would update the progress bar. For now, dots.
		case coordinator.EventFinish:
			if event.Result != nil && event.Result.Err == nil {
				fmt.Printf("✅ [%s] Finished in %s (%d tokens)\n",
					event.Entrant,
					coordinator.FormatDuration(event.Result.TotalTime),
					event.Result.TokenCount)
			}
		case coordinator.EventError:
			fmt.Printf("❌ [%s] Error: %v\n", event.Entrant, event.Err)
		case coordinator.EventWinner:
			fmt.Printf("\n🏆 WINNER: %s (completed in %s)\n\n",
				event.Entrant,
				coordinator.FormatDuration(event.Result.TotalTime))
		}
	}

	// Print final scoreboard.
	results := <-resultsChan
	fmt.Println("\n" + strings.Repeat("─", 60))
	fmt.Println("📊 RACE RESULTS")
	fmt.Println(strings.Repeat("─", 60))

	for i, r := range results {
		medal := " "
		switch {
		case i == 0 && r.Err == nil:
			medal = "🥇"
		case i == 1 && r.Err == nil:
			medal = "🥈"
		case i == 2 && r.Err == nil:
			medal = "🥉"
		}

		if r.Err != nil {
			fmt.Printf("%s %s/%s — ERROR: %v\n", medal, r.Provider, r.Model, r.Err)
		} else {
			fmt.Printf("%s %s/%s — %s (TTFT: %s, Tokens: %d)\n",
				medal, r.Provider, r.Model,
				coordinator.FormatDuration(r.TotalTime),
				coordinator.FormatDuration(r.TTFT),
				r.TokenCount)
		}
	}

	// Print winner's full response.
	if len(results) > 0 && results[0].Err == nil {
		fmt.Println(strings.Repeat("─", 60))
		fmt.Printf("\n💬 Best response (%s/%s):\n\n", results[0].Provider, results[0].Model)
		fmt.Println(results[0].FullText)
	}

	return nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
