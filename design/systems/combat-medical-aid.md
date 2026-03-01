# Combat Medical Aid — TCCC-Informed Casualty Response

**Status:** Planning Draft
**Priority:** High — transforms casualties from binary kill/alive into a multi-system tactical event
**Depends on:** Body Parts & Hit Locations system, Radio Communications, Squad Think, Cognition pipeline

---

## 1. Design Intent

A casualty is not a medical event. It is a **tactical crisis** that simultaneously degrades combat power, command bandwidth, morale, and information flow. This system models the full squad response to a wounded soldier, structured around the three phases of Tactical Combat Casualty Care (TCCC):

1. **Care Under Fire** — suppress the threat, stop the casualty dying in the next 30 seconds.
2. **Tactical Field Care** — structured assessment and treatment once relative safety exists.
3. **Tactical Evacuation Care** — movement to a casualty collection point or higher care.

The system must ensure:

- Treatment is constrained by the tactical situation, not available on demand.
- One casualty can remove two or three soldiers from the fight (the casualty, the buddy, the medic).
- The squad leader's decision bandwidth is consumed by casualty management.
- Self-aid, buddy-aid, and medic-aid are distinct capabilities with different speeds and limits.
- Untreated casualties deteriorate. The clock is always ticking.
- The "medic runs over and health goes up" pattern never occurs.

---

## 2. The Casualty Event — Four Simultaneous Effects

When a soldier is wounded, the simulation must update four systems in the same tick:

### 2.1 Medical State

Driven entirely by the body-parts system. The wound exists, it bleeds, pain accumulates, blood volume drops. No medical system action is needed to *start* deterioration — it happens automatically. The medical system's job is to *slow or stop* it.

### 2.2 Combat Power

Immediate effects on the squad's fighting capability:

| What happens | Combat power cost |
|---|---|
| Casualty stops firing (if non-ambulatory) | -1 rifle |
| Buddy moves to casualty for aid | -1 rifle (temporarily) |
| Medic called forward | -1 specialist (delayed) |
| Leader attention diverted | Decision quality degrades |
| Formation disrupted | Manoeuvre options reduced |
| Movement slowed if carrying | Tempo loss, bunching, signature increase |

A single casualty in a 8-person squad can reduce effective combat power by 30–50% for several minutes.

### 2.3 Command & Information

- Leader must assess: who is hit, how bad, can they move, do I abort?
- Casualty report must be transmitted (radio system) — may be delayed, garbled, or lost.
- Higher command may redirect mission based on casualty count.
- If the casualty *is* the leader or radio operator, command flow breaks.

### 2.4 Morale & Behaviour

- Witnessing a casualty applies stress to nearby soldiers (already partially modeled via `witnessStress`).
- Close bonds amplify the effect (future: buddy-pair bonding).
- Distress sounds from the casualty are a persistent stress source.
- Trained response (drills) compresses panic into action — high-discipline soldiers transition to aid faster.
- Untrained response: tunnel vision, bunching around casualty, freezing.

---

## 3. TCCC Phase Model

The squad's medical response is a **state machine** driven by tactical conditions, not a timer. Phases advance when the situation permits, not on a fixed schedule.

```
                    ┌─────────────────┐
        HIT ──────►│  CARE UNDER     │
                    │  FIRE           │
                    │                 │
                    │  • Return fire  │
                    │  • Tourniquet   │
                    │    (self only)  │
                    │  • Move if able │
                    └────────┬────────┘
                             │
                    Relative safety achieved?
                    (suppressed threat / behind cover /
                     fire superiority / broke contact)
                             │
                             ▼
                    ┌─────────────────┐
                    │  TACTICAL       │
                    │  FIELD CARE     │
                    │                 │
                    │  • MARCH assess │
                    │  • Buddy aid    │
                    │  • Medic aid    │
                    │  • Casualty     │
                    │    report       │
                    │  • Leader       │
                    │    decides next │
                    └────────┬────────┘
                             │
                    Can evacuate?
                    (route clear / carrier available /
                     CCP designated)
                             │
                             ▼
                    ┌─────────────────┐
                    │  TACTICAL       │
                    │  EVACUATION     │
                    │                 │
                    │  • Drag/carry   │
                    │  • CCP staging  │
                    │  • Continued    │
                    │    treatment    │
                    │  • Handoff      │
                    └─────────────────┘
```

