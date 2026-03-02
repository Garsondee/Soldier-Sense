# just is a handy way to save and run project-specific commands.
# The book is at https://just.systems/man/en/
# Install it from https://github.com/casey/just/releases 
#   Windows: winget install --id=Casey.Just -e  (then use Git Bash as your shell)

set shell := ["sh", "-cu"]

[private]
default:
    @just --list

test: unittest lint fmt-check gosec tidy build

unittest:
    go test ./...

lint:
    #!/usr/bin/env sh
    echo "Running linter..."
    if command -v golangci-lint >/dev/null 2>&1; then
      golangci-lint run
    else
      echo "golangci-lint not found, falling back to go vet"
      echo "To install golangci-lint: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"
      go vet ./...
    fi

gosec:
    #!/usr/bin/env sh
    echo "Running security scanner..."
    if command -v gosec >/dev/null 2>&1; then
      gosec -quiet -fmt=text ./...
    else
      echo "gosec not found, skipping security scan"
      echo "To install gosec: go install github.com/securego/gosec/v2/cmd/gosec@latest"
    fi

# Install golangci-lint via go install (ensure ~/go/bin is in your PATH)
install-golangci-lint:
    go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

fmt:
    go fmt ./cmd/... ./internal/...

# Check formatting without modifying files
fmt-check:
    #!/usr/bin/env sh
    unformatted_files="$(gofmt -l ./cmd ./internal)"
    if [ -n "$unformatted_files" ]; then
      echo "Files need formatting:"
      echo "$unformatted_files"
      echo "Run 'just fmt' to fix formatting"
      exit 1
    fi
    echo "All files are properly formatted"

tidy:
    go mod tidy

build:
    #!/usr/bin/env sh
    mkdir -p bin
    case "$(uname -s)" in
      MINGW*|MSYS*|CYGWIN*) OUT=bin/soldier-sense.exe ;;
      *) OUT=bin/soldier-sense ;;
    esac
    go build -ldflags "-X github.com/Garsondee/Soldier-Sense/pkg/commands.Source=https://github.com/Garsondee/Soldier-Sense" -o "$OUT" ./cmd/game

run: build
    go run ./cmd/game

# Run headless mutual-advance simulation N times and print AAR-ready report lines.
# Supports overrides in KEY=VALUE form, e.g.:
#   just headless-report RUNS=20 TICKS=3600 SEED_BASE=42 SEED_STEP=1
headless-report *OVERRIDES:
    sh scripts/headless-report.sh {{OVERRIDES}}

install:
    go install ./cmd/soldier-sense

clean:
    rm -rf bin dist _build

# Initialize decision records
init-decisions:
    python3 scripts/decisions.py --init

# Add a new decision record
add-decision TOPIC:
    python3 scripts/decisions.py --add "{{TOPIC}}"

git-init:
    git init
    git add .gitignore .github *
    git commit -m "Initial commit"

