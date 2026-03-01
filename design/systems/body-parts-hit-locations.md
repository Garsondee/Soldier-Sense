# Body Parts & Hit Location System

**Status:** Planning Draft
**Priority:** High — prerequisite for wound severity, medical aid, and mobility degradation
**Depends on:** Combat system (combat.go), Soldier struct, SoldierProfile

---

## 1. Design Intent

Replace the single `health float64` pool with a **body-region model** where each bullet strike lands on a specific anatomical zone. The hit location determines:

- **Lethality speed** — head/torso kills fast; limbs bleed out slowly.
- **Functional degradation** — a leg wound slows movement; an arm wound degrades accuracy; a head graze causes disorientation.
- **Medical priority** — the MARCH algorithm in the medical-aid system needs to know *where* the wound is to decide *what* to treat first.
- **Stance interaction** — a prone soldier exposes head and arms; a standing soldier exposes full body. Cover further masks regions.

The model is deliberately coarse — not a per-organ simulation, but enough anatomical granularity to produce meaningfully different combat and medical outcomes from each hit.

---

## 2. Body Regions

Six regions, chosen because each maps to a distinct gameplay consequence:

| Region | Weight (standing) | Lethality | Gameplay effect |
|---|---|---|---|
| **Head** | 0.08 | Extreme | Instant kill or severe disorientation; cognitive degradation |
| **Neck** | 0.04 | Very High | Rapid bleed; airway compromise; high fatality rate |
| **Torso** | 0.38 | High | Organ damage, internal bleed; gradual incapacitation |
| **Arms** | 0.20 (0.10 each) | Low | Accuracy penalty; weapon handling degraded; can self-aid with difficulty |
| **Abdomen** | 0.15 | Medium–High | Painful; moderate bleed; mobility reduced; shock risk |
| **Legs** | 0.15 (0.075 each) | Low–Medium | Mobility penalty or loss; ambulatory status determined here |

### 2.1 Weight Interpretation

Weights represent the **probability of a round striking that region** given a centre-mass aimed shot against a standing, exposed target. They sum to 1.0 and are the base distribution before stance and cover modifiers are applied.

### 2.2 Stance Modifiers

Stance changes which regions are exposed and how much of the body is presented:

| Stance | Head | Neck | Torso | Arms | Abdomen | Legs | Notes |
|---|---|---|---|---|---|---|---|
| **Standing** | 0.08 | 0.04 | 0.38 | 0.20 | 0.15 | 0.15 | Full silhouette |
| **Crouching** | 0.10 | 0.05 | 0.35 | 0.22 | 0.13 | 0.15 | Legs partially folded behind torso; head proportionally larger target |
| **Prone** | 0.18 | 0.06 | 0.10 | 0.30 | 0.06 | 0.30 | Head and arms dominate; torso/abdomen shielded by ground |

These are normalized probability tables — the `ProfileMul` from the stance system already reduces the *overall* chance of being hit; these tables only redistribute *where* a confirmed hit lands.

### 2.3 Cover Region Masking

When a target is behind cover, the cover object defines which regions are shielded. A low wall masks legs and abdomen; a window frame masks torso and below. The hit-location roll is re-weighted to exclude or reduce masked regions.

```
exposedWeight[region] = baseWeight[region] * (1.0 - coverMask[region])
// normalize to sum to 1.0, then roll
```

If all regions are fully masked the shot is a miss (already handled by the existing cover-reduction logic in `resolveBullet`).

---

## 3. Region Health & Wound Model

### 3.1 Per-Region HP

Each region has its own small HP pool representing structural integrity:

| Region | Max HP | Notes |
|---|---|---|
| Head | 20 | Fragile; most hits are immediately critical |
| Neck | 15 | Very fragile; vascular |
| Torso | 50 | Largest pool; can absorb multiple rounds before failure |
| Arm (each) | 25 | Resilient muscle/bone |
| Abdomen | 35 | Moderate; organ risk |
| Leg (each) | 30 | Bone + muscle mass |

**Global HP is removed.** The soldier's overall status is derived from region states, not a single number. See §4 for how incapacitation and death are determined.

### 3.2 Wound Entry

