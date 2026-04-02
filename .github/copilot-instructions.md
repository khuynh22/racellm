# RaceLLM — Copilot Instructions

## Project Overview

RaceLLM is a Go CLI that fans out a single prompt to multiple LLM providers simultaneously, streams all responses in parallel, and surfaces timing results through a BubbleTea TUI. The two race modes are `all` (wait for every provider) and `fastest` (cancel the rest once the first completes).

## Package Layout

| Package | Responsibility |
|---|---|
| `cmd` | Cobra CLI entry points (`racellm`, `models`, `version`) |
| `internal/config` | YAML config loading with `$ENV_VAR` key resolution |
| `internal/provider` | `Provider` interface + per-backend implementations (OpenAI, Anthropic, Gemini, Ollama) |
| `internal/coordinator` | Fan-out orchestration, `RaceEvent` channel, result aggregation |
| `internal/race` | Wires config → entrants → coordinator → TUI |
| `internal/tui` | BubbleTea live dashboard (Model / Init / Update / View) |

## Architecture

```
prompt
  └─► coordinator.Race()
        ├─► goroutine: OpenAI/gpt-4o      ──┐
        ├─► goroutine: Anthropic/claude   ──┤─► tokenChan ──► RaceEvent channel ──► TUI
        └─► goroutine: Gemini/gemini-pro  ──┘
```

- Each provider goroutine calls `Provider.Stream()`, writing `Token` values to a shared buffered channel.
- A router goroutine forwards tokens to the `RaceEvent` channel and emits `EventFirst` once per racer.
- In `ModeFastest`, the coordinator calls `cancel()` as soon as one racer succeeds.
- Results are sorted by `TotalTime` (errors go last) before being sent on `resultsChan`.

## Adding a New Provider

1. Create `internal/provider/<name>.go` implementing `Provider` (`Name`, `Models`, `Stream`).
2. Add a `*ProviderEntry` field to `ProvidersConfig` in `internal/config/config.go`.
3. Add the builder block in `race.BuildEntrants`.
4. Call `resolve()` on the new entry in `config.resolveEnvKeys`.

## Key Conventions

- **Go version**: `go 1.25` (see `go.mod`). Use standard library idioms.
- **Error handling**: Wrap with `fmt.Errorf("context: %w", err)`. Never discard errors silently.
- **Context**: Every `Stream` call receives a `context.Context`. Respect cancellation; return immediately when `ctx.Done()` is closed.
- **Linter**: `golangci-lint` with the config in `.golangci.yml`. Run before committing. Inline `//nolint` directives must include a reason.
- **Formatting**: `goimports`. All files must be `gofmt`-clean.
- **Comments**: Exported identifiers must have a Go doc comment (`// TypeName ...`). Do not add inline comments that restate what the code clearly does.
- **Tests**: `go test -race ./...`. Use table-driven tests. No external test dependencies beyond the standard library.
- **Config keys**: Never hard-code API keys. Keys prefixed with `$` in YAML are resolved from environment variables by `resolveEnvKeys`.

## Streaming Protocol per Provider

| Provider | Protocol | Terminator |
|---|---|---|
| OpenAI | SSE (`data: ...`) | `data: [DONE]` |
| Anthropic | SSE (`data: ...`) | event type `message_stop` |
| Gemini | SSE (`data: ...`) | `finishReason` present |
| Ollama | Newline-delimited JSON | `"done": true` |

## TUI State Machine

`racerStatus` transitions: `statusRunning` → `statusWinner` or `statusFinished` or `statusError`.
The `Model.done` flag is set when `resultsMsg` arrives; the program then calls `tea.Quit`.
