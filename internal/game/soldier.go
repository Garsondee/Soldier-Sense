package game

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

const (
	soldierRadius = 6
	soldierSpeed  = 1.5 // pixels per tick
)

// Team distinguishes friendly vs opposing force.
type Team int

const (
	TeamRed  Team = iota // friendly
	TeamBlue             // OpFor
)

// Soldier is a unit that follows a path of waypoints.
type Soldier struct {
	x, y      float64
	team      Team
	path      [][2]float64
	pathIndex int
	// Targets: the soldier bounces between startTarget and endTarget.
	startTarget [2]float64
	endTarget   [2]float64
	goingToEnd  bool
	navGrid     *NavGrid
}

// NewSoldier creates a soldier at (x,y) that will path between start and end.
func NewSoldier(x, y float64, team Team, start, end [2]float64, ng *NavGrid) *Soldier {
	s := &Soldier{
		x:           x,
		y:           y,
		team:        team,
		startTarget: start,
		endTarget:   end,
		goingToEnd:  true,
		navGrid:     ng,
	}
	s.recomputePath()
	return s
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

// Update moves the soldier along its path.
func (s *Soldier) Update() {
	if s.path == nil || s.pathIndex >= len(s.path) {
		// Reached destination (or no path); flip direction.
		s.goingToEnd = !s.goingToEnd
		s.recomputePath()
		if s.path == nil {
			return
		}
	}

	remaining := soldierSpeed
	for remaining > 0 && s.pathIndex < len(s.path) {
		wp := s.path[s.pathIndex]
		dx := wp[0] - s.x
		dy := wp[1] - s.y
		dist := math.Sqrt(dx*dx + dy*dy)

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

	// If we exhausted the path, flip.
	if s.pathIndex >= len(s.path) {
		s.goingToEnd = !s.goingToEnd
		s.recomputePath()
	}
}

// Draw renders the soldier as a filled circle coloured by team.
func (s *Soldier) Draw(screen *ebiten.Image) {
	var c color.RGBA
	switch s.team {
	case TeamRed:
		c = color.RGBA{R: 220, G: 30, B: 30, A: 255}
	case TeamBlue:
		c = color.RGBA{R: 30, G: 80, B: 220, A: 255}
	}
	vector.DrawFilledCircle(screen, float32(s.x), float32(s.y), soldierRadius, c, true)
}
