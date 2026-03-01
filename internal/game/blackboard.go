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
	GoalFlank                             // move perpendicular to enemy then decision point
	GoalOverwatch                         // hold a high-sightline position, scan for threats
	GoalPeek                              // cautious peek around a corner or through a window
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
	case GoalFlank:
		return "flank"
	case GoalOverwatch:
		return "overwatch"
	case GoalPeek:
		return "peek"
	default:
		return "unknown"
	}

}

func overwatchDistanceFactor(contactRange float64) float64 {
	if contactRange <= 0 {
		return 1.0
	}
	maxPractical := float64(maxFireRange)
	if contactRange <= maxPractical {
		return 1.0
	}
	tooFar := maxPractical * 2.0
	if contactRange >= tooFar {
		return 0.15
	}
	t := (contactRange - maxPractical) / (tooFar - maxPractical)
	return 1.0 - t*0.85
}

// --- Officer Orders ---

// OfficerCommandKind is an explicit command issued by a squad leader.
// Commands are strong positive signals, not hard overrides.
type OfficerCommandKind int

const (
	CmdNone OfficerCommandKind = iota
	CmdMoveTo
	CmdHold
	CmdRegroup
	CmdBoundForward
	CmdForm
	CmdFanOut
	CmdAssault
)

func (oc OfficerCommandKind) String() string {
	switch oc {
	case CmdMoveTo:
		return "move_to"
	case CmdHold:
		return "hold"
	case CmdRegroup:
		return "regroup"
	case CmdBoundForward:
		return "bound"
	case CmdForm:
		return "form"
	case CmdFanOut:
		return "fan_out"
	case CmdAssault:
		return "assault"
	default:
		return "none"
	}
}

// OfficerOrderState tracks the lifecycle of an officer order.
type OfficerOrderState int

const (
	OfficerOrderInactive OfficerOrderState = iota
	OfficerOrderActive
	OfficerOrderExpired
	OfficerOrderCancelled
)

// OfficerOrder is the squad-level command emitted by the current squad leader.
type OfficerOrder struct {
	ID          int
	Kind        OfficerCommandKind
	IssuedTick  int
	ExpiresTick int
	Priority    float64 // 0..1
	Strength    float64 // 0..1
	TargetX     float64
	TargetY     float64
	Radius      float64
	Formation   FormationType
	State       OfficerOrderState
}

// IsActiveAt reports whether this order should currently influence behaviour.
func (o OfficerOrder) IsActiveAt(tick int) bool {
	if o.State != OfficerOrderActive || o.Kind == CmdNone {
		return false
	}
	if o.ExpiresTick > 0 && tick > o.ExpiresTick {
		return false
	}
	return true
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
	Source     *Soldier // identity pointer — nil for old positional memories
	X, Y       float64  // last known position
	Confidence float64  // 0-1, decays over time
	LastTick   int      // tick when last observed
	IsVisible  bool     // true = currently in vision cone this tick
}

// --- Blackboard ---

// Blackboard is a soldier's personal working memory.
type Blackboard struct {
	Threats           []ThreatFact
	CurrentGoal       GoalKind
	SquadIntent       SquadIntentKind // set by leader via order
	OrderReceived     bool            // true if an order was written this cycle
	IncomingFireCount int             // shots received (hit or miss) this tick — reset each tick

	// Active officer-order signal propagated from squad leader.
	OfficerOrderKind     OfficerCommandKind
	OfficerOrderTargetX  float64
	OfficerOrderTargetY  float64
	OfficerOrderRadius   float64
	OfficerOrderPriority float64 // 0..1
	OfficerOrderStrength float64 // 0..1
	OfficerOrderActive   bool

	// Internal models each soldier's personal tactical drive for this tick.
	Internal SoldierInternalGoals

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

	// --- Decision pacing ---
	PanicLocked bool // true when fear is so high the soldier can't decide effectively

	// --- Commitment-based decision architecture ---
	// ShatterPressure accumulates from incoming fire, suppression spikes, etc.
	// Only when it exceeds ShatterThreshold does a re-evaluation actually fire.
	// Discipline raises the threshold — veterans are harder to rattle.
	ShatterPressure  float64 // 0+, accumulates from disruptive events
	ShatterThreshold float64 // set once from discipline; typically 0.4–0.8

	// Commitment phase: commit → sustain → review.
	// During commit (first N ticks), shatter pressure is halved (soldier is focused).
	// During sustain, normal pressure applies.
	// During review (lock expired), full re-evaluation runs.
	CommitPhaseTick int // tick when current commitment phase started
	CommitDuration  int // ticks of the commit (immune) phase
	SustainDuration int // ticks of the sustain phase (after commit ends)

	// Decision debt: switching goals incurs a penalty that makes the *next*
	// switch harder. Decays over ~3s. Prevents A→B→A→B cycling.
	DecisionDebt float64 // 0+, added to hysteresis margin on each switch

	// GoalStreak counts consecutive evaluations the same goal won.
	GoalStreak int

	// Hysteresis: the current goal gets a loyalty bonus in SelectGoal.
	// To switch, a competing goal must exceed current by HysteresisMargin.
	// The margin grows with streak depth and decision debt.
	HysteresisMargin float64

	// Legacy fields still needed.
	NextDecisionTick int  // tick at which next review phase begins
	ShatterEvent     bool // kept for backward compat — converted to pressure internally
	IdleCombatTicks  int  // ticks spent far from both combat and allies
	ForceAdvance     bool // override: ignore other drives, move toward nearest ally/contact

	// --- Gunfire hearing (single-tick) ---
	HeardGunfireX    float64 // world position of last heard gunfire
	HeardGunfireY    float64
	HeardGunfire     bool // true if gunfire was heard THIS tick (reset each tick)
	HeardGunfireTick int  // tick when last gunfire was heard

	// --- Combat memory (persistent) ---
	// CombatMemoryStrength decays from 1→0 over ~3600 ticks (60s at 60TPS).
	// Soldiers with memory > 0 are "activated" and will not idle passively.
	CombatMemoryStrength float64 // 0-1, how strongly combat is remembered
	CombatMemoryX        float64 // last gunfire position stored in memory
	CombatMemoryY        float64

	// --- Flanking state ---
	FlankComplete bool    // true when the perpendicular leg is done
	FlankTargetX  float64 // world position of flank waypoint
	FlankTargetY  float64

	// --- Sightline awareness ---
	LocalSightlineScore float64 // 0-1, how many cells visible from current position

	// --- Peeking state ---
	// PeekNoContactCount counts how many consecutive empty peeks at nearby corners.
	// Reduces peek utility so soldiers don't obsessively peek the same empty spot.
	PeekNoContactCount int
	PeekCooldown       int // ticks remaining before another peek is allowed

	// --- Shoot outcome tracking ---
	// ConsecutiveMisses counts unbroken run of missed shots.
	// After 3+, forces a behavioural re-evaluation.
	ConsecutiveMisses int

	// --- Tactical position awareness ---
	PositionDesirability float64 // -1..+1, how tactically valuable current position is
	AtCorner             bool    // true if at a building corner
	AtDoorway            bool    // true if standing in a doorway (bad)
	AtWall               bool    // true if adjacent to a wall
	AtWindowAdj          bool    // true if at an interior cell next to a window (overwatch)
	AtInterior           bool    // true if inside a building footprint

	// --- Position scanning result ---
	// BestNearbyX/Y is the world position of the best nearby tile found
	// by the periodic position scanner. Score is its desirability.
	BestNearbyX     float64
	BestNearbyY     float64
	BestNearbyScore float64
	HasBestNearby   bool // true if a good position was found

	// --- Building takeover ---
	// ClaimedBuildingIdx is the index of the building footprint the squad has claimed.
	// -1 means no building is claimed.
	ClaimedBuildingIdx int
	ClaimedBuildingX   float64 // centroid of claimed building
	ClaimedBuildingY   float64

	// --- Morale-driven reinforcement ---
	// Set by SquadThink when a calm soldier is directed toward a distressed one.
	ShouldReinforce  bool
	ReinforceMemberX float64
	ReinforceMemberY float64

	// --- Flanking assignment ---
	// FlankSide: +1 = flank left (bearing - 90°), -1 = flank right (bearing + 90°).
	// Set by SquadThink so members fan out consistently relative to the enemy normal.
	FlankSide         float64
	SquadEnemyBearing float64 // bearing from squad centroid to enemy, radians

	// --- Outnumbered awareness ---
	// OutnumberedFactor: >1 means we see more enemies than friendlies see enemies
	// (we're outnumbered). <1 means we outnumber them. 1.0 = even.
	// Updated by SquadThink each cycle.
	OutnumberedFactor float64

	// --- Squad posture tracking ---
	// SquadPosture: -1 = full defensive, 0 = balanced, +1 = full offensive.
	// Derived from OutnumberedFactor, squad intent, and casualty state.
	SquadPosture float64

	// --- Social awareness ---
	// VisibleAllyCount is the number of squadmates currently visible to this soldier.
	VisibleAllyCount int
	// SquadAvgFear is the smoothed effective fear of visible/alive squadmates.
	SquadAvgFear float64
	// SquadFearDelta is the per-tick change in SquadAvgFear.
	// Positive values mean ally stress is rising.
	SquadFearDelta float64
	// IsolatedTicks counts consecutive ticks with no visible enemies and no visible allies.
	IsolatedTicks int
	// CloseAllyPressure is a 0..1 crowding signal based on nearby ally spacing.
	// Higher values mean allies are too close and spacing movement is needed.
	CloseAllyPressure float64

	// --- Buddy bounding (squad-level fire and movement) ---
	// BoundGroup: 0 or 1, assigned by squad leader. Groups alternate: one moves
	// while the other overwatches during MoveToContact.
	BoundGroup int
	// BoundRole: true = this soldier should move this cycle; false = overwatch.
	// Toggled by SquadThink each bound cycle.
	BoundMover bool

	// --- Suppression state ---
	// SuppressLevel is a persistent 0-1 value representing how pinned down
	// this soldier is. Unlike IncomingFireCount (reset every tick), this
	// accumulates from hits and near-misses and decays over several seconds.
	// A suppressed soldier stays suppressed across ticks — eliminating the
	// oscillation caused by per-tick fire count resets.
	SuppressLevel float64
	// SuppressDir is the bearing (radians) of the fire that is suppressing
	// this soldier, used to orient cover-seeking correctly.
	SuppressDir float64
	// suppressSpiked is true during the tick when suppression first crosses
	// the SuppressThreshold — used to trigger an immediate ShatterEvent.
	suppressSpiked bool
	// suppressed is a hysteresis-smoothed suppression state.
	// Enter at SuppressThreshold, clear at suppressClearThreshold.
	suppressed bool
}

