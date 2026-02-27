# Combat System — Gunfire

## Overview

Soldiers engage visible enemies with small-arms fire. Shots are resolved as
dice rolls influenced by shooter accuracy, range, and target profile. Hits
reduce target health; at zero health a soldier is incapacitated (dead state).

Tracers are rendered as short-lived pixel lines travelling from shooter to
impact point, giving immediate visual feedback of who is shooting whom.

Incoming fire applies psychological stress — even misses. This feeds the
existing fear/morale system and will naturally push soldiers toward cover,
holding, or retreating via the goal-utility scoring that is already in place.

## Constants

| Name | Value | Notes |
|------|-------|-------|
| `fireInterval` | 30 ticks | ~0.5 s at 60 TPS. Minimum ticks between shots. |
| `maxFireRange` | 300 px | Same as vision range. |
| `baseDamage` | 25 HP | Per hit. |
| `soldierMaxHP` | 100 | Starting health. |
| `tracerLifetime` | 8 ticks | How long a tracer line persists. |
| `tracerSpeed` | 40 px/tick | Visual travel speed of the tracer head. |
| `nearMissStress` | 0.08 | Fear applied to target on a miss. |
| `hitStress` | 0.20 | Fear applied to target on a hit. |
| `witnessStress` | 0.03 | Fear applied to nearby friendlies who see the hit. |

## Hit Probability

```
hitChance = shooterAccuracy * rangeFactor * targetProfile

shooterAccuracy = EffectiveAccuracy()        // already in stats.go
rangeFactor     = 1.0 - (dist / maxFireRange) // linear falloff
targetProfile   = target.Stance.Profile().ProfileMul
```

A uniform random roll in [0,1) < hitChance → hit.

## Firing Decision

A soldier fires when **all** of the following are true:

1. State is not Dead.
2. Has at least one visible contact (`KnownContacts` non-empty).
3. Cooldown (`fireCooldown`) has elapsed.
4. Current goal is **not** GoalSurvive (panicked soldiers don't shoot).

Target selection: closest visible enemy (from KnownContacts).

## Tracer Rendering

Each shot spawns a `Tracer` struct:

```go
type Tracer struct {
    fromX, fromY float64
    toX, toY     float64
    hit          bool
    age          int       // ticks since spawn
}
```

Draw: a short bright line segment whose head travels from `from` to `to` over
`tracerLifetime` ticks. Colour is team-tinted (red team = orange-yellow,
blue team = cyan). Hits end at the target; misses scatter slightly past.

## Stress Propagation

- **Target** receives `nearMissStress` (miss) or `hitStress` (hit).
- **Nearby friendlies** (within 80 px of target) receive `witnessStress`.
- This feeds directly into `PsychState.ApplyStress()`, which raises fear,
  which feeds `EffectiveFear()`, which feeds `SelectGoal()` — no new
  plumbing needed.

## Health

New field on `Soldier`:

```go
health    float64 // starts at soldierMaxHP
```

On hit: `health -= baseDamage`. When `health <= 0`: set `state = SoldierStateDead`.

## Blackboard Integration

A new counter on the blackboard tracks incoming fire pressure:

```go
IncomingFireCount int // shots received (hit or miss) this tick — reset each tick
```

This will later be used to weight suppression and retreat decisions.

## Game Loop Integration

```
Update():
  1. SENSE  (vision)         — existing
  2. COMBAT (fire + resolve) — NEW
  3. SQUAD THINK             — existing
  4. FORMATION               — existing
  5. INDIVIDUAL THINK + ACT  — existing

Draw():
  ... existing ...
  draw tracers             — NEW (after soldiers, before UI)
```

## Future Hooks

- Suppression mechanic (volume of near-misses forces head-down).
- Ammo tracking.
- Sound propagation (gunshots heard by non-visible soldiers).
- Casualty-driven morale collapse / mission abort.
