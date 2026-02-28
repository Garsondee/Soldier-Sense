package game

import (
	"fmt"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// inspectorW / inspectorH — dimensions of the inspector panel in screen pixels.
const (
	inspectorW     = 340
	inspectorH     = 560
	inspectorPad   = 8
	inspectorLineH = 13
)

// Inspector holds the selected soldier and view toggle state.
type Inspector struct {
	selected *Soldier
	rawView  bool // false = curated, true = raw dump
}

// handleClick checks if a mouse click hit a soldier and selects it.
// Returns true if a soldier was hit.
func (g *Game) handleInspectorClick(mx, my int) bool {
	// Inverse of Draw camera transform:
	//   screen = (world - cam) * zoom + vpHalf + offset
	//   world  = (screen - offset - vpHalf) / zoom + cam
	vpW := float64(g.gameWidth)
	vpH := float64(g.gameHeight)
	wx := (float64(mx)-float64(g.offX)-vpW/2)/g.camZoom + g.camX
	wy := (float64(my)-float64(g.offY)-vpH/2)/g.camZoom + g.camY

	// Pick radius: 16 screen pixels expressed in world space.
	clickRadius := 16.0 / g.camZoom
	best := math.MaxFloat64
	var hit *Soldier
	all := append(g.soldiers[:len(g.soldiers):len(g.soldiers)], g.opfor...)
	for _, s := range all {
		if s.state == SoldierStateDead {
			continue
		}
		dx := s.x - wx
		dy := s.y - wy
		d := math.Sqrt(dx*dx + dy*dy)
		if d < clickRadius && d < best {
			best = d
			hit = s
		}
	}
	if hit != nil {
		g.inspector.selected = hit
		return true
	}
	// Click on empty space: deselect.
	g.inspector.selected = nil
	return false
}

// drawInspector renders the inspector panel at the bottom-right of the screen
// (screen-space, not world-space — unaffected by camera).
func (g *Game) drawInspector(screen *ebiten.Image) {
	s := g.inspector.selected
	if s == nil {
		return
	}

	// Panel position: bottom-right corner, above the log panel.
	screenW := g.width
	screenH := g.height
	px := screenW - logPanelWidth - inspectorW - inspectorPad*2 - g.offX
	py := screenH - inspectorH - g.offY - inspectorPad

	// Panel background.
	panelBg := color.RGBA{R: 14, G: 16, B: 14, A: 230}
	panelBorder := color.RGBA{R: 55, G: 80, B: 55, A: 255}
	vector.FillRect(screen, float32(px), float32(py), float32(inspectorW), float32(inspectorH), panelBg, false)
	vector.StrokeRect(screen, float32(px), float32(py), float32(inspectorW), float32(inspectorH), 1.5, panelBorder, false)

	lx := px + inspectorPad
	ly := py + inspectorPad

	// Team colour for the label.
	labelCol := "#FF6060"
	if s.team == TeamBlue {
		labelCol = "#6090FF"
	}
	_ = labelCol

	// Title bar.
	teamStr := "RED"
	if s.team == TeamBlue {
		teamStr = "BLUE"
	}
	leaderStr := ""
	if s.isLeader {
		leaderStr = " [LEADER]"
	}
	title := fmt.Sprintf("[ %s %s%s ]", teamStr, s.label, leaderStr)
	ebitenutil.DebugPrintAt(screen, title, lx, ly)
	ly += inspectorLineH + 2

	// Toggle button hint.
	viewName := "CURATED"
	if g.inspector.rawView {
		viewName = "RAW"
	}
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("view: %s   [I] toggle", viewName), lx, ly)
	ly += inspectorLineH + 4

	// Divider.
	vector.StrokeLine(screen, float32(lx), float32(ly), float32(px+inspectorW-inspectorPad), float32(ly), 1.0, panelBorder, false)
	ly += 4

	if g.inspector.rawView {
		g.drawInspectorRaw(screen, s, lx, ly)
	} else {
		g.drawInspectorCurated(screen, s, lx, ly)
	}
}

