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
	Phase     SquadPhase

	// Active officer command for the squad. Commands are strong signals that
	// bias individual utility, not hard overrides.
	ActiveOrder OfficerOrder
	nextOrderID int

	// smoothedHeading is a low-pass filtered version of the leader's heading,
	// used to prevent formation thrash on minor direction changes.
	smoothedHeading float64
	headingInit     bool

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
	// Claimed-building lifecycle tracking.
	claimedNoContactTicks      int
	claimedOccupiedTicks       int
	buildingClaimCooldownUntil int

	// Intent hysteresis: avoid order thrash at range boundaries.
	intentLockUntil int // tick until which non-critical intent changes are deferred
	// Stalemate controller: detects prolonged non-progress contact and forces
	// proactive pushes to break static long-range deadlocks.
	stalemateTicks     int
	proactivePushUntil int
	// Cooldown for targeted officer intervention on stalled members.
	lastStalledOrderTick int
	lastStalledOrderID   int

	// Phase controller state (squad-level), used to enforce progress while
	// preserving per-soldier autonomy.
	phaseInit          bool
	phaseEnteredTick   int
	lastProgressTick   int
	lastProgressMetric float64

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

	// Cohesion collapse telemetry.
	CasualtyRate       float64
	Stress             float64
	StressDelta        float64
	AvgMorale          float64
	MoraleDelta        float64
	Cohesion           float64
	CohesionDelta      float64
	Broken             bool
	breakLockEnd       int
	breakPressureTicks int

	// Cohesion shock memory: temporary hit from fresh injuries/deaths,
	// decays over time so squads can regain backbone after contact eases.
	cohesionShock         float64
	prevInjuredAliveCount int
	prevCasualtyCount     int

	// Phase A radio net state.
	radioNet                   radioNet
	radioPendingStatus         map[int]int  // memberID -> deadline tick
	radioStatusReplyQueued     map[int]bool // memberID -> true once one reply has been queued for current request
	radioUnresponsive          map[int]bool // memberID -> no-reply inferred
	radioStatusRequestCursor   int
	radioLastStatusRequestTick int
	radioChannelBusyUntil      int
	radioInFlight              *radioTransmission

	// Radio telemetry counters.
	RadioQueued   int
	RadioSent     int
	RadioReceived int
	RadioDropped  int
	RadioGarbled  int
	RadioTimeouts int

	// Transient render data for radio transmission effects.
	radioVisualEvents []radioVisualEvent
	radioChatLines    []radioChatLine
}

const stalledOrderCooldownTicks = 90

func orderImmediateObedienceRoll(tick, soldierID, orderID int) float64 {
	seed := float64((tick + 1) * (soldierID + 3) * (orderID + 5))
	return math.Abs(math.Sin(seed*0.017 + float64(orderID)*0.11 + float64(soldierID)*0.07))
}

func shouldImmediatelyObeyOrder(tick, soldierID, orderID int, chance float64) bool {
	return orderImmediateObedienceRoll(tick, soldierID, orderID) < clamp01(chance)
}

type SquadPhase int

const (
	SquadPhaseApproach SquadPhase = iota
	SquadPhaseFixFire
	SquadPhaseBound
	SquadPhaseAssault
	SquadPhaseStalledRecovery
	SquadPhaseConsolidate
)

func (sp SquadPhase) String() string {
	switch sp {
	case SquadPhaseApproach:
		return "approach"
	case SquadPhaseFixFire:
		return "fix_fire"
	case SquadPhaseBound:
		return "bound"
	case SquadPhaseAssault:
		return "assault"
	case SquadPhaseStalledRecovery:
		return "stalled_recovery"
	case SquadPhaseConsolidate:
		return "consolidate"
	default:
		return "unknown"
	}
}

func (sq *Squad) recoveryRetaskTarget(hasContact bool, contactX, contactY float64) (float64, float64) {
	if sq.Leader == nil {
		return 0, 0
	}
	lx, ly := sq.Leader.x, sq.Leader.y
	tx, ty := sq.Leader.endTarget[0], sq.Leader.endTarget[1]
	if hasContact {
		dx := contactX - lx
		dy := contactY - ly
		dist := math.Hypot(dx, dy)
		if dist > 1e-6 {
			step := math.Min(220.0, math.Max(90.0, dist*0.4))
			tx = lx + dx/dist*step
			ty = ly + dy/dist*step
		} else {
			tx, ty = contactX, contactY
		}
	}
	if sq.Leader.navGrid != nil {
		w := float64(sq.Leader.navGrid.cols * cellSize)
		h := float64(sq.Leader.navGrid.rows * cellSize)
		if tx < 16 {
			tx = 16
		}
		if tx > w-16 {
			tx = w - 16
		}
		if ty < 16 {
			ty = 16
		}
		if ty > h-16 {
			ty = h - 16
		}
	}
	return tx, ty
}

func (sq *Squad) clearStalledPathDebt() int {
	cleared := 0
	for _, m := range sq.Members {
		if m.state == SoldierStateDead {
			continue
		}
		if m.blackboard.CurrentGoal != GoalMoveToContact && m.blackboard.CurrentGoal != GoalEngage {
			continue
		}
		pathLen := len(m.path)
		pathRemain := 0
		if m.path != nil && m.pathIndex >= 0 && m.pathIndex < pathLen {
			pathRemain = pathLen - m.pathIndex
		}
		terminal := m.path == nil || pathRemain <= 1
		if m.state == SoldierStateIdle && terminal {
			m.path = nil
			m.pathIndex = 0
			m.boundHoldTicks = 0
			cleared++
		}
	}
	return cleared
}

