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

func pickSpeechLine(rng *rand.Rand, lines ...string) string {
	if len(lines) == 0 {
		return ""
	}
	return lines[rng.Intn(len(lines))]
}

func speechPressureScore(s *Soldier) float64 {
	bb := &s.blackboard
	ef := clamp01(s.profile.Psych.EffectiveFear())
	pressure := ef*0.42 +
		clamp01(float64(bb.IncomingFireCount)/4.0)*0.20 +
		clamp01(bb.SuppressLevel)*0.20 +
		clamp01(1.0-s.profile.Psych.Morale)*0.16 +
		clamp01(bb.SquadStress)*0.12 +
		clamp01(bb.SquadCasualtyRate)*0.12
	if bb.PanicRetreatActive || bb.Surrendered {
		pressure += 0.25
	}
	if bb.VisibleThreatCount() > 0 {
		pressure += 0.10
	}
	if s.health < soldierMaxHP*0.45 {
		pressure += 0.10
	}
	if bb.OfficerOrderActive && !bb.OfficerOrderImmediate {
		pressure += 0.06
	}
	return clamp01(pressure)
}

// contextualPhrase generates a speech line that reflects the soldier's actual
// state: their goal, threat situation, suppression, and tactical awareness.
func contextualPhrase(rng *rand.Rand, s *Soldier) (string, string) {
	bb := &s.blackboard
	ef := s.profile.Psych.EffectiveFear()
	morale := s.profile.Psych.Morale
	fatigue := s.profile.Physical.Fatigue
	incoming := bb.IncomingFireCount
	threats := bb.VisibleThreatCount()
	goal := bb.CurrentGoal
	suppressed := bb.IsSuppressed()
	hpPct := clamp01(s.health / soldierMaxHP)
	closestThreatDist := bb.ClosestVisibleThreatDist(s.x, s.y)

	contactX, contactY := s.endTarget[0], s.endTarget[1]
	contactSource := "objective"
	if bb.VisibleThreatCount() > 0 {
		best := math.MaxFloat64
		for _, t := range bb.Threats {
			if !t.IsVisible {
				continue
			}
			d := math.Hypot(t.X-s.x, t.Y-s.y)
			if d < best {
				best = d
				contactX, contactY = t.X, t.Y
				contactSource = "visual"
			}
		}
	} else if bb.RadioHasContact {
		contactX, contactY = bb.RadioContactX, bb.RadioContactY
		contactSource = "radio"
	} else if bb.SquadHasContact {
		contactX, contactY = bb.SquadContactX, bb.SquadContactY
		contactSource = "squad"
	} else if bb.HeardGunfire {
		contactX, contactY = bb.HeardGunfireX, bb.HeardGunfireY
		contactSource = "audio"
	} else if bb.IsActivated() {
		contactX, contactY = bb.CombatMemoryX, bb.CombatMemoryY
		contactSource = "memory"
	}
	contactDist := math.Hypot(contactX-s.x, contactY-s.y)

	if bb.Surrendered {
		return pickSpeechLine(rng, "I surrender!", "Don't shoot!", "I'm done!", "I can't fight!"),
			fmt.Sprintf("fear:%.0f%% morale:%.0f%%", ef*100, morale*100)
	}
	if bb.PanicRetreatActive {
		fallback := "scatter"
		if bb.RetreatToOwnLines {
			fallback = "own-lines"
		}
		return pickSpeechLine(rng, "Running!", "I'm breaking!", "Fall back now!", "I can't stay here!"),
			fmt.Sprintf("panic %s fear:%.0f%%", fallback, ef*100)
	}

	if bb.DisobeyingOrders && bb.OfficerOrderActive && !bb.OfficerOrderImmediate {
		return pickSpeechLine(rng, "Can't do that!", "Negative on that order", "No — that's suicide", "Hold your order!"),
			fmt.Sprintf("obedience:%.0f%% pressure:%.0f%%", bb.OfficerOrderObedienceChance*100, speechPressureScore(s)*100)
	}

	if hpPct < 0.32 {
		return pickSpeechLine(rng, "I'm hit bad!", "Need aid!", "Bleeding out!", "I can't keep this up!"),
			fmt.Sprintf("hp:%.0f%% fear:%.0f%%", hpPct*100, ef*100)
	}

	if s.magRounds <= max(3, s.magCapacity/6) && (threats > 0 || incoming > 0) {
		return pickSpeechLine(rng, "Low ammo!", "Almost dry!", "Need a reload window!", "Magazine nearly empty!"),
			fmt.Sprintf("ammo:%d/%d", s.magRounds, s.magCapacity)
	}

	// Panic: high fear overrides everything.
	if ef >= 0.75 {
		detail := fmt.Sprintf("fear:%.0f%% threats:%d hp:%.0f%%", ef*100, threats, hpPct*100)
		return pickSpeechLine(rng, "GET DOWN!", "PULL BACK!", "BREAKING CONTACT!", "WE'RE PINNED!"), detail
	}

	// Suppressed: under heavy fire.
	if suppressed || incoming > 3 {
		stance := "up"
		switch s.profile.Stance {
		case StanceCrouching:
			stance = "crouched"
		case StanceProne:
			stance = "prone"
		}
		rangeBand := "unknown"
		if closestThreatDist < math.MaxFloat64 {
			switch {
			case closestThreatDist <= 8*cellSize:
				rangeBand = "close"
			case closestThreatDist <= 18*cellSize:
				rangeBand = "mid"
			default:
				rangeBand = "far"
			}
		}
		detail := fmt.Sprintf("in:%d sup:%.0f%% %s %s", incoming, bb.SuppressLevel*100, stance, rangeBand)
		return pickSpeechLine(rng, "Taking fire!", "Rounds incoming!", "Cover!", "They're on us!", "Pinned down!"), detail
	}

	if bb.OfficerOrderActive {
		if bb.OfficerOrderImmediate {
			return pickSpeechLine(rng, "Copy, moving!", "Order received!", "Executing!", "On your command!"),
				fmt.Sprintf("%s pri:%.2f", bb.OfficerOrderKind.String(), bb.OfficerOrderPriority)
		}
		if bb.OfficerOrderObedienceChance < 0.45 {
			return pickSpeechLine(rng, "Unsure on that!", "Can't commit yet", "Need better ground first", "Not ready!"),
				fmt.Sprintf("%s obey:%.0f%%", bb.OfficerOrderKind.String(), bb.OfficerOrderObedienceChance*100)
		}
	}

	// Goal-specific contextual speech.
	switch goal {
	case GoalEngage:
		dist := closestThreatDist
		if dist < math.MaxFloat64 {
			if bb.Internal.ShotMomentum > 0.25 {
				return pickSpeechLine(rng, "Got them dialed!", "Shots are landing!", "Keep pressure!", "They're cracking!"),
					fmt.Sprintf("%dm x%d momentum:+%.2f", int(dist/16), threats, bb.Internal.ShotMomentum)
			}
			if bb.Internal.ShotMomentum < -0.20 {
				return pickSpeechLine(rng, "Missing — adjust!", "Need closer shots!", "Can't hit from here!", "Shift fire!"),
					fmt.Sprintf("%dm x%d momentum:%.2f", int(dist/16), threats, bb.Internal.ShotMomentum)
			}
			return pickSpeechLine(rng, "Engaging!", "Contact — firing!", "Targets front!", "Eyes on — engaging"),
				fmt.Sprintf("%dm %d tgt", int(dist/16), threats)
		}
		return "Contact!", fmt.Sprintf("%d threats", threats)

	case GoalMoveToContact:
		if contactSource == "radio" {
			return pickSpeechLine(rng, "Moving on your mark", "Following radio contact", "Advancing to reported contact", "Pushing to callout"),
				fmt.Sprintf("%s %dm", contactSource, int(contactDist/16))
		}
		if contactSource == "audio" {
			return pickSpeechLine(rng, "Heard shots ahead", "Moving to gunfire", "Closing on that noise", "Shifting toward fire"),
				fmt.Sprintf("%s %dm", contactSource, int(contactDist/16))
		}
		return "Moving to contact", fmt.Sprintf("%s %dm", contactSource, int(contactDist/16))

	case GoalFlank:
		sides := "left"
		if bb.FlankSide < 0 {
			sides = "right"
		}
		if bb.BoundMover {
			return "Flanking " + sides, fmt.Sprintf("bound mover %.0f°", bb.SquadEnemyBearing*180/math.Pi)
		}
		return "Holding flank " + sides, fmt.Sprintf("covering %.0f°", bb.SquadEnemyBearing*180/math.Pi)

	case GoalFallback:
		return pickSpeechLine(rng, "Falling back!", "Peeling off!", "Breaking line!", "Disengaging!"),
			fmt.Sprintf("fear:%.0f%% hp:%.0f%%", ef*100, hpPct*100)

	case GoalSurvive:
		return pickSpeechLine(rng, "Can't hold!", "Need cover now!", "I'm exposed!", "Survive first!"),
			fmt.Sprintf("fear:%.0f%% morale:%.0f%%", ef*100, s.profile.Psych.Morale*100)

	case GoalOverwatch:
		pos := "open"
		if bb.AtWindowAdj {
			pos = "window"
		} else if bb.AtCorner {
			pos = "corner"
		} else if bb.AtWall {
			pos = "wall"
		}
		if bb.UnresponsiveMembers > 0 {
			return "Overwatching", fmt.Sprintf("%s comm-gap:%d", pos, bb.UnresponsiveMembers)
		}
		return "Overwatching", fmt.Sprintf("%s sight:%.2f", pos, bb.LocalSightlineScore)

	case GoalHoldPosition:
		if bb.CloseAllyPressure > 0.45 {
			return "Hold and spread", fmt.Sprintf("des:%.1f crowd:%.0f%%", bb.PositionDesirability, bb.CloseAllyPressure*100)
		}
		return "Holding position", fmt.Sprintf("desirability:%.1f", bb.PositionDesirability)

	case GoalAdvance:
		if fatigue > 0.60 {
			return pickSpeechLine(rng, "Advancing slow", "Still moving", "Pushing through", "Step by step"),
				fmt.Sprintf("fatigue:%.0f%%", fatigue*100)
		}
		return pickSpeechLine(rng, "Moving up", "Advancing", "Push forward", "On me"), ""

	case GoalMaintainFormation:
		if bb.VisibleAllyCount == 0 {
			return "Lost visual on squad", "relinking formation"
		}
		if bb.CloseAllyPressure > 0.55 {
			return "Too tight — spread", fmt.Sprintf("crowd:%.0f%%", bb.CloseAllyPressure*100)
		}
		return "In formation", fmt.Sprintf("allies:%d", bb.VisibleAllyCount)

	case GoalRegroup:
		return pickSpeechLine(rng, "Regrouping", "Rally on me", "Reforming line", "Tighten up"),
			fmt.Sprintf("cohesion:%.0f%%", bb.SquadCohesion*100)

	case GoalPeek:
		if threats > 0 {
			return pickSpeechLine(rng, "Peeking contact", "I see movement", "Contact on peek", "Eyes around corner"),
				fmt.Sprintf("peek threats:%d", threats)
		}
		return pickSpeechLine(rng, "Quick peek", "Checking angle", "Leaning out", "Corner check"),
			fmt.Sprintf("cooldown:%d", bb.PeekCooldown)
	}

	// Fallback: calm state.
	if ef < 0.1 && threats == 0 {
		if bb.UnresponsiveMembers > 0 {
			return pickSpeechLine(rng, "Net is thin", "Comms check", "Missing replies", "Radio's quiet"),
				fmt.Sprintf("no-reply:%d", bb.UnresponsiveMembers)
		}
		return pickSpeechLine(rng, "Clear.", "All quiet.", "Set.", "Watching."), ""
	}
	if contactSource != "objective" {
		return pickSpeechLine(rng, "Tracking contact", "Staying sharp", "Watching that sector", "Holding for movement"),
			fmt.Sprintf("%s %dm", contactSource, int(contactDist/16))
	}
	return "Standing by", fmt.Sprintf("fear:%.0f%% morale:%.0f%%", ef*100, morale*100)
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
		pressure := speechPressureScore(s)
		dynamicCooldown := speechCooldown - int(pressure*340)
		if dynamicCooldown < 90 {
			dynamicCooldown = 90
		}
		if g.tick-s.lastSpeechTick < dynamicCooldown {
			return
		}
	}

	ef := s.profile.Psych.EffectiveFear()
	incoming := s.blackboard.IncomingFireCount
	pressure := speechPressureScore(s)

	speakChance := 0.06 + pressure*0.58
	switch s.blackboard.CurrentGoal {
	case GoalEngage, GoalFallback, GoalSurvive, GoalFlank, GoalPeek:
		speakChance += 0.10
	case GoalHoldPosition, GoalOverwatch:
		speakChance += 0.04
	}
	if s.blackboard.OfficerOrderActive {
		speakChance += 0.07
	}
	if s.blackboard.PanicRetreatActive || s.blackboard.Surrendered {
		speakChance += 0.20
	}
	if s.health < soldierMaxHP*0.45 {
		speakChance += 0.08
	}
	speakChance = clamp01(speakChance)

	// Emission probability varies with urgency.
	switch {
	case ef >= 0.75 || incoming > 3 || s.blackboard.PanicRetreatActive || s.blackboard.Surrendered:
		// Panic/suppressed: always speak.
	case ef >= 0.4 || incoming > 0 || s.blackboard.VisibleThreatCount() > 0:
		if rng.Float64() > speakChance {
			return
		}
	default:
		if rng.Float64() > speakChance*0.6 {
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
		const padX = 6
		const padY = 4

		// Inverse zoom scale: make bubbles larger when zoomed out so they stay readable.
		invZoom := float32(1.0 / g.camZoom)
		if invZoom < 1.0 {
			invZoom = 1.0
		}
		if invZoom > 3.0 {
			invZoom = 3.0
		}

		// Measure text dimensions (in unscaled space, then multiply by invZoom).
		lines := 1
		maxLen := len(b.text)
		if b.detail != "" {
			lines = 2
			if len(b.detail) > maxLen {
				maxLen = len(b.detail)
			}
		}
		textW := float32(maxLen*charW) * invZoom
		bgW := textW + float32(padX*2)*invZoom
		bgH := float32(lines*lineH+padY*2) * invZoom

		// Base position above soldier.
		sx := ox + float32(s.x)
		baseY := oy + float32(s.y) - float32(soldierRadius)*invZoom - bgH - 6*invZoom

		// Push up if there's already a bubble on this soldier.
		if prevY, ok := occupied[s.id]; ok {
			if baseY+bgH > prevY {
				baseY = prevY - bgH - 3*invZoom
			}
		}
		baseY += b.yOff * invZoom
		occupied[s.id] = baseY

		bgX := sx - bgW/2
		bgY := baseY

		// Background: darker, more readable, with mood accent.
		baseBG := color.RGBA{R: 16, G: 18, B: 16, A: uint8(220 * alpha)}
		vector.FillRect(screen, bgX, bgY, bgW, bgH, baseBG, false)

		// Accent stripe on the left edge — team coloured.
		stripeW := 4 * invZoom
		var accent color.RGBA
		if s.team == TeamRed {
			accent = color.RGBA{R: 230, G: 55, B: 40, A: uint8(230 * alpha)}
		} else {
			accent = color.RGBA{R: 40, G: 80, B: 230, A: uint8(230 * alpha)}
		}
		vector.FillRect(screen, bgX, bgY, stripeW, bgH, accent, false)

		// Border.
		vector.StrokeRect(screen, bgX, bgY, bgW, bgH, 1.0*invZoom,
			color.RGBA{R: 80, G: 100, B: 80, A: uint8(100 * alpha)}, false)

		// Main text — rendered at 1x into a temporary sub-image, then scaled.
		// Since DebugPrint can't scale, we approximate by drawing at fixed size
		// and accepting the zoom handles it via the camera transform.
		textX := int(bgX + float32(padX)*invZoom + stripeW)
		textY := int(bgY + float32(padY)*invZoom)
		ebitenutil.DebugPrintAt(screen, b.text, textX, textY)

		// Detail line.
		if b.detail != "" {
			ebitenutil.DebugPrintAt(screen, b.detail, textX, textY+int(float32(lineH)*invZoom))
		}

		// Connector line from bubble to soldier.
		vector.StrokeLine(screen, sx, bgY+bgH, sx, oy+float32(s.y)-float32(soldierRadius),
			1.0*invZoom, color.RGBA{R: 80, G: 100, B: 80, A: uint8(50 * alpha)}, false)
	}
}
