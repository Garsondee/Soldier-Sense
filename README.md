# Soldier-Sense - 0.1.0

Soldier-Sense is an emergent behaviour simulation in the domain of urban warfare.

This repository contains the interactive simulation, headless reporting tools, and
analysis utilities used to iterate on squad tactics, combat behaviour, and
performance.

## Requirements

- Go 1.24.0 (see `go.mod`).
- `just` command runner.
- Git Bash on Windows for Justfile recipes.

## Quick Start

1. Install tooling:

   ```bash
   just setup
   ```

2. Run all project checks:

   ```bash
   just test
   ```

3. Launch the interactive simulation:

   ```bash
   just run
   ```

For full environment details, see `docs/development-setup.md`.

## Common Commands

Use `just` from the repository root.

```bash
just test            # unit tests + lint + format check + gosec + tidy + build
just unittest        # go test ./...
just lint            # golangci-lint (or go vet fallback)
just fmt             # go fmt ./cmd/... ./internal/...
just fmt-check       # check formatting without modifying files
just build           # build interactive game binary to ./bin/
just run             # run interactive game via go run ./cmd/game
just headless-report RUNS=20 TICKS=3600 SEED_BASE=42 SEED_STEP=1
```

## Executables

The `cmd/` directory contains project entry points:

- `cmd/game`: interactive Soldier-Sense client.
- `cmd/headless-report`: repeated headless scenario runs with aggregate reporting.
- `cmd/laboratory`: visual laboratory harness for named scenario tests.
- `cmd/trait-test`: headless trait benchmarking between genome profiles.
- `cmd/evolve`: evolutionary optimisation for soldier traits.
- `cmd/analyze`: sensitivity analysis for evolution logs.

Run commands with `go run ./cmd/<name>` to ensure you are running current source.

## Repository Structure

```text
.
├── cmd/                # CLI and executable entry points
├── internal/game/      # Core simulation/game logic
├── docs/               # Development and design documentation
├── design/             # Design notes, systems docs, and plans
├── scripts/            # Helper scripts for reporting and setup
├── Justfile            # Project task runner commands
└── WINDSURF.md         # Project collaboration and coding guidelines
```

## Development Notes

- Follow the guidance in `WINDSURF.md` for coding, style, and collaboration.
- Use UK English spelling in documentation and comments.
- Prefer `go run ./cmd/<program>` when testing behaviour.

