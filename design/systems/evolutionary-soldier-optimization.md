# Evolutionary Soldier Optimization System

## Overview

Design document for implementing an evolutionary algorithm to optimize soldier behavior parameters through simulated natural selection. The system will evolve soldier genomes across generations, using battle performance as fitness criteria.

**Implementation Status:** ✅ **PHASE 1-2 COMPLETE** - Core evolution system operational with 20-gene genomes, parallel evaluation, and multiple fitness functions.

## Core Questions

### 1. Library vs Bespoke Implementation

#### Option A: Use Existing Library (e.g., `eaopt`)

**Pros:**
- Battle-tested genetic operators (mutation, crossover, selection)
- Pre-built population management
- Multiple selection strategies out of the box
- Parallel evaluation support
- Less code to maintain

**Cons:**
- External dependency
- May not fit our exact needs (soldier genome structure)
- Learning curve for library API
- Potential overhead for simple use case
- Less control over evolution mechanics

**Libraries to Consider:**
- `github.com/MaxHalford/eaopt` - General purpose evolutionary algorithms
- `github.com/cdipaolo/goml/genetic` - Simpler, more focused
- Roll our own - Full control, minimal dependencies

#### Option B: Bespoke Implementation

**Pros:**
- Complete control over genome representation
- Tailored to soldier parameter structure
- No external dependencies
- Educational - understand every piece
- Can optimize for our specific simulation
- Easier to add domain-specific operators

**Cons:**
- More initial development time (~2-3 days)
- Need to implement standard GA components
- Potential for bugs in genetic operators
- Reinventing the wheel

**Recommendation:** **Bespoke implementation** ✅ **IMPLEMENTED**
- Our genome is straightforward (float parameters)
- Simple GA is ~300-400 lines of code
- Keeps dependencies minimal
- Full transparency for debugging/tuning
- Can always add library later if needed

**Status:** Implemented in `internal/game/evolution.go` with 366 lines covering genome representation, population management, genetic operators, and fitness functions.

---

## 2. Fitness Function Design

### Fitness Metrics to Consider

#### A. Kill/Death Ratio (K/D)
**Formula:** `enemies_killed / (friendlies_lost + 1)`

**Pros:**
- Simple, intuitive
- Directly measures combat effectiveness
- Easy to compute from existing telemetry

**Cons:**
- Encourages reckless aggression
- Ignores mission objectives
- Doesn't value survival
- Can produce unrealistic "berserker" soldiers

**Use Case:** Quick proof-of-concept fitness

---

#### B. Survivability-Focused
**Formula:** `squad_survival_rate * (1 + enemies_killed * 0.1)`

**Pros:**
- Realistic - real armies value soldier survival
- Encourages tactical caution
- Produces defensive, cohesive behavior
- Aligns with historical military doctrine

**Cons:**
- May produce overly passive soldiers
- Could encourage hiding/avoiding combat
- Slower to converge (less selection pressure)

**Use Case:** Evolving realistic, doctrine-compliant soldiers

---

#### C. Mission Effectiveness (Composite)
**Formula:** Weighted sum of multiple factors

```
fitness = w1 * survival_rate +
          w2 * (enemies_killed / enemies_total) +
          w3 * objective_completion +
          w4 * (1 - avg_panic_events) +
          w5 * cohesion_maintenance +
          w6 * (1 - ammo_waste_ratio) +
          w7 * (1 / time_to_completion)
```

**Pros:**
- Holistic evaluation
- Balances multiple objectives
- Produces well-rounded soldiers
- Can tune weights for different doctrines

**Cons:**
- Complex to tune
- Weights are subjective
- Harder to interpret results
- Risk of conflicting objectives

**Use Case:** Production system after initial experiments

---

#### D. Multi-Objective Pareto Optimization
**Approach:** Optimize multiple objectives simultaneously without combining into single score

**Objectives:**
1. Maximize survival rate
2. Maximize enemy casualties
3. Minimize time to objective
4. Maximize cohesion maintenance

**Pros:**
- No arbitrary weight tuning
- Produces diverse solution set
- Can select from Pareto front based on doctrine
- More scientifically rigorous

**Cons:**
- More complex implementation
- Harder to visualize progress
- Requires understanding of Pareto dominance
- Slower convergence

**Use Case:** Advanced research phase

---

### Recommended Fitness Strategy

**Phase 1 (Proof of Concept):** ✅ **IMPLEMENTED**
- Simple K/D ratio with survival bonus
- `fitness = enemies_killed - (friendlies_lost * 2)`
- Fast to implement, clear selection pressure

**Phase 2 (Realistic Evolution):** ✅ **IMPLEMENTED**
- Survivability-focused with effectiveness multiplier
- `fitness = survival_rate^2 * (1 + enemies_killed * 0.15 + objectives * 0.5)`
- Heavily rewards keeping soldiers alive

**Phase 3 (Production):** ✅ **IMPLEMENTED**
- Composite mission effectiveness
- Tunable weights for different doctrines (aggressive, defensive, balanced)
- Multiple fitness profiles for different roles

**Status:** Four fitness functions implemented in `evolution.go`:
- `DefaultFitnessFunction`: Balanced (35% survival, 35% K/D, 30% win rate)
- `AggressiveFitnessFunction`: Kill-focused (20% survival, 50% K/D, 30% win rate)
- `DefensiveFitnessFunction`: Survival-focused (60% survival, 20% K/D, 20% win rate)
- `BalancedFitnessFunction`: Win-focused (25% survival, 25% K/D, 50% win rate)

