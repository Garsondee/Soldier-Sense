package game

import "math"

const (
	// Default vision parameters.
	defaultFOVDeg   = 120.0 // field of view in degrees
	defaultViewDist = 300.0 // max sight range in pixels
)

// VisionState tracks a soldier's current look direction and what they see.
type VisionState struct {
	Heading  float64 // radians, 0 = right, pi/2 = down
	FOV      float64 // radians, total arc width
	MaxRange float64 // pixels

	// KnownContacts are soldiers this agent can currently see.
	KnownContacts []*Soldier
}

// NewVisionState creates a vision state with defaults.
// initialHeading in radians.
func NewVisionState(initialHeading float64) VisionState {
	return VisionState{
		Heading:  initialHeading,
		FOV:      defaultFOVDeg * math.Pi / 180.0,
		MaxRange: defaultViewDist,
	}
}

// InCone returns true if the point (px,py) is within the vision cone
// of an observer at (ox,oy).
func (v *VisionState) InCone(ox, oy, px, py float64) bool {
	dx := px - ox
	dy := py - oy
	dist := math.Sqrt(dx*dx + dy*dy)
	if dist > v.MaxRange || dist < 1e-6 {
		return false
	}

	angleToTarget := math.Atan2(dy, dx)
	diff := normalizeAngle(angleToTarget - v.Heading)
	halfFOV := v.FOV / 2.0
	return diff >= -halfFOV && diff <= halfFOV
}

// UpdateHeading smoothly rotates the heading toward a target angle.
// turnRate is radians per tick.
func (v *VisionState) UpdateHeading(targetAngle float64, turnRate float64) {
	diff := normalizeAngle(targetAngle - v.Heading)
	if math.Abs(diff) <= turnRate {
		v.Heading = targetAngle
	} else if diff > 0 {
		v.Heading = normalizeAngle(v.Heading + turnRate)
	} else {
		v.Heading = normalizeAngle(v.Heading - turnRate)
	}
}

// HeadingTo returns the angle in radians from (ox,oy) toward (tx,ty).
func HeadingTo(ox, oy, tx, ty float64) float64 {
	return math.Atan2(ty-oy, tx-ox)
}

// normalizeAngle wraps an angle to [-pi, pi].
func normalizeAngle(a float64) float64 {
	for a > math.Pi {
		a -= 2 * math.Pi
	}
	for a < -math.Pi {
		a += 2 * math.Pi
	}
	return a
}

// PerformVisionScan clears known contacts and checks all candidates
// for line-of-sight within the vision cone.
func (v *VisionState) PerformVisionScan(ox, oy float64, candidates []*Soldier, buildings []rect) {
	v.KnownContacts = v.KnownContacts[:0]
	for _, c := range candidates {
		if !v.InCone(ox, oy, c.x, c.y) {
			continue
		}
		// Cone check passed — now do hard LOS (building occlusion).
		if HasLineOfSight(ox, oy, c.x, c.y, buildings) {
			v.KnownContacts = append(v.KnownContacts, c)
		}
	}
}

// DegradeRange reduces effective vision range based on fatigue and wounds.
func (v *VisionState) DegradeRange(fatigue float64) float64 {
	return v.MaxRange * (1.0 - fatigue*0.3)
}
