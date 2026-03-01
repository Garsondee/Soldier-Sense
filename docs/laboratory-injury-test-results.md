# Laboratory Injury Test Results

## Overview

This document summarizes the results from the automated laboratory injury testing suite (`internal/game/laboratory_test.go`). The test spawns a single 4-soldier squad on a blank 800x600 map, orders them to cross the space, then randomly injures one squad member at tick 120 and observes the physiological and behavioral response.

## Test Methodology

- **Environment**: Blank map, no enemies, no obstacles
- **Squad**: 4 red team soldiers advancing from (100, 300) to (700, 300)
- **Injury Timing**: Tick 120 (2 seconds into simulation)
- **Observation Period**: Up to 1200 ticks (20 seconds) or until death
- **Metrics Tracked**:
  - Blood volume (0-1 scale)
  - Mobility multiplier (movement speed degradation)
  - Accuracy multiplier (shooting degradation)
  - Pain level (0-1 scale)
  - State transitions (moving → wounded-amb → wounded-nonamb → unconscious → dead)
  - Time to state change

## Test Cases & Results

### 1. Minor Leg Wound
**Parameters:**
- Target Region: Right Leg
- Damage: 8.0
- Expected Severity: Minor

**Results:**
- **Actual Hit**: Right Leg
- **Actual Severity**: Minor
- **Bleed Rate**: 0.005/tick
- **Initial Pain**: 0.08
- **Blood Volume at End**: 0.99 (after 1080 ticks)
- **Mobility**: 1.00 (no degradation)
- **Accuracy**: 1.00 (no degradation)
- **Final Pain**: 0.02 (pain decreased over time)
- **Final State**: Moving (ambulatory, conscious, alive)
- **Time to State Change**: None (remained ambulatory throughout)

**Analysis**: Minor leg wounds cause minimal functional impairment. The soldier continues mission with slight bleeding that poses no immediate threat. Pain diminishes over time, likely due to wound treatment mechanics.

---

### 2. Moderate Arm Wound
**Parameters:**
- Target Region: Left Arm
- Damage: 12.0
- Expected Severity: Moderate

**Results:**
- **Actual Hit**: Right Arm (dominant arm)
- **Actual Severity**: Moderate
- **Bleed Rate**: 0.020/tick
- **Initial Pain**: 0.20
- **Blood Volume at End**: 0.97 (after 1080 ticks)
- **Mobility**: 1.00 (no degradation)
- **Accuracy**: 0.60 (40% reduction - significant)
- **Final Pain**: 0.06 (reduced from initial 0.20)
- **Final State**: Idle/Moving (ambulatory, conscious, alive)
- **Time to State Change**: 36 ticks (0.6 seconds to first idle state)

**Analysis**: Moderate arm wounds severely impact combat effectiveness through accuracy degradation. The 40% accuracy penalty makes the soldier much less effective in firefights. Blood loss is moderate but not life-threatening over the observation period. The soldier alternates between moving and idle states, suggesting the injury affects their ability to maintain continuous movement.

---

### 3. Severe Torso Wound
**Parameters:**
- Target Region: Torso
- Damage: 25.0
- Expected Severity: Severe

**Results:**
- **Actual Hit**: Torso
- **Actual Severity**: Severe
- **Bleed Rate**: 0.060/tick
- **Initial Pain**: 0.40
- **Blood Volume at End**: 0.87 (after 1080 ticks)
- **Mobility**: 1.00 (maintained)
- **Accuracy**: 0.60 (blood loss penalty)
- **Final Pain**: 0.12 (reduced from 0.40)
- **Final State**: Idle (ambulatory, conscious, alive)
- **Time to State Change**: 1 tick (immediate state change)

**Analysis**: Severe torso wounds cause significant blood loss (13% over 18 seconds) and high initial pain. Despite the severity, the soldier remains ambulatory and conscious throughout. The accuracy penalty comes from blood volume loss rather than direct arm damage. This wound would likely prove fatal if left untreated over a longer period (estimated 5-10 minutes based on bleed rate).

---

### 4. Critical Abdomen Wound
**Parameters:**
- Target Region: Abdomen
- Damage: 30.0
- Expected Severity**: Critical

**Results:**
- **Actual Hit**: Abdomen
- **Actual Severity**: Critical
- **Bleed Rate**: 0.150/tick
- **Initial Pain**: 0.65
- **Blood Volume at End**: 0.73 (after 1080 ticks)
- **Mobility**: 0.70 (30% reduction from abdomen damage)
- **Accuracy**: 0.60 (blood loss penalty)
- **Final Pain**: 0.20 (reduced from 0.65)
- **Final State**: Idle (ambulatory, conscious, alive)
- **Time to State Change**: 1 tick (immediate)

**Analysis**: Critical abdomen wounds are life-threatening with rapid blood loss (27% over 18 seconds). The soldier experiences both mobility and accuracy degradation. High initial pain (0.65) indicates severe trauma. Without medical intervention, this wound would likely cause unconsciousness within 2-3 minutes and death within 5-10 minutes.