When a bullet hits, it creates a `Wound` struct attached to the struck region:

```go
type BodyRegion int

const (
    RegionHead BodyRegion = iota
    RegionNeck
    RegionTorso
    RegionArmLeft
    RegionArmRight
    RegionAbdomen
    RegionLegLeft
    RegionLegRight
)

type WoundSeverity int

const (
    WoundMinor    WoundSeverity = iota // graze / fragment — low bleed, minor pain
    WoundModerate                       // through-and-through or lodged — steady bleed, functional loss
    WoundSevere                         // major vessel / bone damage — rapid bleed, heavy functional loss
    WoundCritical                       // catastrophic — immediate life threat without intervention
)

type Wound struct {
    Region       BodyRegion
    Severity     WoundSeverity
    BleedRate    float64 // HP/tick lost from this wound (before treatment)
    Pain         float64 // 0–1, feeds into fear/stress and cognitive degradation
    Treated      bool    // true once bleeding is controlled by aid
    TreatedTick  int     // tick when treatment was applied
    TickInflicted int    // tick the wound was created
}
```

### 3.3 Severity Determination

Severity is rolled at hit time based on:

1. **Region lethality class** (head/neck bias toward critical; limbs bias toward minor/moderate).
2. **Damage delivered** — `baseDamage * dmgMul * cqbDamageMul`. Higher damage shifts the severity distribution upward.
3. **Random variance** — a small uniform roll so identical shots don't always produce identical wounds.

```
severityScore = (damage / regionMaxHP) + regionLethalityBias + rand(-0.10, +0.10)

    score < 0.25  → Minor
    score < 0.50  → Moderate
    score < 0.75  → Severe
    score >= 0.75 → Critical
```

Region lethality biases:

| Region | Bias |
|---|---|
| Head | +0.35 |
| Neck | +0.30 |
| Torso | +0.10 |
| Abdomen | +0.05 |
| Arm | -0.10 |
| Leg | -0.05 |

### 3.4 Bleed Rates

Each severity tier maps to a base bleed rate (HP/tick from the wounded region):

| Severity | Bleed Rate | Untreated time to region-zero (torso) |
|---|---|---|
| Minor | 0.05 | ~1000 ticks (~17 min) — unlikely to kill alone |
| Moderate | 0.20 | ~250 ticks (~4 min) |
| Severe | 0.60 | ~83 ticks (~1.4 min) |
| Critical | 1.50 | ~33 ticks (~33 sec) — fatal without immediate intervention |

Bleed rates are per-wound. Multiple wounds stack. Treatment (see medical-aid system) reduces or stops a wound's bleed rate.

### 3.5 Pain

Pain is generated per wound and summed across all wounds. Total pain feeds into:

- `PsychState.ApplyStress()` — each tick, `totalPain * 0.02` added as fear.
- Accuracy degradation — `EffectiveAccuracy` penalized by `totalPain * 0.3`.
- Movement speed penalty — stacks with region-specific mobility loss.
- Coherence — at high total pain (>0.7), soldier may become non-verbal / unable to self-report accurately.

---

## 4. Incapacitation & Death

There is no single death threshold. Instead, two parallel checks run each tick:

### 4.1 Regional Failure

If any region's HP reaches 0:

| Region at 0 HP | Outcome |
|---|---|
| **Head** | Instant death (SoldierStateDead) |
| **Neck** | Instant death |
| **Torso** | Unconscious → dead within ~30 ticks without critical intervention |
| **Abdomen** | Unconscious; slower bleed-out (~120 ticks) |
| **Arm** | Arm disabled — cannot use two-handed weapon effectively; not fatal |
| **Leg** | Leg disabled — non-ambulatory (cannot walk); not immediately fatal |

### 4.2 Blood Loss (Global Pool)

A new global `bloodVolume` field (starts at 1.0, representing 100% blood) decreases as wounds bleed:

```
bloodVolume -= sum(wound.BleedRate) * bleedToBloodFactor per tick
```