// drawInspectorCurated draws the organised, human-readable inspector view.
func (g *Game) drawInspectorCurated(screen *ebiten.Image, s *Soldier, lx, ly int) {
	bb := &s.blackboard
	pr := &s.profile

	line := func(text string) {
		ebitenutil.DebugPrintAt(screen, text, lx, ly)
		ly += inspectorLineH
	}
	section := func(title string) {
		ly += 3
		ebitenutil.DebugPrintAt(screen, "-- "+title+" --", lx, ly)
		ly += inspectorLineH
	}
	bar := func(label string, v float64) {
		filled := int(v * 18)
		if filled < 0 {
			filled = 0
		}
		if filled > 18 {
			filled = 18
		}
		rest := 18 - filled
		b := ""
		for i := 0; i < filled; i++ {
			b += "█"
		}
		for i := 0; i < rest; i++ {
			b += "░"
		}
		ebitenutil.DebugPrintAt(screen, fmt.Sprintf("%-10s %s %.2f", label, b, v), lx, ly)
		ly += inspectorLineH
	}

	// ── SITUATION ─────────────────────────────────────────
	section("SITUATION")
	line(fmt.Sprintf("state:  %-10s  stance: %s", s.state, pr.Stance))
	line(fmt.Sprintf("goal:   %-14s", bb.CurrentGoal))
	line(fmt.Sprintf("intent: %-14s", bb.SquadIntent))
	if s.isLeader && s.squad != nil {
		line(fmt.Sprintf("squad:  intent=%s  spread=%.0fpx",
			s.squad.Intent, s.squad.squadSpread()))
	}

	// Threat summary.
	visible := 0
	for _, t := range bb.Threats {
		if t.IsVisible {
			visible++
		}
	}
	line(fmt.Sprintf("threats: %d visible  %d known", visible, len(bb.Threats)))
	if visible > 0 {
		line(fmt.Sprintf("range:  %.0fpx   hit%%: %.0f%%",
			bb.Internal.LastRange, bb.Internal.LastEstimatedHitChance*100))
	}
	if bb.SquadHasContact {
		line(fmt.Sprintf("sqd contact: (%.0f, %.0f)", bb.SquadContactX, bb.SquadContactY))
	}
	if bb.IsActivated() {
		line(fmt.Sprintf("memory: %.0f%%  @ (%.0f,%.0f)",
			bb.CombatMemoryStrength*100, bb.CombatMemoryX, bb.CombatMemoryY))
	}

	// ── DRIVES ────────────────────────────────────────────
	section("DRIVES")
	bar("shoot", bb.Internal.ShootDesire)
	bar("move", bb.Internal.MoveDesire)
	bar("cover", bb.Internal.CoverDesire)
	momentum := bb.Internal.ShotMomentum
	sign := "+"
	if momentum < 0 {
		sign = "-"
	}
	line(fmt.Sprintf("shot momentum: %s%.2f", sign, math.Abs(momentum)))

	// ── PSYCHOLOGY ────────────────────────────────────────
	section("PSYCHOLOGY")
	bar("fear", pr.Psych.Fear)
	bar("eff.fear", pr.Psych.EffectiveFear())
	bar("morale", pr.Psych.Morale)
	bar("composure", pr.Psych.Composure)
	bar("experience", pr.Psych.Experience)

	// ── PHYSICAL ──────────────────────────────────────────
	section("PHYSICAL")
	bar("fitness", pr.Physical.FitnessBase)
	bar("fatigue", pr.Physical.Fatigue)

	// ── SKILLS ────────────────────────────────────────────
	section("SKILLS")
	bar("marksman", pr.Skills.Marksmanship)
	bar("fieldcraft", pr.Skills.Fieldcraft)
	bar("discipline", pr.Skills.Discipline)

	// ── NEXT ACTION ───────────────────────────────────────
	section("NEXT ACTION")
	phaseStr := "review"
	if s.currentTick != nil {
		switch bb.CommitPhase(*s.currentTick) {
		case 0:
			phaseStr = "COMMIT"
		case 1:
			phaseStr = "sustain"
		}
	}
	if bb.PanicLocked {
		phaseStr = "PANIC"
	}
	line(fmt.Sprintf("decide in: %d ticks [%s]", bb.NextDecisionTick-*s.currentTick, phaseStr))
	if bb.FlankComplete {
		line("flank: COMPLETE")
	} else if bb.FlankTargetX != 0 || bb.FlankTargetY != 0 {
		line(fmt.Sprintf("flank target: (%.0f, %.0f)", bb.FlankTargetX, bb.FlankTargetY))
	}
	if bb.ShouldReinforce {
		line(fmt.Sprintf("reinforce → (%.0f, %.0f)", bb.ReinforceMemberX, bb.ReinforceMemberY))
	}
	line(fmt.Sprintf("pos: (%.0f, %.0f)  hp: %.0f%%", s.x, s.y, s.health*100))
}

