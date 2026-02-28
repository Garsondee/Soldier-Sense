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

	// Active officer command for the squad. Commands are strong signals that
	// bias individual utility, not hard overrides.
	ActiveOrder OfficerOrder
	nextOrderID int

	// smoothedHeading is a low-pass filtered version of the leader's heading,
	// used to prevent formation thrash on minor direction changes.
	smoothedHeading float64
	headingInit     bool
	prevIntent      SquadIntentKind

	// EnemyBearing is the squad-level bearing from centroid toward the enemy.
	// Updated each SquadThink when contact exists. Used to assign flank sides.
	EnemyBearing float64

	// Command succession: when the leader dies the next member takes over
	// after a delay scaled by their stress level.
	leaderDeadTick       int  // tick when the leader was first found dead
	leaderSuccessionTick int  // tick when command is re-established
	leaderSucceeding     bool // true while awaiting succession

	// Building takeover: leader claims a building along the advance route.
	// Members then prioritize doors/windows of that building.
	ClaimedBuildingIdx int    // index into buildingFootprints, -1 = none
	claimEvalTick      int    // tick of last building evaluation
	buildingFootprints []rect // shared reference to game footprints

	// Intent hysteresis: avoid order thrash at range boundaries.
	intentLockUntil int // tick until which non-critical intent changes are deferred

	// --- Buddy bounding (fire and movement) ---
	// BoundMovingGroup: which group (0 or 1) is currently the mover.
	// Toggled each bound cycle so groups alternate.
	BoundMovingGroup int
	// boundCycleTick: tick when the current bound cycle started.
	// A cycle lasts until the moving group's members finish their dash
	// (all have dashOverwatchTimer > 0 or are idle), then groups swap.
	boundCycleTick int
	// boundCycleActive: true when buddy bounding is in effect (contact + MoveToContact).
	boundCycleActive bool
}

// NewSquad creates a squad. The first member is designated leader.
func NewSquad(id int, team Team, members []*Soldier) *Squad {
	sq := &Squad{
		ID:                 id,
		Team:               team,
		Members:            members,
		Formation:          FormationWedge,
		ClaimedBuildingIdx: -1,
		ActiveOrder: OfficerOrder{
			Kind:  CmdNone,
			State: OfficerOrderInactive,
		},
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
			// Keep the member's initial path so they start moving immediately.
			// UpdateFormation will redirect them to formation slots once the
			// leader begins moving and the squad spreads naturally.
		}
	}
	return sq
}

// successionDelayTicks is the base delay (in ticks) for command succession.
// Scaled by the new leader's effective fear (panicked = much longer).
const successionDelayTicks = 180 // 3 seconds at 60TPS base

const (
	engageEnterDist = 300.0 // must be this close to enter engage
	engageExitDist  = 360.0 // can stay engaged until this distance
)

func (sq *Squad) issueOfficerOrder(tick int, kind OfficerCommandKind, targetX, targetY, radius float64, formation FormationType, priority, strength float64, ttl int) {
	if sq.ActiveOrder.Kind == kind && sq.ActiveOrder.State == OfficerOrderActive {
		dx := sq.ActiveOrder.TargetX - targetX
		dy := sq.ActiveOrder.TargetY - targetY
		if math.Sqrt(dx*dx+dy*dy) < 16 && sq.ActiveOrder.Formation == formation {
			sq.ActiveOrder.ExpiresTick = tick + ttl
			sq.ActiveOrder.Priority = priority
			sq.ActiveOrder.Strength = strength
			sq.ActiveOrder.Radius = radius
			return
		}
	}

	sq.nextOrderID++
	sq.ActiveOrder = OfficerOrder{
		ID:          sq.nextOrderID,
		Kind:        kind,
		IssuedTick:  tick,
		ExpiresTick: tick + ttl,
		Priority:    priority,
		Strength:    strength,
		TargetX:     targetX,
		TargetY:     targetY,
		Radius:      radius,
		Formation:   formation,
		State:       OfficerOrderActive,
	}
	if sq.Leader != nil {
		sq.Leader.think(fmt.Sprintf("order: %s", kind))
	}
}

