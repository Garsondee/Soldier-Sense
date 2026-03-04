package game

import "math"

const (
	// Default vision parameters.
	defaultFOVDeg   = 120.0              // field of view in degrees
	defaultViewDist = maxFireRange * 4.0 // detect threats at ~2x long-range shot distance
)

// VisionState tracks a soldier's current look direction and what they see.
type VisionState struct {
	// KnownContacts are soldiers this agent can currently see.
	KnownContacts []*Soldier
	inConeScratch map[*Soldier]bool

	Heading  float64 // radians, 0 = right, pi/2 = down
	FOV      float64 // radians, total arc width
	MaxRange float64 // pixels
}

// NewVisionState creates a vision state with defaults.
// InitialHeading in radians.
func NewVisionState(initialHeading float64) VisionState {
	return VisionState{
		Heading:       initialHeading,
		FOV:           defaultFOVDeg * math.Pi / 180.0,
		MaxRange:      defaultViewDist,
		inConeScratch: make(map[*Soldier]bool),
	}
}

func (v *VisionState) scratchInConeMap() map[*Soldier]bool {
	if v.inConeScratch == nil {
		v.inConeScratch = make(map[*Soldier]bool)
	}
	clear(v.inConeScratch)
	return v.inConeScratch
}

// InCone returns true if the point (px,py) is within the vision cone
// of an observer at (ox,oy).
func (v *VisionState) InCone(ox, oy, px, py float64) bool {
	dx := px - ox
	dy := py - oy
	// Quick check: is the target within max range? Uses squared distance to avoid sqrt.
	dist2 := dx*dx + dy*dy
	maxRange2 := v.MaxRange * v.MaxRange
	// 1e-12 is the square of 1e-6, a small distance to prevent treating near-zero distances as in-cone.
	if dist2 > maxRange2 || dist2 < 1e-12 {
		return false
	}

	angleToTarget := math.Atan2(dy, dx)
	diff := normalizeAngle(angleToTarget - v.Heading)
	halfFOV := v.FOV / 2.0
	return diff >= -halfFOV && diff <= halfFOV
}

// UpdateHeading smoothly rotates the heading toward a target angle.
// TurnRate is radians per tick.
func (v *VisionState) UpdateHeading(targetAngle, turnRate float64) {
	diff := normalizeAngle(targetAngle - v.Heading)
	switch {
	case math.Abs(diff) <= turnRate:
		v.Heading = targetAngle
	case diff > 0:
		v.Heading = normalizeAngle(v.Heading + turnRate)
	default:
		v.Heading = normalizeAngle(v.Heading - turnRate)
	}
}

// HeadingTo returns the angle in radians from (ox,oy) toward (tx,ty).
func HeadingTo(ox, oy, tx, ty float64) float64 {
	return math.Atan2(ty-oy, tx-ox)
}

// normalizeAngle wraps an angle to [-pi, pi].
func normalizeAngle(a float64) float64 {
	return math.Remainder(a, 2*math.Pi)
}

