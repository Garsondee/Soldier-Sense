# Bug Fix: Unconscious Leader Succession

## Problem

Soldier R7 was stuck idle at position (1155,660) for 300+ ticks with:
- **No path** (continuously incrementing `no_path_streak` 815→1068)
- **Distance to leader**: 315 units (constant)
- **Distance to formation slot**: 17 units (already at slot)
- **Goal**: `move_to_contact` but unable to path
- **Intent**: `regroup` with officer order `regroup`
- **Leader R3**: **Unconscious** the entire time

## Root Cause

The squad succession system only triggered when the leader was **dead**, not when they were **unconscious**:

```go
// OLD CODE - only checked for dead leaders
func (sq *Squad) SquadThink(intel *IntelStore) {
    if sq.Leader == nil || sq.Leader.state == SoldierStateDead {
        // succession logic...
    }
}
```

This caused:
1. **Unconscious leader remained in command** - no succession triggered
2. **Regroup order active** - squad trying to regroup to unconscious leader
3. **Pathfinding failures** - leader position unreachable or blocked
4. **Soldiers stuck idle** - unable to execute regroup, unable to fight

## Solution

### 1. Trigger Succession on Unconscious Leaders

Modified `SquadThink()` to treat unconscious leaders the same as dead leaders:

```go
leaderIncapacitated := sq.Leader == nil || 
                       sq.Leader.state == SoldierStateDead || 
                       sq.Leader.state == SoldierStateUnconscious
if leaderIncapacitated {
    // succession logic...
}
```

### 2. Exclude Incapacitated Soldiers from Succession Candidates

The original code used `sq.Alive()` which includes unconscious soldiers. This meant an unconscious leader could be selected as their own successor.

Fixed by filtering candidates properly:

```go
// Find capable candidates (alive and not incapacitated)
var candidates []*Soldier
for _, m := range sq.Members {
    if m.state != SoldierStateDead && 
       m.state != SoldierStateUnconscious && 
       m.state != SoldierStateWoundedNonAmbulatory {
        candidates = append(candidates, m)
    }
}

if len(candidates) == 0 {
    // No capable members - squad is combat ineffective
    sq.Intent = IntentHold
    return
}

candidate := candidates[0]
```

## Behavior After Fix

When a leader becomes unconscious:

1. **Succession triggered immediately** - next capable soldier identified
2. **Succession delay applied** - based on candidate's fear/experience (3-10 seconds)
3. **Hold intent during succession** - squad holds position while command transfers
4. **New leader installed** - capable soldier takes command
5. **Squad resumes operations** - new leader can issue orders and make decisions

## Edge Cases Handled

### No Capable Candidates
If all squad members are dead, unconscious, or non-ambulatory wounded:
- Squad intent set to `IntentHold`
- No succession attempt
- Squad is combat ineffective

### Multiple Incapacitated Members
Succession selects first capable candidate from member list (typically next in rank order).

### Leader Recovery During Succession
Current implementation: succession completes even if original leader recovers. This is acceptable behavior - once command transfer begins, it completes to avoid confusion.

Future enhancement could cancel succession if leader recovers before delay expires.

## Testing

Created comprehensive tests in `squad_succession_test.go`:

### TestSquad_UnconsciousLeaderTriggersSuccession ✓
- Leader becomes unconscious
- Succession triggered
- Intent set to Hold during delay
- New leader installed after delay

### TestSquad_DeadLeaderTriggersSuccession ✓
- Leader killed
- Succession triggered
- New leader installed

### TestSquad_RecoveredLeaderDoesNotLoseCommand
- Documents current behavior (succession completes)
- Placeholder for future enhancement

## Files Modified

**`internal/game/squad.go`**
- Line 630: Added `SoldierStateUnconscious` check to `leaderIncapacitated`
- Lines 632-647: Filter candidates to exclude incapacitated soldiers
- Line 641-644: Handle combat ineffective squad (no capable candidates)

**`internal/game/squad_succession_test.go`** (new)
- Comprehensive succession tests

## Impact

**Before**: Squads with unconscious leaders were paralyzed - soldiers stuck trying to regroup to unreachable leader, unable to fight or maneuver.

**After**: Squads automatically transfer command when leader is incapacitated (unconscious or dead), maintaining combat effectiveness.

This fix ensures squads remain functional even when leaders are wounded and unconscious, which is critical for realistic combat simulation where leaders are often targeted and incapacitated.