// GoalThresholds controls when a soldier prefers to shoot, push, or seek safety.
type GoalThresholds struct {
	EngageShotQuality    float64
	LongRangeShotQuality float64
	PushOnMissMomentum   float64
	HoldOnHitMomentum    float64
	CoverFear            float64
}

// SoldierInternalGoals captures per-soldier tactical pressures that evolve over time.
type SoldierInternalGoals struct {
	ShootDesire            float64
	MoveDesire             float64
	CoverDesire            float64
	ShotMomentum           float64
	LastEstimatedHitChance float64
	LastRange              float64
	LastContactRange       float64
	Thresholds             GoalThresholds
	// ThresholdAge is how long (ticks) this threshold set has been active.
	// Used to drive adaptive drift: thresholds slowly shift toward current conditions.
	ThresholdAge int
}

func defaultGoalThresholds() GoalThresholds {
	return GoalThresholds{
		EngageShotQuality:    0.25, // engage even at modest accuracy
		LongRangeShotQuality: 0.30, // push forward if hit chance below 30%
		PushOnMissMomentum:   0.20, // close range after fewer misses
		HoldOnHitMomentum:    0.20,
		CoverFear:            0.70, // only seek cover when genuinely scared
	}
}

func (bb *Blackboard) ensureInternalDefaults() {
	if bb.Internal.Thresholds.EngageShotQuality <= 0 {
		bb.Internal.Thresholds = defaultGoalThresholds()
	}
	// Default shatter threshold if not yet initialised (set properly by InitCommitment).
	if bb.ShatterThreshold <= 0 {
		bb.ShatterThreshold = defaultShatterThreshold
	}
}

// --- Commitment-based decision constants ---
const (
	defaultShatterThreshold = 0.50        // base pressure needed to force re-evaluation
	shatterPressureDecay    = 0.012       // per tick decay (~83 ticks to fully clear from 1.0)
	commitPhaseTicks        = 60          // ticks of commit phase (~1s) — shatter-resistant
	sustainPhaseTicks       = 90          // ticks of sustain phase (~1.5s) — normal
	hysteresisBase          = 0.12        // base loyalty bonus for current goal
	hysteresisStreakScale   = 0.02        // additional per consecutive same-goal selection
	hysteresisDebtScale     = 0.08        // scaling from decision debt into margin
	decisionDebtPerSwitch   = 0.40        // debt added each time the goal changes
	decisionDebtDecay       = 1.0 / 180.0 // per-tick decay (~3s to clear)
	desireEMAAlpha          = 0.08        // blending factor for smoothed desires (lower = smoother)
)

// InitCommitment sets up the shatter threshold from the soldier's discipline.
// Called once when the soldier is created or profiles are randomised.
func (bb *Blackboard) InitCommitment(discipline float64) {
	// Disciplined soldiers: threshold 0.6–0.8 (hard to rattle).
	// Green soldiers: threshold 0.35–0.5 (easily disrupted).
	bb.ShatterThreshold = 0.35 + discipline*0.45
}

// AddShatterPressure adds disruptive pressure from an external event.
// The amount is scaled down during the commit phase.
func (bb *Blackboard) AddShatterPressure(amount float64, tick int) {
	phase := bb.CommitPhase(tick)
	switch phase {
	case 0: // commit — soldier is focused, halve incoming pressure
		amount *= 0.5
	case 1: // sustain — normal
		// no scaling
	case 2: // review — slightly amplified, soldier is already reconsidering
		amount *= 1.2
	}
	bb.ShatterPressure += amount
}

// DecayShatterPressure should be called once per tick.
func (bb *Blackboard) DecayShatterPressure() {
	bb.ShatterPressure = math.Max(0, bb.ShatterPressure-shatterPressureDecay)
}

// ShatterReady returns true if accumulated pressure has exceeded the threshold.
// Consuming the shatter resets pressure to zero.
func (bb *Blackboard) ShatterReady() bool {
	if bb.ShatterPressure >= bb.ShatterThreshold {
		bb.ShatterPressure = 0
		return true
	}
	return false
}

// CommitPhase returns 0=commit, 1=sustain, 2=review based on current tick.
func (bb *Blackboard) CommitPhase(tick int) int {
	elapsed := tick - bb.CommitPhaseTick
	if elapsed < bb.CommitDuration {
		return 0 // commit
	}
	if elapsed < bb.CommitDuration+bb.SustainDuration {
		return 1 // sustain
	}
	return 2 // review
}