// PerformVisionScan uses the spotting accumulator system to gradually detect targets.
// Phase 1 implementation - replaces binary detection with time-weighted confidence build-up.
func (v *VisionState) PerformVisionScan(ox, oy float64, observer *Soldier, candidates []*Soldier, buildings []rect, covers []*CoverObject, threats *[]ThreatFact, currentTick int) { //nolint:gocognit,gocyclo
	const spottingThreshold = 0.85
	const ticksPerSecond = 60.0

	// Clear known contacts - will be rebuilt from confirmed threats
	v.KnownContacts = v.KnownContacts[:0]

	// Mark all existing threats as not in cone this tick.
	inConeThisTick := v.scratchInConeMap()

	for _, c := range candidates {
		if c.state == SoldierStateDead {
			continue
		}

		// Check if target is in vision cone
		if !v.InCone(ox, oy, c.x, c.y) {
			continue
		}

		// Check line of sight
		if !HasLineOfSightWithCover(ox, oy, c.x, c.y, buildings, covers) {
			continue
		}

		inConeThisTick[c] = true

		// Find existing threat or create new one
		var threat *ThreatFact
		for i := range *threats {
			if (*threats)[i].Source == c {
				threat = &(*threats)[i]
				break
			}
		}

		if threat == nil {
			// Create new threat entry
			*threats = append(*threats, ThreatFact{
				Source:              c,
				X:                   c.x,
				Y:                   c.y,
				Confidence:          0.0,
				LastTick:            currentTick,
				IsVisible:           false,
				SpottingAccumulator: 0.0,
			})
			threat = &(*threats)[len(*threats)-1]
		}

		// Calculate concealment and spotting power
		isMoving := c.dashOverwatchTimer > 0 || c.blackboard.CurrentGoal == GoalAdvance ||
			c.blackboard.CurrentGoal == GoalMoveToContact || c.blackboard.CurrentGoal == GoalFlank
		concealment := CalculateConcealmentScore(c, isMoving)

		targetAngle := math.Atan2(c.y-oy, c.x-ox)
		spottingPower := CalculateSpottingPower(observer, targetAngle)

		// Calculate distance falloff
		dx := c.x - ox
		dy := c.y - oy
		dist := math.Sqrt(dx*dx + dy*dy)
		effectiveRange := v.DegradeRange(observer.profile.Physical.Fatigue, observer.profile.Skills.TacticalAwareness, observer.profile.Survival.SituationalAwareness)
		distanceFactor := math.Max(0.15, 1.0-(dist/effectiveRange)*0.85)

		// Accumulate detection progress
		delta := spottingPower * distanceFactor * (1.0 - concealment) * (1.0 / ticksPerSecond)
		threat.SpottingAccumulator += delta
		if threat.SpottingAccumulator > 1.0 {
			threat.SpottingAccumulator = 1.0
		}

		// Check for confirmation
		if threat.SpottingAccumulator >= spottingThreshold {
			// Target confirmed - add to known contacts
			v.KnownContacts = append(v.KnownContacts, c)
			threat.IsVisible = true
			threat.Confidence = 1.0
			threat.X = c.x
			threat.Y = c.y
			threat.LastTick = currentTick
		}
	}

	// Decay accumulator for threats not in cone this tick
	for i := range *threats {
		threat := &(*threats)[i]
		if threat.Source != nil && !inConeThisTick[threat.Source] {
			// Target not in cone - decay accumulator
			lastConcealment := 0.5 // Estimate
			if threat.Source.state != SoldierStateDead {
				isMoving := threat.Source.dashOverwatchTimer > 0 ||
					threat.Source.blackboard.CurrentGoal == GoalAdvance ||
					threat.Source.blackboard.CurrentGoal == GoalMoveToContact ||
					threat.Source.blackboard.CurrentGoal == GoalFlank
				lastConcealment = CalculateConcealmentScore(threat.Source, isMoving)
			}
			decayRate := 0.015 * (1.0 - lastConcealment)
			threat.SpottingAccumulator = math.Max(0, threat.SpottingAccumulator-decayRate)

			// LKP confidence decay
			if !threat.IsVisible && threat.Confidence > 0 {
				fadeRate := 0.008
				age := float64(currentTick - threat.LastTick)
				threat.Confidence = math.Max(0, threat.Confidence-age*fadeRate)
			}
		}
	}

	// Remove dead threats
	kept := (*threats)[:0]
	for i := range *threats {
		t := &(*threats)[i]
		if t.Source != nil && t.Source.state == SoldierStateDead {
			continue
		}
		if t.Confidence <= 0 && t.SpottingAccumulator <= 0 {
			continue
		}
		kept = append(kept, *t)
	}
	*threats = kept
}

// DegradeRange reduces effective vision range based on fatigue and awareness.
// Phase 1: TacticalAwareness and SituationalAwareness extend vision range.
func (v *VisionState) DegradeRange(fatigue, tacticalAwareness, situationalAwareness float64) float64 {
	baseRange := v.MaxRange * (1.0 - fatigue*0.3)
	tacticalBonus := 1.0 + (tacticalAwareness * 0.2)
	situationalBonus := 1.0 + (situationalAwareness * 0.25)
	return baseRange * tacticalBonus * situationalBonus
}

// CalculateConcealmentScore computes how hard a target is to spot (0-1, higher = harder).
func CalculateConcealmentScore(target *Soldier, isMoving bool) float64 {
	// Base concealment from stance
	profileMul := target.profile.Stance.Profile().ProfileMul
	var stanceBase float64
	switch {
	case profileMul >= 1.0: // Standing
		stanceBase = 0.10
	case profileMul >= 0.6: // Crouching
		stanceBase = 0.45
	default: // Prone
		stanceBase = 0.78
	}

	// Movement multiplier
	var movementMul float64
	switch {
	case target.dashOverwatchTimer > 0:
		movementMul = 0.35 // Sprinting
	case isMoving:
		switch target.profile.Stance {
		case StanceProne:
			movementMul = 1.25 // Prone crawl
		case StanceCrouching:
			movementMul = 0.85 // Moving crouched
		default:
			movementMul = 0.70 // Walking
		}
	default:
		// Stationary
		switch target.profile.Stance {
		case StanceProne:
			movementMul = 1.45 // Stationary prone
		default:
			movementMul = 1.15 // Stationary standing/crouching
		}
	}

	concealment := stanceBase * movementMul
	if concealment < 0.05 {
		concealment = 0.05
	}
	if concealment > 0.98 {
		concealment = 0.98
	}
	return concealment
}

// CalculateSpottingPower computes observer's detection capability.
func CalculateSpottingPower(observer *Soldier, targetAngle float64) float64 {
	tactical := observer.profile.Skills.TacticalAwareness
	situational := observer.profile.Survival.SituationalAwareness
	basePower := 0.35 + tactical*0.85 + situational*0.80

	// Angle penalty - targets at edge of FOV are harder to spot
	angleDiff := math.Abs(normalizeAngle(targetAngle - observer.vision.Heading))
	halfFOV := observer.vision.FOV / 2.0
	angleRatio := angleDiff / halfFOV
	anglePenalty := 1.0 - angleRatio*0.35

	return basePower * anglePenalty
}
