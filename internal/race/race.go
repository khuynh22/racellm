// Package race builds the set of provider-model entrants from the loaded configuration.
package race

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/glamour"
	glamourstyles "github.com/charmbracelet/glamour/styles"
	"github.com/khang/racellm/internal/config"
	"github.com/khang/racellm/internal/coordinator"
	"github.com/khang/racellm/internal/provider"
	"github.com/khang/racellm/internal/tui"
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

// Run executes the race using the BubbleTea live dashboard.
func Run(ctx context.Context, cfg *config.Config, prompt string, mode coordinator.RaceMode) error {
	entrants, err := BuildEntrants(cfg)
	if err != nil {
		return err
	}

	raceCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	coord := coordinator.New(entrants, mode)
	eventChan, resultsChan := coord.Race(raceCtx, prompt)

	results, err := tui.Run(cancel, entrants, prompt, mode, eventChan, resultsChan)
	if err != nil {
		return err
	}

	if len(results) > 0 && results[0].Err == nil {
		fmt.Println(strings.Repeat("─", 60))
		fmt.Printf("\n💬 Fastest response (%s/%s):\n\n", results[0].Provider, results[0].Model)
		fmt.Print(renderMarkdown(results[0].FullText))
	}

	return nil
}

// renderMarkdown renders markdown text using a customized glamour dark style
// that replaces heading prefix markers (##, ###, …) with styled plain text.
func renderMarkdown(text string) string {
	style := glamourstyles.DarkStyleConfig
	// Clear the ## / ### … prefixes so headings render as styled text without
	// markdown syntax leaking through. They inherit bold+color from the base
	// Heading style defined in the theme.
	style.H2.Prefix = ""
	style.H3.Prefix = ""
	style.H4.Prefix = ""
	style.H5.Prefix = ""
	style.H6.Prefix = ""

	r, err := glamour.NewTermRenderer(
		glamour.WithStyles(style),
		glamour.WithWordWrap(100),
	)
	if err != nil {
		return text
	}
	rendered, err := r.Render(text)
	if err != nil {
		return text
	}
	return rendered
}
