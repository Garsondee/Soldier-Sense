# Flow-Field Movement System Redesign

**Status:** Planning  
**Priority:** Critical  
**Target:** Complete movement system overhaul  
**Date:** March 2, 2026

---

## Problem Statement

The current A* pathfinding and individual soldier movement system exhibits critical failures:

### Observable Symptoms
- **Jittering and vibration** instead of smooth, purposeful movement
- **Collision deadlocks** with 75-92% immobility rates in extended battles
- **Squad fragmentation** during coordinated maneuvers
- **Pathfinding failures** that never recover (80-97 consecutive stalled events)
- **State thrashing** with 50-109 idle transitions per soldier
- **Collision clusters** of 3-4 soldiers stuck together permanently

### Root Architectural Issues
1. **Individual pathfinding** creates conflicting movement vectors when soldiers converge
2. **No collision avoidance** in path planning - soldiers path through each other
3. **Reactive collision resolution** fails when multiple soldiers occupy same space
4. **No squad-level coordination** - each soldier acts independently
5. **Binary path success/failure** - no graceful degradation or alternative routing
6. **Expensive per-soldier pathfinding** - recomputation thrashing under load

### Impact
- 88% of extended battles have problematic soldiers
- 30-40% of force becomes non-functional in long engagements
- Squad maneuvers (regroup, formation, withdrawal) trigger mass immobility
- Combat effectiveness severely degraded

---

## Flow-Field Pathfinding Fundamentals

### Core Concept

Instead of computing individual paths for each soldier, compute a **vector field** that guides all units toward their objectives. Each grid cell contains a direction vector pointing toward the goal.

### How It Works

1. **Goal Selection**: Identify target position(s) for squad/group
2. **Cost Field Generation**: Compute traversal cost for each cell (terrain, cover, threats)
3. **Integration Field**: Dijkstra-like flood-fill from goal, computing cumulative cost
4. **Flow Field**: For each cell, compute gradient vector pointing toward lowest-cost neighbor
5. **Movement**: Soldiers sample flow field at their position and move along vector

### Key Advantages

**For Our Use Case:**
- ✅ **Shared computation**: One flow field serves entire squad
- ✅ **Natural collision avoidance**: Soldiers follow similar but not identical paths
- ✅ **Smooth movement**: Continuous vector field eliminates jitter
- ✅ **Graceful degradation**: Blocked paths automatically route around obstacles
- ✅ **Squad coordination**: All soldiers use same field = coordinated movement
- ✅ **Dynamic updates**: Field can be recomputed incrementally as situation changes
- ✅ **Formation support**: Multiple goals create natural spacing
- ✅ **Scalability**: Cost is O(grid cells) not O(soldiers × path length)

**Compared to Current A*:**
| Feature | Current A* | Flow-Field |
|---------|-----------|------------|
| Computation | Per-soldier | Per-squad |
| Collision Avoidance | Reactive (fails) | Proactive (built-in) |
| Path Sharing | None | Automatic |
| Recomputation Cost | High (per soldier) | Low (amortized) |
| Coordination | None | Natural |
| Jitter | Severe | Minimal |
| Stuck Recovery | Manual/fails | Automatic |

---

## Proposed Architecture

### Layer 1: Grid and Cost Field

**NavGrid Enhancement:**
```
Current: NavGrid with walkable/unwalkable cells
Proposed: NavGrid with:
  - Base traversal cost (terrain difficulty)
  - Dynamic cost modifiers (cover value, threat exposure, friendly occupancy)
  - Threat heat map (enemy fire lanes, suppression zones)
  - Cover quality map (for tactical positioning)
```

**Cost Field Computation:**
- Base cost: Terrain difficulty (open=1.0, rough=1.5, obstacle=∞)
- Cover bonus: Reduce cost for cells with cover (-0.3 to -0.8)
- Threat penalty: Increase cost for cells under fire (+0.5 to +2.0)
- Friendly occupancy: Slight penalty to encourage spacing (+0.2 per soldier)
- Enemy proximity: Penalty for cells near enemies (unless engaging)