### 3.1 Phase Transition Conditions

| Transition | Condition |
|---|---|
| CUF → TFC | Squad `SuppressLevel` avg < 0.25 AND no incoming fire for 30+ ticks AND casualty is behind cover (or has been dragged to cover) |
| TFC → TACEVAC | Leader decides to evacuate AND a route to CCP/rear exists AND carrier(s) assigned |
| Any → CUF (regression) | Incoming fire resumes while treating — treatment pauses, responders return fire or seek cover |

Phase regression is critical: a medic treating a casualty who takes fire must stop treating and respond to the threat. Treatment does not resume until the tactical situation permits again.

---

## 4. Care Under Fire (Phase 1)

### 4.1 Who Acts

**The casualty themselves** — and almost nobody else. Buddy-aid is extremely limited. The squad's job is to fight, not treat.

### 4.2 Actions Available

| Actor | Action | Condition | Ticks | Effect |
|---|---|---|---|---|
| **Casualty (self)** | Apply tourniquet | Conscious, ≥1 arm functional, limb wound with bleed ≥ Severe | 30 (~0.5s) | Stops bleed on one limb wound |
| **Casualty (self)** | Crawl to cover | Ambulatory or crawl-capable | Movement | Reduces exposure; enables TFC transition |
| **Casualty (self)** | Return fire | Ambulatory, weapon functional, target visible | Normal fire cycle | Maintains some combat contribution |
| **Nearest buddy** | Shout encouragement / direction | Within voice range | 0 (speech) | Reduces casualty fear by 0.05; orients them |
| **Nearest buddy** | Quick drag | Adjacent, casualty non-ambulatory, brief pause in fire | 20 | Moves casualty ~15px toward cover |
| **Squad** | Return fire / suppress | Standard combat | Normal | Winning the fight is the best medicine |

### 4.3 What Does NOT Happen in CUF

- No medic call-forward.
- No wound assessment beyond "I'm hit" / "he's down".
- No bandaging, no airway management, no detailed treatment.
- No casualty movement by multiple bearers.

### 4.4 Self-Aid Detail

Self-aid in CUF is limited to **massive hemorrhage control** — the M in MARCH. The casualty applies a tourniquet to the most dangerous limb bleed. This is modeled as:

```go
type TreatmentAction int

const (
    TreatApplyTourniquet TreatmentAction = iota
    TreatPressureDressing
    TreatAirwayBasic
    TreatChestSeal
    TreatPackWound
)

type TreatmentAttempt struct {
    Action      TreatmentAction
    TargetWound *Wound           // which wound is being addressed
    Provider    *Soldier         // who is performing the treatment
    TicksLeft   int              // countdown to completion
    Interrupted bool             // set true if fire forces pause
    SkillLevel  float64          // provider's FirstAid skill
}
```

Treatment success is not guaranteed:
```
successChance = provider.Skills.FirstAid * (1.0 - totalPain * 0.3) * (1.0 - fear * 0.2)
```

A panicked, badly hurt soldier fumbling a tourniquet on themselves under fire may fail. On failure, they can retry next tick (the attempt doesn't waste the tourniquet, it just doesn't seat properly).

---

## 5. Tactical Field Care (Phase 2)

### 5.1 Entry Conditions

