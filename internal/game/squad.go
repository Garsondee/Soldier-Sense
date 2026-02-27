package game

import "math"

// Squad groups soldiers under a leader.
type Squad struct {
	ID        int
	Team      Team
	Leader    *Soldier
	Members   []*Soldier
	Formation FormationType

	// smoothedHeading is a low-pass filtered version of the leader's heading,
	// used to prevent formation thrash on minor direction changes.
	smoothedHeading float64
	headingInit     bool
}

// NewSquad creates a squad. The first member is designated leader.
func NewSquad(id int, team Team, members []*Soldier) *Squad {
	sq := &Squad{
		ID:        id,
		Team:      team,
		Members:   members,
		Formation: FormationWedge,
	}
	if len(members) > 0 {
		sq.Leader = members[0]
		sq.Leader.isLeader = true
	}
	for i, m := range members {
		m.squad = sq
		if i > 0 {
			m.slotIndex = i
			m.formationMember = true
			// Remove the fixed bounce patrol from non-leaders;
			// they will follow formation slot targets instead.
			m.path = nil
			m.pathIndex = 0
		}
	}
	return sq
}

// UpdateFormation computes world-space slot positions and triggers repath
// for any member whose slot has drifted beyond the threshold.
func (sq *Squad) UpdateFormation() {
	if sq.Leader == nil || sq.Leader.state == SoldierStateDead {
		return
	}
	lx, ly := sq.Leader.x, sq.Leader.y

	// Smooth the leader's heading to dampen formation jitter.
	leaderH := sq.Leader.vision.Heading
	if !sq.headingInit {
		sq.smoothedHeading = leaderH
		sq.headingInit = true
	} else {
		// Exponential moving average — alpha 0.05 ≈ ~20 tick time constant.
		diff := normalizeAngle(leaderH - sq.smoothedHeading)
		sq.smoothedHeading = normalizeAngle(sq.smoothedHeading + diff*0.05)
	}

	offsets := formationOffsets(sq.Formation, len(sq.Members))

	for i, m := range sq.Members {
		if i == 0 || !m.formationMember || m.state == SoldierStateDead {
			continue
		}
		off := offsets[i]
		wx, wy := SlotWorld(lx, ly, sq.smoothedHeading, off[0], off[1])

		// Only repath if slot has moved meaningfully or we have no path.
		dx := wx - m.slotTargetX
		dy := wy - m.slotTargetY
		slotDrift := math.Sqrt(dx*dx + dy*dy)

		if m.path == nil || slotDrift > repathThreshold {
			m.slotTargetX = wx
			m.slotTargetY = wy
			newPath := m.navGrid.FindPath(m.x, m.y, wx, wy)
			if newPath != nil {
				m.path = newPath
				m.pathIndex = 0
			}
		}
	}
}

// Alive returns members that are not incapacitated.
func (sq *Squad) Alive() []*Soldier {
	var alive []*Soldier
	for _, m := range sq.Members {
		if m.state != SoldierStateDead {
			alive = append(alive, m)
		}
	}
	return alive
}

// CasualtyCount returns how many squad members are dead or incapacitated.
func (sq *Squad) CasualtyCount() int {
	count := 0
	for _, m := range sq.Members {
		if m.state == SoldierStateDead {
			count++
		}
	}
	return count
}

// LeaderPosition returns the leader's current position, or the squad
// centroid if the leader is down.
func (sq *Squad) LeaderPosition() (float64, float64) {
	if sq.Leader != nil && sq.Leader.state != SoldierStateDead {
		return sq.Leader.x, sq.Leader.y
	}
	// Fallback: centroid of alive members.
	alive := sq.Alive()
	if len(alive) == 0 {
		return 0, 0
	}
	var sx, sy float64
	for _, m := range alive {
		sx += m.x
		sy += m.y
	}
	n := float64(len(alive))
	return sx / n, sy / n
}
