# Detection and Spotting System

**Status:** Design — not yet implemented
**Priority:** High — prerequisite for awareness traits having mechanical meaning

---

## Overview

The current vision system is binary: a soldier inside the FOV cone with an unobstructed
line of sight is instantly and perfectly detected, regardless of how they are concealed,
how fast they are moving, or how aware the observer is.

This document designs a **spotting accumulator** system that replaces binary detection with
a time-weighted confidence build-up. The result is that:

- Soldiers in cover, prone, or staying still are genuinely hard to find.
- Low-awareness soldiers cannot reliably locate enemies who are not exposing themselves.
- High-awareness soldiers spot concealed targets earlier and track them longer.
- `TacticalAwareness` and `SituationalAwareness` become meaningfully decisive in combat
  outcomes rather than marginal range bonuses.
- A soldier taking fire from a hidden enemy has to scan for them rather than
  automatically knowing their exact position.

---

## 1. What Exists Today

### 1.1 Vision pipeline

```
PerformVisionScan(ox, oy, candidates, buildings, covers)
  for each candidate:
    if in FOV cone AND HasLineOfSightWithCover → append to KnownContacts
```

`KnownContacts` is rebuilt from scratch every tick. Detection is instantaneous: if the
geometry passes, the target is fully known that tick.

### 1.2 Existing hooks that are unused for detection

| Field / System | Current use | Opportunity |
|---|---|---|
| `StanceProfile.ProfileMul` (0.3 / 0.6 / 1.0) | Draw radius only | Target concealment score |
| `ThreatFact.Confidence` (0-1) | LKP decay after LOS lost | Spotting accumulator value |
| `ThreatFact.IsVisible` | LOS-visible this tick | Confirmed-contact gate |
| `VisionState.DegradeRange` | ±20-45% range modifier | Should feed into spotting power |
| `BroadcastGunfire` | Approximate auditory cue | Awareness-gated hearing radius |

### 1.3 The core problem

`ProfileMul` is never read during detection. A prone motionless enemy in rubble is
spotted at exactly the same speed as a soldier sprinting in the open. The evolutionary
run confirmed this directly: awareness traits were minimised to their lower bounds
because they provided no meaningful detection value — only soldiers with high marksmanship
and composure dominated, because those traits actually feed into combat resolution.

---

## 2. Design Goals

1. **Spotting takes time.** Detection is a process, not an event.
2. **Concealment matters.** Stance, movement, cover, and environment all reduce visibility.
3. **Awareness is decisive.** High-awareness observer vs high-concealment target: slow but
   eventual detection. Low-awareness observer vs high-concealment target: may never spot.
4. **LKP is a real tactical state.** Losing sight of a confirmed contact gives a degrading
   last-known position, not instant amnesia.
5. **Fire requires detection.** Aimed fire requires a confirmed contact. Suppression fire
   is allowed against a last-known position.
6. **Performance is preserved.** O(n) per soldier per tick; no new spatial queries.

---

## 3. Concealment Score (Target-Side)

The concealment score `C` is a 0.0-1.0 value: how hard this target is to spot this tick.
Higher = harder to detect.

```
C = clamp(StanceBase × MovementMul × EnvironmentMul, 0.05, 0.98)
```

Hard minimum 0.05 prevents a fully-exposed sprinting soldier from being unspottable.
Hard maximum 0.98 prevents any target from being permanently undetectable at any range.

### 3.1 Stance base concealment

Derived from the existing `StanceProfile.ProfileMul`, remapped to a concealment scale:

| Stance | ProfileMul | Base Concealment |
|---|---|---|
| Standing | 1.0 | 0.10 |
| Crouching | 0.6 | 0.45 |
| Prone | 0.3 | 0.78 |

### 3.2 Movement multiplier

Motion draws the eye. Stillness aids concealment.

| Movement state | Multiplier |
|---|---|
| Sprinting / dash | 0.35 (reduces effective concealment strongly) |
| Walking / advancing | 0.70 |
| Moving crouched | 0.85 |
| Stationary (not moved this tick) | 1.15 |
| Prone crawl | 1.25 |
| Stationary prone | 1.45 |

The multiplier scales `StanceBase` — it does not replace it. A sprinting prone soldier
is still more concealed than a standing one, just less so than a stationary one.

### 3.3 Environment multiplier

