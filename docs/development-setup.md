# Development Setup Guide

This guide covers setting up the development environment for Soldier-Sense with automated code quality checks, linting, and security scanning.

## Quick Start

To set up your development environment with all tools and pre-commit hooks:

```bash
just setup
```

This will:
- Install `golangci-lint` for comprehensive linting
- Install `gosec` for security scanning
- Verify pre-commit hooks are in place
- Display what checks will run automatically

## What Gets Checked Automatically

### Pre-Commit Hooks

Every time you commit, the following checks run automatically:

1. **Code Formatting** - Ensures all Go code is properly formatted with `gofmt`
2. **Go Vet** - Runs Go's built-in static analyzer
3. **Linting** - Comprehensive linting with `golangci-lint` (30+ linters)
4. **Security Scanning** - Security vulnerability detection with `gosec`
5. **Module Tidiness** - Verifies `go.mod` is tidy
6. **Unit Tests** - Runs all tests to ensure nothing breaks
7. **Build Verification** - Confirms the project builds successfully

If any check fails, the commit is blocked until issues are fixed.

### IDE Integration (VS Code/Windsurf)

The `.vscode/settings.json` file configures your IDE to:

- **Format on Save** - Automatically formats Go files when saved
- **Organize Imports** - Automatically organizes imports on save
- **Real-time Linting** - Shows linting errors in the Problems panel as you type
- **Security Warnings** - Displays vulnerability warnings for imported packages
- **Enforce Style** - Matches project style guidelines from `WINDSURF.md`

## Manual Checks

You can run checks manually at any time:

```bash
# Run all checks (same as pre-commit)
just test

# Individual checks
just fmt          # Format code
just fmt-check    # Check formatting without modifying
just lint         # Run linter
just gosec        # Run security scan
just unittest     # Run tests only
just build        # Build only
```

## Installing Tools

### All Tools at Once

```bash
just install-tools
```

### Individual Tools

```bash
# Linter
just install-golangci-lint

# Security scanner
just install-gosec
```

**Note:** Ensure `~/go/bin` (or `%USERPROFILE%\go\bin` on Windows) is in your PATH.

## Configuration Files

### `.golangci.yml`

Configures 30+ linters including:

- **Error checking**: `errcheck`, `errorlint`
- **Code quality**: `govet`, `staticcheck`, `revive`, `gocritic`
- **Security**: `gosec` (with G404 exception for game simulation RNG)
- **Style**: `stylecheck`, `gofmt`, `goimports`
- **Performance**: `prealloc`
- **Complexity**: `gocyclo`, `gocognit`
- **Best practices**: `godot`, `misspell`, `unconvert`

### `.git/hooks/pre-commit`

Bash script that runs all checks before allowing commits. Provides:
- Colored output for easy scanning
- Clear error messages
- Suggestions for fixing issues
- Exit codes that block commits on failure

### `.vscode/settings.json`

IDE configuration that:
- Enables real-time linting with `golangci-lint`
- Enforces project style guidelines
- Configures the Problems panel
- Sets up format-on-save
- Matches `WINDSURF.md` requirements (LF line endings, 120 char width, etc.)

## Bypassing Pre-Commit Checks

**Not recommended**, but if you need to commit without running checks:

```bash
git commit --no-verify
```

Only use this in emergencies. Your collaborator will not be happy if you push code that fails checks.

## Troubleshooting

### Pre-commit hook not running

The hook should be at `.git/hooks/pre-commit`. If it's missing, it was created when you cloned/set up the repo. Check that the file exists and is executable.

### Tools not found

Ensure `~/go/bin` is in your PATH:

```bash
# Check if tools are accessible
which golangci-lint
which gosec

# Add to PATH (add to ~/.bashrc or ~/.zshrc to make permanent)
export PATH="$PATH:$(go env GOPATH)/bin"
```

On Windows PowerShell:
```powershell
$env:PATH += ";$env:USERPROFILE\go\bin"
```

### Linter too slow

The pre-commit hook runs the full linter. For faster feedback during development, VS Code runs linting with the `--fast` flag, which skips some slower checks.

### False positives

If a linter reports a false positive:

1. Add a `//nolint:lintername` comment with justification
2. Or update `.golangci.yml` to exclude the specific check
3. Discuss with your collaborator before disabling checks

### G404 warnings about weak RNG

The project intentionally uses `math/rand` for game simulation (not cryptography). This is excluded in `.golangci.yml`, but gosec may still warn. This is expected and safe.

## What Makes Your Collaborator Happy

✅ **DO:**
- Run `just test` before pushing
- Fix all linting and security issues
- Keep `go.mod` tidy
- Write tests for new code
- Follow the style guide in `WINDSURF.md`
- Let pre-commit hooks run (don't use `--no-verify`)

❌ **DON'T:**
- Push code that doesn't pass `just test`
- Disable linters without discussion
- Use `--no-verify` to bypass checks
- Ignore security warnings from gosec
- Leave commented-out code or TODOs without tracking

## Integration with Existing Workflow

The setup integrates with your existing workflow from `WINDSURF.md`:

> Before submitting code to origin, check it is correctly formatted, passes a lint check, and builds correctly. You can use `just test` to do this with a single command.

Now this happens automatically via pre-commit hooks, and your IDE shows issues in real-time.

## Additional Resources

- [golangci-lint documentation](https://golangci-lint.run/)
- [gosec documentation](https://github.com/securego/gosec)
- [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments)
- Project style guide: `WINDSURF.md`
