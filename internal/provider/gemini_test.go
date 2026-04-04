package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGemini_Stream_Success(t *testing.T) {
	events := []string{
		`data: {"candidates":[{"content":{"parts":[{"text":"Hello"}]},"finishReason":""}]}`,
		`data: {"candidates":[{"content":{"parts":[{"text":", world"}]},"finishReason":"STOP"}]}`,
	}
	srv := newSSEServer(t, events)
	defer srv.Close()

	p := NewGemini(ProviderConfig{APIKey: "test-key", BaseURL: srv.URL + "/v1beta"})
	tokenChan := make(chan Token, 10)

	result, err := p.Stream(context.Background(), "gemini-2.0-flash", "hi", tokenChan)
	close(tokenChan)

	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if result.FullText != "Hello, world" {
		t.Errorf("FullText = %q, want %q", result.FullText, "Hello, world")
	}
	if result.TokenCount != 2 {
		t.Errorf("TokenCount = %d, want 2", result.TokenCount)
	}
	if result.TTFT == 0 {
		t.Error("TTFT should be non-zero")
	}

	var tokens []Token
	for tok := range tokenChan {
		tokens = append(tokens, tok)
	}
	if len(tokens) != 2 {
		t.Errorf("expected 2 tokens on channel, got %d", len(tokens))
	}
}

func TestGemini_Stream_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"error":{"message":"API key not valid"}}`)
	}))
	defer srv.Close()

	p := NewGemini(ProviderConfig{APIKey: "bad-key", BaseURL: srv.URL + "/v1beta"})
	tokenChan := make(chan Token, 10)

	result, err := p.Stream(context.Background(), "gemini-2.0-flash", "hi", tokenChan)
	close(tokenChan)

	if err == nil {
		t.Fatal("Stream() expected error for non-200 status, got nil")
	}
	if result.Err == nil {
		t.Error("result.Err should be set")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error should mention status 403, got: %v", err)
	}
}

func TestGemini_Stream_ContextCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("ResponseWriter does not implement http.Flusher")
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for i := range 20 {
			select {
			case <-r.Context().Done():
				return
			default:
			}
			fmt.Fprintf(w, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"tok%d\"}]},\"finishReason\":\"\"}]}\n\n", i)
			flusher.Flush()
			time.Sleep(20 * time.Millisecond)
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	p := NewGemini(ProviderConfig{APIKey: "test-key", BaseURL: srv.URL + "/v1beta", Timeout: 5})
	tokenChan := make(chan Token, 100)

	go func() {
		time.Sleep(40 * time.Millisecond)
		cancel()
	}()

	result, err := p.Stream(ctx, "gemini-2.0-flash", "hi", tokenChan)
	close(tokenChan)

	if err == nil && !result.Canceled {
		t.Error("expected cancellation signal, got neither err nor Canceled")
	}
}

func TestGemini_Name(t *testing.T) {
	p := NewGemini(ProviderConfig{})
	if p.Name() != "Gemini" {
		t.Errorf("Name() = %q, want %q", p.Name(), "Gemini")
	}
}

func TestGemini_Models(t *testing.T) {
	p := NewGemini(ProviderConfig{})
	if len(p.Models()) == 0 {
		t.Error("Models() should return a non-empty list")
	}
}
