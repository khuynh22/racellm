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

// Gemini implements the Provider interface for Google's Gemini API.
type Gemini struct {
	Config ProviderConfig
}

// NewGemini creates a Gemini provider using the given config.
func NewGemini(cfg ProviderConfig) *Gemini {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://generativelanguage.googleapis.com/v1beta"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 60
	}
	return &Gemini{Config: cfg}
}

// Name returns the display name of the provider.
func (g *Gemini) Name() string { return "Gemini" }

// Models returns the list of supported model identifiers.
func (g *Gemini) Models() []string {
	return []string{"gemini-1.5-pro", "gemini-1.5-flash", "gemini-2.0-flash"}
}

type geminiRequest struct {
	Contents []geminiContent `json:"contents"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiStreamResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason,omitempty"`
	} `json:"candidates"`
}

// Stream sends prompt to the Gemini API and streams tokens onto tokenChan.
func (g *Gemini) Stream(ctx context.Context, model, prompt string, tokenChan chan<- Token) (Result, error) {
	startTime := time.Now()
	result := Result{Provider: g.Name(), Model: model}

	reqBody := geminiRequest{
		Contents: []geminiContent{
			{Parts: []geminiPart{{Text: prompt}}},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		result.Err = fmt.Errorf("marshal request: %w", err)
		return result, result.Err
	}

	url := fmt.Sprintf("%s/models/%s:streamGenerateContent?alt=sse&key=%s",
		g.Config.BaseURL, model, g.Config.APIKey)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		result.Err = fmt.Errorf("create request: %w", err)
		return result, result.Err
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: time.Duration(g.Config.Timeout) * time.Second}
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

		var chunk geminiStreamResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		streamDone := false
		for _, candidate := range chunk.Candidates {
			for _, part := range candidate.Content.Parts {
				text := part.Text
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
					Provider: g.Name(),
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
			if candidate.FinishReason != "" {
				streamDone = true
			}
		}
		if streamDone {
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