| Environment | Multiplier | Condition |
|---|---|---|
| Open ground | 1.0 | Default |
| Behind `CoverRubble` | 1.15 | `IsBehindCover` returns true |
| Behind `CoverChestWall`, crouching or prone | 1.55 | `IsBehindCover` + stance check |
| Inside building (visible through window) | 2.8 | Window in LOS ray path |
| Inside building (no window in path) | — | Hard LOS blocked; invisible |

Whether cover interposes between observer and target is checked using the existing
`IsBehindCover(targetX, targetY, observerX, observerY, covers)` call.

### 3.4 Worked examples

| Scenario | C |
|---|---|
| Standing, sprinting, open ground | 0.10 × 0.35 × 1.0 = 0.035 → clamped 0.05 |
| Standing, stationary, open ground | 0.10 × 1.15 × 1.0 = 0.12 |
| Crouching, walking | 0.45 × 0.70 × 1.0 = 0.32 |
| Crouching, stationary, behind chest wall | 0.45 × 1.15 × 1.55 = 0.80 |
| Prone, stationary, open ground | 0.78 × 1.45 × 1.0 = 1.13 → clamped 0.98 |
| Prone, behind chest wall | 0.78 × 1.45 × 1.55 = 1.75 → clamped 0.98 |

---

## 4. Spotting Power (Observer-Side)

Spotting power `P` represents how quickly the observer can build detection confidence
per tick. Values above 1.0 are above-average observers.

```
P = max(0.05, Base + TacticalBonus + SituationalBonus - FatiguePenalty - StressPenalty) × ArcMul
```

### 4.1 Component values

| Component | Formula | Range |
|---|---|---|
| Base | 0.40 (constant) | 0.40 |
| TacticalBonus | `TacticalAwareness × 1.0` | 0 – 1.0 |
| SituationalBonus | `SituationalAwareness × 0.8` | 0 – 0.8 |
| FatiguePenalty | `Fatigue × 0.45` | 0 – 0.45 |
| StressPenalty | `EffectiveFear × 0.40` | 0 – 0.40 |

**Evolved baseline soldier** (`TactAware=0.20, SitAware=0.20`, negligible fatigue/fear):
`P ≈ 0.40 + 0.20 + 0.16 = 0.76`

**High-awareness soldier** (`TactAware=0.70, SitAware=0.70`):
`P ≈ 0.40 + 0.70 + 0.56 = 1.66` — more than twice the detection speed.

**Fatigued, stressed soldier** (`TactAware=0.20, Fatigue=0.7, Fear=0.6`):
`P ≈ 0.40 + 0.20 + 0.16 - 0.32 - 0.24 = 0.20` — severely degraded.

### 4.2 Arc multiplier

Targets in the observer's peripheral vision are harder to notice.

```
ArcMul = 1.0   if target is within ±30° of heading (forward arc)
       = 0.55  if target is in the outer arc (30° – 60° either side)
```

The full FOV remains 120°. Peripheral detection is not disabled — just slower.

### 4.3 Distance falloff

Concealment increases with distance.

```
D_factor = max(0.15, 1.0 - (dist / effectiveRange) × 0.85)
```

At max visual range, spotting rate is 15% of close-range rate. Combined with high
concealment, distant hidden targets may never be spotted before they move.

---

## 5. Spotting Accumulator

### 5.1 Data change to ThreatFact

```go
type ThreatFact struct {
    Source              *Soldier
    X, Y                float64
    Confidence          float64  // existing: 0-1, LKP decay when not visible
    LastTick            int
    IsVisible           bool     // existing: true = confirmed contact
    SpottingAccumulator float64  // NEW: 0-1, detection progress toward confirmation
}
```

The accumulator is per-observer-per-target because it lives inside each soldier's own
`Blackboard.Threats` slice.

### 5.2 Accumulation per tick

Each tick where a candidate is in the FOV cone and passes the LOS check, but has not
yet been confirmed:

```
delta = P × D_factor × (1.0 - C) × (1.0 / 60.0)
SpottingAccumulator = clamp01(SpottingAccumulator + delta)
```

The `(1.0/60.0)` factor normalises the formula to seconds at 60TPS so that
`SpottingPower` can be reasoned about in human terms (spotting power 1.0 against a
fully exposed target detects in approximately one second at medium range).

### 5.3 Confirmation threshold

When `SpottingAccumulator >= 0.85`:

- Target is added to `KnownContacts`
- `ThreatFact.IsVisible = true`
- `ThreatFact.Confidence = 1.0`

