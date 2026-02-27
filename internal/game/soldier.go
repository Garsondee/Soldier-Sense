package game

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
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
	x, y float64
	team Team

	// Navigation
	path      [][2]float64
	pathIndex int
	// Targets: the soldier bounces between startTarget and endTarget.
	startTarget [2]float64
	endTarget   [2]float64
	goingToEnd  bool
	navGrid     *NavGrid

	// Phase 1: agent model
	state    SoldierState
	profile  SoldierProfile
	vision   VisionState
	isLeader bool
	squad    *Squad

	// Formation
	formationMember bool    // true = follows squad slot, not fixed patrol
	slotIndex       int     // index into formation offsets
	slotTargetX     float64 // current world-space slot target
	slotTargetY     float64
}

// NewSoldier creates a soldier at (x,y) that will path between start and end.
func NewSoldier(x, y float64, team Team, start, end [2]float64, ng *NavGrid) *Soldier {
	// Initial heading: face toward end target.
	initHeading := HeadingTo(x, y, end[0], end[1])

	s := &Soldier{
		x:           x,
		y:           y,
		team:        team,
		startTarget: start,
		endTarget:   end,
		goingToEnd:  true,
		navGrid:     ng,
		state:       SoldierStateMoving,
		vision:      NewVisionState(initHeading),
		profile:     DefaultProfile(),
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
	var target [2]float64
	if s.goingToEnd {
		target = s.endTarget
	} else {
		target = s.startTarget
	}
	s.path = s.navGrid.FindPath(s.x, s.y, target[0], target[1])
	s.pathIndex = 0
}

// Update runs the soldier's per-tick decision loop and movement.
func (s *Soldier) Update() {
	if s.state == SoldierStateDead {
		return
	}

	// --- Tick stat recovery ---
	dt := 1.0 // one tick
	s.profile.Psych.RecoverFear(dt)
	s.profile.Psych.RecoverMorale(dt)

	// --- Decision loop (priority ordered) ---
	switch {
	case s.profile.Psych.EffectiveFear() > 0.8:
		// HIGH FEAR: seek cover / freeze
		s.state = SoldierStateCover
		s.profile.Physical.AccumulateFatigue(0, dt) // resting in cover
		return

	case s.profile.Psych.EffectiveFear() > 0.5:
		// MODERATE FEAR: crouch and slow advance
		if s.profile.Stance != StanceCrouching {
			s.profile.Stance = StanceCrouching
		}
		s.state = SoldierStateMoving

	default:
		// CALM: normal advance
		if s.profile.Stance != StanceStanding {
			s.profile.Stance = StanceStanding
		}
		s.state = SoldierStateMoving
	}

	// --- Movement ---
	s.moveAlongPath(dt)
}

// moveAlongPath advances the soldier along the current A* path,
// using stance-aware speed and updating heading.
func (s *Soldier) moveAlongPath(dt float64) {
	if s.path == nil || s.pathIndex >= len(s.path) {
		if s.formationMember {
			// Formation members idle when they reach their slot; squad will
			// issue a new path next time the slot drifts far enough.
			s.state = SoldierStateIdle
			return
		}
		// Leader / non-formation soldier: flip patrol direction.
		s.goingToEnd = !s.goingToEnd
		s.recomputePath()
		if s.path == nil {
			s.state = SoldierStateIdle
			return
		}
	}

	speed := s.profile.EffectiveSpeed(soldierSpeed)
	exertion := speed / soldierSpeed // ratio of effort
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

	// If we exhausted the path...
	if s.pathIndex >= len(s.path) {
		if s.formationMember {
			s.state = SoldierStateIdle
			return
		}
		s.goingToEnd = !s.goingToEnd
		s.recomputePath()
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

// Draw renders the soldier as a filled circle with a heading indicator and stance ring.
func (s *Soldier) Draw(screen *ebiten.Image) {
	if s.state == SoldierStateDead {
		// Grey cross for dead soldiers.
		grey := color.RGBA{R: 100, G: 100, B: 100, A: 180}
		ebitenutil.DrawLine(screen, s.x-4, s.y-4, s.x+4, s.y+4, grey)
		ebitenutil.DrawLine(screen, s.x+4, s.y-4, s.x-4, s.y+4, grey)
		return
	}

	var c color.RGBA
	switch s.team {
	case TeamRed:
		c = color.RGBA{R: 220, G: 30, B: 30, A: 255}
	case TeamBlue:
		c = color.RGBA{R: 30, G: 80, B: 220, A: 255}
	}

	// Radius shrinks when crouching/prone to show stance.
	radius := float32(soldierRadius) * float32(s.profile.Stance.Profile().ProfileMul)
	if radius < 3 {
		radius = 3
	}
	vector.DrawFilledCircle(screen, float32(s.x), float32(s.y), radius, c, true)

	// Leader marker: white outline.
	if s.isLeader {
		vector.StrokeCircle(screen, float32(s.x), float32(s.y), radius+2, 1.0,
			color.RGBA{R: 255, G: 255, B: 255, A: 200}, true)
	}

	// Heading line.
	hLen := float64(soldierRadius) * 2.0
	hx := s.x + math.Cos(s.vision.Heading)*hLen
	hy := s.y + math.Sin(s.vision.Heading)*hLen
	ebitenutil.DrawLine(screen, s.x, s.y, hx, hy, color.RGBA{R: 255, G: 255, B: 255, A: 160})
}
