# Performance Investigation Log

## Scope and priority

This document records ongoing performance investigation findings across the project.

Priority order for investigation:
1. Rendering performance (highest priority).
2. Simulation performance.

## Method

Investigation so far is static code-path analysis (no runtime profiling yet). Focused on frame-critical draw paths and obvious hot loops.

## Rendering pipeline overview

The main runtime path renders every frame through `Game.Draw`, drawing the whole world into `worldBuf` and then compositing to screen with camera transform.

Key entry points:
- `internal/game/game.go` (`Draw`, `drawWorld`).
- `internal/game/soldier.go` (`Soldier.Draw`).
- `internal/game/draw_overlays.go` (orders, intent lines, labels, selected soldier info).
- `internal/game/thoughtlog.go` (`ThoughtLog.Draw`).

## High-confidence rendering hotspots

### 1) Full-map per-tile ground redraw every frame

`drawWorld` iterates all tile rows/cols and issues one `vector.FillRect` per tile each frame, with per-tile colour jitter/hash work.

Evidence:
- `internal/game/game.go` in `drawWorld` tile loops (`g.tileMap.Rows x g.tileMap.Cols`).
- Includes per-tile `terrainHash` and colour adjustment before draw.

Why this is expensive:
- Very high primitive count every frame.
- Done regardless of camera zoom/visible sub-region.

### 2) Multiple additional full-map passes in the same frame

After base ground pass, additional broad loops/passes are drawn every frame:
- Three grid layers (`drawGridOffset` called 3 times).
- Road centre-line pass scanning tarmac tiles.
- Tile-object pass (`drawTileMapObjects`) scanning all tiles again.

Evidence:
- `internal/game/game.go` (`drawWorld`, `drawTileMapObjects`, `drawGridOffset`).

Impact pattern:
- Repeated full-map scans in one frame create cumulative CPU and draw-call overhead.

### 3) Vision-cone rendering has heavy geometric work per soldier

Each frame/team:
- `visionBuf.Clear()`.
- For each alive soldier, builds a path with 36 steps.
- Each step calls `clipVisionRayToBuildings`, which currently linearly scans all buildings (`for _, b := range g.buildings`).

Evidence:
- `internal/game/game.go` (`drawVisionConesBuffered`, `clipVisionRayToBuildings`).

Impact pattern:
- Work scales with soldiers × steps × buildings.
- Happens every frame even if camera is zoomed into a small region.

### 4) Soldier draw complexity is high per entity

`Soldier.Draw` performs many vector operations per soldier (multiple circles/strokes/stance-dependent overlays/health visuals).

Evidence:
- `internal/game/soldier.go` (`func (s *Soldier) Draw`).

Impact pattern:
- Per-entity visual richness directly multiplies frame cost at larger unit counts.

### 5) Overlays add additional per-frame loops and text drawing

World-space overlays draw per squad/per soldier:
- Officer orders.
- Movement intent dashed lines.
- Squad intent labels.
- Selected soldier info panel.
- Radio visual effects (segmented curved arc strokes with jitter).

Evidence:
- `internal/game/draw_overlays.go`.
- `internal/game/game.go` (`drawRadioVisualEffects`).

Impact pattern:
- Optional/debug-style overlays are currently always drawn when features are active.

### 6) UI panels are redrawn each frame

Even when content changes slowly, these are redrawn every frame:
- Squad status panel(s) (`drawSquadStatusPanels` + `renderSquadPanel`).
- Thought log panel (`ThoughtLog.Draw`).
- HUD (`drawHUD` when enabled).

Evidence:
- `internal/game/game.go`.
- `internal/game/thoughtlog.go`.

Impact pattern:
- Stable UI work contributes to frame time and can be converted to dirty-redraw later.

## Notable nuance vs existing report

There is already a document at `docs/rendering-performance-report.md`. Some of its recommendations are still directionally useful, but parts appear stale relative to current code.

Example:
- It flags per-frame allocation of claimed/solid maps, but `Game` now keeps reusable cached maps (`cachedClaimedTeam`, `cachedSolidSet`) and clears/reuses them in draw code.

This investigation log should be treated as the current source of truth and kept updated with profiling evidence.

## Simulation-side initial findings (lower priority)

Simulation still contains heavy loops, but rendering remains the current top optimisation target.

Observed simulation-heavy areas:
- `simTick` does multiple whole-team passes each tick.
- Combat and sensing paths still perform significant per-entity work.

Evidence:
- `internal/game/game.go` (`simTick`).

## Immediate profiling plan (next investigation step)

Before changing code, gather quantitative evidence:

1. Add coarse frame timing buckets (draw world, overlays, UI panels).
2. Add counters for:
   - Ground tile draw count.
   - Vision ray clip calls.
   - Tile-object draw iterations.
   - Overlay primitive counts.
3. Capture frame-time/FPS under controlled scenarios (low/medium/high entity counts).
4. Rank by measured frame-time contribution.

## Candidate optimisation themes (for later implementation phase)

Rendering-first candidate themes:
1. Static/background layer caching (ground/grid/static building details).
2. Camera-aware culling of world passes and overlays.
3. Vision clipping acceleration structure (avoid full building scan per ray).
4. Dirty redraw for right-column UI and HUD.
5. Optional quality tiers/toggles for expensive overlays.

## Additional investigation findings (round 2)

### Rendering: newly identified hotspots

#### 7) Speech bubble rendering has nested per-bubble soldier scans

`drawSpeechBubbles` currently does more than just draw active bubbles:
- Allocates an `occupied` map every frame.
- For each bubble, rebuilds a combined `all` soldier slice (`soldiers + opfor`).
- For each bubble, scans that slice to avoid overlap and potentially shifts bubble X.