The threshold 0.85 (rather than 1.0) gives a small buffer for frame-rate jitter and
ensures that a near-confirmed target is not bounced in and out by minor LOS interruptions.

### 5.4 Detection time reference table

Medium range (`D_factor = 0.75`), observer at evolved baseline (`P = 0.76`):

| Target state | C | Time to detect (P=0.76) | Time to detect (P=1.66) |
|---|---|---|---|
| Standing, sprinting | 0.05 | ~0.25s | ~0.11s |
| Standing, stationary | 0.12 | ~0.90s | ~0.41s |
| Crouching, walking | 0.32 | ~2.8s | ~1.3s |
| Prone, moving | 0.78 | ~18s | ~8s |
| Prone, stationary | 0.98 | never in practice | ~95s (edge of range) |
| Prone behind chest wall | 0.98 | never in practice | never at distance |

This table explains the evolutionary result directly: with `P=0.76`, prone stationary
targets are essentially unspottable. A soldier that cannot be spotted cannot be
effectively engaged, which makes awareness-versus-concealment the determining factor
in cover-heavy engagements.

### 5.5 Accumulator decay when LOS is lost

```
decayRate = 0.015 × (1.0 - C_last)
SpottingAccumulator = max(0, SpottingAccumulator - decayRate)
```

High-concealment targets (who duck back into cover) retain most of their partial
accumulation. A sprinting soldier who breaks LOS loses their progress quickly. This
means a soldier who briefly exposes themselves before ducking will be detected faster
on the next exposure — the observer is already primed.

---

## 6. Last Known Position and Fire

### 6.1 LKP fade rate scaled by awareness

The existing confidence decay in `UpdateThreats`:

```go
t.Confidence -= age * 0.008   // ~2s full fade
```

Scaled by `TacticalAwareness`:

```go
fadeRate := 0.008 * (1.0 - profile.Skills.TacticalAwareness * 0.6)
t.Confidence -= age * fadeRate
```

High-awareness soldiers (`TactAware=0.70`) retain LKP for ~5 seconds instead of ~2.

### 6.2 Fire modes by detection state

| Detection state | Can fire? | Accuracy | Aim point |
|---|---|---|---|
| Confirmed contact (`IsVisible=true`) | Yes | Normal | Current position |
| LKP (`IsVisible=false, Confidence>0.3`) | Yes — suppression only | −40% effective accuracy | `ThreatFact.X/Y` (last seen) |
| No contact (`Confidence≤0`) | No | — | — |

LKP suppression fire uses the stored last-known position. If the enemy has moved
behind cover, rounds land where the observer last saw them — not where they are now.
This makes cover a real tactic: stepping behind a wall breaks aimed fire, forces the
enemy to fire suppression at a stale position, and buys time to reposition.

### 6.3 Combat integration change

In `ResolveCombat`, the existing check:

```go
if len(s.vision.KnownContacts) == 0 {
    resetBurstState(s)
    continue
}
```

Becomes a two-branch check:

```go
confirmedContacts := visibleConfirmedContacts(s.vision.KnownContacts)
lkpTargets := lkpThreats(s.blackboard.Threats)

if len(confirmedContacts) > 0 {
    // Normal aimed fire — existing logic unchanged.
} else if len(lkpTargets) > 0 && s.profile.Skills.Discipline > 0.3 {
    // Suppression fire at LKP — reduced accuracy, uses stored position.
} else {
    resetBurstState(s)
    continue
}
```

The discipline gate on LKP fire prevents panicking conscripts from spraying at
nothing; disciplined soldiers will lay down suppression to keep the enemy's head down.

---

## 7. Building Interior Detection

### 7.1 Current state

Buildings fully block LOS (`HasLineOfSight` returns false). Soldiers inside buildings
are completely invisible from outside regardless of awareness. This is technically
correct — buildings do block sight — but it eliminates the need for tactical clearing
and makes buildings impenetrable black boxes.

### 7.2 Window-based partial detection (Phase 2)

Buildings have `Windows []rect` stored in `HeadlessBattlefield`. The LOS system
already has `rayAABBHitT` infrastructure.

A new query `HasLineOfSightThroughWindow(ax, ay, bx, by, buildings, windows)` would:

1. Test if the ray hits a building.
2. If yes, test if the ray also passes through a window rect.
3. If yes: return `(true, windowPenalty)` where `windowPenalty = 0.35` (strong
   concealment from interior shadow and structural framing).
4. The window penalty is passed into the concealment formula as `E_mul = 2.8`
   (interior environment multiplier).

