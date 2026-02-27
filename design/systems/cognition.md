# Soldier Cognition System

**Status:** Core Design
**Priority:** Critical — this is the foundation of all emergent behaviour

---

## Overview

Every soldier runs a **Sense → Think → Intend → Act** loop each tick. This loop produces all autonomous behaviour: movement, stance changes, engagement, retreat, compliance, refusal.

Leaders run an additional **Squad Think** step that produces orders for their members. Orders are not commands — they are requests filtered through each soldier's psychology before being acted upon.

There are no scripted behaviours. All outcomes emerge from the interaction of perception, belief, goal selection, and physical capability.

---

## 1. The Cognition Pipeline

Each game tick processes soldiers in this order:

```
┌─────────────────────────────────────────────────────────┐
│                     GAME TICK                           │
│                                                         │
│  1. SENSE          (world → perceptions)                │
│  2. BELIEVE        (perceptions → blackboard facts)     │
│  3. SQUAD THINK    (leader only: beliefs → orders)      │
│  4. INDIVIDUAL THINK (beliefs + orders → goal/task)     │
│  5. ACT            (task → world mutations)             │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

Each step has a single responsibility and a clean data boundary. No step should reach into a later step's domain.

---

## 2. Perception (Step 1: Sense)

Perceptions are **ephemeral, tick-scoped inputs** produced by sense systems. They are raw data — no interpretation, no memory. They are discarded at the end of the tick after beliefs are updated.

### 2.1 Perception Types

| Type | Source System | Data |
|---|---|---|
| **VisualContact** | Vision cone + LOS | Target soldier, position, distance, confidence |
| **SoundEvent** | Sound propagation (later) | Origin estimate, volume, type (gunshot/footstep/voice) |
| **RadioMessage** | Radio system (later) | Sender, content, signal clarity |
| **VoiceCommand** | Proximity (later) | Speaker, content, audibility |
| **SelfStatus** | Internal | Fatigue, fear, stance, ammo (later), wounds (later) |

### 2.2 Design Rules

- A soldier can only perceive what its physical senses allow.
- Perceptions degrade with distance, fatigue, stress, and environmental noise.
- **No omniscience.** A soldier cannot perceive anything outside its sensory range.

---

## 3. Beliefs / Blackboard (Step 2: Believe)

The blackboard is the soldier's **working memory** — a persistent, queryable set of structured facts. It is updated from perceptions each tick and decays over time.

### 3.1 Fact Types

| Fact | Fields | Lifetime |
|---|---|---|
| **KnownThreat** | Position, team, confidence, lastSeenTick, isVisible | Persists; confidence decays if not refreshed |
| **Suppression** | Level (0-1), direction estimate | Decays each tick without reinforcement |
| **CurrentOrder** | OrderType, target position/zone, issuer, receivedTick | Until superseded or completed |
| **SquadState** | Leader alive?, squad intent, formation type | Updated each tick from squad |
| **SelfAssessment** | EffectiveFear, fatigue, ammo status (later), wound status (later) | Recomputed each tick |

### 3.2 Confidence & Decay

Facts are not binary. A `KnownThreat` has a **confidence** value (0–1):

- **1.0** — currently visible this tick
- **0.7** — seen 1–2 seconds ago, position is an estimate
- **0.3** — heard gunfire from that direction, no visual
- **0.0** — expired / forgotten (remove from blackboard)

Confidence decays each tick. The rate depends on the soldier's experience (veterans hold mental models longer).

### 3.3 Design Rules

- Beliefs can be **wrong**. A misheard radio message creates a belief with incorrect content.
- Beliefs are **personal**. Two soldiers may have contradictory beliefs about the same situation.
- The blackboard is the **only input** to the Think step. No reaching back into raw world state.

---

## 4. Squad Think (Step 3: Leaders Only)

The squad leader runs an additional cognition step that operates at squad scope. It reads the leader's own blackboard (and later, reports from members) and produces a **SquadIntent** plus **Orders**.

### 4.1 SquadIntent

SquadIntent is a high-level posture for the squad as a whole:

| Intent | Meaning |
|---|---|
| **Advance** | Move toward objective / current patrol target |
| **Hold** | Maintain position, engage targets of opportunity |
| **Withdraw** | Pull back to a safer position |
| **Regroup** | Rally on leader's position, tighten formation |
| **Suppress** | Lay fire on a zone (later) |
| **Assault** | Aggressive close with objective (later) |

### 4.2 Leader Decision Logic

The leader evaluates (utility-scored):

- **Threat level**: how many threats on blackboard, confidence, proximity
- **Squad cohesion**: are members in formation or scattered?
- **Casualty state**: how many members are down? (later)
- **Morale trend**: is squad morale rising or falling?
- **Current order from above**: player heatmap intent (later)

Example decision rules (initial implementation):

```
IF  threats.visible > 0  AND  threat.distance < 200
    → Intent = Hold