Evidence:
- `internal/game/speech.go` (`drawSpeechBubbles`).

Impact pattern:
- Cost grows with bubble count × soldier count.
- Includes repeated per-frame allocations in a draw path.

#### 8) Cover rendering rebuilds chest-wall neighbour map each frame

`drawCoverObjects` rebuilds a `chestSet` map from all cover objects on every frame before rendering chest-wall neighbour-aware geometry.

Evidence:
- `internal/game/game.go` (`drawCoverObjects`).

Impact pattern:
- Repeated map allocation and insertion in a frame-critical path.
- Work repeats even when cover layout is static.

#### 9) Heatmap overlay rendering scans full layer per enabled intel map

`drawHeatLayer` loops all layer cells and emits `vector.FillRect` for visible cells. In `drawWorld`, this runs once per enabled overlay kind.

Evidence:
- `internal/game/game.go` (`drawWorld`, `drawHeatLayer`).

Impact pattern:
- Worst case scales with map cells × enabled overlays.
- Adds another broad pass over the world-space grid.

#### 10) Tracer and muzzle-flash visuals are intentionally expensive per effect

Tracer and flash draw routines use multiple stroke/fill layers per effect for glow quality.

Evidence:
- `internal/game/combat.go` (`DrawTracer`, `DrawMuzzleFlashes`).

Impact pattern:
- Cost depends on active fire volume.
- Likely a major spike contributor during sustained firefights.

### Simulation: deeper secondary hotspots (still lower priority than rendering)

#### 11) Vision scan still has expensive internals despite spatial hash prefilter

`UpdateVisionSpatial` reduces candidate count, but `PerformVisionScan` remains heavy per candidate:
- LOS call via `HasLineOfSightWithCover`.
- Threat lookup by linear scan over existing threat list.
- Per-tick `inConeThisTick` map allocation.

Evidence:
- `internal/game/soldier.go` (`UpdateVisionSpatial`).
- `internal/game/vision.go` (`PerformVisionScan`).

Impact pattern:
- Better than global brute force, but still significant under high contact density.

#### 12) Peek scan adds extra LOS checks and linear contact dedupe

When wall-adjacent, `peekScan` iterates nearby enemies, performs linear `KnownContacts` dedupe checks, and can perform additional LOS checks.

Evidence:
- `internal/game/soldier.go` (`peekScan`).

Impact pattern:
- Conditional cost spikes for soldiers in corner/doorway states.

## Revised profiling instrumentation shortlist

Add counters/timers for the newly identified paths before implementation changes:

1. Speech bubble draw:
   - bubble count, overlap checks, per-frame allocations.
2. Cover draw:
   - cover count, chest-wall count, chest-set rebuild time.
3. Heat overlays:
   - enabled overlay count, per-layer non-zero tile count, time per layer.
4. Combat effects:
   - active tracers/flashes, draw time during combat bursts.
5. Vision internals (simulation):
   - candidate count post-spatial-hash, LOS checks, threat-list lengths.

## Additional investigation findings (round 3)

### Rendering and frame-time adjacent findings

#### 13) LOS helper is globally hot and currently brute-force over geometry

`HasLineOfSight` and `HasLineOfSightWithCover` both linearly scan full building/cover slices. These are called from multiple systems (vision, combat, sound occlusion, path smoothing, vision-cone clipping).

Evidence:
- `internal/game/los.go`.
- Call sites across `soldier.go`, `combat.go`, and `game.go`.

Impact pattern:
- Even when each caller is individually "reasonable", aggregate LOS cost is likely large.
- A shared acceleration structure would benefit several subsystems at once.

#### 14) Heat layers are touched repeatedly in the same tick by both simulation and draw

Within simulation, intel layers are decayed and recomputed every tick, then sampled repeatedly by soldiers/squads. In draw, enabled overlays scan heat layers again for visualisation.

Evidence:
- `internal/game/intel.go` (`IntelStore.Update`, `Decay`, `computeSafeTerritory`, `clearVisibleCells`, `SumInRadius`).
- `internal/game/game.go` (`drawHeatLayer`).

Impact pattern:
- High cache/memory bandwidth pressure from repeated full-layer traversals.
- Layer-heavy scenarios likely produce frame-time variance when overlays are enabled.

#### 15) Laboratory visual mode has avoidable text allocation in draw path

`drawBasicText` constructs a new font face on every text draw call.

Evidence:
- `internal/game/laboratory_visual.go` (`drawBasicText`).

Impact pattern:
- Not primary game mode, but this can distort perceived performance during diagnostics and test visualisation.

### Simulation-side newly identified hotspots

#### 16) Intel update performs multiple full-grid passes every tick

`IntelStore.Update` currently:
- decays all layers,
- writes soldier-derived updates,
- accumulates threat density over full layer,
- recomputes safe territory over full layer.

Evidence:
- `internal/game/intel.go` (`Update`, `AccumulateThreatDensity`, `computeSafeTerritory`).

Impact pattern:
- Scales with map cell count regardless of contact density.

#### 17) Fog-of-war clearing is per-soldier ray fan work every tick

`clearVisibleCells` samples a cone grid for each soldier each tick (`angularSteps` × `steps`).

Evidence:
- `internal/game/intel.go` (`clearVisibleCells`).

Impact pattern:
- Cost scales directly with alive soldier count.
- Independent of whether tactical state changed meaningfully from previous tick.

#### 18) Tactical scoring uses repeated radial aggregation scans

Soldier think logic repeatedly calls `SumInRadius` over several intel layers, then calls tactical nearby scans. `SumInRadius` itself loops bounded regions with distance checks.

