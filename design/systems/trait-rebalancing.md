# Trait Rebalancing Analysis

**Status:** Critical — Evolutionary results expose fundamental mechanical gaps
**Priority:** High — Multiple traits provide no meaningful combat advantage

---

## Overview

The evolutionary optimization revealed several traits that were minimized to their lower bounds or eliminated entirely, indicating they provide insufficient mechanical advantage to justify their cost. This document analyzes each problematic trait and proposes concrete fixes.

---

## Problematic Traits

### 1. Aggression (Evolved to 0.0) - **CRITICALLY UNDERVALUED**

**Current Usage Pattern:**
```
Engage:        +10% utility (×0.10 coefficient)
MoveToContact: +15% utility (×0.15 coefficient) 
Flank:         +12-18% utility (×0.12-0.18 coefficient)
Advance:       +8-10% utility (×0.08-0.10 coefficient)
```

**Root Cause:** Aggression coefficients are **systematically weaker** than competing traits:
- `ShootDesire` provides ×0.45 to Engage (4.5× stronger than Aggression)
- `MoveDesire` provides ×0.30 to MoveToContact (2× stronger than Aggression)
- `Discipline` provides ×0.15 to Engage (1.5× stronger than Aggression)

**Fix Strategy:** Aggression needs **unique mechanics** that other traits cannot provide:

1. **Fire Rate Bonus:** Aggressive soldiers fire faster in sustained combat
   ```go
   burstCooldown *= (1.0 - profile.Personality.Aggression * 0.25) // Up to 25% faster fire
   ```

2. **Suppression Resistance:** Aggressive soldiers maintain offensive action under fire
   ```go
   // In goal utilities, reduce suppression penalties for aggressive soldiers
   engageUtil -= suppress * (0.40 - profile.Personality.Aggression * 0.20)
   ```

3. **Close Combat Bonus:** Aggressive soldiers get accuracy bonus at short range
   ```go
   if dist < closeRange {
       aggressionBonus := profile.Personality.Aggression * 0.15
       accuracy += aggressionBonus
   }
   ```

### 2. Teamwork (Evolved to 0.0) - **NO COMBAT RELEVANCE**

**Current Usage Pattern:**
```
Formation (Advance):  +12% utility (×0.12 coefficient)
Formation (Hold):     +8% utility (×0.08 coefficient) 
Formation (Regroup):  +15% utility (×0.15 coefficient)
```

**Root Cause:** Formation goals have **lowest priority** and are suppressed during combat:
- `visibleThreats > 0` reduces Formation utility by 60-80%
- Combat always takes precedence over formation maintenance

**Fix Strategy:** Teamwork needs **combat-relevant mechanics**:

1. **Coordination Bonus:** High teamwork soldiers coordinate target selection
   ```go
   // When squad members engage the same target, accuracy bonus
   if squadMatesEngagingSameTarget > 0 {
       teamworkBonus := profile.Personality.Teamwork * 0.10 * float64(squadMatesEngagingSameTarget)
       accuracy += teamworkBonus
   }
   ```

2. **Mutual Support:** Teamwork affects buddy aid and cover sharing
   ```go
   buddyAidUtil += profile.Personality.Teamwork * 0.25 // Major boost to helping wounded
   ```

3. **Communication Efficiency:** High teamwork soldiers share intel better
   ```go
   // Reduce radio message garbling for high teamwork soldiers
   messageClarity += profile.Personality.Teamwork * 0.20
   ```

### 3. Initiative (Evolved to 0.08) - **EXTREMELY NARROW SCOPE**

**Current Usage Pattern:**
```
Advance (when IntentHold + non-leader): +15% utility (×0.15 coefficient)
```

**Root Cause:** Single usage case covers <5% of tactical situations.

**Fix Strategy:** Initiative needs **broad independent action mechanics**:

1. **Leadership Vacuum Response:** Initiative soldiers act when leaders are down
   ```go
   if bb.LeaderDown && !isLeader {
       // Initiative soldiers take charge of local tactical decisions
       engageUtil += profile.Personality.Initiative * 0.20
       moveToContactUtil += profile.Personality.Initiative * 0.15
   }
   ```

2. **Opportunity Recognition:** Initiative soldiers exploit tactical openings
   ```go
   if bb.EnemyExposed && bb.FlankingOpportunity {
       flankUtil += profile.Personality.Initiative * 0.30
   }
   ```

