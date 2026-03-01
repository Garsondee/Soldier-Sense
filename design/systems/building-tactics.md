# Building Tactics & Squad-Building Interaction

## Executive Summary

Squads currently have a **basic building claim system** that works at a tactical level but lacks strategic intelligence. The system successfully identifies and claims buildings along the advance route, and soldiers can seek building positions when under fire. However, there are significant gaps in how squads **plan**, **prioritize**, and **tactically exploit** buildings as force multipliers.

## Current Implementation

### What Works

#### 1. Building Claim System (`squad.go:1543-1680`)
- **Periodic Evaluation**: Leader evaluates buildings every 300 ticks (~5s), or 60 ticks when under fire
- **Scoring Heuristics**:
  - Dot product with advance direction (prefer buildings ahead, not behind)
  - Distance penalty (max 400px range)
  - Contact-aware scoring (bonus for buildings between squad and enemy)
  - Size bonus (larger buildings = more cover)
  - Suppression urgency bonus
  - Overlap bonus (if squad members already near/in building)
- **Lifecycle Management**:
  - Abandons building after 420 ticks of no contact + 150 ticks occupied
  - Cooldown period (360 ticks) after abandoning
  - Propagates claim to all squad members via blackboard

#### 2. Tactical Map System (`tactical_map.go`)
- **Rich Cell Traits**:
  - `CellTraitInterior`: inside building footprint
  - `CellTraitCorner`: building corners (high value)
  - `CellTraitDoorway`: chokepoints (negative value)
  - `CellTraitDoorAdj`: door-adjacent (good firing position)
  - `CellTraitWindow`: window cells (blocks movement)
  - `CellTraitWindowAdj`: interior cells next to windows (prime overwatch)
  - `CellTraitWallAdj`: wall-adjacent cells
- **Desirability Scoring**:
  - Corners: +0.7
  - Window-adjacent: +0.85 (highest)
  - Door-adjacent: +0.4
  - Wall-adjacent: +0.2
  - Interior (baseline): +0.15
  - Doorways: -0.6 (exposed chokepoint)
- **ScanBestNearby**: Finds optimal tactical positions considering:
  - Base desirability
  - Distance penalty
  - Enemy bearing alignment
  - Window facing bonus
  - Claimed building bonus (+0.50)

#### 3. Soldier Building Interaction (`soldier.go:1595-1684`)
- **shouldSeekClaimedBuilding**: Conditions for seeking building
  - Must have claimed building and not already inside
  - Under fire OR visible threats OR officer MoveTo order
  - Distance/ETA checks (max 360px, various tick thresholds)
- **moveToClaimedBuilding**: Pathfinding to best position in building
  - Uses `ScanBestNearby` with enemy bearing
  - Requests crouching stance
  - Repaths when target drifts >32px

#### 4. Goal System Integration (`blackboard.go:1305-1323`)
- **Overwatch Utility Bonuses**:
  - Corner: +0.15
  - Wall (not doorway): +0.05
  - Window-adjacent: +0.30
  - Interior (not doorway): +0.10
  - Doorway: -0.20
- **Hold Position Bonuses** (lines 1345-1358):
  - Position desirability: +0.15 scaling
  - Window-adjacent: +0.25
  - Interior: +0.10
  - Doorway: -0.15

### What's Missing

#### 1. **No Strategic Building Assessment**
- Squads don't evaluate buildings for **defensive value** before contact
- No concept of "key terrain" or "dominant positions"
- Can't identify buildings that control important sightlines or chokepoints
- No understanding of multi-story advantage (all buildings treated as 2D footprints)

#### 2. **Poor Building Prioritization**
- Scoring is purely geometric (ahead + close + big = good)
- Doesn't consider:
  - **Tactical dominance**: Does this building overlook the enemy's likely approach?
  - **Cover quality**: Stone building vs wooden shed
  - **Entry/exit count**: Single door = death trap, multiple exits = flexible
  - **Interior layout**: Open floor vs compartmentalized rooms
  - **Fire arcs**: Can you actually shoot from this building toward the objective?

#### 3. **No Coordinated Building Assault**
- When a building is claimed, soldiers individually path to it
- No breach/clear/hold phases
- No designated entry team vs overwatch team
- No suppression of windows before entry
- No concept of "stacking" at doors or coordinated entry

#### 4. **Weak Building Defense**
- Once inside, soldiers use overwatch goal but don't:
  - Assign sectors of fire
  - Rotate between windows to avoid predictability
  - Prepare fallback positions
  - Coordinate interlocking fields of fire with adjacent buildings
  - Establish rally points if forced to evacuate

#### 5. **No Building-to-Building Movement**
- Squads don't plan **bounding between buildings**
- Can't identify "next good building" along advance route
- No concept of using buildings as waypoints in urban terrain
- Abandonment logic is time-based, not tactical (should abandon when better building is available)

