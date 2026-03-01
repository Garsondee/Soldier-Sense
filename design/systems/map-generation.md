# Map Generation — Per-Tile Terrain System

**Status:** Planning Draft
**Priority:** High — foundational change that touches rendering, nav, LOS, combat, and AI
**Scope:** Replace all non-grid-aligned terrain (roads, grass patches, ground texture) with a strict per-cell tile map. Expand terrain vocabulary. Enrich building interiors. Add vegetation, fortifications, and biome-driven placement.

---

## 1. Design Intent

Every cell on the battlefield is a **1×1 metre tile** (currently `cellSize = 16` px). Each tile carries its own ground type, surface properties, and optional furniture/obstacle. The map should read like a Dwarf-Fortress-style overhead view where zooming in reveals rich per-tile detail.

### Goals

- **Grid purity.** Nothing exists between grid lines. Roads, pavements, grass, gravel — all placed tile-by-tile.
- **Rich terrain vocabulary.** Dozens of ground and object types, each with distinct movement, cover, LOS, and visual properties.
- **Biome-driven placement.** Layered noise decides urban density, vegetation lushness, and terrain roughness — no hand-authored zones.
- **Interesting interiors.** Furniture, pillars, doors (openable, breakable), windows that block movement until smashed.
- **Field fortifications.** Slit trenches, anti-tank barriers, sandbag lines, wire — placed contextually.

### What Changes

| Area | Before | After |
|------|--------|-------|
| Ground | Flat green fill + random rect patches | Per-tile `GroundType` enum rendered individually |
| Roads | Spline polylines, non-grid collision AABBs | Grid-stamped road tiles (tarmac + pavement + kerb) |
| Vegetation | Procedural grass tufts (non-grid) | Per-tile grass/scrub/bush/tree with properties |
| Buildings | Grid walls + BSP rooms, empty floors | Grid walls + BSP rooms + furniture + pillars + doors |
| Windows | Block movement permanently | Block movement until broken; breakable via gunfire or explosions |
| Cover | 3 kinds (tall wall, chest wall, rubble) | Many more: sandbags, crates, vehicles, rubble grades, hedgerows |
| Fortifications | None | Slit trenches, AT barriers, wire entanglements |

---

## 2. Tile Data Model

### 2.1 The Tile Struct

Every cell `(col, row)` in the map has a `Tile`:

```go
type Tile struct {
    Ground      GroundType   // base surface
    Object      ObjectType   // furniture / obstacle / fortification on this cell (ObjectNone if empty)
    Flags       TileFlags    // bitfield: indoor, road-edge, damaged, etc.
    Elevation   int8         // relative height (0 = ground level, -1 = trench, +1 = raised)
    Durability  int16        // hit points for breakable objects (windows, doors, furniture)
}
```

### 2.2 Ground Types

Each ground type defines movement cost, visual colour/pattern, sound profile (future), and cover value.

| ID | Name | Movement Mul | Cover | Notes |
|----|------|-------------|-------|-------|
| 0 | `GroundGrass` | 1.0 | 0.0 | Default open ground |
| 1 | `GroundGrassLong` | 0.9 | 0.05 | Tall grass, minor concealment |
| 2 | `GroundScrub` | 0.8 | 0.08 | Low bushes / bramble |
| 3 | `GroundMud` | 0.6 | 0.0 | Wet / churned ground |
| 4 | `GroundSand` | 0.75 | 0.0 | Sandy / arid patches |
| 5 | `GroundGravel` | 0.85 | 0.0 | Loose stone, noisy |
| 6 | `GroundDirt` | 0.95 | 0.0 | Packed earth path |
| 7 | `GroundTarmac` | 1.0 | 0.0 | Road surface |
| 8 | `GroundPavement` | 1.0 | 0.0 | Sidewalk / paved area |
| 9 | `GroundConcrete` | 1.0 | 0.0 | Building interior floor |
| 10 | `GroundTile` | 1.0 | 0.0 | Interior tile floor (kitchen, bathroom) |
| 11 | `GroundWood` | 1.0 | 0.0 | Interior wood floor |
| 12 | `GroundWater` | 0.3 | 0.0 | Shallow puddle/stream (passable but very slow) |
| 13 | `GroundRubbleLight` | 0.7 | 0.10 | Scattered small debris |
| 14 | `GroundRubbleHeavy` | 0.4 | 0.25 | Dense rubble field |
| 15 | `GroundCrater` | 0.5 | 0.15 | Shell crater (slight depression) |

