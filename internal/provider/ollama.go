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

// Ollama implements the Provider interface for local Ollama models.
type Ollama struct {
	Config ProviderConfig
}

func NewOllama(cfg ProviderConfig) *Ollama {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://localhost:11434"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 120
	}
	return &Ollama{Config: cfg}
}

func (o *Ollama) Name() string { return "Ollama" }

func (o *Ollama) Models() []string {
	return []string{"llama3", "phi3", "mistral", "codellama"}
}

type ollamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type ollamaStreamChunk struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

func (o *Ollama) Stream(ctx context.Context, model, prompt string, tokenChan chan<- Token) (Result, error) {
	startTime := time.Now()
	result := Result{Provider: o.Name(), Model: model}

	reqBody := ollamaRequest{
		Model:  model,
		Prompt: prompt,
		Stream: true,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		result.Err = fmt.Errorf("marshal request: %w", err)
		return result, result.Err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		o.Config.BaseURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		result.Err = fmt.Errorf("create request: %w", err)
		return result, result.Err
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: time.Duration(o.Config.Timeout) * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		result.Err = fmt.Errorf("execute request: %w", err)
		return result, result.Err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096)) //nolint:errcheck // best-effort error body read
		result.Err = fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(respBody))
		return result, result.Err
	}

	var fullText strings.Builder
	tokenIndex := 0
	scanner := bufio.NewScanner(resp.Body)

	// Ollama streams newline-delimited JSON (not SSE).
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
		if line == "" {
			continue
		}

		var chunk ollamaStreamChunk
		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			continue
		}

		if chunk.Done {
			break
		}

		text := chunk.Response
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

	result.FullText = fullText.String()
	result.TokenCount = tokenIndex
	result.TotalTime = time.Since(startTime)
	return result, nil
}
