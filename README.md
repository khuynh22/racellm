# RaceLLM 🏁

> Race your LLMs — fire one prompt at multiple AI models simultaneously and get the fastest response.

RaceLLM is a high-concurrency Go CLI that sends your prompt to every configured model (OpenAI, Anthropic, Gemini, Ollama) at once, streams results in parallel, and declares a winner.

## Why?

- **Fan-Out Concurrency:** 1 prompt → $N$ goroutines → $N$ API calls simultaneously.
- **Streaming Aggregation:** Handles multiple incoming SSE token streams without blocking.
- **Graceful Cancellation:** `context.Context` kills losing connections the instant a winner is declared - saving API costs and CPU.

## Quick Start

```bash
# 1. Install
go install github.com/khang/racellm@latest

# 2. Configure
cp racellm.example.yaml ~/.racellm.yaml
# Edit ~/.racellm.yaml with your API keys (or set env vars)

# 3. Race!
racellm "Explain goroutines in Go"
```

## Usage

```bash
# Race all configured models, wait for everyone
racellm "What is the meaning of life?"

# Race in fastest mode — cancel losers as soon as a winner finishes
racellm "Write a regex for email" --mode fastest

# Use a specific config file
racellm --config ./myconfig.yaml "Hello world"

# List configured models
racellm models

# Version
racellm version
```

## Configuration

Create `~/.racellm.yaml` (or `racellm.yaml` in the current directory):

```yaml
default_mode: all  # "fastest" or "all"

providers:
  openai:
    enabled: true
    api_key: "$OPENAI_API_KEY"   # resolves env var
    models:
      - gpt-4o
      - gpt-4o-mini

  anthropic:
    enabled: true
    api_key: "$ANTHROPIC_API_KEY"
    models:
      - claude-sonnet-4-20250514

  gemini:
    enabled: false
    api_key: "$GEMINI_API_KEY"
    models:
      - gemini-2.0-flash

  ollama:
    enabled: false
    models:
      - llama3
```

API keys prefixed with `$` are automatically resolved from environment variables.

## Architecture

```
User Prompt
     │
     ▼
┌──────────┐      ┌───────────────────────────────────────────┐
│ CLI      │────▶│  Coordinator                               │
│ (Cobra)  │      │                                           │
└──────────┘      │  ┌─── go OpenAI.Stream() ──▶ tokenChan    │
                  │  ├─── go Anthropic.Stream() ──▶ tokenChan │
                  │  ├─── go Gemini.Stream() ──▶ tokenChan    │
                  │  └─── go Ollama.Stream() ──▶ tokenChan    │
                  │                                           │
                  │  ctx.Cancel() ◀── winner detected         │
                  └───────────────────────────────────────────┘
                           │
                           ▼
                    Sorted Results + Scoreboard
```

### Key Components

| Component | Package | Responsibility |
|---|---|---|
| **Provider Interface** | `internal/provider` | Common contract for all AI backends |
| **OpenAI / Anthropic / Gemini / Ollama** | `internal/provider` | SSE stream parsing per API format |
| **Coordinator** | `internal/coordinator` | Fan-out goroutines, channel aggregation, cancellation |
| **Race Runner** | `internal/race` | Builds entrants from config, prints live events + scoreboard |
| **CLI** | `cmd` | Cobra-based command tree, flag parsing, signal handling |
| **Config** | `internal/config` | YAML loading, env var resolution |

### Concurrency Primitives Used

- **`sync.WaitGroup`** — wait for all racers to finish
- **`context.WithCancel`** — cancel losing goroutines in fastest mode
- **`chan Token`** — shared channel for streaming token fan-in
- **`chan RaceEvent`** — event bus from coordinator to UI layer
- **`sync.Mutex`** — protect shared result slice and winner flag
- **`signal.NotifyContext`** — graceful OS signal handling (Ctrl+C)

## Project Structure

```
racellm/
├── main.go                          # Entry point
├── go.mod
├── cmd/
│   └── root.go                      # Cobra CLI commands
├── internal/
│   ├── config/
│   │   └── config.go                # YAML config + env var resolution
│   ├── coordinator/
│   │   └── coordinator.go           # Fan-out, channels, race logic
│   ├── provider/
│   │   ├── provider.go              # Provider interface + types
│   │   ├── openai.go                # OpenAI streaming
│   │   ├── anthropic.go             # Anthropic streaming
│   │   ├── gemini.go                # Google Gemini streaming
│   │   └── ollama.go                # Local Ollama streaming
│   └── race/
│       └── race.go                  # Race runner + result printing
└── racellm.example.yaml             # Example config
```

## Development Milestones

- [x] **Milestone 1 — The Basic Sprint:** Provider interface, concurrent requests, console output
- [x] **Milestone 2 — The Kill Switch:** `context.WithCancel`, goroutine leak prevention
- [ ] **Milestone 3 — The Live Dashboard:** BubbleTea TUI with horse-race visualizer

## Building

```bash
go build -o racellm .
```

## License

MIT