### 2.3 Object Types

Objects sit on top of the ground. They affect movement, LOS, cover, and can be destroyed.

| ID | Name | Blocks Mvmt | Blocks LOS | Cover | Breakable | Notes |
|----|------|-------------|-----------|-------|-----------|-------|
| 0 | `ObjectNone` | — | — | — | — | Empty cell |
| 1 | `ObjectWall` | yes | yes | 0.90 | no | Structural wall |
| 2 | `ObjectWallDamaged` | yes | partial | 0.70 | no | Pre-damaged wall (holes) |
| 3 | `ObjectWindow` | yes | no | 0.20 | yes (HP 30) | Intact window — see through, can't pass |
| 4 | `ObjectWindowBroken` | no | no | 0.05 | no | Broken window — passable, minor snag |
| 5 | `ObjectDoor` | yes | yes | 0.60 | yes (HP 40) | Closed door — blocks move + LOS |
| 6 | `ObjectDoorOpen` | no | no | 0.0 | no | Open door — passable |
| 7 | `ObjectDoorBroken` | no | no | 0.0 | no | Destroyed door frame |
| 8 | `ObjectPillar` | yes | yes | 0.85 | no | Structural column |
| 9 | `ObjectTable` | slows (0.5) | no | 0.30 | yes (HP 20) | Furniture — can be shot through |
| 10 | `ObjectChair` | slows (0.7) | no | 0.15 | yes (HP 10) | Light furniture |
| 11 | `ObjectCrate` | yes | yes | 0.65 | yes (HP 50) | Wooden crate — good cover |
| 12 | `ObjectSandbag` | slows (0.6) | no | 0.70 | yes (HP 80) | Chest-high sandbag wall |
| 13 | `ObjectChestWall` | slows (0.6) | no | 0.70 | no | Low masonry wall |
| 14 | `ObjectTallWall` | yes | yes | 0.85 | no | Freestanding tall wall |
| 15 | `ObjectHedgerow` | slows (0.4) | partial | 0.40 | yes (HP 60) | Thick hedge — slow + partial concealment |
| 16 | `ObjectBush` | slows (0.7) | partial | 0.20 | yes (HP 15) | Decorative bush |
| 17 | `ObjectTreeTrunk` | yes | yes | 0.80 | no | Tree base (1×1 cell, canopy cosmetic) |
| 18 | `ObjectTreeCanopy` | no | partial | 0.10 | no | Overhead foliage (no move block, dappled LOS) |
| 19 | `ObjectRubblePile` | slows (0.5) | partial | 0.55 | no | Heaped debris |
| 20 | `ObjectWire` | slows (0.2) | no | 0.0 | yes (HP 25) | Barbed wire entanglement |
| 21 | `ObjectATBarrier` | yes | no | 0.50 | no | Concrete anti-tank block ("dragon's teeth") |
| 22 | `ObjectSlitTrench` | no | no | 0.75 | no | Dug-in position (uses Elevation -1) |
| 23 | `ObjectVehicleWreck` | yes | yes | 0.80 | no | Burnt-out vehicle hull |
| 24 | `ObjectFence` | slows (0.5) | no | 0.05 | yes (HP 15) | Chain-link or wooden fence |

### 2.4 Tile Flags

```go
type TileFlags uint8

const (
    TileFlagIndoor   TileFlags = 1 << iota // inside a building footprint
    TileFlagRoadEdge                        // pavement / kerb bordering a road
    TileFlagDamaged                         // ground damaged by explosion
    TileFlagTrench                          // part of a trench system
    TileFlagRoof                            // has overhead cover (future: rain, air observation)
)
```

### 2.5 TileMap

```go
type TileMap struct {
    Cols, Rows int
    Tiles      []Tile // row-major: index = row*Cols + col
}

func (tm *TileMap) At(col, row int) *Tile
func (tm *TileMap) Ground(col, row int) GroundType
func (tm *TileMap) IsPassable(col, row int) bool
func (tm *TileMap) MovementCost(col, row int) float64
func (tm *TileMap) LOSOpacity(col, row int) float64   // 0 = transparent, 1 = opaque
func (tm *TileMap) CoverValue(col, row int) float64
func (tm *TileMap) IsIndoor(col, row int) bool
func (tm *TileMap) DamageTile(col, row int, dmg int)   // reduce Durability, transition on break
```

