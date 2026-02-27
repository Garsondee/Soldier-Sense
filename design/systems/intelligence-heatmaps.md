# Intelligence Heatmap System

**Status:** Core Design
**Priority:** High — foundational to officer decision-making and emergent tactical behaviour

---

## Overview

Soldiers and officers accumulate knowledge through their senses. That knowledge is spatially distributed — you don't just _know_ there's an enemy, you know they were _here_ at _some point_. The Intelligence Heatmap system makes this spatial knowledge explicit and persistent: as soldiers observe the battlefield, they write heat into per-team, per-layer grid maps. Those maps are then consulted by leader AI to make decisions without needing omniscience.

This is the link between **individual perception** and **collective decision-making**. A squad leader doesn't know where every enemy is — they know what their own soldiers have seen, what those soldiers have radioed back, and what has decayed from memory over time.

---

## 1. Conceptual Model

Think of each heatmap layer as a **shared operational picture** — a rough, imperfect, decaying spatial summary of a particular kind of information. Layers are:

- **Team-scoped**: Red and Blue each have their own set of maps. Neither side has access to the other's maps.
- **Grid-aligned**: Same cell resolution as the nav/terrain grid.
- **Multi-layer**: Different kinds of information live on separate maps.
- **Decaying**: Heat values fade over time — old intelligence becomes stale.
- **Writable by individuals, readable by leaders**: Any soldier can contribute heat through perception; leaders query maps during Squad Think.

The maps are _not_ visible to individual soldiers in their decision loop — they are read by leaders only (initially). Eventually, platoon-level maps could be aggregated from squad-level reports.

---

## 2. Heatmap Layers

Each layer has a distinct semantic meaning and its own write rules, decay rate, and use in decision-making.

### 2.1 Layer Table

| Layer | Name | Written When | Decay Rate | Used By |
|-------|------|-------------|------------|---------|
| `ContactHeat` | Enemy Contact | A soldier has LOS to an enemy this tick | Fast (seconds) | Leader: advance/hold/suppress decisions |
| `RecentContactHeat` | Recent Enemy Activity | Enemy was visible < N ticks ago | Medium (tens of seconds) | Leader: caution zones, suppress targets |
| `ThreatDensityHeat` | Threat Density | Scaled by number of enemies visible in cell | Medium | Leader: avoid/suppress priority |
| `FriendlyPresenceHeat` | Friendly Positions | A friendly soldier occupies a cell | Very fast (near-instant) | Leader: formation spacing, support positioning |
| `DangerZoneHeat` | Danger / Suppression | Soldiers receive fire from a direction | Medium | Leader: route avoidance, cover selection |
| `UnexploredHeat` | Unexplored | Areas no friendly has seen | Slow (never decays, set to 0 on sight) | Leader: patrol routing, intel gaps |

Additional layers can be added in future phases (e.g. `CasualtyHeat`, `SoundActivityHeat`, `PlayerObjectiveHeat`).

---

## 3. Data Structure

### 3.1 HeatLayer

A single float32 grid representing one type of spatial information for one team.

```go
// HeatLayer is a 2-D float32 grid of heat values for one intel type, one team.
type HeatLayer struct {
    cells    []float32 // row-major: cells[row*cols+col]
    rows     int
    cols     int
    decayRate float32  // subtracted per tick; 0 = no decay
    maxValue  float32  // values clamped to [0, maxValue]
}
```

### 3.2 IntelMap

The full set of layers for a single team.

```go
// IntelMapKind identifies a specific heat layer.
type IntelMapKind int

const (
    IntelContact           IntelMapKind = iota // active enemy visual contact
    IntelRecentContact                         // enemy seen within recent window
    IntelThreatDensity                         // scaled by enemy count
    IntelFriendlyPresence                      // where friendlies are / were
    IntelDangerZone                            // incoming fire origin estimates
    IntelUnexplored                            // cells no friendly has ever seen
    intelMapCount                              // sentinel — total number of layers
)

// IntelMap holds all heat layers for one team.
type IntelMap struct {
    team   Team
    layers [intelMapCount]*HeatLayer
}
```

### 3.3 IntelStore

The world-level container, one `IntelMap` per team.

```go
// IntelStore owns all intelligence maps for all teams.
// Accessed by the game loop each tick.
type IntelStore struct {
    maps map[Team]*IntelMap
}
```

---

## 4. Write Rules

Heat is written during the **Sense** step (Step 1 of the cognition pipeline), immediately after perceptions are produced. Each write is **additive with clamping** — multiple soldiers seeing the same cell stack heat, up to `maxValue`.

### 4.1 ContactHeat

```
FOR each soldier S on team T:
    FOR each visible enemy E in S.visionContacts:
        cell = worldToCell(E.x, E.y)
        intelMap[T].ContactHeat.add(cell, 1.0)
```

This is written at full strength every tick an enemy is visible. Because it decays fast (~0.05/tick at 60 TPS → gone in ~2 seconds), it represents "enemy is here *right now*".

### 4.2 RecentContactHeat

