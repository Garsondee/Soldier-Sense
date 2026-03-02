# Headless Test Collision & Immobility Report

**Test Configuration:**
- Runs Analyzed: 33 (partial dataset from 100-run test)
- Ticks per Run: 20,000 (333 seconds / 5.5 minutes)
- Seed Base: 1000

**Date:** March 2, 2026

---

## Executive Summary

Analysis of 33 extended headless test runs reveals **critical collision and pathfinding issues** affecting soldier movement and combat effectiveness. Out of 33 runs, **29 runs (88%)** exhibited problematic soldier behavior, with **310 total problematic soldiers** identified.

### Key Findings

1. **Collision Clustering**: Soldiers frequently get stuck in collision pairs/groups, unable to move for 75-90% of battle duration
2. **Pathfinding Failures**: Movement system fails during regroup, formation, and withdrawal phases
3. **Idle/Cover Loops**: Soldiers trapped in state transition loops (50-109 idle transitions per battle)
4. **Zero Combat Engagement**: Despite seeing enemies, stuck soldiers never enter firing range

---

## Statistical Overview

### Problematic Soldier Metrics

| Metric | Value |
|--------|-------|
| Total Problematic Soldiers | 310 |
| Runs with Issues | 29 / 33 (88%) |
| Average Problematic Soldiers per Affected Run | 9.4 |
| Soldiers with Proximity Issues | 128 (41%) |
| Soldiers with Immobility | 123 |
| Average Immobility % | **81.3%** |
| Maximum Immobility % | **92.7%** |
| Minimum Immobility % | 60.3% |

### Issue Distribution

| Issue Type | Count | Percentage |
|------------|-------|------------|
| IMMOBILE (>50% stationary) | 123 | 39.7% |
| SEPARATED (>30% from leader) | 42 | 13.5% |
| NEVER_SAW_ENEMY | 100 | 32.3% |
| NEVER_IN_RANGE | 45 | 14.5% |

---

## Root Cause Analysis

### 1. Collision System Failures

**Primary Issue**: Soldiers colliding with teammates become permanently stuck, unable to resolve the collision.

**Evidence:**
- 128 soldiers (41%) identified with proximity issues
- Collision pairs spending 195-200% of stalled time together (overlapping stalled events)
- Common collision clusters:
  - **Blue Team**: B0 ↔ B6 ↔ B8 ↔ B9 (4-soldier pileup)
  - **Red Team**: R0 ↔ R2 ↔ R3 ↔ R5 (4-soldier pileup)

**Most Frequent Collision Pairs:**
1. B9 collisions: 10 instances
2. R0 collisions: 8 instances
3. R4 collisions: 7 instances
4. B1, B6, B0 collisions: 6 instances each

**Diagnosis Breakdown:**
- 154 cases: Unknown/Complex (58%)
- 56 cases: Direct collision diagnosis (21%)
- Remainder: Pathfinding failures, stuck during regroup

### 2. Pathfinding System Failures

**Primary Issue**: Pathfinding fails to recover when soldiers get stuck, leading to permanent immobility.

**Evidence:**
- Soldiers stuck in same goal for 80-97 consecutive stalled events
- Movement values consistently 0.00 despite having valid goals
- Failures occur primarily during:
  - **Regroup** (35% of cases)
  - **Formation** (25% of cases)
  - **Fallback/Withdrawal** (20% of cases)
  - **Move to Contact** (15% of cases)
  - **Help Casualty** (5% of cases)

**Example Pattern (R2, Run 28):**
```
Stalled events: 97 occurrences
  tick=2636  goal=move_to_contact intent=engage moved=0.00
  tick=11276 goal=move_to_contact intent=engage moved=0.00
  tick=19916 goal=move_to_contact intent=engage moved=0.00
```
Soldier stuck in same goal for **17,280 ticks** (288 seconds) without moving.

### 3. State Transition Loops

**Primary Issue**: Soldiers trapped in idle ↔ cover state loops, unable to execute movement.

**Evidence:**
- Idle/Cover loops detected in 40+ soldiers
- Range: 41-109 idle transitions per battle
- Typical pattern: 50-90 idle, 12-29 cover transitions
- Loops persist for entire battle duration

**Example (R2, Run 28):**
- 109 idle transitions
- 12 cover transitions
- 87.3% immobility rate