---

## 3. Map Generation Pipeline

Generation happens in deterministic phases, each fed by the master `mapSeed`.

```
Phase 1: Biome Noise        → urban density, vegetation lushness, terrain roughness
Phase 2: Road Network        → grid-aligned roads with pavements
Phase 3: Building Placement  → footprints along roads, interior subdivision
Phase 4: Building Interiors  → furniture, pillars, doors, windows
Phase 5: Vegetation          → trees, bushes, hedgerows, grass types
Phase 6: Fortifications      → trenches, wire, barriers, sandbags
Phase 7: Battle Damage       → explosions, rubble, craters
Phase 8: Derived Data        → NavGrid, TacticalMap, LOS cache rebuilt from TileMap
```

### 3.1 Phase 1 — Biome Noise

Generate three independent 2D Perlin/Simplex noise fields at the tile-map scale:

| Noise Layer | Frequency | Range | Controls |
|------------|-----------|-------|----------|
| **Urban density** | Low (λ ≈ 400 tiles) | 0.0–1.0 | > 0.55 = urban (buildings, roads, pavement). < 0.3 = rural (open fields). Between = suburban. |
| **Vegetation lushness** | Medium (λ ≈ 200 tiles) | 0.0–1.0 | High = long grass, bushes, trees. Low = bare dirt, sand, gravel. |
| **Terrain roughness** | High (λ ≈ 80 tiles) | 0.0–1.0 | High = uneven ground, mud patches, craters. Low = smooth, flat. |

Optional fourth layer for **moisture** (controls mud vs sand, puddles vs dry ground) — same frequency as vegetation.

Each tile samples these noise fields to determine its character. The noise layers are sampled once and cached as float arrays for the later phases to query.

### 3.2 Phase 2 — Road Network (Grid-Aligned)

Roads are no longer splines. They are **grid-stamped paths** that travel in axis-aligned segments with 90° turns.

#### Algorithm

1. **Seed road endpoints** on opposite map edges (2–3 horizontal routes, 1–2 vertical).
2. **Pathfind** each route through the tile grid using a weighted A* that:
   - Prefers straight runs (penalise turns).
   - Follows the urban-density gradient (roads gravitate toward higher density).
   - Avoids sharp zigzags (minimum straight-run length before a turn: 8–12 tiles).
   - Slight random jitter to avoid perfectly regular layouts.
3. **Stamp tiles** along the path:
   - Centre tiles → `GroundTarmac`
   - Road width: 3–5 tiles (main roads wider, side streets narrower).
   - Outermost road tile on each side → `GroundPavement` with `TileFlagRoadEdge` (X% of the time, controlled by urban density; rural roads skip pavement).
   - Kerb markers baked into rendering by the `RoadEdge` flag.
4. **Intersections** where two roads cross are detected automatically (tiles that get stamped twice). Intersection tiles get a slightly different tint.
5. **Side streets** (short stubs, 10–20 tiles long) branch off main roads in urban areas with probability proportional to urban density.

#### Road Properties

| Width | Name | Pavement | Frequency |
|-------|------|----------|-----------|
| 5 tiles | Main road | 1 tile each side, 80% | 2–3 per map |
| 3 tiles | Side street | 1 tile each side, 40% | 0–4 per map (urban only) |

### 3.3 Phase 3 — Building Placement

Unchanged strategy (place along roads, BSP candidates, overlap/road rejection), but now:

- Buildings stamp `TileFlagIndoor` on every tile within their footprint.
- Floor ground type chosen per-building from `{GroundConcrete, GroundTile, GroundWood}` with weighted random.
- Perimeter walls, windows, and doorways are written as `ObjectType` into the tile map instead of separate `[]rect` slices.
- BSP room subdivision now writes internal partition walls into the tile map.
- Building count and size distribution scale with local urban-density noise.

### 3.4 Phase 4 — Building Interiors

After walls and rooms are placed, furnish each room:

| Element | Placement Rule | Object Type |
|---------|---------------|-------------|
| **Pillars** | Rooms ≥ 5×5: one pillar near centre or at quarter-points. | `ObjectPillar` |
| **Tables** | 30% of rooms: 1–2 table tiles, adjacent chairs. | `ObjectTable`, `ObjectChair` |
| **Doors** | Every internal doorway gets a door (80% closed, 20% open). | `ObjectDoor` / `ObjectDoorOpen` |
| **Crates** | 15% of rooms: 1–3 crate tiles along a wall. | `ObjectCrate` |
| **Exterior doors** | All exterior doorways get a door (70% closed). | `ObjectDoor` / `ObjectDoorOpen` |

Doors and windows have `Durability`. When reduced to 0 by gunfire or explosions they transition:
- `ObjectWindow` → `ObjectWindowBroken` (now passable, low cover)
- `ObjectDoor` → `ObjectDoorBroken` (now passable, no cover)

### 3.5 Phase 5 — Vegetation

For every outdoor tile not already road/building/pavement:

1. Sample vegetation lushness `V` and terrain roughness `R` at this cell.
2. Assign ground type:
   - `V > 0.7` → `GroundGrassLong` (60%) or `GroundScrub` (40%)
   - `V 0.4–0.7` → `GroundGrass`
   - `V 0.2–0.4` → `GroundDirt` (50%) or `GroundGravel` (50%)
   - `V < 0.2` → `GroundSand` (60%) or `GroundGravel` (40%)
   - `R > 0.7` overlay: `GroundMud` (40%), `GroundCrater` (10%)
3. Place vegetation objects (probability scaled by `V`):
   - `V > 0.8` and roll < 0.04 → `ObjectTreeTrunk` (with `ObjectTreeCanopy` on 4-connected neighbours above threshold)
   - `V > 0.6` and roll < 0.06 → `ObjectBush`
   - `V > 0.5` and roll < 0.03 → `ObjectHedgerow` (placed in runs of 3–8 tiles, often along road edges or field boundaries)
4. **Hedgerow runs** are generated as connected horizontal or vertical lines — not isolated tiles. They act as field boundaries in rural areas. Placement probability increases near road edges.

### 3.6 Phase 6 — Fortifications

Placed in zones with medium urban density (suburban / edge of town) and biased toward the map centre (the contested zone).

| Feature | Tiles | Placement |
|---------|-------|-----------|
| **Slit trench** | 4–10 tile run, `ObjectSlitTrench`, `Elevation -1` | Oriented perpendicular to likely axis of advance (horizontal for N/S maps). 2–5 per map. |
| **Sandbag line** | 3–8 tile run, `ObjectSandbag` | Adjacent to trenches or at building corners. |
| **AT barrier row** | 3–6 tiles, `ObjectATBarrier` | Across roads or open lanes. 1–3 per map. |
| **Wire entanglement** | 5–15 tile patch, `ObjectWire` | In front of defensive positions. Slows infantry massively. |
| **Fences** | 8–20 tile run, `ObjectFence` | Along property boundaries in suburban areas. |

### 3.7 Phase 7 — Battle Damage

Same concept as current `generateExplosionRubble`, but now operates on the tile map:

1. Fire `numExplosions` into the map (biased toward buildings).
2. For each explosion centre, iterate tiles within blast radius:
   - Breakable objects → destroy (apply damage exceeding durability).
   - Walls → 30% chance per tile to convert to `ObjectWallDamaged` or delete entirely and scatter `ObjectRubblePile`.
   - Ground → convert to `GroundRubbleLight` or `GroundCrater`.
   - Set `TileFlagDamaged`.
3. Rubble scatters outward: tiles adjacent to destroyed walls get `GroundRubbleLight`.

### 3.8 Phase 8 — Derived Data

Rebuild all downstream systems from the authoritative `TileMap`:

- **NavGrid** — `IsPassable` and movement cost read directly from `TileMap`.
- **TacticalMap** — cell traits derived from object types and flags instead of separate wall/window/footprint slices.
- **LOS** — opacity per tile from `TileMap.LOSOpacity()`.
- **Cover queries** — cover value per tile from `TileMap.CoverValue()`.

The old `[]rect` slices (`buildings`, `windows`, `buildingFootprints`, `roads`, `roadPolylines`, `covers`, `terrainPatches`) are **removed** from `Game`. Everything is read from `TileMap`.

---

## 4. Rendering

### 4.1 Ground Layer