#### 6. **Limited Building Intelligence**
- `AtInterior` flag is binary (inside/outside)
- No tracking of:
  - Which room/floor soldier is in
  - Which window/door they're covering
  - How long they've been static at one window
  - Whether they have clear fire lanes from current position

#### 7. **No Enemy Building Awareness**
- Squads don't track which buildings enemies occupy
- Can't suppress enemy-held buildings before advancing past them
- No concept of "danger buildings" that need to be cleared or bypassed

#### 8. **Officer Orders Don't Leverage Buildings**
- `CmdAssault` doesn't specify "assault that building"
- `CmdHold` doesn't mean "hold this building"
- No `CmdClearBuilding` or `CmdOccupy` commands

## Observed Behavioral Issues

### Issue 1: Premature Abandonment
**Scenario**: Squad claims building, starts moving toward it, then abandons it before arrival because evaluation interval expires and a different building scores higher.

**Root Cause**: No commitment/hysteresis once a building is claimed. The 300-tick evaluation interval is independent of claim state.

### Issue 2: Poor Building Selection Under Fire
**Scenario**: Squad under fire claims nearest building, which is a small shed with one door, instead of a larger building 80px further away with multiple windows.

**Root Cause**: Distance penalty dominates scoring when suppressed. No "quality over proximity" logic.

### Issue 3: Clumping at Entry
**Scenario**: All squad members path to the same "best position" (usually window-adjacent corner), creating a traffic jam at the door.

**Root Cause**: `ScanBestNearby` returns same position for all soldiers. No spatial deconfliction.

### Issue 4: Static Overwatch
**Scenario**: Soldiers reach window positions and stay there indefinitely, even when enemy has clearly identified their position.

**Root Cause**: Overwatch goal has no "reposition timer" or "exposure tracking."

### Issue 5: Ignoring Buildings During Advance
**Scenario**: Squad advances across open ground past multiple buildings that would provide good cover/overwatch.

**Root Cause**: Building evaluation only triggers when under fire or at regular intervals. No proactive "use available cover" logic.

## Recommended Improvements

### Phase 1: Enhanced Building Assessment (Foundation)

#### 1.1 Building Quality Metrics
Add to building footprint data structure:
```go
type BuildingQuality struct {
    TacticalValue    float64 // 0-1: overall tactical worth
    CoverQuality     float64 // 0-1: protection level
    SightlineScore   float64 // 0-1: observation value
    AccessibilityScore float64 // 0-1: entry/exit options
    InteriorComplexity float64 // 0-1: room count, layout
    DominanceScore   float64 // 0-1: height/position advantage
}
```

Compute at map generation:
- Count windows, doors, corners
- Raycast from building to key terrain
- Measure elevation (if multi-story support added)
- Assess exposure (surrounded by open ground vs urban cluster)

#### 1.2 Improved Claim Scoring
Replace simple geometric scoring with multi-factor evaluation:
```
score = 
    tacticalValue * 0.35 +
    (1 - normalizedDistance) * 0.20 +
    contactAlignment * 0.25 +
    currentOccupancy * 0.10 +
    squadPhaseBonus * 0.10
```

Add hysteresis: once claimed, new building must score **0.25 higher** to trigger switch.

#### 1.3 Building State Tracking
Track per-building state in squad:
```go
type BuildingState struct {
    FootprintIdx     int
    ClaimTick        int
    OccupantIDs      []int
    AssignedSectors  map[int]Sector // soldierID -> sector
    LastContactTick  int
    ThreatLevel      float64 // 0-1: how dangerous is this building
}
```

### Phase 2: Coordinated Building Use (Tactics)

#### 2.1 Building Entry Coordination
When claiming a building:
1. Designate **entry team** (2-3 soldiers, highest discipline)
2. Designate **overwatch team** (remainder, cover entry)
3. Entry team bounds to door
4. Overwatch suppresses windows
5. Entry team enters, clears interior
6. Overwatch follows once "clear" signal

#### 2.2 Sector Assignment
Once inside:
- Divide building perimeter into sectors (N, NE, E, SE, S, SW, W, NW)
- Assign each soldier a primary sector based on:
  - Enemy bearing (priority to threat sectors)
  - Window availability
  - Soldier skill (best shots get best windows)
- Rotate sectors every 120-180 ticks to avoid predictability

#### 2.3 Position Deconfliction
Modify `ScanBestNearby` to accept `occupiedPositions []Vec2`:
- Penalize positions within 2 cells of occupied spots
- Returns diverse positions for squad members
- Prevents door clumping

### Phase 3: Building-Centric Maneuver (Strategy)

#### 3.1 Building Chain Planning
Squad leader identifies **building chain** along advance route:
```go
type BuildingChain struct {
    Buildings []int // footprint indices
    Waypoints []Vec2 // positions between buildings
    TotalDistance float64
    RiskScore float64 // exposure during bounds
}
```