3. **Independent Action:** Initiative soldiers act without orders
   ```go
   // When no squad intent is set, initiative soldiers still advance
   if bb.SquadIntent == IntentNone {
       advanceUtil += profile.Personality.Initiative * 0.25
   }
   ```

### 4. Stealth (Evolved to 0.08) - **PURE PENALTY, NO BENEFIT**

**Current Usage Pattern:**
```
Movement Speed: -15% penalty when Stealth is high (×0.15 coefficient)
```

**Root Cause:** Stealth provides **only downsides** (slower movement) with no tactical benefit.

**Fix Strategy:** Stealth needs **concealment and detection mechanics**:

1. **Movement Concealment:** High stealth soldiers are harder to detect when moving
   ```go
   // In CalculateConcealmentScore, add stealth bonus to movement multiplier
   if isMoving {
       stealthBonus := 1.0 + profile.Survival.Stealth * 0.30
       movementMul *= stealthBonus
   }
   ```

2. **Ambush Setup:** Stealth soldiers get first-shot bonus when enemies enter range
   ```go
   if !threat.IsVisible && threat.SpottingAccumulator == 0 && soldierCanSeeTarget {
       // Ambush bonus for stealthy soldiers who spot first
       accuracy += profile.Survival.Stealth * 0.20
   }
   ```

3. **Sound Discipline:** Stealth affects audio signature for enemy detection
   ```go
   // When enemies use BroadcastGunfire, stealth reduces detection radius
   detectionRadius *= (1.0 - profile.Survival.Stealth * 0.25)
   ```

### 5. Fieldcraft (Evolved to 0.20) - **COMPETING WITH STRONGER TRAITS**

**Current Usage Pattern:**
```
Flank:     +35-40% utility (×0.35-0.40 coefficient) — GOOD
Overwatch: +15% utility (×0.15 coefficient)
Search:    +15% utility (×0.15 coefficient)  
Peek:      +25% utility (×0.25 coefficient)
Audio:     +30% triangulation accuracy (×0.30 coefficient)
```

**Root Cause:** Fieldcraft competes with **MoveDesire** and **Discipline** for same behaviors but offers weaker coefficients.

**Fix Strategy:** Fieldcraft needs **unique tactical advantages**:

1. **Terrain Reading:** Fieldcraft soldiers identify better positions
   ```go
   // Fieldcraft improves LocalSightlineScore calculation
   bb.LocalSightlineScore += profile.Skills.Fieldcraft * 0.15
   ```

2. **Enemy Movement Prediction:** Fieldcraft helps anticipate enemy actions
   ```go
   // When enemies break LOS, fieldcraft soldiers retain higher confidence in LKP
   threat.Confidence *= (1.0 + profile.Skills.Fieldcraft * 0.20)
   ```

3. **Environmental Awareness:** Fieldcraft reduces surprise penalties
   ```go
   // Fieldcraft soldiers recover from suppression faster
   suppressionDecay *= (1.0 + profile.Skills.Fieldcraft * 0.25)
   ```

---

## Implementation Priority

### Phase 1: Critical Gaps (Immediate)
1. **Aggression:** Add fire rate bonus and suppression resistance
2. **Teamwork:** Add coordination bonus and buddy aid boost
3. **Stealth:** Add movement concealment mechanics

### Phase 2: Scope Expansion (Next)
4. **Initiative:** Add leadership vacuum and opportunity recognition
5. **Fieldcraft:** Add terrain reading and enemy prediction

### Phase 3: Testing (Validation)
6. Run 10-generation evolution test to verify traits begin climbing from lower bounds
7. Compare fitness results against current baseline

---

## Expected Evolutionary Impact

After fixes, the evolved soldier should show:
- **Aggression:** 0.4-0.6 (balanced offensive/defensive mix)
- **Teamwork:** 0.3-0.5 (meaningful coordination value)  
- **Initiative:** 0.3-0.4 (independent action capability)
- **Stealth:** 0.2-0.4 (situational concealment benefit)
- **Fieldcraft:** 0.4-0.6 (tactical awareness value)

This creates **trait diversity** where different combat roles emerge:
- **Assault specialists:** High Aggression + Discipline
- **Support specialists:** High Teamwork + Fieldcraft  
- **Scout specialists:** High Stealth + Situational Awareness
- **Leader types:** High Initiative + Communication
