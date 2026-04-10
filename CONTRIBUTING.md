# Contributing to RaceLLM

Thank you for your interest in contributing to RaceLLM! This guide will help you get started.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Setup](#development-setup)
- [Making Changes](#making-changes)
- [Coding Standards](#coding-standards)
- [Testing](#testing)
- [Submitting a Pull Request](#submitting-a-pull-request)
- [Reporting Issues](#reporting-issues)
- [Adding a New Provider](#adding-a-new-provider)

## Code of Conduct

This project follows the [Contributor Covenant Code of Conduct](CODE_OF_CONDUCT.md). By participating, you are expected to uphold this code. Please report unacceptable behavior to the maintainers.

## Getting Started

1. **Fork** the repository on GitHub.
2. **Clone** your fork locally:
   ```bash
   git clone https://github.com/<your-username>/racellm.git
   cd racellm
   ```
3. **Add the upstream remote:**
   ```bash
   git remote add upstream https://github.com/khuynh22/racellm.git
   ```

## Development Setup

### Prerequisites

- **Go 1.25+** — [Install Go](https://go.dev/dl/)
- **golangci-lint v2** — [Install golangci-lint](https://golangci-lint.run/welcome/install/)
- At least one LLM provider configured (see [API Keys guide](docs/api-keys.md))

### Build & Run

```bash
# Build the binary
go build -o racellm .

# Run directly
go run . "your prompt here"

# Install to $GOPATH/bin
go install .
```

### Using the Makefile

```bash
make build       # Build the binary
make test        # Run tests with race detector
make lint        # Run golangci-lint
make fmt         # Format code with goimports
make coverage    # Generate coverage report
make clean       # Remove build artifacts
make all         # lint + test + build
```

## Making Changes

1. **Create a branch** from `main`:
   ```bash
   git checkout -b feat/my-feature
   ```
   Use prefixes: `feat/`, `fix/`, `docs/`, `refactor/`, `test/`, `chore/`.

2. **Make your changes.** Keep commits focused and atomic.

3. **Run checks locally** before pushing:
   ```bash
   make lint
   make test
   ```

4. **Push** your branch and open a Pull Request.

## Coding Standards

- **Formatting:** All code must be `gofmt`-clean. Use `goimports` for import ordering.
- **Linter:** Code must pass `golangci-lint` with the project's [`.golangci.yml`](.golangci.yml) config. Run `make lint` before committing.
- **Error handling:** Wrap errors with context using `fmt.Errorf("context: %w", err)`. Never discard errors silently.
- **Context:** Every `Stream` call receives a `context.Context`. Respect cancellation and return immediately when `ctx.Done()` is closed.
- **Comments:** Exported identifiers must have a Go doc comment (`// TypeName ...`). Avoid inline comments that merely restate the code.
- **No hardcoded keys:** API keys must be resolved from environment variables via the `$ENV_VAR` syntax in YAML config.
- **`//nolint` directives:** Must include a reason, e.g., `//nolint:errcheck // fire-and-forget`.

## Testing

- Run the full test suite with the race detector:
  ```bash
  go test -race ./...
  ```
- Use **table-driven tests**. No external test dependencies beyond the standard library.
- If you add a new provider, include unit tests in `internal/provider/<name>_test.go`.
- Aim to maintain or improve test coverage.

## Submitting a Pull Request

1. Ensure all CI checks pass (lint, test, build).
2. Fill in the PR template — describe **what** changed and **why**.
3. Link any related issues (e.g., `Closes #42`).
4. Keep PRs focused. One logical change per PR.
5. Be responsive to review feedback.

### Commit Messages

Use clear, imperative-mood commit messages:

```
feat: add Mistral provider support
fix: handle empty SSE data lines in Anthropic stream
docs: add troubleshooting section for Ollama timeouts
test: add table-driven tests for config resolution
```

## Reporting Issues

- Use the **Bug Report** or **Feature Request** issue templates.
- Include your Go version (`go version`), OS, and RaceLLM version.
- For bugs, include steps to reproduce and the expected vs. actual behavior.

## Adding a New Provider

1. Create `internal/provider/<name>.go` implementing the `Provider` interface (`Name`, `Models`, `Stream`).
2. Create `internal/provider/<name>_test.go` with table-driven tests.
3. Add a `*ProviderEntry` field to `ProvidersConfig` in `internal/config/config.go`.
4. Add the builder block in `race.BuildEntrants`.
5. Call `resolve()` on the new entry in `config.resolveEnvKeys`.
6. Document the provider's streaming protocol in the copilot instructions.
7. Update `racellm.example.yaml` with a sample config block.

## Questions?

If you have questions that aren't answered here, feel free to open a [Discussion](https://github.com/khuynh22/racellm/discussions) or an issue.

Thank you for helping make RaceLLM better!
