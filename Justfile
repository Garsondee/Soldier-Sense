# just is a handy way to save and run project-specific commands.
# The book is at https://just.systems/man/en/
# Install it from https://github.com/casey/just/releases 
#   Windows: winget install --id=Casey.Just -e 

set shell := ["sh", "-cu"]
set windows-shell := ["powershell.exe", "-NoLogo", "-NoProfile", "-Command"]

[private]
default:
    @just --list

test: unittest lint fmt-check gosec tidy build

unittest:
    go test ./...

lint:
    @just lint-{{os()}}

lint-windows:
    @& { Write-Output "Running linter..."; $userHome = [Environment]::GetFolderPath('UserProfile'); $toolDirs = @((Join-Path $userHome "tools/ext/bin"), (Join-Path $userHome "go/bin")); foreach ($dir in ($toolDirs | Select-Object -Unique)) { if (Test-Path $dir) { $env:PATH = "$dir;$env:PATH" } }; $cmd = Get-Command "golangci-lint" -ErrorAction SilentlyContinue; if ($cmd) { & $cmd.Source run; exit $LASTEXITCODE }; Write-Output "golangci-lint not found, falling back to go vet"; Write-Output "To install golangci-lint locally, run: just install-golangci-lint"; go vet ./...; exit $LASTEXITCODE }

lint-linux:
    #!/usr/bin/env sh
    echo "Running linter..."
    if command -v "$HOME/tools/ext/bin/golangci-lint" >/dev/null 2>&1; then
      "$HOME/tools/ext/bin/golangci-lint" run
    elif command -v golangci-lint >/dev/null 2>&1; then
      golangci-lint run
    else
      echo "golangci-lint not found, falling back to go vet"
      echo "To install golangci-lint locally, run: just install-golangci-lint"
      go vet ./...
    fi

lint-macos:
    #!/usr/bin/env sh
    echo "Running linter..."
    if command -v "$HOME/tools/ext/bin/golangci-lint" >/dev/null 2>&1; then
      "$HOME/tools/ext/bin/golangci-lint" run
    elif command -v golangci-lint >/dev/null 2>&1; then
      golangci-lint run
    else
      echo "golangci-lint not found, falling back to go vet"
      echo "To install golangci-lint locally, run: just install-golangci-lint"
      go vet ./...
    fi

gosec:
    @just gosec-{{os()}}

gosec-windows:
    @& { Write-Output "Running security scanner..."; $userHome = [Environment]::GetFolderPath('UserProfile'); $toolDirs = @((Join-Path $userHome "tools/ext/bin"), (Join-Path $userHome "go/bin")); foreach ($dir in ($toolDirs | Select-Object -Unique)) { if (Test-Path $dir) { $env:PATH = "$dir;$env:PATH" } }; $cmd = Get-Command "gosec" -ErrorAction SilentlyContinue; if ($cmd) { & $cmd.Source -quiet -fmt=text ./...; exit $LASTEXITCODE }; Write-Output "gosec not found, skipping security scan"; Write-Output "To install gosec, run: go install github.com/securego/gosec/v2/cmd/gosec@latest" }

gosec-linux:
    #!/usr/bin/env sh
    echo "Running security scanner..."
    if command -v "$HOME/go/bin/gosec" >/dev/null 2>&1; then
      "$HOME/go/bin/gosec" -quiet -fmt=text ./...
    elif command -v gosec >/dev/null 2>&1; then
      gosec -quiet -fmt=text ./...
    else
      echo "gosec not found, skipping security scan"
      echo "To install gosec, run: go install github.com/securego/gosec/v2/cmd/gosec@latest"
    fi

gosec-macos:
    #!/usr/bin/env sh
    echo "Running security scanner..."
    if command -v "$HOME/go/bin/gosec" >/dev/null 2>&1; then
      "$HOME/go/bin/gosec" -quiet -fmt=text ./...
    elif command -v gosec >/dev/null 2>&1; then
      gosec -quiet -fmt=text ./...
    else
      echo "gosec not found, skipping security scan"
      echo "To install gosec, run: go install github.com/securego/gosec/v2/cmd/gosec@latest"
    fi

