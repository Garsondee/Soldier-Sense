package game

import (
	"fmt"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// commitPhaseStr returns a human-readable commit phase label for a soldier.
func commitPhaseStr(bb *Blackboard, s *Soldier) string {
	tick := 0
	if s.currentTick != nil {
		tick = *s.currentTick
	}
	switch bb.CommitPhase(tick) {
	case 0:
		return "commit"
	case 1:
		return "sustain"
	default:
		return "review"
	}
}

// drawMovementIntentLines draws faint lines from each soldier to their path
// endpoint or best nearby position. For the selected soldier, the line is
// brighter and includes a small destination marker.
func (g *Game) drawMovementIntentLines(screen *ebiten.Image) {
	all := append(g.soldiers[:len(g.soldiers):len(g.soldiers)], g.opfor...)
	for _, s := range all {
		if s.state == SoldierStateDead {
			continue
		}
		// Determine destination: path endpoint, or best nearby position.
		var destX, destY float64
		hasDest := false

		if s.path != nil && len(s.path) > 0 {
			last := s.path[len(s.path)-1]
			destX, destY = last[0], last[1]
			hasDest = true
		} else if s.blackboard.HasBestNearby {
			destX = s.blackboard.BestNearbyX
			destY = s.blackboard.BestNearbyY
			hasDest = true
		}

		if !hasDest {
			continue
		}

		// Skip very short lines.
		dx := destX - s.x
		dy := destY - s.y
		dist := math.Sqrt(dx*dx + dy*dy)
		if dist < 20 {
			continue
		}

		// Line colour: faint team colour.
		isSelected := g.inspector.selected == s
		var lineCol color.RGBA
		if s.team == TeamRed {
			if isSelected {
				lineCol = color.RGBA{R: 220, G: 80, B: 60, A: 80}
			} else {
				lineCol = color.RGBA{R: 160, G: 50, B: 40, A: 30}
			}
		} else {
			if isSelected {
				lineCol = color.RGBA{R: 60, G: 100, B: 220, A: 80}
			} else {
				lineCol = color.RGBA{R: 40, G: 60, B: 160, A: 30}
			}
		}

		sx := float32(s.x)
		sy := float32(s.y)
		ex := float32(destX)
		ey := float32(destY)

		// Draw dashed line.
		dashLen := float32(8)
		gapLen := float32(6)
		totalLen := float32(dist)
		ndx := float32(dx) / totalLen
		ndy := float32(dy) / totalLen
		drawn := float32(0)
		for drawn < totalLen {
			segEnd := drawn + dashLen
			if segEnd > totalLen {
				segEnd = totalLen
			}
			x1 := sx + ndx*drawn
			y1 := sy + ndy*drawn
			x2 := sx + ndx*segEnd
			y2 := sy + ndy*segEnd
			thickness := float32(0.5)
			if isSelected {
				thickness = 1.0
			}
			vector.StrokeLine(screen, x1, y1, x2, y2, thickness, lineCol, false)
			drawn = segEnd + gapLen
		}

		// Destination marker for selected soldier.
		if isSelected {
			markerCol := color.RGBA{R: 255, G: 240, B: 60, A: 120}
			r := float32(4)
			for a := 0; a < 8; a++ {
				ang0 := float64(a) / 8.0 * 2 * math.Pi
				ang1 := float64(a+1) / 8.0 * 2 * math.Pi
				vector.StrokeLine(screen,
					ex+r*float32(math.Cos(ang0)),
					ey+r*float32(math.Sin(ang0)),
					ex+r*float32(math.Cos(ang1)),
					ey+r*float32(math.Sin(ang1)),
					1.0, markerCol, false)
			}
		}
	}
}

// drawSquadIntentLabels draws a small label near each squad leader showing
// the squad's current intent and claimed building status.
func (g *Game) drawSquadIntentLabels(screen *ebiten.Image) {
	for _, sq := range g.squads {
		if sq.Leader == nil || sq.Leader.state == SoldierStateDead {
			continue
		}
		lx := float32(sq.Leader.x)
		ly := float32(sq.Leader.y)

		// Intent text.
		intentStr := sq.Intent.String()
		if sq.ClaimedBuildingIdx >= 0 {
			intentStr += " [BLDG]"
		}
		alive := len(sq.Alive())
		total := len(sq.Members)
		label := fmt.Sprintf("SQ%d %s (%d/%d)", sq.ID, intentStr, alive, total)

		// Position below the leader.
		textX := int(lx) - len(label)*3
		textY := int(ly) + soldierRadius + 4

		// Background pill.
		const charW = 6
		const padX = 4
		const padY = 2
		bgW := float32(len(label)*charW + padX*2)
		bgH := float32(14 + padY*2)
		bgX := float32(textX - padX)
		bgY := float32(textY - padY)

		var bgCol color.RGBA
		if sq.Team == TeamRed {
			bgCol = color.RGBA{R: 80, G: 20, B: 15, A: 140}
		} else {
			bgCol = color.RGBA{R: 15, G: 25, B: 80, A: 140}
		}
		vector.FillRect(screen, bgX, bgY, bgW, bgH, bgCol, false)
		ebitenutil.DebugPrintAt(screen, label, textX, textY)
	}
}

// drawSelectedSoldierInfo draws detailed information about the selected soldier
// including their current plan, blackboard state, and tactical awareness.
func (g *Game) drawSelectedSoldierInfo(screen *ebiten.Image) {
	sel := g.inspector.selected
	if sel == nil || sel.state == SoldierStateDead {
		return
	}
	bb := &sel.blackboard

	// Draw info panel below the inspector (or on screen near the soldier).
	// Position: world-space, offset from soldier.
	sx := float32(sel.x) + float32(soldierRadius) + 12
	sy := float32(sel.y) - 40

	lines := []string{
		fmt.Sprintf("Goal: %s", bb.CurrentGoal),
		fmt.Sprintf("Phase: %s", commitPhaseStr(bb, sel)),
		fmt.Sprintf("Fear: %.0f%%  Morale: %.0f%%", sel.profile.Psych.EffectiveFear()*100, sel.profile.Psych.Morale*100),
		fmt.Sprintf("Threats: %d vis  Suppress: %.0f%%", bb.VisibleThreatCount(), bb.SuppressLevel*100),
		fmt.Sprintf("Squad: %s", bb.SquadIntent),
	}
	if bb.HasBestNearby {
		lines = append(lines, fmt.Sprintf("Best pos: (%.0f,%.0f) score:%.2f", bb.BestNearbyX, bb.BestNearbyY, bb.BestNearbyScore))
	}
	if bb.ClaimedBuildingIdx >= 0 {
		lines = append(lines, fmt.Sprintf("Claimed bldg #%d", bb.ClaimedBuildingIdx))
	}
	if bb.AtWindowAdj {
		lines = append(lines, "At window overwatch")
	} else if bb.AtInterior {
		lines = append(lines, "Inside building")
	}
	if bb.ShatterPressure > 0.1 {
		lines = append(lines, fmt.Sprintf("Shatter: %.0f%%/%.0f%%", bb.ShatterPressure*100, bb.ShatterThreshold*100))
	}

	// Background.
	const lineH = 14
	const padX = 6
	const padY = 4
	maxLen := 0
	for _, l := range lines {
		if len(l) > maxLen {
			maxLen = len(l)
		}
	}
	bgW := float32(maxLen*6 + padX*2)
	bgH := float32(len(lines)*lineH + padY*2)
	bgCol := color.RGBA{R: 15, G: 18, B: 15, A: 200}
	vector.FillRect(screen, sx, sy, bgW, bgH, bgCol, false)

	// Accent border.
	var accent color.RGBA
	if sel.team == TeamRed {
		accent = color.RGBA{R: 200, G: 60, B: 40, A: 160}
	} else {
		accent = color.RGBA{R: 40, G: 80, B: 200, A: 160}
	}
	vector.StrokeRect(screen, sx, sy, bgW, bgH, 1.0, accent, false)

	// Text.
	for i, l := range lines {
		ebitenutil.DebugPrintAt(screen, l, int(sx)+padX, int(sy)+padY+i*lineH)
	}
}
