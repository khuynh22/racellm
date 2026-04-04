package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Anthropic implements the Provider interface for the Anthropic (Claude) API.
type Anthropic struct {
	Config ProviderConfig
}

// NewAnthropic creates an Anthropic provider using the given config.
func NewAnthropic(cfg ProviderConfig) *Anthropic {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.anthropic.com/v1"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 60
	}
	return &Anthropic{Config: cfg}
}

// Name returns the display name of the provider.
func (a *Anthropic) Name() string { return "Anthropic" }

// Models returns the list of supported model identifiers.
func (a *Anthropic) Models() []string {
	return []string{"claude-sonnet-4-20250514", "claude-3-5-haiku-20241022", "claude-3-opus-20240229"}
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	Messages  []anthropicMessage `json:"messages"`
	Stream    bool               `json:"stream"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicStreamEvent struct {
	Type  string `json:"type"`
	Delta *struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"delta,omitempty"`
}

// Stream sends prompt to the Anthropic API and streams tokens onto tokenChan.
func (a *Anthropic) Stream(ctx context.Context, model, prompt string, tokenChan chan<- Token) (Result, error) {
	startTime := time.Now()
	result := Result{Provider: a.Name(), Model: model}

	reqBody := anthropicRequest{
		Model:     model,
		MaxTokens: 4096,
		Messages:  []anthropicMessage{{Role: "user", Content: prompt}},
		Stream:    true,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		result.Err = fmt.Errorf("marshal request: %w", err)
		return result, result.Err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		a.Config.BaseURL+"/messages", bytes.NewReader(body))
	if err != nil {
		result.Err = fmt.Errorf("create request: %w", err)
		return result, result.Err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", a.Config.APIKey)
	req.Header.Set("Anthropic-Version", "2023-06-01")

	client := &http.Client{Timeout: time.Duration(a.Config.Timeout) * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		result.Err = fmt.Errorf("execute request: %w", err)
		return result, result.Err
	}
	defer resp.Body.Close() // nolint:errcheck // response body Close error is not actionable in a deferred call

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096)) //nolint:errcheck // best-effort error body read
		result.Err = fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(respBody))
		return result, result.Err
	}

	var fullText strings.Builder
	tokenIndex := 0
	scanner := bufio.NewScanner(resp.Body)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			result.Err = ctx.Err()
			result.FullText = fullText.String()
			result.TokenCount = tokenIndex
			result.TotalTime = time.Since(startTime)
			return result, result.Err
		default:
		}

		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		var event anthropicStreamEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		if event.Type == "content_block_delta" && event.Delta != nil && event.Delta.Text != "" {
			text := event.Delta.Text

			if tokenIndex == 0 {
				result.FirstTokenAt = time.Now()
				result.TTFT = time.Since(startTime)
			}

			fullText.WriteString(text)
			tokenIndex++

			select {
			case tokenChan <- Token{
				Provider: a.Name(),
				Model:    model,
				Text:     text,
				Index:    tokenIndex,
			}:
			case <-ctx.Done():
				result.Err = ctx.Err()
				result.FullText = fullText.String()
				result.TokenCount = tokenIndex
				result.TotalTime = time.Since(startTime)
				return result, result.Err
			}
		}

		if event.Type == "message_stop" {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		result.Err = fmt.Errorf("read response stream: %w", err)
		result.FullText = fullText.String()
		result.TokenCount = tokenIndex
		result.TotalTime = time.Since(startTime)
		return result, result.Err
	}

	result.FullText = fullText.String()
	result.TokenCount = tokenIndex
	result.TotalTime = time.Since(startTime)
	return result, nil
}
