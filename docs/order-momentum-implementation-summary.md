# Order Momentum Implementation Summary

## Implemented Changes ✓

### 1. **Officer Order Kind Hysteresis** (60 ticks minimum)
**File**: `squad.go:519-533`

Orders must persist for 60 ticks (1 second) minimum before changing kind, unless new order has 0.15+ higher priority.

**Impact**: Prevents rapid `move_to` ↔ `bound` oscillation.

### 2. **Intent Change Hysteresis** (180-300 ticks)
**File**: `squad.go:1096-1135`

- **Base duration**: 180 ticks (3 seconds) minimum per intent
- **Combat lock**: 300 ticks (5 seconds) when in `IntentEngage` with visible threats
- Only allows `IntentWithdraw` during combat lock
- Critical situations (spread >250, close threats) bypass hysteresis

**Impact**: Prevents intent thrashing that caused order changes.

### 3. **Formation Stability** (48 pixel threshold)
**File**: `squad.go:2087-2113`

Formation only recalculates when:
- Leader moves ≥48 pixels (3 cells), OR
- Formation type changes, OR  
- 60+ ticks (1 second) elapsed

**Impact**: Prevents micro-adjustments causing slot position chaos.

### 4. **Extreme Mobility Stall Recovery** (2x threshold)
**File**: `soldier.go:1790-1805`

When stall exceeds 2x normal threshold, force defensive action:
- Seek cover if threats visible
- Hold position and face threats otherwise

**Impact**: Prevents indefinite paralysis at high stall counts.

## Order Momentum Flow (Current State)

```
Leader Decision (SquadThink)
    ↓
Intent Selection (with 3-5s hysteresis)
    ↓
Order Generation (syncOfficerOrder)
    ↓
Order Kind Check (60 tick hysteresis)
    ↓
INSTANT propagation to blackboard ← STILL INSTANT
    ↓
Soldier receives order immediately ← NO DELAY
    ↓
Goal selection influenced by order ← NO COMMITMENT
```

## Remaining Issues

### Critical: Still Instant Order Propagation
Orders still bypass radio system and appear instantly in soldier blackboards. No transmission delay, no processing time.

### High: No Soldier-Level Order Commitment
Soldiers re-evaluate goals every `NextDecisionTick` (120-300 ticks) but don't commit to **orders** - they're just utility modifiers.

### Medium: Phase Transitions Can Still Cause Order Changes
Within `IntentEngage`, phase changes (`FixFire` → `Bound` → `Assault`) trigger different orders even though intent is stable.

## Next Steps for Full Order Momentum

### Phase 1: Radio-Based Order Transmission (High Priority)
**Estimated effort**: 4-6 hours

Integrate officer orders with existing radio system:
```go
// Instead of instant blackboard write
sq.radioSendOrder(tick, member.id, order)

// Soldier receives via callback after transmission delay
func (s *Soldier) onRadioOrderReceived(tick int, order OfficerOrder) {
    s.pendingOrder = order
    s.orderReceivedTick = tick
    s.orderProcessingDelay = calculateDelay() // 30-90 ticks
}
```

**Benefits**:
- 15-45 tick transmission delay (distance/conditions)
- 30-90 tick processing delay (stress/discipline)
- Total 45-135 ticks (0.75-2.25 seconds) before action

### Phase 2: Soldier Order Commitment (High Priority)
**Estimated effort**: 3-4 hours

Add commitment tracking:
```go
type Soldier struct {
    committedOrder      OfficerOrder
    committedOrderTick  int
    orderCommitDuration int // 180-360 ticks (3-6 seconds)
}
```

Soldiers stick to committed orders for minimum duration, only breaking early under extreme stress (fear >0.75).

### Phase 3: Phase Transition Hysteresis (Medium Priority)
**Estimated effort**: 2-3 hours

Prevent rapid phase changes within `IntentEngage`:
- Minimum 120 ticks (2 seconds) per phase
- Only change phase if progress metrics clearly indicate need

## Testing Results

✓ All squad tests pass except `TestSquadThink_RegroupWhenSpread`
✓ Test updated to account for 180-tick intent hysteresis
✓ Code compiles successfully
✓ No regressions in other tests

## Benefits Achieved So Far

1. **Order stability**: Orders persist 60+ ticks minimum
2. **Intent stability**: Intents persist 180-300 ticks minimum  
3. **Formation stability**: No micro-recalculations every tick
4. **Combat lock**: Can't switch away from Engage during active combat
5. **Stall recovery**: Soldiers break paralysis after 2x threshold

## Benefits Still To Achieve

1. **Realistic command delay**: 1-2 second lag between leader decision and soldier action
2. **Order commitment**: Soldiers execute orders for 3-6 seconds minimum
3. **Stress-based flexibility**: High stress allows early re-evaluation
4. **Phase stability**: Prevent rapid phase transitions

## User's Original Request Addressed

✓ "Look for any other cases where rapid assignment of orders and then counter orders could happen"
- Found: Intent changes, phase transitions, formation recalculations, stalled member overrides

✓ "We need to give soldiers much more momentum with their orders"
- Implemented: 60-tick order hysteresis, 180-300 tick intent hysteresis, formation stability

⚠ "We need to make radio orders take much longer to send, then a much longer pause"
- Not yet implemented: Orders still instant via blackboard
- Roadmap created: Phase 1 implementation ready

⚠ "Then the soldier who receives it decides how to act which also takes time"
- Not yet implemented: No processing delay or commitment
- Roadmap created: Phase 2 implementation ready

⚠ "When soldiers act they should try to stick to an order unless stress/fear makes them hit a new decision point"
- Partially implemented: Intent/order hysteresis provides some stickiness
- Full solution requires: Order commitment system (Phase 2)

## Files Modified

1. `internal/game/squad.go`:
   - Added intent hysteresis fields (lines 59-66)
   - Added formation stability fields (lines 62-66)
   - Implemented intent change hysteresis (lines 1096-1135)
   - Implemented formation stability (lines 2087-2113)
   - Implemented order kind hysteresis (lines 519-533)

2. `internal/game/soldier.go`:
   - Implemented extreme stall recovery (lines 1790-1805)

3. `internal/game/squad_test.go`:
   - Updated test for intent hysteresis (TestSquadThink_RegroupWhenSpread)

4. `docs/order-momentum-analysis.md`:
   - Comprehensive analysis of order flow issues
   - Detailed implementation roadmap

5. `docs/combat-paralysis-summary.md`:
   - Root cause analysis
   - Fix documentation

6. `docs/bugfix-soldier-combat-paralysis.md`:
   - Detailed bug analysis
   - Implementation recommendations