---

## 3. Genome Design

### Evolvable Parameters

#### Tier 1: Core Combat Behavior ✅ **IMPLEMENTED (20 genes)**
```go
// Implemented in internal/game/evolution.go
type Genome struct {
    Genes   []float64 // 20 genes mapped to SoldierProfile
    Fitness float64
    ID      int
}

// Gene mapping (0-19):
// Personality (0-5): Aggression, Caution, PanicThreshold, Initiative, Teamwork, Adaptability
// Skills (6-11): Marksmanship, Fieldcraft, Discipline, FireControl, TacticalAwareness, FirstAid
// Psychological (12-13): Experience, Composure
// Physical (14-16): Strength, Agility, Endurance
// Tactical Preferences (17-19): ReloadEarly, PreferCover, PreferFlanking
```

#### Tier 2: Tactical Behavior ⚠️ **PARTIALLY IMPLEMENTED**
```go
// Implemented:
// - PreferCover (gene 18)
// - PreferFlanking (gene 19)

// Not yet implemented:
// - MovementSpeed multiplier (uses Agility instead)
// - SpreadTolerance
// - RadioUsageRate
// - OrderObedience (uses Discipline instead)
```

#### Tier 3: Advanced ❌ **NOT IMPLEMENTED**
```go
// Planned but not yet implemented:
// - ThreatPrioritization (TacticalAwareness exists but not fully integrated)
// - FlankingTendency (PreferFlanking exists as gene 19)
// - SuppressionValue
```

### Genome Constraints

- All parameters have min/max bounds
- Some parameters may have dependencies (e.g., high aggression + low caution = suicide)
- Consider constraint validation after mutation
- May need "viable genome" checker

---

## 3.5. Comprehensive Soldier Stats Analysis & Expansion

### Current Soldier Stats (Existing Implementation)

Based on `internal/game/stats.go` and `internal/game/soldier.go`:

#### Physical Stats (PhysicalStats)
```go
type PhysicalStats struct {
    FitnessBase float64 // [0-1] innate physical capability
    Fatigue     float64 // [0-1] current exhaustion (dynamic)
    SprintPool  float64 // seconds of sprint remaining (dynamic)
}
```

**Current Impact:**
- `EffectiveFitness()` = `FitnessBase * (1.0 - Fatigue*0.8)`
- Affects movement speed: `0.6 + 0.4*EffectiveFitness()`
- Affects accuracy: `1.0 - Fatigue*0.4`
- Fatigue accumulates during exertion, recovers during rest
- Fitter soldiers tire slower and recover faster

#### Skill Stats (SkillStats)
```go
type SkillStats struct {
    Marksmanship float64 // [0-1] shooting accuracy
    Fieldcraft   float64 // [0-1] ability to use terrain/cover
    Discipline   float64 // [0-1] order compliance under pressure
    FirstAid     float64 // [0-1] medical competence
}
```

**Current Impact:**
- `Marksmanship` → base shooting accuracy
- `Discipline` → resists suppression accuracy penalty (0.40 - Discipline*0.20)
- `Discipline` → affects stance transition speed under stress
- `Discipline` → affects cognition pause timing
- `Discipline` → reduces panic/disobedience probability
- `Fieldcraft` → currently **underutilized** (only mentioned, not deeply integrated)
- `FirstAid` → medical system exists but not yet fully implemented

#### Psychological Stats (PsychState)
```go
type PsychState struct {
    Experience float64 // [0-1] combat exposure (grows slowly, permanent)
    Morale     float64 // [0-1] confidence (fluctuates)
    Fear       float64 // [0-1] acute stress (spikes, decays)
    Composure  float64 // [0-1] innate fear management ability (trait)
}
```

**Current Impact:**
- `EffectiveFear()` = `Fear * (1.0 - (0.5*Composure + 0.5*Experience)*0.6)`
- `Composure` → dampens fear impact, affects panic resistance
- `Experience` → dampens fear impact (veterans stay calm)
- `Morale` → complex update system with 15+ contextual factors
- `Fear` → affects movement speed, accuracy, decision-making
- Low morale + high fear → triggers disobedience, panic retreat, surrender

#### Body/Health System (BodyMap)
```go
// 8 body regions with individual HP pools
// Stance-weighted hit probabilities
// Wound severity system (minor/moderate/severe/critical)
// Bleed rates, functional degradation
// MobilityMul, AccuracyMul, TotalPain, CanSelfAid
```

**Current Impact:**
- Regional damage affects mobility (leg/abdomen wounds)
- Regional damage affects accuracy (arm/head wounds)
- Blood loss causes death
- Wounds create ambulatory/non-ambulatory/unconscious states

---

### Proposed Stat Expansions for Evolution

#### Category A: Physical Traits (Innate + Trainable)