func (sq *Squad) syncOfficerOrder(tick int, hasContact bool, contactX, contactY float64) {
	if sq.Leader == nil {
		return
	}

	leaderX, leaderY := sq.Leader.x, sq.Leader.y
	goalX, goalY := sq.Leader.endTarget[0], sq.Leader.endTarget[1]
	goalDist := math.Hypot(goalX-leaderX, goalY-leaderY)

	switch sq.Intent {
	case IntentAdvance:
		form := FormationWedge
		if !hasContact && goalDist > 650 {
			form = FormationColumn
		}
		sq.Formation = form
		sq.issueOfficerOrder(tick, CmdMoveTo, goalX, goalY, 120, form, 0.65, 0.80, 360)

	case IntentHold:
		sq.Formation = FormationLine
		sq.issueOfficerOrder(tick, CmdHold, leaderX, leaderY, 150, sq.Formation, 0.70, 0.85, 240)

	case IntentRegroup, IntentWithdraw:
		sq.Formation = FormationWedge
		sq.issueOfficerOrder(tick, CmdRegroup, leaderX, leaderY, 180, sq.Formation, 0.85, 0.95, 220)

	case IntentEngage:
		sq.Formation = FormationLine
		tx, ty := contactX, contactY
		if !hasContact {
			tx, ty = goalX, goalY
		}
		sq.issueOfficerOrder(tick, CmdFanOut, tx, ty, 220, sq.Formation, 0.80, 0.90, 260)
	}

	if sq.ActiveOrder.State == OfficerOrderActive && sq.ActiveOrder.ExpiresTick > 0 && tick > sq.ActiveOrder.ExpiresTick {
		sq.ActiveOrder.State = OfficerOrderExpired
	}
}