| Blood Volume | Effect |
|---|---|
| > 0.80 | Normal function |
| 0.60–0.80 | Mild shock: +0.15 fear, -10% speed, -10% accuracy |
| 0.40–0.60 | Moderate shock: +0.30 fear, -30% speed, -30% accuracy, confusion |
| 0.20–0.40 | Severe shock: non-ambulatory, barely conscious, cannot self-aid |
| ≤ 0.20 | Unconscious → dead within ~60 ticks |

This creates the realistic dynamic where a soldier with "only" a leg wound can still die from cumulative blood loss if untreated.

### 4.3 New Soldier States

Extend `SoldierState` with:

| State | Meaning |
|---|---|
| **SoldierStateWoundedAmbulatory** | Hit but can still move and fight (degraded) |
| **SoldierStateWoundedNonAmbulatory** | Cannot self-move; needs buddy drag/carry |
| **SoldierStateUnconscious** | Alive but no agency; bleeds without self-aid |
| **SoldierStateDead** | Existing — terminal |

Transitions:
```
Moving/Idle → WoundedAmbulatory     (any wound, both legs functional, blood > 0.40)
Moving/Idle → WoundedNonAmbulatory  (leg disabled OR blood 0.20–0.40)
Any wounded → Unconscious           (blood ≤ 0.20 OR torso/abdomen at 0 HP)
Unconscious → Dead                  (blood ≤ 0.0 OR head/neck at 0 HP OR untreated timeout)
```

---

## 5. Functional Degradation

Wounds produce ongoing gameplay penalties beyond raw HP loss:

### 5.1 Mobility

| Condition | Speed Multiplier |
|---|---|
| One leg wounded (moderate+) | 0.50 |
| One leg disabled (0 HP) | 0.15 (crawl only, prone forced) |
| Both legs wounded | 0.25 |
| Both legs disabled | 0.0 (immobile, needs carry) |
| Abdomen severe+ | 0.70 |

### 5.2 Combat Effectiveness

| Condition | Effect |
|---|---|
| Dominant arm wounded | Accuracy × 0.60; reload time × 1.5 |
| Off-hand arm wounded | Accuracy × 0.85; reload time × 1.2 |
| Dominant arm disabled | Cannot fire effectively — weapon handling lost |
| Both arms wounded | Cannot self-aid; cannot fire |
| Head wound (non-fatal) | Disorientation: vision cone narrows 30%; threat confidence decays 2× faster |

### 5.3 Self-Aid Capability

A soldier can only self-aid if:
- At least one arm is functional (HP > 0).
- Conscious (not unconscious state).
- Blood volume > 0.40 (not in severe shock).
- Has available medical supplies (future: individual first-aid kit tracking).

This is the bridge to the medical-aid system — if the soldier cannot self-aid, they depend entirely on buddy aid or the medic.

---

## 6. Integration Points

### 6.1 Combat System (combat.go)

`resolveBullet` currently does:
```go
damage := baseDamage * dmgMul
target.health -= damage
if target.health <= 0 { target.state = SoldierStateDead }
```

Replace with:
1. Roll hit region from stance-adjusted weight table.
2. Apply damage to that region's HP.
3. Determine wound severity.
4. Create `Wound` struct, attach to soldier's wound list.
5. Update soldier state based on §4 rules.

### 6.2 Blackboard

New blackboard fields for self-assessment:
```go
WoundCount       int
WorstWoundRegion BodyRegion
IsAmbulatory     bool
CanSelfAid       bool
BloodVolume      float64  // for internal decision-making
TotalPain        float64
```

### 6.3 Squad Think

Leader now receives richer casualty information:
- How many members are ambulatory wounded vs non-ambulatory vs unconscious.
- Which key roles are degraded (leader hit, machine gunner arm wound, etc.).
- Whether the squad can still manoeuvre or is pinned by casualties.

### 6.4 Radio Reports

Injury radio messages now include:
- Region hit (simplified: "leg wound", "torso hit").
- Ambulatory status.
- Urgency derived from bleed rate / blood volume.

### 6.5 Cognition / Goal Selection

The `HelpCasualty` goal (currently a placeholder in cognition.md) becomes viable: a nearby soldier with a non-ambulatory casualty gets a utility boost for `HelpCasualty` that competes against `Engage` and `Survive`.