Evidence:
- `internal/game/soldier.go` around local search-drive computation and position scan.
- `internal/game/intel.go` (`SumInRadius`).
- `internal/game/tactical_map.go` (`ScanBestNearby`).

Impact pattern:
- Potentially significant cost in high-unit scenes due to repeated area summations.

#### 19) Pathfinding and movement smoothing stack multiple expensive checks

Movement and formation flows can trigger:
- repeated `FindPath` calls (A* allocates/open/closed maps each call),
- per-move look-ahead LOS checks,
- per-step cover scans over all cover objects.

Evidence:
- `internal/game/navmesh.go` (`FindPath`).
- `internal/game/soldier.go` (`moveAlongPath`).
- `internal/game/squad.go` formation repath path.

Impact pattern:
- Under frequent replanning, this can become a major simulation-side CPU contributor.

#### 20) Sightline scoring is intentionally expensive and used in active soldier updates

`ScoreSightline` casts multiple rays and also checks ray/building intersections. It is throttled, but still runs in active soldier thinking.

Evidence:
- `internal/game/sightlines.go`.
- `internal/game/soldier.go` (periodic `ScoreSightline` update).

Impact pattern:
- Burst cost every sightline update interval across many soldiers.

## Expanded measurement checklist

Add measurement points for these additional suspects:

1. Global LOS:
   - total LOS calls per tick/frame by subsystem (vision/combat/render/sound/movement).
2. Intel passes:
   - per-tick time for `Decay`, `AccumulateThreatDensity`, `computeSafeTerritory`, and `clearVisibleCells`.
3. Tactical aggregation:
   - `SumInRadius` call count and average scanned cells per call.
4. Pathing:
   - `FindPath` call count, mean expanded nodes, and time distribution.
5. Sightline:
   - calls per second and average runtime per call.

## Additional investigation findings (round 4: allocation and churn sweep)

### Allocation-heavy patterns in hot paths

#### 21) Per-frame input map allocation in `handleInput`

`handleInput` creates a fresh `currentKeys` map every frame.

Evidence:
- `internal/game/game.go` (`handleInput`, `currentKeys := map[ebiten.Key]bool{}`).

Impact pattern:
- Small per-frame allocation, but guaranteed every frame.
- Also pairs with repeated key-state writes for many keys.

#### 22) Per-tick combined soldier slice allocation in `simTick`

`simTick` creates `all := make([]*Soldier, 0, len(g.soldiers)+len(g.opfor))` each tick and appends both teams.

Evidence:
- `internal/game/game.go` (`simTick`).

Impact pattern:
- Predictable recurring allocation every simulation tick.

#### 23) Speech + overlay code repeatedly builds temporary team-combined slices

Several paths allocate temporary combined soldier slices (`soldiers + opfor`) in frequently called functions.

Evidence:
- `internal/game/speech.go` (`drawSpeechBubbles`, overlap checks).
- `internal/game/draw_overlays.go` (`drawMovementIntentLines`).
- `internal/game/inspector.go` (`handleInspectorClick`).

Impact pattern:
- Repeated allocation + append churn across frame-time sensitive paths.

#### 24) Thought log draw allocates filtered entry slice every frame

`ThoughtLog.Recent` allocates a result slice and rebuilds filtered entries; `ThoughtLog.Draw` calls it for panel rendering.

Evidence:
- `internal/game/thoughtlog.go` (`Recent`, `Draw`).

Impact pattern:
- Repeated allocation in a per-frame UI panel path.

#### 25) Vision scan allocates map per soldier scan

`PerformVisionScan` allocates `inConeThisTick := make(map[*Soldier]bool)` each scan call.

Evidence:
- `internal/game/vision.go` (`PerformVisionScan`).

Impact pattern:
- High-frequency allocation in one of the busiest simulation paths.

#### 26) Squad helper methods return newly allocated slices used in runtime loops

`sq.Alive()` builds and returns a new `[]*Soldier`. This is convenient, but can add churn when called from hot logic/render helpers.

Evidence:
- `internal/game/squad.go` (`Alive`).

Impact pattern:
- Additional allocations proportional to call frequency and squad count.

#### 27) Formation update allocates maps/slices during update

Formation/positioning paths allocate `assigned` maps and temporary position slices during formation updates.

Evidence:
- `internal/game/squad.go` (`computeOrderPositions`, `UpdateFormation`).

Impact pattern:
- Allocation spikes during formation-heavy behaviour windows.

### String/format churn worth measuring

#### 28) HUD and panel text formatting rebuilds strings every draw

HUD, pause/AAR panels, squad panels, and selected-soldier overlays build multiple formatted strings (`fmt.Sprintf`) per frame.

Evidence:
- `internal/game/game.go` (`drawHUD`, `drawPauseMenu`, `drawAAR`, squad panel rendering helpers).
- `internal/game/draw_overlays.go` (selected/squad labels).

Impact pattern:
- May increase GC pressure and CPU time in UI-heavy scenes.

### Allocation-specific profiling additions

Add explicit allocation counters in profiling runs:

1. Per-frame allocations/bytes for:
   - `handleInput`, `drawHUD`, `ThoughtLog.Draw`, `drawSpeechBubbles`.
2. Per-tick allocations/bytes for:
   - `simTick`, `PerformVisionScan`, formation update paths.
3. Count of temporary combined-soldier-slice constructions per frame/tick.
4. Count of `fmt.Sprintf` calls in draw paths (coarse instrumentation acceptable).

Recent code changes have started addressing the allocation findings listed above; continue validating impact with measured profiling.

## Performance improvement checklist (recommended order)

### Phase 1 — Baseline and low-risk wins