// drawInspectorRaw dumps every blackboard field verbatim.
func (g *Game) drawInspectorRaw(screen *ebiten.Image, s *Soldier, lx, ly int) {
	bb := &s.blackboard
	pr := &s.profile

	line := func(text string) {
		ebitenutil.DebugPrintAt(screen, text, lx, ly)
		ly += inspectorLineH
	}

	line(fmt.Sprintf("id=%d label=%s team=%d", s.id, s.label, s.team))
	line(fmt.Sprintf("pos=(%.1f,%.1f) health=%.2f", s.x, s.y, s.health))
	line(fmt.Sprintf("state=%s stance=%s isLeader=%v", s.state, pr.Stance, s.isLeader))
	line(fmt.Sprintf("goal=%s prevGoal=%s", bb.CurrentGoal, s.prevGoal))
	line(fmt.Sprintf("squadIntent=%s orderRecv=%v", bb.SquadIntent, bb.OrderReceived))
	line(fmt.Sprintf("squadHasContact=%v cX=%.0f cY=%.0f", bb.SquadHasContact, bb.SquadContactX, bb.SquadContactY))
	line(fmt.Sprintf("hasMoveOrder=%v oX=%.0f oY=%.0f", bb.HasMoveOrder, bb.OrderMoveX, bb.OrderMoveY))
	line(fmt.Sprintf("threats=%d incomingFire=%d", len(bb.Threats), bb.IncomingFireCount))
	line(fmt.Sprintf("heardGunfire=%v hX=%.0f hY=%.0f", bb.HeardGunfire, bb.HeardGunfireX, bb.HeardGunfireY))
	line(fmt.Sprintf("combatMem=%.2f mX=%.0f mY=%.0f", bb.CombatMemoryStrength, bb.CombatMemoryX, bb.CombatMemoryY))
	line(fmt.Sprintf("shatterP=%.2f/%.2f phase=%d panic=%v debt=%.2f", bb.ShatterPressure, bb.ShatterThreshold, bb.CommitPhase(*s.currentTick), bb.PanicLocked, bb.DecisionDebt))
	line(fmt.Sprintf("nextDecision=%d (now=%d)", bb.NextDecisionTick, *s.currentTick))
	line(fmt.Sprintf("flankComplete=%v fX=%.0f fY=%.0f", bb.FlankComplete, bb.FlankTargetX, bb.FlankTargetY))
	line(fmt.Sprintf("flankSide=%.0f squadBearing=%.2f", bb.FlankSide, bb.SquadEnemyBearing))
	line(fmt.Sprintf("shouldReinforce=%v rX=%.0f rY=%.0f", bb.ShouldReinforce, bb.ReinforceMemberX, bb.ReinforceMemberY))
	line(fmt.Sprintf("sightlineScore=%.2f", bb.LocalSightlineScore))
	line("-- internal --")
	line(fmt.Sprintf("shootDesire=%.2f moveDesire=%.2f", bb.Internal.ShootDesire, bb.Internal.MoveDesire))
	line(fmt.Sprintf("coverDesire=%.2f shotMomentum=%.2f", bb.Internal.CoverDesire, bb.Internal.ShotMomentum))
	line(fmt.Sprintf("lastRange=%.0f hitChance=%.2f", bb.Internal.LastRange, bb.Internal.LastEstimatedHitChance))
	line(fmt.Sprintf("goalStreak=%d hysteresis=%.2f debt=%.2f", bb.GoalStreak, bb.HysteresisMargin, bb.DecisionDebt))
	line(fmt.Sprintf("idleCombatTicks=%d forceAdvance=%v", bb.IdleCombatTicks, bb.ForceAdvance))
	line("-- thresholds (evolved) --")
	th := bb.Internal.Thresholds
	line(fmt.Sprintf("engageShot=%.2f longRange=%.2f age=%d", th.EngageShotQuality, th.LongRangeShotQuality, bb.Internal.ThresholdAge))
	line(fmt.Sprintf("pushMiss=%.2f holdHit=%.2f coverFear=%.2f", th.PushOnMissMomentum, th.HoldOnHitMomentum, th.CoverFear))
	line("-- profile --")
	line(fmt.Sprintf("fitness=%.2f fatigue=%.2f", pr.Physical.FitnessBase, pr.Physical.Fatigue))
	line(fmt.Sprintf("marks=%.2f field=%.2f disc=%.2f", pr.Skills.Marksmanship, pr.Skills.Fieldcraft, pr.Skills.Discipline))
	line(fmt.Sprintf("fear=%.2f effFear=%.2f morale=%.2f", pr.Psych.Fear, pr.Psych.EffectiveFear(), pr.Psych.Morale))
	line(fmt.Sprintf("composure=%.2f exp=%.2f", pr.Psych.Composure, pr.Psych.Experience))
	line(fmt.Sprintf("effSpeed=%.2f effAcc=%.2f", pr.EffectiveSpeed(soldierSpeed), pr.EffectiveAccuracy()))
	// Visible threats detail.
	for i, t := range bb.Threats {
		if i >= 4 {
			line(fmt.Sprintf("  ...+%d more", len(bb.Threats)-4))
			break
		}
		vis := " "
		if t.IsVisible {
			vis = "V"
		}
		line(fmt.Sprintf("  t%d[%s] (%.0f,%.0f) conf=%.2f", i, vis, t.X, t.Y, t.Confidence))
	}
}