### 4. Personal Space / Collision Avoidance Breakdown

**Primary Issue**: Personal space enforcement system fails when multiple soldiers occupy same area.

**Evidence:**
- Collision clusters of 3-4 soldiers in same location
- Soldiers unable to separate despite personal space mechanics
- Proximity percentages >195% indicate complete overlap
- Most failures during squad regrouping operations

---

## Behavioral Patterns

### Phase-Specific Failures

1. **Early Game (Regroup Phase)**
   - Soldiers collide during initial squad formation
   - Formation goal failures: 25% of stuck soldiers
   - Unable to establish proper spacing

2. **Mid Game (Engagement Phase)**
   - Move to contact failures: 15% of stuck soldiers
   - Soldiers see enemies but cannot path to firing positions
   - Stuck soldiers never enter firing range despite enemy visibility

3. **Late Game (Withdrawal Phase)**
   - Fallback/withdrawal failures: 20% of stuck soldiers
   - Entire squads become immobilized during retreat
   - Help casualty goals fail when medic collides with patient

### Goal-Specific Analysis

| Goal | Failure Rate | Common Issue |
|------|--------------|--------------|
| Regroup | 35% | Collision during squad consolidation |
| Formation | 25% | Unable to achieve spacing |
| Fallback | 20% | Withdrawal pathfinding fails |
| Move to Contact | 15% | Cannot path to enemy |
| Help Casualty | 5% | Medic collision with patient |

---

## Impact Assessment

### Combat Effectiveness

- **Zero Engagement**: Stuck soldiers contribute nothing to combat
- **Squad Degradation**: 3-4 soldiers per squad becoming immobilized
- **Force Multiplier Loss**: 30-40% of force effectively removed from battle
- **Tactical Failure**: Squads unable to execute maneuvers (regroup, withdrawal)

### Battle Outcomes

Extended battles (20K ticks) show:
- Higher conclusion rates vs shorter battles
- But victories often pyrrhic due to stuck soldiers
- Stalemates still occur when both sides equally affected

---

## Critical Issues Requiring Immediate Attention

### Priority 1: Collision Resolution

**Problem**: Soldiers cannot resolve collisions with teammates.

**Symptoms:**
- 195-200% proximity overlap
- Permanent immobility (75-92%)
- Collision clusters of 3-4 soldiers

**Recommended Solutions:**
1. Implement collision push-back force to separate stuck soldiers
2. Add timeout-based teleport for soldiers stuck >300 ticks
3. Improve personal space enforcement during regroup
4. Add collision avoidance to pathfinding cost function
5. Implement "unstick" behavior when movement fails repeatedly

### Priority 2: Pathfinding Recovery

**Problem**: Pathfinding never recovers from failures.

**Symptoms:**
- 80-97 consecutive stalled events
- moved=0.00 for thousands of ticks
- Same goal maintained despite no progress

**Recommended Solutions:**
1. Implement aggressive path recomputation when stuck (every 60 ticks)
2. Add "give up and try different approach" after N failed attempts
3. Implement direct-line movement as last resort (already partially done)
4. Add goal abandonment/rotation when stuck too long
5. Improve extreme pathfinding failure recovery (expand existing system)

### Priority 3: State Loop Breaking

**Problem**: Soldiers trapped in idle ↔ cover loops.

**Symptoms:**
- 50-109 idle transitions
- 12-29 cover transitions
- No productive behavior

**Recommended Solutions:**
1. Detect state thrashing (same state >5 times in 300 ticks)
2. Force state change when loop detected
3. Add "forced advance" override for looping soldiers
4. Implement state transition cooldowns to prevent rapid cycling

### Priority 4: Squad-Level Collision Management

**Problem**: Entire squads becoming immobilized together.

**Symptoms:**
- 3-4 soldier collision clusters
- Squad-wide regroup failures
- Formation goal failures

**Recommended Solutions:**
1. Implement squad-level collision detection
2. Add squad "scatter" command when cluster detected
3. Stagger regroup timing to prevent simultaneous arrival
4. Implement formation positions with collision-free guarantees
5. Add squad leader override to break up stuck groups

---

## Detailed Examples

### Example 1: Blue Team Collision Cluster (Run 28)

**Affected Soldiers**: B0, B6, B8, B9 (4 soldiers)