Each tile is rendered as a filled `cellSize × cellSize` rectangle. Colour is determined by `GroundType` with per-tile hash-based variation (±small RGB jitter seeded from `col ^ row ^ mapSeed`) to avoid a flat grid look.

| Ground Type | Base Colour (R,G,B) | Variation |
|------------|-------------------|-----------|
| Grass | (30, 48, 30) | ±6 on G |
| GrassLong | (34, 58, 28) | ±8 on G, slight yellow tint |
| Scrub | (40, 50, 30) | ±6 |
| Mud | (50, 40, 28) | ±4 |
| Sand | (70, 65, 48) | ±5 |
| Gravel | (55, 52, 48) | ±4 |
| Dirt | (48, 42, 34) | ±4 |
| Tarmac | (48, 46, 42) | ±2 (very uniform) |
| Pavement | (62, 60, 56) | ±3 |
| Concrete | (38, 36, 32) | ±2 |
| Tile | (44, 40, 36) | chequerboard pattern via `(col+row)%2` |
| Wood | (52, 40, 28) | alternating grain via `col%3` |
| Water | (28, 38, 55) | animated shimmer (future) |
| RubbleLight | (52, 48, 40) | ±6 |
| RubbleHeavy | (44, 40, 34) | ±8, darker |
| Crater | (36, 32, 28) | darker centre |

### 4.2 Object Layer

Objects are drawn on top of ground. Walls, pillars use the existing neighbour-aware edge-lighting system. New objects:

- **Doors** — rendered as a narrow rect across the doorway, brown/wood colour. Open doors swing 90° (thin line along wall). Broken doors are absent (just frame marks).
- **Furniture** — simple filled rects with edge highlights. Tables are brown, chairs lighter.
- **Trees** — trunk is a dark brown/black filled cell. Canopy cells are semi-transparent green circles (drawn in a separate canopy pass above soldiers for depth).
- **Sandbags** — tan/brown rect, slightly smaller than cell, with darker edge lines.
- **Wire** — thin cross-hatched lines within the cell.
- **AT barriers** — grey concrete block, triangular hint.
- **Trenches** — darker ground, inset border to suggest depth.

### 4.3 Grid Overlay

Keep the existing three-level grid (fine 16px / mid 64px / coarse 256px) drawn over the ground layer, under objects and soldiers.

### 4.4 Draw Order

```
1. Ground tiles (all cells)
2. Grid overlay
3. Road markings (centre-line dashes — now grid-aligned, drawn on tarmac tiles)
4. Trench depth shading
5. Building floor tints (team claim overlay)
6. Objects (walls, furniture, cover, vegetation, fortifications)
7. Intel heatmap overlays
8. Soldiers + tracers + radio arcs
9. Tree canopy layer (semi-transparent, above soldiers)
10. UI / HUD
```

---

## 5. Breakable Objects

### 5.1 Damage Model

```go
func (tm *TileMap) DamageTile(col, row int, dmg int) {
    t := tm.At(col, row)
    if t.Durability <= 0 { return } // already broken or unbreakable
    t.Durability -= int16(dmg)
    if t.Durability <= 0 {
        // Transition to broken state
        switch t.Object {
        case ObjectWindow:      t.Object = ObjectWindowBroken
        case ObjectDoor:        t.Object = ObjectDoorBroken
        case ObjectTable:       t.Object = ObjectNone; t.Ground = GroundRubbleLight
        case ObjectChair:       t.Object = ObjectNone
        case ObjectCrate:       t.Object = ObjectNone; t.Ground = GroundRubbleLight
        case ObjectSandbag:     t.Object = ObjectRubblePile
        case ObjectBush:        t.Object = ObjectNone
        case ObjectHedgerow:    t.Object = ObjectNone; t.Ground = GroundScrub
        case ObjectWire:        t.Object = ObjectNone
        case ObjectFence:       t.Object = ObjectNone; t.Ground = GroundRubbleLight
        }
        tm.rebuildLocalNav(col, row) // update nav/LOS caches in neighbourhood
    }
}
```

### 5.2 Damage Sources

- **Gunfire** — each bullet that strikes a breakable tile applies `baseDamage` (25) to its durability.
- **Explosions** — blast damage applied to all tiles in radius, scaling with distance.
- **Deliberate breach** — soldiers adjacent to a window/door can spend ticks to break it (future mechanic).