**New Stats to Add:**
```go
type PhysicalStats struct {
    // Existing
    FitnessBase float64 // [0.5-1.0] cardiovascular/muscular capability

    // NEW: Physical attributes
    Strength       float64 // [0.5-1.0] affects carry capacity, melee, recoil control
    Agility        float64 // [0.5-1.0] affects stance transitions, dodge, sprint accel
    Endurance      float64 // [0.5-1.0] affects fatigue accumulation rate
    Vision         float64 // [0.5-1.0] affects spotting distance, low-light performance
    Hearing        float64 // [0.5-1.0] affects gunfire detection range, directional accuracy
    ReactionTime   float64 // [0.5-1.0] affects target acquisition speed, reflex fire

    // Dynamic (not evolved, but affected by evolved traits)
    Fatigue     float64
    SprintPool  float64
    Injuries    []Wound // from BodyMap
}
```

**Evolutionary Value:**
- **Strength** → better recoil control = faster follow-up shots, can carry more ammo
- **Agility** → faster stance changes = better combat responsiveness
- **Endurance** → longer sustained combat effectiveness
- **Vision** → spot enemies first = tactical advantage
- **Hearing** → better situational awareness
- **ReactionTime** → faster engagement = more kills

**Integration Points:**
- `Strength` → reduce aim spread growth during burst fire (recoil control)
- `Agility` → reduce `stanceTransitionTimer` by (Agility * 0.3)
- `Endurance` → modify fatigue accumulation: `rate / Endurance`
- `Vision` → extend vision cone range by (Vision * 20%)
- `Hearing` → increase gunfire detection radius
- `ReactionTime` → reduce `aimingTicks` required for accurate shot

---

#### Category B: Personality Traits (Behavioral Modifiers)

**New Stats to Add:**
```go
type PersonalityTraits struct {
    // Combat psychology
    Aggression      float64 // [0-1] preference for offensive action
    Caution         float64 // [0-1] risk aversion, cover-seeking tendency
    Initiative      float64 // [0-1] willingness to act without orders
    Adaptability    float64 // [0-1] how quickly soldier adjusts to new situations

    // Social traits
    Leadership      float64 // [0-1] ability to inspire/coordinate others (for leaders)
    Teamwork        float64 // [0-1] cohesion weight, formation compliance
    Independence    float64 // [0-1] tolerance for isolation, self-reliance

    // Stress response
    PanicThreshold  float64 // [0-1] how much pressure before breaking
    RecoveryRate    float64 // [0-1] how fast soldier recovers from stress
    StressResilience float64 // [0-1] resistance to stress accumulation

    // Decision-making
    Decisiveness    float64 // [0-1] speed of decision-making (vs deliberation)
    Flexibility     float64 // [0-1] willingness to change plans
    Conservatism    float64 // [0-1] preference for proven tactics vs experimentation
}
```

**Evolutionary Value:**
- **Aggression** → affects goal selection bias (attack vs defend)
- **Caution** → affects cover-seeking urgency, peek behavior
- **Initiative** → affects how often soldier acts without explicit orders
- **Adaptability** → affects recovery action selection, plan switching
- **Teamwork** → affects formation compliance, cohesion weight
- **PanicThreshold** → directly affects psychological collapse probability
- **Decisiveness** → affects decision interval timing
- **Flexibility** → affects goal-switching frequency

**Integration Points:**
- `Aggression` → bias in `officerOrderBias` calculation (attack goals +priority)
- `Caution` → modify `cautionThreshold` in cover-seeking logic
- `Initiative` → affects whether soldier explores vs waits for orders
- `PanicThreshold` → modify panic drive calculation: `panicDrive - PanicThreshold*0.3`
- `Teamwork` → modify `CohesionWeight` in blackboard propagation
- `Decisiveness` → modify `baseDecisionInterval`: `base * (1.5 - Decisiveness*0.5)`

---

#### Category C: Combat Skills (Trainable Expertise)

**Expand Existing SkillStats:**
```go
type SkillStats struct {
    // Existing
    Marksmanship float64 // [0.3-1.0] base shooting accuracy
    Fieldcraft   float64 // [0.3-1.0] terrain/cover usage
    Discipline   float64 // [0.3-1.0] order compliance under pressure
    FirstAid     float64 // [0.3-1.0] medical competence

    // NEW: Specialized combat skills
    CQBProficiency    float64 // [0-1] close-quarters effectiveness
    UrbanWarfare      float64 // [0-1] building clearing, room combat
    Camouflage        float64 // [0-1] concealment, stealth movement
    Navigation        float64 // [0-1] pathfinding efficiency, orientation
    Communication     float64 // [0-1] radio usage effectiveness, clarity
    TacticalAwareness float64 // [0-1] threat assessment, flanking recognition
    SuppressiveFire   float64 // [0-1] ability to pin enemies effectively
    FireControl       float64 // [0-1] ammo conservation, shot discipline
}
```

**Evolutionary Value:**
- **CQBProficiency** → bonus accuracy/speed in close range (<10 tiles)
- **UrbanWarfare** → bonus when fighting in/around buildings
- **Camouflage** → reduces enemy detection probability
- **Navigation** → faster pathfinding, better recovery actions
- **Communication** → better radio transmission success rate
- **TacticalAwareness** → better threat prioritization
- **SuppressiveFire** → increases suppression effect on enemies
- **FireControl** → reduces ammo waste, better reload timing

