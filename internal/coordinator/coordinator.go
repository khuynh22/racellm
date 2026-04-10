// Package coordinator manages the race between multiple LLM providers.
package coordinator

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/khuynh22/racellm/internal/provider"
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
		barrierSeen = make(map[string]bool) // per-entrant: first-token or error checked in
		winnerFound bool
	)

	// Fair-start barrier: closed once every entrant has either produced its
	// first token or failed. ModeFastest winner selection is gated on this so
	// that API/model spin-up time is excluded from the race.
	firstReadyCh := make(chan struct{})
	barrierLeft := len(c.Entrants)
	var barrierOnce sync.Once

	barrierCheckin := func(label string) {
		mu.Lock()
		if !barrierSeen[label] {
			barrierSeen[label] = true
			barrierLeft--
			if barrierLeft == 0 {
				barrierOnce.Do(func() { close(firstReadyCh) })
			}
		}
		mu.Unlock()
	}

	tokenChan := make(chan provider.Token, 512)

	for _, entrant := range c.Entrants {
		wg.Add(1)
		go func(e Entrant) {
			defer wg.Done()
			label := fmt.Sprintf("%s/%s", e.Provider.Name(), e.Model)

			result, err := e.Provider.Stream(ctx, e.Model, prompt, tokenChan)

			// If this provider never produced a token (errored before streaming
			// started), release its barrier slot so the countdown can complete.
			barrierCheckin(label)

			// Distinguish context cancellation (caused by another racer winning)
			// from a genuine API/network error.
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				result.Canceled = true
				result.Err = nil
				err = nil
			}

			if err != nil {
				eventChan <- RaceEvent{
					Type:    EventError,
					Entrant: label,
					Err:     err,
				}
			}

			mu.Lock()
			results = append(results, result)
			mu.Unlock()

			// Emit finish immediately so the TUI can update timing.
			eventChan <- RaceEvent{
				Type:    EventFinish,
				Entrant: label,
				Result:  &result,
			}

			// For fastest mode: block until every provider has warmed up, then
			// the first one to have already completed claims the win.
			if c.Mode == ModeFastest && result.Err == nil && !result.Canceled {
				<-firstReadyCh
				mu.Lock()
				alreadyWon := winnerFound
				if !winnerFound {
					winnerFound = true
				}
				mu.Unlock()
				if !alreadyWon {
					cancel()
					eventChan <- RaceEvent{
						Type:    EventWinner,
						Entrant: label,
						Result:  &result,
					}
				}
			}
		}(entrant)
	}

	var routerWg sync.WaitGroup
	routerWg.Add(1)
	go func() {
		defer routerWg.Done()
		for token := range tokenChan {
			label := fmt.Sprintf("%s/%s", token.Provider, token.Model)

			mu.Lock()
			isFirst := !barrierSeen[label]
			mu.Unlock()

			if isFirst {
				barrierCheckin(label)
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
	routerWg.Wait() // drain all buffered tokens before closing eventChan

	// Adjust TotalTime for a fair comparison: measure each provider's elapsed
	// time from when the last provider produced its first token (raceStartTime)
	// so that spin-up latency is excluded from the race ranking.
	mu.Lock()
	var raceStartTime time.Time
	for i := range results {
		if !results[i].FirstTokenAt.IsZero() && results[i].FirstTokenAt.After(raceStartTime) {
			raceStartTime = results[i].FirstTokenAt
		}
	}
	if !raceStartTime.IsZero() {
		for i := range results {
			if !results[i].FirstTokenAt.IsZero() {
				// completionTime = requestStart + TotalTime
				//                = (FirstTokenAt − TTFT) + TotalTime
				completionTime := results[i].FirstTokenAt.Add(results[i].TotalTime - results[i].TTFT)
				adjusted := completionTime.Sub(raceStartTime)
				if adjusted < 0 {
					adjusted = 0
				}
				results[i].TotalTime = adjusted
			}
		}
	}
	mu.Unlock()

	sort.Slice(results, func(i, j int) bool {
		hasErr := func(r provider.Result) bool { return r.Err != nil || r.Canceled }
		if hasErr(results[i]) != hasErr(results[j]) {
			return !hasErr(results[i])
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
