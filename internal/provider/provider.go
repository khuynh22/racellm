// Package provider defines the Provider interface and shared types used by all LLM backends.
package provider

import (
	"context"
	"time"
)

// Token represents a single streamed token from a model.
type Token struct {
	Provider string
	Model    string
	Text     string
	Index    int
}

// Result is the final output from a provider after streaming completes.
type Result struct {
	Provider     string
	Model        string
	FullText     string
	TokenCount   int
	FirstTokenAt time.Time
	TTFT         time.Duration
	TotalTime    time.Duration
	Err          error
	// Canceled is true when the provider was stopped because another racer
	// won in ModeFastest; it is distinct from a genuine API error.
	Canceled bool
}

// Provider is the interface that all AI model backends must implement.
type Provider interface {
	// Name returns the provider's display name (e.g., "OpenAI", "Anthropic").
	Name() string

	// Models returns the list of model IDs this provider supports.
	Models() []string

	// Stream sends a prompt to the specified model, streaming tokens to tokenChan.
	// It respects context cancellation and stops immediately when ctx.Done() is closed.
	// The returned Result contains the full response and timing metadata.
	Stream(ctx context.Context, model string, prompt string, tokenChan chan<- Token) (Result, error)
}

// ProviderConfig holds the common configuration for a provider.
//
//nolint:revive // ProviderConfig is intentionally named; it's clearer than Config when embedded.
type ProviderConfig struct {
	APIKey  string `yaml:"api_key"`
	BaseURL string `yaml:"base_url"`
	Timeout int    `yaml:"timeout_seconds"`
}