- [ ] Capture a reproducible baseline (frame time, sim tick time, allocs/op, GC pause, entity counts).
- [x] Add lightweight timers/counters around the known hotspots listed in this document.
  - [x] Added rolling timing buckets for simulation tick runtime, world draw runtime, and UI draw runtime.
  - [x] Surfaced averaged timing summary in HUD for quick in-run hotspot triage.
- [x] Remove guaranteed per-frame/per-tick allocations in hot paths first:
  - [x] Reuse input key-state storage in `handleInput`.
  - [x] Reuse/retain combined soldier slices used in `simTick` and common draw helpers.
  - [x] Reuse scratch structures in vision scan (`inConeThisTick`) where safe.
  - [x] Reuse chest-wall neighbour lookup storage in `drawCoverObjects` (avoid per-frame chest-set allocation).
- [x] Cache or reuse frequently rebuilt UI text where values are unchanged frame-to-frame.
  - [x] Reuse HUD overlay/filter text lines and scratch line storage; rebuild only when toggle state changes.

### Phase 2 — High-impact shared systems

- [x] Implement a shared LOS acceleration strategy (spatial bins / broad-phase candidate pruning).
  - [x] Added shared `LOSIndex` broad-phase bins for buildings + LOS-blocking cover.
  - [x] Added indexed LOS helpers while preserving legacy wrappers for compatibility.
- [x] Route major LOS consumers to the same accelerated query path.
  - [x] Vision scan + peek scan now use indexed LOS checks.
  - [x] Combat LOS checks (fire lines and gunfire occlusion) now use indexed LOS checks.
  - [x] Squad ally-visibility/fear aggregation LOS checks now use indexed LOS checks.
  - [x] Vision-cone ray clipping now uses indexed building candidates.
- [ ] Validate LOS correctness parity with existing behaviour (windows, cover classes, edge cases).
  - [x] `go test ./internal/game/...` passes after integration.
  - [ ] Add/extend explicit parity tests for indexed vs non-indexed paths (including window LOS helper parity).

### Phase 3 — Intel and tactical map cost reduction

- [ ] Reduce full-grid intel passes where possible (dirty regions / staggered updates / lower-frequency derived layers).
  - [x] Staggered `IntelSafeTerritory` recompute to one team map per tick during normal two-team simulation updates.
  - [x] Staggered `IntelThreatDensity` accumulation to the same one-team-per-tick schedule during normal two-team updates.
  - [x] Preserved full recompute fallback when one side is absent (tests/sandbox scenarios) to avoid stale safe-territory state.
- [x] Optimise fog-of-war clearing (`clearVisibleCells`) to avoid redundant writes.
  - [x] Added per-scan stamped seen-cell dedupe in `IntelStore` so repeated cone samples do not rewrite the same unexplored cell.
- [ ] Replace repeated `SumInRadius` scans with cheaper aggregation (prefix sums, cached tiles, or quantised rings).
  - [x] Reduced `SumInRadius`/`MaxInRadius` inner-loop cost by switching to cell-space delta math (removed per-cell `CellToWorld` conversion).
- [ ] Re-profile tactical decision paths after intel-layer changes.

### Phase 4 — Pathfinding and movement pipeline

- [ ] Add pathfinding counters (calls, expanded nodes, fail rate, average/95p runtime).
  - [x] Added `NavGrid` pathfinding counters and runtime sampling (`calls`, `failures`, `expanded nodes`, `avg ms`, `p95 ms`).
  - [x] Surfaced per-run pathfinding summary line in `cmd/headless-report` output.
- [ ] Reduce avoidable repaths (slot drift thresholds, cooldowns, shared squad-level path intents).
  - [x] Added moderate-drift repath cooldown in `moveToContact` while preserving immediate repath on missing/terminal paths and large target drift.
  - [x] Added formation repath cooldown gating in `UpdateFormation` for moderate slot drift, with forced repath on missing paths/large drift.
- [ ] Optimise path smoothing/look-ahead LOS checks (limit frequency, early exits, caching).
  - [x] Switched movement path-smoothing LOS checks in `moveAlongPath` to indexed LOS queries when available.
  - [x] Added smoothing target/cache reuse with tick-throttled recalculation (`pathLookaheadRecalcTicks`) to reduce repeated LOS scans.
- [ ] Verify no regression in formation cohesion and responsiveness.

### Phase 5 — Rendering pass consolidation

- [ ] Prioritise culling/visibility limits for expensive world draw loops.
  - [x] Added camera-aware culling to speech bubble rendering and overlap checks so off-screen soldiers/bubbles are skipped.
  - [x] Stopped rebuilding chest-wall neighbour lookup every frame; chest-wall set is now rebuilt once and reused while cover layout is unchanged.
- [ ] Reduce redundant full-map passes (terrain overlays, road details, heat overlays) when not visible.
  - [x] Switched terrain cache blit to camera-visible sub-rect (`drawTerrainLayerVisible`) instead of drawing full-map terrain buffer every frame.
  - [x] Added low-zoom gate for heat overlays (`camZoom >= 0.35`) to skip expensive overlay passes when not meaningfully readable.
- [ ] Trim high-cost vector-heavy effects under load (adaptive quality for tracers/glow/labels).
- [ ] Confirm visual fidelity remains acceptable at target frame budgets.

### Phase 6 — Validation and guardrails

- [ ] Re-run baseline scenario and compare before/after metrics side-by-side.
- [ ] Add regression benchmarks for top hotspots (LOS, intel update, pathfinding, speech/UI draw).
- [ ] Define performance budgets and fail thresholds for CI/report tooling.
- [ ] Document final optimisation decisions and trade-offs in this file.

## Baseline snapshot (2026-03-04)

Captured with:

`go run ./cmd/headless-report -runs 5 -ticks 3600 -seed-base 42 -seed-step 1`

### Perf summary (avg/min/max over 5 runs)