# Install golangci-lint to ~/tools/ext/bin
install-golangci-lint:
    @just install-golangci-lint-{{os()}}

install-golangci-lint-windows:
    @$gobin = Join-Path ([Environment]::GetFolderPath('UserProfile')) "tools/ext/bin"; New-Item -ItemType Directory -Force -Path $gobin | Out-Null; $env:GOBIN = $gobin; go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
    @Write-Output "golangci-lint installed locally to this project in ~/tools/ext/bin/"
    @Write-Output "Note that ~/tools/ext/bin is not assumed to be in your PATH"

install-golangci-lint-linux:
    #!/usr/bin/env sh
    mkdir -p "$HOME/tools/ext/bin"
    GOBIN="$HOME/tools/ext/bin" go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
    echo "golangci-lint installed locally to this project in ~/tools/ext/bin/"
    echo "Note that ~/tools/ext/bin is not assumed to be in your PATH"

install-golangci-lint-macos:
    #!/usr/bin/env sh
    mkdir -p "$HOME/tools/ext/bin"
    GOBIN="$HOME/tools/ext/bin" go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
    echo "golangci-lint installed locally to this project in ~/tools/ext/bin/"
    echo "Note that ~/tools/ext/bin is not assumed to be in your PATH"

fmt:
    go fmt ./cmd/... ./internal/...

# Check formatting without modifying files
fmt-check:
    @just fmt-check-{{os()}}

fmt-check-windows:
    @$unformatted = gofmt -l ./cmd ./internal
    @if ($unformatted) { Write-Output "Files need formatting:"; $unformatted | ForEach-Object { Write-Output $_ }; Write-Output "Run 'just fmt' to fix formatting"; exit 1 }
    @Write-Output "All files are properly formatted"

fmt-check-linux:
    #!/usr/bin/env sh
    unformatted_files="$(gofmt -l ./cmd ./internal)"
    if [ -n "$unformatted_files" ]; then
      echo "Files need formatting:"
      echo "$unformatted_files"
      echo "Run 'just fmt' to fix formatting"
      exit 1
    fi
    echo "All files are properly formatted"

fmt-check-macos:
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
    @just build-{{os()}}

build-windows:
    @New-Item -ItemType Directory -Force -Path bin | Out-Null
    @go build -ldflags "-X github.com/Garsondee/Soldier-Sense/pkg/commands.Source=https://github.com/Garsondee/Soldier-Sense" -o bin/soldier-sense.exe ./cmd/game

build-linux:
    #!/usr/bin/env sh
    mkdir -p bin
    go build -ldflags "-X github.com/Garsondee/Soldier-Sense/pkg/commands.Source=https://github.com/Garsondee/Soldier-Sense" -o bin/soldier-sense ./cmd/game

build-macos:
    #!/usr/bin/env sh
    mkdir -p bin
    go build -ldflags "-X github.com/Garsondee/Soldier-Sense/pkg/commands.Source=https://github.com/Garsondee/Soldier-Sense" -o bin/soldier-sense ./cmd/game

run: build
    go run ./cmd/game

# Run headless mutual-advance simulation N times and print AAR-ready report lines.
# Supports overrides in KEY=VALUE form, e.g.:
#   just headless-report RUNS=20 TICKS=3600 SEED_BASE=42 SEED_STEP=1
headless-report *OVERRIDES:
    powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File scripts/headless-report.ps1 {{OVERRIDES}}

install:
    go install ./cmd/soldier-sense

clean:
    @just clean-{{os()}}

clean-windows:
    @Remove-Item -Recurse -Force -ErrorAction SilentlyContinue bin, dist, _build

clean-linux:
    #!/usr/bin/env sh
    rm -rf bin
    rm -rf dist
    rm -rf _build

clean-macos:
    #!/usr/bin/env sh
    rm -rf bin
    rm -rf dist
    rm -rf _build

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

