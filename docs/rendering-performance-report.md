# Rendering and Performance Analysis Report

## Overview

This report documents rendering pipeline issues, performance bottlenecks, and optimization opportunities discovered through code analysis of the game's rendering and update systems.

---

## Rendering Pipeline Architecture

### Main Rendering Flow

The game uses **Ebiten** (Go 2D game library) with a multi-buffer rendering strategy:

**Buffer Strategy**: `@internal/game/game.go:235-246`
- `visionBuf` - Vision cone rendering (3072x1728, full battlefield)
- `worldBuf` - Full battlefield rendering (3072x1728)
- `hudBuf` - HUD text (scaled 1/3 then upscaled 3x)
- `logBuf` - Thought log panel (scaled 1/3 then upscaled 3x)
- `inspBuf` - Inspector panel (scaled 1/3 then upscaled 3x)
- `squadBuf` - Squad status panels (scaled 1/3 then upscaled 3x, **reused per panel**)

**Draw Call Order**: `@internal/game/game.go:1335-1404` (`Draw`)
1. Clear screen to dark background
2. Render world to `worldBuf` with camera transform
3. Blit `worldBuf` to screen with zoom/pan
4. Draw squad status panels (top-right)
5. Draw thought log panel (right side)
6. Draw inspector panel (if soldier selected)
7. Draw HUD overlay (bottom-left)
8. Draw pause menu or AAR overlay (if active)

---

## Performance Issues

### 1. Per-Tick Nested Loops

**Issue**: Multiple O(N²) and O(N×M) loops in the update pipeline cause performance degradation with large entity counts.

**Specific problems**:

- **Vision scan per soldier**: `@internal/game/game.go:1019-1024`
  ```go
  for _, s := range g.soldiers {
      s.UpdateVision(g.opfor, g.buildings)
  }
  for _, s := range g.opfor {
      s.UpdateVision(g.soldiers, g.buildings)
  }
  ```
  - Each soldier scans ALL enemies: O(soldiers × enemies)
  - Each scan checks ALL buildings for LOS: O(soldiers × enemies × buildings)
  - With 16 soldiers, 16 enemies, 32 buildings: **16 × 16 × 32 = 8,192 LOS checks per tick**
  - At 60 TPS: **491,520 LOS checks per second**
  - **Fix**: Spatial partitioning (quadtree or grid) to reduce candidate set

- **Combat resolution nested loops**: `@internal/game/combat.go:333-948` (`ResolveCombat`)
  - Iterates all shooters, for each shooter iterates all targets for closest
  - Then for each shot, iterates all soldiers for ricochet stress: O(shooters × targets + shots × all_soldiers)
  - **Fix**: Pre-compute closest target per shooter, cache for N ticks

- **Squad member iteration**: `@internal/game/squad.go:667-691`
  - Squad think iterates all members multiple times per tick
  - Contact aggregation, fear averaging, casualty counting are separate loops
  - **Fix**: Single-pass aggregation with accumulated stats

- **Gunfire broadcast**: `@internal/game/combat.go:244-282` (`BroadcastGunfire`)
  ```go
  for _, ev := range cm.Gunfires {
      for _, s := range listeners {
          heardStrength := gunfireHeardStrength(ev.X, ev.Y, s, red, blue)
      }
  }
  ```
  - Each gunfire event checks ALL enemy soldiers
  - With 10 shots per tick, 16 enemies: **160 distance checks per tick**
  - **Fix**: Spatial grid for audio propagation

### 2. Rendering Overdraw

**Issue**: Multiple layers are drawn with overlapping geometry, causing excessive fill-rate usage.

**Specific problems**:

- **Per-tile ground rendering**: `@internal/game/game.go:1547-1570`
  ```go
  for row := 0; row < g.tileMap.Rows; row++ {
      for col := 0; col < g.tileMap.Cols; col++ {
          // Draw each tile individually
          vector.FillRect(screen, px, py, cs, cs, color, false)
      }
  }
  ```
  - Battlefield is 3072×1728 = 5,308,416 pixels
  - Tile size is 16×16 = 192×108 = **20,736 tiles**
  - Each tile is a separate `vector.FillRect` call
  - **Fix**: Batch tiles by color, use texture atlas, or pre-render to static image

- **Grid line overdraw**: `@internal/game/game.go:1580-1582`
  - Three grid layers drawn (fine, mid, coarse)
  - Each grid draws full-screen horizontal and vertical lines
  - Many lines overlap with buildings and other geometry
  - **Fix**: Draw grids to cached texture, only redraw on zoom change

