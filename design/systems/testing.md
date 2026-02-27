# Testing System — Log-Driven Automated Verification

**Status:** Planning
**Priority:** High — enables rapid LLM-assisted iteration

---

## Problem

Soldier Sense is an emergent behaviour simulation. Traditional assert-based unit tests are useful for pure functions (LOS, pathfinding, utility math) but cannot verify emergent outcomes like "squads halt when contact is made" or "fearful soldiers break formation."

We need a testing system where:

1. An LLM (Windsurf Cascade) can **run tests with a single command** (`go test ./...`).
2. Tests produce **structured log output** the LLM can read and reason about.
3. The LLM can **change code, re-run, compare logs** in a tight loop.
4. Logging covers the full cognition pipeline, not just pass/fail.

---

## Architecture

```
┌──────────────────────────────────────────────────────┐
│                   go test ./...                       │
│                                                       │
│  ┌─────────────┐   ┌──────────────┐   ┌───────────┐ │
│  │  Unit Tests  │   │ Scenario     │   │  Log      │ │
│  │  (pure func) │   │ Simulations  │   │  Analysis │ │
│  │              │   │              │   │           │ │
│  │ LOS, A*,    │   │ Headless N-  │   │ Parse log │ │
│  │ vision cone,│   │ tick runs    │   │ output &  │ │
│  │ utility math│   │ with logging │   │ check     │ │
│  │              │   │              │   │ invariants│ │
│  └─────────────┘   └──────┬───────┘   └─────┬─────┘ │
│                           │                   │       │
│                    writes structured     reads &      │
│                    logs to buffer       asserts on     │
│                           │             log content    │
│                           ▼                   │       │
│                    ┌──────────────┐            │       │
│                    │  SimLog      │◄───────────┘       │
│                    │  (in-memory  │                    │
│                    │   ring buf)  │                    │
│                    └──────────────┘                    │
└──────────────────────────────────────────────────────┘
```

### Three test tiers

| Tier | What | How | Output |
|---|---|---|---|
| **Unit** | Pure functions: LOS, A*, vision cone, utility scoring, stat math | Standard `go test` assertions | Pass/fail + short message |
| **Scenario** | Multi-tick headless simulation with controlled initial conditions | Run N ticks, collect `SimLog` | Structured log dump (always printed via `t.Log`) |
| **Invariant** | Post-hoc checks on scenario logs | Parse `SimLog` entries, assert properties | Pass/fail + relevant log excerpt |

---

## SimLog — The Structured Logger

A test-only logger that records every meaningful event during a headless simulation. Each entry is a flat struct, easy to grep and reason about.

### Log Entry Types

```go
// SimLogEntry is one recorded event during a test simulation.
type SimLogEntry struct {
    Tick     int
    Soldier  string   // label: "R0", "B3"
    Team     string   // "red" or "blue"
    Category string   // see below
    Key      string   // specific event name
    Value    string   // human-readable detail
    NumVal   float64  // optional numeric (for thresholds)
}
```

### Categories & Keys

| Category | Key | When logged | Example Value |
|---|---|---|---|
| `goal` | `change` | Goal changes | `"advance → hold"` |
| `goal` | `select` | Every tick (verbose mode) | `"hold (util=0.72)"` |
| `squad` | `intent_change` | Squad intent changes | `"advance → regroup"` |
| `squad` | `spread` | Every tick (verbose) | `"87.3px"` |
| `squad` | `cohesion_slow` | Leader slows for cohesion | `"0.3x speed"` |
| `move` | `path_start` | New path computed | `"(32,400) → (1248,400), 94 waypoints"` |
| `move` | `path_complete` | Reached end of path | `"arrived at objective"` |
| `move` | `path_fail` | Pathfinding returned nil | `"no path to (600,300)"` |
| `move` | `position` | Every tick (verbose) | `"(245.3, 401.7)"` |
| `vision` | `contact_new` | New enemy spotted | `"spotted B2 at (800,300)"` |
| `vision` | `contact_lost` | Enemy left vision | `"lost B2"` |
| `threat` | `update` | Blackboard threat refresh | `"3 threats, 2 visible"` |
| `threat` | `decay` | Threat confidence decayed | `"B2 confidence 0.45 → 0.40"` |
| `stance` | `change` | Stance transition | `"standing → crouching"` |
| `state` | `change` | SoldierState transition | `"moving → idle"` |
| `stats` | `fear` | Fear changes significantly | `"fear 0.12 → 0.18"` |
| `stats` | `fatigue` | Fatigue snapshot (verbose) | `"fatigue 0.34"` |
| `stats` | `speed` | Effective speed (verbose) | `"1.05 px/tick"` |
| `formation` | `repath` | Formation slot repath | `"slot drift 24.5px, repathing"` |
| `formation` | `slot_pos` | Slot target (verbose) | `"slot → (300, 280)"` |