- setup: `30.88352ms / 22.0396ms / 41.1687ms`
- sim: `3.08835624s / 2.3073104s / 4.0175575s`
- post: `1.6042ms / 1.0377ms / 2.1423ms`
- total: `3.12084396s / 2.3505871s / 4.0439083s`

Derived from the same report output:

- sim throughput (aggregate avg): ~`1166 ticks/sec` (`3600 / 3.08835624`)
- sim cost per tick (aggregate avg): ~`857.9 µs/tick`

### Scenario aggregate context (same capture)

- runs: `5`
- stalemate runs: `1/5 (20.0%)`
- battle outcomes: `red=0`, `blue=1`, `draw=0`, `inconclusive=4`
- survival rate: `red=56.7%`, `blue=86.7%`

### Notes

- This snapshot is a headless simulation baseline (setup/sim/post/total runtime), not full on-screen rendering frame timing.
- HUD in-game rolling timings (sim/world/ui buckets) were added separately for interactive hotspot triage.

## Rendered runtime snapshot (30s auto-capture)

Added automated rendered capture runner:

- Command: `go run ./cmd/render-perf-capture -seconds 30`
- Just target: `just render-perf-capture SECONDS=30`
- Behaviour: launches full rendered client (fullscreen), captures runtime perf buckets for the first 30 seconds, then auto-exits and prints stats.

Latest captured output:

- duration_target_seconds: `30`
- duration_actual_seconds: `30.033`
- frame_count: `294`
- fps: `9.79`
- sim_tick_count: `598`
- avg_sim_ms_per_tick: `21.997`
- avg_world_ms_per_frame: `52.842`
- avg_ui_ms_per_frame: `1.122`
- avg_frame_cpu_ms_buckets: `53.963`

Notes:

- `avg_frame_cpu_ms_buckets` is `avg_world_ms_per_frame + avg_ui_ms_per_frame` from in-engine timing buckets.
- This capture is hardware/config dependent; use the same machine/settings for before/after comparisons.

### Rendered capture after static terrain layer caching

Change applied:

- Cached static terrain layer (`terrainBuf`) and switched `drawWorld` to blit cached terrain instead of re-rendering ground/grid/road-marking loops every frame.

Capture command:

`go run ./cmd/render-perf-capture -seconds 30`

Latest captured output (post-change):

- duration_target_seconds: `30`
- duration_actual_seconds: `30.057`
- frame_count: `465`
- fps: `15.47`
- sim_tick_count: `1078`
- avg_sim_ms_per_tick: `14.727`
- avg_world_ms_per_frame: `26.843`
- avg_ui_ms_per_frame: `1.204`
- avg_frame_cpu_ms_buckets: `28.047`

Before/after delta vs previous rendered 30s capture:

- fps: `9.79 -> 15.47` (`+58.0%`)
- avg_world_ms_per_frame: `52.842 -> 26.843` (`-49.2%`)
- avg_frame_cpu_ms_buckets: `53.963 -> 28.047` (`-48.0%`)
- avg_sim_ms_per_tick: `21.997 -> 14.727` (`-33.0%`)

### Rendered capture after vision-cone culling + adaptive cone detail

Changes applied:

- Added camera-bounds culling for vision cones (skip off-screen soldiers in `drawVisionConesBuffered`).
- Added adaptive cone tessellation step count based on zoom level (lower detail at wide zoom-out, higher detail when zoomed in).

Capture command:

`go run ./cmd/render-perf-capture -seconds 30`

Latest captured output (post-change):

- duration_target_seconds: `30`
- duration_actual_seconds: `30.016`
- frame_count: `524`
- fps: `17.46`
- sim_tick_count: `1190`
- avg_sim_ms_per_tick: `18.136`
- avg_world_ms_per_frame: `13.044`
- avg_ui_ms_per_frame: `1.164`
- avg_frame_cpu_ms_buckets: `14.209`

Before/after delta vs previous rendered capture (terrain cache only):

- fps: `15.47 -> 17.46` (`+12.9%`)
- avg_world_ms_per_frame: `26.843 -> 13.044` (`-51.4%`)
- avg_frame_cpu_ms_buckets: `28.047 -> 14.209` (`-49.3%`)

### Rendered capture after camera-aware world/overlay culling

Changes applied:

- Added camera-bounds culling in `drawWorld` for:
  - heat overlay tile loops,
  - building footprint floor/tint pass,
  - wall/window passes,
  - tile-map object pass,
  - cover-object pass,
  - soldier draw pass.
- Added camera-bounds culling in overlay draws for:
  - officer orders,
  - movement intent lines,
  - squad intent labels.

Capture command:

`go run ./cmd/render-perf-capture -seconds 30`

Latest captured output (post-change):

- duration_target_seconds: `30`
- duration_actual_seconds: `30.031`
- frame_count: `568`
- fps: `18.91`
- sim_tick_count: `1251`
- avg_sim_ms_per_tick: `17.916`
- avg_world_ms_per_frame: `10.972`
- avg_ui_ms_per_frame: `1.207`
- avg_frame_cpu_ms_buckets: `12.179`

Before/after delta vs previous rendered capture (vision-cone culling/adaptive detail):

- fps: `17.46 -> 18.91` (`+8.3%`)
- avg_world_ms_per_frame: `13.044 -> 10.972` (`-15.9%`)
- avg_frame_cpu_ms_buckets: `14.209 -> 12.179` (`-14.3%`)

### Rendered capture after effects culling + cached vignette layer

Changes applied:

- Added camera-bounds culling for:
  - spotted indicators,
  - radio visual effects,
  - gunfire lighting blooms.
- Added adaptive segment count for radio effect arcs based on zoom.
- Cached static vignette into `vignetteBuf` and blit per-frame instead of re-drawing layered rect bands each frame.