// SquadThink runs the leader's squad-level decision loop.
// It evaluates the leader's blackboard and sets Intent + orders for members.
// intel is the world IntelStore; may be nil (degrades gracefully to blackboard-only).
func (sq *Squad) SquadThink(intel *IntelStore) {
	if sq.Leader == nil || sq.Leader.state == SoldierStateDead {
		alive := sq.Alive()
		if len(alive) == 0 {
			return
		}
		candidate := alive[0]

		if !sq.leaderSucceeding {
			// Leader just died — start succession clock.
			// Delay is longer if candidate is stressed or inexperienced.
			ef := candidate.profile.Psych.EffectiveFear()
			exp := candidate.profile.Psych.Experience
			delay := int(float64(successionDelayTicks) * (1.0 + ef*2.0) * (1.2 - exp*0.8))
			sq.leaderSuccessionTick = *candidate.currentTick + delay
			sq.leaderSucceeding = true
			sq.leaderDeadTick = *candidate.currentTick
			candidate.think("leader down — taking command")
		}

		// During the succession window, squad operates without clear leadership.
		// Members hold/survive on their own until command is re-established.
		if candidate.currentTick != nil && *candidate.currentTick < sq.leaderSuccessionTick {
			sq.Intent = IntentHold
			// Propagate a holding intent but don't update the leader pointer yet.
			for _, m := range sq.Members {
				if m.state == SoldierStateDead {
					continue
				}
				m.blackboard.SquadIntent = IntentHold
			}
			return
		}

		// Succession complete — install new leader.
		sq.Leader = candidate
		sq.Leader.isLeader = true
		sq.leaderSucceeding = false
		candidate.think("command established")
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

	// Fall back to heard gunfire as a contact source (infinite range sound).
	if !hasContact {
		for _, m := range sq.Members {
			if m.state == SoldierStateDead {
				continue
			}
			if m.blackboard.HeardGunfire {
				contactX = m.blackboard.HeardGunfireX
				contactY = m.blackboard.HeardGunfireY
				hasContact = true
				break
			}
		}
	}

	// Final fallback: use the strongest persistent combat memory across all members.
	// This keeps the squad activated and moving even when the field goes quiet.
	if !hasContact {
		bestMem := 0.0
		for _, m := range sq.Members {
			if m.state == SoldierStateDead {
				continue
			}
			if m.blackboard.CombatMemoryStrength > bestMem {
				bestMem = m.blackboard.CombatMemoryStrength
				contactX = m.blackboard.CombatMemoryX
				contactY = m.blackboard.CombatMemoryY
			}
		}
		if bestMem > 0.05 {
			hasContact = true
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
	candidateIntent := sq.Intent
	switch {
	// Cohesion emergency: regroup even under contact if spread is extreme.
	case spread > 250:
		candidateIntent = IntentRegroup
	// Active firefight: any member has LOS on a threat close enough to engage.
	case anyVisibleThreats > 0 && closestDist < engageEnterDist:
		candidateIntent = IntentEngage
	case sq.Intent == IntentEngage && anyVisibleThreats > 0 && closestDist < engageExitDist:
		candidateIntent = IntentEngage
	// Heatmap: heavy danger pressure even without current LOS → hold/fallback.
	case dangerAtPos > 1.5 && anyVisibleThreats == 0:
		candidateIntent = IntentHold
	// Distant contact: keep advancing while watching.
	case anyVisibleThreats > 0 && closestDist >= engageExitDist:
		candidateIntent = IntentAdvance
	// Heatmap: contact heat ahead but no LOS yet → cautious advance.
	case contactAhead > 0.5 && anyVisibleThreats == 0:
		candidateIntent = IntentAdvance
	// Moderate spread: light regroup nudge but don't abandon a fight.
	case spread > 120 && anyVisibleThreats == 0:
		candidateIntent = IntentRegroup
	default:
		candidateIntent = IntentAdvance
	}

	tick := 0
	if sq.Leader != nil && sq.Leader.currentTick != nil {
		tick = *sq.Leader.currentTick
	}
	criticalIntent := spread > 250 || (anyVisibleThreats > 0 && closestDist < engageEnterDist)
	if candidateIntent != sq.Intent {
		if !criticalIntent && tick < sq.intentLockUntil {
			candidateIntent = sq.Intent
		} else {
			leaderFear := sq.Leader.profile.Psych.EffectiveFear()
			fuzzy := int(math.Abs(math.Sin(float64(tick+sq.ID*17))) * 18)
			sq.intentLockUntil = tick + 30 + int(leaderFear*70) + fuzzy
		}
	}
	sq.Intent = candidateIntent
	sq.syncOfficerOrder(tick, hasContact, contactX, contactY)

	// Log intent changes.
	if sq.Intent != oldIntent {
		sq.Leader.think(fmt.Sprintf("squad: %s → %s", oldIntent, sq.Intent))
	}

	// Compute enemy bearing from squad centroid toward contact.
	// This is the shared "normal toward enemy" all members use for flanking.
	if hasContact {
		cx, cy := sq.squadCentroid()
		sq.EnemyBearing = math.Atan2(contactY-cy, contactX-cx)
	}

	// Build per-member spread positions when engaging.
	var moveOrders [][2]float64
	if hasContact && sq.Intent == IntentEngage {
		moveOrders = sq.spreadPositions(contactX, contactY)
	}

	// Assign alternating flank sides based on member index so half go left, half right.
	// Left = bearing - 90°, right = bearing + 90°.
	flankIdx := 0
	for _, m := range sq.Members {
		if m.state == SoldierStateDead {
			continue
		}
		if flankIdx%2 == 0 {
			m.blackboard.FlankSide = +1.0 // left
		} else {
			m.blackboard.FlankSide = -1.0 // right
		}
		m.blackboard.SquadEnemyBearing = sq.EnemyBearing
		flankIdx++
	}

	// --- Outnumbered factor ---
	// Count how many unique enemies the squad can see and how many members have
	// eyes on at least one enemy. The ratio tells us if we're outnumbered.
	membersWithContact := 0
	alive := sq.Alive()
	for _, m := range alive {
		if m.blackboard.VisibleThreatCount() > 0 {
			membersWithContact++
		}
	}
	// OutnumberedFactor: enemies seen / members with contact.
	// >1 = outnumbered (more enemies than friendlies engaged), <1 = we outnumber them.
	outnumberedFactor := 1.0
	if membersWithContact > 0 && anyVisibleThreats > 0 {
		outnumberedFactor = float64(anyVisibleThreats) / float64(membersWithContact)
	}

	// Squad posture: derived from outnumbered factor, intent, and casualties.
	// Outnumbered → more offensive (aggressive push to overwhelm before being picked apart).
	// Outnumbering → more defensive (hold ground, let them come to us).
	posture := 0.0
	if outnumberedFactor > 1.2 {
		// Outnumbered: push offensive to break through.
		posture = math.Min(1.0, (outnumberedFactor-1.0)*0.8)
	} else if outnumberedFactor < 0.8 && outnumberedFactor > 0 {
		// Outnumbering: defensive hold.
		posture = math.Max(-1.0, -(1.0-outnumberedFactor)*0.8)
	}
	// Intent modifiers.
	switch sq.Intent {
	case IntentEngage:
		posture += 0.2
	case IntentHold, IntentWithdraw:
		posture -= 0.2
	}
	// Casualty pressure: more dead = more desperate = more aggressive.
	casualtyRate := 1.0 - float64(len(alive))/float64(len(sq.Members))
	posture += casualtyRate * 0.4
	if posture > 1.0 {
		posture = 1.0
	}
	if posture < -1.0 {
		posture = -1.0
	}

	// Write orders to all members' blackboards, including shared contact position.
	orderIdx := 0
	for _, m := range sq.Members {
		if m.state == SoldierStateDead {
			continue
		}
		m.blackboard.SquadIntent = sq.Intent
		m.blackboard.OrderReceived = true
		if sq.ActiveOrder.IsActiveAt(tick) {
			m.blackboard.OfficerOrderKind = sq.ActiveOrder.Kind
			m.blackboard.OfficerOrderTargetX = sq.ActiveOrder.TargetX
			m.blackboard.OfficerOrderTargetY = sq.ActiveOrder.TargetY
			m.blackboard.OfficerOrderRadius = sq.ActiveOrder.Radius
			m.blackboard.OfficerOrderPriority = sq.ActiveOrder.Priority
			m.blackboard.OfficerOrderStrength = sq.ActiveOrder.Strength
			m.blackboard.OfficerOrderActive = true
		} else {
			m.blackboard.OfficerOrderKind = CmdNone
			m.blackboard.OfficerOrderActive = false
			m.blackboard.OfficerOrderPriority = 0
			m.blackboard.OfficerOrderStrength = 0
		}
		m.blackboard.SquadHasContact = hasContact
		m.blackboard.OutnumberedFactor = outnumberedFactor
		m.blackboard.SquadPosture = posture
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

		// --- Social awareness propagation ---
		m.blackboard.VisibleAllyCount = sq.visibleAlliesFor(m)
		prevAvgFear := m.blackboard.SquadAvgFear
		m.blackboard.SquadAvgFear = sq.avgVisibleAllyFearFor(m)
		m.blackboard.SquadFearDelta = m.blackboard.SquadAvgFear - prevAvgFear
		m.blackboard.CloseAllyPressure = sq.closeAllyPressureFor(m)
		if m.blackboard.VisibleThreatCount() == 0 && m.blackboard.VisibleAllyCount == 0 {
			m.blackboard.IsolatedTicks++
		} else {
			m.blackboard.IsolatedTicks = 0
		}
	}

	// --- Buddy bounding (fire and movement) ---
	// Active when squad has contact and at least 2 alive members.
	// Groups alternate: one moves while the other overwatches.
	if hasContact && len(alive) >= 2 {
		if !sq.boundCycleActive {
			// Start bounding: assign groups and kick off first cycle.
			sq.boundCycleActive = true
			sq.BoundMovingGroup = 0
			sq.boundCycleTick = tick
			grpIdx := 0
			for _, m := range sq.Members {
				if m.state == SoldierStateDead {
					continue
				}
				m.blackboard.BoundGroup = grpIdx % 2
				grpIdx++
			}
			sq.Leader.think("squad: initiating buddy bounding")
		}

		// Check if all movers have finished their dash (idle or in overwatch).
		// If so, swap groups so the overwatchers become movers.
		allMoversSettled := true
		for _, m := range sq.Members {
			if m.state == SoldierStateDead {
				continue
			}
			if m.blackboard.BoundGroup != sq.BoundMovingGroup {
				continue
			}
			// A mover is "settled" if they're in overwatch pause or idle (not actively sprinting).
			if m.blackboard.CurrentGoal == GoalMoveToContact && m.state == SoldierStateMoving {
				allMoversSettled = false
				break
			}
		}
		// Minimum cycle time: don't swap faster than 2 seconds.
		cycleMinTicks := 120
		if allMoversSettled && tick-sq.boundCycleTick >= cycleMinTicks {
			sq.BoundMovingGroup = 1 - sq.BoundMovingGroup
			sq.boundCycleTick = tick
		}

		// Write bound role to each member's blackboard.
		for _, m := range sq.Members {
			if m.state == SoldierStateDead {
				continue
			}
			m.blackboard.BoundMover = m.blackboard.BoundGroup == sq.BoundMovingGroup
		}
	} else {
		if sq.boundCycleActive {
			sq.boundCycleActive = false
			// Clear bound roles — everyone can move freely.
			for _, m := range sq.Members {
				if m.state == SoldierStateDead {
					continue
				}
				m.blackboard.BoundMover = true
			}
		}
	}

	// --- Building takeover ---
	// Leader periodically evaluates nearby buildings along the advance route.
	sq.evaluateBuildings()
	// Propagate claim to all alive members.
	for _, m := range sq.Members {
		if m.state == SoldierStateDead {
			continue
		}
		m.blackboard.ClaimedBuildingIdx = sq.ClaimedBuildingIdx
		if sq.ClaimedBuildingIdx >= 0 && sq.ClaimedBuildingIdx < len(sq.buildingFootprints) {
			fp := sq.buildingFootprints[sq.ClaimedBuildingIdx]
			m.blackboard.ClaimedBuildingX = float64(fp.x) + float64(fp.w)/2
			m.blackboard.ClaimedBuildingY = float64(fp.y) + float64(fp.h)/2
		}
	}

	// --- Morale-driven reinforcement ---
	// The leader identifies the most-stressed alive member and directs calm
	// soldiers toward them. Only active outside a full panic situation.
	leaderFear := sq.Leader.profile.Psych.EffectiveFear()
	if leaderFear < 0.6 {
		// Find the member with the highest effective fear.
		var distressedMember *Soldier
		worstFear := 0.35 // minimum threshold to be considered distressed
		for _, m := range sq.Members {
			if m.state == SoldierStateDead || m == sq.Leader {
				continue
			}
			ef := m.profile.Psych.EffectiveFear()
			if ef > worstFear {
				worstFear = ef
				distressedMember = m
			}
		}

		if distressedMember != nil {
			// Direct calm members toward the distressed one.
			for _, m := range sq.Members {
				if m.state == SoldierStateDead || m == distressedMember {
					continue
				}
				mf := m.profile.Psych.EffectiveFear()
				mm := m.profile.Psych.Morale
				// A soldier needs high morale + low fear to be a reinforcer.
				if mf < 0.25 && mm > 0.55 {
					// Offset target so reinforcers don't stack on the exact same tile.
					// Deterministic pseudo-random per soldier id to keep runs replayable.
					idx := float64((m.id + sq.ID*17) % 8)
					ang := idx * (math.Pi / 4.0)
					r := float64(cellSize) * (1.6 + 0.3*math.Abs(math.Sin(float64(*m.currentTick+1)*0.11+idx)))
					m.blackboard.ReinforceMemberX = distressedMember.x + math.Cos(ang)*r
					m.blackboard.ReinforceMemberY = distressedMember.y + math.Sin(ang)*r
					m.blackboard.ShouldReinforce = true
				} else {
					m.blackboard.ShouldReinforce = false
				}
			}
		}
	}
}

func (sq *Squad) visibleAlliesFor(self *Soldier) int {
	count := 0
	for _, m := range sq.Members {
		if m == self || m.state == SoldierStateDead {
			continue
		}
		if !self.vision.InCone(self.x, self.y, m.x, m.y) {
			continue
		}
		if HasLineOfSightWithCover(self.x, self.y, m.x, m.y, self.buildings, self.covers) {
			count++
		}
	}
	return count
}

func (sq *Squad) avgVisibleAllyFearFor(self *Soldier) float64 {
	var sum float64
	count := 0
	for _, m := range sq.Members {
		if m == self || m.state == SoldierStateDead {
			continue
		}
		if !self.vision.InCone(self.x, self.y, m.x, m.y) {
			continue
		}
		if !HasLineOfSightWithCover(self.x, self.y, m.x, m.y, self.buildings, self.covers) {
			continue
		}
		sum += m.profile.Psych.EffectiveFear()
		count++
	}
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}

func (sq *Squad) closeAllyPressureFor(self *Soldier) float64 {
	const idealSpacing = float64(cellSize) * 2.2
	const nearSpacing = float64(cellSize) * 1.2

	pressure := 0.0
	samples := 0.0
	for _, m := range sq.Members {
		if m == self || m.state == SoldierStateDead {
			continue
		}
		dx := m.x - self.x
		dy := m.y - self.y
		d := math.Sqrt(dx*dx + dy*dy)
		if d > idealSpacing {
			continue
		}
		t := clamp01((idealSpacing - d) / (idealSpacing - nearSpacing))
		pressure += t
		samples++
	}
	if samples == 0 {
		return 0
	}
	return clamp01(pressure / samples)
}

// buildingClaimInterval is how often (ticks) the leader re-evaluates buildings.
const buildingClaimInterval = 300 // ~5s at 60TPS

// evaluateBuildings checks nearby buildings along the advance route and claims
// one if it lies roughly ahead of the squad. Prefers buildings that are:
// (a) between the squad and its advance target, (b) close to the squad,
// (c) not already behind the squad.
func (sq *Squad) evaluateBuildings() {
	if sq.Leader == nil || sq.Leader.state == SoldierStateDead {
		return
	}
	if len(sq.buildingFootprints) == 0 {
		return
	}
	tick := 0
	if sq.Leader.currentTick != nil {
		tick = *sq.Leader.currentTick
	}
	if tick-sq.claimEvalTick < buildingClaimInterval {
		return
	}
	sq.claimEvalTick = tick

	lx, ly := sq.Leader.x, sq.Leader.y
	// Advance direction: from start toward end target.
	advX := sq.Leader.endTarget[0] - lx
	advY := sq.Leader.endTarget[1] - ly
	advLen := math.Sqrt(advX*advX + advY*advY)
	if advLen < 1 {
		return
	}
	advX /= advLen
	advY /= advLen

	bestIdx := -1
	bestScore := -999.0
	maxDist := 400.0 // only consider buildings within 400px

	for i, fp := range sq.buildingFootprints {
		// Building centroid.
		cx := float64(fp.x) + float64(fp.w)/2
		cy := float64(fp.y) + float64(fp.h)/2
		dx := cx - lx
		dy := cy - ly
		dist := math.Sqrt(dx*dx + dy*dy)
		if dist > maxDist || dist < 1 {
			continue
		}
		// Dot product: how much the building is "ahead" of the squad.
		dot := (dx*advX + dy*advY) / dist
		if dot < 0.1 {
			continue // building is behind or to the side
		}
		// Score: prefer ahead and close.
		score := dot*0.6 - dist/maxDist*0.4
		// Bigger buildings are more valuable (more cover).
		area := float64(fp.w * fp.h)
		score += math.Min(0.3, area/50000.0)

		if score > bestScore {
			bestScore = score
			bestIdx = i
		}
	}

	if bestIdx >= 0 && bestScore > 0.1 {
		if bestIdx != sq.ClaimedBuildingIdx {
			sq.ClaimedBuildingIdx = bestIdx
			fp := sq.buildingFootprints[bestIdx]
			sq.Leader.think(fmt.Sprintf("claiming building at (%d,%d)", fp.x, fp.y))
		}
	}
}

// spreadPositions returns spread positions for members during IntentEngage.
// Soldiers are placed in a lateral line perpendicular to the enemy bearing,
// at a standoff distance from the contact point. This prevents the joust pattern
// (running straight at the enemy) by ensuring everyone approaches from a flank angle.
func (sq *Squad) spreadPositions(cx, cy float64) [][2]float64 {
	alive := sq.Alive()
	n := len(alive)
	if n == 0 {
		return nil
	}

	// Use the squad-level enemy bearing (from centroid), not the leader alone.
	bearing := sq.EnemyBearing
	perpAngle := bearing + math.Pi/2

	// Standoff: positions are this far back from the contact, so nobody runs
	// directly into the enemy's face.
	const standoff = 180.0
	// Lateral spacing: members spread wider so they approach from multiple angles.
	const lateralSpacing = 55.0

	// Base point: contact pulled back by standoff along the approach bearing.
	baseX := cx - math.Cos(bearing)*standoff
	baseY := cy - math.Sin(bearing)*standoff

	positions := make([][2]float64, n)
	for i := range alive {
		// Symmetric lateral offset: centre is at 0, then alternating ±1, ±2...
		halfN := float64(n-1) / 2.0
		lateral := (float64(i) - halfN) * lateralSpacing
		positions[i] = [2]float64{
			baseX + math.Cos(perpAngle)*lateral,
			baseY + math.Sin(perpAngle)*lateral,
		}
	}
	return positions
}

// squadCentroid returns the average position of all alive members.
func (sq *Squad) squadCentroid() (float64, float64) {
	alive := sq.Alive()
	if len(alive) == 0 {
		return sq.Leader.x, sq.Leader.y
	}
	var sumX, sumY float64
	for _, m := range alive {
		sumX += m.x
		sumY += m.y
	}
	n := float64(len(alive))
	return sumX / n, sumY / n
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
	// Thresholds widened for 8-man squads.
	// An 8-man wedge has max slot distance ~197px (slot 7: depth=4*28, side=4*28).
	// Leaders should only slow when members are genuinely left behind, not just
	// spread in a valid formation.
	switch {
	case spread > 420:
		return 0.0 // stop: squad is truly scattered
	case spread > 340:
		return 0.3 // crawl
	case spread > 280:
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
		if g == GoalMoveToContact || g == GoalEngage || g == GoalFallback || g == GoalFlank || g == GoalOverwatch {
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
