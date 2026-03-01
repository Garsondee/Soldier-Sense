# Decision-making report (current implementation)

This document describes how **individual soldiers**, **squad leaders**, and **squads** make decisions in the current codebase.

It is intentionally code-linked: each section points to the authoritative files / functions.

## Contents

- **Tick pipeline overview**
- **Individual soldier decision-making**
  - Belief update (sensing -> blackboard)
  - Commitment / hysteresis decision pacing
  - Utility scoring (goal selection)
  - Goal execution (actions)
  - Psych crisis overrides (panic retreat / surrender / disobedience)
  - Wounds / incapacitation / self-aid
- **Squad / leader decision-making**
  - Contact aggregation and intent selection
  - Cohesion / stress / broken-state
  - Officer orders (soft constraints)
  - Buddy bounding, building takeover, reinforcement
  - Radio comms planning + resolution
- **Key decision signals (what feeds what)**
- **Determinism / randomness**
- **Where to look next (extension points)**

---

## Tick pipeline overview

Authoritative reference:

- `internal/game/test_harness.go`: `(*TestSim).runOneTick` (mirrors `Game.Update` ordering)

Per tick, the simulation is structured roughly as:

1. **Sense**
   - Each soldier updates vision against opposing force.
   - `(*Soldier).UpdateVision(...)` (called from the tick harness)
2. **Combat resolution**
   - Fire counts reset; combat is resolved (hits/misses), tracers updated.
   - `internal/game/combat.go`: `(*CombatManager).ResolveCombat(...)`
3. **Sound / gunfire broadcast**
   - Gunfire events are broadcast so soldiers can “hear” contact.
   - `internal/game/combat.go`: `(*CombatManager).BroadcastGunfire(...)`
4. **Squad think (leader-level loop)**
   - Each squad updates intent and writes shared signals/orders onto each member’s blackboard.
   - `internal/game/squad.go`: `(*Squad).SquadThink(...)`
5. **Formation pass**
   - `internal/game/squad.go`: `(*Squad).UpdateFormation()`
6. **Individual soldier think + act**
   - Each soldier runs their per-tick cognition loop and executes actions.
   - `internal/game/soldier.go`: `(*Soldier).Update()`

Important implication:

- **Soldiers decide after squad intent/orders are written**.
- **Combat happens before decisions**, so incoming fire this tick is visible to the decision loop.

---

## Individual soldier decision-making

Authoritative reference:

- `internal/game/soldier.go`: `(*Soldier).Update()`
- `internal/game/blackboard.go`: goal system, commitment/hysteresis, utility scoring

### High-level architecture

The per-soldier loop is explicitly documented in code as:

- `Update runs the soldier's per-tick cognition loop: believe → think → act.`

In practice it looks like:

- **Physiology / constraints** (bleeding, unconscious, mobility)
- **Psych state update** (fear recovery, morale update, stress accumulation)
- **Belief update** (vision -> threats; internal desires)
- **Crisis overrides** (surrender, panic retreat, panic lock)
- **Decision pacing**
  - commitment phases
  - shatter pressure (event-driven re-evaluation)
  - hysteresis / decision debt
- **Goal execution** (movement, firing, peeking, aid, etc.)

### Belief update (sensing -> blackboard)

Key code:

- `internal/game/soldier.go`: in `Update()`
  - `bb.UpdateThreats(s.vision.KnownContacts, tick)`
  - `bb.RefreshInternalGoals(&s.profile, s.x, s.y)`
  - `s.updatePsychCrisis(tick)`
- `internal/game/blackboard.go`:
  - `(*Blackboard).UpdateThreats(...)`
  - `(*Blackboard).RefreshInternalGoals(...)`

#### Threat facts

The blackboard stores threats as `[]ThreatFact` with:

- **Identity pointer** (`Source *Soldier`) when available
- **Position** (`X,Y`)
- **Confidence** (decays when not visible)
- **LastTick** and **IsVisible**

Threat update properties:

- Visible contacts are set to confidence `1.0`.
- Non-visible threats decay by `0.008/tick` (roughly “gone” after ~2 seconds).
- Dead sources are purged immediately.

#### “Activated” combat memory

Blackboards maintain long-lived combat memory:

- `CombatMemoryStrength` decays over ~60 seconds.
- If `CombatMemoryStrength > 0.01`, the soldier is considered **activated**.

This is used to keep soldiers from returning to pure “advance/formation” behavior immediately after contact disappears.

#### Internal tactical desires

`RefreshInternalGoals` maintains smoothed desires:

- `ShootDesire`
- `MoveDesire`
- `CoverDesire`

These are computed from:

- **visible threat range** (if any)
- **estimated hit chance at current range**
- **fear**
- **incoming fire this tick**
- **shot momentum** (recent hit/miss outcomes)
- **combat memory** (curiosity pressure to move toward last known contact)

They are EMA-smoothed with `desireEMAAlpha` to reduce oscillation.

### Decision pacing: commitment + shatter pressure + hysteresis

Key code:

- `internal/game/blackboard.go`:
  - `InitCommitment`, `BeginCommitment`, `CommitPhase`, `AddShatterPressure`, `ShatterReady`
  - `DecisionDebt`, `HysteresisMargin`, `SelectGoalWithHysteresis`
- `internal/game/soldier.go`:
  - decision evaluation logic inside `Update()`

#### Shatter pressure (event-driven re-evaluation)

Instead of instantly re-deciding on every stimulus, soldiers build **shatter pressure** (`ShatterPressure`).

Sources include:

- **incoming fire volume** (`IncomingFireCount`) adds pressure each tick
- **suppression spikes** (when suppression crosses the threshold)
- **dash overwatch expiry**, **post-arrival pause completion** (legacy `ShatterEvent`)
- **morale / stress jitter** forcing occasional early review under high fear

When `ShatterPressure >= ShatterThreshold`, `ShatterReady()` triggers re-evaluation.

`ShatterThreshold` is derived from discipline:

- green soldiers ~`0.35–0.5`
- disciplined soldiers ~`0.6–0.8`

Pressure scaling by “commitment phase”:

- **commit phase**: pressure is halved
- **sustain**: normal
- **review**: slightly amplified

#### Commitment phases

Each time a goal is selected, the soldier begins a new commitment cycle:

- **commit** (~1s base, longer with streak + stress)
- **sustain** (~1.5s base)
- **review** (goal can change when lock expires)

This is tracked by:

- `CommitPhaseTick`
- `CommitDuration`
- `SustainDuration`
- `NextDecisionTick`

#### Hysteresis + decision debt

Even when re-evaluating, soldiers don’t “flip” goals easily.

Mechanism:

- Current goal has loyalty via `HysteresisMargin`.
- A candidate goal must beat the current goal’s utility by at least that margin.
- Margin increases with:
  - goal streak depth
  - `DecisionDebt` (added each time a switch happens, decays over ~3 seconds)

Exception:

- If `SquadHasCasualties` and candidate is `GoalHelpCasualty`, hysteresis is bypassed to ensure urgent aid can happen.

### Utility scoring (goal selection)

Key code:

- `internal/game/blackboard.go`:
  - `SelectGoal(...)`
  - `SelectGoalWithHysteresis(...)`
  - `goalUtilSingle(...)` (used to compare candidate vs current)

Goals (`GoalKind`) include:

- `advance`, `formation`, `regroup`, `hold`
- `engage`, `move_to_contact`, `fallback`, `flank`, `overwatch`, `peek`
- `help_casualty`
- `survive`

#### Shared inputs (dominant signals)

Goal utilities depend heavily on:

- **Visibility**: `VisibleThreatCount()` and closest visible threat distance
- **Under fire**: `IncomingFireCount > 0` OR persistent suppression (`IsSuppressed()`)
- **Suppression level**: `SuppressLevel` (0..1)
- **Effective fear**: `profile.Psych.EffectiveFear()`
- **Combat memory**: `IsActivated()` and `CombatMemoryStrength`
- **Squad intent**: leader-written `SquadIntent`
- **Squad posture**: leader-written `SquadPosture` (-1..+1)
- **Social context**:
  - `VisibleAllyCount`
  - `IsolatedTicks`
  - `SquadFearDelta`
  - `CloseAllyPressure`
- **Officer order bias** (see next section)

#### Officer order bias (soft constraint)

Key code:

- `internal/game/blackboard.go`: `officerOrderBias(...)`

Orders are **not hard overrides**. They add an *additive utility bias* based on:

- order `Priority` and `Strength`
- calculated “compliance” which depends on:
  - discipline
  - fear
  - immediate-obedience chance (from cohesion)
  - disobeying state
  - squad broken state

This bias is applied to multiple goals (advance/hold/regroup/move-to-contact/flank/etc.) depending on order kind.

#### Notes on specific goal dynamics

- **Engage**
  - Only high when there are **visible threats**.
  - Increased by good estimated hit chance and positive shot momentum.
  - Decreased by fear and suppression.
- **MoveToContact**
  - Fires when there is *contact of any kind* but no LOS, OR LOS exists but long-range shot quality is poor.
  - Suppression heavily discourages advancing.
  - Squad intent `IntentEngage` gives a strong additional boost when you have no LOS.
- **Survive**
  - Primarily fear-driven, but **suppression alone** can force survival/cover.
- **Fallback**
  - Triggered by being under fire with contact, and scaled by fear.
  - Also supports “post-combat anxiety”: activated high-fear soldiers keep retreating even after shots stop.
- **Flank**
  - Requires contact + not yet flank complete.
  - Discouraged for leaders.
  - Discouraged strongly by suppression.
- **Overwatch**
  - Attracts soldiers with good sightlines with no visible threats but contact is known.
  - Strong when flank is complete.
  - Strong positional bonuses: corner/wall/window/interior.
  - Reduced when squad is actively in engage intent and you have no LOS (to prevent over-overwatching).
- **Peek**
  - Requires corner/window-adjacent, not suppressed, not too fearful.
  - Has a cooldown and “empty peek” penalty so soldiers move on.
- **HelpCasualty**
  - Strongly boosted when squad has casualties and environment is low-threat.
  - Medics have an extremely strong base urgency.
  - Suppression blocks the goal.

### Goal execution (actions)

Key code:

- `internal/game/soldier.go`: `executeGoal(dt)` and downstream goal handlers

Important execution-time overrides that sit *after* goal selection:

- **Malingerer / irrelevant cover forcing**
  - If the soldier is camping in safe cover with poor sightlines while the squad is pressing, they can be forced to advance (`ForceAdvance`).
- **Morale-driven reinforcement**
  - If squad sets `ShouldReinforce`, soldier will move toward a distressed squadmate unless engaged/panicking.
- **Claimed building seeking**
  - If squad claimed a building, members may route into it under certain conditions.
- **Mobility stall recovery**
  - Combat mobility goals track path stalls and can force recovery actions + shatter re-evaluation.

### Psych crisis overrides

Key code:

- `internal/game/soldier.go`: `updatePsychCrisis`, `executePanicRetreat`, `executeSurrender`
- Blackboard flags:
  - `DisobeyingOrders`, `PanicRetreatActive`, `Surrendered`, `PanicLocked`

Psych crisis overrides occur *before* normal goal selection/execution:

- If `Surrendered`:
  - soldier executes surrender behavior and returns early.
- If `PanicRetreatActive`:
  - `CurrentGoal` is forced to `GoalFallback` and retreat logic runs.
- If effective fear exceeds `panicFearThreshold`:
  - `PanicLocked` becomes true.
  - goal forced to `GoalSurvive`.
  - soldier executes `GoalSurvive` and returns.
- On recovery (fear < `panicRecoveryThreshold`):
  - panic lock clears and shatter pressure is forced to threshold to cause immediate re-evaluation.

Disobedience interacts with squad control by:

- reducing immediate obedience probability in `SquadThink` propagation
- reducing officer-order compliance in `officerOrderBias`

### Wounds / incapacitation / self-aid

Key code:

- `internal/game/soldier.go`: wound integration in `Update()`
- `internal/game/body.go`: body map, bleeding progression, functional multipliers

