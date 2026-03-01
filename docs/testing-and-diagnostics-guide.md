# Testing and Diagnostics Guide

**Date**: March 1, 2026  
**Purpose**: Comprehensive guide to testing infrastructure and diagnostic tools for identifying issues

---

## Overview

This guide documents the testing and diagnostic capabilities available for identifying behavioral issues, performance bottlenecks, and decision-making problems in the soldier AI system.

---

## Existing Testing Infrastructure

### 1. TestSim - Headless Simulation Harness

**Location**: `internal/game/test_harness.go`

**Purpose**: Deterministic headless simulation for automated testing without Ebiten dependency.

**Key Features**:
- Deterministic RNG seeding for reproducible tests
- Structured event logging via SimLog
- Periodic state snapshots via SimReporter
- Combat effectiveness probes (stall detection, detachment detection)
- Performance tracking per soldier

**Usage**:
```go
ts := &TestSim{
    Width:  1600,
    Height: 1200,
    SimLog: NewSimLog(false), // set true for verbose logging
    Reporter: NewSimReporter(600, false),
}
// ... setup soldiers, squads, buildings
ts.runOneTick(tick)
```

### 2. SimLog - Structured Event Logging

**Location**: `internal/game/sim_log.go`

**Purpose**: Unbounded machine-readable event log for test analysis.

**Event Categories**:
- `goal` - Goal changes and selections
- `squad` - Squad intent changes, phase transitions
- `vision` - Contact gained/lost
- `state` - Soldier state changes (moving, cover, wounded, etc.)
- `psych` - Psychological events (panic, disobedience, surrender)
- `effectiveness` - Combat effectiveness alerts (stalled, detached)
- `radio` - Radio communication events

**Key Methods**:
```go
// Filter by category/key
entries := simLog.Filter("goal", "change")

// Find oscillations
goalChanges := simLog.Filter("goal", "change")

// Get soldier timeline
timeline := simLog.FilterSoldier("R0")

// Time range analysis
window := simLog.FilterTickRange(100, 200)
```

### 3. SimReporter - Periodic State Snapshots

**Location**: `internal/game/reporter.go`

**Purpose**: Captures full simulation state at regular intervals for trend analysis.

**Captured Metrics**:
- Goal distribution per team
- Alive/dead/injured counts
- Visibility and contact statistics
- Psychological state (disobedience, panic, surrender)
- Combat effectiveness (stalled, detached)
- Squad metrics (stress, casualty rate, posture)

**Usage**:
```go
reporter := NewSimReporter(600, false) // 600 tick window
reporter.Capture(tick, soldiers, squads, effProbes, perfTrackers)

// Get windowed averages
window := reporter.WindowReport(tick)
fmt.Printf("Avg red stress: %.2f\n", window.RedStress)
```

---

## New Diagnostic Tools (Round 5)

### 4. DecisionTracker - Goal Utility Analysis

**Location**: `internal/game/decision_tracker.go`

**Purpose**: Track goal selection decisions with full utility scores to identify decision-making issues.

**Key Features**:
- Records all goal utility calculations
- Tracks winner/runner-up margins
- Detects goal oscillations (A→B→A patterns)
- Identifies low-margin decisions (unstable choices)
- Captures decision context (fear, morale, threats, etc.)

**Usage**:
```go
// Create tracker (enable in tests)
dt := NewDecisionTracker(true)

// Record decision (call from goal selection code)
dt.Record(tick, soldier, prevGoal, newGoal, utilities)

// Analyze oscillations
oscillations := dt.FindOscillations(soldierID, 120) // within 120 ticks
for _, osc := range oscillations {
    fmt.Println(osc.String())
    // Output: R0 oscillation T=10-30 (20t): advance ↔ engage (margins: 0.05, 0.03)
}

// Find unstable decisions
lowMargin := dt.FindLowMarginDecisions(0.1) // margin < 0.1
for _, entry := range lowMargin {
    fmt.Printf("T=%d %s: %s (%.2f) barely beat %s (%.2f)\n",
        entry.Tick, entry.Label, entry.Winner, entry.WinnerUtil,
        entry.RunnerUp, entry.RunnerUtil)
}

// Get all goal changes
changes := dt.FindGoalChanges()

// Generate oscillation report
report := dt.SummarizeOscillations(120)
fmt.Println(report)
```