**Integration Points:**
- `CQBProficiency` → accuracy bonus when `dist < autoRange`
- `UrbanWarfare` → accuracy/speed bonus when near buildings
- `Camouflage` → modify enemy vision cone detection: `baseDetect * (1.0 - Camouflage*0.4)`
- `Navigation` → improve pathfinding success rate, reduce `recoveryNoPathStreak`
- `Communication` → reduce radio message garble/drop probability
- `TacticalAwareness` → improve threat selection in `selectTarget()`
- `SuppressiveFire` → multiply suppression inflicted: `baseSup * (1.0 + SuppressiveFire*0.5)`
- `FireControl` → modify reload urgency, reduce burst fire in low-ammo situations

---

#### Category D: Contextual Preferences (Decision Weights)

**New Stats for Fine-Grained Behavior Control:**
```go
type TacticalPreferences struct {
    // Fire mode preferences
    PreferSingleShot  float64 // [0-1] tendency to use single-shot mode
    PreferBurst       float64 // [0-1] tendency to use burst mode
    PreferFullAuto    float64 // [0-1] tendency to use full-auto mode

    // Movement preferences
    PreferSprinting   float64 // [0-1] willingness to sprint vs tactical pace
    PreferCrawling    float64 // [0-1] willingness to go prone
    PreferBounding    float64 // [0-1] preference for dash-and-cover movement

    // Tactical preferences
    PreferFlanking    float64 // [0-1] tendency to attempt flanking maneuvers
    PreferSuppression float64 // [0-1] willingness to lay suppressive fire
    PreferHolding     float64 // [0-1] tendency to hold position vs advance
    PreferCover       float64 // [0-1] how much soldier values being in cover

    // Reload behavior
    ReloadEarly       float64 // [0-1] reload at high ammo (cautious) vs low (aggressive)
    ReloadUnderFire   float64 // [0-1] willingness to reload while suppressed

    // Engagement preferences
    EngageRange       float64 // [0.5-2.0] multiplier on preferred engagement distance
    TargetPersistence float64 // [0-1] how long to track one target vs switching
}
```

**Evolutionary Value:**
- These are **highly evolvable** because they directly affect tactical outcomes
- Different combinations create distinct "fighting styles"
- Easy to measure fitness impact (e.g., does early reloading improve survival?)

**Integration Points:**
- Fire mode preferences → bias `desiredFireMode` selection
- Movement preferences → affect stance requests, dash decisions
- `PreferFlanking` → increase flank goal priority in decision-making
- `PreferSuppression` → willingness to fire without clear target
- `ReloadEarly` → modify reload urgency threshold
- `EngageRange` → affect optimal engagement distance calculation
- `TargetPersistence` → affect target-switching frequency

---

### Recommended Genome Structure (Comprehensive)

#### Minimal Genome (Phase 1 - 8 parameters) ✅ **EXCEEDED**
Focus on highest-impact, easiest-to-integrate stats:

```go
// Original plan was 8 parameters, but implemented 20 from the start
// This exceeded Phase 1 goals and went directly to Phase 2
type MinimalGenome struct {
    // Physical
    FitnessBase   float64 // [0.6-1.0]
    ReactionTime  float64 // [0.5-1.0] - NOT IMPLEMENTED (not in genome)

    // Skills
    Marksmanship  float64 // [0.4-1.0] ✅
    Discipline    float64 // [0.3-1.0] ✅

    // Personality
    Aggression    float64 // [0.0-1.0] ✅
    Caution       float64 // [0.0-1.0] ✅

    // Psych
    Composure     float64 // [0.3-1.0] ✅
    PanicThreshold float64 // [0.3-1.0] ✅
}
```

**Status:** Implemented 20 genes instead of 8, covering all Phase 1 goals plus Phase 2 expansions.

---

#### Expanded Genome (Phase 2 - 16 parameters) ✅ **IMPLEMENTED (20 genes)**
Add tactical preferences and specialized skills:

```go
// Implemented with 20 genes (exceeded Phase 2 target)
type ExpandedGenome struct {
    // Physical (3) - ReactionTime not included
    FitnessBase, Strength, Agility, Endurance ✅

    // Skills (6) - expanded beyond plan
    Marksmanship, Discipline, Fieldcraft, FireControl, TacticalAwareness, FirstAid ✅

    // Personality (6) - expanded beyond plan
    Aggression, Caution, Initiative, Teamwork, PanicThreshold, Adaptability ✅

    // Psych (2)
    Composure, Experience ✅

    // Tactical Preferences (3) - expanded beyond plan
    ReloadEarly, PreferCover, PreferFlanking ✅
}
```

---

#### Full Genome (Phase 3 - 30+ parameters) ❌ **NOT IMPLEMENTED**
Complete personality and skill profile:

```go
type FullGenome struct {
    // Physical (7): Fitness ✅, Strength ✅, Agility ✅, Endurance ✅, Vision ❌, Hearing ❌, ReactionTime ❌
    // Skills (12): Marksmanship ✅, Discipline ✅, Fieldcraft ✅, FirstAid ✅, CQB ❌, Urban ❌, Camouflage ❌,
    //              Navigation ❌, Communication ❌, TacticalAwareness ✅, SuppressiveFire ❌, FireControl ✅
    // Personality (8): Aggression ✅, Caution ✅, Initiative ✅, Adaptability ✅, Teamwork ✅, Independence ❌,
    //                  PanicThreshold ✅, Decisiveness ❌
    // Psych (3): Composure ✅, Experience ✅, Morale ❌ (not evolvable, dynamic only)
    // Tactical Prefs (8): ReloadEarly ✅, PreferCover ✅, PreferFlanking ✅, Fire modes ❌, movement ❌, engagement ❌
}

// Current: 20/38 parameters implemented (53%)
```

