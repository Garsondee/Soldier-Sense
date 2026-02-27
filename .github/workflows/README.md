# GitHub Actions Workflows

This project uses two workflows. One for pushes to main and the other for releases, triggered by tag-pushes.

## `build-and-test.yml` — Continuous Integration

Triggered on every push or pull request targeting `main`.

Runs four parallel jobs:

### `test`

Executes the full quality-gate matrix across **Linux and macOS × Go 1.23 and 1.24** (four combinations):

| Step | Detail |
|---|---|
| Checkout | `actions/checkout@v4` |
| Format check | `gofmt -s -l` — fails if any file needs reformatting |
| Vet | `go vet ./...` |
| Test | `go test -race -coverprofile=coverage.out -covermode=atomic ./...` |
| Build | `go build -v -o soldier-sense ./cmd/soldier-sense` |
| Smoke test | Runs `./soldier-sense version` and `./soldier-sense --version` |
| Artifact upload | Binary uploaded for 1 day (`soldier-sense-<os>-go<version>`) |
| Coverage upload | Coverage sent to Codecov (ubuntu + Go 1.24 run only) |

### `integration`

Builds the binary on **Linux and macOS** using Go 1.24. Provides a dedicated integration-test slot that can be expanded independently of the unit-test matrix.

### `lint`

Runs on **ubuntu-latest** with Go 1.24:

- `golangci-lint` (latest, 5 min timeout) via the official action
- `staticcheck`

### `security`

Runs on **ubuntu-latest** with Go 1.24:

- `gosec` security scanner (`-exclude-generated ./...`)

---

## `release.yml` — Release

Triggered when a tag matching `v*.*.*` is pushed. Requires `contents: write` and `packages: write` permissions.

### Jobs

1. **`test`** — runs `go test ./...` on ubuntu/Go 1.24 as a gate.
2. **`release`** (needs `test`) — checks out the full git history (`fetch-depth: 0`, required by GoReleaser for changelog generation) then runs **GoReleaser v2** with `--clean`, authenticated via `GITHUB_TOKEN`.

GoReleaser builds for Linux and macOS (amd64 + arm64), produces `.tar.gz` archives, generates a changelog from conventional commits, and creates a GitHub release.

---

## Tagging a release

Releases follow [Semantic Versioning](https://semver.org/). Push a tag to trigger the release workflow:

```bash
# Full release
git tag v1.2.3
git push origin v1.2.3

# Pre-release (alpha / beta / rc)
git tag v1.2.3-alpha.1
git push origin v1.2.3-alpha.1
```

GoReleaser's `prerelease: auto` setting automatically marks any tag containing a SemVer pre-release identifier (e.g. `-alpha.1`, `-beta.2`, `-rc.1`) as a GitHub pre-release. Tags without a pre-release identifier are published as full releases.
