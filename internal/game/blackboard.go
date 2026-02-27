package game

import "math"

// --- Goal System ---

// GoalKind identifies what a soldier is trying to achieve this tick.
type GoalKind int

const (
	GoalAdvance           GoalKind = iota // push toward objective (far side of map)
	GoalMaintainFormation                 // follow formation slot
	GoalRegroup                           // rally on leader / tighten up
	GoalHoldPosition                      // stay put, scan for threats
	GoalSurvive                           // seek cover / break LOS / flee
	GoalEngage                            // visible enemy: hold ground, face, shoot
	GoalMoveToContact                     // no LOS but squad has contact: close distance
	GoalFallback                          // retreat away from contact under fire
)

func (g GoalKind) String() string {
	switch g {
	case GoalAdvance:
		return "advance"
	case GoalMaintainFormation:
		return "formation"
	case GoalRegroup:
		return "regroup"
	case GoalHoldPosition:
		return "hold"
	case GoalSurvive:
		return "survive"
	case GoalEngage:
		return "engage"
	case GoalMoveToContact:
		return "move_to_contact"
	case GoalFallback:
		return "fallback"
	default:
		return "unknown"
	}
}

// --- Squad Intent ---

// SquadIntentKind is the high-level posture a leader sets for the squad.
type SquadIntentKind int

const (
	IntentAdvance  SquadIntentKind = iota // move toward objective
	IntentHold                            // maintain position, engage targets of opportunity
	IntentRegroup                         // rally on leader
	IntentWithdraw                        // pull back
	IntentEngage                          // active firefight: spread tactically, move to contact
)

func (si SquadIntentKind) String() string {
	switch si {
	case IntentAdvance:
		return "advance"
	case IntentHold:
		return "hold"
	case IntentRegroup:
		return "regroup"
	case IntentWithdraw:
		return "withdraw"
	case IntentEngage:
		return "engage"
	default:
		return "unknown"
	}
}

// --- Threat Facts ---

// ThreatFact is a single remembered enemy contact on the blackboard.
type ThreatFact struct {
	X, Y       float64 // last known position
	Confidence float64 // 0-1, decays over time
	LastTick   int     // tick when last observed
	IsVisible  bool    // true = currently in vision cone this tick
}

// --- Blackboard ---

// Blackboard is a soldier's personal working memory.
type Blackboard struct {
	Threats           []ThreatFact
	CurrentGoal       GoalKind
	SquadIntent       SquadIntentKind // set by leader via order
	OrderReceived     bool            // true if an order was written this cycle
	IncomingFireCount int             // shots received (hit or miss) this tick — reset each tick

	// Shared contact: squad leader writes the last-known enemy position so
	// members without LOS can still move toward the fight.
	SquadContactX   float64
	SquadContactY   float64
	SquadHasContact bool // true when the squad has an active known contact

	// Per-member move order: leader assigns each member a spread position to
	// advance toward during IntentEngage, rather than all converging on one point.
	OrderMoveX   float64
	OrderMoveY   float64
	HasMoveOrder bool
}

// UpdateThreats refreshes the blackboard from the current vision contacts.
// Existing threats that are still visible get refreshed; new contacts are added;
// stale contacts decay in confidence.
func (bb *Blackboard) UpdateThreats(contacts []*Soldier, currentTick int) {
	// Mark all existing as not visible this tick.
	for i := range bb.Threats {
		bb.Threats[i].IsVisible = false
	}

	// Refresh or add from current contacts.
	for _, c := range contacts {
		found := false
		for i := range bb.Threats {
			dx := bb.Threats[i].X - c.x
			dy := bb.Threats[i].Y - c.y
			if math.Sqrt(dx*dx+dy*dy) < 40 {
				// Same contact — refresh.
				bb.Threats[i].X = c.x
				bb.Threats[i].Y = c.y
				bb.Threats[i].Confidence = 1.0
				bb.Threats[i].LastTick = currentTick
				bb.Threats[i].IsVisible = true
				found = true
				break
			}
		}
		if !found {
			bb.Threats = append(bb.Threats, ThreatFact{
				X: c.x, Y: c.y,
				Confidence: 1.0,
				LastTick:   currentTick,
				IsVisible:  true,
			})
		}
	}

	// Decay stale threats.
	kept := bb.Threats[:0]
	for _, t := range bb.Threats {
		if !t.IsVisible {
			age := float64(currentTick - t.LastTick)
			t.Confidence = math.Max(0, t.Confidence-age*0.005)
		}
		if t.Confidence > 0.01 {
			kept = append(kept, t)
		}
	}
	bb.Threats = kept
}

// VisibleThreatCount returns how many threats are currently visible.
func (bb *Blackboard) VisibleThreatCount() int {
	n := 0
	for _, t := range bb.Threats {
		if t.IsVisible {
			n++
		}
	}
	return n
}