```
FOR each ThreatFact on S.Blackboard where Confidence > 0.3:
    cell = worldToCell(ThreatFact.X, ThreatFact.Y)
    intelMap[T].RecentContactHeat.add(cell, ThreatFact.Confidence * 0.8)
```

Written from blackboard facts, not raw vision. Decays over ~10-20 seconds. Represents "we know they were around here recently".

### 4.3 ThreatDensityHeat

```
FOR each cell with ContactHeat > 0:
    ThreatDensityHeat.add(cell, ContactHeat.at(cell) * 0.5)
```

Accumulated over time (slower decay). Cells that repeatedly show contact heat build up a persistent density signature — a hot zone that indicates the enemy operates there frequently.

### 4.4 FriendlyPresenceHeat

```
FOR each soldier S on team T:
    cell = worldToCell(S.x, S.y)
    intelMap[T].FriendlyPresence.set(cell, 1.0)
```

Simple stamp of where friendlies are. Used by leaders to avoid clustering orders (spacing) and to identify unsupported flanks.

### 4.5 DangerZoneHeat

```
FOR each soldier S on team T:
    IF S.Blackboard.IncomingFireCount > 0:
        cell = worldToCell(S.x, S.y)
        intelMap[T].DangerZone.add(cell, 0.5 * IncomingFireCount)
```

Cells where friendlies are being shot at accumulate danger heat. Leaders will avoid routing squads through these cells (pathfinding cost modifier) and will direct suppression toward them.

### 4.6 UnexploredHeat

Initialised to `maxValue` (1.0) for every cell at mission start. When any friendly soldier can see a cell, that cell is cleared to 0. It never refills — explored is explored. Used by patrol AI to direct soldiers toward unseen areas.

```
FOR each cell C in S.visibleCells:
    intelMap[T].Unexplored.set(C, 0.0)
```

---

## 5. Decay

Each tick, the game loop calls `intelMap.Decay()` on all layers for all teams.

```
FOR each layer L in IntelMap:
    FOR each cell C in L:
        L.cells[C] = max(0, L.cells[C] - L.decayRate)
```

### 5.1 Decay Rate Reference

| Layer | Decay/Tick (60 TPS) | Approx Time to Zero (from 1.0) |
|-------|---------------------|-------------------------------|
| `ContactHeat` | 0.05 | ~1.2 seconds |
| `RecentContactHeat` | 0.003 | ~30 seconds |
| `ThreatDensityHeat` | 0.0005 | ~3.5 minutes |
| `FriendlyPresenceHeat` | 0.1 | ~0.7 seconds |
| `DangerZoneHeat` | 0.002 | ~45 seconds |
| `UnexploredHeat` | 0.0 | Never (cleared by sight) |

These are tuning parameters. Faster decay = more reactive, more forgetful. Slower decay = persistent, longer memory.

---

## 6. Leader AI Integration (Reader Side)

Leaders query the `IntelMap` during **Squad Think** (Step 3). The maps replace or augment direct blackboard queries, particularly for situations where the leader has no personal LOS but their soldiers' shared observations are pooled.

### 6.1 Query Functions

```go
// SumInRadius returns total heat within a world-space radius around a point.
func (l *HeatLayer) SumInRadius(wx, wy, radius float64) float32

// MaxInRadius returns peak heat value in radius — useful for hotspot detection.
func (l *HeatLayer) MaxInRadius(wx, wy, radius float64) float32

// Centroid returns the heat-weighted centroid of a layer — where the "centre
// of mass" of the intelligence is. Useful for finding the most likely enemy
// concentration point.
func (l *HeatLayer) Centroid() (wx, wy float64, ok bool)

// SampleAt returns the heat value at a specific world position.
func (l *HeatLayer) SampleAt(wx, wy float64) float32
```

### 6.2 Decision Rules Using Maps

These augment (and eventually replace) the rough blackboard-count checks currently in Squad Think:

```
// Advance decision: is the route ahead safe?
IF ContactHeat.SumInRadius(advanceTarget, 150) < 0.1
   AND DangerZone.SumInRadius(currentPos, 100) < 0.05
    → Safe to advance

// Suppress decision: where should fire be directed?
suppressTarget = RecentContactHeat.Centroid()
    → Issue SuppressZone order at suppressTarget

// Withdraw decision: are we in a hot zone?
IF DangerZone.SumInRadius(currentPos, 80) > 0.5
    → Intent = Withdraw toward DangerZone cold direction

// Patrol / exploration routing:
nextPatrolTarget = UnexploredHeat.Centroid()
    → Advance toward least-seen area
```

### 6.3 Platoon-Level Aggregation (Future)

When platoon leaders exist, they will read from a **merged** view of all their squads' maps — each squad's observations contribute to the platoon map, weighted by radio signal quality. A squad that's been out of radio contact stops contributing to the platoon picture until comms are restored.

---

## 7. Propagation — Who Can Write, Who Can Read

This follows the same **no-omniscience** principle as the blackboard:

