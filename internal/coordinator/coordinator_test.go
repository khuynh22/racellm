package coordinator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/khuynh22/racellm/internal/provider"
)

// mockProvider is a controllable Provider for testing.
type mockProvider struct {
	name   string
	tokens []string
	delay  time.Duration // delay between each token
	err    error         // if non-nil, returned immediately with no tokens
}

func (m *mockProvider) Name() string     { return m.name }
func (m *mockProvider) Models() []string { return []string{"mock-model"} }
func (m *mockProvider) Stream(ctx context.Context, model, _ string, tokenChan chan<- provider.Token) (provider.Result, error) {
	result := provider.Result{Provider: m.name, Model: model}
	startTime := time.Now()

	if m.err != nil {
		result.Err = m.err
		return result, m.err
	}

	var sb strings.Builder
	for i, tok := range m.tokens {
		if m.delay > 0 {
			select {
			case <-ctx.Done():
				result.Err = ctx.Err()
				result.FullText = sb.String()
				result.TokenCount = i
				result.TotalTime = time.Since(startTime)
				return result, result.Err
			case <-time.After(m.delay):
			}
		}

		select {
		case <-ctx.Done():
			result.Err = ctx.Err()
			result.FullText = sb.String()
			result.TokenCount = i
			result.TotalTime = time.Since(startTime)
			return result, result.Err
		default:
		}

		if i == 0 {
			result.FirstTokenAt = time.Now()
			result.TTFT = time.Since(startTime)
		}
		sb.WriteString(tok)
		select {
		case tokenChan <- provider.Token{Provider: m.name, Model: model, Text: tok, Index: i + 1}:
		case <-ctx.Done():
			result.Err = ctx.Err()
			result.FullText = sb.String()
			result.TokenCount = i + 1
			result.TotalTime = time.Since(startTime)
			return result, result.Err
		}
	}

	result.FullText = sb.String()
	result.TokenCount = len(m.tokens)
	result.TotalTime = time.Since(startTime)
	return result, nil
}

func entrant(p *mockProvider) Entrant {
	return Entrant{Provider: p, Model: "mock-model"}
}

// drainEvents reads all events from eventChan until it is closed.
func drainEvents(eventChan <-chan RaceEvent) []RaceEvent {
	var events []RaceEvent
	for e := range eventChan {
		events = append(events, e)
	}
	return events
}

func TestModeAll_BothSucceed(t *testing.T) {
	fast := &mockProvider{name: "fast", tokens: []string{"hello", " world"}}
	slow := &mockProvider{name: "slow", tokens: []string{"hi"}, delay: 20 * time.Millisecond}

	c := New([]Entrant{entrant(fast), entrant(slow)}, ModeAll)
	eventChan, resultsChan := c.Race(context.Background(), "test prompt")

	drainEvents(eventChan)
	results := <-resultsChan

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	// Both must succeed.
	for _, r := range results {
		if r.Err != nil {
			t.Errorf("result %s/%s has unexpected error: %v", r.Provider, r.Model, r.Err)
		}
		if r.Canceled {
			t.Errorf("result %s/%s unexpectedly canceled in ModeAll", r.Provider, r.Model)
		}
	}
	// Fast provider should be first (lower TotalTime).
	if results[0].Provider != "fast" {
		t.Errorf("expected fast provider first, got %q", results[0].Provider)
	}
}

func TestModeFastest_SlowerGetsCanceled(t *testing.T) {
	fast := &mockProvider{name: "fast", tokens: []string{"go"}}
	slow := &mockProvider{name: "slow", tokens: []string{"a", "b", "c", "d", "e"}, delay: 30 * time.Millisecond}

	c := New([]Entrant{entrant(fast), entrant(slow)}, ModeFastest)
	eventChan, resultsChan := c.Race(context.Background(), "test prompt")

	events := drainEvents(eventChan)
	results := <-resultsChan

	// Exactly one EventWinner must be emitted.
	var winners []string
	for _, e := range events {
		if e.Type == EventWinner {
			winners = append(winners, e.Entrant)
		}
	}
	if len(winners) != 1 {
		t.Errorf("expected 1 EventWinner, got %d: %v", len(winners), winners)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	// Winner first, canceled second.
	if results[0].Provider != "fast" {
		t.Errorf("expected fast provider as winner, got %q", results[0].Provider)
	}
	if !results[1].Canceled {
		t.Errorf("expected slow provider to be canceled, got Canceled=%v, Err=%v", results[1].Canceled, results[1].Err)
	}
}

func TestModeAll_OneErrors(t *testing.T) {
	good := &mockProvider{name: "good", tokens: []string{"ok"}}
	bad := &mockProvider{name: "bad", err: errors.New("api failure")}

	c := New([]Entrant{entrant(good), entrant(bad)}, ModeAll)
	eventChan, resultsChan := c.Race(context.Background(), "test prompt")

	events := drainEvents(eventChan)
	results := <-resultsChan

	// One EventError expected.
	var errEvents int
	for _, e := range events {
		if e.Type == EventError {
			errEvents++
		}
	}
	if errEvents != 1 {
		t.Errorf("expected 1 EventError, got %d", errEvents)
	}

	// Successful result first, error last.
	if results[0].Provider != "good" || results[0].Err != nil {
		t.Errorf("expected good provider first with no error, got %+v", results[0])
	}
	if results[1].Provider != "bad" || results[1].Err == nil {
		t.Errorf("expected bad provider last with error, got %+v", results[1])
	}
}

func TestModeAll_AllError(t *testing.T) {
	a := &mockProvider{name: "a", err: errors.New("err a")}
	b := &mockProvider{name: "b", err: errors.New("err b")}

	c := New([]Entrant{entrant(a), entrant(b)}, ModeAll)
	eventChan, resultsChan := c.Race(context.Background(), "test prompt")

	drainEvents(eventChan)
	results := <-resultsChan

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if r.Err == nil {
			t.Errorf("result %s should have an error", r.Provider)
		}
	}
}

func TestContextCancelStopsRace(t *testing.T) {
	slow := &mockProvider{name: "slow", tokens: []string{"a", "b", "c"}, delay: 50 * time.Millisecond}

	ctx, cancel := context.WithCancel(context.Background())
	c := New([]Entrant{entrant(slow)}, ModeAll)
	eventChan, resultsChan := c.Race(ctx, "test")

	// Cancel before the slow provider finishes.
	cancel()

	drainEvents(eventChan)
	results := <-resultsChan

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Err != nil {
		t.Errorf("expected Err=nil (canceled=true), got Err=%v", results[0].Err)
	}
	if !results[0].Canceled {
		t.Errorf("expected Canceled=true for externally canceled racer")
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		input time.Duration
		want  string
	}{
		{500 * time.Millisecond, "500ms"},
		{999 * time.Millisecond, "999ms"},
		{1 * time.Second, "1.00s"},
		{1500 * time.Millisecond, "1.50s"},
		{2*time.Second + 345*time.Millisecond, "2.34s"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%v", tt.input), func(t *testing.T) {
			if got := FormatDuration(tt.input); got != tt.want {
				t.Errorf("FormatDuration(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
