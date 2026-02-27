# 0000 - Why the Ebiten library, 2026-02-27

## Decision

We are using [Ebitengine](https://ebitengine.org/) (formerly known as Ebiten) as the 2D game library for Soldier-Sense.

## Rationale

This was a pragmatic first-pass choice rather than the result of a formal evaluation. The reasoning was:

- Ebiten is well-suited to fast, efficient pixel-level 2D graphics, which fits the top-down tactical rendering this game requires.
- It is a pure Go library with minimal external dependencies, keeping the build simple and cross-platform.
- It has an active community and reasonable documentation.

No strong alternatives were evaluated at the time — this was essentially a "suck it and see" pick to get the project moving.

## Status

Provisional. The library can be replaced if a concrete reason emerges (e.g. performance limitations, missing features, licensing concerns). Because all rendering is encapsulated inside `internal/game`, swapping the library would primarily affect that package rather than requiring changes across the codebase.

## Notes

Ebiten requires CGO and several system graphics libraries (OpenGL, X11, etc.) on Linux, which adds a small amount of CI friction. See [`.github/scripts/install-ebiten-deps.sh`](../../../.github/scripts/install-ebiten-deps.sh).