| Action | Rules |
|--------|-------|
| **Write** | Any soldier writes to their team's maps during Sense. Writes reflect only what that soldier can personally perceive right now. |
| **Read (individual soldier)** | Currently **none** — individual decision-making uses the personal blackboard, not maps. Maps are aggregated knowledge above the individual. |
| **Read (squad leader)** | Full read access to own team's maps. Used in Squad Think. |
| **Read (platoon leader, future)** | Aggregated map fed by squad reports (not direct write access from all soldiers — mediated by comms). |
| **Cross-team** | **Never.** Red cannot read Blue's maps. Information captured from enemy (e.g. radio intercept) would be a separate system. |

---

## 8. Rendering (Debug / Spectator)

The maps should be renderable as optional overlays for the spectator/developer to observe:

- Each layer rendered as a translucent colour wash over the terrain.
- Colour coded by layer type (e.g. red wash for ContactHeat, orange for DangerZone, blue for FriendlyPresence, grey for Unexplored).
- Per-team views can be toggled independently.
- Heat intensity maps to alpha: value 0 = fully transparent, 1.0 = full-colour at configured opacity.

This also sets up the visual language for the eventual **player heatmap painting** overlay (Phase 5), which will use the same rendering pipeline.

---

## 9. Interaction with Existing Systems

| Existing System | Interaction |
|----------------|-------------|
| `Blackboard.UpdateThreats()` | Already fires on vision contacts — heat writes happen at the same point, from the same contact list. |
| `SelectGoal()` | Initially unchanged. Later, individual goals can be nudged by local map queries (e.g. reduce advance utility when ContactHeat ahead is high). |
| `Squad Think` (leader logic) | Primary reader of maps. Replaces the crude `VisibleThreatCount()` checks with richer spatial queries. |
| A* nav grid | `DangerZoneHeat` feeds path cost: cells with high danger heat cost more to traverse, steering squads around hot zones. |
| Player heatmap overlays (Phase 5) | Player-painted overlays become additional write operations on dedicated `PlayerObjective` / `PlayerDanger` layers. These use the same infrastructure and decay/propagation rules. |

---

## 10. Implementation Plan

### Phase A — Foundation (prerequisite: none)

- [ ] `HeatLayer` struct: flat float32 slice, `rows`, `cols`, `decayRate`, `maxValue`
- [ ] `Add(row, col, delta)`, `Set(row, col, v)`, `At(row, col)` methods with clamping
- [ ] `Decay()` method (single pass over all cells)
- [ ] `IntelMapKind` constants and `IntelMap` struct (array of `*HeatLayer`)
- [ ] `IntelStore` with per-team maps
- [ ] `NewIntelStore(rows, cols int, cellSize float64)` — initialises all layers with tuned decay rates
- [ ] World ↔ cell coordinate helpers: `WorldToCell`, `CellToWorld`

### Phase B — Writes (prerequisite: Phase A)

- [ ] In `vision.go` / sense step: write `ContactHeat` from `KnownContacts`
- [ ] In `blackboard.go` `UpdateThreats`: write `RecentContactHeat` from threat facts with confidence
- [ ] Write `FriendlyPresenceHeat` from soldier positions each tick
- [ ] Write `DangerZoneHeat` from `IncomingFireCount > 0` on any soldier
- [ ] Initialise `UnexploredHeat` to 1.0; clear on vision cell scan
- [ ] Accumulate `ThreatDensityHeat` from `ContactHeat` each tick (derived layer)

### Phase C — Leader Reads (prerequisite: Phase B, combat implemented)

- [ ] Implement query functions: `SumInRadius`, `MaxInRadius`, `Centroid`, `SampleAt`
- [ ] Replace rough threat-count logic in leader Squad Think with `ContactHeat.SumInRadius` queries
- [ ] Add `DangerZone` avoidance to advance/withdraw decisions
- [ ] Route suppress orders toward `RecentContactHeat.Centroid()`
- [ ] Steer patrol/advance goals toward `UnexploredHeat.Centroid()`

### Phase D — Nav Integration (prerequisite: Phase C)

- [ ] Add per-cell cost modifier to A* based on `DangerZoneHeat` value at each cell
- [ ] Tune cost weight so squads noticeably skirt hot zones without being paralysed

### Phase E — Debug Rendering (prerequisite: Phase A)

- [ ] Heat layer renderer: translucent alpha-blended colour wash per layer
- [ ] Toggle keys to show/hide individual layers and per-team views
- [ ] Colour legend overlay

---

## 11. Design Principles

1. **No magic knowledge.** Maps are only as good as the observations written into them. An isolated squad contributes nothing to the platoon picture.
2. **Staleness is a feature.** Slow decay means leaders act on outdated intelligence. This is intentional — it's realistic and creates tactical mistakes.
3. **Gradual enrichment.** Start with `ContactHeat` and `RecentContactHeat` driving leader decisions. Add layers incrementally as Phase C+ systems come online.
4. **Same rendering pipeline as player overlays.** The intelligence heatmaps and the player-painted heatmaps are the same kind of thing — a spatial float32 grid rendered as a colour wash. Phase 5 player painting is just another set of write operations into dedicated layers.
5. **Fail gracefully.** If the intel store is nil or a layer is not yet initialised, leader queries return 0 — equivalent to "no known information", which is a safe default.