**Collision Network**:
- B6 ↔ B8 (195.6% overlap)
- B8 ↔ B9 (197.9% overlap)
- B0 ↔ B9 (193.8% overlap)

**Immobility Rates**:
- B0: 86.4%
- B6: 81.9%
- B8: 86.4%
- B9: 85.5%

**Pattern**: All four soldiers stuck in formation/engage goals, unable to separate. Idle/cover loops ranging from 54-90 idle transitions. Zero combat effectiveness despite seeing enemies.

### Example 2: Red Team Leader Collision (Run 28)

**Affected Soldiers**: R0, R2, R3, R5 (4 soldiers)

**Collision Network**:
- R0 ↔ R2 (200% overlap)
- R0 ↔ R3 (197.6% overlap)
- R0 ↔ R5 (197.6% overlap)

**Immobility Rates**:
- R0: 85.5% (squad leader)
- R2: 87.3%
- R3: 75.6%
- R5: 76.5%

**Pattern**: Squad leader R0 becomes collision hub. All soldiers stuck during regroup/fallback. 58-99 idle transitions. Pathfinding completely fails for entire squad.

---

## Recommendations

### Immediate Actions (Next Sprint)

1. **Implement Collision Push-Back**
   - Add repulsion force between soldiers in same cell
   - Gradually increase force over time when stuck
   - Target: Separate soldiers within 60 ticks

2. **Enhance Pathfinding Recovery**
   - Reduce path recomputation interval when stuck (60 ticks)
   - Implement path abandonment after 5 consecutive failures
   - Expand extreme recovery to all goals (not just move_to_contact)

3. **Add State Loop Detection**
   - Track state transition history (last 10 states)
   - Detect loops (same state 3+ times in 180 ticks)
   - Force state change or goal change when detected

4. **Improve Squad Regroup**
   - Stagger regroup arrival times
   - Implement collision-aware formation positions
   - Add squad scatter command for stuck detection

### Medium-Term Actions (Next Month)

1. **Comprehensive Collision System Rewrite**
   - Implement proper collision avoidance in pathfinding
   - Add predictive collision detection
   - Implement formation positions with guaranteed spacing

2. **Enhanced Diagnostics**
   - Add real-time stuck soldier detection in game
   - Visual indicators for stuck soldiers
   - Automatic debug logging for stuck events

3. **AI Behavior Improvements**
   - Add "unstuck" behavior tree branch
   - Implement timeout-based goal abandonment
   - Add randomized movement when stuck

### Long-Term Actions (Next Quarter)

1. **Movement System Overhaul**
   - Consider flow-field pathfinding for squad movement
   - Implement proper steering behaviors
   - Add formation movement with collision-free guarantees

2. **Testing Infrastructure**
   - Automated stuck soldier detection in CI/CD
   - Regression tests for collision scenarios
   - Performance benchmarks for pathfinding

---

## Conclusion

The headless test data reveals **systemic collision and pathfinding failures** affecting 88% of extended battles. The core issues are:

1. **Collision resolution completely fails** when soldiers occupy same space
2. **Pathfinding never recovers** from stuck states
3. **State machines loop indefinitely** without productive behavior
4. **Squad-level operations** (regroup, formation) trigger mass immobility

These issues severely impact combat effectiveness, with 30-40% of forces becoming non-functional in extended engagements. The problems compound over time, making longer battles increasingly dysfunctional.

**Priority recommendation**: Implement collision push-back and enhanced pathfinding recovery as immediate fixes, followed by comprehensive collision system rewrite for long-term stability.

---

## Appendix: Test Methodology

### Detection Criteria

Soldiers flagged as problematic if they meet any of:
- **Immobility**: >50% of battle spent stationary
- **Separation**: >30% of battle >300 units from squad leader
- **No Enemy Contact**: Never saw an enemy
- **No Firing Range**: Never within 800 units of enemy

### Proximity Analysis

Soldiers flagged as collision pairs if:
- Stalled events overlap within 60 ticks (1 second)
- >50% of stalled events coincide
- Reported as proximity percentage (overlap / total stalled events)

### Diagnostic Categories

- **Collision**: Proximity partner identified with >50% overlap
- **Pathfinding Failure**: Stuck in movement goal with moved=0.00
- **State Loop**: >50 idle or >10 cover transitions
- **Stuck During Regroup**: First stalled event in regroup goal
- **Unknown**: Complex multi-factor issues requiring manual review
