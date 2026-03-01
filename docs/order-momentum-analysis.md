# Order Momentum & Radio Communication Analysis

## Current Order Flow Issues

### 1. **Instantaneous Order Propagation**
**Location**: `Squad.SquadThink()` → writes directly to `member.blackboard.OfficerOrder*` fields

**Problem**: Orders are issued by the leader and **instantly** appear in every squad member's blackboard in the same tick. No transmission delay, no processing time, no decision lag.

```go
// squad.go:1234-1275 - INSTANT propagation
for _, m := range sq.Members {
    m.blackboard.OfficerOrderKind = sq.ActiveOrder.Kind
    m.blackboard.OfficerOrderTargetX = sq.ActiveOrder.TargetX
    // ... all fields written immediately
}
```

### 2. **Rapid Order Changes at Squad Level**
**Locations where orders can change rapidly**:

#### A. Intent-Driven Order Changes (`syncOfficerOrder`)
Every tick, `SquadThink()` calls `syncOfficerOrder()` which issues new orders based on current intent:
- `IntentAdvance` → `CmdMoveTo` 
- `IntentHold` → `CmdHold`
- `IntentRegroup` → `CmdRegroup`
- `IntentEngage` → `CmdBoundForward`, `CmdAssault`, `CmdHold` (varies by phase)

**Issue**: Intent can change tick-to-tick, causing order churn even with our new 60-tick hysteresis.

#### B. Phase-Driven Order Changes (within IntentEngage)
When `IntentEngage` is active, orders change based on squad phase:
- `SquadPhaseFixFire` → `CmdHold` or `CmdAssault`
- `SquadPhaseBound` → `CmdBoundForward`
- `SquadPhaseAssault` → `CmdAssault`
- `SquadPhaseStalledRecovery` → `CmdMoveTo` or `CmdAssault`

**Issue**: Phase transitions trigger order changes even if intent is stable.

#### C. Stalled Member Override
```go
// squad.go:1310-1323 - Individual stalled soldier gets priority order
if stalledPriorityMember == m {
    m.blackboard.OfficerOrderKind = CmdMoveTo
    m.blackboard.OfficerOrderTargetX = stalledPriorityX
    // ... overrides squad order for this soldier
}
```

**Issue**: Individual soldiers can get different orders than the rest of the squad.

### 3. **No Order Commitment at Soldier Level**
**Location**: Goal selection happens every `NextDecisionTick` (typically 120-300 ticks)

**Problem**: While goal selection has commitment phases, **officer orders** are re-evaluated every tick through the blackboard. A soldier might:
- Tick 100: Receive `CmdBoundForward`, start moving
- Tick 101: Receive `CmdHold`, stop moving
- Tick 102: Receive `CmdBoundForward` again, restart moving

The soldier has no "I'm already executing this order" persistence.

### 4. **Goal Selection Ignores Order Momentum**
**Location**: `SelectGoalWithHysteresis()` in `blackboard.go`

Officer orders influence goal utilities but don't create commitment:
```go
// blackboard.go - officer orders add utility bias
if bb.OfficerOrderActive {
    switch bb.OfficerOrderKind {
    case CmdMoveTo:
        advanceUtil += bb.OfficerOrderStrength * 0.45
    case CmdBoundForward:
        moveToContactUtil += bb.OfficerOrderStrength * 0.55
    // ...
    }
}
```

**Issue**: Orders are just utility modifiers, not commitments. If other factors change slightly, the soldier can ignore the order.

## Rapid Order Change Scenarios

### Scenario 1: Intent Oscillation
```
Tick 1000: Intent=Engage → issues CmdBoundForward
Tick 1020: Intent=Hold (pressure spike) → issues CmdHold
Tick 1040: Intent=Engage (pressure drops) → issues CmdBoundForward
```

Even with 60-tick hysteresis, intent changes bypass it because they're considered different contexts.

### Scenario 2: Phase Thrashing
```
Tick 1000: Phase=Bound, Intent=Engage → CmdBoundForward
Tick 1050: Phase=FixFire (contact close) → CmdHold
Tick 1100: Phase=Bound (contact moves) → CmdBoundForward
```

