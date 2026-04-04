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

func TestOllama_Stream_Success(t *testing.T) {
	// Ollama uses newline-delimited JSON, not SSE.
	lines := []string{
		`{"response":"Hello","done":false}`,
		`{"response":", world","done":false}`,
		`{"response":"","done":true}`,
	}
	srv := newNDJSONServer(t, lines)
	defer srv.Close()

	p := NewOllama(ProviderConfig{BaseURL: srv.URL})
	tokenChan := make(chan Token, 10)

	result, err := p.Stream(context.Background(), "llama3", "hi", tokenChan)
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

func TestOllama_Stream_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"error":"model not found"}`)
	}))
	defer srv.Close()

	p := NewOllama(ProviderConfig{BaseURL: srv.URL})
	tokenChan := make(chan Token, 10)

	result, err := p.Stream(context.Background(), "no-such-model", "hi", tokenChan)
	close(tokenChan)

	if err == nil {
		t.Fatal("Stream() expected error for non-200 status, got nil")
	}
	if result.Err == nil {
		t.Error("result.Err should be set")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error should mention status 404, got: %v", err)
	}
}

func TestOllama_Stream_ContextCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("ResponseWriter does not implement http.Flusher")
			return
		}
		for i := range 20 {
			select {
			case <-r.Context().Done():
				return
			default:
			}
			fmt.Fprintf(w, "{\"response\":\"tok%d\",\"done\":false}\n", i)
			flusher.Flush()
			time.Sleep(20 * time.Millisecond)
		}
		fmt.Fprintln(w, `{"response":"","done":true}`)
		flusher.Flush()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	p := NewOllama(ProviderConfig{BaseURL: srv.URL, Timeout: 5})
	tokenChan := make(chan Token, 100)

	go func() {
		time.Sleep(40 * time.Millisecond)
		cancel()
	}()

	result, err := p.Stream(ctx, "llama3", "hi", tokenChan)
	close(tokenChan)

	if err == nil && !result.Canceled {
		t.Error("expected cancellation signal, got neither err nor Canceled")
	}
}

func TestOllama_Name(t *testing.T) {
	p := NewOllama(ProviderConfig{})
	if p.Name() != "Ollama" {
		t.Errorf("Name() = %q, want %q", p.Name(), "Ollama")
	}
}

func TestOllama_Models(t *testing.T) {
	p := NewOllama(ProviderConfig{})
	if len(p.Models()) == 0 {
		t.Error("Models() should return a non-empty list")
	}
}

// newNDJSONServer creates a test server that writes newline-delimited JSON lines.
func newNDJSONServer(t *testing.T, lines []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("ResponseWriter does not implement http.Flusher")
			return
		}
		for _, line := range lines {
			fmt.Fprintln(w, line)
			flusher.Flush()
		}
	}))
}