**Decision Context Captured**:
- Visible threats count
- Squad contact status
- Incoming fire count
- Suppression state
- Fear, morale, health
- Squad intent and broken state
- Activation status

---

## Testing Patterns

### Pattern 1: Scenario Tests

**Purpose**: Test specific tactical situations with expected outcomes.

**Example**: `internal/game/scenario_test.go`
```go
func TestScenario_CloseEngagement(t *testing.T) {
    ts := setupMutualAdvanceScenario(t, 42)
    
    // Run simulation
    for tick := 1; tick <= 300; tick++ {
        ts.runOneTick(tick)
    }
    
    // Analyze results
    log := ts.SimLog
    
    // Check for expected behaviors
    if !log.HasEntry("psych", "panic_retreat", "") {
        t.Error("Expected panic retreat under pressure")
    }
    
    // Verify effectiveness
    stalled := log.CountCategory("effectiveness", "stalled_in_combat")
    if stalled > 5 {
        t.Errorf("Too many stall events: %d", stalled)
    }
}
```

### Pattern 2: Oscillation Detection

**Purpose**: Identify goal flip-flopping that indicates utility tuning issues.

```go
func TestGoalStability_NoOscillation(t *testing.T) {
    ts := setupScenario(t)
    dt := NewDecisionTracker(true)
    
    // Hook into goal selection (requires integration)
    // ... run simulation with decision tracking ...
    
    // Check for oscillations
    for _, s := range ts.Soldiers {
        oscillations := dt.FindOscillations(s.id, 60)
        if len(oscillations) > 0 {
            t.Errorf("%s oscillated: %v", s.label, oscillations)
        }
    }
}
```

### Pattern 3: Performance Profiling

**Purpose**: Identify performance bottlenecks.

**Existing**: `internal/game/performance.go` provides PerfTracker
```go
// Already integrated in TestSim
perfData := ts.PerfTrackers[soldierID]
if perfData.VisionScanTime > threshold {
    t.Errorf("Vision scan too slow: %v", perfData.VisionScanTime)
}
```

---

## Diagnostic Workflow

### Step 1: Reproduce Issue

1. Create scenario test that exhibits the problem
2. Enable verbose logging: `NewSimLog(true)`
3. Enable decision tracking if decision-making related
4. Run test and capture full log

### Step 2: Analyze Logs

**For Decision-Making Issues**:
```go
// Find all goal changes for soldier
changes := simLog.FilterSoldier("R0").Filter("goal", "change")

// Look for patterns
for _, e := range changes {
    fmt.Printf("T=%d: %s → %s\n", e.Tick, e.Value)
}

// Check decision margins (if DecisionTracker integrated)
lowMargin := dt.FindLowMarginDecisions(0.15)
```

**For Behavioral Issues**:
```go
// Check psychological state transitions
psychEvents := simLog.Filter("psych", "")

// Check effectiveness alerts
stalled := simLog.Filter("effectiveness", "stalled_in_combat")
detached := simLog.Filter("effectiveness", "detached_from_engagement")
```

**For Squad Coordination**:
```go
// Track squad intent changes
intentChanges := simLog.Filter("squad", "intent_change")

// Check formation issues
formationEvents := simLog.Filter("formation", "")
```

### Step 3: Generate Reports

```go
// Windowed behavior report
window := reporter.WindowReport(tick)
fmt.Println(window.Format())

// Oscillation summary
oscReport := dt.SummarizeOscillations(120)
fmt.Println(oscReport)

// Full timeline for specific soldier
timeline := simLog.FilterSoldier("R0")
for _, e := range timeline {
    fmt.Println(e.String())
}
```

---

## Integration Points

### Adding DecisionTracker to Goal Selection

To enable decision tracking in production code:

```go
// In blackboard.go SelectGoalWithHysteresis()
func SelectGoalWithHysteresis(bb *Blackboard, profile *SoldierProfile, 
    isLeader bool, hasPath bool, dt *DecisionTracker, tick int, s *Soldier) GoalKind {
    
    // ... calculate utilities ...
    
    utilities := map[GoalKind]float64{
        GoalAdvance: advanceUtil,
        GoalEngage: engageUtil,
        // ... etc
    }
    
    prevGoal := bb.CurrentGoal
    newGoal := winner
    
    // Record decision
    if dt != nil {
        dt.Record(tick, s, prevGoal, newGoal, utilities)
    }
    
    return newGoal
}
```