// NewSquad creates a squad. The first member is designated leader.
func NewSquad(id int, team Team, members []*Soldier) *Squad {
	sq := &Squad{
		ID:                 id,
		Team:               team,
		Members:            members,
		Formation:          FormationWedge,
		Phase:              SquadPhaseApproach,
		Cohesion:           1.0,
		ClaimedBuildingIdx: -1,
		radioNet: radioNet{
			netID: id + 1,
		},
		radioPendingStatus:     make(map[int]int),
		radioStatusReplyQueued: make(map[int]bool),
		radioUnresponsive:      make(map[int]bool),
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
	stalemateRange  = accurateFireRange * 1.05

	leaderPreferredForwardMin = 72.0
	leaderPreferredForwardMax = 170.0
	leaderPreferredFlankBase  = 64.0

	phaseMinHoldTicks    = 90
	phaseStallTicks      = 240
	phaseProgressEpsilon = 12.0
	phaseRecoveryTicks   = 120

	squadBreakPressureThreshold = 0.82
	squadBreakRecoverThreshold  = 0.42
	squadBreakLockTicks         = 120

	phaseEventSteerMinHoldTicks = 36
	stalemateTriggerTicks       = 150
	stalemateForceTicks         = 240
)

func (sq *Squad) claimedBuildingCentroid() (float64, float64, bool) {
	if sq.ClaimedBuildingIdx < 0 || sq.ClaimedBuildingIdx >= len(sq.buildingFootprints) {
		return 0, 0, false
	}
	fp := sq.buildingFootprints[sq.ClaimedBuildingIdx]
	return float64(fp.x) + float64(fp.w)/2, float64(fp.y) + float64(fp.h)/2, true
}

func (sq *Squad) updateStalematePressure(tick int, hasContact bool, anyVisibleThreats int, closestDist float64, phaseStalled bool, terminalStalledCount, aliveCount int) (bool, bool) {
	if sq.Leader == nil || !hasContact || aliveCount == 0 {
		sq.stalemateTicks = 0
		if tick >= sq.proactivePushUntil {
			sq.proactivePushUntil = 0
		}
		return false, tick < sq.proactivePushUntil
	}

	leaderUnderFire := sq.Leader.blackboard.IncomingFireCount > 0 || sq.Leader.blackboard.IsSuppressed()
	outOfEffectiveRange := closestDist > stalemateRange
	limitedVisualContact := anyVisibleThreats == 0
	manyTerminalStalled := aliveCount >= 2 && terminalStalledCount*2 >= aliveCount

	if outOfEffectiveRange && !leaderUnderFire && (phaseStalled || limitedVisualContact || manyTerminalStalled) {
		sq.stalemateTicks++
	} else if sq.stalemateTicks > 0 {
		sq.stalemateTicks -= 2
		if sq.stalemateTicks < 0 {
			sq.stalemateTicks = 0
		}
	}

	stalemateActive := sq.stalemateTicks >= stalemateTriggerTicks
	if stalemateActive {
		if tick >= sq.proactivePushUntil && sq.Leader != nil {
			sq.Leader.think("stalemate detected — forcing proactive push")
		}
		if sq.proactivePushUntil < tick+stalemateForceTicks {
			sq.proactivePushUntil = tick + stalemateForceTicks
		}
	}

	return stalemateActive, tick < sq.proactivePushUntil
}

func (sq *Squad) leaderObservedPhaseSteer(hasContact bool, closestDist float64, anyVisibleThreats int) (SquadPhase, bool) {
	if sq.Leader == nil || sq.Leader.state == SoldierStateDead {
		return sq.Phase, false
	}
	bb := &sq.Leader.blackboard

	leaderPinned := bb.IncomingFireCount >= 2 || bb.SuppressLevel > 0.58
	leaderCloseThreat := bb.VisibleThreatCount() > 0 && closestDist > 0 && closestDist < engageEnterDist*0.85
	leaderBlindInFight := bb.SquadHasContact && bb.VisibleThreatCount() == 0 && bb.HeardGunfire && bb.LocalSightlineScore < 0.32
	leaderOpportunity := hasContact && anyVisibleThreats > 0 && closestDist > 0 &&
		closestDist < engageEnterDist*0.90 && bb.IncomingFireCount == 0 && bb.SuppressLevel < 0.25 && bb.LocalSightlineScore > 0.34

	switch {
	case leaderPinned:
		if hasContact {
			return SquadPhaseFixFire, true
		}
		return SquadPhaseConsolidate, true
	case leaderBlindInFight:
		return SquadPhaseFixFire, true
	case leaderOpportunity:
		if closestDist < engageEnterDist*0.70 {
			return SquadPhaseAssault, true
		}
		return SquadPhaseBound, true
	case leaderCloseThreat && sq.Phase == SquadPhaseApproach:
		return SquadPhaseFixFire, true
	default:
		return sq.Phase, false
	}
}

func (sq *Squad) applyLeaderPhaseSteering(tick int, hasContact bool, closestDist float64, anyVisibleThreats int) {
	next, ok := sq.leaderObservedPhaseSteer(hasContact, closestDist, anyVisibleThreats)
	if !ok || next == sq.Phase {
		return
	}
	elapsed := tick - sq.phaseEnteredTick
	if elapsed < phaseEventSteerMinHoldTicks && next != SquadPhaseConsolidate {
		return
	}
	sq.advancePhase(tick, next)
}

func (sq *Squad) phaseProgressMetric(hasContact bool, contactX, contactY float64) float64 {
	if sq.Leader == nil {
		return 0
	}
	lx, ly := sq.Leader.x, sq.Leader.y
	if hasContact {
		return math.Hypot(contactX-lx, contactY-ly)
	}
	return math.Hypot(sq.Leader.endTarget[0]-lx, sq.Leader.endTarget[1]-ly)
}

func (sq *Squad) advancePhase(tick int, next SquadPhase) {
	if next == sq.Phase {
		return
	}
	old := sq.Phase
	sq.Phase = next
	sq.phaseEnteredTick = tick
	if sq.Leader != nil {
		sq.Leader.think(fmt.Sprintf("phase: %s -> %s", old, sq.Phase))
	}
}

func (sq *Squad) updatePhaseWithGuards(tick int, hasContact bool, anyVisibleThreats int, closestDist, spread float64, contactX, contactY float64, terminalStalledCount, aliveCount int) bool {
	if sq.Leader == nil {
		return false
	}
	progressMetric := sq.phaseProgressMetric(hasContact, contactX, contactY)
	if !sq.phaseInit {
		sq.phaseInit = true
		sq.phaseEnteredTick = tick
		sq.lastProgressTick = tick
		sq.lastProgressMetric = progressMetric
	}

	if progressMetric+phaseProgressEpsilon < sq.lastProgressMetric {
		sq.lastProgressMetric = progressMetric
		sq.lastProgressTick = tick
	}

	stalled := tick-sq.lastProgressTick >= phaseStallTicks
	elapsed := tick - sq.phaseEnteredTick
	next := sq.Phase
	manyTerminalStalled := aliveCount >= 2 && terminalStalledCount*2 >= aliveCount

	switch sq.Phase {
	case SquadPhaseApproach:
		if spread > 250 {
			next = SquadPhaseConsolidate
		} else if hasContact && anyVisibleThreats > 0 && closestDist < engageExitDist {
			next = SquadPhaseFixFire
		}
	case SquadPhaseFixFire:
		if spread > 250 {
			next = SquadPhaseConsolidate
		} else if !hasContact {
			next = SquadPhaseApproach
		} else if closestDist < engageEnterDist && elapsed >= phaseMinHoldTicks {
			next = SquadPhaseBound
		}
	case SquadPhaseBound:
		if spread > 260 {
			next = SquadPhaseConsolidate
		} else if !hasContact {
			next = SquadPhaseApproach
		} else if closestDist < engageEnterDist*0.80 && elapsed >= phaseMinHoldTicks {
			next = SquadPhaseAssault
		}
	case SquadPhaseAssault:
		if spread > 260 {
			next = SquadPhaseConsolidate
		} else if !hasContact {
			next = SquadPhaseApproach
		} else if closestDist > engageExitDist && elapsed >= phaseMinHoldTicks {
			next = SquadPhaseFixFire
		}
	case SquadPhaseStalledRecovery:
		if spread > 260 {
			next = SquadPhaseConsolidate
		} else if !hasContact {
			next = SquadPhaseApproach
		} else if elapsed >= phaseRecoveryTicks && !manyTerminalStalled {
			next = SquadPhaseBound
		}
	case SquadPhaseConsolidate:
		if spread < 160 && elapsed >= phaseMinHoldTicks/2 {
			next = SquadPhaseApproach
		}
	}

	if manyTerminalStalled && sq.Phase != SquadPhaseStalledRecovery && elapsed >= phaseMinHoldTicks/2 {
		next = SquadPhaseStalledRecovery
	}

	if stalled {
		switch sq.Phase {
		case SquadPhaseApproach, SquadPhaseFixFire, SquadPhaseBound, SquadPhaseAssault:
			next = SquadPhaseStalledRecovery
		case SquadPhaseStalledRecovery:
			next = SquadPhaseConsolidate
		case SquadPhaseConsolidate:
			next = SquadPhaseApproach
		}
		sq.lastProgressTick = tick
		sq.lastProgressMetric = progressMetric
	}

	if next != sq.Phase && (elapsed >= phaseMinHoldTicks || stalled || next == SquadPhaseConsolidate || next == SquadPhaseStalledRecovery) {
		sq.advancePhase(tick, next)
	}

	return stalled
}

func (sq *Squad) issueOfficerOrder(tick int, kind OfficerCommandKind, targetX, targetY, radius float64, formation FormationType, priority, strength float64, ttl int) {
	if sq.ActiveOrder.Kind == kind && sq.ActiveOrder.State == OfficerOrderActive {
		dx := sq.ActiveOrder.TargetX - targetX
		dy := sq.ActiveOrder.TargetY - targetY
		if sq.ActiveOrder.Formation == formation && withinRadius2(dx, dy, 16*16) {
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

func (sq *Squad) syncOfficerOrder(tick int, hasContact bool, contactX, contactY float64, stalemateActive, forceProactive bool) {
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
		tx, ty := goalX, goalY
		priority, strength := 0.65, 0.80
		if forceProactive {
			if bx, by, ok := sq.claimedBuildingCentroid(); ok {
				tx, ty = bx, by
			}
			priority = 0.84
			strength = 0.92
		}
		sq.issueOfficerOrder(tick, CmdMoveTo, tx, ty, 120, form, priority, strength, 360)

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
		if forceProactive {
			if bx, by, ok := sq.claimedBuildingCentroid(); ok {
				tx, ty = bx, by
			}
		}
		switch sq.Phase {
		case SquadPhaseFixFire:
			if forceProactive || stalemateActive {
				sq.issueOfficerOrder(tick, CmdAssault, tx, ty, 210, sq.Formation, 0.92, 0.98, 220)
			} else {
				sq.issueOfficerOrder(tick, CmdHold, leaderX, leaderY, 170, sq.Formation, 0.80, 0.92, 220)
			}
		case SquadPhaseBound:
			priority, strength := 0.84, 0.95
			if forceProactive {
				priority = 0.90
				strength = 0.98
			}
			sq.issueOfficerOrder(tick, CmdBoundForward, tx, ty, 220, sq.Formation, priority, strength, 220)
		case SquadPhaseAssault:
			sq.issueOfficerOrder(tick, CmdAssault, tx, ty, 230, sq.Formation, 0.88, 0.98, 220)
		case SquadPhaseStalledRecovery:
			if forceProactive {
				sq.issueOfficerOrder(tick, CmdAssault, tx, ty, 220, sq.Formation, 0.94, 0.99, 200)
			} else {
				rx, ry := sq.recoveryRetaskTarget(hasContact, tx, ty)
				sq.issueOfficerOrder(tick, CmdMoveTo, rx, ry, 190, sq.Formation, 0.90, 0.96, 180)
			}
		default:
			if forceProactive {
				sq.issueOfficerOrder(tick, CmdBoundForward, tx, ty, 220, sq.Formation, 0.88, 0.96, 240)
			} else {
				sq.issueOfficerOrder(tick, CmdFanOut, tx, ty, 220, sq.Formation, 0.80, 0.90, 260)
			}
		}
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
	maxVisibleThreatsByMember := 0
	closestDist := math.MaxFloat64
	var contactX, contactY float64
	hasContact := false
	for _, m := range sq.Members {
		if m.state == SoldierStateDead {
			continue
		}
		visibleByMember := 0
		for _, t := range m.blackboard.Threats {
			if !t.IsVisible {
				continue
			}
			anyVisibleThreats++
			visibleByMember++
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
		if visibleByMember > maxVisibleThreatsByMember {
			maxVisibleThreatsByMember = visibleByMember
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

	aliveCount := 0
	terminalStalledCount := 0
	fearSum := 0.0
	moraleSum := 0.0
	injuredAliveCount := 0
	underFireCount := 0
	for _, m := range sq.Members {
		if m.state == SoldierStateDead {
			continue
		}
		aliveCount++
		fearSum += m.profile.Psych.EffectiveFear()
		moraleSum += m.profile.Psych.Morale
		if m.health < soldierMaxHP {
			injuredAliveCount++
		}
		if m.blackboard.IncomingFireCount > 0 || m.blackboard.IsSuppressed() {
			underFireCount++
		}
		if m.blackboard.CurrentGoal != GoalMoveToContact && m.blackboard.CurrentGoal != GoalEngage {
			continue
		}
		pathLen := len(m.path)
		pathRemain := 0
		if m.path != nil && m.pathIndex >= 0 && m.pathIndex < pathLen {
			pathRemain = pathLen - m.pathIndex
		}
		terminal := m.path == nil || pathRemain <= 1
		if m.state == SoldierStateIdle && terminal {
			terminalStalledCount++
		}
	}

	tick := 0
	if sq.Leader != nil && sq.Leader.currentTick != nil {
		tick = *sq.Leader.currentTick
	}

	avgFear := 0.0
	avgMorale := 0.0
	if aliveCount > 0 {
		avgFear = fearSum / float64(aliveCount)
		avgMorale = moraleSum / float64(aliveCount)
	}
	casualties := sq.CasualtyCount()
	casualtyRate := 0.0
	if len(sq.Members) > 0 {
		casualtyRate = float64(casualties) / float64(len(sq.Members))
	}
	stallPressure := 0.0
	if aliveCount > 0 {
		stallPressure = clamp01(float64(terminalStalledCount) / float64(aliveCount))
	}
	spreadPressure := 0.0
	if spread > 160 {
		spreadPressure = clamp01((spread - 160) / 220.0)
	}
	leaderUnderFire := sq.Leader.blackboard.IncomingFireCount > 0 || sq.Leader.blackboard.IsSuppressed()
	fearStressWeight := 0.34
	if leaderUnderFire || anyVisibleThreats > 0 {
		fearStressWeight = 0.50
	}
	prevStress := sq.Stress
	prevCohesion := sq.Cohesion
	prevAvgMorale := sq.AvgMorale
	sq.CasualtyRate = casualtyRate
	sq.Stress = clamp01(avgFear*fearStressWeight + casualtyRate*0.75 + stallPressure*0.20)
	sq.StressDelta = sq.Stress - prevStress
	sq.AvgMorale = avgMorale
	sq.MoraleDelta = sq.AvgMorale - prevAvgMorale

	// Cohesion should degrade from tangible battlefield shocks only:
	// 1) significantly outnumbered, 2) nearby incoming fire/suppression,
	// 3) injuries in the squad, 4) squad deaths.
	outnumberedPressure := 0.0
	if aliveCount > 0 {
		enemyPerFriendly := float64(maxVisibleThreatsByMember) / float64(aliveCount)
		if enemyPerFriendly > 1.75 {
			outnumberedPressure = clamp01((enemyPerFriendly - 1.75) / 1.25)
		}
	}
	incomingFirePressure := 0.0
	if aliveCount > 0 {
		incomingFirePressure = clamp01(float64(underFireCount) / float64(aliveCount))
	}
	deathPressure := casualtyRate

	// Fresh injuries/deaths create a temporary cohesion shock that fades.
	newInjuries := max(0, injuredAliveCount-sq.prevInjuredAliveCount)
	newDeaths := max(0, casualties-sq.prevCasualtyCount)
	sq.cohesionShock = clamp01(sq.cohesionShock*0.90 + float64(newInjuries)*0.18 + float64(newDeaths)*0.35)

	cohesionPressure := clamp01(outnumberedPressure*0.45 + incomingFirePressure*0.75 + deathPressure*1.10 + sq.cohesionShock*0.85)
	cohesionDecay := cohesionPressure * 0.0065
	cohesionRecovery := 0.0
	if cohesionPressure < 0.35 && incomingFirePressure < 0.20 && outnumberedPressure < 0.20 {
		calmFactor := clamp01(1.0 - cohesionPressure)
		cohesionRecovery = (0.0025 + calmFactor*0.0035) * (0.75 + 0.25*avgMorale)
		if casualtyRate == 0 {
			cohesionRecovery += 0.0015
		}
	}
	sq.Cohesion = clamp01(sq.Cohesion - cohesionDecay + cohesionRecovery)
	sq.CohesionDelta = sq.Cohesion - prevCohesion
	sq.prevInjuredAliveCount = injuredAliveCount
	sq.prevCasualtyCount = casualties
	breakPressure := clamp01(sq.Stress + spreadPressure*0.35 + casualtyRate*0.35)
	if sq.Broken {
		sq.breakPressureTicks = 0
		if breakPressure <= squadBreakRecoverThreshold && sq.Cohesion >= 0.45 && tick >= sq.breakLockEnd {
			sq.Broken = false
			if sq.Leader != nil {
				sq.Leader.think("squad reforming — regaining cohesion")
			}
		}
	} else {
		catastrophic := casualtyRate > 0.70
		trigger := breakPressure >= squadBreakPressureThreshold && sq.Cohesion <= 0.35
		if trigger {
			sq.breakPressureTicks++
		} else if sq.breakPressureTicks > 0 {
			sq.breakPressureTicks = max(0, sq.breakPressureTicks-2)
		}
		sustained := sq.breakPressureTicks >= 120
		if catastrophic || sustained {
			sq.Broken = true
			sq.breakLockEnd = tick + squadBreakLockTicks
			sq.breakPressureTicks = 0
			if sq.Leader != nil {
				sq.Leader.think("squad cohesion collapsing — break apart")
			}
		}
	}

	phaseStalled := sq.updatePhaseWithGuards(tick, hasContact, anyVisibleThreats, closestDist, spread, contactX, contactY, terminalStalledCount, aliveCount)
	sq.applyLeaderPhaseSteering(tick, hasContact, closestDist, anyVisibleThreats)
	stalemateActive, forceProactive := sq.updateStalematePressure(tick, hasContact, anyVisibleThreats, closestDist, phaseStalled, terminalStalledCount, aliveCount)

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

	// Decide squad intent (phase-aware, still soft; individual soldiers retain
	// local autonomy and self-preservation via their own utility functions).
	oldIntent := sq.Intent
	var candidateIntent SquadIntentKind
	switch {
	// Cohesion emergency: regroup even under contact if spread is extreme.
	case spread > 250:
		candidateIntent = IntentRegroup
	// Active firefight: any member has LOS on a threat close enough to engage.
	case anyVisibleThreats > 0 && closestDist < engageEnterDist:
		candidateIntent = IntentEngage
	case sq.Intent == IntentEngage && anyVisibleThreats > 0 && closestDist < engageExitDist:
		candidateIntent = IntentEngage
	// Heatmap: heavy danger pressure should only trigger hold when combat pressure
	// is current (incoming fire or close inferred contact), not from stale fear alone.
	case dangerAtPos > 1.5 && anyVisibleThreats == 0 && (leaderUnderFire || (hasContact && closestDist < engageExitDist*1.10)):
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

	// Phase-to-intent shaping: constrains indecisive thrash at squad level while
	// preserving individual agency under fear/suppression/self-preservation.
	switch sq.Phase {
	case SquadPhaseApproach:
		if spread < 120 && candidateIntent == IntentRegroup {
			candidateIntent = IntentAdvance
		}
	case SquadPhaseFixFire:
		if hasContact && candidateIntent == IntentAdvance {
			candidateIntent = IntentHold
		}
	case SquadPhaseBound:
		if hasContact && candidateIntent != IntentRegroup {
			candidateIntent = IntentEngage
		}
	case SquadPhaseAssault:
		if hasContact {
			candidateIntent = IntentEngage
		}
	case SquadPhaseConsolidate:
		if spread > 120 {
			candidateIntent = IntentRegroup
		} else {
			candidateIntent = IntentHold
		}
	case SquadPhaseStalledRecovery:
		if hasContact {
			candidateIntent = IntentEngage
		} else {
			candidateIntent = IntentAdvance
		}
	}

	if phaseStalled && hasContact && candidateIntent != IntentRegroup {
		candidateIntent = IntentEngage
	}
	if forceProactive && !sq.Broken {
		if hasContact {
			candidateIntent = IntentEngage
		} else {
			candidateIntent = IntentAdvance
		}
	}
	if sq.Broken {
		candidateIntent = IntentWithdraw
	}
	criticalIntent := forceProactive || spread > 250 || (anyVisibleThreats > 0 && closestDist < engageEnterDist)
	if candidateIntent != sq.Intent {
		if !criticalIntent && tick < sq.intentLockUntil {
			candidateIntent = sq.Intent
		} else {
			leaderFear := sq.Leader.profile.Psych.EffectiveFear()
			if forceProactive && sq.Leader.blackboard.IncomingFireCount == 0 && !sq.Leader.blackboard.IsSuppressed() {
				leaderFear = math.Min(leaderFear, 0.35)
			}
			if sq.Broken {
				leaderFear = 1.0
			}
			fuzzy := int(math.Abs(math.Sin(float64(tick+sq.ID*17))) * 18)
			sq.intentLockUntil = tick + 30 + int(leaderFear*70) + fuzzy
		}
	}
	sq.Intent = candidateIntent
	sq.syncOfficerOrder(tick, hasContact, contactX, contactY, stalemateActive, forceProactive)

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

	// Build per-member preferred positions for movement intents.
	var moveOrders [][2]float64
	switch sq.Intent {
	case IntentEngage, IntentAdvance:
		moveOrders = sq.preferredOrderPositions(hasContact, contactX, contactY)
	}

	// Officer intervention: if one mobility soldier is clearly stalled, prioritize
	// giving that soldier a direct move order to break local stalemate.
	var stalledPriorityMember *Soldier
	stalledPriorityX, stalledPriorityY := 0.0, 0.0
	stalledPriorityScore := 0.0
	if hasContact && spread < 260 {
		for _, m := range sq.Members {
			if m.state == SoldierStateDead {
				continue
			}
			if sq.lastStalledOrderID == m.id && tick-sq.lastStalledOrderTick < stalledOrderCooldownTicks {
				continue
			}
			if !isCombatMobilityGoal(m.blackboard.CurrentGoal) {
				continue
			}
			pathLen := len(m.path)
			pathRemain := 0
			if m.path != nil && m.pathIndex >= 0 && m.pathIndex < pathLen {
				pathRemain = pathLen - m.pathIndex
			}
			terminal := m.path == nil || pathRemain <= 1
			if !terminal && m.mobilityStallTicks < 24 && m.recoveryNoPathStreak == 0 {
				continue
			}

			score := float64(m.mobilityStallTicks) + float64(m.recoveryNoPathStreak)*30.0 + m.recoveryRouteFailEMA*35.0
			if m.state == SoldierStateIdle && terminal {
				score += 25
			}
			if score <= stalledPriorityScore {
				continue
			}

			tx, ty := m.recoveryTargetHint()
			if spread > 180 && sq.Leader != nil {
				lx, ly := sq.Leader.x, sq.Leader.y
				ab := math.Atan2(ty-ly, tx-lx)
				r := float64(cellSize) * 3.0
				tx = lx + math.Cos(ab)*r
				ty = ly + math.Sin(ab)*r
			}
			stalledPriorityMember = m
			stalledPriorityX, stalledPriorityY = tx, ty
			stalledPriorityScore = score
		}
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
	casualtyPressure := 1.0 - float64(len(alive))/float64(len(sq.Members))
	posture += casualtyPressure * 0.4
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
		if sq.Broken {
			m.blackboard.SquadIntent = IntentWithdraw
		}
		m.blackboard.OrderReceived = true
		m.blackboard.SquadBroken = sq.Broken
		m.blackboard.SquadCasualtyRate = sq.CasualtyRate
		m.blackboard.SquadStress = sq.Stress
		m.blackboard.SquadCohesion = sq.Cohesion
		m.blackboard.OfficerOrderImmediate = false
		m.blackboard.OfficerOrderObedienceChance = 0
		if sq.ActiveOrder.IsActiveAt(tick) {
			obedienceChance := sq.Cohesion
			if m.blackboard.DisobeyingOrders {
				obedienceChance *= 0.55
			}
			if m.blackboard.PanicRetreatActive || m.blackboard.Surrendered {
				obedienceChance = 0
			}
			obeyNow := shouldImmediatelyObeyOrder(tick, m.id, sq.ActiveOrder.ID, obedienceChance)
			priorityScale := 1.0
			strengthScale := 1.0
			if !obeyNow {
				priorityScale = 0.25
				strengthScale = 0.35
			}
			m.blackboard.OfficerOrderKind = sq.ActiveOrder.Kind
			m.blackboard.OfficerOrderTargetX = sq.ActiveOrder.TargetX
			m.blackboard.OfficerOrderTargetY = sq.ActiveOrder.TargetY
			m.blackboard.OfficerOrderRadius = sq.ActiveOrder.Radius
			m.blackboard.OfficerOrderPriority = sq.ActiveOrder.Priority * priorityScale
			m.blackboard.OfficerOrderStrength = sq.ActiveOrder.Strength * strengthScale
			m.blackboard.OfficerOrderActive = true
			m.blackboard.OfficerOrderImmediate = obeyNow
			m.blackboard.OfficerOrderObedienceChance = clamp01(obedienceChance)
		} else {
			m.blackboard.OfficerOrderKind = CmdNone
			m.blackboard.OfficerOrderActive = false
			m.blackboard.OfficerOrderPriority = 0
			m.blackboard.OfficerOrderStrength = 0
			m.blackboard.OfficerOrderImmediate = false
			m.blackboard.OfficerOrderObedienceChance = 0
		}
		if sq.Broken {
			m.blackboard.OfficerOrderKind = CmdNone
			m.blackboard.OfficerOrderActive = false
			m.blackboard.OfficerOrderPriority = 0
			m.blackboard.OfficerOrderStrength = 0
			m.blackboard.OfficerOrderImmediate = false
			m.blackboard.OfficerOrderObedienceChance = 0
		}
		m.blackboard.SquadHasContact = hasContact
		m.blackboard.OutnumberedFactor = outnumberedFactor
		m.blackboard.SquadPosture = posture
		if hasContact {
			m.blackboard.SquadContactX = contactX
			m.blackboard.SquadContactY = contactY
		}
		// Assign a unique spread position for engage orders.
		if m == sq.Leader {
			m.blackboard.HasMoveOrder = false
		} else if !sq.Broken && len(moveOrders) > 0 && orderIdx < len(moveOrders) {
			m.blackboard.OrderMoveX = moveOrders[orderIdx][0]
			m.blackboard.OrderMoveY = moveOrders[orderIdx][1]
			m.blackboard.HasMoveOrder = true
			orderIdx++
		} else {
			m.blackboard.HasMoveOrder = false
		}
		if !sq.Broken && stalledPriorityMember == m {
			m.blackboard.HasMoveOrder = true
			m.blackboard.OrderMoveX = stalledPriorityX
			m.blackboard.OrderMoveY = stalledPriorityY
			m.blackboard.OfficerOrderKind = CmdMoveTo
			m.blackboard.OfficerOrderTargetX = stalledPriorityX
			m.blackboard.OfficerOrderTargetY = stalledPriorityY
			m.blackboard.OfficerOrderRadius = 150
			m.blackboard.OfficerOrderPriority = math.Max(m.blackboard.OfficerOrderPriority, 0.92)
			m.blackboard.OfficerOrderStrength = math.Max(m.blackboard.OfficerOrderStrength, 0.98)
			m.blackboard.OfficerOrderActive = true
			sq.lastStalledOrderTick = tick
			sq.lastStalledOrderID = m.id
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

	if sq.Phase == SquadPhaseStalledRecovery {
		if cleared := sq.clearStalledPathDebt(); cleared > 0 && sq.Leader != nil {
			sq.Leader.think(fmt.Sprintf("recovery: cleared %d stalled paths", cleared))
		}
	}

	// --- Buddy bounding (fire and movement) ---
	// Active only in attack-oriented intents while contact exists and at least 2
	// members are alive. During regroup/hold, disable bounding so everyone can
	// move to restore cohesion instead of half the squad idling as overwatch.
	// Groups alternate: one moves while the other overwatches.
	boundingAllowed := (sq.Intent == IntentAdvance || sq.Intent == IntentEngage) && sq.Phase != SquadPhaseStalledRecovery && !sq.Broken
	if hasContact && len(alive) >= 2 && boundingAllowed {
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
		// Swap cadence:
		// - don't swap faster than 2 seconds (lets movers make progress),
		// - but force a swap after a hard cap to prevent one group starving.
		cycleMinTicks := 120
		cycleMaxTicks := 180
		elapsed := tick - sq.boundCycleTick
		if (allMoversSettled && elapsed >= cycleMinTicks) || elapsed >= cycleMaxTicks {
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
		sq.boundCycleActive = false
		// Clear bound roles — everyone can move freely.
		for _, m := range sq.Members {
			if m.state == SoldierStateDead {
				continue
			}
			m.blackboard.BoundMover = true
		}
	}

	// --- Building takeover ---
	// Leader periodically evaluates nearby buildings along the advance route.
	sq.evaluateBuildings()
	if sq.ClaimedBuildingIdx >= 0 && sq.ClaimedBuildingIdx < len(sq.buildingFootprints) {
		occupants := 0
		for _, m := range sq.Members {
			if m.state == SoldierStateDead {
				continue
			}
			if m.blackboard.AtInterior {
				occupants++
			}
		}
		if occupants > 0 {
			sq.claimedOccupiedTicks++
		} else if sq.claimedOccupiedTicks > 0 {
			sq.claimedOccupiedTicks--
		}
		if hasContact {
			sq.claimedNoContactTicks = 0
		} else {
			sq.claimedNoContactTicks++
		}
		if !hasContact && sq.claimedNoContactTicks > 420 && sq.claimedOccupiedTicks > 150 {
			sq.ClaimedBuildingIdx = -1
			sq.claimedNoContactTicks = 0
			sq.claimedOccupiedTicks = 0
			sq.buildingClaimCooldownUntil = tick + 360
			if sq.Leader != nil {
				sq.Leader.think("building low value — abandon and continue search")
			}
		}
	} else {
		sq.claimedNoContactTicks = 0
		if sq.claimedOccupiedTicks > 0 {
			sq.claimedOccupiedTicks--
		}
	}
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
	if tick < sq.buildingClaimCooldownUntil {
		return
	}
	b := &sq.Leader.blackboard
	claimInterval := buildingClaimInterval
	if b.IncomingFireCount > 0 || b.IsSuppressed() || b.SquadHasContact || b.HeardGunfire {
		claimInterval = 60
	}
	if tick-sq.claimEvalTick < claimInterval {
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

	obsContactX, obsContactY := 0.0, 0.0
	obsHasContact := false
	if b.VisibleThreatCount() > 0 {
		best := math.MaxFloat64
		for _, t := range b.Threats {
			if !t.IsVisible {
				continue
			}
			d := math.Hypot(t.X-lx, t.Y-ly)
			if d < best {
				best = d
				obsContactX, obsContactY = t.X, t.Y
				obsHasContact = true
			}
		}
	} else if b.SquadHasContact {
		obsContactX, obsContactY = b.SquadContactX, b.SquadContactY
		obsHasContact = true
	} else if b.HeardGunfire {
		obsContactX, obsContactY = b.HeardGunfireX, b.HeardGunfireY
		obsHasContact = true
	}

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
		overlapCount := 0
		for _, m := range sq.Members {
			if m.state == SoldierStateDead {
				continue
			}
			if m.x >= float64(fp.x)-float64(cellSize) && m.x <= float64(fp.x+fp.w)+float64(cellSize) &&
				m.y >= float64(fp.y)-float64(cellSize) && m.y <= float64(fp.y+fp.h)+float64(cellSize) {
				overlapCount++
			}
		}
		if dist > 240 && overlapCount == 0 {
			continue
		}
		// Dot product: how much the building is "ahead" of the squad.
		dot := (dx*advX + dy*advY) / dist
		if dot < 0.1 {
			continue // building is behind or to the side
		}
		// Score: prefer ahead and close.
		score := dot*0.6 - dist/maxDist*0.4

		if obsHasContact {
			cdx := obsContactX - lx
			cdy := obsContactY - ly
			cd := math.Hypot(cdx, cdy)
			if cd > 1 {
				cDot := (dx*cdx + dy*cdy) / (dist * cd)
				score += cDot * 0.45
				if cDot < -0.05 {
					score -= 0.35
				}
			}
		}
		// Bigger buildings are more valuable (more cover).
		area := float64(fp.w * fp.h)
		score += math.Min(0.3, area/50000.0)

		if b.IncomingFireCount > 0 || b.IsSuppressed() {
			score += math.Max(0, 0.30-dist/maxDist*0.25)
		}
		if overlapCount > 0 {
			score += 0.20
		}

		if score > bestScore {
			bestScore = score
			bestIdx = i
		}
	}

	if bestIdx >= 0 && bestScore > 0.1 {
		if bestIdx != sq.ClaimedBuildingIdx {
			sq.ClaimedBuildingIdx = bestIdx
			sq.claimedNoContactTicks = 0
			sq.claimedOccupiedTicks = 0
			fp := sq.buildingFootprints[bestIdx]
			sq.Leader.think(fmt.Sprintf("claiming building at (%d,%d)", fp.x, fp.y))
		}
	}
}

// spreadPositions returns spread positions for members during IntentEngage.
// Soldiers are placed in a lateral line perpendicular to the enemy bearing,
// at a standoff distance from the contact point. This prevents the joust pattern
// (running straight at the enemy) by ensuring everyone approaches from a flank angle.
//
//nolint:unused
func (sq *Squad) spreadPositions(cx, cy float64) [][2]float64 {
	alive := sq.Alive()
	n := len(alive)
	if n == 0 {
		return nil
	}

	// Use the squad-level enemy bearing (from centroid), not the leader alone.
	bearing := sq.EnemyBearing
	perpAngle := bearing + math.Pi/2
	tick := 0
	if sq.Leader != nil && sq.Leader.currentTick != nil {
		tick = *sq.Leader.currentTick
	}

	// Phase-aware standoff/spacing keeps movement expressive while preserving cohesion.
	standoff := 180.0
	spacing := 55.0
	switch sq.Phase {
	case SquadPhaseAssault:
		standoff = 150.0
		spacing = 60.0
	case SquadPhaseFixFire:
		standoff = 205.0
		spacing = 52.0
	case SquadPhaseConsolidate:
		standoff = 210.0
		spacing = 46.0
	case SquadPhaseBound:
		standoff = 170.0
		spacing = 58.0
	}
	contactDist := 0.0
	if sq.Leader != nil {
		contactDist = math.Hypot(cx-sq.Leader.x, cy-sq.Leader.y)
	}
	if contactDist > float64(maxFireRange)*1.25 {
		standoff += 18.0
	}
	if sq.squadSpread() > 220 {
		spacing *= 0.88
	}

	// Base point: contact pulled back by standoff along the approach bearing.
	baseX := cx - math.Cos(bearing)*standoff
	baseY := cy - math.Sin(bearing)*standoff

	positions := make([][2]float64, n)
	for i, m := range alive {
		// Symmetric lateral offset: centre is at 0, then alternating ±1, ±2...
		halfN := float64(n-1) / 2.0
		lateral := (float64(i) - halfN) * spacing

		// Deterministic tempo/lane dynamics: soldiers do subtle micro-maneuvers
		// instead of marching on rigid rails.
		osc := math.Sin(float64(tick+m.id*23) * 0.055)
		osc2 := math.Cos(float64(tick*2+m.id*13) * 0.031)
		lateral += osc * 10.0

		depthOffset := osc2 * 12.0
		if (i % 3) == 0 {
			depthOffset += 14.0 // periodic "surge" slot
		} else if (i % 3) == 1 {
			depthOffset -= 8.0 // trailing support slot
		}

		// Fearful soldiers naturally lag a bit; experienced/calm soldiers close distance.
		ef := m.profile.Psych.EffectiveFear()
		depthOffset -= ef * 14.0

		forwardX := math.Cos(bearing) * depthOffset
		forwardY := math.Sin(bearing) * depthOffset
		positions[i] = [2]float64{
			baseX + math.Cos(perpAngle)*lateral + forwardX,
			baseY + math.Sin(perpAngle)*lateral + forwardY,
		}
	}
	return positions
}

// preferredOrderPositions returns leader-directed preferred endpoints that place
// troops slightly ahead and on both flanks of the squad axis.
//
// These are soft targets (HasMoveOrder), not hard lock positions: once soldiers
// arrive, their own utility/cover logic can pull them into better overwatch,
// cover, or direct engagement positions.
func (sq *Squad) preferredOrderPositions(hasContact bool, contactX, contactY float64) [][2]float64 {
	alive := sq.Alive()
	followers := make([]*Soldier, 0, len(alive))
	for _, m := range alive {
		if m == sq.Leader {
			continue
		}
		followers = append(followers, m)
	}
	n := len(followers)
	if n == 0 || sq.Leader == nil {
		return nil
	}

	tick := 0
	if sq.Leader.currentTick != nil {
		tick = *sq.Leader.currentTick
	}

	leaderX, leaderY := sq.Leader.x, sq.Leader.y
	anchorX, anchorY := sq.Leader.endTarget[0], sq.Leader.endTarget[1]
	if sq.ActiveOrder.IsActiveAt(tick) {
		anchorX, anchorY = sq.ActiveOrder.TargetX, sq.ActiveOrder.TargetY
	}
	if hasContact {
		anchorX, anchorY = contactX, contactY
	}

	bearing := sq.Leader.vision.Heading
	if hasContact {
		bearing = sq.EnemyBearing
	} else {
		dx := anchorX - leaderX
		dy := anchorY - leaderY
		if math.Hypot(dx, dy) > 1e-6 {
			bearing = math.Atan2(dy, dx)
		}
	}
	perp := bearing + math.Pi/2

	anchorDist := math.Hypot(anchorX-leaderX, anchorY-leaderY)
	forward := math.Max(leaderPreferredForwardMin, math.Min(leaderPreferredForwardMax, anchorDist*0.42))
	flankSpacing := leaderPreferredFlankBase
	if hasContact {
		flankSpacing += 8
	}
	switch sq.Phase {
	case SquadPhaseFixFire:
		forward *= 0.85
		flankSpacing += 10
	case SquadPhaseBound:
		forward *= 1.05
		flankSpacing += 6
	case SquadPhaseAssault:
		forward *= 1.12
		flankSpacing += 4
	case SquadPhaseConsolidate:
		forward *= 0.90
		flankSpacing -= 8
	}

	assigned := make(map[int][2]float64, n)
	positions := make([][2]float64, n)
	for i, m := range followers {
		flankRank := (i + 1) / 2
		lateral := 0.0
		if i > 0 {
			side := 1.0
			if i%2 == 1 {
				side = -1.0
			}
			lateral = side * flankSpacing * float64(flankRank)
		}

		depth := forward + 14
		if flankRank > 1 {
			depth -= float64(flankRank-1) * 14
		}

		osc := math.Sin(float64(tick+m.id*19) * 0.07)
		depth += osc * 7

		wx := leaderX + math.Cos(bearing)*depth + math.Cos(perp)*lateral
		wy := leaderY + math.Sin(bearing)*depth + math.Sin(perp)*lateral
		wx, wy = adjustFormationTarget(m.navGrid, wx, wy, sq.Leader, sq.Members, assigned)

		positions[i] = [2]float64{wx, wy}
		assigned[m.id] = [2]float64{wx, wy}
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
	max2 := 0.0
	for _, m := range sq.Members {
		if m == sq.Leader || m.state == SoldierStateDead {
			continue
		}
		dx := m.x - sq.Leader.x
		dy := m.y - sq.Leader.y
		if d2 := dx*dx + dy*dy; d2 > max2 {
			max2 = d2
		}
	}
	return math.Sqrt(max2)
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
		if g == GoalEngage || g == GoalFallback || g == GoalFlank || g == GoalOverwatch {
			continue
		}
		if g == GoalMoveToContact && (m.blackboard.VisibleThreatCount() > 0 || m.blackboard.SquadHasContact || m.blackboard.HeardGunfire) {
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
