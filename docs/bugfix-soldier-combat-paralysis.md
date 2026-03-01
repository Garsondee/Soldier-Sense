# Bug Analysis: Soldier Combat Paralysis

## Problem Report

Soldier R5 and leader R8 are stuck idle for 297/300 ticks despite having 3 visible threats:
- **R5**: Position (1405-1406, 1152), barely moves, stall=89, no firing
- **R8**: Position (1353-1361, 1193), barely moves, stall=89, no firing
- Both have `thr:3V/3` (3 visible threats) throughout
- Orders oscillate: `move_to` ↔ `bound` every 1-2 ticks
- Intent switches from `engage` → `regroup` at T=4222
- Formation slot distance chaos: R5's dslot jumps 23→178→181→247

## Root Causes Identified

### 1. Order Thrashing (Critical)
**Issue**: Squad leader issues different orders every tick, creating decision paralysis.

**Evidence**:
- T=4102-4103: `ord:bound`
- T=4104-4105: `ord:bound`  
- T=4106: `ord:move_to`
- T=4107: `ord:bound`
- T=4108: `ord:move_to`
- T=4109: `ord:bound`

**Cause**: `issueOfficerOrder()` only has 16-pixel hysteresis for target position, but no hysteresis for order **kind** changes. When conditions fluctuate slightly (e.g., phase changes, distance thresholds), orders flip-flop.

**Location**: `@c:\Users\Ingram\Documents\SoliderSense\internal\game\squad.go:506-536`

### 2. High Mobility Stall Without Recovery
**Issue**: Soldiers reach stall=89 and stay stuck indefinitely.

**Evidence**: Both R5 and R8 hit stall=89 and remain there for 40+ ticks.

**Cause**: Mobility stall recovery only triggers at threshold (30-90 ticks depending on context), but if pathfinding keeps failing, soldiers can exceed the threshold and stay stuck. The recovery action itself may also fail to find a path.

**Location**: `@c:\Users\Ingram\Documents\SoliderSense\internal\game\soldier.go:1772-1801`

### 3. Mid-Combat Intent Change
**Issue**: Squad switches from `IntentEngage` to `IntentRegroup` during active combat.

**Evidence**: T=4222 intent changes from `engage` to `regroup` while both soldiers have visible threats.

**Cause**: Squad intent selection doesn't have sufficient hysteresis or combat-lock to prevent intent changes during active engagement.

**Impact**: Soldiers abandon combat behavior and try to regroup, even though they're already near each other and should be fighting.

### 4. Formation Instability
**Issue**: Formation slots recalculate constantly, preventing stable positioning.

**Evidence**: R5's `dslot` (distance to formation slot) jumps wildly: 23→178→181→247 pixels.

**Cause**: Formation updates every tick without checking if the change is significant. Minor leader movement or intent changes trigger full formation recalculation.

**Location**: `@c:\Users\Ingram\Documents\SoliderSense\internal\game\squad.go` (UpdateFormation)

### 5. No Firing Despite Visible Threats
**Issue**: Soldiers have visible threats but don't fire.

**Analysis**: Combat resolution checks `len(s.vision.KnownContacts) == 0` at `@c:\Users\Ingram\Documents\SoliderSense\internal\game\combat.go:396`. Vision scans happen before combat resolution in the game loop, so `KnownContacts` should be populated. However, soldiers stuck in movement logic may not be firing due to:
- Being in wrong goal state (GoalRegroup instead of GoalEngage)
- Mobility stall preventing proper combat stance
- Order thrashing preventing commitment to any action

## Recommended Fixes

### Fix 1: Add Order Kind Hysteresis
Prevent order kind changes unless there's a significant reason:

```go
func (sq *Squad) issueOfficerOrder(tick int, kind OfficerCommandKind, ...) {
    // Current order is same kind and active - extend it
    if sq.ActiveOrder.Kind == kind && sq.ActiveOrder.State == OfficerOrderActive {
        // ... existing position check ...
        return
    }
    
    // NEW: Prevent rapid order kind changes
    if sq.ActiveOrder.State == OfficerOrderActive {
        ticksSinceIssued := tick - sq.ActiveOrder.IssuedTick
        if ticksSinceIssued < 60 { // 1 second minimum order duration
            // Only allow change if new order is much higher priority
            if priority <= sq.ActiveOrder.Priority + 0.15 {
                return // Keep current order
            }
        }
    }
    
    // Issue new order...
}
```

