package game

import (
	"fmt"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// Inspector panel — rendered into an offscreen buffer at 1× then blitted at inspScale.
const (
	inspScale = 3   // scale factor for inspector text rendering
	inspBufW  = 220 // buffer width in pixels (~36 chars at debug font)
	inspBufH  = 280 // buffer height in pixels
	inspPad   = 4   // padding in buffer-space pixels
	inspLineH = 13  // line height in buffer-space pixels
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
	clickRadius2 := sqr(clickRadius)
	best2 := math.MaxFloat64
	var hit *Soldier
	all := append(g.soldiers[:len(g.soldiers):len(g.soldiers)], g.opfor...)
	for _, s := range all {
		if s.state == SoldierStateDead {
			continue
		}
		dx := s.x - wx
		dy := s.y - wy
		// Avoid sqrt by comparing squared distances to the squared click radius.
		d2 := dx*dx + dy*dy
		if d2 < clickRadius2 && d2 < best2 {
			best2 = d2
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

// drawInspector renders the inspector panel into an offscreen buffer at 1×,
// then blits it onto the screen at inspScale for readability.
func (g *Game) drawInspector(screen *ebiten.Image) {
	s := g.inspector.selected
	if s == nil {
		return
	}

	g.inspBuf.Clear()

	buf := g.inspBuf
	bw := float32(inspBufW)
	bh := float32(inspBufH)

	// Panel background.
	panelBg := color.RGBA{R: 14, G: 16, B: 14, A: 230}
	panelBorder := color.RGBA{R: 55, G: 80, B: 55, A: 255}
	vector.FillRect(buf, 0, 0, bw, bh, panelBg, false)
	vector.StrokeRect(buf, 0, 0, bw, bh, 1.0, panelBorder, false)
	// Inner highlight along top edge.
	vector.StrokeLine(buf, 1, 1, bw-1, 1, 1.0, color.RGBA{R: 70, G: 110, B: 70, A: 60}, false)

	lx := inspPad
	ly := inspPad

	// Title bar.
	teamStr := "RED"
	if s.team == TeamBlue {
		teamStr = "BLUE"
	}
	leaderStr := ""
	if s.isLeader {
		leaderStr = " [LDR]"
	}
	title := fmt.Sprintf("[ %s %s%s ]", teamStr, s.label, leaderStr)
	ebitenutil.DebugPrintAt(buf, title, lx, ly)
	ly += inspLineH + 2

	// Toggle button hint.
	viewName := "CURATED"
	if g.inspector.rawView {
		viewName = "RAW"
	}
	ebitenutil.DebugPrintAt(buf, fmt.Sprintf("view: %s  [I] toggle", viewName), lx, ly)
	ly += inspLineH + 4

	// Divider.
	vector.StrokeLine(buf, float32(lx), float32(ly), bw-float32(inspPad), float32(ly), 1.0, panelBorder, false)
	ly += 4

	if g.inspector.rawView {
		g.drawInspectorRaw(buf, s, lx, ly)
	} else {
		g.drawInspectorCurated(buf, s, lx, ly)
	}

	// Blit inspBuf onto screen at inspScale, positioned bottom-right.
	screenW := g.width
	screenH := g.height
	px := screenW - logPanelWidth - inspBufW*inspScale - g.offX - 12
	py := screenH - inspBufH*inspScale - g.offY - 8
	opts := &ebiten.DrawImageOptions{}
	opts.GeoM.Scale(float64(inspScale), float64(inspScale))
	opts.GeoM.Translate(float64(px), float64(py))
	screen.DrawImage(buf, opts)
}

// drawInspectorCurated draws the organised, human-readable inspector view.
func (g *Game) drawInspectorCurated(buf *ebiten.Image, s *Soldier, lx, ly int) {
	bb := &s.blackboard
	pr := &s.profile

	line := func(text string) {
		ebitenutil.DebugPrintAt(buf, text, lx, ly)
		ly += inspLineH
	}
	section := func(title string) {
		ly += 3
		ebitenutil.DebugPrintAt(buf, "-- "+title+" --", lx, ly)
		ly += inspLineH
	}
	bar := func(label string, v float64) {
		filled := int(v * 14)
		if filled < 0 {
			filled = 0
		}
		if filled > 14 {
			filled = 14
		}
		rest := 14 - filled
		b := ""
		for i := 0; i < filled; i++ {
			b += "█"
		}
		for i := 0; i < rest; i++ {
			b += "░"
		}
		ebitenutil.DebugPrintAt(buf, fmt.Sprintf("%-8s %s %.1f", label, b, v), lx, ly)
		ly += inspLineH
	}

	// ── SITUATION ──────────────────────────
	section("SITUATION")
	line(fmt.Sprintf("state: %-8s stance: %s", s.state, pr.Stance))
	line(fmt.Sprintf("goal:  %s", bb.CurrentGoal))
	line(fmt.Sprintf("intent: %s", bb.SquadIntent))
	if s.isLeader && s.squad != nil {
		line(fmt.Sprintf("sqd: %s sprd=%.0f",
			s.squad.Intent, s.squad.squadSpread()))
	}

	// Threat summary.
	visible := 0
	for _, t := range bb.Threats {
		if t.IsVisible {
			visible++
		}
	}
	line(fmt.Sprintf("threats: %dV %dK", visible, len(bb.Threats)))
	if visible > 0 {
		line(fmt.Sprintf("rng:%.0f hit:%.0f%%",
			bb.Internal.LastRange, bb.Internal.LastEstimatedHitChance*100))
	}
	if bb.SquadHasContact {
		line(fmt.Sprintf("contact:(%.0f,%.0f)", bb.SquadContactX, bb.SquadContactY))
	}
	if bb.IrrelevantCoverTicks > 0 {
		line(fmt.Sprintf("irrelevant_cover:%dt", bb.IrrelevantCoverTicks))
	}
	if bb.IsActivated() {
		line(fmt.Sprintf("mem:%.0f%% @(%.0f,%.0f)",
			bb.CombatMemoryStrength*100, bb.CombatMemoryX, bb.CombatMemoryY))
	}

	// ── DRIVES ─────────────────────────────
	section("DRIVES")
	bar("shoot", bb.Internal.ShootDesire)
	bar("move", bb.Internal.MoveDesire)
	bar("cover", bb.Internal.CoverDesire)
	momentum := bb.Internal.ShotMomentum
	sign := "+"
	if momentum < 0 {
		sign = "-"
	}
	line(fmt.Sprintf("momentum: %s%.2f", sign, math.Abs(momentum)))

	// ── PSYCHOLOGY ──────────────────────────
	section("PSYCHOLOGY")
	bar("fear", pr.Psych.Fear)
	bar("eff.fear", pr.Psych.EffectiveFear())
	bar("morale", pr.Psych.Morale)
	bar("composure", pr.Psych.Composure)
	bar("exp", pr.Psych.Experience)

	// ── PHYSICAL ───────────────────────────
	section("PHYSICAL")
	bar("fitness", pr.Physical.FitnessBase)
	bar("fatigue", pr.Physical.Fatigue)

	// ── SKILLS ─────────────────────────────
	section("SKILLS")
	bar("marks", pr.Skills.Marksmanship)
	bar("field", pr.Skills.Fieldcraft)
	bar("disc", pr.Skills.Discipline)

	// ── NEXT ACTION ────────────────────────
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
	line(fmt.Sprintf("in %d ticks [%s]", bb.NextDecisionTick-*s.currentTick, phaseStr))
	if bb.FlankComplete {
		line("flank: COMPLETE")
	} else if bb.FlankTargetX != 0 || bb.FlankTargetY != 0 {
		line(fmt.Sprintf("flank:(%.0f,%.0f)", bb.FlankTargetX, bb.FlankTargetY))
	}
	if bb.ShouldReinforce {
		line(fmt.Sprintf("reinf:(%.0f,%.0f)", bb.ReinforceMemberX, bb.ReinforceMemberY))
	}
	line(fmt.Sprintf("pos:(%.0f,%.0f) hp:%.0f%%", s.x, s.y, s.health*100))
}

// drawInspectorRaw dumps every blackboard field verbatim.
func (g *Game) drawInspectorRaw(buf *ebiten.Image, s *Soldier, lx, ly int) {
	bb := &s.blackboard
	pr := &s.profile

	line := func(text string) {
		ebitenutil.DebugPrintAt(buf, text, lx, ly)
		ly += inspLineH
	}

	line(fmt.Sprintf("id=%d %s team=%d", s.id, s.label, s.team))
	line(fmt.Sprintf("pos=(%.0f,%.0f) hp=%.2f", s.x, s.y, s.health))
	line(fmt.Sprintf("st=%s stn=%s ldr=%v", s.state, pr.Stance, s.isLeader))
	line(fmt.Sprintf("goal=%s prev=%s", bb.CurrentGoal, s.prevGoal))
	line(fmt.Sprintf("intent=%s ord=%v", bb.SquadIntent, bb.OrderReceived))
	line(fmt.Sprintf("sqdC=%v (%.0f,%.0f)", bb.SquadHasContact, bb.SquadContactX, bb.SquadContactY))
	line(fmt.Sprintf("moveOrd=%v (%.0f,%.0f)", bb.HasMoveOrder, bb.OrderMoveX, bb.OrderMoveY))
	line(fmt.Sprintf("thr=%d incFire=%d", len(bb.Threats), bb.IncomingFireCount))
	line(fmt.Sprintf("gun=%v (%.0f,%.0f)", bb.HeardGunfire, bb.HeardGunfireX, bb.HeardGunfireY))
	line(fmt.Sprintf("cmem=%.2f (%.0f,%.0f)", bb.CombatMemoryStrength, bb.CombatMemoryX, bb.CombatMemoryY))
	line(fmt.Sprintf("shat=%.2f/%.2f ph=%d pn=%v", bb.ShatterPressure, bb.ShatterThreshold, bb.CommitPhase(*s.currentTick), bb.PanicLocked))
	line(fmt.Sprintf("debt=%.2f next=%d", bb.DecisionDebt, bb.NextDecisionTick))
	line(fmt.Sprintf("flnk=%v (%.0f,%.0f)", bb.FlankComplete, bb.FlankTargetX, bb.FlankTargetY))
	line(fmt.Sprintf("fSide=%.0f bear=%.2f", bb.FlankSide, bb.SquadEnemyBearing))
	line(fmt.Sprintf("reinf=%v (%.0f,%.0f)", bb.ShouldReinforce, bb.ReinforceMemberX, bb.ReinforceMemberY))
	line(fmt.Sprintf("sight=%.2f", bb.LocalSightlineScore))
	line("-- internal --")
	line(fmt.Sprintf("shoot=%.2f move=%.2f", bb.Internal.ShootDesire, bb.Internal.MoveDesire))
	line(fmt.Sprintf("cover=%.2f mom=%.2f", bb.Internal.CoverDesire, bb.Internal.ShotMomentum))
	line(fmt.Sprintf("rng=%.0f hit=%.2f", bb.Internal.LastRange, bb.Internal.LastEstimatedHitChance))
	line(fmt.Sprintf("strk=%d hyst=%.2f", bb.GoalStreak, bb.HysteresisMargin))
	line(fmt.Sprintf("idle=%d fAdv=%v", bb.IdleCombatTicks, bb.ForceAdvance))
	line("-- thresholds --")
	th := bb.Internal.Thresholds
	line(fmt.Sprintf("eng=%.2f lng=%.2f a=%d", th.EngageShotQuality, th.LongRangeShotQuality, bb.Internal.ThresholdAge))
	line(fmt.Sprintf("psh=%.2f hld=%.2f cf=%.2f", th.PushOnMissMomentum, th.HoldOnHitMomentum, th.CoverFear))
	line("-- profile --")
	line(fmt.Sprintf("fit=%.2f fat=%.2f", pr.Physical.FitnessBase, pr.Physical.Fatigue))
	line(fmt.Sprintf("mrk=%.2f fld=%.2f dsc=%.2f", pr.Skills.Marksmanship, pr.Skills.Fieldcraft, pr.Skills.Discipline))
	line(fmt.Sprintf("fear=%.2f ef=%.2f mor=%.2f", pr.Psych.Fear, pr.Psych.EffectiveFear(), pr.Psych.Morale))
	line(fmt.Sprintf("comp=%.2f exp=%.2f", pr.Psych.Composure, pr.Psych.Experience))
	line(fmt.Sprintf("spd=%.2f acc=%.2f", pr.EffectiveSpeed(soldierSpeed), pr.EffectiveAccuracy()))
	// Visible threats detail.
	for i, t := range bb.Threats {
		if i >= 3 {
			line(fmt.Sprintf("  +%d more", len(bb.Threats)-3))
			break
		}
		vis := " "
		if t.IsVisible {
			vis = "V"
		}
		line(fmt.Sprintf("  t%d[%s] (%.0f,%.0f) c=%.2f", i, vis, t.X, t.Y, t.Confidence))
	}
}