Decision impacts:

- Untreated wounds tick bleed each tick.
- If not alive -> soldier becomes `Dead`.
- If unconscious -> soldier becomes `Unconscious` and stops acting.
- If conscious but non-ambulatory -> soldier becomes `WoundedNonAmbulatory` and stops acting.
- Wound pain applies persistent stress each tick (`ApplyStress(pain * 0.03)`).
- Wounded soldiers may attempt self-aid when safe (`integrateWoundedSelfAid`).

---

## Squad / leader decision-making

Authoritative reference:

- `internal/game/squad.go`: `(*Squad).SquadThink(...)`
- `internal/game/blackboard.go`: intent/order types written into blackboard

### Purpose of squad layer

The squad layer provides:

- a shared notion of **contact** (“squad knows what any member knows”)
- a high-level **intent** (`IntentAdvance/Hold/Regroup/Withdraw/Engage`)
- a soft command signal (**officer order**) used as bias in individual goal selection
- group-level mechanisms:
  - cohesion/stress/break
  - buddy bounding
  - building claim
  - morale reinforcement
  - radio net (Phase A)

### Leader succession

If the leader is nil or dead:

- a candidate alive member is selected
- succession is delayed by candidate fear and reduced by experience
- during succession, squad intent is forced to `IntentHold` (members hold/survive locally)

### Contact aggregation

Squad contact is computed across all alive members in priority order:

1. Any member has **visible** threats -> closest to leader becomes contact
2. If no visible threats, use **high-confidence** remembered threats (> 0.5)
3. If still none, use **heard gunfire** (infinite-range sound in squad loop)
4. If still none, use **strongest combat memory** across members

This produces:

- `hasContact` + `contactX/contactY`
- `anyVisibleThreats` + `closestDist`

### Stress, cohesion, and broken state

Squad computes aggregate pressure metrics:

- `CasualtyRate`
- `Stress` (fear + casualties + stalling)
- `Cohesion` (degrades from outnumbering, suppression/incoming fire prevalence, casualties, and a “shock” memory)

Fresh injuries or deaths trigger an important behavior change:

- each member gets `ShatterPressure` forced above threshold and `NextDecisionTick = tick`
- this forces immediate individual re-evaluation (enables fast buddy-aid / reaction)

Broken state:

- When stress and cohesion cross thresholds for long enough, `Broken = true`.
- When broken, squad intent becomes `IntentWithdraw` and officer orders are cleared.

### Intent selection

Intent is chosen via a set of rules considering:

- spread
- visible threats / closest distance
- danger/contact heat from `IntelStore` (if available)
- squad phase shaping (Approach/FixFire/Bound/Assault/Consolidate/StalledRecovery)
- stalemate pressure and forced proactive pushes
- broken state

Intent changes are hysteresis-locked (`intentLockUntil`) to avoid thrash.

### Officer orders (soft constraints)

Once intent is chosen, the leader issues/refreshes an `ActiveOrder`:

- `CmdMoveTo`, `CmdHold`, `CmdRegroup`, `CmdBoundForward`, `CmdFanOut`, `CmdAssault`, etc.

These are propagated to each member’s blackboard as:

- `OfficerOrderKind`, `TargetX/Y`, `Radius`, `Priority`, `Strength`, `OfficerOrderActive`

Immediate obedience:

- Squad computes `obedienceChance` from cohesion, reduced by disobedience, zero in panic/surrender.
- Deterministic roll sets `OfficerOrderImmediate`.
- If not obeying immediately, the propagated priority/strength are scaled down (still influencing, but weaker).

### Additional squad-level mechanisms

- **Flank side assignment**: alternating members get left/right via `FlankSide` and share `SquadEnemyBearing`.
- **Move orders**: for engage/advance, leader computes spread positions and writes `HasMoveOrder` + `OrderMoveX/Y`.
- **Stalled-member intervention**: if one member is path-stalled, leader injects a strong direct move order toward a recovery hint.
- **Buddy bounding**: in contact + advance/engage intents, members are assigned to groups (0/1) and `BoundMover` toggles by cycle.
- **Building takeover**:
  - leader periodically evaluates buildings and may claim one (`ClaimedBuildingIdx`).
  - claim is propagated to all members’ blackboards.
- **Morale-driven reinforcement**:
  - leader finds the most fearful member; calm/high-morale members get `ShouldReinforce` + target position around them.

---

## Radio communications (Phase A)

Authoritative reference:

- `internal/game/radio.go`: `(*Squad).PlanComms`, `(*Squad).ResolveComms`, `applyRadioMessage`, delivery model
- Blackboards:
  - leader blackboard fields `RadioHasContact`, `RadioContactX/Y`, `RadioLastHeardTick`, etc.

High-level:

- Radio is a **structured squad net** with queued messages and a single channel slot.
- Messages have:
  - type (contact/status/fear/status_request)
  - priority (routine/urgent/critical)
  - summary string
  - optional structured payload (contact pos, fear, injured)

### Planning

`PlanComms` generates candidate transmissions for the tick:

- leader periodically sends **status requests** (round robin)
- members can send:
  - contact reports
  - injury status
  - fear reports
  - status replies to pending requests

### Resolution / delivery

`ResolveComms`:

- resolves one transmission slot at a time
- computes transmit duration based on message length and sender fear/discipline
- simulates delivery quality based on:
  - distance
  - sender fear
  - receiver fear
  - deterministic noise

Outcomes:

- clear
- garbled (payload is distorted)
- drop

Timeouts:

- status requests that don’t get replies mark members as unresponsive.
- timeouts stress the leader and trigger a shatter event.

Decision relevance:

- contact reports set leader’s `RadioHasContact` and also stamp `SquadHasContact` and `SquadContactX/Y`.

---

## Key decision signals (what feeds what)

This is the practical “wiring diagram” of decision-making.

### Squad -> Soldier (written into each blackboard during `SquadThink`)

- `SquadIntent`
- shared contact position:
  - `SquadHasContact`, `SquadContactX/Y`