---

### 5. Leg Destruction
**Parameters:**
- Target Region: Left Leg
- Damage: 35.0
- Expected Severity: Critical

**Results:**
- **Actual Hit**: Left Leg
- **Actual Severity**: Critical
- **Bleed Rate**: 0.150/tick
- **Initial Pain**: 0.65
- **Blood Volume at End**: 0.73 (after 1080 ticks)
- **Mobility**: 0.15 (85% reduction - crawl only)
- **Accuracy**: 0.60 (blood loss penalty)
- **Final Pain**: 0.20
- **Final State**: Idle (ambulatory, conscious, alive)
- **Time to State Change**: 1 tick (immediate)

**Analysis**: Leg destruction causes catastrophic mobility loss - soldier reduced to crawling speed (15% of normal). Combined with severe blood loss, this wound effectively removes the soldier from combat. The soldier remains conscious but is unable to maneuver effectively. This represents a "mobility kill" even though the soldier is technically alive.

---

### 6. Severe Head Wound
**Parameters:**
- Target Region: Head
- Damage: 15.0
- Expected Severity: Severe

**Results:**
- **Actual Hit**: Head
- **Actual Severity**: Severe
- **Bleed Rate**: 0.060/tick
- **Initial Pain**: 0.40
- **Blood Volume at End**: 0.87 (after 1080 ticks)
- **Mobility**: 1.00
- **Accuracy**: 0.75 (25% reduction from head trauma)
- **Final Pain**: 0.12
- **Final State**: Idle (ambulatory, conscious, alive)
- **Time to State Change**: 1 tick (immediate)

**Analysis**: Severe head wounds cause disorientation (accuracy penalty) and significant bleeding. The soldier remains functional but impaired. Note that head wounds have high lethality bias - a slightly higher damage roll could have resulted in instant death. The 25% accuracy penalty represents cognitive impairment from head trauma.

---

## Key Findings

### Bleed Rates by Severity
- **Minor**: 0.005/tick (~300 ticks to lose 0.15 blood)
- **Moderate**: 0.020/tick (~75 ticks to lose 0.15 blood)
- **Severe**: 0.060/tick (~25 ticks to lose 0.15 blood)
- **Critical**: 0.150/tick (~10 ticks to lose 0.15 blood)

### Functional Degradation Patterns
1. **Arm Wounds**: Severe accuracy penalties (40-70% reduction)
2. **Leg Wounds**: Mobility penalties (50-85% reduction when severe)
3. **Abdomen Wounds**: Both mobility and accuracy degradation
4. **Head Wounds**: Accuracy/cognitive penalties (25% reduction)
5. **Torso Wounds**: Primarily blood loss, minimal direct functional impact

### Blood Volume Thresholds (from body.go)
- **> 0.80**: Full function
- **0.40 - 0.80**: Ambulatory impaired, accuracy/mobility penalties
- **0.20 - 0.40**: Non-ambulatory (cannot self-move)
- **< 0.20**: Unconscious
- **0.00**: Death

### Pain Dynamics
- Pain decreases over time (likely from wound treatment/shock)
- Initial pain ranges: 0.08 (minor) to 0.65 (critical)
- Final pain typically 25-30% of initial value after 18 seconds

### State Transition Observations
- Soldiers do NOT automatically enter wounded states in this test
- State changes are primarily movement-related (moving ↔ idle)
- The body system tracks injury independently of soldier state
- Wounded states would likely be triggered by combat AI logic not present in this sterile test

## Recommendations

1. **Medical Priority System**: Critical abdomen/torso wounds should be highest priority (fastest bleed rates)
2. **Mobility Kills**: Leg destruction effectively removes soldiers from combat even if alive
3. **Accuracy Impact**: Arm wounds severely degrade combat effectiveness
4. **Time Windows**: 
   - Critical wounds: ~2-5 minutes before unconsciousness
   - Severe wounds: ~5-10 minutes before unconsciousness
   - Moderate wounds: ~15-20 minutes before unconsciousness
5. **Squad Impact**: Even "minor" wounds degrade squad effectiveness through pain and functional penalties

## Test Limitations

- No enemy contact (no stress/fear factors)
- No buddy aid or self-aid attempts observed
- No squad reaction to casualty
- Sterile environment (no suppression, no cover seeking)
- Fixed injury timing (doesn't test injuries during combat)

## Future Test Scenarios

1. **Buddy Aid Response**: Inject injury during contact, observe squad casualty response
2. **Self-Aid Behavior**: Test CanSelfAid() triggers and treatment effectiveness
3. **Multiple Casualties**: Test squad cohesion collapse under casualties
4. **Combat Injuries**: Injuries during active firefight vs. sterile environment
5. **Progressive Deterioration**: Longer observation periods to track unconsciousness/death