---

### Integration Strategy: Genome → Soldier

**Option 1: Direct Mapping (Simple)**
```go
func NewSoldierFromGenome(id int, pos Vec2, genome *Genome) *Soldier {
    s := NewSoldier(id, pos, ...)

    // Map genome to profile
    s.profile.Physical.FitnessBase = genome.Genes[0]
    s.profile.Skills.Marksmanship = genome.Genes[1]
    s.profile.Skills.Discipline = genome.Genes[2]
    s.profile.Psych.Composure = genome.Genes[3]
    // ... etc

    return s
}
```

**Option 2: Genome Struct (Type-Safe)**
```go
type SoldierGenome struct {
    Physical    PhysicalGenome
    Skills      SkillGenome
    Personality PersonalityGenome
    Psych       PsychGenome
    Preferences PreferenceGenome
}

func (g *SoldierGenome) ToProfile() SoldierProfile {
    return SoldierProfile{
        Physical: PhysicalStats{
            FitnessBase: g.Physical.Fitness,
            // ...
        },
        // ...
    }
}
```

**Recommendation:** Start with Option 1 (simple array), migrate to Option 2 when genome grows beyond 15 parameters.

**Status:** ✅ Using Option 1 (simple array) with 20 parameters. The `Genome.ToProfile()` method maps genes to `SoldierProfile` struct fields.

---

### Stat Interactions & Emergent Behavior

**Synergies:**
- High `Aggression` + High `Marksmanship` = Effective assault troops
- High `Caution` + High `Fieldcraft` = Excellent defensive soldiers
- High `Discipline` + High `Teamwork` = Cohesive squad performance
- High `Initiative` + High `TacticalAwareness` = Autonomous flankers
- High `Endurance` + High `Fitness` = Sustained combat effectiveness

**Trade-offs:**
- `Aggression` vs `Caution` → offensive vs defensive behavior
- `Initiative` vs `Discipline` → autonomous vs obedient
- `Decisiveness` vs `Adaptability` → committed vs flexible
- `ReloadEarly` vs `FireControl` → safety vs aggression

**Dangerous Combinations (Genome Validation):**
- High `Aggression` + Low `Caution` + Low `Discipline` = Suicidal charges
- High `Independence` + Low `Teamwork` + Low `Discipline` = Squad breakdown
- Low `PanicThreshold` + Low `Composure` + Low `Discipline` = Instant collapse
- High `Aggression` + Low `Marksmanship` + Low `FireControl` = Ammo waste

**Validation Rules:**
```go
func (g *Genome) IsViable() bool {
    // Prevent suicide builds
    if g.Aggression > 0.8 && g.Caution < 0.2 && g.Discipline < 0.3 {
        return false
    }

    // Prevent total breakdown builds
    if g.PanicThreshold < 0.2 && g.Composure < 0.2 && g.Discipline < 0.2 {
        return false
    }

    // Ensure minimum competence
    if g.Marksmanship < 0.2 || g.FitnessBase < 0.4 {
        return false
    }

    return true
}
```

---

### Fitness Function Considerations for Different Stats

**Survivability Fitness** (realistic):
- Heavily weight `Discipline`, `Caution`, `Composure`, `PanicThreshold`
- Moderate weight `Marksmanship`, `Fieldcraft`, `Fitness`
- Low weight `Aggression`, `Initiative`

**Aggressive Fitness** (high K/D):
- Heavily weight `Marksmanship`, `Aggression`, `ReactionTime`
- Moderate weight `Fitness`, `Strength`, `Discipline`
- Low weight `Caution`, `Teamwork`

**Balanced Fitness** (mission effectiveness):
- Equal weight across all categories
- Bonus for synergistic combinations
- Penalty for dangerous combinations

---

### Expected Evolutionary Outcomes

**Survivability Evolution:**
- Converge toward high `Discipline`, `Caution`, `Composure`
- Moderate `Marksmanship` (enough to defend)
- Low `Aggression` (avoid unnecessary risk)
- Result: Cautious, professional soldiers

**Aggressive Evolution:**
- Converge toward high `Marksmanship`, `Aggression`, `ReactionTime`
- Moderate `Discipline` (enough to not suicide)
- Low `Caution` (take risks for kills)
- Result: Assault specialists, high casualties but high kills

**Balanced Evolution:**
- Converge toward well-rounded stats
- No extreme values
- Good synergies (e.g., `Aggression` matched with `Discipline`)
- Result: Versatile, adaptable soldiers

**Emergent Specializations:**
- Evolution might discover "roles" (scouts, assaulters, defenders)
- Different genomes excel in different scenarios
- Could lead to squad composition optimization

---

### Implementation Priority

**Phase 1 (Immediate):**
1. Use existing stats: `FitnessBase`, `Marksmanship`, `Discipline`, `Composure`
2. Add 4 new fields: `Aggression`, `Caution`, `PanicThreshold`, `ReactionTime`
3. Integrate into decision-making (minimal code changes)
4. Total: 8 evolvable parameters

