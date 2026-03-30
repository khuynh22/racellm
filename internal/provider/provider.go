package provider

import (
	"context"
	"time"
)

// Token represents a single streamed token from a model.
type Token struct {
	Provider string // Which provider sent this token
	Model    string // Model name (e.g., "gpt-4o", "claude-sonnet-4-20250514")
	Text     string // The token text
	Index    int    // Token sequence number
}

// Result is the final output from a provider after streaming completes.
type Result struct {
	Provider     string
	Model        string
	FullText     string
	TokenCount   int
	FirstTokenAt time.Time     // When the first token arrived
	TTFT         time.Duration // Time To First Token
	TotalTime    time.Duration
	Err          error
}

// Provider is the interface that all AI model backends must implement.
// Each provider sends tokens on the tokenChan as they stream in,
// and returns a Result when the stream is complete or canceled.
type Provider interface {
	// Name returns the provider's display name (e.g., "OpenAI", "Anthropic").
	Name() string

	// Models returns the list of model IDs this provider supports.
	Models() []string

	// Stream sends a prompt to the specified model, streaming tokens to tokenChan.
	// It must respect context cancellation and stop streaming immediately when
	// ctx.Done() is signaled. The returned Result contains the full response
	// and timing metadata.
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
