package game

import (
	"fmt"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

const (
	soldierRadius   = 6
	soldierSpeed    = 1.5  // base pixels per tick
	dashSpeedMul    = 2.2  // sprint multiplier during combat dashes
	turnRate        = 0.12 // radians per tick
	coverSearchDist = 80.0 // pixels to search for cover

	// Aim spread constants (radians).
	aimSpreadBase      = 0.05  // resting spread for average soldier (~3°)
	aimSpreadMax       = 0.38  // maximum spread while running/sprinting (~22°)
	aimSpreadGrowRate  = 0.010 // spread increase per tick while moving
	aimSpreadDecayRate = 0.014 // spread decrease per tick while still

	// Dash-overwatch constants.
	dashOverwatchBase = 120 // base ticks soldier pauses after a dash (~2s at 60TPS)

	// Post-arrival pause: ticks a soldier waits after reaching a destination
	// before re-evaluating their goal (assess → perceive → decide cadence).
	postArrivalBase = 50 // ~0.8s base pause

	// Peek constants.
	peekDuration      = 90  // ticks spent peeking (~1.5s)
	peekCooldownEmpty = 480 // cooldown after empty peek (~8s)
	peekCooldownHit   = 180 // cooldown after contact peek (~3s)

	// Decision pacing.
	// baseDecisionInterval is the MINIMUM re-evaluation window at zero stress.
	// Under stress, the window GROWS (soldiers commit longer when scared).
	baseDecisionInterval   = 60    // ticks (~1s at 60 TPS) — generous baseline
	minDecisionInterval    = 30    // floor: never re-evaluate faster than 0.5s
	panicFearThreshold     = 0.8   // EffectiveFear above this = panic-locked
	panicRecoveryThreshold = 0.5   // EffectiveFear below this = panic unlocks (hysteresis)
	flankDistance          = 200.0 // px perpendicular travel during flank
	sightlineUpdateRate    = 120   // ticks between sightline score recalcs
	goalPauseBase          = 10    // base ticks to pause briefly after a non-critical goal switch
	stressReevalPeriod     = 18    // ticks between stress-jitter re-evaluation probes
)

// FireMode represents the soldier's current weapon engagement mode.
type FireMode int

const (
	FireModeSingle FireMode = iota // deliberate single shots — long range
	FireModeBurst                  // 3-round bursts — mid range
	FireModeAuto                   // sustained automatic — CQB
)

func (fm FireMode) String() string {
	switch fm {
	case FireModeSingle:
		return "single"
	case FireModeBurst:
		return "burst"
	case FireModeAuto:
		return "auto"
	default:
		return "unknown"
	}
}

// Fire mode distance thresholds (in pixels).
const (
	// autoRange: 10 tiles — CQB range. Auto only triggers here AND in low-sightline terrain.
	autoRange = 10 * cellSize // 160px
	// burstRange: 20 tiles — mid-range, committed engagement.
	burstRange = 20 * cellSize // 320px
	// singleRange: up to maxFireRange — deliberate long-range fire.
	// Below this, single is always available.

	// Sightline threshold for auto mode — tight/enclosed terrain.
	autoSightlineThresh = 0.40

	// modeSwitchTicks: firing pause while changing mode (~0.4s at 60TPS).
	modeSwitchTicks = 25

	// proxEnemyStressRange: distance at which nearby enemies cause stress (pixels).
	proxEnemyStressRange = 6 * cellSize // 96px
	// proxFriendCrowdRange: distance causing crowding stress from friendlies.
	proxFriendCrowdRange = 3 * cellSize // 48px
)

// Team distinguishes friendly vs opposing force.
type Team int

const (
	TeamRed  Team = iota // friendly
	TeamBlue             // OpFor
)

// SoldierState represents the high-level behaviour state.
type SoldierState int

const (
	SoldierStateIdle   SoldierState = iota // holding, scanning
	SoldierStateMoving                     // advancing along path
	SoldierStateCover                      // in cover / suppressed
	SoldierStateDead                       // incapacitated
)

func (ss SoldierState) String() string {
	switch ss {
	case SoldierStateIdle:
		return "idle"
	case SoldierStateMoving:
		return "moving"
	case SoldierStateCover:
		return "cover"
	case SoldierStateDead:
		return "dead"
	default:
		return "unknown"
	}
}

// Soldier is an autonomous agent on the battlefield.
type Soldier struct {
	id    int
	label string // e.g. "R1", "B3"
	x, y  float64
	team  Team

	// Navigation
	path      [][2]float64
	pathIndex int
	// Objective: one-way advance from start toward objective.
	startTarget [2]float64
	endTarget   [2]float64
	navGrid     *NavGrid

	// Phase 1: agent model
	state    SoldierState
	profile  SoldierProfile
	vision   VisionState
	isLeader bool
	squad    *Squad

	// Combat
	health       float64 // hit points, 0 = incapacitated
	fireCooldown int     // ticks until next shot allowed

	// Multi-round trigger state (burst/auto pacing).
	burstShotsRemaining int // queued rounds left in current trigger pull
	burstShotIndex      int // next queued shot index (0-based)
	burstTargetID       int // target id locked for queued rounds
	burstAnyHit         bool
	burstHitChance      float64
	burstDist           float64
	burstBaseSpread     float64

	// Long-range aiming state.
	aimingTargetID int // target id currently being lined up (-1 means none)
	aimingTicks    int // consecutive ticks spent aiming at aimingTargetID

	// Fire mode
	currentFireMode FireMode // mode currently in use
	desiredFireMode FireMode // mode the soldier wants based on range/terrain
	modeSwitchTimer int      // >0 means switching; firing blocked until 0

	// Cognition
	blackboard  Blackboard
	prevGoal    GoalKind
	thoughtLog  *ThoughtLog
	currentTick *int // pointer to game tick counter

	// Formation
	formationMember bool    // true = follows squad slot, not fixed patrol
	slotIndex       int     // index into formation offsets
	slotTargetX     float64 // current world-space slot target
	slotTargetY     float64

	// Cover
	covers      []*CoverObject // shared reference to map cover objects
	coverTarget *CoverObject   // the cover object this soldier is moving toward

	// World reference for sightline queries.
	buildings          []rect
	buildingFootprints []rect
	tacticalMap        *TacticalMap

	// Sightline cache.
	lastSightlineTick int

	// Speech cooldown.
	lastSpeechTick int

	// --- Fuzzy aim ---
	// aimSpread grows when moving and decays when still.
	// Used in combat.go to compute physical bullet deflection.
	aimSpread float64

	// --- Stop/start dash movement ---
	// dashOverwatchTimer: >0 means soldier just dashed and is now overwatch-stopped.
	dashOverwatchTimer int
	// boundHoldTicks: consecutive ticks held as non-mover during buddy bounding.
	// Used to periodically allow micro-reposition so overwatchers don't freeze indefinitely.
	boundHoldTicks int

	// --- Cover-to-cover bounding ---
	// boundDestX/Y is the ultimate destination of a multi-bound advance.
	// Each dash targets an intermediate cover position along the bearing to boundDest.
	boundDestX, boundDestY float64
	boundDestSet           bool // true when a multi-bound advance is active
	// suppressionAbort: true when a dash was interrupted by incoming fire.
	// The soldier seeks cover immediately and waits for fire to lift before resuming.
	suppressionAbort bool

	// --- Post-arrival pause (assess → perceive → decide cadence) ---
	// postArrivalTimer: countdown after reaching a destination before re-evaluation.
	postArrivalTimer int

	// --- Peek system ---
	peekTarget [2]float64 // world position of the peek point
	peekTimer  int        // countdown while performing a peek

	// --- Action pacing ---
	// goalPauseTimer inserts a short pause after a non-critical goal switch.
	goalPauseTimer int

	// --- Fuzzy path-reacquisition memory ---
	// These track short-horizon movement confidence and support a human-like
	// "try another approach" response when direct repath repeatedly fails.
	recoveryNoPathStreak int
	recoveryRouteFailEMA float64
	recoveryStallEMA     float64
	recoveryCommitTicks  int
	recoveryAction       RecoveryAction
}

type RecoveryAction int

const (
	RecoveryActionDirect RecoveryAction = iota
	RecoveryActionLateral
	RecoveryActionAnchor
	RecoveryActionHold
)

const (
	personalSpaceRadius = float64(cellSize) * 1.1
	cellOverlapEpsilon  = 0.35
)

// NewSoldier creates a soldier at (x,y) that will advance toward end.
func NewSoldier(id int, x, y float64, team Team, start, end [2]float64, ng *NavGrid, covers []*CoverObject, buildings []rect, tl *ThoughtLog, tick *int, tm ...*TacticalMap) *Soldier {
	// Initial heading: face toward end target.
	initHeading := HeadingTo(x, y, end[0], end[1])

	prefix := "R"
	if team == TeamBlue {
		prefix = "B"
	}

	s := &Soldier{
		id:             id,
		label:          prefix + string(rune('0'+id%10)),
		x:              x,
		y:              y,
		team:           team,
		startTarget:    start,
		endTarget:      end,
		navGrid:        ng,
		covers:         covers,
		buildings:      buildings,
		state:          SoldierStateMoving,
		health:         soldierMaxHP,
		vision:         NewVisionState(initHeading),
		profile:        DefaultProfile(),
		thoughtLog:     tl,
		currentTick:    tick,
		prevGoal:       GoalAdvance,
		aimingTargetID: -1,
		burstTargetID:  -1,
	}
	if len(tm) > 0 && tm[0] != nil {
		s.tacticalMap = tm[0]
	}
	s.recomputePath()
	return s
}

// DefaultProfile returns a baseline average soldier.
func DefaultProfile() SoldierProfile {
	return SoldierProfile{
		Physical: PhysicalStats{
			FitnessBase: 0.6,
			Fatigue:     0.0,
			SprintPool:  10.0,
		},
		Skills: SkillStats{
			Marksmanship: 0.5,
			Fieldcraft:   0.4,
			Discipline:   0.6,
			FirstAid:     0.3,
		},
		Psych: PsychState{
			Experience: 0.2,
			Morale:     0.7,
			Fear:       0.0,
			Composure:  0.5,
		},
		Stance: StanceStanding,
	}
}

func (s *Soldier) recomputePath() {
	s.path = s.navGrid.FindPath(s.x, s.y, s.endTarget[0], s.endTarget[1])
	s.pathIndex = 0
}

// think logs a thought if the message represents a goal/state change.
func (s *Soldier) think(msg string) {
	if s.thoughtLog != nil && s.currentTick != nil {
		s.thoughtLog.Add(*s.currentTick, s.label, s.team, msg)
	}
}

// decisionInterval returns a base interval for backward compatibility.
// The commitment system (BeginCommitment) is the primary pacing mechanism now.
func (s *Soldier) decisionInterval() int {
	return commitPhaseTicks + sustainPhaseTicks
}

// reinforceCurrentGoal checks if positive feedback supports staying in the current goal.
// Returns true if the goal should be extended (decision deferred).
func (s *Soldier) reinforceCurrentGoal() bool {
	bb := &s.blackboard
	goal := bb.CurrentGoal

	switch goal {
	case GoalEngage:
		// Positive: landing hits (positive momentum) and in cover.
		if bb.Internal.ShotMomentum > 0.1 && s.isInCover() {
			return true
		}

	case GoalMoveToContact:
		// Positive: not under fire and still have path steps left.
		// Do NOT reinforce if we've arrived near the memory target and have no live contact —
		// that should trigger re-evaluation into overwatch/engage/flank.
		if bb.IncomingFireCount > 0 {
			return false // under fire — shatter and re-evaluate
		}
		// Check proximity to the destination we're heading toward.
		var destX, destY float64
		if bb.SquadHasContact {
			destX, destY = bb.SquadContactX, bb.SquadContactY
		} else if bb.HeardGunfire {
			destX, destY = bb.HeardGunfireX, bb.HeardGunfireY
		} else {
			destX, destY = bb.CombatMemoryX, bb.CombatMemoryY
		}
		dx := destX - s.x
		dy := destY - s.y
		if math.Sqrt(dx*dx+dy*dy) < 50 {
			return false // arrived at target — re-evaluate now
		}
		// Still en route with no fire: reinforce only if path is progressing.
		if s.path != nil && s.pathIndex < len(s.path) {
			return true
		}

	case GoalFlank:
		// Reinforce if not under fire and path is still active.
		if bb.IncomingFireCount == 0 && s.path != nil && s.pathIndex < len(s.path) {
			return true
		}

	case GoalFallback:
		// Positive: incoming fire has stopped but fear is still elevated.
		if bb.IncomingFireCount == 0 && s.profile.Psych.EffectiveFear() > 0.2 {
			return true
		}

	case GoalSurvive:
		// Always re-evaluate survive — it's panic, check if we can snap out.
		return false

	case GoalOverwatch:
		// Positive: good sightlines, no incoming fire, still activated/has contact.
		if bb.IncomingFireCount == 0 && bb.LocalSightlineScore > 0.4 && bb.IsActivated() {
			return true
		}

	case GoalPeek:
		// Reinforce if still actively peeking (timer running) and not under fire.
		if s.peekTimer > 0 && bb.IncomingFireCount == 0 {
			return true
		}
	}
	return false
}

// Update runs the soldier's per-tick cognition loop: believe → think → act.
func (s *Soldier) Update() {
	if s.state == SoldierStateDead {
		return
	}
	if false {
		_ = baseDecisionInterval
		_ = minDecisionInterval
		_ = s.decisionInterval()
	}

	dt := 1.0
	s.profile.Psych.RecoverFear(dt)
	s.profile.Psych.RecoverMorale(dt)

	bb := &s.blackboard

	// --- Fuzzy aim spread: grows when moving, decays when still ---
	baseSpread := aimSpreadBase * (1.0 + (1.0 - s.profile.Skills.Marksmanship))
	if s.state == SoldierStateMoving {
		s.aimSpread = math.Min(aimSpreadMax, s.aimSpread+aimSpreadGrowRate)
	} else {
		s.aimSpread = math.Max(baseSpread, s.aimSpread-aimSpreadDecayRate)
	}

	// --- Peek cooldown decay ---
	if bb.PeekCooldown > 0 {
		bb.PeekCooldown--
	}

	// --- Dash overwatch timer ---
	// Soldier is frozen post-dash; tick down and force re-evaluation on expiry.
	if s.dashOverwatchTimer > 0 {
		s.dashOverwatchTimer--
		if s.dashOverwatchTimer == 0 {
			bb.ShatterEvent = true // expired: re-assess
			s.think("overwatch expired — reassessing")
		}
	}

	// --- Post-arrival pause (assess→perceive→decide cadence) ---
	if s.postArrivalTimer > 0 {
		s.postArrivalTimer--
		if s.postArrivalTimer == 0 {
			bb.ShatterEvent = true // pause complete: time to decide
		}
	}

	if s.goalPauseTimer > 0 {
		s.goalPauseTimer--
	}

	// --- Per-tick decay of commitment-based decision state ---
	bb.DecayShatterPressure()
	bb.DecayDecisionDebt()

	// Decay persistent suppression every tick. If suppression just crossed
	// the threshold this tick, convert to shatter pressure (not instant shatter).
	if bb.DecaySuppression() {
		bb.AddShatterPressure(0.30, s.tickVal())
		s.think(fmt.Sprintf("suppressed (%.2f) — pressure building", bb.SuppressLevel))
	}

	// --- Step 2: BELIEVE — update blackboard from vision ---
	tick := s.tickVal()
	bb.UpdateThreats(s.vision.KnownContacts, tick)
	bb.RefreshInternalGoals(&s.profile, s.x, s.y)

	// Incoming fire adds shatter pressure proportional to volume.
	// Passive goals receive more pressure (easy to interrupt).
	// Active combat goals resist better (soldier is already reacting).
	if bb.IncomingFireCount > 0 {
		g := bb.CurrentGoal
		passiveGoal := g == GoalAdvance || g == GoalMaintainFormation || g == GoalHoldPosition || g == GoalOverwatch
		pressurePerRound := 0.15
		if passiveGoal {
			pressurePerRound = 0.25 // easier to rattle a soldier who isn't fighting
		}
		bb.AddShatterPressure(pressurePerRound*float64(bb.IncomingFireCount), tick)
	}

	// Check panic threshold.
	ef := s.profile.Psych.EffectiveFear()
	if ef >= panicFearThreshold {
		if !bb.PanicLocked {
			bb.PanicLocked = true
			s.think("PANIC — unable to decide")
		}
		if bb.CurrentGoal != GoalSurvive {
			s.think(fmt.Sprintf("goal: %s → %s (panic)", bb.CurrentGoal, GoalSurvive))
			bb.CurrentGoal = GoalSurvive
			s.prevGoal = GoalSurvive
		}
		s.executeGoal(dt)
		return
	}
	// Panic recovery: once fear drops well below threshold, unlock.
	if bb.PanicLocked && ef < panicRecoveryThreshold {
		bb.PanicLocked = false
		bb.ShatterPressure = bb.ShatterThreshold // force immediate re-evaluation
		s.think("panic subsiding — regaining composure")
	}

	// Periodically update sightline score (expensive, so not every tick).
	if tick-s.lastSightlineTick >= sightlineUpdateRate {
		s.lastSightlineTick = tick
		bb.LocalSightlineScore = ScoreSightline(s.x, s.y, s.navGrid, s.buildings)

		if bb.LocalSightlineScore < 0.25 {
			nervousness := (0.25 - bb.LocalSightlineScore) * 0.03
			s.profile.Psych.ApplyStress(nervousness)
		}

		if s.tacticalMap != nil {
			trait := s.tacticalMap.TraitAt(s.x, s.y)
			bb.AtCorner = trait&CellTraitCorner != 0
			bb.AtDoorway = trait&CellTraitDoorway != 0
			bb.AtWall = trait&CellTraitWallAdj != 0
			bb.AtWindowAdj = trait&CellTraitWindowAdj != 0
			bb.AtInterior = trait&CellTraitInterior != 0
			bb.PositionDesirability = s.tacticalMap.DesirabilityAt(s.x, s.y)

			if bb.AtDoorway {
				s.profile.Psych.ApplyStress(0.02)
			}
			if bb.AtCorner && bb.VisibleThreatCount() == 0 {
				s.profile.Psych.RecoverFear(0.5)
			}
			// Window overwatch: soldier at a window position feels secure.
			if bb.AtWindowAdj {
				s.profile.Psych.RecoverFear(0.3)
			}

			// Position scan: find best nearby tile considering enemy direction and claimed building.
			hasEnemy := bb.SquadHasContact
			bearing := bb.SquadEnemyBearing
			claimedIdx := bb.ClaimedBuildingIdx
			var footprints []rect
			// footprints are passed via the game — store reference on soldier.
			if s.buildingFootprints != nil {
				footprints = s.buildingFootprints
			}
			bx, by, bscore := s.tacticalMap.ScanBestNearby(s.x, s.y, 10, bearing, hasEnemy, claimedIdx, footprints)
			if bscore > bb.PositionDesirability+0.15 {
				bb.BestNearbyX = bx
				bb.BestNearbyY = by
				bb.BestNearbyScore = bscore
				bb.HasBestNearby = true
			} else {
				bb.HasBestNearby = false
			}
		}

		s.applyProximityStress()

		momentum := bb.Internal.ShotMomentum
		if momentum < -0.3 {
			movePush := clamp01((-momentum - 0.3) * 0.7)
			bb.Internal.MoveDesire = math.Min(1.0,
				bb.Internal.MoveDesire+movePush*0.4)
			if movePush > 0.25 {
				s.think(fmt.Sprintf("missing — moving closer (momentum %.2f)", momentum))
			}
		}
	}

	// --- Decision evaluation ---
	// Three paths to re-evaluation:
	//   1. Shatter pressure exceeded threshold (disruptive event accumulated enough).
	//   2. Commitment phases expired (review phase reached).
	//   3. Legacy ShatterEvent flag (backward compat, converted to pressure).
	if bb.ShatterEvent {
		bb.AddShatterPressure(bb.ShatterThreshold, tick) // convert to instant-shatter
		bb.ShatterEvent = false
	}

	shouldEval := false
	if bb.PanicLocked {
		// Panic-locked: no decisions.
		shouldEval = false
	} else if bb.ShatterReady() {
		// Accumulated pressure broke through — force re-evaluation.
		shouldEval = true
	} else if bb.CommitPhase(tick) == 2 && tick >= bb.NextDecisionTick {
		// Review phase reached and lock expired — scheduled re-evaluation.
		shouldEval = true
	} else if ef > 0.62 && bb.CommitPhase(tick) >= 1 && tick%stressReevalPeriod == 0 {
		// Stress jitter: under elevated fear, occasionally force an early review.
		// Deterministic pseudo-random roll based on id+tick keeps behaviour varied
		// without introducing non-replayable global randomness.
		roll := math.Abs(math.Sin(float64((tick + 1) * (s.id + 3))))
		if roll < (ef-0.62)*0.65 {
			shouldEval = true
			bb.AddShatterPressure(0.10+ef*0.08, tick)
		}
	}

	if shouldEval {
		// Check positive reinforcement before full re-evaluation.
		if bb.CurrentGoal != GoalAdvance && s.reinforceCurrentGoal() {
			// Extend: stay in current goal, start a new sustain window.
			bb.NextDecisionTick = tick + sustainPhaseTicks
		} else {
			// Full utility re-evaluation with hysteresis.
			goal := SelectGoalWithHysteresis(bb, &s.profile, s.isLeader, s.path != nil)
			stress := s.profile.Psych.EffectiveFear()

			bb.EvolveThresholds(goal, stress)

			sameGoal := goal == s.prevGoal
			bb.BeginCommitment(tick, sameGoal, stress)

			if !sameGoal {
				s.think(fmt.Sprintf("goal: %s → %s", s.prevGoal, goal))
				s.prevGoal = goal
				s.goalPauseTimer = s.goalSwitchPauseDuration(stress, bb.IncomingFireCount > 0)
				if goal != GoalFlank {
					bb.FlankComplete = false
				}
				// Clear MoveToContact bounding state when leaving that goal.
				if goal != GoalMoveToContact {
					s.suppressionAbort = false
					s.boundDestSet = false
				}
			}
			bb.CurrentGoal = goal
		}
	}

	// --- Malingerer override ---
	if bb.SquadHasContact || bb.IsActivated() {
		isPassive := bb.CurrentGoal == GoalAdvance ||
			bb.CurrentGoal == GoalMaintainFormation ||
			bb.CurrentGoal == GoalHoldPosition ||
			bb.CurrentGoal == GoalOverwatch
		if isPassive && bb.VisibleThreatCount() == 0 {
			bb.IdleCombatTicks++
		} else {
			bb.IdleCombatTicks = 0
			bb.ForceAdvance = false
		}
		if bb.IdleCombatTicks > 300 {
			bb.ForceAdvance = true
			bb.IdleCombatTicks = 0
			s.think("malingering — forced to advance")
		}
	} else {
		bb.IdleCombatTicks = 0
		bb.ForceAdvance = false
	}

	s.executeGoal(dt)
	s.enforcePersonalSpace()
}

// tickVal returns the current tick, defaulting to 0 if pointer is nil.
func (s *Soldier) tickVal() int {
	if s.currentTick != nil {
		return *s.currentTick
	}
	return 0
}

func (s *Soldier) goalSwitchPauseDuration(stress float64, underFire bool) int {
	pause := goalPauseBase + int(stress*16)
	if !underFire {
		pause += 6
	} else {
		pause = int(float64(pause) * 0.45)
	}
	if pause < 0 {
		pause = 0
	}
	if pause > 28 {
		pause = 28
	}
	return pause
}

// executeGoal runs the behaviour for the soldier's current goal.
func (s *Soldier) executeGoal(dt float64) {
	bb := &s.blackboard

	// Malingerer override: soldier has been idle too long while contact is known.
	// Force them toward the squad contact or combat memory position.
	if bb.ForceAdvance {
		goal := bb.CurrentGoal
		if goal != GoalEngage && goal != GoalSurvive && goal != GoalFallback && goal != GoalFlank {
			tx, ty := bb.SquadContactX, bb.SquadContactY
			if !bb.SquadHasContact {
				tx, ty = bb.CombatMemoryX, bb.CombatMemoryY
			}
			if tx != 0 || ty != 0 {
				dx := tx - s.x
				dy := ty - s.y
				if math.Sqrt(dx*dx+dy*dy) < 120 {
					bb.ForceAdvance = false
				} else {
					s.state = SoldierStateMoving
					if s.profile.Stance != StanceCrouching {
						s.profile.Stance = StanceCrouching
					}
					if s.path == nil || s.pathIndex >= len(s.path) {
						s.path = s.navGrid.FindPath(s.x, s.y, tx, ty)
						s.pathIndex = 0
					}
					s.moveAlongPath(dt)
					return
				}
			} else {
				bb.ForceAdvance = false
			}
		}
	}

	// Morale-driven reinforcement overrides passive goals.
	// If the squad leader directed this soldier toward a distressed squadmate,
	// move there unless we're already engaged or panicking.
	if bb.ShouldReinforce {
		goal := bb.CurrentGoal
		if goal != GoalEngage && goal != GoalSurvive && goal != GoalFallback {
			dx := bb.ReinforceMemberX - s.x
			dy := bb.ReinforceMemberY - s.y
			if math.Sqrt(dx*dx+dy*dy) < 60 {
				bb.ShouldReinforce = false
			} else {
				s.state = SoldierStateMoving
				if s.profile.Stance != StanceCrouching {
					s.profile.Stance = StanceCrouching
				}
				if s.path == nil || s.pathIndex >= len(s.path) {
					s.path = s.navGrid.FindPath(s.x, s.y, bb.ReinforceMemberX, bb.ReinforceMemberY)
					s.pathIndex = 0
				}
				s.moveAlongPath(dt)
				return
			}
		}
	}

	goal := s.blackboard.CurrentGoal
	if goal != GoalMoveToContact {
		s.boundHoldTicks = 0
	}
	if s.goalPauseTimer > 0 && bb.IncomingFireCount == 0 &&
		goal != GoalSurvive && goal != GoalFallback && goal != GoalEngage {
		s.state = SoldierStateIdle
		s.profile.Physical.AccumulateFatigue(0, dt)
		s.faceNearestThreatOrContact()
		return
	}

	switch goal {
	case GoalSurvive:
		if s.profile.Stance != StanceCrouching {
			s.profile.Stance = StanceCrouching
			s.think("crouching — seeking cover")
		}
		s.state = SoldierStateCover
		s.seekCoverFromThreat(dt)

	case GoalEngage:
		if s.profile.Stance != StanceCrouching {
			s.profile.Stance = StanceCrouching
		}
		bl := &s.blackboard
		// Only advance if genuinely out of effective fire range (beyond maxFireRange).
		// Inside accurateFireRange, always hold and use cover — stop the suicidal rush.
		outOfRange := bl.Internal.LastRange > maxFireRange
		// Poor range: beyond burstRange with a low hit chance. The soldier CAN fire
		// but isn't effective — they need to close distance rather than idle in cover.
		poorRange := bl.Internal.LastRange > float64(burstRange) &&
			bl.Internal.LastEstimatedHitChance < 0.55
		if outOfRange || poorRange {
			s.state = SoldierStateMoving
			s.moveToContact(dt)
		} else if !s.isInCover() {
			s.seekCoverFromThreat(dt)
		} else {
			s.state = SoldierStateIdle
			s.profile.Physical.AccumulateFatigue(0, dt)
			s.faceNearestThreat()
		}

	case GoalMoveToContact:
		if s.profile.Stance != StanceCrouching {
			s.profile.Stance = StanceCrouching
		}

		// --- Suppression interrupt: incoming fire aborts the dash ---
		if bb.IncomingFireCount > 0 && s.state == SoldierStateMoving {
			s.suppressionAbort = true
			s.path = nil
			s.pathIndex = 0
			s.think("taking fire mid-bound — seeking cover")
		}
		if s.suppressionAbort {
			// Stay suppressed until fire lifts for at least one tick.
			if bb.IncomingFireCount > 0 {
				s.seekCoverFromThreat(dt)
				break
			}
			// Fire has lifted — clear abort, resume bounding next tick.
			s.suppressionAbort = false
			s.dashOverwatchTimer = s.dashOverwatchDuration(true)
			s.think("fire lifted — holding before next bound")
		}

		// --- Buddy bounding: overwatchers hold position, movers advance ---
		if !bb.BoundMover {
			s.boundHoldTicks++
			leaderDist := 0.0
			if s.squad != nil && s.squad.Leader != nil && s.squad.Leader != s {
				leaderDist = math.Hypot(s.squad.Leader.x-s.x, s.squad.Leader.y-s.y)
			}
			distToSlot := math.Hypot(s.slotTargetX-s.x, s.slotTargetY-s.y)
			contactTooFar := bb.VisibleThreatCount() == 0 &&
				bb.Internal.LastContactRange > float64(maxFireRange)*1.05
			holdLimit := 60
			if contactTooFar {
				holdLimit = 24
			}
			if s.boundHoldTicks >= holdLimit && (contactTooFar || distToSlot > 96 || leaderDist > 180) {
				s.boundHoldTicks = 0
				s.state = SoldierStateMoving
				s.moveToContact(dt)
				break
			}
			s.state = SoldierStateIdle
			s.profile.Physical.AccumulateFatigue(0, dt)
			s.faceNearestThreatOrContact()
			break
		}
		s.boundHoldTicks = 0

		// Dash overwatch: hold still after a dash until the timer expires.
		if s.dashOverwatchTimer > 0 {
			s.state = SoldierStateIdle
			s.faceNearestThreatOrContact()
			break
		}
		s.state = SoldierStateMoving
		s.moveCombatDash(dt)

	case GoalFallback:
		if s.profile.Stance != StanceCrouching {
			s.profile.Stance = StanceCrouching
		}
		s.state = SoldierStateMoving
		s.moveFallback(dt)

	case GoalFlank:
		if s.profile.Stance != StanceCrouching {
			s.profile.Stance = StanceCrouching
		}
		// Dash overwatch: hold still after a dash until the timer expires.
		if s.dashOverwatchTimer > 0 {
			s.state = SoldierStateIdle
			s.faceNearestThreatOrContact()
			break
		}
		s.state = SoldierStateMoving
		s.moveFlank(dt)

	case GoalOverwatch:
		if s.profile.Stance != StanceCrouching {
			s.profile.Stance = StanceCrouching
		}
		s.state = SoldierStateIdle
		s.profile.Physical.AccumulateFatigue(0, dt)
		// Face toward known contact direction or gunfire.
		if s.blackboard.VisibleThreatCount() > 0 {
			s.faceNearestThreat()
		} else if s.blackboard.HeardGunfire {
			targetH := math.Atan2(s.blackboard.HeardGunfireY-s.y, s.blackboard.HeardGunfireX-s.x)
			s.vision.UpdateHeading(targetH, turnRate)
		}

	case GoalRegroup:
		if s.blackboard.VisibleThreatCount() > 0 {
			if s.profile.Stance != StanceCrouching {
				s.profile.Stance = StanceCrouching
			}
		} else {
			if s.profile.Stance != StanceStanding {
				s.profile.Stance = StanceStanding
			}
		}
		s.state = SoldierStateMoving
		s.moveAlongPath(dt)

	case GoalHoldPosition:
		if s.state != SoldierStateIdle {
			s.think("holding position")
		}
		if s.profile.Stance != StanceCrouching {
			s.profile.Stance = StanceCrouching
		}
		s.state = SoldierStateIdle
		s.profile.Physical.AccumulateFatigue(0, dt)
		if s.blackboard.VisibleThreatCount() > 0 {
			s.faceNearestThreat()
		}

	case GoalMaintainFormation:
		if s.profile.Stance != StanceStanding {
			s.profile.Stance = StanceStanding
		}
		s.state = SoldierStateMoving
		s.moveAlongPath(dt)

	case GoalAdvance:
		if s.profile.Stance != StanceStanding {
			s.profile.Stance = StanceStanding
		}
		s.state = SoldierStateMoving
		s.moveAlongPath(dt)

	case GoalPeek:
		if s.profile.Stance != StanceCrouching {
			s.profile.Stance = StanceCrouching
		}
		s.executePeek(dt)
	}
}

// faceNearestThreat turns the soldier toward the closest visible threat.
func (s *Soldier) faceNearestThreat() {
	best := math.MaxFloat64
	var bx, by float64
	for _, t := range s.blackboard.Threats {
		if !t.IsVisible {
			continue
		}
		dx := t.X - s.x
		dy := t.Y - s.y
		d := dx*dx + dy*dy
		if d < best {
			best = d
			bx = t.X
			by = t.Y
		}
	}
	if best < math.MaxFloat64 {
		targetH := math.Atan2(by-s.y, bx-s.x)
		s.vision.UpdateHeading(targetH, turnRate)
	}
}

const (
	// contactLeashMul is how many times the normal formation leash distance
	// a MoveToContact soldier can stray from the leader before pulling back.
	contactLeashMul   = 2.0
	contactLeashBase  = 240.0 // px, fallback when no squad slot info
	contactRepathDist = 32.0  // repath when contact position drifts this much
)

func (s *Soldier) recoveryUrgency() float64 {
	if s.squad == nil {
		return 0.5
	}
	switch s.squad.Phase {
	case SquadPhaseAssault:
		return 0.95
	case SquadPhaseBound:
		return 0.85
	case SquadPhaseFixFire:
		return 0.70
	case SquadPhaseStalledRecovery:
		return 0.90
	case SquadPhaseConsolidate:
		return 0.35
	default:
		return 0.55
	}
}

func (s *Soldier) chooseRecoveryAction() RecoveryAction {
	if s.recoveryCommitTicks > 0 {
		return s.recoveryAction
	}
	bb := &s.blackboard

	stallSeverity := clamp01(float64(s.recoveryNoPathStreak)/4.0*0.45 + s.recoveryStallEMA*0.55)
	routeConfidence := 1.0 - clamp01(s.recoveryRouteFailEMA)
	threatPressure := clamp01(bb.SuppressLevel*0.65 + float64(bb.IncomingFireCount)*0.12)
	supportConfidence := clamp01(float64(bb.VisibleAllyCount) / 3.0)
	urgency := s.recoveryUrgency()

	directScore := routeConfidence*0.50 + supportConfidence*0.20 + urgency*0.30 - stallSeverity*0.40
	lateralScore := stallSeverity*0.45 + urgency*0.25 + (1.0-threatPressure)*0.20 + (1.0-routeConfidence)*0.20
	anchorScore := supportConfidence*0.45 + threatPressure*0.20 + stallSeverity*0.20 + urgency*0.15
	holdScore := threatPressure*0.60 + (1.0-supportConfidence)*0.20 + stallSeverity*0.20

	best := RecoveryActionDirect
	bestScore := directScore
	if lateralScore > bestScore {
		best = RecoveryActionLateral
		bestScore = lateralScore
	}
	if anchorScore > bestScore {
		best = RecoveryActionAnchor
		bestScore = anchorScore
	}
	if holdScore > bestScore {
		best = RecoveryActionHold
	}

	s.recoveryAction = best
	s.recoveryCommitTicks = 45
	return best
}

func (s *Soldier) applyRecoveryAction(dt, targetX, targetY float64) {
	bb := &s.blackboard
	action := s.chooseRecoveryAction()
	if s.recoveryCommitTicks > 0 {
		s.recoveryCommitTicks--
	}

	tryPath := func(tx, ty float64) bool {
		newPath := s.navGrid.FindPath(s.x, s.y, tx, ty)
		if newPath == nil {
			return false
		}
		s.path = newPath
		s.pathIndex = 0
		s.slotTargetX = tx
		s.slotTargetY = ty
		s.recoveryNoPathStreak = 0
		s.recoveryRouteFailEMA = emaBlend(s.recoveryRouteFailEMA, 0, 0.30)
		s.recoveryStallEMA = emaBlend(s.recoveryStallEMA, 0.15, 0.20)
		s.state = SoldierStateMoving
		s.moveAlongPath(dt)
		return true
	}

	baseBearing := math.Atan2(targetY-s.y, targetX-s.x)

	s.recoveryNoPathStreak++
	s.recoveryRouteFailEMA = emaBlend(s.recoveryRouteFailEMA, 1, 0.30)
	s.recoveryStallEMA = emaBlend(s.recoveryStallEMA, 1, 0.25)

	switch action {
	case RecoveryActionDirect:
		if tryPath(targetX, targetY) {
			return
		}
	case RecoveryActionLateral:
		side := 1.0
		if (s.id+max(1, s.recoveryNoPathStreak))%2 == 0 {
			side = -1.0
		}
		lat := float64(cellSize) * (1.8 + 0.25*float64(min(4, s.recoveryNoPathStreak)))
		ltx := s.x + math.Cos(baseBearing+side*math.Pi/2)*lat
		lty := s.y + math.Sin(baseBearing+side*math.Pi/2)*lat
		if tryPath(ltx, lty) {
			return
		}
		if tryPath(targetX, targetY) {
			return
		}
	case RecoveryActionAnchor:
		ax, ay := targetX, targetY
		if s.squad != nil && s.squad.Leader != nil && s.squad.Leader != s {
			lx, ly := s.squad.Leader.x, s.squad.Leader.y
			anchorBearing := math.Atan2(targetY-ly, targetX-lx)
			stand := float64(cellSize) * 2.5
			ax = lx + math.Cos(anchorBearing)*stand
			ay = ly + math.Sin(anchorBearing)*stand
		}
		if tryPath(ax, ay) {
			return
		}
		if tryPath(targetX, targetY) {
			return
		}
	case RecoveryActionHold:
		if bb.IncomingFireCount > 0 || bb.IsSuppressed() {
			s.state = SoldierStateCover
			s.seekCoverFromThreat(dt)
		} else {
			s.state = SoldierStateIdle
			s.profile.Physical.AccumulateFatigue(0, dt)
			s.faceNearestThreatOrContact()
		}
		return
	}

	// Last resort if all movement recovery options failed this tick.
	s.state = SoldierStateIdle
	s.profile.Physical.AccumulateFatigue(0, dt)
	s.faceNearestThreatOrContact()
}

// moveToContact paths the soldier toward their assigned spread position (or the
// squad contact if no individual order has been issued), within the leash limit.
// Falls back to heard gunfire direction if no squad contact is available.
func (s *Soldier) moveToContact(dt float64) {
	bb := &s.blackboard
	visible := bb.VisibleThreatCount() > 0
	if !visible && !bb.SquadHasContact && !bb.HeardGunfire && !bb.IsActivated() {
		s.state = SoldierStateIdle
		return
	}

	// Prefer: visible threat > assigned spread order > squad contact > fresh audio > memory.
	var targetX, targetY float64
	if visible {
		// Find nearest visible threat.
		best := math.MaxFloat64
		var nearestTX, nearestTY float64
		for _, t := range bb.Threats {
			if !t.IsVisible {
				continue
			}
			dx := t.X - s.x
			dy := t.Y - s.y
			d := dx*dx + dy*dy
			if d < best {
				best = d
				nearestTX = t.X
				nearestTY = t.Y
			}
		}
		// Don't close inside burst range when already in a valid firing position.
		// Stop at ~burstRange so soldiers fight at effective distance, not point-blank.
		// Only push past burst range if the shot quality is genuinely poor (long-range miss streak).
		stopDist := float64(burstRange) * 0.75 // ~240px — inside burst, outside CQB rush
		distToThreat := math.Sqrt(best)
		if distToThreat <= stopDist {
			// Already in effective range — don't advance further, seek cover instead.
			s.state = SoldierStateIdle
			s.seekCoverFromThreat(dt)
			return
		}
		// Aim for a point burstRange*0.75 short of the enemy, not the enemy itself.
		bearing := math.Atan2(nearestTY-s.y, nearestTX-s.x)
		targetX = nearestTX - math.Cos(bearing)*stopDist
		targetY = nearestTY - math.Sin(bearing)*stopDist
	} else if bb.HasMoveOrder {
		targetX = bb.OrderMoveX
		targetY = bb.OrderMoveY
	} else if bb.SquadHasContact {
		targetX = bb.SquadContactX
		targetY = bb.SquadContactY
	} else if bb.HeardGunfire {
		targetX = bb.HeardGunfireX
		targetY = bb.HeardGunfireY
	} else {
		// Use persistent combat memory — last known gunfire position.
		targetX = bb.CombatMemoryX
		targetY = bb.CombatMemoryY
	}

	// Leash: don't stray too far from leader.
	if s.squad != nil && s.squad.Leader != nil && s.squad.Leader != s {
		lx, ly := s.squad.Leader.x, s.squad.Leader.y
		dx := s.x - lx
		dy := s.y - ly
		distFromLeader := math.Sqrt(dx*dx + dy*dy)
		leash := contactLeashBase * contactLeashMul
		if distFromLeader > leash {
			targetX = lx
			targetY = ly
		}
	}

	// Repath if the target has moved significantly or we have no path.
	dx := targetX - s.slotTargetX
	dy := targetY - s.slotTargetY
	drift := math.Sqrt(dx*dx + dy*dy)
	shouldRepath := s.path == nil || s.pathIndex >= len(s.path) || drift > contactRepathDist
	if !shouldRepath && s.path != nil {
		remaining := len(s.path) - s.pathIndex
		distToTarget := math.Hypot(targetX-s.x, targetY-s.y)
		if remaining <= 1 && distToTarget > float64(cellSize)*1.5 {
			shouldRepath = true
		}
	}
	if shouldRepath {
		newPath := s.navGrid.FindPath(s.x, s.y, targetX, targetY)
		if newPath != nil {
			s.path = newPath
			s.pathIndex = 0
			s.slotTargetX = targetX
			s.slotTargetY = targetY
			s.recoveryNoPathStreak = 0
			s.recoveryRouteFailEMA = emaBlend(s.recoveryRouteFailEMA, 0, 0.30)
			s.recoveryStallEMA = emaBlend(s.recoveryStallEMA, 0.10, 0.20)
			s.recoveryCommitTicks = 0
		} else {
			s.applyRecoveryAction(dt, targetX, targetY)
			return
		}
	}

	if s.path == nil || s.pathIndex >= len(s.path) {
		distToTarget := math.Hypot(targetX-s.x, targetY-s.y)
		if distToTarget > float64(cellSize) {
			s.applyRecoveryAction(dt, targetX, targetY)
			return
		}
	}

	s.moveAlongPath(dt)
}

// moveFallback paths the soldier directly away from the squad contact position.
// It picks a point behind the soldier relative to the contact, at a fixed retreat
// distance, then A*-paths there.
func (s *Soldier) moveFallback(dt float64) {
	bb := &s.blackboard

	// Resolve the contact position to retreat from.
	// Priority: visible threat > squad contact > heard gunfire.
	var cX, cY float64
	hasC := false
	if bb.VisibleThreatCount() > 0 {
		best := math.MaxFloat64
		for _, t := range bb.Threats {
			if !t.IsVisible {
				continue
			}
			dx2 := t.X - s.x
			dy2 := t.Y - s.y
			d := dx2*dx2 + dy2*dy2
			if d < best {
				best = d
				cX, cY = t.X, t.Y
				hasC = true
			}
		}
	} else if bb.SquadHasContact {
		cX, cY = bb.SquadContactX, bb.SquadContactY
		hasC = true
	} else if bb.HeardGunfire {
		cX, cY = bb.HeardGunfireX, bb.HeardGunfireY
		hasC = true
	}
	if !hasC {
		s.state = SoldierStateIdle
		return
	}

	const retreatDist = 120.0

	// Direction away from contact.
	dx := s.x - cX
	dy := s.y - cY
	dist := math.Sqrt(dx*dx + dy*dy)
	if dist < 1e-6 {
		// Degenerate: retreat toward start side of map.
		dx, dy = -1, 0
		dist = 1
	}
	targetX := s.x + (dx/dist)*retreatDist
	targetY := s.y + (dy/dist)*retreatDist

	// Clamp to map bounds roughly.
	if s.navGrid != nil {
		w := float64(s.navGrid.cols * cellSize)
		h := float64(s.navGrid.rows * cellSize)
		if targetX < 16 {
			targetX = 16
		}
		if targetX > w-16 {
			targetX = w - 16
		}
		if targetY < 16 {
			targetY = 16
		}
		if targetY > h-16 {
			targetY = h - 16
		}
	}

	// Repath when the retreat point drifts (fear may rise/fall tick by tick).
	radx := targetX - s.slotTargetX
	rady := targetY - s.slotTargetY
	drift := math.Sqrt(radx*radx + rady*rady)
	if s.path == nil || s.pathIndex >= len(s.path) || drift > contactRepathDist {
		newPath := s.navGrid.FindPath(s.x, s.y, targetX, targetY)
		if newPath != nil {
			s.path = newPath
			s.pathIndex = 0
			s.slotTargetX = targetX
			s.slotTargetY = targetY
		}
	}

	s.moveAlongPath(dt)
}

// moveFlank moves the soldier perpendicular to the enemy bearing for flankDistance.
// Once the perpendicular leg is complete, it sets FlankComplete=true and triggers
// a shatter event so the soldier immediately re-evaluates (overwatch or advance).
func (s *Soldier) moveFlank(dt float64) {
	bb := &s.blackboard

	// Need a contact direction to flank relative to.
	// Priority: visible threat > squad contact > fresh audio > persistent memory.
	var contactX, contactY float64
	if bb.VisibleThreatCount() > 0 {
		best := math.MaxFloat64
		for _, t := range bb.Threats {
			if !t.IsVisible {
				continue
			}
			dx := t.X - s.x
			dy := t.Y - s.y
			d := dx*dx + dy*dy
			if d < best {
				best = d
				contactX, contactY = t.X, t.Y
			}
		}
	} else if bb.SquadHasContact {
		contactX, contactY = bb.SquadContactX, bb.SquadContactY
	} else if bb.HeardGunfire {
		contactX, contactY = bb.HeardGunfireX, bb.HeardGunfireY
	} else if bb.IsActivated() {
		contactX, contactY = bb.CombatMemoryX, bb.CombatMemoryY
	} else {
		s.state = SoldierStateIdle
		return
	}

	// If we haven't set a flank target yet, compute it.
	if bb.FlankTargetX == 0 && bb.FlankTargetY == 0 && !bb.FlankComplete {
		// Use the squad-level enemy bearing so all members share a consistent
		// reference frame. FlankSide +1 = left of enemy normal, -1 = right.
		bearing := bb.SquadEnemyBearing
		if bearing == 0 {
			// Fallback: compute from visible contact if squad bearing not yet set.
			bearing = math.Atan2(contactY-s.y, contactX-s.x)
		}
		side := bb.FlankSide
		if side == 0 {
			side = 1.0
		}
		// perpAngle: left of bearing = bearing - Pi/2, right = bearing + Pi/2
		perpAngle := bearing - side*math.Pi/2
		bb.FlankTargetX = s.x + math.Cos(perpAngle)*flankDistance
		bb.FlankTargetY = s.y + math.Sin(perpAngle)*flankDistance

		// Clamp to map bounds.
		if s.navGrid != nil {
			w := float64(s.navGrid.cols * cellSize)
			h := float64(s.navGrid.rows * cellSize)
			if bb.FlankTargetX < 16 {
				bb.FlankTargetX = 16
			}
			if bb.FlankTargetX > w-16 {
				bb.FlankTargetX = w - 16
			}
			if bb.FlankTargetY < 16 {
				bb.FlankTargetY = 16
			}
			if bb.FlankTargetY > h-16 {
				bb.FlankTargetY = h - 16
			}
		}

		newPath := s.navGrid.FindPath(s.x, s.y, bb.FlankTargetX, bb.FlankTargetY)
		if newPath != nil {
			s.path = newPath
			s.pathIndex = 0
			s.slotTargetX = bb.FlankTargetX
			s.slotTargetY = bb.FlankTargetY
			s.think("flanking")
		}
	}

	// Check if we've arrived at the flank target.
	dx := bb.FlankTargetX - s.x
	dy := bb.FlankTargetY - s.y
	if math.Sqrt(dx*dx+dy*dy) < 20 || (s.path != nil && s.pathIndex >= len(s.path)) {
		bb.FlankComplete = true
		bb.ShatterEvent = true // force immediate re-evaluation → overwatch or advance
		bb.FlankTargetX = 0
		bb.FlankTargetY = 0
		s.think("flank complete — reassessing")
		return
	}

	s.moveAlongPath(dt)
}

// pathLookaheadMax is the maximum number of waypoints to skip via LOS smoothing.
const pathLookaheadMax = 12

// moveAlongPath advances the soldier along the current A* path,
// using stance-aware speed and updating heading.
//
// Path smoothing: instead of following every cell-sized waypoint, the soldier
// looks ahead for the farthest waypoint with clear LOS and moves directly
// toward it. The look-ahead distance is stress-dependent — calm soldiers plan
// longer movement legs; stressed or suppressed soldiers take shorter, cautious steps.
func (s *Soldier) moveAlongPath(dt float64) {
	if s.path == nil || s.pathIndex >= len(s.path) {
		// One-way advance: idle at objective.
		s.state = SoldierStateIdle
		return
	}

	// LOS-based path smoothing: skip intermediate waypoints the soldier can see.
	// Look-ahead scales inversely with stress/suppression.
	stress := s.profile.Psych.EffectiveFear()
	suppression := float64(s.blackboard.IncomingFireCount) * 0.15
	pressure := math.Min(1.0, stress+suppression)
	// Calm: look ahead up to pathLookaheadMax waypoints. Panicked: only 1-2.
	lookahead := int(float64(pathLookaheadMax) * (1.0 - pressure*0.85))
	if lookahead < 1 {
		lookahead = 1
	}

	// Find the farthest reachable waypoint with clear LOS from current position.
	bestIdx := s.pathIndex
	maxCheck := s.pathIndex + lookahead
	if maxCheck > len(s.path) {
		maxCheck = len(s.path)
	}
	for i := s.pathIndex + 1; i < maxCheck; i++ {
		wp := s.path[i]
		if HasLineOfSight(s.x, s.y, wp[0], wp[1], s.buildings) {
			bestIdx = i
		} else {
			break // walls block further look-ahead
		}
	}
	// Skip intermediate waypoints.
	if bestIdx > s.pathIndex {
		s.pathIndex = bestIdx
	}

	speed := s.profile.EffectiveSpeed(soldierSpeed)
	// Leader cohesion: slow down when squad is spread out.
	if s.isLeader && s.squad != nil {
		speed *= s.squad.LeaderCohesionSlowdown()
	}
	// Cover terrain slowdown: rubble and chest-walls reduce speed.
	coverMul := 1.0
	for _, c := range s.covers {
		if c.SlowsMovement() {
			cx0, cy0, cx1, cy1 := c.Rect()
			if s.x >= float64(cx0) && s.x < float64(cx1) && s.y >= float64(cy0) && s.y < float64(cy1) {
				m := c.MovementMul()
				if m < coverMul {
					coverMul = m
				}
			}
		}
	}
	speed *= coverMul
	exertion := speed / soldierSpeed
	s.profile.Physical.AccumulateFatigue(exertion, dt)

	remaining := speed
	for remaining > 0 && s.pathIndex < len(s.path) {
		wp := s.path[s.pathIndex]
		dx := wp[0] - s.x
		dy := wp[1] - s.y
		dist := math.Sqrt(dx*dx + dy*dy)

		// Turn toward next waypoint.
		if dist > 1e-6 {
			targetHeading := math.Atan2(dy, dx)
			s.vision.UpdateHeading(targetHeading, turnRate)
		}

		if dist <= remaining {
			s.x = wp[0]
			s.y = wp[1]
			remaining -= dist
			s.pathIndex++
		} else {
			s.x += (dx / dist) * remaining
			s.y += (dy / dist) * remaining
			remaining = 0
		}
	}

	if s.pathIndex >= len(s.path) {
		s.state = SoldierStateIdle
		// Post-arrival pause: brief scan before re-evaluating goal.
		// Skip for combat-critical goals (dashOverwatchTimer handles those).
		g := s.blackboard.CurrentGoal
		combatGoal := g == GoalMoveToContact || g == GoalFlank || g == GoalFallback || g == GoalSurvive
		if !combatGoal && s.postArrivalTimer == 0 {
			ef := s.profile.Psych.EffectiveFear()
			// Calm disciplined soldiers take a deliberate pause (~0.8-2s).
			// Fearful or activated soldiers pause less.
			pauseTicks := int(float64(postArrivalBase) *
				(0.5 + s.profile.Skills.Discipline*0.5) *
				(1.0 - ef*0.6))
			if pauseTicks < 10 {
				pauseTicks = 10
			}
			s.postArrivalTimer = pauseTicks
		}
	}
}

func (s *Soldier) enforcePersonalSpace() {
	if s.squad == nil {
		return
	}
	for _, m := range s.squad.Members {
		if m == s || m.state == SoldierStateDead {
			continue
		}
		// Resolve each pair once to avoid symmetric push jitter.
		if s.id < m.id {
			s.resolveCellOverlapPair(m)
			s.applySeparationPair(m)
		}
	}
}

func (s *Soldier) resolveCellOverlapPair(other *Soldier) {
	if int(s.x/float64(cellSize)) != int(other.x/float64(cellSize)) ||
		int(s.y/float64(cellSize)) != int(other.y/float64(cellSize)) {
		return
	}

	dx := s.x - other.x
	dy := s.y - other.y
	if math.Abs(dx)+math.Abs(dy) < 1e-6 {
		t := float64((s.tickVal() + 1) * (s.id + 7))
		dx = math.Cos(t * 0.73)
		dy = math.Sin(t * 0.91)
	}
	d := math.Sqrt(dx*dx + dy*dy)
	if d < 1e-6 {
		return
	}
	nx := dx / d
	ny := dy / d
	push := float64(cellSize)*0.55 + cellOverlapEpsilon
	half := push * 0.5
	s.x += nx * half
	s.y += ny * half
	other.x -= nx * half
	other.y -= ny * half
}

func (s *Soldier) applySeparationPair(other *Soldier) {
	dx := s.x - other.x
	dy := s.y - other.y
	d := math.Sqrt(dx*dx + dy*dy)
	if d < 1e-6 || d >= personalSpaceRadius {
		return
	}
	pressure := (personalSpaceRadius - d) / personalSpaceRadius
	nx := dx / d
	ny := dy / d
	push := pressure * 0.55
	if push < 0.05 {
		push = 0.05
	}
	half := push * 0.5
	s.x += nx * half
	s.y += ny * half
	other.x -= nx * half
	other.y -= ny * half
}

// Corner peek vision parameters.
const (
	peekFOVDeg = 45.0  // narrow peek arc in degrees
	peekRange  = 120.0 // reduced range for cautious glance (pixels)
)

// UpdateVision performs vision scan against enemies.
// When at a corner or doorway, soldiers also get a limited peek scan.
func (s *Soldier) UpdateVision(enemies []*Soldier, buildings []rect) {
	if s.state == SoldierStateDead {
		return
	}
	s.vision.PerformVisionScan(s.x, s.y, enemies, buildings, s.covers)

	// Corner/doorway peek: if wall-adjacent and at a corner, perform a
	// supplementary narrow-FOV scan in peek directions. This simulates
	// cautiously glancing around edges.
	if s.tacticalMap != nil && s.blackboard.AtWall {
		peekDirs := s.tacticalMap.CornerPeekDirections(s.x, s.y)
		if len(peekDirs) > 0 {
			peekFOV := peekFOVDeg * math.Pi / 180.0
			for _, dir := range peekDirs {
				s.peekScan(dir, peekFOV, peekRange, enemies, buildings)
			}
		}
	}

	// Seeing enemies increases fear, but should not permanently prevent recovery
	// when contact is distant and no rounds are landing.
	if len(s.vision.KnownContacts) > 0 {
		minDist := math.MaxFloat64
		for _, e := range s.vision.KnownContacts {
			dx := e.x - s.x
			dy := e.y - s.y
			d := math.Sqrt(dx*dx + dy*dy)
			if d < minDist {
				minDist = d
			}
		}
		// Near threats create meaningful stress; far threats create minimal pressure.
		nearFactor := clamp01(1.0 - minDist/(maxFireRange*1.25))
		stress := 0.004 * float64(len(s.vision.KnownContacts)) * nearFactor
		if s.blackboard.IncomingFireCount > 0 {
			stress += 0.004
		}
		if stress > 0 {
			s.profile.Psych.ApplyStress(stress)
		}
	}

	// A corner that reveals enemies is considered high value — reinforce staying.
	if s.blackboard.AtCorner && len(s.vision.KnownContacts) > 0 {
		// Boost position desirability when the corner provides tactical advantage.
		s.blackboard.PositionDesirability = math.Min(1.0, s.blackboard.PositionDesirability+0.3)
	}
}

// peekScan performs a narrow vision scan in a specific direction, adding any
// newly spotted enemies to KnownContacts. Used for corner/doorway peeking.
func (s *Soldier) peekScan(direction, fov, maxRange float64, enemies []*Soldier, buildings []rect) {
	halfFOV := fov / 2.0
	for _, e := range enemies {
		if e.state == SoldierStateDead {
			continue
		}
		// Check if already in KnownContacts.
		alreadyKnown := false
		for _, kc := range s.vision.KnownContacts {
			if kc == e {
				alreadyKnown = true
				break
			}
		}
		if alreadyKnown {
			continue
		}

		dx := e.x - s.x
		dy := e.y - s.y
		dist := math.Sqrt(dx*dx + dy*dy)
		if dist > maxRange || dist < 1e-6 {
			continue
		}

		// Check if within the narrow peek cone.
		angleToTarget := math.Atan2(dy, dx)
		diff := normalizeAngle(angleToTarget - direction)
		if diff < -halfFOV || diff > halfFOV {
			continue
		}

		// LOS check through buildings and cover.
		if HasLineOfSightWithCover(s.x, s.y, e.x, e.y, buildings, s.covers) {
			s.vision.KnownContacts = append(s.vision.KnownContacts, e)
		}
	}
}

// Draw renders the soldier with layered circles, a directional chevron,
// stance rings, goal-state colour coding, and a health bar.
func (s *Soldier) Draw(screen *ebiten.Image, offX, offY int) {
	ox, oy := float32(offX), float32(offY)
	sx, sy := ox+float32(s.x), oy+float32(s.y)

	if s.state == SoldierStateDead {
		// Pool of darkness under the body.
		vector.FillCircle(screen, sx+1.5, sy+1.5, float32(soldierRadius)+4, color.RGBA{R: 20, G: 5, B: 5, A: 140}, false)
		var dc color.RGBA
		if s.team == TeamRed {
			dc = color.RGBA{R: 70, G: 18, B: 18, A: 180}
		} else {
			dc = color.RGBA{R: 18, G: 28, B: 70, A: 180}
		}
		vector.FillCircle(screen, sx, sy, float32(soldierRadius)+1, dc, false)
		// White X.
		d := float32(soldierRadius) * 0.7
		grey := color.RGBA{R: 220, G: 220, B: 220, A: 200}
		vector.StrokeLine(screen, sx-d, sy-d, sx+d, sy+d, 2.0, grey, false)
		vector.StrokeLine(screen, sx+d, sy-d, sx-d, sy+d, 2.0, grey, false)
		return
	}

	// --- Radius by stance ---
	// Standing=full, crouching=smaller, prone=very small.
	baseR := float32(soldierRadius) + 2 // slightly larger than before
	radius := baseR * float32(s.profile.Stance.Profile().ProfileMul)
	if radius < 3 {
		radius = 3
	}

	// --- Goal-based fill colour ---
	// Body colour encodes current goal state for quick readability.
	var fill color.RGBA
	switch s.team {
	case TeamRed:
		fill = color.RGBA{R: 210, G: 35, B: 35, A: 255}
	case TeamBlue:
		fill = color.RGBA{R: 35, G: 75, B: 215, A: 255}
	}
	switch s.blackboard.CurrentGoal {
	case GoalSurvive:
		// Panic / cowering — bright yellow warning.
		fill = color.RGBA{R: 240, G: 220, B: 20, A: 255}
	case GoalEngage:
		// Engaging — brighten the team colour.
		fill = color.RGBA{R: uint8(min8(255, int(fill.R)+50)), G: uint8(min8(255, int(fill.G)+20)), B: uint8(min8(255, int(fill.B)+20)), A: 255}
	case GoalFallback:
		// Falling back — orange tint.
		if s.team == TeamRed {
			fill = color.RGBA{R: 220, G: 120, B: 20, A: 255}
		} else {
			fill = color.RGBA{R: 20, G: 120, B: 180, A: 255}
		}
	case GoalSurvive - 1: // GoalMoveToContact or GoalFlank — darker, purposeful
	}
	if s.state == SoldierStateCover {
		// In cover — darken significantly.
		fill = color.RGBA{R: fill.R / 2, G: fill.G / 2, B: fill.B / 2, A: 255}
	}

	// --- Shadow drop ---
	vector.FillCircle(screen, sx+1.5, sy+2.0, radius+1.0, color.RGBA{R: 0, G: 0, B: 0, A: 100}, false)

	// --- Outer dark outline ring ---
	vector.FillCircle(screen, sx, sy, radius+2.0, color.RGBA{R: 0, G: 0, B: 0, A: 200}, false)

	// --- Team rim ring (bright team colour at edge) ---
	var rimCol color.RGBA
	if s.team == TeamRed {
		rimCol = color.RGBA{R: 255, G: 80, B: 80, A: 220}
	} else {
		rimCol = color.RGBA{R: 80, G: 140, B: 255, A: 220}
	}
	vector.StrokeCircle(screen, sx, sy, radius+0.8, 2.0, rimCol, false)

	// --- Body fill ---
	vector.FillCircle(screen, sx, sy, radius, fill, false)

	// --- Inner highlight (top-left gleam) ---
	hlR := radius * 0.45
	vector.FillCircle(screen, sx-radius*0.22, sy-radius*0.22, hlR,
		color.RGBA{R: 255, G: 255, B: 255, A: 35}, false)

	// --- Stance indicator ring ---
	// Prone: inner dot. Crouching: dashed inner ring.
	switch s.profile.Stance {
	case StanceProne:
		// Small central dot — soldier is flat.
		vector.FillCircle(screen, sx, sy, 2.0, color.RGBA{R: 255, G: 255, B: 255, A: 160}, false)
	case StanceCrouching:
		// Subtle inner ring.
		vector.StrokeCircle(screen, sx, sy, radius*0.55, 1.0,
			color.RGBA{R: 255, G: 255, B: 255, A: 60}, false)
	}

	// --- Leader marker: gold double ring ---
	if s.isLeader {
		vector.StrokeCircle(screen, sx, sy, radius+3.5, 1.5,
			color.RGBA{R: 255, G: 230, B: 60, A: 230}, false)
		vector.StrokeCircle(screen, sx, sy, radius+5.5, 1.0,
			color.RGBA{R: 255, G: 230, B: 60, A: 100}, false)
	}

	// --- Directional chevron instead of a plain line ---
	// Points in heading direction; length scales with radius.
	h := s.vision.Heading
	tipDist := radius + 6.0
	wingBack := radius * 0.5
	wingSpread := radius * 0.55

	tipX := sx + float32(math.Cos(h))*tipDist
	tipY := sy + float32(math.Sin(h))*tipDist
	lbX := sx + float32(math.Cos(h+2.6))*wingBack
	lbY := sy + float32(math.Sin(h+2.6))*wingBack
	rbX := sx + float32(math.Cos(h-2.6))*wingBack
	rbY := sy + float32(math.Sin(h-2.6))*wingBack
	// Wingtip spread perpendicular.
	lwX := tipX + float32(math.Cos(h+math.Pi/2))*wingSpread
	lwY := tipY + float32(math.Sin(h+math.Pi/2))*wingSpread
	rwX := tipX + float32(math.Cos(h-math.Pi/2))*wingSpread
	rwY := tipY + float32(math.Sin(h-math.Pi/2))*wingSpread

	var chevCol color.RGBA
	if s.team == TeamRed {
		chevCol = color.RGBA{R: 255, G: 210, B: 200, A: 220}
	} else {
		chevCol = color.RGBA{R: 200, G: 220, B: 255, A: 220}
	}
	// Left arm.
	vector.StrokeLine(screen, lbX, lbY, tipX, tipY, 1.8, chevCol, false)
	// Right arm.
	vector.StrokeLine(screen, rbX, rbY, tipX, tipY, 1.8, chevCol, false)
	// Wingtips (short barbs).
	vector.StrokeLine(screen, tipX, tipY, lwX, lwY, 1.2, chevCol, false)
	vector.StrokeLine(screen, tipX, tipY, rwX, rwY, 1.2, chevCol, false)
	_ = lbX
	_ = rbX

	// --- Panic indicator: pulsing ring ---
	if s.blackboard.PanicLocked {
		tick := 0
		if s.currentTick != nil {
			tick = *s.currentTick
		}
		pulseAlpha := uint8(100 + 80*math.Abs(math.Sin(float64(tick)*0.15)))
		vector.StrokeCircle(screen, sx, sy, radius+8.0, 2.0,
			color.RGBA{R: 255, G: 220, B: 0, A: pulseAlpha}, false)
	}

	// --- Health bar ---
	barW := (radius + 4) * 2.2
	barH := float32(4.0)
	bx := sx - barW/2
	barY := sy + radius + 4
	vector.FillRect(screen, bx-1, barY-1, barW+2, barH+2, color.RGBA{R: 0, G: 0, B: 0, A: 200}, false)
	vector.FillRect(screen, bx, barY, barW, barH, color.RGBA{R: 25, G: 25, B: 25, A: 220}, false)
	frac := float32(s.health / soldierMaxHP)
	filled := barW * frac
	var hpR, hpG uint8
	if frac > 0.5 {
		t2 := (frac - 0.5) * 2.0
		hpR = uint8(255 * (1.0 - t2))
		hpG = 220
	} else {
		t2 := frac * 2.0
		hpR = 255
		hpG = uint8(220 * t2)
	}
	vector.FillRect(screen, bx, barY, filled, barH, color.RGBA{R: hpR, G: hpG, B: 20, A: 240}, false)
	// Glossy highlight.
	vector.StrokeLine(screen, bx, barY, bx+filled, barY, 1.0,
		color.RGBA{R: 255, G: 255, B: 255, A: 50}, false)
}

// isInCover returns true if the soldier currently has meaningful cover between
// them and the nearest visible threat.
func (s *Soldier) isInCover() bool {
	for _, t := range s.blackboard.Threats {
		if !t.IsVisible {
			continue
		}
		inCover, defence := IsBehindCover(s.x, s.y, t.X, t.Y, s.covers)
		if inCover && defence >= 0.30 {
			return true
		}
	}
	return false
}

// threatDirection returns the average angle from this soldier toward all visible
// threats. Returns (0, false) if no visible threats.
func (s *Soldier) threatDirection() (float64, bool) {
	var sumX, sumY float64
	count := 0
	for _, t := range s.blackboard.Threats {
		if !t.IsVisible {
			continue
		}
		dx := t.X - s.x
		dy := t.Y - s.y
		sumX += dx
		sumY += dy
		count++
	}
	if count == 0 {
		// Fall back to squad contact direction if available.
		if s.blackboard.SquadHasContact {
			dx := s.blackboard.SquadContactX - s.x
			dy := s.blackboard.SquadContactY - s.y
			if dx*dx+dy*dy > 1 {
				return math.Atan2(dy, dx), true
			}
		}
		return 0, false
	}
	return math.Atan2(sumY, sumX), true
}

// seekCoverFromThreat finds the best nearby cover object relative to the threat
// direction and paths the soldier toward the protected side of it.
// If no cover is found, or the soldier is already in good cover, holds in place.
func (s *Soldier) seekCoverFromThreat(dt float64) {
	threatAngle, hasThreat := s.threatDirection()
	if !hasThreat {
		// No threat info — just freeze in place.
		s.state = SoldierStateCover
		s.profile.Physical.AccumulateFatigue(0, dt)
		return
	}

	// Already in cover from this threat — hold and face the enemy.
	if s.isInCover() {
		s.coverTarget = nil
		s.state = SoldierStateCover
		s.profile.Physical.AccumulateFatigue(0, dt)
		s.faceNearestThreat()
		return
	}

	// Find a new cover target if we don't have one or have reached the old one.
	if s.coverTarget == nil || s.isNearCoverTarget() {
		best := FindCoverForThreat(s.x, s.y, threatAngle, s.covers, nil, coverSearchDist)
		if best != nil {
			s.coverTarget = best
			px, py := CoverPositionBehind(best, threatAngle)
			newPath := s.navGrid.FindPath(s.x, s.y, px, py)
			if newPath != nil {
				s.path = newPath
				s.pathIndex = 0
				s.slotTargetX = px
				s.slotTargetY = py
				s.think("seeking cover")
			} else {
				// Can't path there — clear target so we try again next tick.
				s.coverTarget = nil
			}
		}
	}

	if s.path != nil && s.pathIndex < len(s.path) {
		s.state = SoldierStateMoving
		s.moveAlongPath(dt)
	} else {
		s.state = SoldierStateCover
		s.profile.Physical.AccumulateFatigue(0, dt)
		s.faceNearestThreat()
	}
}

// isNearCoverTarget returns true when the soldier is close enough to their
// current cover target that they should stop pathing.
func (s *Soldier) isNearCoverTarget() bool {
	if s.coverTarget == nil {
		return false
	}
	cx := float64(s.coverTarget.x) + coverCellSize/2.0
	cy := float64(s.coverTarget.y) + coverCellSize/2.0
	dx := cx - s.x
	dy := cy - s.y
	return dx*dx+dy*dy < float64(coverCellSize*coverCellSize)*2.0
}

func min8(a, b int) uint8 {
	if a < b {
		return uint8(a) // #nosec G115 -- a is always in [0,255] at all call sites
	}
	return uint8(b) // #nosec G115 -- b is always in [0,255] at all call sites
}

// applyProximityStress evaluates nearby soldiers and applies fuzzy stress responses.
//
// Two sources:
//  1. Enemy proximity — being close to an enemy is terrifying even without shooting.
//     Fuzzy ramp: max stress at point-blank, zero at proxEnemyStressRange.
//     CQB claustrophobia: extra stress when BOTH range is close AND sightline is low.
//  2. Friendly crowding — being bunched with teammates is uncomfortable under fire.
//     Softer ramp, only active when already under combat stress.
//
// Both effects are dampened by the soldier's composure stat (veterans handle it better).
// Called every sightlineUpdateRate ticks (~2s), not per-tick.
func (s *Soldier) applyProximityStress() {
	composureDamp := 0.5 + 0.5*s.profile.Psych.Composure // 0.5..1.0 dampening

	// -- Enemy proximity --
	for _, e := range s.vision.KnownContacts {
		if e.state == SoldierStateDead {
			continue
		}
		dx := e.x - s.x
		dy := e.y - s.y
		dist := math.Sqrt(dx*dx + dy*dy)
		if dist >= float64(proxEnemyStressRange) {
			continue
		}
		// Fuzzy ramp: t=1 at point-blank, t=0 at proxEnemyStressRange.
		t := 1.0 - dist/float64(proxEnemyStressRange)
		// Base stress: up to 0.12 for a visible enemy very close.
		stress := 0.12 * t * t // quadratic — sharper spike at very short range
		// CQB claustrophobia multiplier: low sightline AND close range.
		if s.blackboard.LocalSightlineScore < autoSightlineThresh && dist < float64(autoRange) {
			cqbFactor := clamp01((autoSightlineThresh - s.blackboard.LocalSightlineScore) / autoSightlineThresh)
			distFactor := clamp01(1.0 - dist/float64(autoRange))
			stress *= 1.0 + cqbFactor*distFactor*1.5
		}
		s.profile.Psych.ApplyStress(stress / composureDamp)
	}

	// -- Friendly crowding (only meaningful under fire) --
	if s.blackboard.IncomingFireCount == 0 && s.profile.Psych.Fear < 0.3 {
		return // calm soldiers don't mind their squadmates nearby
	}
	if s.squad == nil {
		return
	}
	for _, m := range s.squad.Members {
		if m == s || m.state == SoldierStateDead {
			continue
		}
		dx := m.x - s.x
		dy := m.y - s.y
		dist := math.Sqrt(dx*dx + dy*dy)
		if dist >= float64(proxFriendCrowdRange) {
			continue
		}
		// Very soft ramp — crowding is uncomfortable but not panic-inducing.
		t := 1.0 - dist/float64(proxFriendCrowdRange)
		stress := 0.03 * t
		s.profile.Psych.ApplyStress(stress / composureDamp)
	}
}

// faceNearestThreatOrContact turns toward the nearest visible threat, falling
// back to squad contact position or heard gunfire if no threat is visible.
func (s *Soldier) faceNearestThreatOrContact() {
	if s.blackboard.VisibleThreatCount() > 0 {
		s.faceNearestThreat()
		return
	}
	bb := &s.blackboard
	var tx, ty float64
	if bb.SquadHasContact {
		tx, ty = bb.SquadContactX, bb.SquadContactY
	} else if bb.HeardGunfire {
		tx, ty = bb.HeardGunfireX, bb.HeardGunfireY
	} else if bb.IsActivated() {
		tx, ty = bb.CombatMemoryX, bb.CombatMemoryY
	} else {
		return
	}
	heading := math.Atan2(ty-s.y, tx-s.x)
	s.vision.UpdateHeading(heading, turnRate)
}

// dashOverwatchDuration computes how many ticks a soldier pauses after a dash.
// Disciplined soldiers hold overwatch longer; fearful ones are less patient.
func (s *Soldier) dashOverwatchDuration(underFire bool) int {
	base := float64(dashOverwatchBase)
	disciplineMul := 0.7 + s.profile.Skills.Discipline*0.6 // 0.7–1.3
	fearMul := 1.0 - s.profile.Psych.EffectiveFear()*0.5   // 0.5–1.0
	if underFire {
		fearMul *= 0.4 // much shorter pause when rounds are incoming
	}
	d := int(base * disciplineMul * fearMul)
	if d < 20 {
		d = 20
	}
	return d
}

// moveCombatDash moves the soldier in a cover-to-cover bounding pattern:
//  1. Resolve the ultimate destination (contact / squad order / gunfire / memory).
//  2. Compute bearing toward that destination.
//  3. Search for an intermediate cover position along the bearing using the TacticalMap.
//  4. Sprint (dash speed) to the intermediate cover position.
//  5. On arrival, start a dashOverwatchTimer to pause and scan.
//  6. Next cycle, repeat from step 2 — each bound leapfrogs closer.
//
// Bound distance scales with proximity to contact: far away = long bounds (8-12 cells),
// close = short cautious bounds (3-5 cells). If no TacticalMap is available, falls
// back to a direct dash toward the ultimate destination.
func (s *Soldier) moveCombatDash(_ float64) {
	bb := &s.blackboard

	// --- Step 1: Resolve ultimate destination ---
	var destX, destY float64
	if bb.HasMoveOrder {
		destX = bb.OrderMoveX
		destY = bb.OrderMoveY
	} else if bb.HasBestNearby && bb.BestNearbyScore > bb.PositionDesirability+0.10 {
		destX = bb.BestNearbyX
		destY = bb.BestNearbyY
	} else if bb.SquadHasContact {
		cBearing := math.Atan2(bb.SquadContactY-s.y, bb.SquadContactX-s.x)
		stopDist := float64(burstRange) * 0.75
		destX = bb.SquadContactX - math.Cos(cBearing)*stopDist
		destY = bb.SquadContactY - math.Sin(cBearing)*stopDist
	} else if bb.HeardGunfire {
		destX = bb.HeardGunfireX
		destY = bb.HeardGunfireY
	} else if bb.IsActivated() {
		destX = bb.CombatMemoryX
		destY = bb.CombatMemoryY
	} else {
		s.state = SoldierStateIdle
		return
	}

	// Leash: don't stray too far from leader.
	if s.squad != nil && s.squad.Leader != nil && s.squad.Leader != s {
		lx, ly := s.squad.Leader.x, s.squad.Leader.y
		dx := s.x - lx
		dy := s.y - ly
		if math.Sqrt(dx*dx+dy*dy) > contactLeashBase*contactLeashMul {
			destX = lx
			destY = ly
		}
	}

	// Update stored ultimate destination.
	s.boundDestX = destX
	s.boundDestY = destY
	s.boundDestSet = true

	// --- Step 2: Compute bearing and distance to destination ---
	ddx := destX - s.x
	ddy := destY - s.y
	distToDest := math.Sqrt(ddx*ddx + ddy*ddy)
	bearing := math.Atan2(ddy, ddx)

	// --- Step 3: Pick an intermediate bound target ---
	// Bound distance scales with proximity: far = long bounds, close = short cautious bounds.
	// Distance thresholds in cells (1 cell = cellSize px).
	var targetX, targetY float64
	usedCover := false

	if s.tacticalMap != nil && distToDest > float64(cellSize)*4 {
		// Scale bound length by distance to contact.
		maxBound := 12 // cells, ~192px — long bound when far away
		minBound := 3  // cells, ~48px — minimum bound distance
		// Close approach: tighten bounds as distance shrinks.
		distCells := int(distToDest / float64(cellSize))
		boundLen := maxBound
		if distCells < 25 {
			// Linear ramp: 25 cells → max, 5 cells → min.
			t := clamp01(float64(distCells-5) / 20.0)
			boundLen = minBound + int(t*float64(maxBound-minBound))
		}
		// Disciplined soldiers use longer, more deliberate bounds.
		boundLen += int(s.profile.Skills.Discipline * 3)
		if boundLen > maxBound+3 {
			boundLen = maxBound + 3
		}

		// Search for cover along the bearing.
		bx, by, bscore, found := s.tacticalMap.FindBoundCover(s.x, s.y, bearing, minBound, boundLen)
		if found && bscore > -0.5 {
			targetX = bx
			targetY = by
			usedCover = true
		}
	}

	// Fallback: if no cover found or no tactical map, dash directly toward destination.
	if !usedCover {
		// Clamp to a maximum dash distance so we don't sprint endlessly.
		maxDashDist := float64(cellSize) * 10
		if distToDest > maxDashDist {
			targetX = s.x + (ddx/distToDest)*maxDashDist
			targetY = s.y + (ddy/distToDest)*maxDashDist
		} else {
			targetX = destX
			targetY = destY
		}
	}

	// If we're very close to the final destination, just go straight there.
	if distToDest < float64(cellSize)*3 {
		targetX = destX
		targetY = destY
	}

	// --- Step 4: Path and sprint to the intermediate target ---
	// Repath if target drifted significantly from where we were heading.
	dx := targetX - s.slotTargetX
	dy := targetY - s.slotTargetY
	if s.path == nil || s.pathIndex >= len(s.path) || math.Sqrt(dx*dx+dy*dy) > contactRepathDist {
		newPath := s.navGrid.FindPath(s.x, s.y, targetX, targetY)
		if newPath != nil {
			s.path = newPath
			s.pathIndex = 0
			s.slotTargetX = targetX
			s.slotTargetY = targetY
		}
	}

	if s.path == nil || s.pathIndex >= len(s.path) {
		// Arrived at intermediate cover — start overwatch pause.
		s.state = SoldierStateIdle
		s.dashOverwatchTimer = s.dashOverwatchDuration(bb.IncomingFireCount > 0)
		if usedCover {
			s.think(fmt.Sprintf("bound complete — holding cover %d ticks", s.dashOverwatchTimer))
		} else {
			s.think(fmt.Sprintf("dash arrived — overwatch %d ticks", s.dashOverwatchTimer))
		}
		s.postArrivalTimer = 0
		return
	}

	// --- Step 5: Move at dash speed ---
	speed := s.profile.EffectiveSpeed(soldierSpeed * dashSpeedMul)
	remaining := speed
	for remaining > 0 && s.pathIndex < len(s.path) {
		wp := s.path[s.pathIndex]
		wdx := wp[0] - s.x
		wdy := wp[1] - s.y
		dist := math.Sqrt(wdx*wdx + wdy*wdy)
		if dist > 1e-6 {
			targetHeading := math.Atan2(wdy, wdx)
			s.vision.UpdateHeading(targetHeading, turnRate*2)
		}
		if dist <= remaining {
			s.x = wp[0]
			s.y = wp[1]
			remaining -= dist
			s.pathIndex++
		} else {
			s.x += (wdx / dist) * remaining
			s.y += (wdy / dist) * remaining
			remaining = 0
		}
	}
	if s.pathIndex >= len(s.path) {
		s.state = SoldierStateIdle
		s.dashOverwatchTimer = s.dashOverwatchDuration(bb.IncomingFireCount > 0)
		if usedCover {
			s.think(fmt.Sprintf("bound to cover — holding %d ticks", s.dashOverwatchTimer))
		} else {
			s.think(fmt.Sprintf("dash complete — overwatch %d ticks", s.dashOverwatchTimer))
		}
	}
}

// executePeek carries out the GoalPeek behaviour:
//  1. Move crouching to the peek point (corner or window edge).
//  2. Stand still, face the interesting direction, wait peekDuration ticks.
//  3. On expiry: if enemy seen → stay (ShatterEvent → overwatch); else decay.
func (s *Soldier) executePeek(dt float64) {
	bb := &s.blackboard

	// Pick a peek target if we don't have one yet.
	if s.peekTarget[0] == 0 && s.peekTarget[1] == 0 {
		tx, ty, ok := s.pickPeekTarget()
		if !ok {
			// Nothing interesting nearby — abort and re-evaluate.
			bb.ShatterEvent = true
			return
		}
		s.peekTarget = [2]float64{tx, ty}
		s.peekTimer = peekDuration
		s.think(fmt.Sprintf("peeking toward (%.0f,%.0f)", tx, ty))
	}

	// Move toward the peek position.
	dx := s.peekTarget[0] - s.x
	dy := s.peekTarget[1] - s.y
	dist := math.Sqrt(dx*dx + dy*dy)
	if dist > 10 {
		// Path to peek point.
		if s.path == nil || s.pathIndex >= len(s.path) {
			s.path = s.navGrid.FindPath(s.x, s.y, s.peekTarget[0], s.peekTarget[1])
			s.pathIndex = 0
		}
		s.state = SoldierStateMoving
		s.moveAlongPath(dt)
		return
	}

	// At peek point — stand still and face it.
	s.state = SoldierStateIdle
	peekAngle := math.Atan2(dy+0.001, dx+0.001) // face toward the peek direction
	// Snap to the angle faster during a peek so it's deliberate.
	s.vision.UpdateHeading(peekAngle, turnRate*3)

	s.peekTimer--
	if s.peekTimer > 0 {
		return
	}

	// Peek complete — evaluate what we saw.
	hadContact := bb.VisibleThreatCount() > 0
	s.peekTarget = [2]float64{}
	s.peekTimer = 0

	if hadContact {
		bb.PeekNoContactCount = 0
		bb.PeekCooldown = peekCooldownHit
		s.think("peek — contact spotted! staying")
		bb.ShatterEvent = true // re-evaluate → GoalOverwatch or GoalEngage
	} else {
		bb.PeekNoContactCount++
		bb.PeekCooldown = peekCooldownEmpty
		s.think(fmt.Sprintf("peek — no contact (count %d) — moving on", bb.PeekNoContactCount))
		bb.ShatterEvent = true // re-evaluate → probably move elsewhere
	}
}

// pickPeekTarget finds the nearest tactically interesting tile (corner or
// window-adjacent) within a short radius and returns its world coordinates.
func (s *Soldier) pickPeekTarget() (float64, float64, bool) {
	if s.tacticalMap == nil {
		return 0, 0, false
	}
	cx, cy := WorldToCell(s.x, s.y)
	const searchRadius = 4

	bestDist := math.MaxFloat64
	bestX, bestY := 0.0, 0.0
	found := false

	for dy := -searchRadius; dy <= searchRadius; dy++ {
		for dx := -searchRadius; dx <= searchRadius; dx++ {
			nx, ny := cx+dx, cy+dy
			if nx < 0 || ny < 0 || nx >= s.tacticalMap.cols || ny >= s.tacticalMap.rows {
				continue
			}
			trait := s.tacticalMap.traits[ny*s.tacticalMap.cols+nx]
			if trait&CellTraitCorner == 0 && trait&CellTraitWindowAdj == 0 {
				continue
			}
			// Only peek at walkable tiles.
			if s.navGrid != nil && s.navGrid.IsBlocked(nx, ny) {
				continue
			}
			wx, wy := CellToWorld(nx, ny)
			tdx := wx - s.x
			tdy := wy - s.y
			d := math.Sqrt(tdx*tdx + tdy*tdy)
			if d < bestDist {
				bestDist = d
				bestX, bestY = wx, wy
				found = true
			}
		}
	}
	return bestX, bestY, found
}
