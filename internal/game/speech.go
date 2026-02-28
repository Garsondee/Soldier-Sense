package game

import (
	"fmt"
	"image/color"
	"math"
	"math/rand"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// speechLifetime is how many ticks a speech bubble stays visible (~3 seconds).
const speechLifetime = 180

// speechCooldown is the minimum ticks between speeches per soldier (~8 seconds).
const speechCooldown = 480

// SpeechBubble holds an active speech event above a soldier.
type SpeechBubble struct {
	soldier *Soldier
	text    string
	detail  string // second line of context (goal, threat info)
	age     int
	yOff    float32 // vertical offset to prevent overlaps
}

// contextualPhrase generates a speech line that reflects the soldier's actual
// state: their goal, threat situation, suppression, and tactical awareness.
func contextualPhrase(rng *rand.Rand, s *Soldier) (string, string) {
	bb := &s.blackboard
	ef := s.profile.Psych.EffectiveFear()
	incoming := bb.IncomingFireCount
	threats := bb.VisibleThreatCount()
	goal := bb.CurrentGoal
	suppressed := bb.IsSuppressed()

	// Panic: high fear overrides everything.
	if ef >= 0.75 {
		texts := []string{"GET DOWN!", "PULL BACK!", "BREAKING CONTACT!", "WE'RE PINNED!"}
		detail := fmt.Sprintf("fear:%.0f%% threats:%d", ef*100, threats)
		return texts[rng.Intn(len(texts))], detail
	}

	// Suppressed: under heavy fire.
	if suppressed || incoming > 3 {
		texts := []string{"Taking fire!", "Rounds incoming!", "Cover!", "They're on us!", "Suppressing!"}
		detail := fmt.Sprintf("incoming:%d suppress:%.0f%%", incoming, bb.SuppressLevel*100)
		return texts[rng.Intn(len(texts))], detail
	}

	// Goal-specific contextual speech.
	switch goal {
	case GoalEngage:
		dist := bb.ClosestVisibleThreatDist(s.x, s.y)
		if dist < math.MaxFloat64 {
			texts := []string{"Engaging!", "Contact — firing!", "Targets front!", "Eyes on — engaging"}
			return texts[rng.Intn(len(texts))], fmt.Sprintf("%dm %d tgt", int(dist/16), threats)
		}
		return "Contact!", fmt.Sprintf("%d threats", threats)

	case GoalMoveToContact:
		return "Moving to contact", fmt.Sprintf("closing %dm", int(bb.ClosestVisibleThreatDist(s.x, s.y)/16))

	case GoalFlank:
		sides := "left"
		if bb.FlankSide < 0 {
			sides = "right"
		}
		return "Flanking " + sides, fmt.Sprintf("bearing %.0f°", bb.SquadEnemyBearing*180/math.Pi)

	case GoalFallback:
		return "Falling back!", fmt.Sprintf("fear:%.0f%%", ef*100)

	case GoalSurvive:
		return "Can't hold!", fmt.Sprintf("fear:%.0f%% morale:%.0f%%", ef*100, s.profile.Psych.Morale*100)

	case GoalOverwatch:
		pos := "open"
		if bb.AtWindowAdj {
			pos = "window"
		} else if bb.AtCorner {
			pos = "corner"
		} else if bb.AtWall {
			pos = "wall"
		}
		return "Overwatching", pos

	case GoalHoldPosition:
		return "Holding position", fmt.Sprintf("desirability:%.1f", bb.PositionDesirability)

	case GoalAdvance:
		texts := []string{"Moving up", "Advancing", "Push forward", "On me"}
		return texts[rng.Intn(len(texts))], ""

	case GoalMaintainFormation:
		return "In formation", ""

	case GoalRegroup:
		return "Regrouping", ""
	}

	// Fallback: calm state.
	if ef < 0.1 && threats == 0 {
		texts := []string{"Clear.", "All quiet.", "Set.", "Watching."}
		return texts[rng.Intn(len(texts))], ""
	}
	return "Standing by", fmt.Sprintf("fear:%.0f%%", ef*100)
}

// UpdateSpeech ticks all speech bubbles and occasionally generates new ones
// for soldiers whose mood warrants commentary.
func (g *Game) UpdateSpeech(rng *rand.Rand) {
	// Age existing bubbles, prune expired.
	kept := g.speechBubbles[:0]
	for _, b := range g.speechBubbles {
		b.age++
		if b.age < speechLifetime {
			kept = append(kept, b)
		}
	}
	g.speechBubbles = kept

	// Try to emit a new bubble from a random eligible soldier each tick.
	all := append(g.soldiers[:len(g.soldiers):len(g.soldiers)], g.opfor...)
	if len(all) == 0 {
		return
	}
	s := all[rng.Intn(len(all))]
	if s.state == SoldierStateDead {
		return
	}
	if g.tick-s.lastSpeechTick < speechCooldown {
		return
	}

	ef := s.profile.Psych.EffectiveFear()
	incoming := s.blackboard.IncomingFireCount

	// Emission probability varies with urgency.
	switch {
	case ef >= 0.75 || incoming > 3:
		// Panic/suppressed: always speak.
	case ef >= 0.4 || incoming > 0:
		if rng.Float64() > 0.40 {
			return
		}
	case s.blackboard.VisibleThreatCount() > 0:
		if rng.Float64() > 0.30 {
			return
		}
	default:
		if rng.Float64() > 0.08 {
			return
		}
	}

	text, detail := contextualPhrase(rng, s)

	// Overlap prevention: count bubbles already on this soldier and offset.
	var yOff float32
	for _, existing := range g.speechBubbles {
		if existing.soldier == s {
			yOff -= 22
		}
	}

	s.lastSpeechTick = g.tick
	g.speechBubbles = append(g.speechBubbles, &SpeechBubble{
		soldier: s,
		text:    text,
		detail:  detail,
		yOff:    yOff,
	})

	// Log to thought log.
	label := "RED"
	if s.team == TeamBlue {
		label = "BLU"
	}
	logText := fmt.Sprintf("[%d] %s", s.id, text)
	if detail != "" {
		logText += " (" + detail + ")"
	}
	g.thoughtLog.AddSpeech(g.tick, label, s.team, logText)
}

// drawSpeechBubbles renders active speech bubbles above each soldier.
func (g *Game) drawSpeechBubbles(screen *ebiten.Image, offX, offY int) {
	ox, oy := float32(offX), float32(offY)

	// Overlap prevention: track occupied Y bands per soldier to push bubbles up.
	type bubblePos struct{ x, y float32 }
	occupied := make(map[int]float32) // soldier ID → lowest Y used

	for _, b := range g.speechBubbles {
		s := b.soldier
		if s.state == SoldierStateDead {
			continue
		}
		progress := float64(b.age) / float64(speechLifetime)
		alpha := float32(1.0)
		if progress > 0.70 {
			alpha = float32(1.0 - (progress-0.70)/0.30)
		}
		if alpha < 0.05 {
			continue
		}

		const charW = 6
		const lineH = 14
		const padX = 5
		const padY = 3

		// Measure text dimensions.
		lines := 1
		maxLen := len(b.text)
		if b.detail != "" {
			lines = 2
			if len(b.detail) > maxLen {
				maxLen = len(b.detail)
			}
		}
		textW := float32(maxLen * charW)
		bgW := textW + float32(padX*2)
		bgH := float32(lines*lineH + padY*2)

		// Base position above soldier.
		sx := ox + float32(s.x)
		baseY := oy + float32(s.y) - float32(soldierRadius) - bgH - 4

		// Push up if there's already a bubble on this soldier.
		if prevY, ok := occupied[s.id]; ok {
			if baseY+bgH > prevY {
				baseY = prevY - bgH - 2
			}
		}
		baseY += b.yOff
		occupied[s.id] = baseY

		bgX := sx - bgW/2
		bgY := baseY

		// Background: darker, more readable, with mood accent.
		baseBG := color.RGBA{R: 20, G: 22, B: 20, A: uint8(210 * alpha)}
		vector.FillRect(screen, bgX, bgY, bgW, bgH, baseBG, false)

		// Accent stripe on the left edge — team coloured.
		var accent color.RGBA
		if s.team == TeamRed {
			accent = color.RGBA{R: 220, G: 55, B: 40, A: uint8(220 * alpha)}
		} else {
			accent = color.RGBA{R: 40, G: 80, B: 220, A: uint8(220 * alpha)}
		}
		vector.FillRect(screen, bgX, bgY, 3, bgH, accent, false)

		// Subtle border.
		vector.StrokeRect(screen, bgX, bgY, bgW, bgH, 0.5,
			color.RGBA{R: 100, G: 100, B: 100, A: uint8(80 * alpha)}, false)

		// Main text.
		textX := int(bgX + float32(padX) + 3)
		textY := int(bgY + float32(padY))
		ebitenutil.DebugPrintAt(screen, b.text, textX, textY)

		// Detail line (smaller, dimmer).
		if b.detail != "" {
			ebitenutil.DebugPrintAt(screen, b.detail, textX, textY+lineH)
		}

		// Connector line from bubble to soldier.
		vector.StrokeLine(screen, sx, bgY+bgH, sx, oy+float32(s.y)-float32(soldierRadius),
			0.5, color.RGBA{R: 100, G: 100, B: 100, A: uint8(60 * alpha)}, false)
	}
}
