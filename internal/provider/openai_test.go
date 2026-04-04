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

func TestOpenAI_Stream_Success(t *testing.T) {
	events := []string{
		`data: {"choices":[{"delta":{"content":"Hello"},"finish_reason":null}]}`,
		`data: {"choices":[{"delta":{"content":", world"},"finish_reason":null}]}`,
		`data: [DONE]`,
	}
	srv := newSSEServer(t, events)
	defer srv.Close()

	p := NewOpenAI(ProviderConfig{APIKey: "test-key", BaseURL: srv.URL + "/v1"})
	tokenChan := make(chan Token, 10)

	result, err := p.Stream(context.Background(), "gpt-4o", "hi", tokenChan)
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

func TestOpenAI_Stream_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":{"message":"invalid api key"}}`)
	}))
	defer srv.Close()

	p := NewOpenAI(ProviderConfig{APIKey: "bad-key", BaseURL: srv.URL + "/v1"})
	tokenChan := make(chan Token, 10)

	result, err := p.Stream(context.Background(), "gpt-4o", "hi", tokenChan)
	close(tokenChan)

	if err == nil {
		t.Fatal("Stream() expected error for non-200 status, got nil")
	}
	if result.Err == nil {
		t.Error("result.Err should be set")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error should mention status 401, got: %v", err)
	}
}

func TestOpenAI_Stream_ContextCanceled(t *testing.T) {
	// Server sends tokens slowly so we can cancel mid-stream.
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
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"tok%d\"},\"finish_reason\":null}]}\n\n", i)
			flusher.Flush()
			time.Sleep(20 * time.Millisecond)
		}
		fmt.Fprintln(w, "data: [DONE]")
		flusher.Flush()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	p := NewOpenAI(ProviderConfig{APIKey: "test-key", BaseURL: srv.URL + "/v1", Timeout: 5})
	tokenChan := make(chan Token, 100)

	go func() {
		time.Sleep(40 * time.Millisecond)
		cancel()
	}()

	result, err := p.Stream(ctx, "gpt-4o", "hi", tokenChan)
	close(tokenChan)

	// After context cancellation the provider should return quickly.
	if err == nil && !result.Canceled {
		t.Error("expected cancellation signal, got neither err nor Canceled")
	}
}

func TestOpenAI_Name(t *testing.T) {
	p := NewOpenAI(ProviderConfig{})
	if p.Name() != "OpenAI" {
		t.Errorf("Name() = %q, want %q", p.Name(), "OpenAI")
	}
}

func TestOpenAI_Models(t *testing.T) {
	p := NewOpenAI(ProviderConfig{})
	if len(p.Models()) == 0 {
		t.Error("Models() should return a non-empty list")
	}
}

// newSSEServer creates an httptest.Server that writes SSE event lines and flushes after each.
func newSSEServer(t *testing.T, events []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("ResponseWriter does not implement http.Flusher")
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, e := range events {
			fmt.Fprintln(w, e)
			fmt.Fprintln(w) // blank line between SSE events
			flusher.Flush()
		}
	}))
}