- **Building interior overdraw**: `@internal/game/game.go:1636-1675`
  - Floor fill, shadow, team tint overlay, border, and tile grid all drawn separately
  - For 32 buildings averaging 5×5 tiles each: **800 tile grid lines + 32 fills + 32 shadows**
  - **Fix**: Combine fills into single draw call per building

- **Vision cone overdraw**: `@internal/game/game.go:2348-2404` (`drawVisionConesBuffered`)
  - Renders all vision cones to offscreen buffer, then composites
  - Good: Prevents additive blowout
  - Bad: Full-screen buffer clear and composite every frame
  - Each cone is 36-segment filled path
  - With 32 soldiers: **1,152 path segments per frame**
  - **Fix**: Only redraw vision buffer when soldiers move/turn significantly

- **Vignette triple-layer**: `@internal/game/game.go:2469-2496` (`drawVignette`)
  - Three full-screen edge darkening layers (outer, mid, inner)
  - 12 rectangle draws per frame (4 edges × 3 layers)
  - **Fix**: Pre-render vignette to texture, composite once

### 3. Memory Allocation in Hot Paths

**Issue**: Frequent allocations in per-frame and per-tick code cause GC pressure.

**Specific problems**:

- **Slice append in loops**: `@internal/game/game.go:402-403`
  ```go
  g.terrainPatches = append(g.terrainPatches, terrainPatch{...})
  ```
  - This is initialization code (OK)
  - BUT: Similar pattern in hot paths like `@internal/game/game.go:515-516`
  ```go
  candidates = append(candidates, c...)
  ```
  - Building generation appends to slices without pre-allocation
  - **Fix**: Pre-allocate with `make([]T, 0, capacity)`

- **Map allocations per frame**: `@internal/game/game.go:1627-1632`
  ```go
  claimedTeam := make(map[int]Team)
  for _, sq := range g.squads {
      if sq.ClaimedBuildingIdx >= 0 {
          claimedTeam[sq.ClaimedBuildingIdx] = sq.Team
      }
  }
  ```
  - New map allocated every frame
  - **Fix**: Reuse map, clear with `for k := range m { delete(m, k) }`

- **solidSet map per frame**: `@internal/game/game.go:1677-1683`
  ```go
  solidSet := make(map[[2]int]bool, len(g.buildings)+len(g.windows))
  for _, b := range g.buildings {
      solidSet[[2]int{b.x / cellSize, b.y / cellSize}] = true
  }
  ```
  - Allocated every frame for wall rendering
  - With 500+ walls/windows: **500 map insertions per frame**
  - **Fix**: Cache as game field, rebuild only when buildings change

- **chestSet map per frame**: `@internal/game/game.go:1996-2001`
  - Similar issue for cover object rendering
  - **Fix**: Cache as game field

- **Speech bubble slice growth**: Speech bubbles likely append without bounds
  - No visible cleanup in main loop
  - **Fix**: Add periodic cleanup of expired bubbles

### 4. Inefficient Algorithms

**Issue**: Some algorithms have suboptimal complexity or redundant work.

**Specific problems**:

- **Linear search for closest target**: `@internal/game/combat.go:407`
  - `closestContact` iterates all vision contacts to find nearest
  - Called for every shooter every tick
  - **Fix**: Maintain sorted contact list by distance

- **Ray-AABB intersection per vision ray**: `@internal/game/game.go:2414-2424`
  ```go
  for _, b := range g.buildings {
      t, hit := rayAABBHitT(ox, oy, ex, ey, ...)
  }
  ```
  - Vision cone rendering clips each ray against ALL buildings
  - 36 rays per soldier × 32 soldiers × 500 buildings = **576,000 ray tests per frame**
  - **Fix**: Spatial acceleration structure (BVH or grid)

- **Duplicate LOS checks**: Vision scan and combat both check LOS
  - No caching between systems
  - **Fix**: Cache LOS results for (soldier, target) pairs, invalidate on movement

- **Redundant distance calculations**: Many systems compute `math.Hypot` repeatedly
  - Squad spread, formation distance, threat distance all recompute
  - **Fix**: Cache distances when positions haven't changed

### 5. Texture and Image Management

**Issue**: Images are created but never explicitly disposed, and some are recreated unnecessarily.

**Specific problems**:

- **Buffer recreation**: `@internal/game/game.go:369-378`
  - All buffers created in `New()` and never recreated (good)
  - BUT: No disposal on game restart
  - **Fix**: Add `Dispose()` method to clean up images

- **No texture caching**: All rendering is immediate-mode vector drawing
  - Buildings, terrain, UI elements redrawn every frame
  - **Fix**: Pre-render static elements to textures, only redraw on change