The squad (or at least the casualty's immediate area) has achieved relative safety:
- Average suppression level around the casualty < 0.25.
- No incoming fire directed at the casualty for 30+ ticks.
- Casualty is behind cover or in a position where treatment can occur.

### 5.2 MARCH Assessment

When a provider (buddy or medic) reaches the casualty and begins TFC, they perform a structured assessment. This is modeled as a **sequential check** that takes real ticks:

| Step | Check | Ticks | What it finds | Treatment |
|---|---|---|---|---|
| **M** — Massive hemorrhage | Scan all wounds for bleed ≥ Severe | 10 | Worst bleeder identified | Tourniquet or pack wound |
| **A** — Airway | Check neck region HP, consciousness | 8 | Airway compromised? | Basic airway manoeuvre |
| **R** — Respiration | Check torso wounds for chest involvement | 8 | Chest seal needed? | Apply chest seal |
| **C** — Circulation | Estimate blood volume from wound count + consciousness | 10 | Shock severity | IV access (medic only), pressure control |
| **H** — Head injury / Hypothermia | Check head region, blood volume, exposure time | 6 | Cognitive state, cold risk | Monitor, position, insulate |

Total assessment time: ~42 ticks (~0.7 seconds). A trained provider (high FirstAid) moves faster:

```
assessTicks = baseTicks * (1.2 - provider.Skills.FirstAid)
```

A medic (FirstAid ~0.9) completes assessment in ~60% of base time. A regular soldier (FirstAid ~0.3) takes ~90% of base time and may miss findings.

### 5.3 Treatment Actions

After assessment, the provider treats in MARCH priority order:

| Treatment | Provider | Base Ticks | Effect on Wound |
|---|---|---|---|
| **Tourniquet** (limb) | Any | 30 | BleedRate → 0 for that wound |
| **Pressure dressing** | Any | 60 | BleedRate × 0.2 |
| **Wound packing** | Medic preferred | 90 | BleedRate × 0.1 (deep wounds) |
| **Chest seal** | Trained+ | 45 | Stops torso bleed escalation; prevents respiratory failure |
| **Basic airway** | Any (trained) | 20 | Prevents airway death from neck/head injury |
| **IV / fluid resuscitation** | Medic only | 120 | Blood volume recovery: +0.005/tick for 200 ticks |

Treatment tick counts are modified by:
- Provider skill: `ticks *= (1.3 - provider.Skills.FirstAid * 0.5)`
- Stress: `ticks *= (1.0 + provider.Psych.EffectiveFear() * 0.4)`
- Light conditions: future hook for night/smoke penalties.

### 5.4 Buddy Aid vs Medic Aid

| Capability | Regular Soldier | Medic |
|---|---|---|
| Tourniquet | Yes | Yes |
| Pressure dressing | Yes (slower) | Yes |
| Wound packing | Unreliable (50% effective) | Yes |
| Chest seal | If trained (FirstAid > 0.5) | Yes |
| Airway management | Basic only | Advanced |
| IV access | No | Yes |
| Drug administration | No | Yes (future) |
| Assessment speed | Slower, may miss findings | Fast, thorough |
| Concurrent treatment | One wound at a time | Can prioritize better, still one at a time |

The key difference: a buddy can keep someone alive for a few minutes. A medic can stabilize them for evacuation.

### 5.5 Provider Assignment

When a casualty needs aid, the system determines who provides it:

**Priority order:**
1. **Self-aid** — if the casualty can treat themselves (see body-parts §5.3).
2. **Adjacent buddy** — the nearest squad member who is not currently engaged in a higher-priority task.
3. **Medic** — called forward if available and reachable.

A soldier becomes a "provider" by having the `GoalHelpCasualty` win their goal-utility competition. This means:

- A soldier under heavy fire will NOT stop to help (Survive > HelpCasualty).
- A soldier with nothing to shoot at and a bleeding buddy nearby WILL help.
- The leader can issue an order to render aid, boosting HelpCasualty utility.

```
helpCasualtyUtility =
    baseCasualtyUrgency * 0.40          // how fast is the casualty dying
  + bondStrength * 0.15                 // future: buddy-pair affinity
  + provider.Skills.FirstAid * 0.10    // skilled soldiers feel more compelled
  + orderBoost * 0.25                   // leader ordered aid
  - provider.Psych.EffectiveFear() * 0.35
  - (engageUtility * 0.20)             // active targets reduce willingness
```

### 5.6 One Casualty, Multiple Responders

Realistically, one buddy starts aid while the medic moves forward. The system allows at most **two providers** on one casualty simultaneously:

- Provider 1 (buddy): handles the first MARCH step (usually tourniquet).
- Provider 2 (medic): takes over assessment and higher-capability treatment on arrival.

When the medic arrives and begins treating, the buddy can be released back to fighting. The leader (or the buddy's own goal selection) decides when.

---

## 6. Tactical Evacuation Care (Phase 3)

### 6.1 Decision to Evacuate

The squad leader decides to evacuate when:
- The casualty is stabilized (bleeding controlled) but cannot fight.
- The mission is continuing and the casualty is a burden.
- The casualty is critical and needs higher care than the medic can provide.
- The squad is withdrawing and must take the casualty.

This is a **leader decision**, modeled in Squad Think:

```
evacuateUrgency =
    casualtyDeteriorationRate * 0.30
  + (1.0 - squadCombatPower) * 0.20   // weaker squad more likely to pull back
  + missionAbortPressure * 0.25
  + casualtiesNonAmbulatory * 0.15
  - activeThreatLevel * 0.30           // can't evacuate under fire
```

### 6.2 Casualty Movement

Moving a non-ambulatory casualty requires bearers:

| Method | Bearers | Speed Multiplier | Combat capability of bearers |
|---|---|---|---|
| **Assist-walk** | 1 | 0.50 | Bearer can fire sidearm (future) |
| **Fireman carry** | 1 | 0.30 | Bearer cannot fire |
| **Two-man drag** | 2 | 0.25 | Neither can fire |
| **Litter carry** | 2–4 | 0.40 | None can fire; requires litter (future equipment) |

Bearer assignment consumes combat power:
```
effectiveSquadSize -= bearerCount  // for combat power calculations
```

### 6.3 Casualty Collection Point (CCP)

If the squad establishes a hold position, the leader may designate a CCP — a covered location behind the squad's position where casualties are consolidated. This is modeled as:

```go
type CasualtyCollectionPoint struct {
    X, Y       float64
    Designated bool
    CoverScore float64      // how well-covered is this point
    Casualties []*Soldier   // soldiers staged here
}
```

The CCP is chosen by the leader based on:
- Distance behind the squad's front.
- Cover quality.
- Route accessibility.

### 6.4 Continued Treatment During Evacuation

Treatment actions can continue during movement (at reduced effectiveness):
- Bleed control maintained: wounds already treated stay treated.
- New treatment actions take 1.5× longer while moving.
- Medic accompanying the carry group can monitor and intervene.

### 6.5 Handoff

When a casualty reaches the CCP or is handed to a higher echelon (outside sim scope for now), the squad's obligation ends. The casualty is removed from the squad's active roster but the combat power cost has already been paid.

---

## 7. The Leader's Casualty Decision Tree

The squad leader's Squad Think gains a **casualty management branch** that runs whenever `squad.CasualtyCount > 0`:

```
CASUALTY EVENT DETECTED
│
├─ Is casualty ambulatory?
│   ├─ YES → Tell casualty to self-aid and continue fighting
│   │        Monitor for deterioration
│   │        No immediate squad response needed
│   │
│   └─ NO → How many non-ambulatory?
│       ├─ 1 casualty
│       │   ├─ Under fire? → Stay in CUF; suppress threat; casualty self-aids if able
│       │   └─ Relative safety? → Assign 1 buddy for TFC; medic forward if available
│       │       ├─ Mission viable? → Continue with reduced strength
│       │       └─ Mission compromised? → Request support / prepare to evacuate
│       │
│       ├─ 2+ casualties
│       │   ├─ Squad combat power < 50%? → Consider breaking contact
│       │   ├─ Medic overwhelmed? → Triage: treat most saveable first
│       │   └─ Leader hit? → Succession; new leader inherits casualty state
│       │
│       └─ Mass casualty (>50% squad)
│           └─ Emergency: break contact, request immediate support, survival mode
│
├─ Key role hit?
│   ├─ Leader → Trigger succession protocol (existing system)
│   ├─ Machine gunner → Reassign weapon if possible; fire superiority degraded
│   ├─ Radio operator → Comms degraded; leader may need to self-operate radio
│   └─ Medic → No advanced treatment available; buddy-aid only
│
└─ Report upward
    ├─ Casualty count and severity → radio to higher (9-liner format, simplified)
    ├─ Mission status → can/cannot continue
    └─ Evacuation request if needed
```

---

## 8. Triage

When multiple casualties exist simultaneously, the medic (or treating soldier) must triage:

| Category | Condition | Priority | Treatment |
|---|---|---|---|
| **Immediate** | Saveable with quick intervention; critical bleed, airway | Treat first | Full MARCH |
| **Delayed** | Stable enough to wait; ambulatory wounded, controlled bleeds | Treat second | Monitor, reassess |
| **Expectant** | Injuries incompatible with survival given available resources | Treat last / comfort only | Pain management (future) |
| **Dead** | No signs of life | Do not treat | — |

Triage is performed by the treating soldier's assessment. A medic triages more accurately (correctly identifies saveable vs expectant). A low-FirstAid soldier may waste time on an expectant casualty or neglect an immediate one.

```
triageAccuracy = provider.Skills.FirstAid * 0.6 + provider.Psych.Experience * 0.3
// + 0.1 baseline recognition

// Misclassification chance:
misclassifyChance = (1.0 - triageAccuracy) * 0.4
```

---

## 9. The Time Budget — What Happens When

A typical single-casualty scenario in a trained squad:

| Time (ticks) | Real time | What is happening |
|---|---|---|
| 0 | 0s | **Hit.** Casualty shouts / falls. Squad perceives casualty event. |
| 1–5 | 0–0.08s | **Shock.** Witness stress applied. Leader registers casualty on blackboard. |
| 5–30 | 0.08–0.5s | **CUF.** Squad returns fire. Casualty applies self-tourniquet if able. Leader shouts orders. |
| 30–60 | 0.5–1.0s | **Assessment.** Leader determines: ambulatory? Can we suppress? Buddy available? |
| 60–120 | 1.0–2.0s | **Transition.** If suppression achieved: buddy moves to casualty. Medic called forward. |
| 120–240 | 2.0–4.0s | **TFC begins.** Buddy starts MARCH. Medic en route. Leader manages squad posture. |
| 240–480 | 4.0–8.0s | **Treatment.** Major bleeds controlled. Assessment complete. Casualty stabilizing or not. |
| 480–720 | 8.0–12.0s | **Decision.** Leader decides: continue mission, hold for evac, or break contact. |
| 720+ | 12.0s+ | **Evacuation** if needed. Bearers assigned. Movement to CCP. Squad resumes or withdraws. |

In an untrained or broken squad, these timings stretch dramatically. Panic, hesitation, no self-aid, medic not called, leader frozen — every failure point extends the bleed clock.

---

## 10. Interactions with Existing Systems

### 10.1 Cognition Pipeline

New perception: `CasualtyEvent` — visual detection that a friendly is hit.

New belief: `KnownCasualty` on blackboard — who is down, where, estimated severity, ambulatory status.

New goal: `GoalHelpCasualty` activated with utility function (§5.5).

New goal modifier: `GoalEngage` utility boosted when squad has casualty (suppress to enable treatment).

### 10.2 Squad Think

`SquadThink` gains the casualty decision tree (§7). New squad intents:

| Intent | Meaning |
|---|---|
| **CasualtyResponse** | Squad is managing a casualty; reduced aggression, buddy assigned |
| **BreakContact** | Squad withdrawing due to casualties / mission abort |

### 10.3 Radio Communications

New message types:

| Type | Content | Priority |
|---|---|---|
| `RadioMsgCasualtyReport` | Who, where, severity, ambulatory status | Urgent |
| `RadioMsgMedicRequest` | Casualty needs medic; location | Urgent |
| `RadioMsgCasualtyUpdate` | Treatment status, deterioration/stabilization | Routine |
| `RadioMsgEvacRequest` | Need evacuation; casualty count, grid, urgency | Flash |

### 10.4 Morale / Psych

- Casualty event is a stress multiplier for the whole squad (extends existing `witnessStress`).
- Ongoing distress sounds from non-ambulatory casualty: +0.01 fear/tick to soldiers within 80px.
- Successful treatment (bleed stopped) provides small morale recovery to nearby soldiers.
- Casualty of a leader applies the existing officer-down succession stress.

### 10.5 Body Parts System

This system is the primary *consumer* of the body-parts wound model. It reads wound data, determines treatment priority, and modifies wound state (BleedRate, Treated flag).

---

## 11. Data Structures Summary

```go
// CasualtyState tracks the medical-response status for one wounded soldier.
type CasualtyState struct {
    Phase          TCCCPhase       // CUF, TFC, TACEVAC
    PhaseTick      int             // tick when current phase began
    SelfAidActive  bool            // casualty is treating themselves
    CurrentTreat   *TreatmentAttempt
    Providers      []*Soldier      // soldiers currently rendering aid (max 2)
    TriageCategory TriageCategory
    Evacuating     bool
    Bearers        []*Soldier      // soldiers carrying this casualty
    CCPTarget      *CasualtyCollectionPoint
    ReportSent     bool            // casualty report transmitted to leader
    StabilizedTick int             // tick when all critical bleeds controlled (0 = not yet)
}

type TCCCPhase int

const (
    PhaseCUF     TCCCPhase = iota // Care Under Fire
    PhaseTFC                       // Tactical Field Care
    PhaseTACEVAC                   // Tactical Evacuation Care
)

type TriageCategory int

const (
    TriageImmediate TriageCategory = iota
    TriageDelayed
    TriageExpectant
    TriageDead
)

// MedicalManager runs the per-tick medical resolution for all casualties.
type MedicalManager struct {
    Casualties []*Soldier                // all soldiers with active wounds
    CCPs       []CasualtyCollectionPoint // designated collection points
}

// ResolveMedical is called each tick after combat resolution.
// It advances bleeding, treatment, phase transitions, and evacuation.
func (mm *MedicalManager) ResolveMedical(tick int, squads []*Squad)
```

---

## 12. Implementation Phases

### Phase A — Self-Aid Under Fire

- [ ] Add `CasualtyState` to Soldier.
- [ ] On wound creation (from body-parts system), initialize CasualtyState in CUF phase.
- [ ] Implement self-tourniquet action: conscious + arm functional + limb bleed → reduce BleedRate.
- [ ] Treatment attempt with skill/stress modifiers and failure chance.
- [ ] Tests: wounded soldier with good FirstAid applies tourniquet within expected ticks; wounded soldier with both arms disabled cannot self-aid.

### Phase B — Buddy Aid & TFC Transition

- [ ] Phase transition logic: CUF → TFC based on suppression / cover conditions.
- [ ] `GoalHelpCasualty` utility function in goal selection.
- [ ] Buddy-aid behavior: move to casualty, begin MARCH assessment, treat in priority order.
- [ ] Provider assignment: nearest non-engaged buddy with adequate FirstAid.
- [ ] Treatment interruption on incoming fire (phase regression to CUF).
- [ ] Tests: buddy moves to casualty when safe; buddy stops treatment when fired upon; treatment resumes when safe.

### Phase C — Medic & Advanced Care

- [ ] Medic role concept on soldier (flag or skill threshold, e.g., FirstAid > 0.8).
- [ ] Medic call-forward behavior: leader orders medic to casualty via radio.
- [ ] Medic-only treatments: IV access, wound packing (reliable), advanced airway.
- [ ] Handoff: buddy releases back to fighting when medic arrives.
- [ ] Triage logic for multiple casualties.
- [ ] Tests: medic prioritizes immediate over delayed; medic treats faster than buddy; triage accuracy scales with skill.

### Phase D — Evacuation & Casualty Movement

- [ ] Leader evacuation decision in Squad Think.
- [ ] Bearer assignment and movement (drag, carry, assist-walk).
- [ ] Speed penalties for bearers.
- [ ] CCP designation by leader.
- [ ] Combat power reduction accounting.
- [ ] Tests: bearer pair moves casualty at expected speed; squad combat power correctly reduced.

### Phase E — Integration & Reporting

- [ ] Casualty radio messages (report, medic request, update, evac request).
- [ ] Blackboard: `KnownCasualty` beliefs for squad awareness.
- [ ] Leader casualty decision tree in Squad Think.
- [ ] Morale effects: distress sounds, treatment success recovery.
- [ ] Visual feedback: treatment indicators, bearer movement, CCP markers.
- [ ] Telemetry: casualty response times, treatment success rates, bleed-out deaths, evacuation times.

---

## 13. Design Principles

1. **The fight comes first.** Treatment is always constrained by the tactical situation. Good medicine can be bad tactics.
2. **Time kills.** Every tick of untreated bleeding is damage. The system rewards fast, correct response and punishes hesitation.
3. **One casualty costs many.** The combat power cost of a casualty is always greater than one soldier. This is the most important emergent property.
4. **Skill matters but isn't magic.** A medic is faster and more capable, but they are one person in one place. They can be suppressed, wounded, or dead.
5. **Information is imperfect.** The leader's casualty picture is built from reports that may be late, wrong, or missing. Triage accuracy depends on the assessor's skill.
6. **No teleportation.** The medic has to physically move to the casualty. The casualty has to be physically carried to the CCP. Distance and terrain are real.
7. **Phase regression is normal.** Treatment getting interrupted by fire is not a failure state — it is the expected reality. The system must handle it gracefully.
8. **Autonomy preserved.** Soldiers decide to help casualties through the same goal-utility system that drives all other behavior. Orders influence but do not override individual psychology.