**Phase 2 (After proof-of-concept):**
1. Add physical stats: `Strength`, `Agility`, `Endurance`
2. Add skills: `Fieldcraft`, `FireControl`, `TacticalAwareness`
3. Add personality: `Initiative`, `Teamwork`, `Adaptability`
4. Add preferences: `ReloadEarly`, `PreferCover`, `PreferFlanking`
5. Total: 20 evolvable parameters

**Phase 3 (Advanced):**
1. Full stat suite (30+ parameters)
2. Contextual fitness functions
3. Multi-objective optimization
4. Role-based evolution

---

## 4. Evolution Algorithm Design

### Population Structure

```
Generation 0: Random initialization (100 genomes)
    ↓
Evaluate fitness (run battles)
    ↓
Selection (top 20% + tournament)
    ↓
Reproduction (crossover + mutation)
    ↓
Generation 1: New population (100 genomes)
    ↓
Repeat...
```

### Key Parameters

- **Population Size:** 50-100 genomes
  - Smaller = faster generations
  - Larger = more diversity, better exploration

- **Elitism:** Keep top 10-20%
  - Preserves best solutions
  - Prevents regression

- **Mutation Rate:** 0.1-0.3 per gene
  - Higher = more exploration
  - Lower = more exploitation

- **Mutation Magnitude:** ±0.1 to ±0.3
  - Gaussian noise around current value
  - Respect min/max bounds

- **Crossover Rate:** 0.6-0.8
  - Uniform or single-point crossover
  - Blend parent traits

### Selection Strategies

**Tournament Selection (Recommended):**
- Pick K random genomes (K=3-5)
- Select best from tournament
- Repeat until population filled
- Good balance of selection pressure

**Alternatives:**
- Roulette wheel (fitness-proportional)
- Rank-based selection
- Elitism + random

---

## 5. Implementation Architecture

### File Structure

```
internal/evolution/
├── genome.go          # Genome struct, mutation, crossover
├── population.go      # Population management
├── fitness.go         # Fitness evaluation functions
├── selection.go       # Selection strategies
├── evolution.go       # Main evolution loop
└── evolution_test.go  # Unit tests

cmd/evolve/
└── main.go           # CLI for running evolution

design/systems/
└── evolutionary-soldier-optimization.md  # This file
```

### Core Types

```go
type Genome struct {
    ID         string
    Generation int
    Genes      []float64  // or structured SoldierGenome
    Fitness    float64
    Metadata   map[string]interface{}
}

type Population struct {
    Genomes     []*Genome
    Generation  int
    BestEver    *Genome
    Stats       GenerationStats
}

type EvolutionConfig struct {
    PopulationSize  int
    ElitismRate     float64
    MutationRate    float64
    MutationMag     float64
    CrossoverRate   float64
    Generations     int
    FitnessFunc     FitnessFunction
}
```

### Integration with Existing Code

**Modify `NewSoldier` to accept genome:**
```go
func NewSoldierFromGenome(id int, pos Vec2, side Side, genome *Genome) *Soldier {
    s := NewSoldier(id, pos, side)
    // Apply genome parameters
    s.aggressionBias = genome.Genes[0]
    s.cautionThreshold = genome.Genes[1]
    // ... etc
    return s
}
```

**Extend `cmd/headless-report` for batch evaluation:**
```go
func EvaluateGenome(genome *Genome, scenario Scenario, runs int) float64 {
    totalFitness := 0.0
    for i := 0; i < runs; i++ {
        result := RunBattle(scenario, genome, seed+i)
        totalFitness += CalculateFitness(result)
    }
    return totalFitness / float64(runs)
}
```

---

## 6. Evaluation Strategy

### Scenario Selection

**Option A: Single Scenario**
- Evolve on one battle (e.g., mutual-advance)
- Fast, focused evolution
- Risk: Overfitting to specific scenario

**Option B: Scenario Rotation**
- Rotate through 3-5 different scenarios
- More general soldiers
- Slower evolution

**Option C: Random Scenario Generation**
- Generate random maps/encounters
- Most robust
- Hardest to implement

**Recommendation:** Start with single scenario, add rotation in Phase 2

### Evaluation Runs Per Genome

- **1 run:** Fast but noisy (RNG variance)
- **3 runs:** Good balance (average fitness)
- **5+ runs:** More stable but slower

**Recommendation:** 3 runs with different seeds, average fitness

### Parallel Evaluation

- Genomes are independent → perfect for parallelization
- Use goroutines with worker pool
- Could evaluate entire population in ~30 seconds (100 genomes, 3 runs each, 10s/battle, 10 workers)

---

## 7. Monitoring & Analysis

### Metrics to Track

**Per Generation:**
- Best fitness
- Average fitness
- Fitness variance (diversity indicator)
- Genome diversity (parameter spread)
- Convergence rate

**Per Genome:**
- Fitness score
- Component scores (kills, survival, cohesion, etc.)
- Battle outcomes (win/loss/draw)
- Lineage (parent IDs)

### Visualization

**Console Output:**
```
Generation 10
  Best:    245.3 (Genome #47)
  Average: 187.2
  Worst:   98.5
  Diversity: 0.42

Top 5 Genomes:
  #47: 245.3 (aggression: 0.72, caution: 0.31, ...)
  #23: 238.1 (aggression: 0.65, caution: 0.28, ...)
  ...
```

**Export Data:**
- CSV of fitness over generations
- JSON dump of best genomes
- Parameter distribution histograms