This means soldiers inside buildings can be spotted through windows, but it takes
a high-awareness observer a long time at any meaningful range. Soldiers without LOS
through a window remain invisible — they must be physically located by entering the
building or by a squad member who has window LOS.

### 7.3 Tactical implications

- A squad advancing on a building cannot immediately see defenders inside.
- A high-awareness soldier using `GoalOverwatch` near a window may eventually spot
  movement inside — especially if the defender is standing and moving.
- Defenders going prone inside a building near a window are very nearly invisible.
- This creates genuine pressure to clear buildings rather than standing off and
  trying to snipe through windows.

---

## 8. Hearing as a Detection Substitute

`BroadcastGunfire` already writes an approximate enemy position to
`bb.HeardGunfireX/Y`. This is the low-awareness soldier's only tool when taking
fire from a hidden enemy. The system should be explicit about this:

- A soldier with no visual contact but with `bb.HeardGunfire=true` has a noisy
  direction estimate only — not a position.
- They can move toward the heard gunfire (`GoalMoveToContact`) but cannot fire
  accurately at a position they have only heard, not seen.
- Hearing range is currently flat. It should be gated by `FieldCraft` — a soldier
  with high fieldcraft interprets gunfire echos better and gets a more accurate
  directional estimate (lower position jitter on the heard point).

---

## 9. Relationship to Evolutionary Findings

The regular fitness run (Gen 50, 24,000 battles) showed:

- `TacticalAwareness` minimised to 0.20 (lower bound)
- `SituationalAwareness` minimised to 0.20 (lower bound)
- Dominant strategy: max marksmanship + max composure, ignore awareness

This is exactly the behaviour predicted by the current binary detection model. If
detection is instant, there is no benefit to being more aware — you will spot the enemy
at the same moment regardless. The only trait that matters once spotted is accuracy.

After implementing this system, a re-run should show awareness and marksmanship as
co-selected traits. High marksmanship is worthless if you cannot find your target;
high awareness is wasteful if you cannot hit them once found. The evolutionary
pressure should produce balanced soldiers rather than pure marksmanship maximisers.

---

## 10. Implementation Plan

### Phase 1 — Spotting accumulator (open ground only)

Changes to: `vision.go`, `blackboard.go`

1. Add `SpottingAccumulator float64` to `ThreatFact`.
2. Rewrite `PerformVisionScan` to accumulate rather than add directly to `KnownContacts`.
3. Implement concealment score using only stance (`ProfileMul`) and movement state.
4. Implement spotting power using `TacticalAwareness`, `SituationalAwareness`, fatigue.
5. Confirm contact at threshold; update `KnownContacts` accordingly.
6. Decay accumulator when target leaves cone.
7. Run evolution with regular fitness; verify awareness traits begin to be selected.

### Phase 2 — Cover concealment

Changes to: `vision.go`

8. Add environment multiplier using `IsBehindCover` result.
9. Scale LKP fade rate by `TacticalAwareness` in `UpdateThreats`.
10. Add LKP suppression fire branch in `ResolveCombat` (discipline-gated).

### Phase 3 — Building windows

Changes to: `los.go`, `vision.go`

11. Implement `HasLineOfSightThroughWindow` using existing window rects.
12. Feed window penalty into environment multiplier.
13. Update `GoalSearch` and `GoalPeek` to actively scan windows.

### Phase 4 — Hearing integration

Changes to: `combat.go`, `soldier.go`

14. Gate `BroadcastGunfire` hearing radius by `FieldCraft`.
15. Add position jitter to heard gunfire based on inverse fieldcraft.
16. Prevent LKP fire at heard-only positions (require visual LKP for suppression).

---

## 11. Testing Strategy

Each phase should add targeted tests:

- **Accumulator correctness:** Stationary prone target takes expected ticks to detect
  at given awareness level.
- **Confirmation stability:** A nearly-confirmed target that briefly breaks LOS
  reaches confirmation faster on re-exposure (accumulator retained).
- **LKP fire:** Soldier fires at last-known position when target breaks LOS;
  rounds land at the stored position, not the target's new position.
- **Awareness comparison:** Two-soldier scenario, identical except awareness;
  high-awareness one detects a prone target before the low-awareness one fires.
- **Evolution regression:** After Phase 1, re-run 10-generation evolution; awareness
  traits should begin climbing from lower bounds.

---

## 12. Resolved Design Decisions

### 12.1 Debug spotting display