### 6.6 Visual Feedback

- Wounded ambulatory soldiers rendered with a small injury indicator.
- Non-ambulatory soldiers rendered prone with a distinct color tint.
- Blood volume < 0.60 adds a pulsing effect.

---

## 7. Data Structures Summary

```go
// BodyMap holds per-region health for one soldier.
type BodyMap struct {
    HP     [8]float64 // indexed by BodyRegion
    MaxHP  [8]float64
    Wounds []Wound
}

// NewBodyMap returns a fully healthy body.
func NewBodyMap() BodyMap { ... }

// ApplyHit rolls a region, creates a wound, reduces region HP.
// Returns the created Wound and whether the soldier died instantly.
func (bm *BodyMap) ApplyHit(damage float64, stance Stance, coverMask [8]float64, rng *rand.Rand) (Wound, bool)

// TickBleed advances all wound bleeding, reduces blood volume,
// returns updated status flags (ambulatory, conscious, alive).
func (bm *BodyMap) TickBleed(bloodVolume *float64) (ambulatory, conscious, alive bool)

// TotalPain returns summed pain across all active wounds.
func (bm *BodyMap) TotalPain() float64

// MobilityMul returns the combined speed multiplier from all leg/abdomen wounds.
func (bm *BodyMap) MobilityMul() float64

// AccuracyMul returns accuracy multiplier from arm/head wounds.
func (bm *BodyMap) AccuracyMul() float64

// CanSelfAid returns true if the soldier has a functional arm and is conscious.
func (bm *BodyMap) CanSelfAid(conscious bool) bool
```

---

## 8. Implementation Phases

### Phase 1 — Region Hit & Wound Creation

- [ ] Define `BodyRegion`, `WoundSeverity`, `Wound`, `BodyMap` types.
- [ ] Implement stance-weighted hit-region roll.
- [ ] Replace `target.health -= damage` with `BodyMap.ApplyHit(...)`.
- [ ] Derive soldier alive/dead from region HP (head/neck zero = instant death; torso zero = delayed death).
- [ ] Keep `soldierMaxHP` as a legacy compat field computed from sum of region HPs for any code that reads it.
- [ ] Tests: hit distribution matches expected stance weights; head hit kills; limb hit does not.

### Phase 2 — Bleeding & Blood Volume

- [ ] Add `bloodVolume float64` to Soldier.
- [ ] Implement per-tick bleed: each untreated wound drains blood.
- [ ] Blood volume thresholds trigger shock effects (speed, accuracy, consciousness).
- [ ] State transitions: ambulatory → non-ambulatory → unconscious → dead from blood loss.
- [ ] Tests: untreated critical torso wound kills within expected tick window; minor limb wound does not kill within 500 ticks.

### Phase 3 — Functional Degradation

- [ ] Mobility penalties from leg/abdomen wounds.
- [ ] Accuracy penalties from arm/head wounds.
- [ ] Pain → stress pipeline.
- [ ] Self-aid capability check.
- [ ] Tests: leg wound reduces speed; arm wound reduces accuracy; both-arms-disabled blocks self-aid.

### Phase 4 — Integration

- [ ] Blackboard fields for wound awareness.
- [ ] Squad Think uses ambulatory count and casualty severity.
- [ ] Radio reports include wound region and urgency.
- [ ] Visual indicators for wounded states.
- [ ] Cover region masking integration.

---

## 9. Design Principles

1. **Coarse, not clinical.** Six regions, four severities. Enough to drive meaningful tactical and medical decisions, not enough to require an anatomy textbook.
2. **Wounds are persistent.** A wound does not vanish — it bleeds until treated or the soldier dies. This forces the medical-aid system to exist.
3. **Function degrades before death.** Most injuries reduce capability long before they kill. A wounded squad is weaker even if nobody is dead yet.
4. **Stance and cover matter twice.** They affect both *whether* you're hit (existing ProfileMul/cover systems) and *where* you're hit (region weight redistribution). Going prone trades leg exposure for head exposure — a real tactical tradeoff.
5. **No magic recovery.** There is no passive HP regeneration. All healing flows through the medical-aid system.