### Verbosity Levels

- **Normal** (default): Only state changes — goal changes, intent changes, contact gain/loss, stance changes, path events.
- **Verbose**: Everything, every tick. Position, speed, fatigue, per-tick utility scores. Produces large output but useful for debugging specific tick ranges.

The LLM can request verbose mode by passing `-v` to `go test` or by setting a test flag.

---

## Headless Simulation Harness

The key enabler: a way to construct a `Game`-like state without Ebiten, run N ticks, and collect logs.

### Design

```go
// TestSim is a headless simulation harness for testing.
type TestSim struct {
    Width, Height int
    Buildings     []rect
    NavGrid       *NavGrid
    Soldiers      []*Soldier  // all soldiers (both teams)
    Squads        []*Squad
    SimLog        *SimLog
    Tick          int
    Verbose       bool
}
```

**Key methods:**

| Method | Purpose |
|---|---|
| `NewTestSim(opts ...SimOption)` | Create a sim with builder-pattern options |
| `AddSoldier(id, x, y, team, targetX, targetY)` | Place a soldier with patrol endpoints |
| `AddBuilding(x, y, w, h)` | Place an obstacle |
| `FormSquad(team, soldierIDs)` | Group soldiers into a squad |
| `RunTicks(n)` | Advance simulation N ticks, logging everything |
| `RunUntil(predicate, maxTicks)` | Advance until predicate is true or max ticks |
| `Log() []SimLogEntry` | Get all log entries |
| `LogFilter(category, key) []SimLogEntry` | Filter log entries |
| `DumpLog(t *testing.T)` | Print full log to test output |
| `DumpLogRange(t, fromTick, toTick)` | Print log for a tick range |
| `Snapshot() SimSnapshot` | Capture current positions/states of all soldiers |

### SimOption Builder

```go
sim := NewTestSim(
    WithMapSize(800, 600),
    WithBuilding(200, 200, 64, 64),
    WithRedSoldier(0, 50, 300, 750, 300),   // id, startX, startY, targetX, targetY
    WithBlueSoldier(6, 750, 300, 50, 300),
    WithSquad(TeamRed, []int{0, 1, 2}),
    WithVerbose(true),
    WithSeed(42),   // deterministic RNG
)
```

`WithSeed` is critical — makes scenarios reproducible. The current code uses `time.Now()` seeds; the test harness overrides with a fixed seed.

---

## Test Scenarios

### Tier 1: Unit Tests (pure functions, no simulation)

These are standard Go tests. They already implicitly exist for the math but aren't written yet.

| Test | File | What it verifies |
|---|---|---|
| `TestLOS_ClearLine` | `los_test.go` | Unobstructed LOS returns true |
| `TestLOS_BlockedByBuilding` | `los_test.go` | Building blocks LOS |
| `TestLOS_EdgeCases` | `los_test.go` | Vertical/horizontal/zero-length rays |
| `TestNavGrid_BlockedCells` | `navmesh_test.go` | Buildings mark correct cells |
| `TestNavGrid_FindPath_Basic` | `navmesh_test.go` | A* finds a path around a building |
| `TestNavGrid_FindPath_NoPath` | `navmesh_test.go` | Returns nil when impossible |
| `TestVisionCone_InCone` | `vision_test.go` | Points inside/outside FOV arc |
| `TestVisionCone_Range` | `vision_test.go` | Max range respected |
| `TestNormalizeAngle` | `vision_test.go` | Wraps to [-π, π] |
| `TestSelectGoal_NoThreats` | `blackboard_test.go` | Advance or formation when clear |
| `TestSelectGoal_HighFear` | `blackboard_test.go` | Survive wins when fear is high |
| `TestSelectGoal_HoldIntent` | `blackboard_test.go` | Hold wins when squad says hold |
| `TestEffectiveSpeed` | `stats_test.go` | Speed multipliers stack correctly |
| `TestEffectiveAccuracy` | `stats_test.go` | Accuracy factors combine correctly |
| `TestFatigue_Accumulation` | `stats_test.go` | Fatigue rises with exertion |
| `TestFear_Recovery` | `stats_test.go` | Fear decays over time |
| `TestFormationOffsets` | `formation_test.go` | Wedge/line/column slot positions |
| `TestSlotWorld` | `formation_test.go` | Local→world coordinate transform |
| `TestBlackboard_ThreatDecay` | `blackboard_test.go` | Stale threats lose confidence |
| `TestBlackboard_ThreatRefresh` | `blackboard_test.go` | Visible threats stay at 1.0 |
| `TestSquadSpread` | `squad_test.go` | Max member distance calculated correctly |
| `TestLeaderCohesionSlowdown` | `squad_test.go` | Speed reduction thresholds |

### Tier 2: Scenario Simulations (headless, logged)

