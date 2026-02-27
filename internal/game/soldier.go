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
	turnRate        = 0.12 // radians per tick
	coverSearchDist = 80.0 // pixels to search for cover
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
}

// NewSoldier creates a soldier at (x,y) that will advance toward end.
func NewSoldier(id int, x, y float64, team Team, start, end [2]float64, ng *NavGrid, tl *ThoughtLog, tick *int) *Soldier {
	// Initial heading: face toward end target.
	initHeading := HeadingTo(x, y, end[0], end[1])

	prefix := "R"
	if team == TeamBlue {
		prefix = "B"
	}

	s := &Soldier{
		id:          id,
		label:       prefix + string(rune('0'+id%10)),
		x:           x,
		y:           y,
		team:        team,
		startTarget: start,
		endTarget:   end,
		navGrid:     ng,
		state:       SoldierStateMoving,
		health:      soldierMaxHP,
		vision:      NewVisionState(initHeading),
		profile:     DefaultProfile(),
		thoughtLog:  tl,
		currentTick: tick,
		prevGoal:    GoalAdvance,
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

// Update runs the soldier's per-tick cognition loop: believe → think → act.
func (s *Soldier) Update() {
	if s.state == SoldierStateDead {
		return
	}

	dt := 1.0
	s.profile.Psych.RecoverFear(dt)
	s.profile.Psych.RecoverMorale(dt)

	// --- Step 2: BELIEVE — update blackboard from vision ---
	tick := 0
	if s.currentTick != nil {
		tick = *s.currentTick
	}
	s.blackboard.UpdateThreats(s.vision.KnownContacts, tick)

	// --- Step 4: INDIVIDUAL THINK — goal selection ---
	goal := SelectGoal(&s.blackboard, &s.profile, s.isLeader, s.path != nil)

	// Log goal changes.
	if goal != s.prevGoal {
		s.think(fmt.Sprintf("goal: %s → %s", s.prevGoal, goal))
		s.prevGoal = goal
	}
	s.blackboard.CurrentGoal = goal

	// --- Derive task from goal ---
	switch goal {
	case GoalSurvive:
		// High fear: freeze / take cover.
		if s.profile.Stance != StanceCrouching {
			s.profile.Stance = StanceCrouching
			s.think("crouching — taking cover")
		}
		s.state = SoldierStateCover
		s.profile.Physical.AccumulateFatigue(0, dt)
		return

	case GoalEngage:
		// Visible enemy: crouch, face threat, shoot (combat system handles firing).
		if s.profile.Stance != StanceCrouching {
			s.profile.Stance = StanceCrouching
		}
		s.state = SoldierStateIdle
		s.profile.Physical.AccumulateFatigue(0, dt)
		s.faceNearestThreat()
		return

	case GoalMoveToContact:
		// No LOS but squad knows where the enemy is — close in.
		// Use the per-member spread position if the leader has issued one.
		if s.profile.Stance != StanceCrouching {
			s.profile.Stance = StanceCrouching
		}
		s.state = SoldierStateMoving
		s.moveToContact(dt)
		return

	case GoalFallback:
		// Retreat away from contact under fire.
		if s.profile.Stance != StanceCrouching {
			s.profile.Stance = StanceCrouching
		}
		s.state = SoldierStateMoving
		s.moveFallback(dt)
		return

	case GoalRegroup:
		// Regroup: tighten cohesion around leader (implemented as moving to formation slot).
		// Under contact, default to crouching while moving.
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
		return

	case GoalHoldPosition:
		// Hold: stop moving, crouch, scan.
		if s.state != SoldierStateIdle {
			s.think("holding position")
		}
		if s.profile.Stance != StanceCrouching {
			s.profile.Stance = StanceCrouching
		}
		s.state = SoldierStateIdle
		s.profile.Physical.AccumulateFatigue(0, dt)
		// Face toward nearest threat if any.
		if s.blackboard.VisibleThreatCount() > 0 {
			s.faceNearestThreat()
		}
		return

	case GoalMaintainFormation:
		// Follow formation slot (path set by squad.UpdateFormation).
		if s.profile.Stance != StanceStanding {
			s.profile.Stance = StanceStanding
		}
		s.state = SoldierStateMoving
		s.moveAlongPath(dt)
		return

	case GoalAdvance:
		// Push toward objective.
		if s.profile.Stance != StanceStanding {
			s.profile.Stance = StanceStanding
		}
		s.state = SoldierStateMoving
		s.moveAlongPath(dt)
		return
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

// moveToContact paths the soldier toward their assigned spread position (or the
// squad contact if no individual order has been issued), within the leash limit.
func (s *Soldier) moveToContact(dt float64) {
	bb := &s.blackboard
	if !bb.SquadHasContact {
		s.state = SoldierStateIdle
		return
	}

	// Prefer the per-member assigned position; fall back to raw contact.
	var targetX, targetY float64
	if bb.HasMoveOrder {
		targetX = bb.OrderMoveX
		targetY = bb.OrderMoveY
	} else {
		targetX = bb.SquadContactX
		targetY = bb.SquadContactY
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

// moveFallback paths the soldier directly away from the squad contact position.
// It picks a point behind the soldier relative to the contact, at a fixed retreat
// distance, then A*-paths there.
func (s *Soldier) moveFallback(dt float64) {
	bb := &s.blackboard
	if !bb.SquadHasContact {
		s.state = SoldierStateIdle
		return
	}

	const retreatDist = 120.0

	// Direction away from contact.
	dx := s.x - bb.SquadContactX
	dy := s.y - bb.SquadContactY
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

// moveAlongPath advances the soldier along the current A* path,
// using stance-aware speed and updating heading.
func (s *Soldier) moveAlongPath(dt float64) {
	if s.path == nil || s.pathIndex >= len(s.path) {
		// One-way advance: idle at objective.
		s.state = SoldierStateIdle
		return
	}

	speed := s.profile.EffectiveSpeed(soldierSpeed)
	// Leader cohesion: slow down when squad is spread out.
	if s.isLeader && s.squad != nil {
		speed *= s.squad.LeaderCohesionSlowdown()
	}
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
	}
}

// UpdateVision performs vision scan against enemies.
func (s *Soldier) UpdateVision(enemies []*Soldier, buildings []rect) {
	if s.state == SoldierStateDead {
		return
	}
	s.vision.PerformVisionScan(s.x, s.y, enemies, buildings)

	// Seeing enemies increases fear slightly.
	if len(s.vision.KnownContacts) > 0 {
		stress := 0.02 * float64(len(s.vision.KnownContacts))
		s.profile.Psych.ApplyStress(stress)
	}
}

// Draw renders the soldier as a filled circle with a heading indicator, health bar, and state tint.
func (s *Soldier) Draw(screen *ebiten.Image, offX, offY int) {
	ox, oy := float32(offX), float32(offY)
	sx, sy := ox+float32(s.x), oy+float32(s.y)

	if s.state == SoldierStateDead {
		// Faded team-coloured dot with grey X overlay.
		var dc color.RGBA
		if s.team == TeamRed {
			dc = color.RGBA{R: 90, G: 30, B: 30, A: 140}
		} else {
			dc = color.RGBA{R: 20, G: 40, B: 90, A: 140}
		}
		vector.FillCircle(screen, sx, sy, float32(soldierRadius)-1, dc, false)
		grey := color.RGBA{R: 180, G: 180, B: 180, A: 200}
		d := float32(4)
		vector.StrokeLine(screen, sx-d, sy-d, sx+d, sy+d, 1.5, grey, false)
		vector.StrokeLine(screen, sx+d, sy-d, sx-d, sy+d, 1.5, grey, false)
		return
	}

	// Base team colour.
	var base color.RGBA
	switch s.team {
	case TeamRed:
		base = color.RGBA{R: 220, G: 30, B: 30, A: 255}
	case TeamBlue:
		base = color.RGBA{R: 30, G: 80, B: 220, A: 255}
	}

	// State tint: darken when in cover/suppressed, lighten when engaging.
	c := base
	switch s.state {
	case SoldierStateCover:
		c = color.RGBA{R: base.R / 2, G: base.G / 2, B: base.B / 2, A: 255}
	case SoldierStateIdle:
		// engaging / holding — keep base but slightly desaturate toward white
		r := uint8(min8(255, int(base.R)+30))
		g2 := uint8(min8(255, int(base.G)+30))
		b2 := uint8(min8(255, int(base.B)+30))
		c = color.RGBA{R: r, G: g2, B: b2, A: 255}
	}

	// Radius shrinks when crouching/prone to show stance.
	radius := float32(soldierRadius) * float32(s.profile.Stance.Profile().ProfileMul)
	if radius < 3 {
		radius = 3
	}

	// Dark outline for contrast against terrain.
	vector.FillCircle(screen, sx, sy, radius+1.5, color.RGBA{R: 0, G: 0, B: 0, A: 180}, false)
	vector.FillCircle(screen, sx, sy, radius, c, false)

	// Leader marker: bright white outline ring.
	if s.isLeader {
		vector.StrokeCircle(screen, sx, sy, radius+2.5, 1.5,
			color.RGBA{R: 255, G: 240, B: 100, A: 220}, true)
	}

	// Heading line (thicker, slightly team-tinted).
	hLen := float64(soldierRadius) * 2.2
	hx := ox + float32(s.x+math.Cos(s.vision.Heading)*hLen)
	hy := oy + float32(s.y+math.Sin(s.vision.Heading)*hLen)
	vector.StrokeLine(screen, sx, sy, hx, hy, 2.0, color.RGBA{R: 255, G: 255, B: 255, A: 200}, false)

	// Health bar (drawn just below the soldier circle).
	if s.health < soldierMaxHP {
		barW := float32(soldierRadius) * 2.2
		barH := float32(2.5)
		bx := sx - barW/2
		by := sy + radius + 3
		// Background.
		vector.FillRect(screen, bx, by, barW, barH, color.RGBA{R: 30, G: 30, B: 30, A: 200}, false)
		// Fill proportional to health.
		filled := barW * float32(s.health/soldierMaxHP)
		hpR := uint8(255 - uint8(s.health/soldierMaxHP*200))
		hpG := uint8(s.health / soldierMaxHP * 220)
		vector.FillRect(screen, bx, by, filled, barH, color.RGBA{R: hpR, G: hpG, B: 20, A: 220}, false)
	}
}

func min8(a, b int) int {
	if a < b {
		return a
	}
	return b
}