### Fix 2: Aggressive Stall Recovery
Add escalating recovery when stall exceeds threshold:

```go
if s.mobilityStallTicks >= stallThreshold {
    s.mobilityStallTicks = 0
    tx, ty := s.recoveryTargetHint()
    bb.ShatterEvent = true
    s.applyRecoveryAction(dt, tx, ty)
    return
}

// NEW: Extreme stall - force immediate action
if s.mobilityStallTicks > stallThreshold * 2 {
    s.mobilityStallTicks = 0
    bb.ShatterEvent = true
    // Force seek cover or hold position
    if bb.VisibleThreatCount() > 0 {
        s.seekCoverFromThreat(dt)
    } else {
        s.state = SoldierStateIdle
        s.faceNearestThreatOrContact()
    }
    s.think("EXTREME STALL - forcing recovery")
    return
}
```

### Fix 3: Combat Intent Lock
Prevent intent changes during active combat:

```go
// In SquadThink, before setting new intent:
if sq.Intent == IntentEngage && anyVisibleThreats > 0 {
    ticksInCombat := tick - sq.lastEngageStartTick
    if ticksInCombat < 180 { // 3 seconds minimum engagement
        // Lock intent to Engage unless overwhelming reason to change
        if candidateIntent != IntentEngage && candidateIntent != IntentWithdraw {
            candidateIntent = IntentEngage
        }
    }
}
```

### Fix 4: Formation Stability
Only recalculate formation if significant change:

```go
func (sq *Squad) UpdateFormation() {
    // Check if recalculation needed
    if sq.Leader != nil {
        dx := sq.Leader.x - sq.lastFormationLeaderX
        dy := sq.Leader.y - sq.lastFormationLeaderY
        distMoved := math.Sqrt(dx*dx + dy*dy)
        
        if distMoved < 32 && sq.Formation == sq.lastFormation {
            return // Formation stable, no update needed
        }
        
        sq.lastFormationLeaderX = sq.Leader.x
        sq.lastFormationLeaderY = sq.Leader.y
        sq.lastFormation = sq.Formation
    }
    
    // ... existing formation calculation ...
}
```

### Fix 5: Leader Aggression Reduction
Leaders are too willing to charge across open ground. Add terrain and threat awareness:

```go
// In leader movement logic, check exposure before advancing
if s.isLeader && bb.VisibleThreatCount() > 0 {
    if s.tacticalMap != nil {
        currentCover := s.tacticalMap.CoverAt(int(s.x), int(s.y))
        targetCover := s.tacticalMap.CoverAt(int(destX), int(destY))
        
        // Don't leave good cover for open ground under fire
        if currentCover > 0.3 && targetCover < 0.1 {
            distToTarget := math.Hypot(destX-s.x, destY-s.y)
            if distToTarget > 200 { // Long open advance
                // Prefer staying in cover and directing squad
                s.state = SoldierStateIdle
                return
            }
        }
    }
}
```

## Implementation Priority

1. **Order Kind Hysteresis** (Critical) - Fixes thrashing immediately
2. **Aggressive Stall Recovery** (Critical) - Prevents indefinite paralysis
3. **Combat Intent Lock** (High) - Prevents mid-combat disruption
4. **Formation Stability** (Medium) - Reduces unnecessary recalculation
5. **Leader Aggression** (Medium) - Improves tactical behavior

## Testing Plan

1. Reproduce scenario with seed=1772401015798047700
2. Verify order thrashing stops (orders stable for 60+ ticks)
3. Verify soldiers recover from stall within 2x threshold
4. Verify intent stays Engage during combat
5. Verify formation slots stable when leader not moving significantly
6. Verify soldiers fire at visible threats