Capture command:

`go run ./cmd/render-perf-capture -seconds 30`

Latest captured output (post-change):

- duration_target_seconds: `30`
- duration_actual_seconds: `30.006`
- frame_count: `562`
- fps: `18.73`
- sim_tick_count: `1243`
- avg_sim_ms_per_tick: `18.732`
- avg_world_ms_per_frame: `9.289`
- avg_ui_ms_per_frame: `1.227`
- avg_frame_cpu_ms_buckets: `10.515`

Before/after delta vs previous rendered capture (camera-aware world/overlay culling):

- fps: `18.91 -> 18.73` (`-1.0%`)
- avg_world_ms_per_frame: `10.972 -> 9.289` (`-15.3%`)
- avg_frame_cpu_ms_buckets: `12.179 -> 10.515` (`-13.7%`)

### Rendered capture after low-zoom tactical overlay gating

Changes applied:

- Added low-zoom quality gates (`camZoom > 0.75`) to skip expensive tactical overlays at wide zoom:
  - movement intent lines,
  - officer orders,
  - squad intent labels,
  - spotted indicators.
- Added map-generation stability guard in exterior feature geometry placement (`safeIntn`) so performance capture runs no longer fail intermittently on invalid `Intn` bounds.

Capture command:

`go run ./cmd/render-perf-capture -seconds 30`

Latest captured output (post-change):

- duration_target_seconds: `30`
- duration_actual_seconds: `30.019`
- frame_count: `556`
- fps: `18.52`
- sim_tick_count: `1260`
- avg_sim_ms_per_tick: `14.022`
- avg_world_ms_per_frame: `19.189`
- avg_ui_ms_per_frame: `1.098`
- avg_frame_cpu_ms_buckets: `20.287`

Notes:

- Rendered capture remains highly map-seed dependent (building/exterior density has a large impact).
- For tighter before/after attribution, the next step should be adding an explicit map-seed flag to rendered perf capture and running fixed-seed comparisons.

### Rendered capture after Phase 2 LOS acceleration integration

Changes applied:

- Added shared LOS broad-phase index (`LOSIndex`) for buildings + LOS-blocking cover.
- Routed major LOS consumers to indexed queries (vision scan/peek, combat LOS and occlusion, squad ally visibility checks).
- Updated vision-cone ray clipping to use indexed building candidate queries.

Capture command:

`go run ./cmd/render-perf-capture -seconds 30`

Latest captured output (post-change):

- duration_target_seconds: `30`
- duration_actual_seconds: `30.009`
- frame_count: `417`
- fps: `13.90`
- sim_tick_count: `869`
- avg_sim_ms_per_tick: `23.890`
- avg_world_ms_per_frame: `18.581`
- avg_ui_ms_per_frame: `1.535`
- avg_frame_cpu_ms_buckets: `20.116`

Before/after delta vs previous rendered capture (low-zoom tactical overlay gating):

- fps: `18.52 -> 13.90` (`-24.9%`)
- avg_world_ms_per_frame: `19.189 -> 18.581` (`-3.2%`)
- avg_ui_ms_per_frame: `1.098 -> 1.535` (`+39.8%`)
- avg_frame_cpu_ms_buckets: `20.287 -> 20.116` (`-0.8%`)
- avg_sim_ms_per_tick: `14.022 -> 23.890` (`+70.4%`)

Notes:

- This run used map seed `1772662917844503700` and generated a different layout density profile, so these deltas are not clean attribution for the LOS change set.
- World-frame CPU bucket stayed roughly flat/slightly improved, but simulation tick cost was significantly higher in this seed.
- Next mandatory step for credible attribution: repeat A/B runs with identical seeds.

### Perf capture protocol update (fixed seed policy)

From this point onwards, all rendered performance captures should use an explicit map seed so before/after comparisons are attributable.

Standard command format:

`go run ./cmd/render-perf-capture -seconds 30 -map-seed 1772662917844503700`

Policy:

- Keep `-map-seed` constant while measuring a change set.
- If changing the seed for scenario coverage, log it explicitly and do not compare directly against other seeds.

### Seeded capture after adding explicit `-map-seed` support

Capture command:

`go run ./cmd/render-perf-capture -seconds 30 -map-seed 1772662917844503700`

Captured output:

- duration_target_seconds: `30`
- duration_actual_seconds: `30.040`
- frame_count: `344`
- fps: `11.45`
- sim_tick_count: `677`
- avg_sim_ms_per_tick: `28.783`
- avg_world_ms_per_frame: `25.878`
- avg_ui_ms_per_frame: `1.947`
- avg_frame_cpu_ms_buckets: `27.825`

Notes:

- This seeded run provided a deterministic baseline for further LOS-index tuning.

### Seeded capture after LOS index query-overhead reduction

Changes applied:

- Reworked LOS candidate de-duplication to use generation-stamped seen maps, avoiding per-query full map clears in index lookups.

Capture command:

`go run ./cmd/render-perf-capture -seconds 30 -map-seed 1772662917844503700`

Captured output:

- duration_target_seconds: `30`
- duration_actual_seconds: `30.025`
- frame_count: `459`
- fps: `15.29`
- sim_tick_count: `920`
- avg_sim_ms_per_tick: `25.047`
- avg_world_ms_per_frame: `11.732`
- avg_ui_ms_per_frame: `1.727`
- avg_frame_cpu_ms_buckets: `13.459`

Before/after delta vs previous seeded run (same seed):