IF  threats.visible > 2  AND  squad.morale < 0.4
    → Intent = Withdraw

IF  threats.visible == 0  AND  squad.cohesion > 0.7
    → Intent = Advance

IF  squad.cohesion < 0.4
    → Intent = Regroup
```

### 4.3 Orders

Orders are the concrete instructions that flow from intent to members:

| Order | Fields | Triggered By |
|---|---|---|
| **AdvanceTo** | Target position | Intent = Advance |
| **HoldPosition** | (current pos) | Intent = Hold |
| **WithdrawTo** | Fallback position | Intent = Withdraw |
| **RallyOn** | Leader position | Intent = Regroup |
| **SuppressZone** | Target area (later) | Intent = Suppress |

Orders are delivered to each member's blackboard as a `CurrentOrder` fact. In the current phase, delivery is instant (same tick). Later, orders will travel through voice/radio with delay and potential garbling.

### 4.4 Design Rules

- The leader is also a soldier and runs its own individual Think step after Squad Think.
- The leader does not directly control member movement. It sets orders; members decide compliance.
- If the leader is killed, the squad loses its Squad Think step until a new leader is promoted.

---

## 5. Individual Think (Step 4: Everyone)

Every soldier (including leaders) selects a **goal** based on their blackboard, then derives a **task** (plan) to achieve it.

### 5.1 Goal Selection (Utility AI)

Goals compete for activation. Each goal has a `Utility(blackboard, profile) → float64` score. The highest-utility goal wins.

| Goal | Drives | Utility Rises When... |
|---|---|---|
| **Survive** | Seek cover, break LOS, flee | Fear high, threats visible, suppressed |
| **ComplyWithOrder** | Execute current order | Order exists, discipline high, fear manageable |
| **MaintainFormation** | Path to formation slot | No threats, order is Advance, slot drift > threshold |
| **EngageTarget** | Aim and fire (later) | Threat visible, ammo available (later), fear low |
| **HelpCasualty** | Move to casualty, apply aid (later) | Bond high, casualty nearby, fear low |

### 5.2 Compliance vs Self-Preservation

This is the **core tension** of the system. The compliance check is:

```
compliance_score = discipline + morale * 0.4
                 - effective_fear * 0.6
                 - fatigue * 0.2

IF compliance_score > 0.3  → willing to follow order
IF compliance_score 0.1–0.3 → hesitant (delayed/partial execution)
IF compliance_score < 0.1  → refuse (freeze or flee)
```

A soldier with high fear and low discipline will refuse a "Hold Position" order and instead pursue `Survive`. A veteran with high composure will comply despite fear.

### 5.3 Task Derivation

Once a goal is selected, it produces a concrete task (short plan):

| Goal | → Task |
|---|---|
| Survive | `MoveTo(nearest cover)` or `ChangeStance(prone)` or `Flee(away from threat)` |
| ComplyWithOrder(AdvanceTo) | `MoveInFormation(slot target)` |
| ComplyWithOrder(Hold) | `HoldPosition` + `ScanForThreats` |
| ComplyWithOrder(Withdraw) | `MoveTo(fallback position)` |
| MaintainFormation | `MoveTo(formation slot)` |

Tasks can be **interrupted**. If a soldier is executing `MoveInFormation` and suddenly sees an enemy at close range while fear is high, the next tick's goal selection will switch to `Survive`, which will override the current task with `SeekCover`.

### 5.4 Design Rules

- Goal selection runs **every tick**. There is no "lock-in" to a goal — the world can change.
- Tasks are **short-lived intentions**, not commitments. A task can be abandoned mid-execution.
- The system should be tuned so that disciplined, experienced soldiers maintain goals longer (less "flickering" between goals).

---

## 6. Act (Step 5: Motor Output)

Actions are the only step that mutates world state. They are the physical output of the chosen task.

### 6.1 Action Types

| Action | Effect |
|---|---|
| **Steer** | Set movement target for pathfinding / direct steering |
| **ChangeStance** | Transition to standing / crouching / prone |
| **TurnToFace** | Update vision heading |
| **Fire** | (later) discharge weapon at target |
| **Speak** | (later) emit voice command / report as sound event |
| **Transmit** | (later) send radio message |

### 6.2 Design Rules

- One action per category per tick (can steer + change stance + turn in the same tick).
- Actions respect physical constraints (can't sprint while prone, can't fire while sprinting — later).
- Actions feed back into the world, which produces new perceptions next tick, closing the loop.

---

## 7. Data Flow Diagram

```
                    ┌──────────┐
                    │  WORLD   │
                    └────┬─────┘
                         │ (stimuli)
                         ▼
               ┌─────────────────┐
               │   1. SENSE      │  Vision, Sound, Radio, Self-check
               └────────┬────────┘
                        │ (perceptions)
                        ▼
               ┌─────────────────┐
               │   2. BELIEVE    │  Update blackboard facts
               └────────┬────────┘
                        │ (beliefs)
                        ▼
          ┌─────────────────────────────┐
          │   3. SQUAD THINK            │  Leader only:
          │      Evaluate squad state   │  beliefs → SquadIntent → Orders
          │      Issue orders           │  Orders written to member blackboards
          └─────────────┬───────────────┘
                        │ (beliefs + orders)
                        ▼
               ┌─────────────────┐
               │ 4. INDIVIDUAL   │  Goal selection (utility scoring)
               │    THINK        │  Task derivation
               │                 │  Compliance check
               └────────┬────────┘
                        │ (chosen task)
                        ▼
               ┌─────────────────┐
               │   5. ACT        │  Movement, stance, heading, fire (later)
               └────────┬────────┘
                        │ (world mutations)
                        ▼
                    ┌──────────┐
                    │  WORLD   │  ← loop closes
                    └──────────┘