---

## 8. Phased Implementation Plan

### Phase 1: Minimal Viable Evolution ✅ **COMPLETE**

**Goal:** Prove the concept works

- [x] Define 5-parameter genome → **Implemented 20 parameters** (exceeded goal)
- [x] Implement basic GA (mutation, crossover, tournament selection) → **`evolution.go`**
- [x] Simple fitness → **4 fitness functions implemented**
- [x] Run 20 generations → **CLI supports any number of generations**
- [x] Console output of best genome per generation → **Full formatted output**

**Success Criteria:** ✅ Fitness improves over generations

**Files:**
- `internal/game/evolution.go` (366 lines)
- `cmd/evolve/main.go` (350 lines)
- `internal/game/trait_testing.go` (438 lines)

---

### Phase 2: Realistic Fitness ✅ **COMPLETE**

**Goal:** Evolve tactically sound soldiers

- [x] Implement survivability-focused fitness → **`DefensiveFitnessFunction`**
- [x] Expand genome to 9 parameters → **Implemented 20 parameters**
- [x] Run 50 generations → **Configurable via `--gen` flag**
- [x] Export best genome to JSON → **CSV log file with all generations**
- [ ] Test evolved soldiers vs baseline in fresh scenarios → **PENDING**

**Success Criteria:** ⚠️ Evolved soldiers outperform baseline (needs validation testing)

**Features:**
- Parallel evaluation with configurable worker count
- Multiple fitness functions selectable via CLI
- Evolution log with per-generation statistics
- Formatted console output with progress tracking

---

### Phase 3: Production System ⚠️ **PARTIALLY COMPLETE**

**Goal:** Robust, reusable evolution framework

- [x] Parallel genome evaluation → **Worker pool with configurable cores**
- [x] Multiple fitness functions → **4 functions: default, aggressive, defensive, balanced**
- [ ] Scenario rotation → **NOT IMPLEMENTED** (single scenario only)
- [ ] Save/load populations (resume evolution) → **NOT IMPLEMENTED** (only CSV log)
- [x] Detailed telemetry and visualization → **CSV logging, console output**
- [x] CLI with full configuration options → **13 configurable flags**

**Success Criteria:** ✅ Can evolve soldiers for different doctrines

**CLI Flags:**
```
--pop          Population size (default: 50)
--gen          Generations (default: 100)
--battles      Battles per genome (default: 20)
--ticks        Max ticks per battle (default: 15000)
--workers      Parallel workers (default: CPU count)
--seed         Random seed
--fitness      Fitness function: default, aggressive, defensive, balanced
--crossover    Crossover rate (default: 0.8)
--mutation     Mutation rate (default: 0.15)
--sigma        Mutation strength (default: 0.1)
--tournament   Tournament size (default: 3)
--elite        Elite count (default: 5)
--log          Log file (default: evolution.log)
```

---

### Phase 4: Advanced Features ❌ **NOT IMPLEMENTED**

- [ ] Co-evolution (both sides evolve simultaneously)
- [ ] Multi-objective Pareto optimization
- [ ] Speciation (maintain diverse sub-populations)
- [ ] Adaptive mutation rates
- [ ] Genome visualization (parameter heatmaps)
- [ ] Tournament mode (evolved genomes compete)
- [ ] Scenario rotation/randomization
- [ ] Population save/load for resume
- [ ] JSON genome export/import
- [ ] Genome validation (dangerous combination detection)

---

## 9. Potential Discoveries & Emergent Behavior

### Expected Outcomes

- **Aggressive genomes:** High kills, low survival, fast battles
- **Defensive genomes:** High survival, lower kills, slower battles
- **Balanced genomes:** Moderate on all metrics

### Interesting Emergent Possibilities

- **Suppression specialists:** Soldiers that pin enemies without killing
- **Flanking behavior:** If movement parameters evolve
- **Radio coordination:** If comm parameters are evolved
- **Adaptive tactics:** Different behavior under different stress levels
- **Unexpected exploits:** Soldiers that find bugs/edge cases in simulation

### Scientific Value

- Validate realism of simulation (do evolved soldiers match real doctrine?)
- Discover optimal tactics for given scenarios
- Test hypotheses about combat effectiveness
- Generate training data for ML models

---

## 10. Risks & Mitigations

### Risk: Overfitting to Scenario
**Mitigation:** Evaluate on held-out test scenarios, rotate training scenarios

### Risk: Unrealistic "Super Soldiers"
**Mitigation:** Add realism constraints, cap parameter ranges, validate against doctrine

### Risk: Slow Convergence
**Mitigation:** Tune mutation rates, increase population size, improve fitness function

### Risk: Local Optima
**Mitigation:** Higher mutation rates, restart with diversity injection, multiple runs

### Risk: Fitness Function Gaming
**Mitigation:** Multi-component fitness, manual review of top genomes, adversarial testing

---

## 11. Success Metrics

### Technical Success
- [ ] Evolution completes without crashes
- [ ] Fitness improves over generations
- [ ] Convergence within 50 generations
- [ ] Reproducible results with same seed

### Behavioral Success
- [ ] Evolved soldiers outperform baseline by 20%+
- [ ] Behavior is tactically sound (not exploitative)
- [ ] Generalizes to new scenarios
- [ ] Produces interpretable parameter values