### Layer 2: Integration Field

**Dijkstra Flood-Fill:**
```
1. Initialize goal cell(s) with cost 0
2. Priority queue of cells to process
3. For each cell, compute cost = neighbor_cost + traversal_cost
4. Propagate outward until entire reachable area covered
5. Result: Each cell knows cumulative cost to reach goal
```

**Multi-Goal Support:**
- Squad formation: Multiple goal cells (one per soldier position)
- Regroup: Single goal cell (leader position)
- Engagement: Goal zone (firing positions around enemy)
- Withdrawal: Multiple fallback positions

**Incremental Updates:**
- Track "dirty" regions when costs change
- Only recompute affected portions of field
- Amortize updates over multiple frames
- Full recomputation only when goal changes significantly

### Layer 3: Flow Field Generation

**Gradient Computation:**
```
For each cell:
  - Sample 8 neighbors (N, NE, E, SE, S, SW, W, NW)
  - Find neighbor with lowest integration cost
  - Compute vector pointing toward that neighbor
  - Normalize to unit vector
  - Store as flow direction
```

**Vector Field Smoothing:**
- Average flow vectors with neighbors to reduce sharp turns
- Weight by integration cost gradient (stronger flow in clear directions)
- Preserve tactical features (don't smooth away cover approaches)

**Special Handling:**
- **Local minima**: Detect and mark, use alternative routing
- **Dead ends**: Flow toward nearest exit
- **Impassable regions**: Mark as "no flow" - soldiers stop and replan

### Layer 4: Soldier Movement

**Flow Following:**
```
Each tick:
  1. Sample flow field at current position
  2. Get flow vector (direction toward goal)
  3. Apply steering behaviors:
     - Alignment: Follow flow direction
     - Separation: Avoid immediate neighbors
     - Cohesion: Stay near squad mates
  4. Compute final velocity vector
  5. Apply movement with collision detection
  6. Update position
```

**Steering Behavior Weights:**
- Flow following: 0.6 (primary driver)
- Separation: 0.3 (collision avoidance)
- Cohesion: 0.1 (squad integrity)
- Obstacle avoidance: 1.0 (override when imminent collision)

**Collision Handling:**
- **Prediction**: Check if movement will collide
- **Avoidance**: Adjust velocity to avoid predicted collision
- **Sliding**: If blocked, slide along obstacle edge
- **Unstuck**: If stationary for >60 ticks, apply random jitter + recompute flow

### Layer 5: Squad Coordination

**Formation Movement:**
```
1. Define formation shape (line, wedge, column, etc.)
2. Compute formation goal positions relative to leader
3. Generate flow field with multiple goals (one per position)
4. Each soldier follows flow to their assigned position
5. Formation naturally emerges from flow field
```

**Coordinated Maneuvers:**
- **Advance**: Flow field toward contact, formation maintained
- **Regroup**: Flow field toward rally point, soldiers converge naturally
- **Withdrawal**: Flow field toward fallback positions, covering fire positions
- **Bounding**: Alternate flow fields for moving/stationary groups

**Dynamic Adaptation:**
- Flow field updates as tactical situation changes
- Cover positions automatically incorporated into cost field
- Threat zones dynamically adjust flow to avoid danger
- Casualties automatically recompute formation positions

---

## Tactical Enhancements

### Cover-Aware Movement

**Cost Field Integration:**
- Cells with cover have reduced cost when moving toward enemy
- Approach vectors favor covered routes
- Final positions bias toward cover cells
- Exposure penalty for open ground under fire

**Fire and Movement:**
- Moving group: Flow field toward objective
- Covering group: Flow field toward overwatch positions
- Automatic coordination through shared cost field
- Suppression zones increase cost, naturally routing around

### Threat Avoidance

**Dynamic Threat Map:**
- Enemy positions generate threat heat
- Suppression zones marked as high-cost
- Grenade/explosive danger zones temporarily impassable
- Sniper lanes marked as high-cost corridors

**Adaptive Routing:**
- Flow field automatically routes around threats
- Balance between speed (direct route) and safety (covered route)
- Aggression stat influences threat cost weighting
- Panic/fear increases threat avoidance

### Engagement Positioning

**Attack Flow Fields:**
- Goal zone: Arc of firing positions around enemy
- Cost field favors cover with good firing angles
- Natural flanking emerges from multi-goal field
- Spacing maintained through friendly occupancy cost

**Defensive Flow Fields:**
- Goal zone: Defensive perimeter
- Cost field favors cover facing enemy approach
- Overlapping fields of fire naturally emerge
- Fallback positions pre-computed for withdrawal

---

## Implementation Strategy

### Phase 1: Core Flow-Field Engine (Week 1-2)

**Deliverables:**
- Enhanced NavGrid with cost field support
- Integration field computation (Dijkstra flood-fill)
- Flow field generation (gradient computation)
- Basic flow following for individual soldiers
- Unit tests for field generation

**Success Criteria:**
- Single soldier can follow flow field to goal
- Field updates when goal changes
- No jitter in straight-line movement
- Handles static obstacles correctly

### Phase 2: Collision Avoidance & Steering (Week 3)

**Deliverables:**
- Separation steering behavior
- Cohesion steering behavior
- Predictive collision detection
- Sliding along obstacles
- Unstuck behavior for trapped soldiers

**Success Criteria:**
- Multiple soldiers move without colliding
- Soldiers naturally space themselves
- No permanent stuck states
- Smooth movement around obstacles

### Phase 3: Squad Coordination (Week 4)

**Deliverables:**
- Formation definitions (line, wedge, column)
- Multi-goal flow field generation
- Formation assignment and maintenance
- Squad-level flow field management
- Coordinated maneuver support

**Success Criteria:**
- Squad maintains formation while moving
- Regroup operations complete without collision
- Formation adapts to terrain
- Casualties don't break formation

### Phase 4: Tactical Integration (Week 5-6)

**Deliverables:**
- Cover-aware cost field
- Threat map integration
- Dynamic cost updates (suppression, threats)
- Engagement positioning flow fields
- Fire and movement coordination

**Success Criteria:**
- Soldiers prefer covered routes
- Avoid suppression zones
- Natural flanking behavior
- Coordinated bounding overwatch

### Phase 5: Optimization & Polish (Week 7-8)

**Deliverables:**
- Incremental field updates
- Field caching and reuse
- Performance profiling and optimization
- Visual debugging tools
- Comprehensive testing

**Success Criteria:**
- <1ms per squad flow field update
- Handles 10+ squads simultaneously
- No performance degradation in long battles
- Zero collision deadlocks in 100-run test

---

## Technical Specifications

### Data Structures

**CostField:**
```go
type CostField struct {
    width, height int
    baseCost     []float64  // Terrain difficulty
    coverBonus   []float64  // Cover value
    threatCost   []float64  // Enemy threat
    occupancy    []int      // Friendly soldier count
    dirty        []bool     // Needs recomputation
}

func (cf *CostField) GetCost(x, y int) float64 {
    base := cf.baseCost[y*cf.width+x]
    cover := cf.coverBonus[y*cf.width+x]
    threat := cf.threatCost[y*cf.width+x]
    occupancy := float64(cf.occupancy[y*cf.width+x]) * 0.2
    return base - cover + threat + occupancy
}
```

**IntegrationField:**
```go
type IntegrationField struct {
    width, height int
    cost         []float64  // Cumulative cost to goal
    goals        []Vec2i    // Goal cell positions
    computed     bool
}

func (ifield *IntegrationField) Compute(costField *CostField) {
    // Dijkstra flood-fill from goals
    // Priority queue by cost
    // Propagate until all reachable cells covered
}
```

**FlowField:**
```go
type FlowField struct {
    width, height int
    vectors      []Vec2     // Flow direction per cell
    integration  *IntegrationField
}

func (ff *FlowField) GetFlow(x, y int) Vec2 {
    return ff.vectors[y*ff.width+x]
}

func (ff *FlowField) Generate(integration *IntegrationField) {
    // For each cell, compute gradient toward lowest neighbor
    // Normalize to unit vector
    // Optional: smooth with neighbors
}
```

**SquadFlowController:**
```go
type SquadFlowController struct {
    squad        *Squad
    costField    *CostField
    flowField    *FlowField
    goals        []Vec2i
    updateTicks  int
    dirty        bool
}

func (sfc *SquadFlowController) Update() {
    if sfc.dirty || sfc.updateTicks > 60 {
        sfc.RecomputeFlow()
        sfc.updateTicks = 0
        sfc.dirty = false
    }
    sfc.updateTicks++
}

func (sfc *SquadFlowController) RecomputeFlow() {
    sfc.costField.Update()
    integration := NewIntegrationField(sfc.goals)
    integration.Compute(sfc.costField)
    sfc.flowField.Generate(integration)
}
```

### Movement Integration

**Soldier Movement Update:**
```go
func (s *Soldier) moveWithFlowField(dt float64) {
    // Sample flow field at current position
    flow := s.squad.flowController.flowField.GetFlow(s.cellX(), s.cellY())
    
    // Compute steering forces
    flowForce := flow.Scale(0.6)
    separationForce := s.computeSeparation().Scale(0.3)
    cohesionForce := s.computeCohesion().Scale(0.1)
    
    // Combine forces
    desiredVelocity := flowForce.Add(separationForce).Add(cohesionForce)
    
    // Apply obstacle avoidance
    if obstacle := s.detectImmediateObstacle(desiredVelocity); obstacle != nil {
        desiredVelocity = s.avoidObstacle(obstacle, desiredVelocity)
    }
    
    // Limit to max speed (stance-dependent)
    maxSpeed := s.getMaxSpeed()
    if desiredVelocity.Length() > maxSpeed {
        desiredVelocity = desiredVelocity.Normalize().Scale(maxSpeed)
    }
    
    // Apply movement
    s.x += desiredVelocity.X * dt
    s.y += desiredVelocity.Y * dt
    
    // Check if stuck
    if s.velocity.Length() < 0.1 {
        s.stuckTicks++
        if s.stuckTicks > 60 {
            s.applyUnstuckBehavior()
        }
    } else {
        s.stuckTicks = 0
    }
}
```

**Separation Steering:**
```go
func (s *Soldier) computeSeparation() Vec2 {
    separationRadius := 30.0 // pixels
    force := Vec2{0, 0}
    
    for _, other := range s.squad.members {
        if other == s {
            continue
        }
        
        dist := s.distanceTo(other)
        if dist < separationRadius && dist > 0 {
            // Push away from nearby soldiers
            away := Vec2{s.x - other.x, s.y - other.y}
            away = away.Normalize()
            strength := (separationRadius - dist) / separationRadius
            force = force.Add(away.Scale(strength))
        }
    }
    
    return force
}
```

**Cohesion Steering:**
```go
func (s *Soldier) computeCohesion() Vec2 {
    cohesionRadius := 100.0 // pixels
    center := Vec2{0, 0}
    count := 0
    
    for _, other := range s.squad.members {
        if other == s {
            continue
        }
        
        dist := s.distanceTo(other)
        if dist < cohesionRadius {
            center.X += other.x
            center.Y += other.y
            count++
        }
    }
    
    if count == 0 {
        return Vec2{0, 0}
    }
    
    center.X /= float64(count)
    center.Y /= float64(count)
    
    toward := Vec2{center.X - s.x, center.Y - s.y}
    return toward.Normalize()
}
```

### Cost Field Updates

**Dynamic Threat Integration:**
```go
func (cf *CostField) UpdateThreats(enemies []*Soldier) {
    // Clear previous threats
    for i := range cf.threatCost {
        cf.threatCost[i] = 0
    }
    
    // Add threat heat from each enemy
    for _, enemy := range enemies {
        ex, ey := enemy.cellX(), enemy.cellY()
        threatRadius := 15 // cells
        
        for dy := -threatRadius; dy <= threatRadius; dy++ {
            for dx := -threatRadius; dx <= threatRadius; dx++ {
                x, y := ex+dx, ey+dy
                if !cf.inBounds(x, y) {
                    continue
                }
                
                dist := math.Sqrt(float64(dx*dx + dy*dy))
                if dist > float64(threatRadius) {
                    continue
                }
                
                // Threat falls off with distance
                threat := 2.0 * (1.0 - dist/float64(threatRadius))
                idx := y*cf.width + x
                cf.threatCost[idx] += threat
                cf.dirty[idx] = true
            }
        }
    }
}
```

**Suppression Zone Integration:**
```go
func (cf *CostField) UpdateSuppression(suppressionZones []SuppressionZone) {
    for _, zone := range suppressionZones {
        for y := zone.minY; y <= zone.maxY; y++ {
            for x := zone.minX; x <= zone.maxX; x++ {
                if !cf.inBounds(x, y) {
                    continue
                }
                
                idx := y*cf.width + x
                // High cost for suppressed areas
                cf.threatCost[idx] += zone.intensity * 3.0
                cf.dirty[idx] = true
            }
        }
    }
}
```

---

## Formation System

### Formation Definitions

**Formation Types:**
```go
type FormationType int

const (
    FormationLine FormationType = iota
    FormationWedge
    FormationColumn
    FormationBox
    FormationSkirmish
)

type Formation struct {
    ftype     FormationType
    positions []Vec2  // Relative positions from leader
    spacing   float64
}
```

**Line Formation:**
```
Leader at center, soldiers spread left/right
Spacing: 25 pixels

[S] [S] [L] [S] [S]

Positions relative to leader:
  [-50, 0], [-25, 0], [0, 0], [25, 0], [50, 0]
```

**Wedge Formation:**
```
Leader at front, soldiers form V behind
Spacing: 30 pixels

      [L]
    [S] [S]
  [S]     [S]

Positions:
  [0, 0], [-30, -30], [30, -30], [-60, -60], [60, -60]
```

**Column Formation:**
```
Leader at front, soldiers follow in line
Spacing: 20 pixels

[L]
[S]
[S]
[S]
[S]

Positions:
  [0, 0], [0, -20], [0, -40], [0, -60], [0, -80]
```

### Formation Flow Fields

**Multi-Goal Generation:**
```go
func (sfc *SquadFlowController) SetFormation(formation *Formation) {
    // Compute world positions for each formation slot
    leaderPos := sfc.squad.leader.position()
    leaderFacing := sfc.squad.leader.facing
    
    goals := make([]Vec2i, len(formation.positions))
    for i, relPos := range formation.positions {
        // Rotate relative position by leader facing
        worldPos := rotatePoint(relPos, leaderFacing)
        worldPos = worldPos.Add(leaderPos)
        goals[i] = worldToCellCoords(worldPos)
    }
    
    sfc.goals = goals
    sfc.dirty = true
}
```

**Formation Assignment:**
```go
func (squad *Squad) AssignFormationPositions(formation *Formation) {
    // Assign each soldier to nearest formation position
    positions := formation.positions
    soldiers := squad.members
    
    // Hungarian algorithm for optimal assignment
    // Or simple greedy: each soldier takes nearest unassigned position
    
    for i, soldier := range soldiers {
        soldier.formationIndex = i
        soldier.formationGoal = positions[i]
    }
}
```

**Formation Maintenance:**
```go
func (sfc *SquadFlowController) MaintainFormation() {
    // Check if formation is broken
    maxDeviation := 50.0 // pixels
    broken := false
    
    for _, soldier := range sfc.squad.members {
        goalPos := sfc.getFormationGoalPosition(soldier)
        dist := soldier.position().Distance(goalPos)
        if dist > maxDeviation {
            broken = true
            break
        }
    }
    
    if broken {
        // Recompute formation flow field
        sfc.SetFormation(sfc.squad.formation)
    }
}
```

---

## Debugging and Visualization

### Visual Debug Overlays

**Flow Field Visualization:**
- Draw arrow at each cell showing flow direction
- Color by integration cost (blue=low, red=high)
- Highlight goal cells in green
- Show impassable cells in black

**Cost Field Visualization:**
- Heat map overlay showing total cost
- Separate layers for terrain, cover, threat, occupancy
- Toggle individual layers on/off
- Real-time updates as costs change

**Soldier Debug Info:**
- Current flow vector (arrow from soldier)
- Separation forces (red arrows)
- Cohesion force (blue arrow)
- Desired velocity (green arrow)
- Stuck counter (text overlay)

### Debug Commands

```go
// Toggle flow field visualization
debug.ShowFlowField(squad)

// Show cost field for specific layer
debug.ShowCostField(squad, CostLayerThreat)

// Highlight stuck soldiers
debug.HighlightStuckSoldiers()

// Show formation goals
debug.ShowFormationGoals(squad)

// Trace soldier movement history
debug.TraceSoldierPath(soldier, lastNTicks)
```

### Performance Monitoring

**Metrics to Track:**
- Flow field computation time per squad
- Cost field update time
- Number of dirty cells per update
- Soldiers stuck >60 ticks
- Average velocity per soldier
- Formation deviation

**Telemetry:**
```go
type FlowFieldMetrics struct {
    computeTimeMs      float64
    dirtyCellCount     int
    soldierCount       int
    stuckSoldiers      int
    avgVelocity        float64
    formationDeviation float64
}

func (sfc *SquadFlowController) CollectMetrics() FlowFieldMetrics {
    // Gather performance and behavior metrics
    // Log to telemetry system
    // Alert if thresholds exceeded
}
```

---

## Testing Strategy

### Unit Tests

**Flow Field Generation:**
- Single goal, empty field → straight line flow
- Multiple goals → flow splits appropriately
- Obstacles → flow routes around
- Impassable regions → no flow vectors

**Cost Field:**
- Threat zones increase cost correctly
- Cover reduces cost correctly
- Occupancy adds cost correctly
- Dynamic updates mark dirty cells

**Steering Behaviors:**
- Separation pushes soldiers apart
- Cohesion pulls toward group center
- Flow following tracks field direction
- Combined forces produce expected movement

### Integration Tests

**Squad Movement:**
- Squad reaches goal without collision
- Formation maintained during movement
- Regroup completes successfully
- Withdrawal executes without stuck soldiers

**Tactical Scenarios:**
- Soldiers prefer covered routes
- Avoid suppression zones
- Flank naturally when multiple goals
- Bounding overwatch coordination

### Regression Tests

**Collision Scenarios:**
- 4 soldiers converging on same point → no deadlock
- Squad passing through doorway → orderly flow
- Two squads crossing paths → smooth passage
- Casualty evacuation → medic reaches patient

**Performance Tests:**
- 10 squads (60 soldiers) moving simultaneously
- 20,000 tick battle with no stuck soldiers
- Flow field updates <1ms per squad
- No memory leaks over extended battles

### Headless Test Integration

**Success Criteria:**
- Zero soldiers with >50% immobility
- Zero collision proximity issues
- Zero pathfinding failures
- <5% soldiers with >30% separation
- All squads complete regroup/formation maneuvers

**Automated Validation:**
```go
func ValidateFlowFieldSystem(testRuns int) {
    for i := 0; i < testRuns; i++ {
        sim := RunHeadlessTest(20000)
        problematic := AnalyzeSoldierPerformance(sim)
        
        assert(len(problematic) == 0, "No problematic soldiers")
        assert(sim.CollisionEvents == 0, "No collision deadlocks")
        assert(sim.PathfindingFailures == 0, "No pathfinding failures")
    }
}
```

---

## Migration Plan

### Backward Compatibility

**Dual System Operation:**
- Keep existing A* pathfinding as fallback
- Add feature flag: `useFlowFieldMovement`
- Gradually migrate goals to flow field
- Monitor performance and behavior
- Full cutover when validated

**Incremental Rollout:**
1. **Week 1-2**: Flow field for simple movement goals only
2. **Week 3-4**: Add formation movement
3. **Week 5-6**: Add tactical features (cover, threats)
4. **Week 7-8**: Full migration, remove A* fallback

### Risk Mitigation

**Rollback Plan:**
- Feature flag allows instant revert
- A* system remains intact during migration
- Comprehensive logging of flow field decisions
- Side-by-side comparison in headless tests

**Validation Gates:**
- Each phase requires passing headless tests
- No regression in combat effectiveness
- Performance must meet targets
- Zero increase in stuck soldier rate

---

## Success Metrics

### Quantitative Goals

| Metric | Current | Target | Measurement |
|--------|---------|--------|-------------|
| Stuck Soldiers (>50% immobile) | 123/310 (40%) | 0% | Headless test |
| Collision Deadlocks | 128/310 (41%) | 0% | Proximity analysis |
| Pathfinding Failures | 80-97 events | 0 | Stalled event count |
| Squad Regroup Success | ~60% | 100% | Maneuver completion |
| Formation Maintenance | Poor | >90% | Deviation tracking |
| Movement Smoothness | Jittery | Smooth | Visual inspection |
| Flow Field Compute Time | N/A | <1ms | Performance profiling |

### Qualitative Goals

- ✅ Soldiers move with purpose and confidence
- ✅ Squads maintain cohesion during maneuvers
- ✅ Natural flanking and tactical positioning
- ✅ No visible jitter or vibration
- ✅ Graceful handling of blocked paths
- ✅ Coordinated fire and movement
- ✅ Believable squad behavior

---

## Future Enhancements

### Advanced Features (Post-MVP)

**Hierarchical Flow Fields:**
- Strategic layer: Squad-level objectives
- Tactical layer: Individual positioning
- Micro layer: Immediate collision avoidance
- Seamless blending between layers

**Predictive Flow Fields:**
- Anticipate enemy movement
- Pre-compute fallback routes
- Dynamic threat prediction
- Proactive positioning

**Learning and Adaptation:**
- Track which routes succeed/fail
- Adjust cost weights based on outcomes
- Learn terrain preferences
- Adapt to player tactics

**Multi-Squad Coordination:**
- Shared cost fields for multiple squads
- Coordinated flanking maneuvers
- Traffic management for choke points
- Synchronized assaults

### Performance Optimizations

**GPU Acceleration:**
- Compute flow fields on GPU
- Parallel field generation
- Real-time updates for large maps
- Support for 100+ simultaneous squads

**Spatial Hashing:**
- Efficient neighbor queries
- Fast collision detection
- Optimized separation/cohesion
- Reduced computational overhead

**Lazy Evaluation:**
- Only compute fields for active squads
- Cache and reuse stable fields
- Incremental updates for dynamic regions
- Adaptive update rates based on activity

---

## Conclusion

The flow-field movement system represents a **fundamental architectural shift** from individual pathfinding to coordinated, field-based movement. This addresses the root causes of current movement failures:

**Problems Solved:**
- ✅ Collision deadlocks → Natural spacing through separation steering
- ✅ Pathfinding failures → Graceful degradation through continuous field
- ✅ Squad fragmentation → Coordinated movement through shared field
- ✅ Jitter and vibration → Smooth flow following
- ✅ State thrashing → Continuous movement without discrete path steps

**Implementation Approach:**
- 8-week phased rollout
- Incremental migration with fallback
- Comprehensive testing at each phase
- Clear success metrics and validation gates

**Expected Outcome:**
- Zero collision deadlocks in extended battles
- Smooth, purposeful squad movement
- Natural tactical behavior (flanking, cover usage)
- Scalable to large battles (10+ squads)
- Foundation for advanced AI behaviors

This redesign transforms movement from a **source of failures** into a **foundation for emergent tactical behavior**.