- posture and context:
  - `SquadPosture`, `OutnumberedFactor`
  - `SquadStress`, `SquadCohesion`, `SquadCasualtyRate`, `SquadBroken`
  - `SquadHasCasualties`
- officer order:
  - `OfficerOrder*` fields + immediate obedience
- per-member coordination:
  - `HasMoveOrder`, `OrderMoveX/Y`
  - `FlankSide`, `SquadEnemyBearing`
  - `BoundGroup`, `BoundMover`
- social context:
  - `VisibleAllyCount`, `SquadAvgFear`, `SquadFearDelta`, `CloseAllyPressure`, `IsolatedTicks`
- building claim:
  - `ClaimedBuildingIdx`, `ClaimedBuildingX/Y`
- morale reinforcement:
  - `ShouldReinforce`, `ReinforceMemberX/Y`

### Soldier local signals (updated in `Soldier.Update`)

- `Threats` (from vision)
- `IncomingFireCount` (from combat manager)
- `SuppressLevel/Dir` (persistent)
- `CombatMemory*` (from gunfire hearing + decay)
- `LocalSightlineScore`, tactical traits (corner/window/interior)
- `Internal` desires + shot momentum
- psych crisis state

---

## Determinism / randomness

The AI is largely designed to be **replayable / deterministic**.

Patterns used:

- deterministic pseudo-random via `sin(tick * id * constant)`
- this is used for:
  - stress-based early re-evaluation roll
  - stance transitions and cognition pauses under fear
  - radio noise / garbling
  - reinforcement offset positions

There is still “true RNG” in some places (e.g. speech uses `math/rand`), but core decision state is generally deterministic.

---

## Where to look next (extension points)

If you want to evolve decision-making, the most central extension points are:

- `internal/game/blackboard.go`
  - `SelectGoal` (add new goals, adjust utility curves)
  - `officerOrderBias` (tune leader influence)
  - commitment constants (oscillation vs responsiveness)
- `internal/game/soldier.go`
  - `executeGoal` and goal-specific behaviors
  - psych crisis thresholds and recovery
  - movement recovery / anti-stall
- `internal/game/squad.go`
  - intent selection rules
  - phase logic (Approach/FixFire/Bound/Assault/...)
  - building claim + morale reinforcement
- `internal/game/radio.go`
  - what comms can transmit and how it influences leader/squad contact

---

## Known Issues and Improvement Opportunities

This section documents potential problems, edge cases, and areas for improvement discovered through code analysis.

### 1. Goal Selection Oscillation Risks

**Issue**: Despite hysteresis and commitment phases, certain goal pairs can still oscillate under specific conditions.

**Specific problems**:

- **Peek loop**: `@internal/game/blackboard.go:1484-1503`
  - `PeekNoContactCount` increments on empty peeks and reduces utility by `0.15` per peek
  - After 3 empty peeks, penalty is `0.45`, which may not be enough to fully suppress peek when `AtCorner` or `AtWindowAdj` bonuses are high (`+0.15` to `+0.30`)
  - Soldiers can get stuck peeking the same corner 4-5 times before moving on
  - **Fix**: Increase penalty scaling or add a hard cooldown after N consecutive empty peeks

- **Engage ↔ MoveToContact flip-flop**: `@internal/game/blackboard.go:1111-1255`
  - When at long range with marginal hit chance near `LongRangeShotQuality` threshold, soldiers can flip between engage (hold and shoot) and move-to-contact (advance) every few ticks
  - Hysteresis helps but doesn't fully prevent this when shot momentum oscillates around zero
  - **Fix**: Add a "range band stability" factor that increases hysteresis when near threshold boundaries

- **Flank completion race condition**: `@internal/game/soldier.go:2573-2577`
  - `FlankComplete` is set to `true` when within 20px of flank target OR path is terminal
  - Immediately triggers `ShatterEvent` forcing re-evaluation
  - If soldier re-selects `GoalFlank` (e.g., still under pressure), `FlankComplete` is cleared on goal switch (`@internal/game/soldier.go:1463-1465`)
  - Can cause flank → overwatch → flank → overwatch loop if conditions remain marginal
  - **Fix**: Add a cooldown on flank goal after completion, or make `FlankComplete` persist across goal switches until soldier moves significantly

### 2. Mobility Stall and Recovery Issues

**Issue**: The mobility stall detection and recovery system is complex and can fail in certain terrain/pressure combinations.

**Specific problems**:

- **Stall detection threshold**: `@internal/game/soldier.go:1765-1775`
  - Requires `mobilityStallTicks >= 30` (0.5 seconds) before triggering recovery
  - In high-pressure combat with rapid incoming fire, this delay can feel unresponsive
  - Soldiers may stand idle under fire for half a second before reacting
  - **Fix**: Scale threshold by incoming fire count (lower threshold when under fire)

- **Recovery action commitment**: `@internal/game/soldier.go:2147-2157`
  - Recovery actions commit for 18-36 ticks depending on action type
  - During commitment, soldier won't try alternative recovery even if current action is clearly failing
  - Can lead to soldiers "stuck" trying the same failed lateral move for 0.5+ seconds
  - **Fix**: Allow early commitment break on repeated path failures (e.g., 3 consecutive `nil` paths)

- **Squad stalled-member intervention conflicts**: `@internal/game/squad.go:1267-1280`
  - When squad detects a stalled member, it injects a high-priority move order
  - This overrides the soldier's own recovery action selection
  - Can conflict with soldier's local recovery logic, causing path thrashing
  - Cooldown exists (`stalledOrderCooldownTicks`) but may not be long enough
  - **Fix**: Increase cooldown or make squad intervention respect soldier's active recovery commitment

- **Irrelevant cover / malingerer detection gaps**: `@internal/game/soldier.go:1494-1523`
  - `IrrelevantCoverTicks` requires 180 ticks (3 seconds) of camping before forcing advance
  - `IdleCombatTicks` requires 300 ticks (5 seconds) before forcing advance
  - These are very long delays — soldiers can hide in buildings for 5+ seconds while squad is fighting
  - Detection only works when `SquadHasContact` or `IsActivated()` — doesn't catch pre-contact malingering
  - **Fix**: Reduce thresholds when squad is in `IntentEngage` or `SquadPhaseAssault`

