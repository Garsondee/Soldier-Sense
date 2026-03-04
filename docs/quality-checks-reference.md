# Code Quality Checks - Quick Reference

## 🚀 One-Time Setup

```bash
just setup
```

## 🔄 Daily Workflow

### Automatic (No Action Needed)
- **On Save** - IDE formats code and organizes imports
- **On Commit** - Pre-commit hook runs all checks automatically

### Manual Checks
```bash
just test          # Run everything (recommended before push)
just fmt           # Format all code
just lint          # Lint only
just gosec         # Security scan only
just unittest      # Tests only
```

## 📋 What Gets Checked

| Check | Tool | When | Blocks Commit |
|-------|------|------|---------------|
| Formatting | `gofmt` | On save, pre-commit | ✅ Yes |
| Imports | `goimports` | On save, pre-commit | ✅ Yes |
| Linting | `golangci-lint` | Real-time, pre-commit | ✅ Yes |
| Security | `gosec` | Pre-commit | ✅ Yes |
| Tests | `go test` | Pre-commit | ✅ Yes |
| Build | `go build` | Pre-commit | ✅ Yes |
| Mod tidy | `go mod tidy` | Pre-commit | ✅ Yes |

## 🔍 Problem Panel (IDE)

Your IDE's Problems panel shows:
- ❌ **Errors** - Must fix before commit
- ⚠️ **Warnings** - Should fix, may block commit
- 💡 **Info** - Suggestions for improvement

Filter by:
- **File** - Current file only
- **Workspace** - All files
- **Severity** - Errors/Warnings/Info

## 🛠️ Common Fixes

### "Files need formatting"
```bash
just fmt
```

### "Linting errors"
```bash
just lint          # See what's wrong
# Fix issues, or add //nolint:rulename with justification
```

### "Security issues"
```bash
just gosec         # See vulnerabilities
# Fix or document why it's safe
```

### "Tests failing"
```bash
just unittest      # Run tests
# Fix broken tests
```

### "go.mod not tidy"
```bash
go mod tidy
```

## 🚫 Bypassing Checks (Emergency Only)

```bash
git commit --no-verify
```

**Warning:** Your collaborator will reject PRs that bypass checks.

## 📊 Linters Enabled (30+)

**Error Handling:** errcheck, errorlint, errname  
**Security:** gosec, noctx, bodyclose, sqlclosecheck  
**Performance:** prealloc  
**Code Quality:** govet, staticcheck, revive, gocritic  
**Style:** stylecheck, gofmt, goimports, godot  
**Complexity:** gocyclo, gocognit  
**Best Practices:** unconvert, unparam, unused, misspell

## 🎯 Success Criteria

Before pushing:
- ✅ `just test` passes
- ✅ No errors in Problems panel
- ✅ All tests pass
- ✅ Code formatted
- ✅ No security warnings
- ✅ Builds successfully

## 📚 More Info

- Full guide: `docs/development-setup.md`
- Contributing: `.github/CONTRIBUTING.md`
- Style guide: `WINDSURF.md`
