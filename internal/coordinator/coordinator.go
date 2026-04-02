// Package coordinator manages the race between multiple LLM providers.
package coordinator

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/khang/racellm/internal/provider"
)

// RaceMode determines how the race behaves once a winner is found.
type RaceMode int

const (
	// ModeAll waits for all providers to finish.
	ModeAll RaceMode = iota
	// ModeFastest cancels remaining providers once the first one completes.
	ModeFastest
)

// Entrant is a single racer: a provider + model combination.
type Entrant struct {
	Provider provider.Provider
	Model    string
}

// RaceEvent is emitted on the event channel to inform the UI of race progress.
type RaceEvent struct {
	Type    EventType
	Token   *provider.Token
	Result  *provider.Result
	Entrant string
	Err     error
}

// EventType classifies the kind of race event emitted on the event channel.
type EventType int

// Event type constants used in RaceEvent.Type.
const (
	EventToken EventType = iota
	EventFirst
	EventFinish
	EventError
	EventWinner
)

// Coordinator manages the concurrent fan-out of prompts to multiple providers
// and aggregates their streaming results through channels.
type Coordinator struct {
	Entrants []Entrant
	Mode     RaceMode
}

// New creates a Coordinator with the given entrants and mode.
func New(entrants []Entrant, mode RaceMode) *Coordinator {
	return &Coordinator{
		Entrants: entrants,
		Mode:     mode,
	}
}

// Race starts the concurrent race. It returns:
// - eventChan: real-time events for the UI (tokens, finishes, errors)
// - resultsChan: final sorted results once all racers are done
//
// The caller should read from eventChan until it is closed, then read
// from resultsChan for the final summary.
func (c *Coordinator) Race(ctx context.Context, prompt string) (<-chan RaceEvent, <-chan []provider.Result) {
	eventChan := make(chan RaceEvent, 256)
	resultsChan := make(chan []provider.Result, 1)

	go c.run(ctx, prompt, eventChan, resultsChan)

	return eventChan, resultsChan
}

func (c *Coordinator) run(parentCtx context.Context, prompt string, eventChan chan<- RaceEvent, resultsChan chan<- []provider.Result) {
	defer close(eventChan)
	defer close(resultsChan)

	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel()

	var (
		wg          sync.WaitGroup
		mu          sync.Mutex
		results     []provider.Result
		firstTokens = make(map[string]bool) // track which entrants got their first token
		winnerFound bool
	)

	tokenChan := make(chan provider.Token, 512)

	for _, entrant := range c.Entrants {
		wg.Add(1)
		go func(e Entrant) {
			defer wg.Done()
			label := fmt.Sprintf("%s/%s", e.Provider.Name(), e.Model)

			result, err := e.Provider.Stream(ctx, e.Model, prompt, tokenChan)
			if err != nil && ctx.Err() == nil {
				eventChan <- RaceEvent{
					Type:    EventError,
					Entrant: label,
					Err:     err,
				}
			}

			mu.Lock()
			results = append(results, result)

			if c.Mode == ModeFastest && !winnerFound && result.Err == nil {
				winnerFound = true
				eventChan <- RaceEvent{
					Type:    EventWinner,
					Entrant: label,
					Result:  &result,
				}
				cancel()
			}
			mu.Unlock()

			eventChan <- RaceEvent{
				Type:    EventFinish,
				Entrant: label,
				Result:  &result,
			}
		}(entrant)
	}

	go func() {
		for token := range tokenChan {
			label := fmt.Sprintf("%s/%s", token.Provider, token.Model)

			mu.Lock()
			isFirst := !firstTokens[label]
			if isFirst {
				firstTokens[label] = true
			}
			mu.Unlock()

			if isFirst {
				eventChan <- RaceEvent{
					Type:    EventFirst,
					Token:   &token,
					Entrant: label,
				}
			}

			eventChan <- RaceEvent{
				Type:    EventToken,
				Token:   &token,
				Entrant: label,
			}
		}
	}()

	wg.Wait()
	close(tokenChan)

	sort.Slice(results, func(i, j int) bool {
		if results[i].Err != nil && results[j].Err == nil {
			return false
		}
		if results[i].Err == nil && results[j].Err != nil {
			return true
		}
		return results[i].TotalTime < results[j].TotalTime
	})

	resultsChan <- results
}

// FormatDuration returns a human-friendly duration string.
func FormatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.2fs", d.Seconds())
}