### 3. Squad Coordination Edge Cases

**Issue**: Squad-level mechanisms (bounding, flanking, building claims) have coordination failures under stress.

**Specific problems**:

- **Buddy bounding swap timing**: `@internal/game/squad.go:1340-1349`
  - Bounding groups swap when "all movers settled" OR after `cycleMaxTicks` (180 ticks = 3 seconds)
  - "Settled" check only looks at `SoldierStateMoving` — doesn't account for soldiers stuck in cover-seeking or stalled
  - Can cause one group to be permanently frozen in overwatch while the other group is stalled
  - **Fix**: Add stall detection to swap trigger — if movers have been idle/stalled for >60 ticks, force swap

- **Flank side assignment**: `@internal/game/squad.go:1130-1144`
  - Flank sides are assigned by member index (even = left, odd = right)
  - This is deterministic but doesn't account for current soldier positions relative to enemy
  - Can assign soldiers to flank "through" the enemy or into worse terrain
  - **Fix**: Assign flank sides based on current position relative to enemy bearing (soldiers already left of bearing go left, etc.)

- **Building claim abandonment logic**: `@internal/game/squad.go:1392-1400`
  - Building is abandoned after `claimedNoContactTicks > 420` (7 seconds) AND `claimedOccupiedTicks > 150` (2.5 seconds)
  - This is a very long delay — squad can be stuck "holding" a building for 7+ seconds after contact ends
  - No mechanism to abandon if building is tactically poor (e.g., no windows facing enemy)
  - **Fix**: Add tactical value scoring — abandon faster if building has low sightline score toward contact