Each scenario sets up a controlled situation, runs ticks, and dumps logs. The logs are **always printed** so the LLM can read them. Some scenarios also have invariant checks.

| Scenario | Setup | Ticks | What to observe in logs |
|---|---|---|---|
| `TestScenario_AdvanceNoContact` | 6 red soldiers, no blue, no buildings, wedge formation | 500 | All reds advance across map. Formation maintained. No goal changes to survive/hold. |
| `TestScenario_AdvanceWithBuildings` | 6 red, no blue, 3 buildings | 500 | Paths route around buildings. No stuck soldiers (all reach objective or keep moving). |
| `TestScenario_ContactHalt` | 6 red advancing, 1 blue stationary in their path | 300 | Red squad transitions to Hold intent when blue is spotted. Members switch to hold/survive goals. |
| `TestScenario_MutualAdvance` | 6 red vs 6 blue advancing toward each other | 600 | Both squads detect each other. Intent changes. Goal distribution shifts. |
| `TestScenario_FearfulSoldier` | 1 soldier with fear=0.9, composure=0.1, facing 2 enemies | 100 | Survive goal dominates. Soldier crouches. Doesn't advance. |
| `TestScenario_DisciplinedSquad` | 6 soldiers with discipline=0.9, under hold intent, enemies visible | 200 | All members hold position. No breaking formation. |
| `TestScenario_UndisciplinedSquad` | 6 soldiers with discipline=0.2, fear=0.5, enemies visible | 200 | Some members override orders, seek cover. Goal diversity. |
| `TestScenario_FormationCohesion` | Leader + 5 members, wedge, leader advances | 300 | Members follow formation slots. Repath events logged. Spread stays below threshold. |
| `TestScenario_LeaderSlowdown` | Leader sprints ahead, members lag | 200 | Leader slowdown kicks in at spread thresholds. Logged speed reductions. |
| `TestScenario_RegroupOrder` | Squad spread > 120px, leader sets regroup | 200 | Regroup intent issued. Members converge. Spread decreases over time. |
| `TestScenario_ThreatDecayMemory` | Soldier sees enemy, enemy moves behind building | 400 | Contact logged, then lost. Threat confidence decays. Eventually removed from blackboard. |
| `TestScenario_VisionConeBlind` | Enemy directly behind soldier (outside FOV) | 100 | No contact detected. Soldier continues unaware. |

### Tier 3: Invariant Checks (post-hoc log analysis)

These run after a scenario and assert properties on the collected logs.

| Invariant | Applied to | Check |
|---|---|---|
| **No stuck soldiers** | Any advance scenario | Every moving soldier's position changes over any 50-tick window (unless in cover/idle) |
| **Formation bounded** | Formation scenarios | Squad spread never exceeds 150px for more than 20 consecutive ticks |
| **Goal coherence** | All scenarios | No soldier flips goal more than 5 times in 10 ticks (oscillation detector) |
| **Threat bookkeeping** | Contact scenarios | Visible threat count never exceeds actual enemies in FOV |
| **Fear bounded** | All scenarios | Fear never exceeds 1.0 or goes below 0.0 |
| **Dead soldiers don't act** | Combat scenarios (later) | No goal/move/stance log entries for dead soldiers |
| **Intent propagation** | Squad scenarios | When leader changes intent, all alive members receive it within 1 tick |

---

## File Layout

```
internal/
  game/
    sim_log.go            # SimLog, SimLogEntry, filtering, formatting
    test_harness.go       # TestSim, SimOption builders, RunTicks, etc.
    los_test.go           # Tier 1: LOS unit tests
    navmesh_test.go       # Tier 1: pathfinding unit tests
    vision_test.go        # Tier 1: vision cone unit tests
    blackboard_test.go    # Tier 1: goal selection & threat tests
    stats_test.go         # Tier 1: stats math tests
    formation_test.go     # Tier 1: formation offset tests
    squad_test.go         # Tier 1: squad logic tests
    scenario_test.go      # Tier 2: all scenario simulations
    invariant_test.go     # Tier 3: invariant checkers (helpers)
```

`sim_log.go` and `test_harness.go` are production code (in package `game`) but only used by tests. They contain no Ebiten imports. Alternatively, they can live in a `game_test` package alongside the test files — either works.

---

## LLM Workflow

The intended workflow for Cascade (or any LLM in Windsurf):

### 1. Run all tests

```
go test ./internal/game/ -v -count=1 2>&1
```

`-v` prints all `t.Log` output (including full simulation logs).
`-count=1` disables test caching so logs are always fresh.

### 2. Run a single scenario

```
go test ./internal/game/ -v -run TestScenario_ContactHalt -count=1 2>&1
```

### 3. Read the log output

The LLM reads structured log lines like:

```
[T=042] R0 squad   intent_change  advance → hold
[T=042] R0 goal    change         advance → hold
[T=043] R1 goal    change         formation → hold
[T=043] R2 goal    change         formation → survive
[T=043] R2 stance  change         standing → crouching
```

### 4. Identify the issue

From the logs, the LLM can see:
- R2 went to `survive` instead of `hold` — maybe fear is too high or discipline too low.
- Or: that's actually correct emergent behaviour for an undisciplined soldier.

### 5. Make a code change

Edit utility weights, stat ranges, decay rates, etc.

### 6. Re-run and compare

Run the same test again, diff the log output. Verify the change had the intended effect without breaking other scenarios.

### Workflow command summary for Cascade

| Step | Command |
|---|---|
| Run all | `go test ./internal/game/ -v -count=1 2>&1` |
| Run one | `go test ./internal/game/ -v -run TestScenario_X -count=1 2>&1` |
| Run unit only | `go test ./internal/game/ -v -run "^Test[^S]" -count=1 2>&1` |
| Run scenarios only | `go test ./internal/game/ -v -run "^TestScenario" -count=1 2>&1` |

---

## Log Format Spec

All scenario log output uses a fixed-width format for easy parsing:

```
[T=DDD] LL CCCCCCCC KKKKKKKKKKKKKK VVVVVVVV
```

Where:
- `T=DDD` — tick number, zero-padded to 3 digits
- `LL` — soldier label (R0, B3, etc.) or `--` for global events
- `CCCCCCCC` — category, left-padded to 8 chars
- `KKKKKKKKKKKKKK` — key, left-padded to 14 chars
- `VVVVVVVV` — freeform value string

Example output block:

```
=== TestScenario_ContactHalt ===
--- Setup: 6 red (wedge) advancing east, 1 blue stationary at (600,300) ---
[T=000] R0    squad   intent_change  → advance
[T=000] R0     move     path_start   (32,300) → (1248,300), 76 wp
[T=000] R1     move     path_start   following formation slot
...
[T=038] R0   vision   contact_new    spotted B0 at (600,300), dist=214
[T=038] R0    squad   intent_change  advance → hold
[T=038] R0     goal        change    advance → hold
[T=039] R1     goal        change    formation → hold
[T=039] R2     goal        change    formation → survive
[T=039] R2   stance        change    standing → crouching
[T=039] R3     goal        change    formation → hold
...
--- Summary at T=300 ---
Red goals:  hold=4  survive=2  advance=0  formation=0
Red spread: 34.2px (from 87.1px at T=038)
Blue alive:  1
Contacts:    R0→B0, R3→B0
```

The summary block at the end gives the LLM a quick snapshot without reading every line.

---

## Determinism & Seed Control

All test scenarios use a **fixed RNG seed** so that:

- Results are reproducible across runs.
- The LLM can compare output before/after a code change knowing the randomness is identical.
- Flaky tests are eliminated.

The `WithSeed(n)` option seeds all RNG in the test harness. The current `time.Now()` seeds in production code are not changed — only the test harness overrides them.

---

## Implementation Order

1. **`sim_log.go`** — SimLog struct, entry types, formatting, filtering.
2. **`test_harness.go`** — TestSim, options, RunTicks, headless tick loop (mirrors `Game.Update` but without Ebiten).
3. **Tier 1 unit tests** — Start with LOS, navmesh, vision (most self-contained).
4. **One scenario** — `TestScenario_AdvanceNoContact` as proof of concept.
5. **Invariant helpers** — `CheckNoStuck`, `CheckGoalOscillation`.
6. **Remaining scenarios** — Build out scenario catalogue.
7. **Iterate** — Add new scenarios as new systems are implemented (combat, sound, radio).

---

## Integration with Existing ThoughtLog

The existing `ThoughtLog` (in-game UI panel) and `SimLog` (test-only) are separate systems:

- `ThoughtLog` — visual, ring-buffer, rendered on screen. Stays as-is.
- `SimLog` — structured, unbounded (within test), machine-readable. Test-only.

Soldiers call `s.think(msg)` for the UI log. In test mode, the harness also writes to `SimLog` with richer structured data. The `think()` method is not changed — instead, the test harness instruments the tick loop to capture events at each stage of the cognition pipeline.

---

## Design Principles

1. **Logging is the primary output.** Tests that run scenarios always print logs, even when passing. The LLM needs the data.
2. **Structured over freeform.** Every log entry has typed fields. No printf debugging.
3. **Deterministic.** Fixed seeds. No flaky tests.
4. **Headless.** No Ebiten dependency in tests. Pure simulation logic only.
5. **Additive.** New systems (combat, sound, radio) add new log categories. Existing tests don't break.
6. **Fast.** Scenarios run hundreds of ticks in milliseconds. The LLM can run the full suite on every iteration.
