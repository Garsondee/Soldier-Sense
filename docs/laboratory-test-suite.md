# Laboratory Test Suite

## Overview

The laboratory test suite provides controlled, repeatable experiments to validate soldier AI behavior and decision-making. Tests run in predetermined "rooms" with specific stimuli and expected responses, allowing both automated validation and visual inspection.

## Architecture

### Components

1. **LaboratoryTest** - Defines a test scenario with setup, stimulus, measurement, and validation
2. **LaboratoryObservation** - Records measurements during test execution
3. **LaboratoryVisualMode** - Renders tests as interactive visual scenes
4. **Automated Tests** - Go test functions that run tests headlessly and validate results

### Files

- `internal/game/laboratory.go` - Core test infrastructure and test catalog
- `internal/game/laboratory_visual.go` - Visual rendering mode
- `internal/game/laboratory_test.go` - Automated test functions
- `cmd/laboratory/main.go` - Command-line visual test runner

## Running Tests

### Automated Tests (Headless)

Run all laboratory tests:
```bash
go test ./internal/game -v -run TestLaboratory
```

Run a specific test:
```bash
go test ./internal/game -v -run TestLaboratory_SuppressionResponse
```

### Visual Tests (Interactive)

Build and run the laboratory viewer:
```bash
go build -o laboratory.exe ./cmd/laboratory
./laboratory.exe
```

List available tests:
```bash
./laboratory.exe -list
```

Run a specific test:
```bash
./laboratory.exe -test "Suppression Response"
```

### Visual Controls

- **SPACE** - Pause/Resume simulation
- **1/2/4** - Set speed (1x, 2x, 4x)
- **Arrow Keys** - Pan camera
- **+/-** - Zoom in/out
- **H** - Toggle help overlay
- **E** - Toggle events panel
- **ESC** - Exit to menu

## Current Test Catalog

### 1. Suppression Response Test

**Description:** Single soldier in open terrain receives sustained suppression fire

**Expected Behavior:**
- Soldier should seek cover
- Crouch or go prone
- Fear increases
- Goal changes to survive

**Duration:** 600 ticks (10 seconds)

**Validation Criteria:**
- Fear > 0.1
- At least one stance change
- At least one goal change
- Either reached cover OR took defensive stance

---

### 2. Fear Threshold Test

**Description:** Single soldier exposed to gradually increasing threats

**Expected Behavior:**
- Fear accumulates over time
- Eventually triggers panic retreat or disobedience
- Psychological threshold clearly observable

**Duration:** 900 ticks (15 seconds)

**Validation Criteria:**
- Max fear reaches > 0.5
- Panic retreat OR disobedience triggered

---

### 3. Formation Maintenance Test

**Description:** 6-soldier squad advances in wedge formation across open terrain

**Expected Behavior:**
- Squad maintains formation
- Spread stays below threshold
- Members follow formation slots
- Leader may slow for cohesion

**Duration:** 600 ticks (10 seconds)

**Validation Criteria:**
- Max formation spread < 200px
- Spread never exceeds 150px for extended period

---

### 4. First Contact Response Test

**Description:** Squad advancing encounters single enemy at 300m range

**Expected Behavior:**
- Squad detects enemy
- Squad halts or slows
- Intent changes to engage
- Members take defensive stances
- Goal changes from advance to combat goals

**Duration:** 300 ticks (5 seconds)

**Validation Criteria:**
- Contact detected
- At least one goal change
- At least one stance change

---

### 5. Cohesion Collapse Test

**Description:** Squad takes casualties at regular intervals until cohesion breaks

**Expected Behavior:**
- Squad cohesion degrades with casualties
- Eventually breaks at ~50% casualties
- Triggers panic, disobedience, or surrender
- Psychological cascade observable

**Duration:** 600 ticks (10 seconds)

**Validation Criteria:**
- Squad cohesion breaks
- Casualty rate > 30% when break occurs

---

## Visual Indicators

### Soldier Colors
- **Red circles** - Red team soldiers
- **Blue circles** - Blue team soldiers
- **Yellow circles** - Panicking soldiers
- **Orange circles** - Disobeying soldiers
- **Gray circles** - Dead soldiers

### UI Panels

**Test Info Panel (Top Left)**
- Test name
- Description
- Expected behavior

**Status Panel (Top Right)**
- Current tick / total ticks
- Simulation speed
- Key observations (contact, fear, panic, cohesion)
- Pass/fail result (when complete)

**Events Panel (Bottom Left)**
- Recent events log
- Color-coded by event type
- Shows last 10 events

**Help Overlay (Center)**
- Keyboard controls
- Visual indicator legend
- Toggle with 'H' key

## Creating New Tests

### Test Structure

```go
func MyNewTest() *LaboratoryTest {
    return &LaboratoryTest{
        Name:        "Test Name",
        Description: "What the test does",
        Expected:    "What should happen",
        DurationTicks: 600,
        
        Setup: func() *TestSim {
            // Create test environment
            ts := NewTestSim(
                WithMapSize(800, 600),
                WithSeed(42),
                // Add soldiers, buildings, etc.
            )
            return ts
        },
        
        Stimulus: func(ts *TestSim, tick int) {
            // Apply stimulus at specific ticks
            if tick == 60 {
                // Do something at tick 60
            }
        },
        
        Measure: func(ts *TestSim, tick int, obs *LaboratoryObservation) {
            // Record custom measurements
            obs.Metrics["my_metric"] = someValue
            obs.Flags["my_flag"] = someCondition
        },
        
        Validate: func(obs *LaboratoryObservation) (bool, string) {
            // Check if test passed
            if obs.MaxFear < 0.1 {
                return false, "Fear too low"
            }
            return true, "Test passed"
        },
    }
}
```