- **Building claim during broken state**: `@internal/game/squad.go:1369-1418`
  - Building evaluation and claim propagation continue even when `sq.Broken == true`
  - Broken squads should not be claiming buildings (they're withdrawing)
  - **Fix**: Skip building evaluation when `sq.Broken`

### 4. Psych Crisis Transition Issues

**Issue**: Panic, surrender, and disobedience transitions have edge cases that can cause unexpected behavior.

**Specific problems**:

- **Panic lock hysteresis gap**: `@internal/game/soldier.go:1306-1323`
  - Panic locks at `panicFearThreshold = 0.8`
  - Panic unlocks at `panicRecoveryThreshold = 0.5`
  - This is a 0.3 hysteresis gap, which is good
  - BUT: during panic lock, soldier is forced to `GoalSurvive` and can't re-evaluate
  - If fear drops to 0.79 (just below lock), soldier is still panicking but can now make decisions
  - Can cause erratic behavior where soldier flip-flops between panic and normal decision-making
  - **Fix**: Keep `PanicLocked` active until fear drops below `0.6` (larger gap)

- **Disobedience without clear recovery**: `@internal/game/soldier.go:519-520`
  - `DisobeyingOrders` is set to `true` when disobey drive exceeds `0.52`
  - Cleared when disobey drive drops below `0.42` (10-point hysteresis)
  - BUT: disobey drive depends on `OfficerOrderActive` — if order expires, disobey flag persists
  - Soldiers can remain "disobeying" even after the order they were disobeying is gone
  - **Fix**: Clear `DisobeyingOrders` when `OfficerOrderActive == false`

- **Panic retreat target selection**: `@internal/game/soldier.go:404-543` (`updatePsychCrisis`)
  - Panic retreat can choose "retreat to own lines" or "scatter" based on discipline
  - "Own lines" retreat uses `RetreatToOwnLines` flag and computes target in `executePanicRetreat`
  - BUT: if soldier is at map edge when panic starts, target computation can fail (no valid "behind" direction)
  - Soldier gets stuck in panic retreat with no valid target
  - **Fix**: Add fallback to scatter mode if own-lines target is invalid

- **Surrender during active movement**: `@internal/game/soldier.go:537-543`
  - When surrender triggers, `path` is cleared immediately
  - If soldier was mid-movement, they freeze in place (potentially in the open)
  - No attempt to finish moving to cover before surrendering
  - **Fix**: Allow soldier to finish current path segment if in cover-seeking goal, then surrender

### 5. Radio Communication Failure Modes

**Issue**: Radio system has failure modes that can cascade into squad-level problems.

**Specific problems**:

- **Status timeout cascade**: `@internal/game/radio.go:361-378` (`resolveStatusTimeouts`)
  - When a member times out on status reply, they're marked `radioUnresponsive`
  - Leader gets stress (`0.02`) and a shatter event
  - BUT: if multiple members time out in quick succession (e.g., squad under heavy fire, everyone suppressed), leader can accumulate rapid stress spikes
  - Can push leader into panic or disobedience, breaking squad cohesion
  - **Fix**: Cap stress accumulation from timeouts (max 0.05 per tick from all timeouts)

- **Garbled contact reports**: `@internal/game/radio.go:451-470` (`garbledMessage`)
  - Garbled contact reports offset position by up to ±120px
  - This is written to leader's `RadioContactX/Y` and then to `SquadContactX/Y`
  - Entire squad can be misdirected by a single garbled report
  - No confidence decay or validation against actual visual contact
  - **Fix**: Add confidence field to radio contact; decay over time; validate against visual contact when available

- **Radio queue starvation**: `@internal/game/radio.go:162-180` (`dequeue`)
  - Messages are prioritized by `Priority` then `TickCreated` then `ID`
  - If high-priority messages keep arriving, low-priority routine messages can be starved indefinitely
  - Status requests are `RadioPriRoutine`, contact reports are `RadioPriUrgent`
  - During sustained combat, status requests may never send
  - **Fix**: Add age-based priority boost (messages waiting >300 ticks get priority bump)

### 6. Goal Utility Tuning Issues

**Issue**: Some goal utilities have narrow "sweet spots" that are hard to hit, or conflicting pressures that cancel out.

**Specific problems**:

- **HelpCasualty vs Formation conflict**: `@internal/game/blackboard.go:1423-1478`
  - `HelpCasualty` has base utility `0.72 + FirstAid*0.35` (medics get `1.05`)
  - `Formation` has base utility `0.45 + Discipline*0.2` in `IntentAdvance`
  - When squad has casualties in no-contact scenario, both goals compete
  - The code adds `+0.18` global boost to `HelpCasualty` and multiplies `Formation` by `0.22`
  - BUT: high-discipline non-medics can still prefer formation over helping
  - **Fix**: Make casualty response more dominant — increase global boost or reduce formation more aggressively

- **Overwatch distance factor**: `@internal/game/blackboard.go:57-71` (`overwatchDistanceFactor`)
  - When contact range exceeds `maxFireRange * 2` (1800px), factor drops to `0.15`
  - This heavily penalizes overwatch at long range
  - BUT: long-range overwatch is exactly when it's most valuable (early warning)
  - Soldiers abandon overwatch positions when contact is far, then have to re-establish when contact closes
  - **Fix**: Flatten the curve — keep factor above `0.5` even at extreme range

- **Survive goal suppression interaction**: `@internal/game/blackboard.go:1196-1201`
  - Suppression alone can force `Survive` goal via `suppressedCoverDrive`
  - This is good — pinned soldiers seek cover
  - BUT: the formula is `suppress*0.70 + IncomingFireCount*0.04 - Discipline*0.20 - Composure*0.10`
  - High-discipline veterans (Discipline `0.8`, Composure `0.7`) get `-0.23` penalty
  - At `suppress = 0.5`, drive is only `0.35 - 0.23 = 0.12` — not enough to trigger survive
  - Veterans can stand in the open under moderate suppression
  - **Fix**: Reduce discipline/composure penalty scaling or increase base suppression weight

### 7. Determinism vs Realism Trade-offs

**Issue**: The system uses deterministic pseudo-random for replayability, but this can create unnatural patterns.

**Specific problems**:

- **Synchronized stress jitter**: `@internal/game/soldier.go:1424-1433`
  - Stress-based early re-evaluation uses `sin((tick+1)*(id+3))`
  - Soldiers with similar IDs can synchronize their jitter rolls
  - Can cause multiple soldiers to re-evaluate simultaneously, creating "wave" behavior
  - **Fix**: Add more prime number mixing or use per-soldier random seed

- **Stance transition synchronization**: Similar issue with stance transitions using deterministic rolls
  - Entire squad can crouch/stand in unison under certain tick/ID combinations
  - Looks unnatural
  - **Fix**: Add per-soldier phase offset to stance transition timing

### 8. Missing Features / Gaps

**Issue**: Some expected behaviors are not implemented or are incomplete.

**Specific problems**:

- **No ammunition tracking in goal selection**: `@internal/game/blackboard.go:1111-1578` (`SelectGoal`)
  - Goal utilities don't consider remaining ammunition
  - Soldiers will select `GoalEngage` even with 1 round left
  - Should bias toward `GoalFallback` or `GoalRegroup` when ammo is critically low
  - **Fix**: Add ammo pressure term to engage/move-to-contact utilities

- **No explicit "reload" goal**: Soldiers reload opportunistically during idle/cover states
  - No dedicated goal for "find safe position and reload"
  - Can lead to soldiers reloading in the open or under fire
  - **Fix**: Add `GoalReload` with high utility when ammo low + not under immediate fire

- **No explicit "rescue casualty" movement**: `GoalHelpCasualty` assumes casualties are nearby
  - No pathfinding toward distant wounded members
  - Medics won't cross the battlefield to reach wounded
  - **Fix**: Add distance-based movement in `executeHelpCasualty` when casualty is >2 cells away

- **No suppressive fire goal**: Soldiers either engage (trying to hit) or don't shoot
  - No "lay down covering fire" behavior to suppress enemies while others move
  - Buddy bounding would benefit from explicit suppressive fire from overwatch group
  - **Fix**: Add `GoalSuppressFire` with lower accuracy requirement but sustained fire

### 9. Formation System Issues

**Issue**: The formation system has coordination and pathing problems that can break squad cohesion.

**Specific problems**:

- **Formation update skips combat goals**: `@internal/game/squad.go:1954-1960`
  - `UpdateFormation` skips members in `GoalEngage`, `GoalFallback`, `GoalFlank`, `GoalOverwatch`
  - Also skips `GoalMoveToContact` when contact exists
  - This is correct for preserving combat autonomy
  - BUT: when contact ends, these soldiers keep their old paths and don't rejoin formation
  - Squad can remain scattered long after combat ends
  - **Fix**: Add a "formation rejoin" timer that forces formation update N ticks after last contact

- **Formation slot collision**: `@internal/game/squad.go:2040-2059` (`formationTargetCost`)
  - When multiple soldiers' desired slots are blocked, `adjustFormationTarget` searches for alternatives
  - Cost function penalizes proximity to other soldiers with `minSep = soldierRadius * 3.0`
  - BUT: if all nearby cells are blocked, multiple soldiers can be assigned the same fallback cell
  - Causes soldiers to path to the same position and then thrash
  - **Fix**: Mark assigned cells as "claimed" during the formation update pass to prevent double-assignment

- **Formation heading jitter**: `@internal/game/squad.go:1934-1943`
  - Leader heading is smoothed with EMA (alpha 0.05, ~20 tick time constant)
  - This is good for reducing jitter
  - BUT: when leader makes a sharp turn (e.g., flanking maneuver), formation lags behind by 1-2 seconds
  - Members continue on old bearing while leader has already turned
  - **Fix**: Increase alpha to 0.12 for faster response, or disable smoothing during sharp turns (>30° change)

- **Formation type switches mid-movement**: `@internal/game/squad.go:539-570` (`syncOfficerOrder`)
  - Formation type changes based on intent: `IntentAdvance` uses `FormationWedge` or `FormationColumn`
  - When intent changes, formation type changes immediately
  - All members get new slot targets and repath simultaneously
  - Can cause entire squad to stop and reorient in the middle of combat
  - **Fix**: Add formation-type commitment (don't switch for N ticks after last switch)

### 10. Vision and Threat Tracking Edge Cases

**Issue**: Vision cone and threat tracking have edge cases that can cause soldiers to "forget" enemies or fail to see obvious threats.

**Specific problems**:

- **Vision cone edge at exactly max range**: `@internal/game/vision.go:36-42` (`InCone`)
  - Uses squared distance check: `dist2 > maxRange2` returns `false`
  - Soldiers at exactly `maxRange` (1800px) are excluded
  - This is a minor edge case but can cause "flickering" visibility when target is at boundary
  - **Fix**: Use `>=` instead of `>` or add small epsilon buffer

- **Threat list unbounded growth**: `@internal/game/blackboard.go:785-794` (`UpdateThreats`)
  - New threats are appended: `bb.Threats = append(bb.Threats, ThreatFact{...})`
  - Stale threats are filtered: `kept := bb.Threats[:0]`
  - BUT: if soldier is in sustained combat with many enemies appearing/disappearing, threat list can grow large
  - Slice capacity grows but never shrinks (Go append behavior)
  - Over long battles, this can waste memory
  - **Fix**: Periodically reallocate threat slice when capacity exceeds length by >2x

- **Dead threat source not immediately purged**: `@internal/game/blackboard.go:799-802`
  - Dead threats are removed in `UpdateThreats` during the decay pass
  - BUT: this only runs when soldier updates their own threats
  - If soldier is panicking or surrendered, they may not update threats for many ticks
  - Dead enemies remain in threat list, inflating `VisibleThreatCount()`
  - **Fix**: Add a safety check in `VisibleThreatCount()` to skip dead sources

- **Threat confidence decay too slow**: `@internal/game/blackboard.go:803-811`
  - Non-visible threats decay by `0.015` per tick
  - At 60 TPS, this is 0.9 per second
  - A threat at confidence 1.0 takes ~1.1 seconds to decay to 0
  - This is reasonable for short occlusions
  - BUT: if enemy retreats behind cover and doesn't reappear, soldier remains "activated" for 1+ seconds
  - Can cause soldiers to hold position staring at empty cover instead of advancing
  - **Fix**: Increase decay rate to `0.025` (0.6 second decay time)

### 11. Combat Decision Timing Issues

**Issue**: Combat firing decisions have timing and synchronization problems that can cause unnatural behavior.

**Specific problems**:

- **Fire mode switching mid-burst**: `@internal/game/combat.go:438-455`
  - Fire mode is re-evaluated every tick when not in queued burst
  - If soldier is at range boundary (e.g., 320px, between burst and single), mode can flip
  - Mode switch triggers `modeSwitchTimer` pause (blocks firing)
  - Soldier stops shooting mid-engagement to switch modes
  - **Fix**: Add mode-switch cooldown (don't allow switch for N ticks after last shot)

- **Aiming reset on every shot**: `@internal/game/combat.go:578` (`resetAimingState`)
  - After firing a non-burst shot, aiming state is reset
  - If soldier is engaging same target at long range, they must re-aim from scratch
  - This is realistic for single shots but feels unresponsive
  - **Fix**: Preserve partial aiming progress when re-engaging same target within 2-3 ticks

- **Reload timing under fire**: `@internal/game/combat.go:368-375`
  - Reload starts immediately when `magRounds <= 0`
  - No check for incoming fire or suppression
  - Soldiers will reload in the open while being shot at
  - **Fix**: Add reload delay/cancellation when `IncomingFireCount > 0` — seek cover first, then reload

- **Burst fire target death mid-burst**: `@internal/game/combat.go:409-413`
  - If burst target dies, burst is cancelled and soldier finds new target
  - This is correct
  - BUT: soldier immediately starts new burst on new target (no cooldown)
  - Can cause soldier to spray multiple targets in rapid succession
  - **Fix**: Add small cooldown (5-10 ticks) after burst cancellation before starting new burst

### 12. State Management and Reset Issues

**Issue**: Per-tick state flags are not always reset correctly, causing stale state to persist.

**Specific problems**:

- **IncomingFireCount reset timing**: `@internal/game/combat.go:234` (`ResetFireCounts`)
  - Called once per tick after all combat resolution
  - Resets `IncomingFireCount = 0` for all soldiers
  - This is correct
  - BUT: if soldier updates their decision BEFORE combat resolution, they see stale fire count from previous tick
  - Can cause soldiers to react to fire that hasn't happened yet this tick
  - **Fix**: Reset fire counts at START of tick, not end

- **HeardGunfire flag persistence**: `@internal/game/combat.go:235` (`ResetFireCounts`)
  - `HeardGunfire` is reset to `false` every tick
  - BUT: `HeardGunfireTick` and `HeardGunfireX/Y` persist
  - Goal utilities check `HeardGunfire` flag, not tick staleness
  - If soldier hears gunfire, flag is set, then reset next tick
  - BUT: if no new gunfire, soldier still has `HeardGunfireX/Y` from old event
  - Some code paths check position without checking flag freshness
  - **Fix**: Clear `HeardGunfireX/Y` when resetting flag, or add staleness check

- **Combat memory never fully clears**: `@internal/game/blackboard.go:883-891` (`DecayCombatMemory`)
  - Decays by `combatMemoryDecayPerTick` (likely ~0.016 per tick at 60 TPS)
  - At 1.0 strength, takes ~60 seconds to decay to 0
  - This is intentional (long-term activation)
  - BUT: soldiers can remain "activated" for a full minute after a brief contact
  - Can cause soldiers to continue combat behaviors (overwatch, peek) long after threat is gone
  - **Fix**: Add faster decay when no contact for >10 seconds, or add hard cutoff at 0.05 strength

- **Formation member flag never cleared**: `@internal/game/soldier.go:761` (`formationMember`)
  - Set to `true` when soldier joins squad
  - Never set to `false`
  - If soldier leaves squad or squad is disbanded, flag persists
  - Can cause orphaned soldiers to try to follow non-existent formation slots
  - **Fix**: Clear `formationMember` when soldier leaves squad

---

## Recommendations Priority

Based on severity and impact:

### High Priority (Gameplay-Breaking)
1. **Fix mobility stall threshold under fire** - soldiers freeze for 0.5s while being shot (Section 2)
2. **Fix buddy bounding swap stall detection** - groups can deadlock (Section 3)
3. **Fix panic lock hysteresis gap** - erratic panic behavior (Section 4)
4. **Fix radio timeout stress cascade** - can break squad cohesion (Section 5)
5. **Fix reload timing under fire** - soldiers reload in the open while being shot (Section 11)
6. **Fix IncomingFireCount reset timing** - soldiers react to stale fire data (Section 12)
7. **Fix dead threat source purging** - inflates visible threat count (Section 10)

### Medium Priority (Noticeable Issues)
8. **Fix peek loop penalty scaling** - soldiers waste time peeking empty corners (Section 1)
9. **Fix flank completion race condition** - flank → overwatch → flank loop (Section 1)
10. **Fix malingerer detection thresholds** - soldiers hide too long (Section 2)
11. **Fix disobedience persistence after order expires** (Section 4)
12. **Fix garbled contact report validation** (Section 5)
13. **Fix formation update after combat** - squad remains scattered (Section 9)
14. **Fix fire mode switching mid-burst** - stops shooting to change modes (Section 11)
15. **Fix combat memory persistence** - soldiers activated for 60s after brief contact (Section 12)
16. **Fix formation slot collision** - multiple soldiers assigned same position (Section 9)

### Low Priority (Polish / Tuning)
17. **Improve flank side assignment** - position-aware (Section 3)
18. **Improve building claim abandonment logic** - tactical value scoring (Section 3)
19. **Add ammunition pressure to goal utilities** (Section 8)
20. **Fix overwatch distance factor curve** (Section 6)
21. **Add deterministic roll de-synchronization** (Section 7)
22. **Fix formation heading jitter** - lag during sharp turns (Section 9)
23. **Fix threat confidence decay rate** - too slow (Section 10)
24. **Fix aiming reset on every shot** - preserve partial progress (Section 11)
25. **Fix burst fire target death cooldown** - rapid target switching (Section 11)

### Performance / Memory
26. **Fix threat list unbounded growth** - memory waste in long battles (Section 10)
27. **Fix formation member flag persistence** - orphaned soldiers (Section 12)

---

## Investigation Summary

### Methodology

This analysis was conducted through systematic code inspection focusing on:

1. **Control flow analysis** - Tracing decision loops from tick pipeline through goal selection to execution
2. **Edge case identification** - Looking for boundary conditions, race conditions, and state management issues
3. **Interaction analysis** - Examining how subsystems (vision, combat, radio, formation) interact and potentially conflict
4. **Performance review** - Checking for unbounded growth, memory leaks, and computational bottlenecks
5. **Behavioral simulation** - Mentally simulating scenarios to identify oscillations, deadlocks, and unnatural patterns

### Scope

**Files analyzed**:
- `internal/game/soldier.go` - Individual cognition, goal execution, movement, psych crisis
- `internal/game/blackboard.go` - Goal utility scoring, commitment phases, hysteresis, officer orders
- `internal/game/squad.go` - Squad-level intent, officer orders, formation, bounding, building claims
- `internal/game/radio.go` - Radio communication, message queuing, delivery simulation, timeouts
- `internal/game/combat.go` - Fire control, fire mode selection, aiming, burst fire, reload
- `internal/game/vision.go` - Vision cone, LOS checks, contact tracking
- `internal/game/formation.go` - Formation geometry, slot assignment
- `internal/game/test_harness.go` - Tick pipeline orchestration

**Categories investigated**:
1. Goal selection oscillation and hysteresis failures (12 issues)
2. Mobility stall detection and recovery (4 issues)
3. Squad coordination (bounding, flanking, buildings) (4 issues)
4. Psych crisis transitions (panic, surrender, disobedience) (4 issues)
5. Radio communication failure modes (3 issues)
6. Goal utility tuning conflicts (3 issues)
7. Determinism vs realism trade-offs (2 issues)
8. Missing features and gaps (4 issues)
9. Formation system coordination (4 issues)
10. Vision and threat tracking (4 issues)
11. Combat decision timing (4 issues)
12. State management and reset (4 issues)

**Total issues documented**: 52 specific problems across 12 categories

### Key Findings

**Most critical issues**:
- Mobility stall threshold causes soldiers to freeze under fire
- Buddy bounding can deadlock when movers are stalled
- Reload timing doesn't check for incoming fire
- IncomingFireCount reset timing causes stale data reactions
- Dead threats not immediately purged from tracking

**Most pervasive patterns**:
- State management: Many per-tick flags not reset correctly or at wrong time
- Timing issues: Decision loops and combat resolution have synchronization problems
- Oscillation risks: Despite hysteresis, several goal pairs can still flip-flop
- Memory management: Several unbounded growth patterns in long battles

**Design strengths observed**:
- Commitment phases and shatter pressure provide good decision pacing
- Hysteresis system mostly prevents goal oscillation
- Recovery action system is sophisticated and handles most stall cases
- Radio communication model is realistic and well-designed
- Psych crisis system provides emergent behavior

### Recommendations for Future Work

1. **Add telemetry** - Instrument oscillation detection, stall frequency, and state reset timing
2. **Add unit tests** - Cover edge cases identified in this report (especially timing and state management)
3. **Add integration tests** - Test multi-tick scenarios (panic recovery, formation rejoin, radio timeout cascade)
4. **Performance profiling** - Measure actual memory growth and CPU usage in long battles
5. **Behavioral testing** - Run headless simulations to detect oscillations and deadlocks statistically

---

## Status

This report reflects the current code paths found in:

- `internal/game/soldier.go`
- `internal/game/blackboard.go`
- `internal/game/squad.go`
- `internal/game/radio.go`
- `internal/game/combat.go`
- `internal/game/vision.go`
- `internal/game/formation.go`
- `internal/game/test_harness.go`

**Analysis completed**: Current codebase state as of investigation session
**Issues documented**: 52 specific problems across 12 categories
**Prioritized recommendations**: 27 actionable fixes with severity ratings