// BeginCommitment starts a new commitment cycle for the selected goal.
func (bb *Blackboard) BeginCommitment(tick int, sameGoal bool, stress float64) {
	bb.CommitPhaseTick = tick
	bb.ShatterPressure = 0 // clean slate for new commitment

	if sameGoal {
		bb.GoalStreak++
	} else {
		// Goal changed — incur decision debt.
		bb.DecisionDebt += decisionDebtPerSwitch
		bb.GoalStreak = 0
	}

	// Commit duration scales with streak depth and stress.
	// Experienced commitment: soldiers who've been in a goal longer get more immunity.
	streakBonus := math.Log1p(float64(bb.GoalStreak)) * 15 // 0→0, 5→27, 10→36 ticks
	stressBonus := stress * 30                             // stressed soldiers commit longer
	bb.CommitDuration = commitPhaseTicks + int(streakBonus+stressBonus)
	bb.SustainDuration = sustainPhaseTicks

	// Update hysteresis margin: grows with streak and debt.
	bb.HysteresisMargin = hysteresisBase +
		float64(bb.GoalStreak)*hysteresisStreakScale +
		bb.DecisionDebt*hysteresisDebtScale
	if bb.HysteresisMargin > 0.40 {
		bb.HysteresisMargin = 0.40 // cap — never make switching impossible
	}

	// Set NextDecisionTick to the start of review phase.
	bb.NextDecisionTick = tick + bb.CommitDuration + bb.SustainDuration
}

// DecayDecisionDebt should be called once per tick.
func (bb *Blackboard) DecayDecisionDebt() {
	if bb.DecisionDebt > 0 {
		bb.DecisionDebt = math.Max(0, bb.DecisionDebt-decisionDebtDecay)
	}
}

// EvolveThresholds drifts the soldier's thresholds toward current conditions
// to prevent ping-pong oscillation. Called once per decision tick.
//
// The core idea: the longer a soldier stays in a goal, the more their thresholds
// shift to reinforce it (commitment grows). When the goal changes the thresholds
// reset back toward defaults gradually.
//
// Additionally:
//   - Under sustained incoming fire → CoverFear threshold lowers (they become more cover-seeking)
//   - With high hit momentum      → EngageShotQuality lowers (they become bolder)
//   - With sustained misses       → LongRangeShotQuality rises (they give up on long shots)
func (bb *Blackboard) EvolveThresholds(currentGoal GoalKind, stress float64) {
	bb.ensureInternalDefaults()
	th := &bb.Internal.Thresholds
	def := defaultGoalThresholds()
	bb.Internal.ThresholdAge++

	// Drift rate: slow so thresholds take ~300 ticks to fully move.
	const driftRate = 1.0 / 300.0

	// Adaptive drift based on current goal — reinforce the current behaviour.
	switch currentGoal {
	case GoalEngage, GoalFlank, GoalMoveToContact:
		// Soldier is actively fighting — lower the bar to keep engaging.
		th.EngageShotQuality -= driftRate * 0.5
		th.LongRangeShotQuality -= driftRate * 0.3
		th.PushOnMissMomentum += driftRate * 0.2
	case GoalFallback, GoalSurvive:
		// Soldier is retreating — raise the bar to re-engage (harder to pull back in).
		th.CoverFear -= driftRate * 0.8
		th.EngageShotQuality += driftRate * 0.6
	case GoalHoldPosition, GoalOverwatch:
		// Holding — become slightly more trigger-happy over time.
		th.EngageShotQuality -= driftRate * 0.4
		th.CoverFear += driftRate * 0.3
	}

	// Fear-driven drift: sustained incoming fire lowers cover threshold.
	if bb.IncomingFireCount > 0 {
		th.CoverFear -= driftRate * float64(bb.IncomingFireCount) * 0.5
	}

	// Hit-streak drift: hitting the enemy makes soldier bolder.
	if bb.Internal.ShotMomentum > 0.3 {
		th.EngageShotQuality -= driftRate * bb.Internal.ShotMomentum * 0.6
	}
	// Miss-streak drift: sustained misses raise long-range quality bar.
	if bb.Internal.ShotMomentum < -0.3 {
		th.LongRangeShotQuality += driftRate * (-bb.Internal.ShotMomentum) * 0.4
	}

	// Stress-based reset: under high stress thresholds slowly snap back to defaults.
	// This prevents a cowering soldier from permanently suppressing their engage drive.
	resetRate := driftRate * stress * 0.5
	th.EngageShotQuality += (def.EngageShotQuality - th.EngageShotQuality) * resetRate
	th.LongRangeShotQuality += (def.LongRangeShotQuality - th.LongRangeShotQuality) * resetRate
	th.CoverFear += (def.CoverFear - th.CoverFear) * resetRate

	// Hard clamp: thresholds must stay in sensible ranges.
	th.EngageShotQuality = clamp(th.EngageShotQuality, 0.05, 0.60)
	th.LongRangeShotQuality = clamp(th.LongRangeShotQuality, 0.10, 0.65)
	th.PushOnMissMomentum = clamp(th.PushOnMissMomentum, 0.05, 0.60)
	th.HoldOnHitMomentum = clamp(th.HoldOnHitMomentum, 0.05, 0.50)
	th.CoverFear = clamp(th.CoverFear, 0.30, 0.90)
}

