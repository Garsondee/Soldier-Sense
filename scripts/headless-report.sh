#!/usr/bin/env sh
# Run headless mutual-advance simulation and print AAR-ready report lines.
# Accepts overrides as KEY=VALUE arguments, e.g.:
#   sh scripts/headless-report.sh RUNS=20 TICKS=3600 SEED_BASE=42 SEED_STEP=1

RUNS=5
TICKS=3600
SEED_BASE=42
SEED_STEP=1

for pair in "$@"; do
    key="${pair%%=*}"
    value="${pair#*=}"
    case "$key" in
        RUNS)      RUNS="$value" ;;
        TICKS)     TICKS="$value" ;;
        SEED_BASE) SEED_BASE="$value" ;;
        SEED_STEP) SEED_STEP="$value" ;;
    esac
done

go run ./cmd/headless-report -runs "$RUNS" -ticks "$TICKS" -seed-base "$SEED_BASE" -seed-step "$SEED_STEP"
