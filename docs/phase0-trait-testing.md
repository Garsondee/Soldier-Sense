# Phase 0: Personality Trait Testing Implementation

## Overview

Phase 0 implements a scientific testing framework to validate that personality traits create measurable differences in combat outcomes. This is the foundation for future evolutionary optimization.

## Implementation Summary

### 1. Personality Traits Added

**New struct in `internal/game/stats.go`:**
```go
type PersonalityTraits struct {
    Aggression     float64 // [0-1] preference for offensive action
    Caution        float64 // [0-1] risk aversion, cover-seeking tendency
    PanicThreshold float64 // [0-1] resistance to psychological collapse
}
```

**Integration points:**
- Added to `SoldierProfile` struct
- Default values: `Aggression: 0.5, Caution: 0.5, PanicThreshold: 0.5`

### 2. Behavioral Integration

**Panic System (`internal/game/soldier.go`):**
- `PanicThreshold` reduces panic drive by `0.25 * PanicThreshold`
- Higher threshold = more resistant to panic retreat and surrender

**Goal Selection (`internal/game/blackboard.go`):**
- `Aggression` increases `MoveToContact` utility by `+0.15`
- `Aggression` increases `Engage` utility by `+0.10`
- `Caution` increases `Survive` (cover-seeking) utility by `+0.12`

### 3. Test Genomes

**Control Group (Blue Team):**
- Neutral baseline: `Aggression: 0.5, Caution: 0.5, PanicThreshold: 0.5`
- Used as scientific control in all tests

**Experimental Genomes (Red Team):**
1. **Aggressive** - High aggression (0.9), low caution (0.2)
2. **Cautious** - Low aggression (0.2), high caution (0.9)
3. **Fearless** - High panic threshold (0.95)
4. **Panicky** - Low panic threshold (0.15)
5. **Berserker** - Maximum aggression (1.0), zero caution (0.0)
6. **Balanced-Aggressive** - Moderate aggression (0.7) + good panic resistance (0.7)
7. **Balanced-Defensive** - Moderate caution (0.7) + good panic resistance (0.7)
8. **Elite-Aggressive** - High skills + aggressive personality
9. **Elite-Defensive** - High skills + defensive personality

### 4. Test Harness

**Command-line tool:** `cmd/trait-test/main.go`

**Usage:**
```bash
go run ./cmd/trait-test -runs 50 -ticks 3600 -seed 1000
```

**Flags:**
- `-runs N` - Number of battles per genome (default: 50)
- `-ticks N` - Maximum ticks per battle (default: 3600)
- `-seed N` - Base RNG seed (default: 1000)
- `-v` - Verbose output

**Methodology:**
- Blue team always uses control genome (neutral stats)
- Red team uses experimental genome (varied personality)
- Each genome runs N battles with different seeds
- Results aggregated and compared to control

### 5. Metrics Collected

**Per Battle:**
- Survival rate (survivors / total)
- K/D ratio (kills / deaths)
- Win rate (red survivors > blue survivors)
- Battle duration (ticks)
- Panic events count
- Surrender count
- Disobedience count

**Aggregated:**
- Average and standard deviation for all metrics
- Statistical comparison vs control group
- Best/worst performers identification

### 6. Expected Outcomes

**If traits are working correctly:**
- **Aggressive** genome: Higher K/D, lower survival, faster battles
- **Cautious** genome: Higher survival, lower K/D, slower battles
- **Fearless** genome: Fewer panic events, better performance under pressure
- **Panicky** genome: More panic events, worse performance under pressure
- **Berserker** genome: Highest K/D but lowest survival (high risk)

**If traits have no effect:**
- All genomes perform similarly to control
- No >5% difference in survival rates
- Indicates traits need stronger integration

## Files Modified/Created

### Modified:
- `internal/game/stats.go` - Added PersonalityTraits struct
- `internal/game/soldier.go` - Integrated PanicThreshold, added IsAlive/SetProfile methods
- `internal/game/blackboard.go` - Integrated Aggression/Caution into goal utilities

### Created:
- `internal/game/trait_testing.go` - Test genome definitions and result analysis
- `cmd/trait-test/main.go` - Test harness CLI tool
- `docs/phase0-trait-testing.md` - This document

## Next Steps

### Immediate:
1. Run initial test suite: `go run ./cmd/trait-test -runs 100`
2. Analyze results to validate trait impact
3. Adjust integration weights if effects are too weak/strong

### Phase 1 (After validation):
1. Implement basic genetic algorithm
2. Evolve 8-parameter genomes over 20 generations
3. Compare evolved soldiers to hand-crafted genomes

### Phase 2 (Advanced):
1. Expand to 16+ parameters
2. Multiple fitness functions (aggressive, defensive, balanced)
3. Scenario rotation for generalization
4. Parallel evaluation for speed

## Scientific Approach

**Control Group Design:**
- Blue team always uses neutral baseline
- Isolates effect of Red team personality changes
- Eliminates confounding variables

**Statistical Validation:**
- Multiple runs per genome (50-100)
- Standard deviation tracking
- >5% difference threshold for significance
- Reproducible with seed control

**Hypothesis Testing:**
- H0: Personality traits have no effect on outcomes
- H1: Personality traits create measurable differences
- If H1 confirmed → proceed to evolutionary optimization
- If H0 confirmed → strengthen trait integration

## Running Your First Test

```bash
# Build the test tool
go build ./cmd/trait-test

# Run a quick test (10 runs per genome)
./trait-test -runs 10 -ticks 1800

# Run full test suite (100 runs per genome)
./trait-test -runs 100 -ticks 3600

# Verbose mode to see individual battles
./trait-test -runs 10 -v
```

## Success Criteria

**Phase 0 is successful if:**
1. ✅ Code compiles and runs without errors
2. ⏳ At least 3 genomes show >5% survival difference from control
3. ⏳ Aggressive genomes have higher K/D than cautious genomes
4. ⏳ Panicky genome shows more panic events than fearless genome
5. ⏳ Results are reproducible with same seed

**Status:** Implementation complete, ready for testing

---

**Implementation Date:** 2026-03-03  
**Branch:** trait-testing-phase0  
**Next Milestone:** Run test suite and validate trait impact
