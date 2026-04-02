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

// OpenAI implements the Provider interface for the OpenAI API.
type OpenAI struct {
	Config ProviderConfig
}

// NewOpenAI creates an OpenAI provider using the given config.
func NewOpenAI(cfg ProviderConfig) *OpenAI {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com/v1"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 60
	}
	return &OpenAI{Config: cfg}
}

// Name returns the display name of the provider.
func (o *OpenAI) Name() string { return "OpenAI" }

// Models returns the list of supported model identifiers.
func (o *OpenAI) Models() []string {
	return []string{"gpt-4o", "gpt-4o-mini", "gpt-4-turbo", "gpt-3.5-turbo"}
}

type openAIRequest struct {
	Model    string          `json:"model"`
	Messages []openAIMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

// Stream sends prompt to the OpenAI API and streams tokens onto tokenChan.
func (o *OpenAI) Stream(ctx context.Context, model, prompt string, tokenChan chan<- Token) (Result, error) {
	startTime := time.Now()
	result := Result{Provider: o.Name(), Model: model}

	reqBody := openAIRequest{
		Model:    model,
		Messages: []openAIMessage{{Role: "user", Content: prompt}},
		Stream:   true,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		result.Err = fmt.Errorf("marshal request: %w", err)
		return result, result.Err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		o.Config.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		result.Err = fmt.Errorf("create request: %w", err)
		return result, result.Err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.Config.APIKey)

	client := &http.Client{Timeout: time.Duration(o.Config.Timeout) * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		result.Err = fmt.Errorf("execute request: %w", err)
		return result, result.Err
	}
	defer resp.Body.Close() //nolint:errcheck // response body Close error is not actionable in a deferred call

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096)) // nolint:errcheck // best-effort error body read
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
		if data == "[DONE]" {
			break
		}

		var chunk openAIStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		for _, choice := range chunk.Choices {
			text := choice.Delta.Content
			if text == "" {
				continue
			}

			if tokenIndex == 0 {
				result.FirstTokenAt = time.Now()
				result.TTFT = time.Since(startTime)
			}

			fullText.WriteString(text)
			tokenIndex++

			select {
			case tokenChan <- Token{
				Provider: o.Name(),
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
	}

	result.FullText = fullText.String()
	result.TokenCount = tokenIndex
	result.TotalTime = time.Since(startTime)
	return result, nil
}