- fps: `11.45 -> 15.29` (`+33.5%`)
- avg_world_ms_per_frame: `25.878 -> 11.732` (`-54.7%`)
- avg_ui_ms_per_frame: `1.947 -> 1.727` (`-11.3%`)
- avg_frame_cpu_ms_buckets: `27.825 -> 13.459` (`-51.6%`)
- avg_sim_ms_per_tick: `28.783 -> 25.047` (`-13.0%`)

### Static object/rubble flattening experiment (seeded)

Hypothesis:

- Flattening static tile objects + cover/rubble into a cached layer should reduce per-frame draw cost.

Implementation trial:

- Added a static object cache layer and blitted it in `drawWorld`.
- Left notes in code to support future invalidation for dynamic battlefields.

Seeded captures during trial (same seed):

`go run ./cmd/render-perf-capture -seconds 30 -map-seed 1772662917844503700`

Run A:

- fps: `13.72`
- avg_sim_ms_per_tick: `30.761`
- avg_world_ms_per_frame: `12.760`
- avg_frame_cpu_ms_buckets: `14.929`

Run B:

- fps: `12.12`
- avg_sim_ms_per_tick: `24.688`
- avg_world_ms_per_frame: `27.134`
- avg_frame_cpu_ms_buckets: `28.999`

Assessment:

- Results were unstable and generally worse than the prior best seeded run (`fps 15.29`, `avg_frame_cpu_ms_buckets 13.459`).
- Decision: revert full static object-layer flattening for now.

Forward path (keeps future dynamic battlefields open):

- Keep current direct rendering for tile objects/cover.
- If revisiting flattening later, prefer smaller cached sub-layers or tile sprites with explicit dirty-region invalidation hooks.

### Seeded capture after additional Phase 3 full-grid intel pass reduction

Changes applied:

- Reduced full-grid derived intel work during normal two-team simulation updates by staggering `IntelThreatDensity` accumulation to one team map per tick (aligned with staggered `IntelSafeTerritory` recompute).
- Preserved full derived-layer recompute when one side is absent to maintain expected behaviour in test/sandbox cases.

Capture command:

`go run ./cmd/render-perf-capture -seconds 30 -map-seed 1772662917844503700`

Captured output:

- duration_target_seconds: `30`
- duration_actual_seconds: `30.010`
- frame_count: `489`
- fps: `16.29`
- sim_tick_count: `1065`
- avg_sim_ms_per_tick: `18.015`
- avg_world_ms_per_frame: `18.920`
- avg_ui_ms_per_frame: `1.251`
- avg_frame_cpu_ms_buckets: `20.171`

Before/after delta vs previous seeded run (post `SumInRadius`/`MaxInRadius` cell-space optimisation):

- fps: `16.21 -> 16.29` (`+0.5%`)
- avg_sim_ms_per_tick: `18.508 -> 18.015` (`-2.7%`)
- avg_world_ms_per_frame: `18.273 -> 18.920` (`+3.5%`)
- avg_ui_ms_per_frame: `1.100 -> 1.251` (`+13.7%`)
- avg_frame_cpu_ms_buckets: `19.373 -> 20.171` (`+4.1%`)

### Headless capture after Phase 4 repath gating pass

Changes applied:

- Added pathfinding counters in `NavGrid` and surfaced per-run pathfinding summary in `cmd/headless-report`.
- Added cooldown-gated repath policy for moderate drift in both contact movement and squad formation updates.

Capture command:

`go run ./cmd/headless-report -runs 5 -ticks 3600 -seed-base 42 -seed-step 1`

Captured output (perf summary):

- setup: `28.08184ms / 19.5357ms / 38.9985ms`
- sim: `2.8775942s / 2.0332964s / 3.7292921s`
- post: `1.33242ms / 1.0174ms / 2.0725ms`
- total: `2.90700846s / 2.0706525s / 3.7562368s`

Derived from the same report output:

- sim throughput (aggregate avg): ~`1251 ticks/sec` (`3600 / 2.8775942`)
- sim cost per tick (aggregate avg): ~`799.3 µs/tick`

Comparison notes:

- Versus baseline headless snapshot (`sim 3.08835624s`, `857.9 µs/tick`): improved by ~`6.8%` in sim duration and ~`6.8%` in per-tick cost.
- Versus previous recent Phase 4 trial (`sim 3.38636304s`): improved by ~`15.0%` in sim duration.

### Seeded rendered capture after Phase 4 repath gating changes

Changes applied since prior seeded rendered capture:

- Added cooldown-gated moderate-drift repath logic in `moveToContact`.
- Added cooldown-gated moderate-drift repath logic in squad `UpdateFormation`.

Capture command:

`go run ./cmd/render-perf-capture -seconds 30 -map-seed 1772662917844503700`

Captured output:

- duration_target_seconds: `30`
- duration_actual_seconds: `30.049`
- frame_count: `489`
- fps: `16.27`
- sim_tick_count: `1128`
- avg_sim_ms_per_tick: `16.220`
- avg_world_ms_per_frame: `20.956`
- avg_ui_ms_per_frame: `1.051`
- avg_frame_cpu_ms_buckets: `22.007`

Before/after delta vs previous seeded rendered capture (`fps 16.29`, `avg_frame_cpu_ms_buckets 20.171`):

- fps: `16.29 -> 16.27` (`-0.1%`)
- avg_sim_ms_per_tick: `18.015 -> 16.220` (`-10.0%`)
- avg_world_ms_per_frame: `18.920 -> 20.956` (`+10.8%`)
- avg_ui_ms_per_frame: `1.251 -> 1.051` (`-16.0%`)
- avg_frame_cpu_ms_buckets: `20.171 -> 22.007` (`+9.1%`)

Assessment:

- FPS is effectively flat on this seeded rendered scenario.
- Simulation tick cost improved, but world-frame draw bucket increased enough that total frame CPU bucket rose.
- Net rendered gain is not established yet for this change set.