Phase changes within the same intent trigger order changes.

### Scenario 3: Formation Recalculation
```
Tick 1000: Formation=Wedge → slot at (100, 200)
Tick 1001: Leader moves slightly → Formation recalc → slot at (102, 201)
Tick 1002: Formation=Line (intent change) → slot at (150, 200)
```

Formation slots update every tick, causing micro-adjustments.

## Existing Radio System (Phase A)

**Location**: `radio.go` - Already implemented!

The game already has a radio communication system with:
- Message transmission delays
- Garbled/dropped messages
- Anti-talk-over arbitration
- Status requests with timeouts

**Current Usage**: Only for status updates and leader succession messages.

**Not Used For**: Officer orders! Orders bypass radio entirely.

## Proposed Solution: Order Momentum System

### Phase 1: Radio-Based Order Transmission

#### A. Orders Go Through Radio System
```go
// Instead of instant blackboard write:
sq.radioSendOrder(tick, m.id, sq.ActiveOrder)

// Soldier receives via radio callback:
func (s *Soldier) onRadioOrderReceived(tick int, order OfficerOrder) {
    s.pendingOrder = order
    s.orderReceivedTick = tick
    s.orderProcessingDelay = s.calculateOrderProcessingDelay()
}
```

**Delays**:
- Transmission time: 15-45 ticks (0.25-0.75 seconds) based on distance/conditions
- Processing time: 30-90 ticks (0.5-1.5 seconds) based on stress/discipline
- **Total**: 45-135 ticks (0.75-2.25 seconds) before soldier acts on order

#### B. Order Processing Delay
```go
func (s *Soldier) calculateOrderProcessingDelay() int {
    base := 60 // 1 second base
    
    // Stress increases processing time
    fear := s.profile.Psych.EffectiveFear()
    stress := fear + s.blackboard.SuppressLevel*0.7
    stressDelay := int(stress * 60) // 0-60 ticks
    
    // Discipline reduces processing time
    discipline := s.profile.Skills.Discipline
    disciplineBonus := int(discipline * 30) // 0-30 ticks reduction
    
    // Under fire = faster reaction
    underFire := s.blackboard.IncomingFireCount > 0
    if underFire {
        base = 30 // 0.5 seconds when under fire
    }
    
    total := base + stressDelay - disciplineBonus
    if total < 15 {
        total = 15 // minimum 0.25 seconds
    }
    return total
}
```

### Phase 2: Order Commitment

#### A. Committed Order State
```go
type Soldier struct {
    // ...existing fields...
    
    committedOrder        OfficerOrder
    committedOrderTick    int
    orderCommitDuration   int // how long to stick with this order
}
```

#### B. Order Commitment Logic
```go
func (s *Soldier) processOrder(tick int) {
    // Still processing previous order
    if tick < s.orderReceivedTick + s.orderProcessingDelay {
        return
    }
    
    // Commit to the new order
    s.committedOrder = s.pendingOrder
    s.committedOrderTick = tick
    s.orderCommitDuration = s.calculateOrderCommitDuration()
    s.think(fmt.Sprintf("committed to order: %s", s.committedOrder.Kind))
}

func (s *Soldier) calculateOrderCommitDuration() int {
    base := 180 // 3 seconds minimum commitment
    
    // Higher priority orders get longer commitment
    priority := s.committedOrder.Priority
    priorityBonus := int(priority * 120) // 0-120 ticks
    
    // Discipline increases commitment
    discipline := s.profile.Skills.Discipline
    disciplineBonus := int(discipline * 60) // 0-60 ticks
    
    return base + priorityBonus + disciplineBonus
}
```