Select chain that:
- Minimizes open ground exposure
- Maximizes overwatch coverage
- Provides fallback options

#### 3.2 Bound-by-Building Movement
Replace linear advance with building-to-building bounds:
1. Occupy Building A
2. Identify Building B (next in chain)
3. Overwatch team stays in A, covers approach to B
4. Assault team bounds to B
5. Once B secured, overwatch team follows
6. Repeat

#### 3.3 Enemy Building Tracking
Add to intel system:
```go
type BuildingIntel struct {
    FootprintIdx int
    EnemyPresence float64 // 0-1 confidence
    LastObservedTick int
    ThreatLevel float64 // how dangerous
    Cleared bool
}
```

Update when:
- Shots fired from building
- Enemy spotted in/near building
- Building cleared by friendly forces

### Phase 4: Advanced Building Tactics (Polish)

#### 4.1 Building Assault Doctrine
Add new officer command: `CmdClearBuilding`
- Specifies target building
- Triggers coordinated breach/clear sequence
- High priority, overrides most other goals

#### 4.2 Building Defense Doctrine
When holding a building:
- Prepare fallback route (identify exit, rally point)
- Stagger positions (don't all crowd windows)
- Suppress approaching enemies before they reach cover
- Abandon if:
  - Casualties > 50%
  - Enemy has superior position
  - Surrounded with no exit

#### 4.3 Urban Terrain Specialization
Add building-specific skills to soldier profiles:
- `UrbanWarfare`: bonus to building combat effectiveness
- `Breaching`: faster/safer building entry
- `RoomClearing`: reduced friendly fire in tight spaces

#### 4.4 Multi-Story Support (Future)
If buildings gain vertical dimension:
- Upper floors provide sightline advantage
- Stairs are chokepoints (like doorways)
- Roof positions are high-value overwatch
- Falling damage for jumping from windows

## Implementation Priority

### High Priority (Core Functionality)
1. **Building quality metrics** (Phase 1.1) - Foundation for all improvements
2. **Improved claim scoring with hysteresis** (Phase 1.2) - Fixes premature abandonment
3. **Position deconfliction** (Phase 2.3) - Fixes clumping
4. **Sector assignment** (Phase 2.2) - Makes building defense effective

### Medium Priority (Tactical Depth)
5. **Building state tracking** (Phase 1.3) - Enables coordination
6. **Entry coordination** (Phase 2.1) - Makes building assault realistic
7. **Enemy building tracking** (Phase 3.3) - Threat awareness

### Low Priority (Strategic Layer)
8. **Building chain planning** (Phase 3.1) - Complex but high payoff
9. **Bound-by-building movement** (Phase 3.2) - Requires chain planning
10. **Advanced doctrines** (Phase 4) - Polish and specialization

## Testing Strategy

### Unit Tests
- Building quality scoring (various footprints)
- Claim hysteresis (switching thresholds)
- Sector assignment (even distribution)
- Position deconfliction (no overlaps)

### Scenario Tests
- **Urban Advance**: Squad must cross town, using buildings as cover
- **Building Defense**: Squad holds building against assault
- **Building Assault**: Squad must clear enemy-occupied building
- **Building Bypass**: Squad must advance past enemy building without clearing it

### Metrics to Track
- Building occupancy rate (% of time in buildings during urban combat)
- Building claim churn (claims per minute)
- Position diversity (spread of soldiers within building)
- Building-related casualties (deaths while entering/exiting)
- Tactical effectiveness (kills per building occupied)

## Open Questions

1. **Should buildings be destructible?** (Affects long-term hold value)
2. **How to handle multi-team building occupation?** (Enemy on floor 1, friendly on floor 2)
3. **Should building interiors be fully navigable?** (Rooms, hallways) or abstract?
4. **How to represent building damage?** (Walls breached, windows broken)
5. **Should there be building types?** (House, warehouse, bunker, tower)

## Related Systems

- **Radio Communications**: Building interiors may degrade radio (Phase A affected)
- **Psychological Model**: Building provides morale bonus (safety, cover)
- **Medical System**: Buildings are ideal casualty collection points
- **Suppression**: Buildings reduce suppression accumulation
- **Vision**: Windows provide one-way visibility advantage
- **Navigation**: Building interiors need special pathfinding

## Conclusion

The current building system provides a **foundation** but lacks the **intelligence** to make buildings a central tactical element. Squads claim buildings reactively rather than proactively, use them passively rather than actively, and abandon them arbitrarily rather than strategically.

The recommended improvements transform buildings from "terrain features that provide cover" into **tactical objectives** that squads fight to control, defend, and exploit. This will make urban combat feel purposeful and intelligent rather than incidental.

**Next Steps**:
1. Implement building quality metrics (Phase 1.1)
2. Add claim hysteresis to prevent thrashing (Phase 1.2)
3. Test with urban scenario (multiple buildings, contested terrain)
4. Iterate based on observed behavior