```

---

## 8. Interaction Between Squad and Individual Loops

The squad loop and individual loop are **loosely coupled** through the blackboard:

```
LEADER                              MEMBER
──────                              ──────
Sense → own blackboard              Sense → own blackboard
         │                                   │
    Squad Think                              │
    ├─ read own beliefs                      │
    ├─ evaluate squad state                  │
    ├─ choose SquadIntent                    │
    └─ write Order to ──────────────► CurrentOrder fact
         │                                   │
    Individual Think                  Individual Think
    ├─ read own beliefs               ├─ read own beliefs
    ├─ read own order (if any)        ├─ read CurrentOrder
    ├─ goal selection                 ├─ goal selection
    │   (leader may also comply       │   (compliance check)
    │    with higher orders later)     │
    └─ task                           └─ task
         │                                   │
    Act                               Act
```

**Key property**: the leader does not bypass the member's individual Think. It influences it by writing orders, but the member always has final say based on its own psychology.

---

## 9. Rank & Authority

### 9.1 Current Model

Authority is **structural**: `Squad.Leader` pointer designates who runs Squad Think. There is no explicit rank stat.

### 9.2 Future Extension

When the command hierarchy deepens (platoon leaders commanding multiple squad leaders), rank becomes a property on `SoldierProfile`:

| Rank | Scope | Responsibilities |
|---|---|---|
| **Private** | Self only | Execute tasks, report contacts |
| **Fireteam Leader** | 3-4 soldiers | Tactical positioning, immediate fire control |
| **Squad Leader** | 6-12 soldiers | Squad intent, formation, manoeuvre |
| **Platoon Leader** | 2-4 squads (later) | Coordinate squads, relay player intent |

For now, the binary `isLeader` field is sufficient. The system is designed so that adding rank tiers later only means adding more Squad Think instances at higher scopes — the individual Think loop does not change.

### 9.3 Leader Death & Succession

When a leader is killed:

1. Squad Think stops running (no orders issued).
2. Members default to self-preservation + last received order.
3. (Later) the most experienced/highest-discipline member is auto-promoted.
4. New leader begins running Squad Think next tick.

---

## 10. Implementation Phases

### Phase 1.5 — Minimal Cognition (Prove the Loop)

- [ ] `Blackboard` struct on each soldier: `KnownThreats`, `CurrentOrder`, `SelfAssessment`
- [ ] Belief update: vision contacts → `KnownThreat` facts with confidence + decay
- [ ] Leader Squad Think: if threats visible → `Hold`; if clear → `Advance`
- [ ] Individual goal selection: `Survive` vs `ComplyWithOrder` vs `MaintainFormation`
- [ ] Task execution: `SeekCover`, `HoldPosition`, `MoveInFormation`
- [ ] Compliance check gates order execution

**Expected emergent behaviour**: squads advance in formation, halt when contact is made, individuals with low discipline/high fear break formation to seek cover independently.

### Phase 2.5 — Combat Cognition (later)

- [ ] `EngageTarget` goal + firing task
- [ ] Suppression as a belief fact
- [ ] Sound events create `KnownThreat` facts with low confidence
- [ ] Leader issues `Suppress` and `Withdraw` orders based on casualty/morale assessment

### Phase 3.5 — Communication Cognition (later)

- [ ] Orders travel via radio/voice (not instant)
- [ ] Contact reports from members to leader
- [ ] Garbled messages under stress/noise
- [ ] Shared squad blackboard populated by reports, not omniscience

---

## 11. Design Principles

1. **No omniscience.** A soldier's decisions are based only on its blackboard, never on raw world state.
2. **No guaranteed compliance.** Every order is filtered through the soldier's psychology.
3. **No permanent plans.** Goals are re-evaluated every tick. Tasks are interruptible.
4. **Loose coupling.** Squad Think writes to blackboards; Individual Think reads from blackboards. They never call each other directly.
5. **Emergent over scripted.** Tune the utility functions and stat ranges; do not add special-case if/else branches for specific scenarios.
6. **Fail gracefully.** If a leader dies, if an order is garbled, if a path is blocked — the system should degrade naturally, not crash or produce nonsensical behaviour.