// UpdateGoalStreak is kept for backward compatibility but now delegates
// to BeginCommitment. Returns the total lock duration (commit + sustain).
func (bb *Blackboard) UpdateGoalStreak(sameGoal bool, stress float64) int {
	// Tick is needed but not available here — callers should use BeginCommitment directly.
	// This shim returns a reasonable interval.
	if sameGoal {
		bb.GoalStreak++
	} else {
		bb.DecisionDebt += decisionDebtPerSwitch
		bb.GoalStreak = 0
	}
	streakBonus := math.Log1p(float64(bb.GoalStreak)) * 15
	stressBonus := stress * 30
	commit := commitPhaseTicks + int(streakBonus+stressBonus)
	return commit + sustainPhaseTicks
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// emaBlend smoothly blends a previous value toward a new target using exponential moving average.
// alpha controls responsiveness: 0.08 = smooth (12-tick half-life), 1.0 = instant.
func emaBlend(prev, target, alpha float64) float64 {
	return prev*(1.0-alpha) + target*alpha
}

// RefreshInternalGoals updates tactical desires from current threat geometry,
// personal profile, and recent firing momentum.
// Desires are EMA-smoothed to prevent frame-to-frame noise from causing oscillation.
func (bb *Blackboard) RefreshInternalGoals(profile *SoldierProfile, selfX, selfY float64) {
	bb.ensureInternalDefaults()

	dist := bb.ClosestVisibleThreatDist(selfX, selfY)
	if dist == math.MaxFloat64 {
		contactDist := math.MaxFloat64
		if bb.SquadHasContact {
			contactDist = math.Hypot(bb.SquadContactX-selfX, bb.SquadContactY-selfY)
		}
		if bb.HeardGunfire {
			audioDist := math.Hypot(bb.HeardGunfireX-selfX, bb.HeardGunfireY-selfY)
			if audioDist < contactDist {
				contactDist = audioDist
			}
		}
		if bb.IsActivated() {
			memoryDist := math.Hypot(bb.CombatMemoryX-selfX, bb.CombatMemoryY-selfY)
			if memoryDist < contactDist {
				contactDist = memoryDist
			}
		}
		if contactDist < math.MaxFloat64 {
			bb.Internal.LastContactRange = contactDist
		} else {
			bb.Internal.LastContactRange = 0
		}
		bb.Internal.LastRange = 0
		bb.Internal.LastEstimatedHitChance = 0

		rawShoot := 0.0
		rawMove := 0.35 + math.Max(0, -bb.Internal.ShotMomentum)*0.25
		if bb.IsActivated() {
			curiosity := bb.CombatMemoryStrength * (0.55 + profile.Skills.Discipline*0.20)
			curiosity *= (1.0 - profile.Psych.EffectiveFear()*0.6)
			rawMove += curiosity
		}
		rawCover := profile.Psych.EffectiveFear() * 0.8

		bb.Internal.ShootDesire = emaBlend(bb.Internal.ShootDesire, clamp01(rawShoot), desireEMAAlpha)
		bb.Internal.MoveDesire = emaBlend(bb.Internal.MoveDesire, clamp01(rawMove), desireEMAAlpha)
		bb.Internal.CoverDesire = emaBlend(bb.Internal.CoverDesire, clamp01(rawCover), desireEMAAlpha)
		return
	}

	estHitChance := estimateHitChanceAtRange(profile, dist)
	fear := profile.Psych.EffectiveFear()
	rangePressure := clamp01((dist - accurateFireRange) / (maxFireRange - accurateFireRange))

	bb.Internal.LastRange = dist
	bb.Internal.LastContactRange = dist
	bb.Internal.LastEstimatedHitChance = estHitChance

	rawShoot := clamp01(estHitChance + bb.Internal.ShotMomentum*0.30 - fear*0.20)
	rawMove := clamp01((1.0-estHitChance)*0.55 + rangePressure*0.35 + math.Max(0, -bb.Internal.ShotMomentum)*0.25)
	rawCover := clamp01(fear*0.8 + float64(bb.IncomingFireCount)*0.08)

	bb.Internal.ShootDesire = emaBlend(bb.Internal.ShootDesire, rawShoot, desireEMAAlpha)
	bb.Internal.MoveDesire = emaBlend(bb.Internal.MoveDesire, rawMove, desireEMAAlpha)
	bb.Internal.CoverDesire = emaBlend(bb.Internal.CoverDesire, rawCover, desireEMAAlpha)
}

// RecordShotOutcome updates momentum and consecutive-miss counter.
// Returns true when the miss streak is long enough to force a behaviour change.
func (bb *Blackboard) RecordShotOutcome(hit bool, expectedHitChance, shotDist float64) bool {
	bb.ensureInternalDefaults()

	if hit {
		bb.Internal.ShotMomentum += 0.12 + expectedHitChance*0.08
		bb.ConsecutiveMisses = 0
	} else {
		bb.Internal.ShotMomentum -= 0.10 + (1.0-expectedHitChance)*0.06
		bb.ConsecutiveMisses++
	}
	bb.Internal.ShotMomentum = math.Max(-1.0, math.Min(1.0, bb.Internal.ShotMomentum))
	bb.Internal.LastEstimatedHitChance = expectedHitChance
	bb.Internal.LastRange = shotDist

	// After 3 consecutive misses, signal a forced re-evaluation.
	if !hit && bb.ConsecutiveMisses >= 3 {
		bb.ConsecutiveMisses = 0 // reset counter so we don't fire every tick
		return true
	}
	return false
}

func estimateHitChanceAtRange(profile *SoldierProfile, dist float64) float64 {
	accuracy := profile.EffectiveAccuracy()
	return clamp01(accuracy - shotRangePenalty(dist))
}

// UpdateThreats refreshes the blackboard from the current vision contacts.
// Existing threats are matched by pointer identity so a moving soldier's
// position is always up-to-date and dead soldiers are purged immediately.
func (bb *Blackboard) UpdateThreats(contacts []*Soldier, currentTick int) {
	// Mark all existing as not visible this tick.
	for i := range bb.Threats {
		bb.Threats[i].IsVisible = false
	}

	// Refresh or add from current contacts.
	for _, c := range contacts {
		// Dead contacts are never added or refreshed.
		if c.state == SoldierStateDead {
			continue
		}
		found := false
		for i := range bb.Threats {
			if bb.Threats[i].Source == c {
				// Identity match — always use current position.
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
				Source:     c,
				X:          c.x,
				Y:          c.y,
				Confidence: 1.0,
				LastTick:   currentTick,
				IsVisible:  true,
			})
		}
	}

	// Decay stale threats and purge dead sources.
	kept := bb.Threats[:0]
	for _, t := range bb.Threats {
		// Immediately drop threats whose source is now dead.
		if t.Source != nil && t.Source.state == SoldierStateDead {
			continue
		}
		if !t.IsVisible {
			// Decay by tick delta: 0.008/tick ≈ full decay in ~125 ticks (~2s at 60TPS).
			age := float64(currentTick - t.LastTick)
			t.Confidence = math.Max(0, t.Confidence-age*0.008)
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

// combatMemoryDecayPerTick is 1/3600 so memory lasts ~60s at 60TPS.
const combatMemoryDecayPerTick = 1.0 / 3600.0

// RecordGunfire stamps the persistent combat memory with fresh strength=1.
func (bb *Blackboard) RecordGunfire(x, y float64) {
	bb.RecordGunfireWithStrength(x, y, 1.0)
}

// RecordGunfireWithStrength stamps persistent combat memory with a bounded
// strength in [0,1]. We keep the strongest recent memory and only move the
// remembered position when the new signal is at least as strong.
func (bb *Blackboard) RecordGunfireWithStrength(x, y, strength float64) {
	strength = clamp01(strength)
	if strength <= 0 {
		return
	}
	if strength >= bb.CombatMemoryStrength {
		bb.CombatMemoryX = x
		bb.CombatMemoryY = y
	}
	if strength > bb.CombatMemoryStrength {
		bb.CombatMemoryStrength = strength
	}
}

// DecayCombatMemory should be called once per tick per soldier.
// It reduces combat memory toward zero and clears the flag when negligible.
func (bb *Blackboard) DecayCombatMemory() {
	if bb.CombatMemoryStrength <= 0 {
		return
	}
	bb.CombatMemoryStrength -= combatMemoryDecayPerTick
	if bb.CombatMemoryStrength < 0 {
		bb.CombatMemoryStrength = 0
	}
}

// IsActivated returns true if this soldier has recent combat memory strong
// enough to drive activity even without current visual contact.
func (bb *Blackboard) IsActivated() bool {
	return bb.CombatMemoryStrength > 0.01
}

// --- Suppression ---

const (
	// SuppressThreshold: SuppressLevel above this means the soldier is
	// meaningfully pinned — goal selection will react.
	SuppressThreshold = 0.30
	// suppressClearThreshold is lower than SuppressThreshold to avoid
	// suppression chatter at the boundary.
	suppressClearThreshold = 0.22

	// suppressDecayPerTick decays suppression toward zero.
	// ~4s to fully clear from 1.0 at 60TPS (1/240 ≈ 0.00417).
	suppressDecayPerTick = 1.0 / 240.0

	// suppressHitDelta: suppression added per bullet that hits.
	suppressHitDelta = 0.28

	// suppressNearMissDelta: suppression added per bullet that narrowly misses.
	suppressNearMissDelta = 0.14
)

// AccumulateSuppression adds suppression from an incoming round and records
// the direction of the threat so cover-seeking is oriented correctly.
// fromX, fromY is the shooter's position.
func (bb *Blackboard) AccumulateSuppression(hit bool, fromX, fromY, toX, toY float64) {
	prev := bb.SuppressLevel
	if hit {
		bb.SuppressLevel = math.Min(1.0, bb.SuppressLevel+suppressHitDelta)
	} else {
		bb.SuppressLevel = math.Min(1.0, bb.SuppressLevel+suppressNearMissDelta)
	}
	bb.updateSuppressedState()
	// Update the suppression direction (bearing FROM the target TOWARD the shooter —
	// the direction cover must be taken from).
	dx := fromX - toX
	dy := fromY - toY
	if dx*dx+dy*dy > 1e-6 {
		bb.SuppressDir = math.Atan2(dy, dx)
	}
	// Flag the spike so the Update loop can issue a ShatterEvent once.
	if prev < SuppressThreshold && bb.SuppressLevel >= SuppressThreshold {
		bb.suppressSpiked = true
	}
}

// DecaySuppression should be called once per tick. Decays suppression toward
// zero. Returns true if a spike was consumed this tick (for shatter events).
func (bb *Blackboard) DecaySuppression() bool {
	bb.SuppressLevel = math.Max(0, bb.SuppressLevel-suppressDecayPerTick)
	bb.updateSuppressedState()
	spiked := bb.suppressSpiked
	bb.suppressSpiked = false
	return spiked
}

func (bb *Blackboard) updateSuppressedState() {
	if bb.suppressed {
		if bb.SuppressLevel < suppressClearThreshold {
			bb.suppressed = false
		}
		return
	}
	if bb.SuppressLevel >= SuppressThreshold {
		bb.suppressed = true
	}
}

// IsSuppressed returns true when the soldier's suppression is high enough
// to meaningfully change their behaviour.
func (bb *Blackboard) IsSuppressed() bool {
	bb.updateSuppressedState()
	return bb.suppressed
}

// --- Goal Utility Scoring ---

// socialMovePressure computes additional movement pressure from social context:
// rising ally stress and isolation from both allies and enemies.
func socialMovePressure(bb *Blackboard, ef float64) (isolationPush, supportPush float64) {
	if bb.VisibleThreatCount() == 0 && bb.VisibleAllyCount == 0 && bb.IsolatedTicks > 0 {
		isolationPush = 0.15 + clamp01(float64(bb.IsolatedTicks)/180.0)*0.45
	}
	if bb.VisibleThreatCount() == 0 && bb.SquadFearDelta > 0.01 && ef < 0.55 {
		calmFactor := clamp01((0.55 - ef) / 0.55)
		supportPush = clamp01(bb.SquadFearDelta*8.0) * (0.25 + calmFactor*0.35)
	}
	return isolationPush, supportPush
}

// officerOrderBias computes an additive utility bias for a specific goal when a
// squad leader order is active. It nudges behaviour strongly but does not force
// outcomes, preserving self-preservation and local context effects.
func officerOrderBias(goal GoalKind, bb *Blackboard, profile *SoldierProfile) float64 {
	if !bb.OfficerOrderActive || bb.OfficerOrderKind == CmdNone {
		return 0
	}

	ef := profile.Psych.EffectiveFear()
	compliance := (0.45 + profile.Skills.Discipline*0.55) * (1.0 - ef*0.45)
	if compliance < 0.1 {
		compliance = 0.1
	}
	if compliance > 1.0 {
		compliance = 1.0
	}

	base := (0.15 + bb.OfficerOrderPriority*0.20 + bb.OfficerOrderStrength*0.35) * compliance

	switch bb.OfficerOrderKind {
	case CmdMoveTo:
		switch goal {
		case GoalMaintainFormation:
			return base * 1.15
		case GoalAdvance:
			return base * 1.00
		case GoalMoveToContact:
			return base * 0.50
		}
	case CmdHold:
		switch goal {
		case GoalHoldPosition:
			return base * 1.20
		case GoalOverwatch:
			return base * 0.75
		}
	case CmdRegroup:
		switch goal {
		case GoalRegroup:
			return base * 1.25
		case GoalMaintainFormation:
			return base * 0.60
		}
	case CmdBoundForward:
		switch goal {
		case GoalMoveToContact:
			return base * 1.00
		case GoalOverwatch:
			return base * 0.90
		}
	case CmdForm:
		if goal == GoalMaintainFormation {
			return base * 1.20
		}
	case CmdFanOut:
		switch goal {
		case GoalFlank:
			return base * 1.10
		case GoalMoveToContact:
			return base * 0.85
		case GoalOverwatch:
			return base * 0.35
		}
	case CmdAssault:
		switch goal {
		case GoalMoveToContact:
			return base * 1.20
		case GoalEngage:
			return base * 0.95
		case GoalFlank:
			return base * 0.55
		}
	}

	return 0
}

// SelectGoal evaluates competing goals and returns the highest-utility one.
func SelectGoal(bb *Blackboard, profile *SoldierProfile, isLeader bool, hasPath bool) GoalKind {
	bb.ensureInternalDefaults()
	internal := bb.Internal

	visibleThreats := bb.VisibleThreatCount()
	// underFire is true when rounds are arriving THIS tick OR the soldier is
	// still suppressed from recent fire (persistent, not ephemeral).
	underFire := bb.IncomingFireCount > 0 || bb.IsSuppressed()
	suppress := bb.SuppressLevel // 0-1 persistent suppression level
	ef := profile.Psych.EffectiveFear()

	// activated is true whenever combat memory is strong enough to drive behaviour.
	// This persists for up to ~60s after the last heard shot.
	activated := bb.IsActivated()
	// mem scales 0→1 indicating how fresh the combat memory is.
	mem := bb.CombatMemoryStrength

	// anyContact covers all sources: visual, squad intel, fresh audio, or persistent memory.
	hasAudioContact := bb.HeardGunfire && visibleThreats == 0
	anyContact := bb.SquadHasContact || hasAudioContact || activated
	combatCohesionPush := 0.0
	if anyContact && bb.VisibleAllyCount == 0 {
		combatCohesionPush = 0.35 + clamp01(float64(bb.IsolatedTicks)/120.0)*0.35
	}
	clumpPush := bb.CloseAllyPressure

	// Posture influence: positive = offensive push, negative = defensive hold.
	// When outnumbered the squad pushes aggressively; when outnumbering it holds.
	posture := bb.SquadPosture // -1..+1

	// --- Engage: visible enemy, controlled fire. ---
	engageUtil := 0.0
	if visibleThreats > 0 {
		engageUtil = 0.45 + profile.Skills.Discipline*0.15 + internal.ShootDesire*0.45
		if internal.LastEstimatedHitChance >= internal.Thresholds.EngageShotQuality {
			engageUtil += 0.20
		}
		if internal.ShotMomentum >= internal.Thresholds.HoldOnHitMomentum {
			engageUtil += 0.15
		}
		if internal.LastRange > accurateFireRange && internal.LastEstimatedHitChance < internal.Thresholds.LongRangeShotQuality {
			engageUtil -= 0.18
		}
		engageUtil -= ef * 0.30 // less fear-suppression of engagement
		if underFire {
			engageUtil += 0.03
		}
		// Suppression pins the soldier: heavy fire halves the urge to stand and fight.
		// Disciplined soldiers resist this more — they return fire from cover.
		engageUtil -= suppress * (0.40 - profile.Skills.Discipline*0.25)
		// Outnumbered posture: push engage harder when aggressive.
		if posture > 0 {
			engageUtil += posture * 0.15
		}
	}

	// --- Fallback: retreat away from contact. ---
	// Active under fire as before. Also available to activated HIGH-FEAR soldiers
	// even when the shooting has stopped — they want distance from the last known
	// contact area.
	fallbackUtil := 0.0
	{
		if underFire && bb.SquadHasContact {
			if ef > 0.25 {
				fallbackUtil = (ef-0.25)*1.4 + 0.1*float64(bb.IncomingFireCount)
				fallbackUtil -= profile.Skills.Discipline * 0.5
				fallbackUtil += internal.CoverDesire * 0.15
			}
			// Sustained suppression at high levels pushes even disciplined soldiers back.
			// This only kicks in when truly pinned (suppress > 0.6) — not on every hit.
			if suppress > 0.60 {
				fallbackUtil += (suppress-0.60)*0.8 - profile.Skills.Discipline*0.3
			}
		} else if activated && visibleThreats == 0 && ef > 0.35 {
			// Post-combat anxiety: high-fear soldiers keep retreating after a fight.
			fallbackUtil = (ef-0.35)*1.0*mem - profile.Skills.Discipline*0.55
		}
		// Outnumbered posture: suppress fallback when we need to push.
		if posture > 0 {
			fallbackUtil -= posture * 0.20
		}
		// Outnumbering posture: fallback is more acceptable when defensive.
		if posture < 0 {
			fallbackUtil += (-posture) * 0.10
		}
	}

	// --- Survive: panic / freeze / seek cover — last resort. ---
	surviveUtil := 0.0
	if ef > internal.Thresholds.CoverFear {
		surviveUtil = (ef - internal.Thresholds.CoverFear) * 2.0
		if underFire {
			surviveUtil += 0.05 * float64(bb.IncomingFireCount)
		}
		// Activated soldiers with very high fear stay in survive longer.
		if activated {
			surviveUtil += mem * 0.15
		}
	}
	// Suppression alone (even without peak fear) can force a soldier into cover.
	// This is the key: a pinned soldier seeks cover regardless of fear threshold.
	// Discipline and composure resist — experienced soldiers return fire from cover
	// rather than freezing completely.
	if bb.IsSuppressed() {
		suppressedCoverDrive := suppress*0.55 - profile.Skills.Discipline*0.20 - profile.Psych.Composure*0.10
		if suppressedCoverDrive > surviveUtil {
			surviveUtil = suppressedCoverDrive
		}
	}

	// --- MoveToContact: close distance to last known enemy area. ---
	// Triggered by contact of any kind. Also fires when visible but at long range
	// with poor shot quality — the soldier needs to push forward to effective range.
	moveToContactUtil := 0.0
	lowQualityLongRange := visibleThreats > 0 && internal.LastRange > accurateFireRange && internal.LastEstimatedHitChance < internal.Thresholds.LongRangeShotQuality
	if anyContact && (visibleThreats == 0 || lowQualityLongRange) {
		moveToContactUtil = 0.50 + profile.Skills.Discipline*0.1 + internal.MoveDesire*0.30
		if lowQualityLongRange {
			// Stronger urgency: the further out of range, the more they need to close.
			rangePressure := clamp01((internal.LastRange - accurateFireRange) / accurateFireRange)
			moveToContactUtil += 0.20 + rangePressure*0.25
		}
		if hasAudioContact && !bb.SquadHasContact {
			moveToContactUtil += 0.10
		}
		// Memory-only contact: scale down proportional to how stale the memory is.
		if activated && !bb.SquadHasContact && !hasAudioContact {
			moveToContactUtil *= mem * 0.8
		}
		if internal.ShotMomentum <= -internal.Thresholds.PushOnMissMomentum {
			moveToContactUtil += 0.20
		}
		moveToContactUtil -= ef * 0.15 // reduced fear penalty — even scared soldiers push
		if !hasPath {
			moveToContactUtil -= 0.10 // reduced penalty — don't freeze just because path is stale
		}
		// Suppression stops advances: a pinned soldier won't charge forward.
		// Tapers off as discipline rises (veteran pushes through suppression).
		moveToContactUtil -= suppress * (0.45 - profile.Skills.Discipline*0.20)
		// Outnumbered posture: push forward more aggressively.
		if posture > 0 {
			moveToContactUtil += posture * 0.12
		}
		// Squad is actively fighting: members without LOS should bias toward
		// closing distance to support, rather than staying static in overwatch.
		if bb.SquadIntent == IntentEngage && visibleThreats == 0 {
			moveToContactUtil += 0.35
		}
	}

	// --- Flank: move perpendicular to enemy direction. ---
	// Available even with visible threats at poor range — flanking is a valid
	// response to being pinned at long range with no effective shot.
	flankUtil := 0.0
	if anyContact && !bb.FlankComplete {
		if visibleThreats == 0 {
			flankUtil = 0.35 + profile.Skills.Fieldcraft*0.35 + internal.MoveDesire*0.20
			// Memory-only: scale by memory freshness and fieldcraft.
			if activated && !bb.SquadHasContact && !hasAudioContact {
				flankUtil *= mem * 0.9
			}
		} else if lowQualityLongRange {
			// Visible enemy at poor range: aggressive soldiers choose to flank.
			flankUtil = 0.20 + profile.Skills.Fieldcraft*0.40 + internal.MoveDesire*0.15
		}
		flankUtil -= ef * 0.30
		// A suppressed soldier can't safely manoeuvre — flanking requires movement.
		flankUtil -= suppress * 0.35
		if isLeader {
			flankUtil -= 0.25
		}
		// Outnumbered posture: flanking is more valuable under pressure.
		if posture > 0 {
			flankUtil += posture * 0.10
		}
	}

	// --- Overwatch: hold a good sightline position, wait for contact. ---
	// Post-flank has strong pull. Also attractive for calm disciplined soldiers
	// in good terrain who remember there's a fight nearby.
	overwatchUtil := 0.0
	if bb.FlankComplete {
		overwatchUtil = 0.70 + profile.Skills.Fieldcraft*0.15
	} else if bb.LocalSightlineScore > 0.5 && visibleThreats == 0 && anyContact {
		overwatchUtil = 0.40 + bb.LocalSightlineScore*0.25 + profile.Skills.Discipline*0.15
		// Memory-only: scale with freshness and calm disposition.
		if activated && !bb.SquadHasContact && !hasAudioContact {
			overwatchUtil *= mem*0.7 + (1.0-ef)*0.3
		}
		overwatchUtil -= ef * 0.20
	}
	// Tactical position bonus: corners and wall-adjacent cells are great overwatch spots.
	if bb.AtCorner {
		overwatchUtil += 0.15
	}
	if bb.AtWall && !bb.AtDoorway {
		overwatchUtil += 0.05
	}
	// Window-adjacent interior: prime overwatch position.
	if bb.AtWindowAdj {
		overwatchUtil += 0.30
	}
	// Interior of building is generally safer for overwatching.
	if bb.AtInterior && !bb.AtDoorway {
		overwatchUtil += 0.10
	}
	// Doorways are terrible overwatch positions — soldiers should move through them.
	if bb.AtDoorway {
		overwatchUtil -= 0.20
	}
	if anyContact && visibleThreats == 0 {
		overwatchUtil *= overwatchDistanceFactor(internal.LastContactRange)
	}
	if bb.SquadIntent == IntentEngage && visibleThreats == 0 {
		overwatchUtil -= 0.25
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
		// Good tactical positions reinforce holding.
		if bb.PositionDesirability > 0 {
			holdUtil += bb.PositionDesirability * 0.15
		}
		// Window-adjacent: excellent hold position.
		if bb.AtWindowAdj {
			holdUtil += 0.25
		}
		if bb.AtInterior && !bb.AtDoorway {
			holdUtil += 0.10
		}
		// Doorways are bad to hold in.
		if bb.AtDoorway {
			holdUtil -= 0.15
		}
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
			// Activated soldiers break formation to deal with the threat.
			if activated {
				formationUtil *= (1.0 - mem*0.7)
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
	// Activated soldiers suppress pure advance — they're dealing with a known threat.
	if activated && visibleThreats == 0 {
		advanceUtil *= (1.0 - mem*0.65)
	}
	// Suppressed soldiers don't advance in the open.
	if bb.IsSuppressed() {
		advanceUtil *= clamp01(1.0 - suppress*1.5)
	}

	// --- Peek: cautious look around a corner or through a window. ---
	// Very attractive when near a tactically interesting cell and not under fire.
	// Each empty peek decays the utility so soldiers eventually move on.
	peekUtil := 0.0
	if bb.PeekCooldown <= 0 && !bb.IsSuppressed() && ef < 0.70 {
		if bb.AtCorner || bb.AtWindowAdj {
			peekUtil = 0.60 + profile.Skills.Fieldcraft*0.25
			// Reduce for each successive empty peek at this spot.
			noContactPenalty := math.Min(0.45, float64(bb.PeekNoContactCount)*0.15)
			peekUtil -= noContactPenalty
			// When visibly engaged, peek is pointless — just fire.
			if visibleThreats > 0 {
				peekUtil *= 0.10
			}
			// Activated soldiers peek more deliberately.
			if anyContact && visibleThreats == 0 {
				peekUtil += 0.10
			}
			// During active squad engagement, no-LOS members should avoid prolonged
			// local peeking and instead rejoin the fight.
			if bb.SquadIntent == IntentEngage && visibleThreats == 0 {
				peekUtil *= 0.25
			}
		}
	}

	// --- Social movement pressure adjustments ---
	isolationPush, supportPush := socialMovePressure(bb, ef)
	if isolationPush > 0 {
		advanceUtil += isolationPush
		formationUtil += isolationPush * 0.85
		holdUtil -= isolationPush * 0.70
		overwatchUtil -= isolationPush * 0.55
		if regroupUtil > 0 {
			regroupUtil += isolationPush * 0.25
		}
	}
	if supportPush > 0 {
		advanceUtil += supportPush * 0.25
		holdUtil -= supportPush * 0.30
		overwatchUtil -= supportPush * 0.25
		if anyContact {
			moveToContactUtil += supportPush
			flankUtil += supportPush * 0.35
		}
	}
	if combatCohesionPush > 0 {
		formationUtil += combatCohesionPush * 1.10
		if !isLeader {
			regroupUtil += combatCohesionPush * 0.90
		}
		moveToContactUtil += combatCohesionPush * 0.35
		holdUtil -= combatCohesionPush * 0.50
		overwatchUtil -= combatCohesionPush * 0.40
	}
	if clumpPush > 0 {
		advanceUtil += clumpPush * 0.30
		moveToContactUtil += clumpPush * 0.25
		flankUtil += clumpPush * 0.20
		formationUtil -= clumpPush * 0.25
		holdUtil -= clumpPush * 0.55
		overwatchUtil -= clumpPush * 0.45
	}

	// --- Officer order signal (strong positive bias, not hard override) ---
	advanceUtil += officerOrderBias(GoalAdvance, bb, profile)
	formationUtil += officerOrderBias(GoalMaintainFormation, bb, profile)
	regroupUtil += officerOrderBias(GoalRegroup, bb, profile)
	holdUtil += officerOrderBias(GoalHoldPosition, bb, profile)
	engageUtil += officerOrderBias(GoalEngage, bb, profile)
	moveToContactUtil += officerOrderBias(GoalMoveToContact, bb, profile)
	flankUtil += officerOrderBias(GoalFlank, bb, profile)
	overwatchUtil += officerOrderBias(GoalOverwatch, bb, profile)

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
	check(GoalFlank, flankUtil)
	check(GoalOverwatch, overwatchUtil)
	check(GoalFallback, fallbackUtil)
	check(GoalSurvive, surviveUtil)
	check(GoalPeek, peekUtil)

	return best
}

// SelectGoalWithHysteresis runs the full utility scoring but applies the
// blackboard's HysteresisMargin as a loyalty bonus to the current goal.
// A competing goal must exceed the current goal's utility by at least
// HysteresisMargin to cause a switch.
func SelectGoalWithHysteresis(bb *Blackboard, profile *SoldierProfile, isLeader bool, hasPath bool) GoalKind {
	candidate := SelectGoal(bb, profile, isLeader, hasPath)
	if candidate == bb.CurrentGoal {
		return candidate
	}

	margin := bb.HysteresisMargin
	if margin < hysteresisBase {
		margin = hysteresisBase
	}

	// Compare utilities: candidate must beat current by margin to switch.
	currentUtil := goalUtilSingle(bb, profile, isLeader, hasPath, bb.CurrentGoal)
	candidateUtil := goalUtilSingle(bb, profile, isLeader, hasPath, candidate)

	if candidateUtil > currentUtil+margin {
		return candidate
	}
	return bb.CurrentGoal
}

// goalUtilSingle computes the utility for a single goal by running SelectGoal's
// full scoring and extracting the value for the requested goal.
// Implementation: we score all goals (cheap) and return the one we want.
func goalUtilSingle(bb *Blackboard, profile *SoldierProfile, isLeader bool, hasPath bool, goal GoalKind) float64 {
	bb.ensureInternalDefaults()
	internal := bb.Internal

	visibleThreats := bb.VisibleThreatCount()
	underFire := bb.IncomingFireCount > 0 || bb.IsSuppressed()
	suppress := bb.SuppressLevel
	ef := profile.Psych.EffectiveFear()
	activated := bb.IsActivated()
	mem := bb.CombatMemoryStrength
	hasAudioContact := bb.HeardGunfire && visibleThreats == 0
	anyContact := bb.SquadHasContact || hasAudioContact || activated
	posture := bb.SquadPosture
	isolationPush, supportPush := socialMovePressure(bb, ef)
	combatCohesionPush := 0.0
	if anyContact && bb.VisibleAllyCount == 0 {
		combatCohesionPush = 0.35 + clamp01(float64(bb.IsolatedTicks)/120.0)*0.35
	}
	clumpPush := bb.CloseAllyPressure
	orderBias := officerOrderBias(goal, bb, profile)

	switch goal {
	case GoalEngage:
		u := 0.0
		if visibleThreats > 0 {
			u = 0.45 + profile.Skills.Discipline*0.15 + internal.ShootDesire*0.45
			if internal.LastEstimatedHitChance >= internal.Thresholds.EngageShotQuality {
				u += 0.20
			}
			if internal.ShotMomentum >= internal.Thresholds.HoldOnHitMomentum {
				u += 0.15
			}
			if internal.LastRange > accurateFireRange && internal.LastEstimatedHitChance < internal.Thresholds.LongRangeShotQuality {
				u -= 0.18
			}
			u -= ef * 0.30
			if underFire {
				u += 0.03
			}
			u -= suppress * (0.40 - profile.Skills.Discipline*0.25)
			if posture > 0 {
				u += posture * 0.15
			}
		}
		return u + orderBias

	case GoalFallback:
		u := 0.0
		if underFire && bb.SquadHasContact {
			if ef > 0.25 {
				u = (ef-0.25)*1.4 + 0.1*float64(bb.IncomingFireCount)
				u -= profile.Skills.Discipline * 0.5
				u += internal.CoverDesire * 0.15
			}
			if suppress > 0.60 {
				u += (suppress-0.60)*0.8 - profile.Skills.Discipline*0.3
			}
		} else if activated && visibleThreats == 0 && ef > 0.35 {
			u = (ef-0.35)*1.0*mem - profile.Skills.Discipline*0.55
		}
		if posture > 0 {
			u -= posture * 0.20
		}
		if posture < 0 {
			u += (-posture) * 0.10
		}
		return u + orderBias

	case GoalSurvive:
		u := 0.0
		if ef > internal.Thresholds.CoverFear {
			u = (ef - internal.Thresholds.CoverFear) * 2.0
			if underFire {
				u += 0.05 * float64(bb.IncomingFireCount)
			}
			if activated {
				u += mem * 0.15
			}
		}
		if bb.IsSuppressed() {
			sc := suppress*0.55 - profile.Skills.Discipline*0.20 - profile.Psych.Composure*0.10
			if sc > u {
				u = sc
			}
		}
		return u

	case GoalMoveToContact:
		u := 0.0
		lowQ := visibleThreats > 0 && internal.LastRange > accurateFireRange && internal.LastEstimatedHitChance < internal.Thresholds.LongRangeShotQuality
		if anyContact && (visibleThreats == 0 || lowQ) {
			u = 0.50 + profile.Skills.Discipline*0.1 + internal.MoveDesire*0.30
			if lowQ {
				rp := clamp01((internal.LastRange - accurateFireRange) / accurateFireRange)
				u += 0.20 + rp*0.25
			}
			if hasAudioContact && !bb.SquadHasContact {
				u += 0.10
			}
			if activated && !bb.SquadHasContact && !hasAudioContact {
				u *= mem * 0.8
			}
			if internal.ShotMomentum <= -internal.Thresholds.PushOnMissMomentum {
				u += 0.20
			}
			u -= ef * 0.15
			if !hasPath {
				u -= 0.10
			}
			u -= suppress * (0.45 - profile.Skills.Discipline*0.20)
			if posture > 0 {
				u += posture * 0.12
			}
			if bb.SquadIntent == IntentEngage && visibleThreats == 0 {
				u += 0.35
			}
		}
		if anyContact {
			u += supportPush
			u += combatCohesionPush * 0.35
		}
		u += clumpPush * 0.25
		return u

	case GoalFlank:
		u := 0.0
		lowQ := visibleThreats > 0 && internal.LastRange > accurateFireRange && internal.LastEstimatedHitChance < internal.Thresholds.LongRangeShotQuality
		if anyContact && !bb.FlankComplete {
			if visibleThreats == 0 {
				u = 0.35 + profile.Skills.Fieldcraft*0.35 + internal.MoveDesire*0.20
				if activated && !bb.SquadHasContact && !hasAudioContact {
					u *= mem * 0.9
				}
			} else if lowQ {
				u = 0.20 + profile.Skills.Fieldcraft*0.40 + internal.MoveDesire*0.15
			}
			u -= ef * 0.30
			u -= suppress * 0.35
			if isLeader {
				u -= 0.25
			}
			if posture > 0 {
				u += posture * 0.10
			}
		}
		if anyContact {
			u += supportPush * 0.35
		}
		u += clumpPush * 0.20
		return u

	case GoalOverwatch:
		u := 0.0
		if bb.FlankComplete {
			u = 0.70 + profile.Skills.Fieldcraft*0.15
		} else if bb.LocalSightlineScore > 0.5 && visibleThreats == 0 && anyContact {
			u = 0.40 + bb.LocalSightlineScore*0.25 + profile.Skills.Discipline*0.15
			if activated && !bb.SquadHasContact && !hasAudioContact {
				u *= mem*0.7 + (1.0-ef)*0.3
			}
			u -= ef * 0.20
		}
		if bb.AtCorner {
			u += 0.15
		}
		if bb.AtWall && !bb.AtDoorway {
			u += 0.05
		}
		if bb.AtWindowAdj {
			u += 0.30
		}
		if bb.AtInterior && !bb.AtDoorway {
			u += 0.10
		}
		if bb.AtDoorway {
			u -= 0.20
		}
		if anyContact && visibleThreats == 0 {
			u *= overwatchDistanceFactor(internal.LastContactRange)
		}
		if bb.SquadIntent == IntentEngage && visibleThreats == 0 {
			u -= 0.25
		}
		u -= isolationPush * 0.55
		u -= supportPush * 0.25
		u -= combatCohesionPush * 0.40
		u -= clumpPush * 0.45
		return u

	case GoalRegroup:
		u := 0.0
		if bb.SquadIntent == IntentRegroup && !isLeader {
			u = 0.75 + profile.Skills.Discipline*0.2
			if visibleThreats > 0 {
				u *= 0.6
			}
		}
		if u > 0 {
			u += isolationPush * 0.25
			u += combatCohesionPush * 0.90
		}
		return u

	case GoalHoldPosition:
		u := 0.0
		if bb.SquadIntent == IntentHold {
			u = 0.5 + profile.Skills.Discipline*0.25
			if bb.PositionDesirability > 0 {
				u += bb.PositionDesirability * 0.15
			}
			if bb.AtWindowAdj {
				u += 0.25
			}
			if bb.AtInterior && !bb.AtDoorway {
				u += 0.10
			}
			if bb.AtDoorway {
				u -= 0.15
			}
		}
		u -= isolationPush * 0.70
		u -= supportPush * 0.30
		u -= combatCohesionPush * 0.50
		u -= clumpPush * 0.55
		return u

	case GoalMaintainFormation:
		u := 0.0
		if !isLeader {
			switch bb.SquadIntent {
			case IntentAdvance:
				u = 0.45 + profile.Skills.Discipline*0.2
				if visibleThreats > 0 {
					u *= 0.2
				}
				if activated {
					u *= (1.0 - mem*0.7)
				}
			case IntentHold:
				u = 0.3 + profile.Skills.Discipline*0.1
				if visibleThreats > 0 {
					u *= 0.4
				}
			case IntentRegroup:
				u = 0.55 + profile.Skills.Discipline*0.15
				if visibleThreats > 0 {
					u *= 0.7
				}
			case IntentEngage:
				u = 0.1
			}
		}
		u += isolationPush * 0.85
		u += combatCohesionPush * 1.10
		u -= clumpPush * 0.25
		return u

	case GoalAdvance:
		u := 0.3
		if bb.SquadIntent == IntentAdvance {
			u = 0.5
		}
		if visibleThreats > 0 {
			u *= 0.05
		}
		if isLeader {
			u += 0.1
		}
		if activated && visibleThreats == 0 {
			u *= (1.0 - mem*0.65)
		}
		if bb.IsSuppressed() {
			u *= clamp01(1.0 - suppress*1.5)
		}
		u += isolationPush
		u += supportPush * 0.25
		u += clumpPush * 0.30
		return u

	case GoalPeek:
		u := 0.0
		if bb.PeekCooldown <= 0 && !bb.IsSuppressed() && ef < 0.70 {
			if bb.AtCorner || bb.AtWindowAdj {
				u = 0.60 + profile.Skills.Fieldcraft*0.25
				noContactPenalty := math.Min(0.45, float64(bb.PeekNoContactCount)*0.15)
				u -= noContactPenalty
				if visibleThreats > 0 {
					u *= 0.10
				}
				if anyContact && visibleThreats == 0 {
					u += 0.10
				}
				if bb.SquadIntent == IntentEngage && visibleThreats == 0 {
					u *= 0.25
				}
			}
		}
		return u
	}
	return 0
}
