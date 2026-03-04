# Contributing to Soldier-Sense

## Code Quality Standards

This project enforces strict code quality, security, and style standards. All contributions must pass automated checks before being merged.

## Before Your First Commit

Set up your development environment:

```bash
just setup
```

This installs linting tools, security scanners, and activates pre-commit hooks.

## Before Every Commit

Pre-commit hooks automatically run:
- ✅ Code formatting checks
- ✅ Linting (30+ checks)
- ✅ Security scanning
- ✅ Unit tests
- ✅ Build verification

**If checks fail, your commit will be blocked.** Fix the issues and try again.

## Quick Commands

```bash
just test          # Run all checks (same as pre-commit)
just fmt           # Auto-format code
just lint          # Run linter only
just gosec         # Run security scan only
just unittest      # Run tests only
```

## IDE Setup

For VS Code/Windsurf users, the `.vscode/settings.json` file automatically:
- Shows linting errors in the Problems panel
- Formats code on save
- Organizes imports on save
- Highlights security issues

Install recommended extensions when prompted.

## Style Guidelines

Follow the guidelines in `WINDSURF.md`:
- LF line endings (not CRLF)
- 120 character line width
- Tabs for Go code, 4 spaces for other languages
- Proper sentence comments with punctuation
- No trailing whitespace

## What Gets Rejected

❌ Code that doesn't pass `just test`  
❌ Unformatted code  
❌ Security vulnerabilities  
❌ Failing tests  
❌ Linting errors without justification  
❌ Commits made with `--no-verify`

## Getting Help

- Full setup guide: `docs/development-setup.md`
- Project guidelines: `WINDSURF.md`
- Ask questions in issues or discussions

## For Maintainers

When reviewing PRs, verify:
1. All CI checks pass
2. Code follows project style
3. Tests cover new functionality
4. No security warnings
5. Documentation is updated