### 5.3 Tactical Implications

- Soldiers can shoot out windows to create entry points.
- Explosions open buildings up, reducing defender advantage.
- Furniture provides temporary cover that degrades under fire.
- Wire entanglements can be cleared by concentrated fire.

---

## 6. Noise Implementation

### 6.1 Simplex Noise

Use a simple 2D gradient noise implementation (no external dependency). A minimal Go port of the classic Simplex algorithm seeded from `mapSeed` is sufficient — ~100 lines.

### 6.2 Sampling

```go
func biomeAt(col, row int, noiseFields *NoiseFields) BiomeSample {
    x, y := float64(col), float64(row)
    return BiomeSample{
        UrbanDensity: noiseFields.Urban.At(x/400, y/400)*0.5 + 0.5,
        Vegetation:   noiseFields.Veg.At(x/200, y/200)*0.5 + 0.5,
        Roughness:    noiseFields.Rough.At(x/80, y/80)*0.5 + 0.5,
        Moisture:     noiseFields.Moisture.At(x/200, y/200)*0.5 + 0.5,
    }
}
```

### 6.3 Thresholds

All threshold values (e.g. "urban > 0.55") are constants in a config struct so they can be tuned without code changes.

---

## 7. Migration Strategy

The refactor is large. Break it into phases that each leave the game fully playable.

### Phase A — TileMap Foundation + Ground Types

1. Add `TileMap` struct and `GroundType` enum.
2. After existing generation, **write** current state into a `TileMap` (ground = grass everywhere, mark indoor, stamp roads as tarmac).
3. Render ground from `TileMap` instead of terrain patches. Remove `terrainPatches`.
4. NavGrid and TacticalMap still built from old slices — no behaviour change.
5. **Verify:** game looks and plays identically (roads still old-style polylines for now, ground just flatter).

### Phase B — Grid-Aligned Roads

1. Replace `initRoads` with grid-stamped road generation.
2. Remove `roadPolylines`, `roadSegment`, spline code.
3. Road tiles written directly into `TileMap`.
4. Pavement tiles along road edges.
5. Road rendering reads from tile map.
6. **Verify:** roads are grid-aligned, buildings still cluster near them, nav works.

### Phase C — Buildings Write to TileMap

1. `initBuildings` / `addBuildingWalls` write `ObjectWall`, `ObjectWindow`, `ObjectDoor` into `TileMap` instead of appending to `[]rect` slices.
2. `buildingFootprints` → tiles flagged `TileFlagIndoor`.
3. NavGrid and TacticalMap rebuilt from `TileMap` in Phase 8.
4. Remove `g.buildings`, `g.windows`, `g.buildingFootprints` fields.
5. **Verify:** buildings render correctly, LOS/nav/cover unchanged.

### Phase D — Building Interiors

1. Add pillar, furniture, door placement after room subdivision.
2. Doors are breakable objects with durability.
3. Windows become breakable.
4. Add `DamageTile` integration into combat (bullets, explosions).
5. **Verify:** interiors have furniture, doors/windows break under fire.

### Phase E — Biome Noise + Vegetation

1. Add simplex noise generator.
2. Generate biome noise fields from `mapSeed`.
3. Ground types assigned per-tile based on biome samples.
4. Vegetation objects (trees, bushes, hedgerows) placed.
5. Replace old `initCover` wall/corridor generation with noise-driven placement.
6. **Verify:** map has varied terrain, trees provide cover/LOS effects.

### Phase F — Fortifications

1. Add slit trench, AT barrier, wire, sandbag, fence placement.
2. Trench elevation affects cover calculations.
3. **Verify:** fortifications appear, affect gameplay.

### Phase G — Cleanup + Polish

1. Remove all legacy generation code (`terrainPatches`, `roadPolylines`, old `GenerateCover`).
2. Rendering polish: per-tile colour jitter, edge effects, canopy layer.
3. Tune biome thresholds and object densities via playtesting.
4. Update headless-report and tests.

---

## 8. Impact on Existing Systems

### 8.1 NavGrid

Rebuilt from `TileMap.IsPassable()`. No longer needs `buildings []rect`, `windows []rect`, or `covers []*CoverObject` as inputs. Movement cost per cell is `TileMap.MovementCost()`.

### 8.2 TacticalMap

