package game

import (
	"fmt"
	"math"
)

// Squad groups soldiers under a leader.
type Squad struct {
	ID        int
	Team      Team
	Leader    *Soldier
	Members   []*Soldier
	Formation FormationType
	Intent    SquadIntentKind

	// smoothedHeading is a low-pass filtered version of the leader's heading,
	// used to prevent formation thrash on minor direction changes.
	smoothedHeading float64
	headingInit     bool
	prevIntent      SquadIntentKind
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

// SquadThink runs the leader's squad-level decision loop.
// It evaluates the leader's blackboard and sets Intent + orders for members.
// intel is the world IntelStore; may be nil (degrades gracefully to blackboard-only).
func (sq *Squad) SquadThink(intel *IntelStore) {
	if sq.Leader == nil || sq.Leader.state == SoldierStateDead {
		// If leader is down, use the most senior alive member as a proxy.
		alive := sq.Alive()
		if len(alive) == 0 {
			return
		}
		sq.Leader = alive[0]
		sq.Leader.isLeader = true
	}

	// Gather contact info across ALL alive members, not just leader.
	// A squad knows what any member can see.
	anyVisibleThreats := 0
	closestDist := math.MaxFloat64
	var contactX, contactY float64
	hasContact := false
	for _, m := range sq.Members {
		if m.state == SoldierStateDead {
			continue
		}
		for _, t := range m.blackboard.Threats {
			if !t.IsVisible {
				continue
			}
			anyVisibleThreats++
			dx := t.X - sq.Leader.x
			dy := t.Y - sq.Leader.y
			d := math.Sqrt(dx*dx + dy*dy)
			if d < closestDist {
				closestDist = d
				contactX = t.X
				contactY = t.Y
				hasContact = true
			}
		}
	}

	// Also include non-visible but high-confidence threats for contact tracking.
	if !hasContact {
		for _, m := range sq.Members {
			if m.state == SoldierStateDead {
				continue
			}
			for _, t := range m.blackboard.Threats {
				if t.Confidence > 0.5 {
					contactX = t.X
					contactY = t.Y
					hasContact = true
					break
				}
			}
			if hasContact {
				break
			}
		}
	}

	spread := sq.squadSpread()

	// --- Intel map queries (augment blackboard counts with spatial data) ---
	// dangerAtPos: how much danger heat is around the leader's current position.
	// contactAhead: how much contact heat is between here and the advance direction.
	// Both default to 0 when intel is not yet available.
	var dangerAtPos, contactAhead float32
	if intel != nil {
		im := intel.For(sq.Team)
		if im != nil {
			lx, ly := sq.Leader.x, sq.Leader.y
			dangerAtPos = im.Layer(IntelDangerZone).SumInRadius(lx, ly, 120)
			contactAhead = im.Layer(IntelContact).SumInRadius(lx, ly, 300)
		}
	}

	// Decide squad intent.
	oldIntent := sq.Intent
	switch {
	// Cohesion emergency: regroup even under contact if spread is extreme.
	case spread > 250:
		sq.Intent = IntentRegroup
	// Active firefight: any member has LOS on a threat close enough to engage.
	case anyVisibleThreats > 0 && closestDist < 320:
		sq.Intent = IntentEngage
	// Heatmap: heavy danger pressure even without current LOS → hold/fallback.
	case dangerAtPos > 1.5 && anyVisibleThreats == 0:
		sq.Intent = IntentHold
	// Distant contact: keep advancing while watching.
	case anyVisibleThreats > 0 && closestDist >= 320:
		sq.Intent = IntentAdvance
	// Heatmap: contact heat ahead but no LOS yet → cautious advance.
	case contactAhead > 0.5 && anyVisibleThreats == 0:
		sq.Intent = IntentAdvance
	// Moderate spread: light regroup nudge but don't abandon a fight.
	case spread > 120 && anyVisibleThreats == 0:
		sq.Intent = IntentRegroup
	default:
		sq.Intent = IntentAdvance
	}

	// Log intent changes.
	if sq.Intent != oldIntent {
		sq.Leader.think(fmt.Sprintf("squad: %s → %s", oldIntent, sq.Intent))
	}

	// Build per-member move orders when engaging.
	// Fan members out around the contact point so they don't all pile on the same cell.
	var moveOrders [][2]float64
	if hasContact && sq.Intent == IntentEngage {
		moveOrders = sq.spreadPositions(contactX, contactY)
	}

	// Write orders to all members' blackboards, including shared contact position.
	orderIdx := 0
	for _, m := range sq.Members {
		if m.state == SoldierStateDead {
			continue
		}
		m.blackboard.SquadIntent = sq.Intent
		m.blackboard.OrderReceived = true
		m.blackboard.SquadHasContact = hasContact
		if hasContact {
			m.blackboard.SquadContactX = contactX
			m.blackboard.SquadContactY = contactY
		}
		// Assign a unique spread position for engage orders.
		if len(moveOrders) > 0 && orderIdx < len(moveOrders) {
			m.blackboard.OrderMoveX = moveOrders[orderIdx][0]
			m.blackboard.OrderMoveY = moveOrders[orderIdx][1]
			m.blackboard.HasMoveOrder = true
			orderIdx++
		} else {
			m.blackboard.HasMoveOrder = false
		}
	}
}

// spreadPositions returns a set of world-space positions fanned around
// the contact point, one per alive member, at a tactically useful standoff.
func (sq *Squad) spreadPositions(cx, cy float64) [][2]float64 {
	alive := sq.Alive()
	n := len(alive)
	if n == 0 {
		return nil
	}

	// Approach bearing: from leader toward contact.
	lx, ly := sq.Leader.x, sq.Leader.y
	bearing := math.Atan2(cy-ly, cx-lx)

	// Standoff: stop short of the contact by this many px.
	const standoff = 160.0
	// Lateral spacing between members.
	const lateralSpacing = 40.0

	// Spread symmetrically left/right of the bearing, perpendicular.
	positions := make([][2]float64, n)
	for i, m := range alive {
		_ = m
		// Symmetric index: 0 is centre, then alternating ±1, ±2, ...
		halfN := float64(n-1) / 2.0
		lateral := (float64(i) - halfN) * lateralSpacing

		// Position = contact point pulled back by standoff, then offset laterally.
		perpAngle := bearing + math.Pi/2
		targetX := cx - math.Cos(bearing)*standoff + math.Cos(perpAngle)*lateral
		targetY := cy - math.Sin(bearing)*standoff + math.Sin(perpAngle)*lateral
		positions[i] = [2]float64{targetX, targetY}
	}
	return positions
}

// squadSpread returns the max distance of any alive member from the leader.
func (sq *Squad) squadSpread() float64 {
	if sq.Leader == nil {
		return 0
	}
	max := 0.0
	for _, m := range sq.Members {
		if m == sq.Leader || m.state == SoldierStateDead {
			continue
		}
		dx := m.x - sq.Leader.x
		dy := m.y - sq.Leader.y
		d := math.Sqrt(dx*dx + dy*dy)
		if d > max {
			max = d
		}
	}
	return max
}

// LeaderCohesionSlowdown adjusts the leader's effective speed based on
// how spread out the squad is. If members are far behind, the leader slows
// or stops to let them catch up.
func (sq *Squad) LeaderCohesionSlowdown() float64 {
	spread := sq.squadSpread()
	switch {
	// Note: a normal 6-man wedge formation has a max slot distance of ~119px
	// from leader (sqrt(84^2 + 84^2)). Thresholds must be above that or the
	// leader will stop even when the squad is correctly formed.
	case spread > 220:
		return 0.0 // stop completely, let members catch up
	case spread > 180:
		return 0.3 // crawl speed
	case spread > 150:
		return 0.6 // slow
	default:
		return 1.0 // full speed
	}
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
	assigned := make(map[int][2]float64, len(sq.Members))

	for i, m := range sq.Members {
		if i == 0 || !m.formationMember || m.state == SoldierStateDead || i >= len(offsets) {
			continue
		}
		// Don't clobber paths for members who are actively engaging or closing on contact.
		// Their paths are managed by moveToContact / GoalEngage logic.
		g := m.blackboard.CurrentGoal
		if g == GoalMoveToContact || g == GoalEngage || g == GoalFallback {
			continue
		}
		// If a member has completed their current path, force a repath to the
		// current slot target so they don't fall into an idle state while the
		// leader continues moving.
		if m.path != nil && m.pathIndex >= len(m.path) {
			m.path = nil
			m.pathIndex = 0
		}
		off := offsets[i]
		wx, wy := SlotWorld(lx, ly, sq.smoothedHeading, off[0], off[1])
		wx, wy = adjustFormationTarget(m.navGrid, wx, wy, sq.Leader, sq.Members, assigned)

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
		assigned[m.id] = [2]float64{wx, wy}
	}
}

func adjustFormationTarget(ng *NavGrid, desiredX, desiredY float64, leader *Soldier, members []*Soldier, assigned map[int][2]float64) (float64, float64) {
	if ng == nil {
		return desiredX, desiredY
	}
	cx, cy := WorldToCell(desiredX, desiredY)
	if !ng.IsBlocked(cx, cy) {
		return desiredX, desiredY
	}

	maxR := 10
	bestX, bestY := desiredX, desiredY
	bestCost := math.MaxFloat64

	for r := 0; r <= maxR; r++ {
		for dy := -r; dy <= r; dy++ {
			for dx := -r; dx <= r; dx++ {
				if abs(dx) != r && abs(dy) != r {
					continue
				}
				nx, ny := cx+dx, cy+dy
				if ng.IsBlocked(nx, ny) {
					continue
				}
				wx, wy := CellToWorld(nx, ny)
				cost := formationTargetCost(wx, wy, desiredX, desiredY, leader, members, assigned)
				if cost < bestCost {
					bestCost = cost
					bestX, bestY = wx, wy
				}
			}
		}
		if bestCost < math.MaxFloat64 {
			break
		}
	}

	return bestX, bestY
}

func formationTargetCost(wx, wy, desiredX, desiredY float64, leader *Soldier, members []*Soldier, assigned map[int][2]float64) float64 {
	dx := wx - desiredX
	dy := wy - desiredY
	cost := math.Sqrt(dx*dx+dy*dy) * 1.0

	if leader != nil {
		dlx := wx - leader.x
		dly := wy - leader.y
		cost += math.Sqrt(dlx*dlx+dly*dly) * 0.01
	}

	minSep := float64(soldierRadius) * 3.0
	for _, m := range members {
		if m == nil || m.state == SoldierStateDead {
			continue
		}
		mx, my := m.x, m.y
		if p, ok := assigned[m.id]; ok {
			mx, my = p[0], p[1]
		}
		dsx := wx - mx
		dsy := wy - my
		d := math.Sqrt(dsx*dsx + dsy*dsy)
		if d < 1e-6 {
			cost += 1e6
			continue
		}
		if d < minSep {
			cost += (minSep - d) * 200.0
		}
	}

	return cost
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
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