Partially-accumulated targets should appear on a developer debug overlay showing
the current `SpottingAccumulator` value as a progress arc or bar above each soldier
that is in the observer's cone but not yet confirmed. This is essential for tuning
the concealment and spotting power constants without running full evolution cycles.
The display should be toggled by a debug key and should not affect headless simulation
performance.

### 12.2 Squad contact sharing — quality degraded by stress and role

Shared contacts are permitted but are not equal to direct observation. The information
quality a soldier can transmit depends on what they can currently perceive.

**Reporting quality** is a 0-1 scalar derived from the reporting soldier's state:

| Soldier state | Report quality | Notes |
|---|---|---|
| Calm, not under fire, actively observing | 0.85 – 1.0 | Best reports; precise position |
| Engaged in firefight but not suppressed | 0.50 – 0.70 | Busy; position estimate degrades |
| Suppressed (pinned) | 0.15 – 0.30 | Can barely report direction let alone position |
| Panic retreat / surrendered | ~0.0 | No useful information |

A directly-observed confirmed contact has precise position (`ThreatFact.X/Y` is
exact). A shared contact arrives as that position plus Gaussian jitter scaled
by `(1.0 - reportQuality) × maxPositionError` (approximately 80px at low quality).

The receiving soldier does not add a shared contact to `KnownContacts` directly.
Instead, it pre-loads their `SpottingAccumulator` for that target to a value
proportional to report quality:

```
receivedAccumulator = reportQuality × 0.60
```

This means a high-quality shared contact cuts the receiver's spotting time
approximately in half without bypassing detection entirely. A low-quality report
(from a suppressed soldier) only marginally primes the receiver. This integrates
naturally with the radio communications system — message clarity degrades the
report quality further before delivery.

### 12.3 Suppression feedback loop — intentional, no floor

The feedback loop between suppression and reduced observability is intentional and
should not have a protective floor. This is the mechanical expression of the
**pin, flank, kill** doctrine:

1. **Pin:** Suppressed soldiers have reduced `P` (spotting power). They cannot reliably
   detect flanking movement even when it passes within their peripheral arc.
2. **Can't report:** Their report quality collapses (see 12.2), denying the squad leader
   useful contact information at exactly the moment it is most needed.
3. **Can't see the flank:** Low `P` under high stress means they miss the squad moving
   into position. The threat accumulates undetected.
4. **Can't move:** Attempting to reposition exposes them as a standing/running target
   with minimal concealment (`C ≈ 0.05`), making them trivially detectable and lethal
   to engage.

This loop should be allowed to play out fully. Suppression is lethal not because it
hits the pinned soldier directly, but because it blinds them to what is coming next.
The correct counter is a squad member who is **not** suppressed breaking LOS and
returning fire — which is exactly the squad-level behaviour that `GoalFallback`,
`GoalFlank`, and `BoundMover` are designed to produce.

### 12.4 First-shot detection — direction only, not position

A gunshot gives the hearing soldier a **direction of fire**, not a position.
This is a deliberate distinction from LKP (which is a known position from visual
confirmation). The difference matters for what the soldier can do with the information:

**What a gunshot provides:**
- A bearing toward the origin, jittered by `(1.0 - FieldCraft) × maxBearingError`
  (up to ±35° at very low fieldcraft, ±5° at high fieldcraft)
- No distance estimate — the shot could have come from 50px or 500px away
- All buildings within a cone (~40°) along that bearing become **suspected positions**
  and are flagged in the blackboard as `SuspectedContactDirection`

**What a gunshot does NOT provide:**
- A position to fire at — suppression fire at heard-only contacts is not permitted
- A `SpottingAccumulator` spike — the soldier must still visually confirm
- An LKP entry — `ThreatFact` requires visual confirmation before it exists

**Tactical consequence:**
The heard bearing triggers a **scan reorientation**: the soldier's vision heading
rotates toward the heard direction and `GoalMoveToContact` becomes viable using the
bearing rather than a position. `GoalSearch` targets buildings along the bearing.
This makes the first shot a tactical alarm that organises squad response (everyone
looks toward the threat direction) without giving away the shooter's exact position.

**Future — suppressive fire:**
Suppressive fire (not yet designed) will cover an angular sector rather than target
a specific point. Its purpose is to raise the effective concealment of all targets
in a sector (forcing them to go prone or stay behind cover) rather than to kill
specific soldiers. The distinction between suppressive fire and aimed fire will
require a separate fire mode and intent system.