#### C. Stress-Based Re-evaluation
```go
func (s *Soldier) shouldBreakOrderCommitment(tick int) bool {
    // Haven't committed long enough
    ticksCommitted := tick - s.committedOrderTick
    if ticksCommitted < s.orderCommitDuration {
        // Only break early under extreme stress
        fear := s.profile.Psych.EffectiveFear()
        suppress := s.blackboard.SuppressLevel
        stress := fear + suppress*0.7
        
        // Need very high stress to break commitment early
        if stress < 0.75 {
            return false
        }
        
        // Even under stress, need to be committed for minimum time
        minCommit := 60 // 1 second absolute minimum
        if ticksCommitted < minCommit {
            return false
        }
        
        // Probabilistic break based on stress level
        breakChance := (stress - 0.75) * 2.0 // 0-0.5 chance
        roll := math.Abs(math.Sin(float64(tick * (s.id + 1))))
        return roll < breakChance
    }
    
    return true // Commitment period expired
}
```

### Phase 3: Intent Stability

#### A. Intent Change Hysteresis
```go
type Squad struct {
    // ...existing fields...
    
    lastIntentChangeTick int
    intentCommitDuration int
}

func (sq *Squad) canChangeIntent(tick int, newIntent SquadIntentKind) bool {
    if sq.Intent == newIntent {
        return true // Not actually changing
    }
    
    ticksSinceChange := tick - sq.lastIntentChangeTick
    minDuration := 180 // 3 seconds minimum per intent
    
    // Combat lock: don't change away from Engage during active combat
    if sq.Intent == IntentEngage {
        anyVisibleThreats := 0
        for _, m := range sq.Members {
            if m.blackboard.VisibleThreatCount() > 0 {
                anyVisibleThreats++
            }
        }
        if anyVisibleThreats > 0 && ticksSinceChange < 300 {
            // Only allow Withdraw, not other intents
            if newIntent != IntentWithdraw {
                return false
            }
        }
    }
    
    return ticksSinceChange >= minDuration
}
```

### Phase 4: Formation Stability

#### A. Formation Change Threshold
```go
type Squad struct {
    // ...existing fields...
    
    lastFormationLeaderX float64
    lastFormationLeaderY float64
    lastFormationUpdate  int
}

func (sq *Squad) shouldUpdateFormation(tick int) bool {
    if sq.Leader == nil {
        return false
    }
    
    // Time-based: don't update more than once per second
    if tick - sq.lastFormationUpdate < 60 {
        return false
    }
    
    // Distance-based: only if leader moved significantly
    dx := sq.Leader.x - sq.lastFormationLeaderX
    dy := sq.Leader.y - sq.lastFormationLeaderY
    distMoved := math.Sqrt(dx*dx + dy*dy)
    
    if distMoved < 48 { // 3 cells
        return false
    }
    
    return true
}
```

## Implementation Priority

### Critical (Immediate)
1. **Intent stability** - Prevent rapid intent changes (3-5 second minimum)
2. **Formation stability** - Only recalculate when leader moves >48 pixels
3. **Order commitment duration** - Soldiers stick to orders for 3-6 seconds

### High (Next)
4. **Radio-based order transmission** - Orders go through existing radio system
5. **Order processing delay** - Soldiers take time to process and act on orders
6. **Stress-based commitment breaking** - High stress allows early re-evaluation

### Medium (Future)
7. **Phase transition hysteresis** - Prevent rapid phase changes
8. **Goal-order alignment** - Goals should respect committed orders more strongly

## Benefits

1. **Realistic command delay**: 1-2 second lag between leader decision and soldier action
2. **Order persistence**: Soldiers commit to orders for 3-6 seconds minimum
3. **Reduced thrashing**: Intent/phase/formation changes don't cause instant chaos
4. **Stress-based flexibility**: Panicked soldiers can break commitment, calm soldiers stay focused
5. **Uses existing radio system**: Leverages already-implemented infrastructure

## Testing Plan

1. Create scenario with rapid intent changes → verify orders stable for 3+ seconds
2. Create scenario with phase transitions → verify soldiers don't instantly switch
3. Create scenario with leader movement → verify formation doesn't recalculate every tick
4. Create scenario with high stress → verify soldiers can break commitment early
5. Verify radio transmission delays work for orders (15-45 tick transmission + 30-90 tick processing)