Cell traits derived from tile data:
- `CellTraitWallAdj` → neighbour has `ObjectWall`/`ObjectPillar`
- `CellTraitCorner` → two perpendicular wall neighbours
- `CellTraitDoorway` → tile has `ObjectDoor`/`ObjectDoorOpen`/`ObjectDoorBroken`
- `CellTraitWindowAdj` → neighbour has `ObjectWindow`/`ObjectWindowBroken`
- `CellTraitInterior` → `TileFlagIndoor`
- New: `CellTraitTrench` — tile has `ObjectSlitTrench`

Desirability scores updated to account for furniture cover, trench bonus, tree cover.

### 8.3 LOS / Vision

`LOSOpacity` per tile replaces the current wall/window ray-cast lists. The ray-march accumulates opacity; if cumulative > threshold, LOS is blocked. Partial-LOS objects (hedgerows, tree canopy, bush) add fractional opacity.

### 8.4 Combat / Cover

`IsBehindCover` queries `TileMap.CoverValue()` for tiles between shooter and target along the fire ray. Multiple partial-cover tiles can stack (capped at 0.90).

### 8.5 CoverObject

The standalone `CoverObject` struct is retired. Cover properties are embedded in `ObjectType` lookup tables. `FindCoverForThreat` scans nearby tiles instead of a separate slice.

### 8.6 Soldier Movement

`MovementCost()` replaces the binary blocked/free model. Soldiers move faster on roads, slower through scrub/mud/rubble. The pathfinder (A*) uses tile movement cost as edge weight.

---

## 9. Testing Strategy

### Unit Tests

- `TestTileMapGroundAssignment` — verify biome noise → ground type mapping for known seeds.
- `TestGridRoadGeneration` — roads are fully grid-aligned, correct width, pavement presence.
- `TestBuildingWritesToTileMap` — walls, windows, doors all present as ObjectTypes.
- `TestBreakableWindow` — DamageTile transitions window → broken, passable after.
- `TestBreakableDoor` — same for doors.
- `TestNavGridFromTileMap` — blocked cells match impassable tiles.
- `TestTacticalMapFromTileMap` — traits match expected for a known layout.
- `TestLOSPartialCover` — hedge/bush correctly reduces but doesn't fully block LOS.
- `TestMovementCostVariation` — tarmac faster than mud faster than rubble.

### Integration Tests

- Headless simulation runs produce valid results with new map gen.
- No panics or OOB on 100 random seeds.
- Per-tile ground type distribution is reasonable (not 100% one type).

### Visual Verification

- Side-by-side screenshots before/after each migration phase.
- Zoom-in checks that tiles are crisply aligned to the 16px grid.

---

## 10. Constants & Tuning

All generation parameters should be grouped in a `MapGenConfig` struct with sensible defaults. This allows quick iteration without hunting through code.

```go
type MapGenConfig struct {
    // Road network
    MainRoadCount    int     // 2–3
    MainRoadWidth    int     // 5 tiles
    SideStreetCount  int     // 0–4
    SideStreetWidth  int     // 3 tiles
    PavementChance   float64 // 0.0–1.0, scaled by urban density

    // Buildings
    MinBuildings     int
    MaxBuildings     int
    FurnitureChance  float64 // per-room probability of any furniture

    // Vegetation
    TreeDensity      float64 // base probability per qualifying tile
    BushDensity      float64
    HedgerowDensity  float64

    // Fortifications
    TrenchCount      int
    ATBarrierCount   int
    WirePatches      int

    // Battle damage
    ExplosionCount   int
    ExplosionRadius  int // tiles

    // Noise
    UrbanFreq        float64
    VegFreq          float64
    RoughFreq        float64
    MoistureFreq     float64
}
```

---

## 11. Future Hooks

- **Multi-storey buildings** — Elevation > 0 for upper floors, stairwell tiles.
- **Destructible walls** — walls gain durability, heavy weapons can breach.
- **Weather effects** — rain increases moisture noise, turns dirt→mud, reduces visibility.
- **Sound propagation** — ground type affects footstep audibility (gravel loud, grass quiet).
- **Dynamic doors** — soldiers open/close doors as tactical actions.
- **Vehicle pathing** — vehicles use road tiles preferentially, cannot enter buildings or wire.
- **Map editor** — expose TileMap for hand-authored scenarios.
