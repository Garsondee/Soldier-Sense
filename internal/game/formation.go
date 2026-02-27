package game

import "math"

// FormationType identifies the shape of a squad formation.
type FormationType int

const (
	FormationLine    FormationType = iota // side-by-side perpendicular to heading
	FormationWedge                        // V-shape, leader at point
	FormationColumn                       // single file behind leader
	FormationEchelon                      // diagonal line offset to one flank
)

// slotSpacing is the pixel gap between adjacent formation slots.
const slotSpacing = 28.0

// formationOffsets returns the local (forward, right) offsets for each slot
// in a formation of `count` members (slot 0 is the leader).
// Forward is along the movement direction; right is 90° clockwise.
func formationOffsets(ft FormationType, count int) [][2]float64 {
	offsets := make([][2]float64, count)
	if count == 0 {
		return offsets
	}
	// Slot 0 is always the leader (no offset).
	offsets[0] = [2]float64{0, 0}

	switch ft {
	case FormationLine:
		// All members on the same line perpendicular to heading.
		// Spread symmetrically: ...-2,-1,0,+1,+2,...
		for i := 1; i < count; i++ {
			side := float64((i+1)/2) * slotSpacing
			if i%2 == 1 {
				side = -side
			}
			offsets[i] = [2]float64{0, side} // forward=0, right=±side
		}

	case FormationWedge:
		// Members trail behind and spread outward.
		for i := 1; i < count; i++ {
			depth := float64((i+1)/2) * slotSpacing
			side := float64((i+1)/2) * slotSpacing
			if i%2 == 1 {
				side = -side
			}
			offsets[i] = [2]float64{-depth, side}
		}

	case FormationColumn:
		// Single file directly behind leader.
		for i := 1; i < count; i++ {
			offsets[i] = [2]float64{-float64(i) * slotSpacing, 0}
		}

	case FormationEchelon:
		// Each member is one step back and one step to the right.
		for i := 1; i < count; i++ {
			offsets[i] = [2]float64{-float64(i) * slotSpacing * 0.7, float64(i) * slotSpacing * 0.7}
		}
	}
	return offsets
}

// SlotWorld converts a local (forward, right) offset into a world position
// given the leader's world position and heading.
func SlotWorld(leaderX, leaderY, heading, fwd, right float64) (float64, float64) {
	// Forward unit vector along heading.
	fx := math.Cos(heading)
	fy := math.Sin(heading)
	// Right unit vector (90° clockwise from forward).
	rx := -fy
	ry := fx

	wx := leaderX + fx*fwd + rx*right
	wy := leaderY + fy*fwd + ry*right
	return wx, wy
}

// repathThreshold is how far (pixels) a slot target must drift before
// a soldier will request a new A* path to it.
const repathThreshold = 20.0

// FormationSlot is assigned to one non-leader squad member.
type FormationSlot struct {
	Index     int        // slot index in the formation
	TargetX   float64    // current world-space slot position
	TargetY   float64
}