// ClosestVisibleThreatDist returns the distance to the nearest visible threat
// from the given position. Returns math.MaxFloat64 if none visible.
func (bb *Blackboard) ClosestVisibleThreatDist(x, y float64) float64 {
	best := math.MaxFloat64
	for _, t := range bb.Threats {
		if !t.IsVisible {
			continue
		}
		dx := t.X - x
		dy := t.Y - y
		d := math.Sqrt(dx*dx + dy*dy)
		if d < best {
			best = d
		}
	}
	return best
}

// --- Goal Utility Scoring ---

// SelectGoal evaluates competing goals and returns the highest-utility one.
func SelectGoal(bb *Blackboard, profile *SoldierProfile, isLeader bool, hasPath bool) GoalKind {
	visibleThreats := bb.VisibleThreatCount()
	underFire := bb.IncomingFireCount > 0

	// --- Engage: visible enemy, controlled fire.
	// This is the default combat response for composed soldiers — it must win
	// for a long time before fear erodes it enough for fallback/survive to take over.
	engageUtil := 0.0
	if visibleThreats > 0 {
		engageUtil = 0.75 + profile.Skills.Discipline*0.2
		// Fear degrades willingness only significantly once EffectiveFear > 0.5.
		engageUtil -= profile.Psych.EffectiveFear() * 0.5
		if underFire {
			engageUtil += 0.05 // adrenaline — already in a fight
		}
	}

	// --- Fallback: mobile retreat under fire.
	// Activates at moderate fear (EffectiveFear ~0.4+) for low-discipline soldiers.
	// Distinct from Survive (freeze/catatonic) — soldier actively moves away.
	fallbackUtil := 0.0
	if underFire && bb.SquadHasContact {
		ef := profile.Psych.EffectiveFear()
		// Only becomes competitive once fear has meaningfully accumulated.
		if ef > 0.25 {
			fallbackUtil = (ef-0.25)*1.4 + 0.1*float64(bb.IncomingFireCount)
			fallbackUtil -= profile.Skills.Discipline * 0.5
		}
	}

	// --- Survive: panic / freeze / seek cover — last resort.
	// Requires very high effective fear. A composed veteran never reaches this;
	// a green low-composure soldier breaks sooner but fallback activates first.
	surviveUtil := 0.0
	ef := profile.Psych.EffectiveFear()
	if ef > 0.55 {
		surviveUtil = (ef - 0.55) * 2.0
		if underFire {
			surviveUtil += 0.05 * float64(bb.IncomingFireCount)
		}
	}

	// --- MoveToContact: squad has contact, I don't — close in.
	moveToContactUtil := 0.0
	if bb.SquadHasContact && visibleThreats == 0 {
		moveToContactUtil = 0.55 + profile.Skills.Discipline*0.1
		// Fear reduces eagerness to close in.
		moveToContactUtil -= profile.Psych.EffectiveFear() * 0.3
	}

	// --- Regroup: cohesion emergency.
	regroupUtil := 0.0
	if bb.SquadIntent == IntentRegroup && !isLeader {
		regroupUtil = 0.75 + profile.Skills.Discipline*0.2
		if visibleThreats > 0 {
			regroupUtil *= 0.6
		}
	}

	// --- Hold: disciplined stationary fire.
	holdUtil := 0.0
	if bb.SquadIntent == IntentHold {
		holdUtil = 0.5 + profile.Skills.Discipline*0.25
	}

	// --- Formation: follow slot when advancing without contact.
	formationUtil := 0.0
	if !isLeader {
		switch bb.SquadIntent {
		case IntentAdvance:
			formationUtil = 0.45 + profile.Skills.Discipline*0.2
			if visibleThreats > 0 {
				formationUtil *= 0.2
			}
		case IntentHold:
			formationUtil = 0.3 + profile.Skills.Discipline*0.1
			if visibleThreats > 0 {
				formationUtil *= 0.4
			}
		case IntentRegroup:
			formationUtil = 0.55 + profile.Skills.Discipline*0.15
			if visibleThreats > 0 {
				formationUtil *= 0.7
			}
		case IntentEngage:
			// In a firefight, formation is irrelevant — don't pull people back to slots.
			formationUtil = 0.1
		}
	}

	// --- Advance: baseline objective drive.
	advanceUtil := 0.3
	if bb.SquadIntent == IntentAdvance {
		advanceUtil = 0.5
	}
	if visibleThreats > 0 {
		advanceUtil *= 0.05
	}
	if isLeader {
		advanceUtil += 0.1
	}

	// --- Pick highest utility ---
	best := GoalAdvance
	bestVal := advanceUtil

	check := func(g GoalKind, u float64) {
		if u > bestVal {
			best = g
			bestVal = u
		}
	}

	check(GoalMaintainFormation, formationUtil)
	check(GoalRegroup, regroupUtil)
	check(GoalHoldPosition, holdUtil)
	check(GoalEngage, engageUtil)
	check(GoalMoveToContact, moveToContactUtil)
	check(GoalFallback, fallbackUtil)
	check(GoalSurvive, surviveUtil)

	return best
}