---

## Common Issues and Diagnostics

### Issue: Goal Oscillation

**Symptoms**: Soldier rapidly switches between two goals
**Diagnostic**: 
```go
oscillations := dt.FindOscillations(soldierID, 60)
// Check utility margins - if both goals have similar utility, increase hysteresis
```

### Issue: Soldiers Stalling in Combat

**Symptoms**: `stalled_in_combat` effectiveness alerts
**Diagnostic**:
```go
stalled := simLog.Filter("effectiveness", "stalled_in_combat")
// Check goal at stall time, visible threats, path state
```

### Issue: Squad Scatter

**Symptoms**: High squad spread, detachment alerts
**Diagnostic**:
```go
detached := simLog.Filter("effectiveness", "detached_from_engagement")
spreadData := reporter.WindowReport(tick)
// Check formation update logic, goal priorities
```

### Issue: Excessive Panic/Disobedience

**Symptoms**: Many psych events
**Diagnostic**:
```go
panic := simLog.Filter("psych", "panic_retreat")
disobey := simLog.Filter("psych", "disobedience")
// Check stress accumulation, morale decay, cohesion pressure
```

---

## Future Enhancements

### Planned Additions

1. **Utility Heatmaps**: Visualize utility scores over time for each goal
2. **Path Analysis**: Track path quality, repath frequency, stuck detection
3. **LOS Cache Analysis**: Measure cache hit rates, identify redundant checks
4. **Formation Metrics**: Track formation cohesion, slot assignment conflicts
5. **Radio Network Analysis**: Message delivery rates, timeout patterns
6. **Automated Regression Detection**: Compare test runs to detect behavioral changes

### Performance Profiling Enhancements

1. **Frame-by-frame profiling**: Identify specific tick spikes
2. **Memory allocation tracking**: Detect allocation hotspots
3. **Bottleneck visualization**: Generate flame graphs from perf data

---

## Best Practices

1. **Start with SimLog**: Use structured logging before adding specialized trackers
2. **Reproducible tests**: Always use deterministic RNG seeds
3. **Windowed analysis**: Use SimReporter for trend analysis over time
4. **Targeted tracking**: Enable DecisionTracker only for specific tests (overhead)
5. **Incremental diagnosis**: Start broad (SimLog), narrow down (DecisionTracker)
6. **Document findings**: Add comments in test files explaining what each test validates

---

## Example: Complete Diagnostic Session

```go
func TestDiagnose_PeekLoopIssue(t *testing.T) {
    // Setup
    ts := setupPeekScenario(t, 42)
    dt := NewDecisionTracker(true)
    
    // Run simulation
    for tick := 1; tick <= 300; tick++ {
        ts.runOneTick(tick)
        // Hook decision tracking here if integrated
    }
    
    // Analyze
    log := ts.SimLog
    
    // 1. Check for peek goal oscillations
    peekChanges := log.Filter("goal", "change")
    peekCount := 0
    for _, e := range peekChanges {
        if strings.Contains(e.Value, "peek") {
            peekCount++
        }
    }
    
    // 2. Check decision margins
    lowMargin := dt.FindLowMarginDecisions(0.1)
    peekDecisions := 0
    for _, d := range lowMargin {
        if d.Winner == GoalPeek || d.RunnerUp == GoalPeek {
            peekDecisions++
        }
    }
    
    // 3. Generate report
    t.Logf("Peek goal selections: %d", peekCount)
    t.Logf("Low-margin peek decisions: %d", peekDecisions)
    t.Logf("Oscillations:\n%s", dt.SummarizeOscillations(60))
    
    // 4. Assert expectations
    if peekCount > 10 {
        t.Errorf("Excessive peek oscillation: %d changes", peekCount)
    }
}
```

---

## Summary

The testing infrastructure provides multiple layers of observability:

1. **SimLog**: Event-level detail for understanding what happened
2. **SimReporter**: Aggregate metrics for trend analysis
3. **DecisionTracker**: Decision-level detail for utility tuning
4. **PerfTracker**: Performance metrics for optimization

Use these tools in combination to diagnose issues efficiently:
- Start with SimLog to identify when/where issues occur
- Use SimReporter to understand trends and patterns
- Use DecisionTracker to analyze why specific decisions were made
- Use PerfTracker to identify performance bottlenecks

All tools are designed to work with deterministic tests for reproducible diagnosis.