### Headless capture after Phase 4 path-smoothing LOS optimisation

Changes applied:

- `moveAlongPath` now uses indexed LOS (`HasLineOfSightIndexed`) for waypoint look-ahead smoothing checks.
- Added short tick-throttled smoothing recalc cadence (`pathLookaheadRecalcTicks`) and reuse of the last smoothing target index.

Capture command:

`go run ./cmd/headless-report -runs 5 -ticks 3600 -seed-base 42 -seed-step 1`

Captured output (perf summary):

- setup: `26.30058ms / 19.4151ms / 33.9919ms`
- sim: `2.71086814s / 1.8438446s / 3.5809813s`
- post: `1.70174ms / 510µs / 2.7816ms`
- total: `2.73887046s / 1.8785806s / 3.6062831s`

Derived from the same report output:

- sim throughput (aggregate avg): ~`1328 ticks/sec` (`3600 / 2.71086814`)
- sim cost per tick (aggregate avg): ~`753.0 µs/tick`

Comparison notes:

- Versus previous Phase 4 repath-gating headless capture (`sim 2.8775942s`, `799.3 µs/tick`): improved by ~`5.8%` in sim duration and per-tick cost.
- Versus baseline headless snapshot (`sim 3.08835624s`, `857.9 µs/tick`): improved by ~`12.2%` in sim duration and per-tick cost.

### Seeded rendered capture after Phase 5 speech-bubble culling

Changes applied:

- Added camera-aware culling in `drawSpeechBubbles` to skip off-screen bubble rendering and off-screen overlap checks.

Capture command:

`go run ./cmd/render-perf-capture -seconds 30 -map-seed 1772662917844503700`

Captured output:

- duration_target_seconds: `30`
- duration_actual_seconds: `30.021`
- frame_count: `518`
- fps: `17.25`
- sim_tick_count: `1203`
- avg_sim_ms_per_tick: `16.084`
- avg_world_ms_per_frame: `17.733`
- avg_ui_ms_per_frame: `1.057`
- avg_frame_cpu_ms_buckets: `18.790`

Before/after delta vs previous seeded rendered capture (`fps 16.27`, `avg_frame_cpu_ms_buckets 22.007`):

- fps: `16.27 -> 17.25` (`+6.0%`)
- avg_sim_ms_per_tick: `16.220 -> 16.084` (`-0.8%`)
- avg_world_ms_per_frame: `20.956 -> 17.733` (`-15.4%`)
- avg_ui_ms_per_frame: `1.051 -> 1.057` (`+0.6%`)
- avg_frame_cpu_ms_buckets: `22.007 -> 18.790` (`-14.6%`)

### Seeded rendered capture after Phase 5 cover-neighbour cache reuse

Changes applied:

- `drawCoverObjects` no longer rebuilds chest-wall neighbor lookup each frame.
- Chest-wall set is now lazily rebuilt once and reused until cover layout changes.

Capture command:

`go run ./cmd/render-perf-capture -seconds 30 -map-seed 1772662917844503700`

Captured output:

- duration_target_seconds: `30`
- duration_actual_seconds: `30.008`
- frame_count: `455`
- fps: `15.16`
- sim_tick_count: `1017`
- avg_sim_ms_per_tick: `16.874`
- avg_world_ms_per_frame: `24.792`
- avg_ui_ms_per_frame: `1.182`
- avg_frame_cpu_ms_buckets: `25.975`

Before/after delta vs previous seeded rendered capture (`fps 17.25`, `avg_frame_cpu_ms_buckets 18.790`):

- fps: `17.25 -> 15.16` (`-12.1%`)
- avg_sim_ms_per_tick: `16.084 -> 16.874` (`+4.9%`)
- avg_world_ms_per_frame: `17.733 -> 24.792` (`+39.8%`)
- avg_ui_ms_per_frame: `1.057 -> 1.182` (`+11.8%`)
- avg_frame_cpu_ms_buckets: `18.790 -> 25.975` (`+38.2%`)

Notes:

- This regression is unlikely to be caused solely by the chest-wall cache change, given the scope and expected cost of that path.
- Camera behavior changed to auto-frame-follow in the same development window, which may materially alter rendered workload per frame versus previous captures.
- For cleaner attribution of future rendering changes, keep map seed fixed and camera mode fixed during A/B runs.

### Seeded rendered capture after terrain visible-blit + low-zoom heat overlay gating

Changes applied:

- Terrain layer now draws only the camera-visible sub-rectangle from `terrainBuf` (`drawTerrainLayerVisible`).
- Heat overlays now skip drawing when zoomed out below `0.35` where detail is not readable.

Capture command:

`go run ./cmd/render-perf-capture -seconds 30 -map-seed 1772662917844503700`

Captured output:

- duration_target_seconds: `30`
- duration_actual_seconds: `30.017`
- frame_count: `477`
- fps: `15.89`
- sim_tick_count: `1119`
- avg_sim_ms_per_tick: `15.563`
- avg_world_ms_per_frame: `23.160`
- avg_ui_ms_per_frame: `1.130`
- avg_frame_cpu_ms_buckets: `24.290`

Before/after delta vs previous seeded rendered capture (`fps 15.16`, `avg_frame_cpu_ms_buckets 25.975`):

- fps: `15.16 -> 15.89` (`+4.8%`)
- avg_sim_ms_per_tick: `16.874 -> 15.563` (`-7.8%`)
- avg_world_ms_per_frame: `24.792 -> 23.160` (`-6.6%`)
- avg_ui_ms_per_frame: `1.182 -> 1.130` (`-4.4%`)
- avg_frame_cpu_ms_buckets: `25.975 -> 24.290` (`-6.5%`)
