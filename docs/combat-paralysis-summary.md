# Combat Paralysis Bug - Summary & Fixes

## Your Questions Answered

### Q: "Do they fire at all?"
**A: No.** Despite having 3 visible threats for 300 ticks, neither R5 nor R8 fired a single shot. The debug report shows no combat events, only `mobility_stall` events.

### Q: "Squad leaders are too willing to lead from the front, they often charge enemies in open ground"
**A: Confirmed.** Leaders don't currently check terrain exposure before advancing. They'll leave good cover to cross open ground under fire. This needs a tactical map cover check before movement.

### Q: "Are vision distances much longer than long range engagement distances?"
**A: Yes.** Vision range is typically 400-600 pixels, while:
- Accurate fire range: 256 pixels
- Burst range: 320 pixels  
- Max fire range: 480 pixels
- Pot-shot range: beyond 320 pixels

Soldiers can see enemies they can't effectively engage, creating frustration.

### Q: "We need a way to make soldiers look for defensive opportunities if threatened, but be more aggressive if not threatened"
**A: This exists but is broken by the paralysis bugs.** The goal selection system already has:
- `GoalEngage` for offensive fire
- `GoalSurvive` for defensive cover-seeking
- Threat perception drives the balance

However, order thrashing and mobility stall prevent these from working properly.

## Root Causes Identified

### 1. Order Thrashing (CRITICAL)
**Problem**: Orders flip between `move_to` ↔ `bound` every 1-2 ticks, creating decision paralysis.

**Fix Applied**: Added 60-tick (1 second) minimum order duration with priority-based override:
```go
// Orders must persist unless new order is 0.15+ higher priority
if ticksSinceIssued < 60 && priorityGap < 0.15 {
    return // Keep current order
}
```

### 2. Extreme Mobility Stall (CRITICAL)
**Problem**: Soldiers hit stall=89 and stay stuck indefinitely.

**Fix Applied**: Added extreme stall recovery at 2x threshold:
```go
if s.mobilityStallTicks > extremeStallThreshold {
    // Force defensive posture - seek cover or hold and fight
    if bb.VisibleThreatCount() > 0 {
        s.seekCoverFromThreat(dt)
    } else {
        s.state = SoldierStateIdle
        s.faceNearestThreatOrContact()
    }
}
```

### 3. Mid-Combat Intent Change
**Problem**: Squad switches from `IntentEngage` to `IntentRegroup` at T=4222 during active combat.

**Status**: Not yet fixed. Needs combat intent lock to prevent disruption.

### 4. Formation Instability
**Problem**: Formation slots recalculate constantly (dslot: 23→178→181→247).

**Status**: Not yet fixed. Needs stability check before recalculation.

### 5. Leader Aggression
**Problem**: Leaders charge across open ground under fire.

**Status**: Not yet fixed. Needs terrain exposure check.

## Fixes Implemented ✓

1. **Order Kind Hysteresis** - Prevents rapid order changes
2. **Extreme Stall Recovery** - Forces action after 2x threshold

## Recommended Next Steps

### High Priority
1. **Combat Intent Lock**: Prevent intent changes during active engagement
2. **Formation Stability**: Only recalculate when leader moves >32 pixels
3. **Leader Cover Awareness**: Check terrain before leaving cover

### Medium Priority  
4. **Threat-Based Aggression Scaling**: 
   - No visible threats → aggressive advance
   - Visible threats at long range → cautious advance with cover
   - Visible threats at close range → defensive positioning
   
5. **Vision/Engagement Range Balance**:
   - Reduce vision range slightly (500→400 pixels)
   - OR increase engagement willingness at medium range
   - OR add "suppressive fire" at long range to keep enemies pinned

### Implementation Example: Leader Cover Awareness

```go
// In leader movement decision (moveCombatDash or similar)
if s.isLeader && bb.VisibleThreatCount() > 0 && s.tacticalMap != nil {
    currentCover := s.tacticalMap.CoverAt(int(s.x), int(s.y))
    targetCover := s.tacticalMap.CoverAt(int(destX), int(destY))
    distToTarget := math.Hypot(destX-s.x, destY-s.y)
    
    // Don't leave good cover for open ground on long advance
    if currentCover > 0.3 && targetCover < 0.1 && distToTarget > 200 {
        // Stay in cover and direct squad instead
        s.state = SoldierStateIdle
        s.think("holding cover - directing squad")
        return
    }
}
```

### Implementation Example: Threat-Based Aggression

```go
// In goal selection or movement logic
threatLevel := bb.VisibleThreatCount()
closestThreatDist := bb.ClosestVisibleThreatDist(s.x, s.y)

aggressionFactor := 1.0
if threatLevel == 0 {
    aggressionFactor = 1.5 // Bold when no threats
} else if closestThreatDist > accurateFireRange * 1.5 {
    aggressionFactor = 1.0 // Normal at long range
} else if closestThreatDist < accurateFireRange {
    aggressionFactor = 0.6 // Cautious at close range
}

// Apply to movement speed, bound length, cover-seeking threshold
```

## Testing Recommendations

1. **Reproduce original scenario**: seed=1772401015798047700, verify order stability
2. **Leader aggression test**: Place leader in cover with enemies in open, verify stays in cover
3. **Threat scaling test**: Vary enemy distance, verify aggression changes appropriately
4. **Formation stability test**: Leader moves slowly, verify formation doesn't thrash

## Files Modified

- `internal/game/squad.go`: Order hysteresis (lines 519-533)
- `internal/game/soldier.go`: Extreme stall recovery (lines 1790-1805)
- `docs/bugfix-soldier-combat-paralysis.md`: Full analysis
- `docs/combat-paralysis-summary.md`: This summary