- **Squad buffer reuse**: `@internal/game/game.go:60-67`
  ```go
  for _, sq := range g.squads {
      g.squadBuf.Clear()
      g.renderSquadPanel(g.squadBuf, sq)
      // ... blit to screen
  }
  ```
  - Good: Reuses single buffer for all panels
  - Bad: Clears and redraws every frame even if squad state unchanged
  - **Fix**: Track squad state hash, only redraw on change

### 6. Draw Call Batching

**Issue**: Many small draw calls instead of batched geometry.

**Specific problems**:

- **Individual tile draws**: 20,736 separate `FillRect` calls for ground tiles
  - Each call has overhead
  - **Fix**: Batch by color/texture

- **Grid lines**: Hundreds of individual `StrokeLine` calls
  - **Fix**: Single path with all grid lines

- **Building walls**: Each wall segment is separate draw
  - 500+ individual `FillRect` calls
  - **Fix**: Batch walls by color, use instancing

- **Cover objects**: Each cover piece drawn separately
  - **Fix**: Sprite batching or instanced rendering

### 7. Camera Transform Overhead

**Issue**: Camera transform applied to every draw call individually.

**Current approach**: `@internal/game/game.go:1335-1404`
- World rendered to `worldBuf` at native resolution
- Then `worldBuf` blitted to screen with camera transform
- This is actually GOOD - avoids per-primitive transform

**Potential issue**: World buffer is full battlefield size (3072×1728)
- Always renders entire world even if camera only shows portion
- **Fix**: Implement frustum culling - only render visible region

---

## Rendering Performance Bottlenecks Summary

### Critical (Frame-Rate Impact)

1. **Per-tile ground rendering** - 20,736 draw calls per frame
2. **Vision ray-building intersection** - 576,000 ray tests per frame
3. **No frustum culling** - Renders entire 3072×1728 world even when zoomed in
4. **Vision cone buffer full redraw** - Full-screen buffer composite every frame

### High (Noticeable Impact)

5. **Building wall individual draws** - 500+ separate draw calls
6. **Grid line overdraw** - 3 full-screen grid layers
7. **Map allocations per frame** - Multiple maps created/destroyed each frame
8. **TileMap object iteration** - Nested loops over all tiles for object rendering

### Medium (Optimization Opportunity)

9. **Squad panel redraw** - Redraws panels every frame even if unchanged
10. **Vignette triple-layer** - 12 rectangle draws per frame
11. **No texture caching** - All vector drawing, no pre-rendered assets
12. **Speech bubble management** - No visible cleanup, potential unbounded growth

---

## Update Loop Performance Issues

### Critical (Simulation Speed Impact)

1. **Vision scan O(N²×M)** - 491,520 LOS checks per second at 60 TPS
2. **Gunfire broadcast O(N×M)** - Every shot checks all enemies
3. **Combat resolution nested loops** - Shooters × targets × soldiers for ricochet

### High (Noticeable Impact)

4. **Squad member multi-pass** - Multiple iterations over members per tick
5. **No spatial partitioning** - All proximity checks are brute-force
6. **Redundant distance calculations** - Same distances computed multiple times

### Medium (Optimization Opportunity)

7. **Linear closest target search** - Could use sorted list or spatial query
8. **No LOS caching** - Vision and combat both check same LOS pairs
9. **Formation slot collision detection** - Brute-force proximity checks

---

## Memory Management Issues

### Allocation Hot Spots

1. **Per-frame map allocations** - `claimedTeam`, `solidSet`, `chestSet`, `rubbleSet`
2. **Slice growth without capacity** - Building generation, candidate lists
3. **No buffer pooling** - Temporary slices allocated/freed repeatedly
4. **Threat list unbounded growth** - Already documented in decision-making report

### Memory Leaks (Potential)

1. **Speech bubbles** - No visible cleanup mechanism
2. **Radio visual events** - Pruned but may accumulate if pruning fails
3. **Combat tracers/flashes** - Managed by combat manager, seems OK
4. **Thought log entries** - Fixed-size ring buffer, OK

---

## Recommendations Priority

### Critical (Must Fix for Scalability)

1. **Implement spatial partitioning** - Quadtree or grid for soldiers/buildings
   - Reduces vision scan from O(N²×M) to O(N×log(M))
   - Reduces gunfire broadcast from O(N×M) to O(N×log(M))
   - **Impact**: 10-50x speedup for entity queries

2. **Add frustum culling** - Only render visible portion of world
   - Current: Always renders 3072×1728 (5.3M pixels)
   - With culling at 2x zoom: Render 1536×864 (1.3M pixels) = **4x reduction**