### Adding to Catalog

1. Create test function in `internal/game/laboratory.go`
2. Add to `GetAllLaboratoryTests()` function
3. Create automated test in `internal/game/laboratory_test.go`:

```go
func TestLaboratory_MyNewTest(t *testing.T) {
    test := MyNewTest()
    obs := RunLaboratoryTest(test)
    
    printBehavioralTestResult(t, test, obs)
    
    passed, reason := test.Validate(obs)
    if !passed {
        t.Errorf("Test failed: %s", reason)
    }
}
```

## Measurement Types

### Automatic Measurements

These are tracked automatically by `RunLaboratoryTest`:

- `FirstContactTick` - When first enemy spotted
- `FirstFearIncreaseTick` - When fear first increases
- `FirstGoalChangeTick` - When first goal change occurs
- `FirstStanceChangeTick` - When first stance change occurs
- `FirstPanicTick` - When panic retreat activates
- `FirstDisobeyTick` - When disobedience starts
- `CohesionBreakTick` - When squad cohesion breaks
- `MaxFear` / `MaxFearTick` - Highest fear level observed
- `GoalChanges` - Total number of goal changes
- `StanceChanges` - Total number of stance changes
- `FormationSpreadMax` / `FormationSpreadFinal` - Formation metrics

### Custom Measurements

Use the `Measure` function to track custom metrics:

```go
Measure: func(ts *TestSim, tick int, obs *LaboratoryObservation) {
    // Numeric metrics
    obs.Metrics["distance_to_cover"] = calculateDistance()
    obs.Metrics["shots_fired"] = float64(shotCount)
    
    // Boolean flags
    obs.Flags["reached_objective"] = atObjective
    obs.Flags["squad_halted"] = avgSpeed < threshold
    
    // Text values
    obs.Texts["squad_intent"] = squad.Intent.String()
}
```

### Event Recording

Record discrete events during the test:

```go
obs.AddEvent(tick, soldierID, label, eventType, description, value)
```

Example:
```go
obs.AddEvent(120, s.id, s.label, "cover_reached", 
    "Reached cover position", distanceTraveled)
```

## Design Principles

### Repeatability
- All tests use fixed seeds for deterministic behavior
- Same test always produces same results
- Enables regression detection

### Isolation
- Each test focuses on one behavior or system
- Minimal external factors
- Clear cause and effect

### Observability
- Visual rendering shows what's happening
- Detailed event logs
- Quantitative metrics for validation

### Automation
- Tests can run headlessly in CI/CD
- Pass/fail validation
- Detailed failure reasons

## Future Test Ideas

### Decision-Making Tests
- **Discipline vs Fear** - High vs low discipline under same threat
- **Target Priority** - Multiple enemies at different ranges
- **Zero Ammo Behavior** - Soldier with no ammunition
- **Lone Survivor** - Last soldier in broken squad

### Movement Tests
- **Obstacle Avoidance** - Path around building
- **Collision Avoidance** - Two soldiers crossing paths
- **Doorway Transit** - Squad through chokepoint

### Psychological Tests
- **Panic Recovery** - Soldier recovers after threat removal
- **Surrender Conditions** - Isolated soldier vs overwhelming force
- **Morale Cascade** - Squad morale collapse

### Medical Tests
- **Buddy Aid Priority** - Multiple casualties, who gets aid first
- **Self-Aid Decision** - When to self-aid vs continue mission
- **Casualty Evacuation** - Squad behavior with non-ambulatory casualty

### Communication Tests
- **Radio Range** - Message delivery at various distances
- **Officer Down** - Leadership succession
- **Shared Awareness** - Threat info propagation

### Combat Tests
- **Cover Seeking** - Under fire, move to cover
- **Stance Selection** - Appropriate stance for situation
- **Flanking Detection** - React to enemy from side/rear

## Troubleshooting

### Test Fails Unexpectedly

1. Run test visually to observe behavior
2. Check event log for unexpected events
3. Verify stimulus is being applied correctly
4. Check validation criteria are appropriate

### Visual Mode Crashes

1. Ensure all dependencies are installed
2. Check graphics drivers are up to date
3. Try reducing window size or zoom level

### Tests Take Too Long

1. Reduce `DurationTicks` if possible
2. Run headless tests only (skip visual)
3. Use faster hardware

## Best Practices

1. **Start Simple** - Test one behavior at a time
2. **Use Fixed Seeds** - Ensure repeatability
3. **Document Expected Behavior** - Be specific about what should happen
4. **Validate Quantitatively** - Use metrics, not just visual inspection
5. **Test Edge Cases** - Extreme values, boundary conditions
6. **Keep Tests Fast** - Under 20 seconds when possible
7. **Visual Inspection** - Always watch tests visually at least once
8. **Regression Testing** - Re-run tests after code changes

## Integration with CI/CD

Laboratory tests can be integrated into continuous integration:

```bash
# Run all laboratory tests
go test ./internal/game -v -run TestLaboratory -timeout 5m

# Generate test report
go test ./internal/game -v -run TestLaboratory -json > lab-results.json
```

Tests will fail if validation criteria are not met, preventing regressions from being merged.