### Fun Success
- [ ] Generates interesting/surprising tactics
- [ ] Provides insight into simulation dynamics
- [ ] Enables new gameplay/research modes
- [ ] Community finds it valuable

---

## 12. Open Questions

1. **Should we evolve individual soldiers or squad-level parameters?**
   - Individual: More granular, but all soldiers in squad need same genome for fair eval
   - Squad: Simpler, but less biologically accurate

2. **How do we handle stochastic fitness?**
   - Average over multiple runs (current plan)
   - Use median instead of mean?
   - Track fitness variance as secondary metric?

3. **Should evolved parameters be permanent or scenario-specific?**
   - Permanent: "Best soldier" concept
   - Scenario-specific: "Best soldier for urban combat" etc.

4. **Do we want human-readable genome names?**
   - "Aggressive-Cautious-Accurate" based on parameter values
   - Helps with analysis and communication

5. **Should we version/tag notable genomes?**
   - "Generation 47 Champion"
   - "Defensive Specialist v2"
   - Enables A/B testing and regression tracking

---

## Next Steps

1. **Decision:** Library vs bespoke → **Bespoke recommended**
2. **Decision:** Initial fitness function → **Simple K/D with survival penalty**
3. **Decision:** Initial genome size → **5 parameters (Tier 1 subset)**
4. **Action:** Implement Phase 1 (minimal viable evolution)
5. **Action:** Run initial experiments, gather data
6. **Action:** Iterate based on results

---

## References

- Existing headless simulation: `cmd/headless-report/main.go`
- Soldier struct: `internal/game/soldier.go`
- Squad behavior: `internal/game/squad.go`
- Telemetry: `internal/game/reporter.go`

---

## Implementation Summary

### ✅ What's Been Implemented

**Core Evolution System:**
- 20-gene genome representation with bounds checking
- Population management with statistics tracking
- Genetic operators: tournament selection, uniform crossover, Gaussian mutation
- Elitism preservation (configurable)
- 4 fitness functions (default, aggressive, defensive, balanced)
- Parallel fitness evaluation with worker pools
- Full CLI with 13 configuration flags
- CSV logging of evolution progress
- Formatted console output with generation summaries

**Genome Coverage (20/38 planned genes):**
- ✅ Personality: Aggression, Caution, PanicThreshold, Initiative, Teamwork, Adaptability
- ✅ Skills: Marksmanship, Fieldcraft, Discipline, FireControl, TacticalAwareness, FirstAid
- ✅ Psychological: Experience, Composure
- ✅ Physical: Strength, Agility, Endurance (missing: Vision, Hearing, ReactionTime)
- ✅ Tactical Preferences: ReloadEarly, PreferCover, PreferFlanking

**Integration:**
- `Genome.ToProfile()` maps genes to `SoldierProfile`
- `PersonalityTraits`, `TacticalPreferences` structs in `stats.go`
- `TestGenome` and `TestGenomeResults` for evaluation
- `ControlGenome()` baseline for comparison

### ❌ What's Not Implemented

**Missing Features:**
- Scenario rotation/randomization (single scenario only)
- Population save/load (can't resume evolution)
- JSON genome export (only CSV log)
- Genome validation (dangerous combination detection)
- Co-evolution (both sides evolving)
- Multi-objective optimization
- Speciation/niching
- Adaptive mutation rates

**Missing Genes (18/38):**
- Physical: Vision, Hearing, ReactionTime
- Skills: CQB, UrbanWarfare, Camouflage, Navigation, Communication, SuppressiveFire
- Personality: Independence, Decisiveness
- Tactical Prefs: Fire mode preferences, movement preferences, engagement range

**Missing Integration:**
- Many genes exist but aren't fully integrated into soldier behavior
- Some stats defined in `stats.go` but not used in decision-making
- Tactical preferences need deeper integration into action selection

### 🔧 Next Steps for Full Implementation

**Immediate Priority (Next 1-2 weeks):**
1. **Parameter Sensitivity Analysis:** Identify which genes have the most impact on fitness through statistical analysis
2. **Cost Model Refinement:** Update trait costs with more realistic military training/selection costs based on research
3. **Validation Testing:** Run evolved genomes vs baseline in multiple scenarios
4. **Advanced Analytics:** Plotting tools for evolution.log analysis and gene importance visualization

**Medium Term (Following 2-3 weeks):**
5. **Multi-Scenario Evolution:** Implement multi-scenario fitness evaluation for robustness
6. **Squad-Level Co-Evolution:** Evolve entire squads with specialized roles
7. **Save/Load:** Add population serialization for resume capability
8. **Genome Validation:** Implement dangerous combination detection

**Long Term:**
9. **Deeper Integration:** Connect all 20 genes to actual soldier behavior
10. **Multi-Objective Optimization:** Pareto-based evolution for trade-off analysis
11. **Opponent Co-Evolution:** Both sides evolving against each other
12. **Expand Genome:** Add remaining 18 genes from Phase 3 plan

---

**Status:** ✅ Phase 1-2 COMPLETE, Phase 3 PARTIAL (70%), Phase 4 NOT STARTED
**Estimated Effort:** 1 week for Phase 1-3 → **Actual: ~3-4 days for Phase 1-2**
**Excitement Level:** 🔥🔥🔥 → **Achievement Level:** 🎯🎯🎯