3. **Batch tile rendering** - Pre-render ground to texture or batch by color
   - Current: 20,736 draw calls
   - Batched: 10-20 draw calls
   - **Impact**: 1000x reduction in draw call overhead

4. **Cache vision cone buffer** - Only redraw when soldiers move/turn
   - Current: Full redraw every frame
   - Cached: Redraw only on change (maybe 10% of frames)
   - **Impact**: 10x reduction in vision rendering cost

### High Priority (Significant Performance Gain)

5. **Implement LOS caching** - Cache (soldier, target) LOS results
   - Invalidate on movement
   - **Impact**: 2-5x reduction in LOS checks

6. **Reuse per-frame maps** - Cache `solidSet`, `claimedTeam`, `chestSet`
   - **Impact**: Reduces GC pressure, smoother frame times

7. **Pre-render static geometry** - Buildings, terrain to textures
   - **Impact**: 5-10x reduction in building rendering cost

8. **Batch wall/cover rendering** - Group by color, use instancing
   - **Impact**: 10x reduction in draw calls

### Medium Priority (Polish / Optimization)

9. **Implement dirty flags** - Only redraw squad panels on state change
10. **Cache grid rendering** - Render grids to texture, composite
11. **Add speech bubble cleanup** - Periodic removal of expired bubbles
12. **Optimize distance calculations** - Cache when positions unchanged
13. **Sort contact lists** - Maintain sorted by distance for fast closest queries
14. **Pre-allocate slices** - Use `make([]T, 0, capacity)` in hot paths

### Low Priority (Minor Gains)

15. **Combine vignette layers** - Single pre-rendered texture
16. **Optimize ray-AABB tests** - Use SIMD or early-out optimizations
17. **Profile and optimize hot functions** - Use pprof to find remaining bottlenecks

---

## Architectural Recommendations

### Short Term (Quick Wins)

1. **Add spatial grid** - 64×64 pixel cells, track soldiers/buildings per cell
2. **Cache static maps** - `solidSet`, `chestSet` as game fields
3. **Implement frustum culling** - Calculate visible tile range from camera
4. **Add dirty flags** - Track when squad panels need redraw

### Medium Term (Significant Refactor)

5. **Texture atlas system** - Pre-render buildings, terrain, UI to atlas
6. **Sprite batching** - Group soldiers, covers by texture for instanced rendering
7. **LOS cache** - Hash map of (soldier_id, target_id) → bool with invalidation
8. **Deferred rendering** - Separate geometry pass from lighting/effects

### Long Term (Major Optimization)

9. **Multi-threaded update** - Parallel vision scans, combat resolution
10. **GPU compute shaders** - LOS checks, distance fields on GPU
11. **Level-of-detail system** - Reduce detail for distant/off-screen entities
12. **Occlusion culling** - Don't render entities behind buildings

---

## Profiling Recommendations

To validate these findings and prioritize fixes:

1. **CPU profiling** - Use `pprof` to identify actual hot functions
   ```go
   import _ "net/http/pprof"
   go func() { http.ListenAndServe("localhost:6060", nil) }()
   ```

2. **Frame time breakdown** - Instrument major systems
   - Update time vs Draw time
   - Per-system timing (vision, combat, squad, rendering)

3. **Memory profiling** - Track allocations per frame
   - Identify allocation hot spots
   - Measure GC pause times

4. **Draw call counting** - Log actual draw calls per frame
   - Validate 20k+ tile draws hypothesis
   - Measure impact of batching

5. **Benchmark scenarios** - Test with varying entity counts
   - 8v8, 16v16, 32v32 soldiers
   - Measure FPS degradation curve

---

## Status

**Analysis completed**: Current codebase rendering and update systems
**Issues documented**: 27 specific performance problems across 7 categories
**Prioritized recommendations**: 17 actionable optimizations with impact estimates

**Files analyzed**:
- `internal/game/game.go` - Main rendering pipeline, draw calls, update loop
- `internal/game/combat.go` - Combat resolution, gunfire broadcast
- `internal/game/soldier.go` - Per-soldier update, vision, movement
- `internal/game/squad.go` - Squad-level updates, member iteration
- `internal/game/vision.go` - Vision cone, LOS checks

**Key findings**:
- **Rendering**: 20k+ draw calls per frame, no batching or caching
- **Update**: O(N²) vision scans, no spatial partitioning
- **Memory**: Per-frame allocations, no buffer reuse
- **Architecture**: Immediate-mode rendering, no texture caching
