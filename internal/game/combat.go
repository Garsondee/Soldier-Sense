package game

import (
	"fmt"
	"image/color"
	"math"
	"math/rand"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// --- Combat constants ---

const (
	soldierMaxHP        = 100.0              // starting health
	accurateFireRange   = 450.0              // px, reliable engagement envelope (first half of rifle range)
	maxFireRange        = 900.0              // px, max rifle range (last half is pot-shot territory)
	potShotMaxFireRange = maxFireRange * 2.0 // px, extended pot-shot engagement envelope
	tracerLifetime      = 12                 // ticks a tracer persists
	tracerSpeed         = 2.8                // bullet travels full distance in this many ticks (faster = more visible)
	nearMissStress      = 0.08               // fear added to target on miss
	hitStress           = 0.20               // fear added to target on hit
	witnessStress       = 0.03               // fear added to nearby friendlies seeing a hit
	witnessRadius       = 80.0               // px radius for witness stress
	flashLifetime       = 4                  // ticks a muzzle flash persists

	// Per fire-mode fire intervals (ticks between trigger pulls).
	// Single: deliberate, slow. Burst: semi-rapid. Auto: rapid.
	fireIntervalSingle = 40 // ~0.67s — aim, squeeze
	fireIntervalBurst  = 20 // ~0.33s — committed burst rhythm
	fireIntervalAuto   = 10 // ~0.17s — sustained fire
	burstInterShotGap  = 2  // ticks between rounds in a burst/auto string

	// Long-range aiming behavior.
	aimingSuppressionBlock = 0.12 // suppression above this blocks deliberate aiming
	aimingBaseTicks        = 4    // minimum ticks to line up a long-range shot
	aimingExtraTicks       = 12   // additional ticks at max range
	aimingMaxSpreadBonus   = 0.45 // max spread reduction from full deliberate aim

	// Base damage per bullet. Short-range bonus applied separately.
	baseDamage = 25.0

	// Scatter radius for misses (pixels). Auto has wider scatter.
	missScatterSingle = 14.0
	missScatterBurst  = 22.0
	missScatterAuto   = 34.0

	// CQB distance: below this, damage and stress are boosted.
	cqbRange = autoRange // 160px — inside this is lethal

	// Gunfire hearing model.
	// Sounds beyond this distance are ignored.
	gunfireHearingMaxRange = 1400.0
	// Buildings strongly muffle gunfire but do not fully block it.
	gunfireOccludedMul = 0.40
	// Below this heard strength, soldiers ignore the signal.
	gunfireMinHeardStrength = 0.12
)

// shotRangePenalty returns accuracy loss for the given shot distance.
// Short range has a BONUS (negative penalty — closer = easier to hit).
// Inside CQB range, the shooter simply cannot miss much.
// Beyond accurateFireRange, penalties ramp hard in the pot-shot band.
func shotRangePenalty(dist float64) float64 {
	if dist <= 0 {
		return -0.32 // point-blank: strong bonus
	}
	if dist <= cqbRange {
		// Smooth bonus: 0 at cqbRange, -0.32 at zero.
		return -0.32 * (1.0 - dist/cqbRange)
	}
	if dist <= accurateFireRange {
		// Gentle linear penalty across the accurate band.
		return 0.08 * ((dist - cqbRange) / (accurateFireRange - cqbRange))
	}
	if dist >= potShotMaxFireRange {
		return 0.90
	}
	if dist >= maxFireRange {
		t := (dist - maxFireRange) / (potShotMaxFireRange - maxFireRange)
		return 0.78 + (0.90-0.78)*clamp01(t)
	}
	t := (dist - accurateFireRange) / (maxFireRange - accurateFireRange)
	return 0.12 + 0.66*math.Pow(t, 1.15)
}

func potShotFactor(dist float64) float64 {
	if dist <= accurateFireRange {
		return 0
	}
	return clamp01((dist - accurateFireRange) / (potShotMaxFireRange - accurateFireRange))
}

func shouldDeliberatelyAimLongRange(s *Soldier, dist, pressure float64) bool {
	pot := potShotFactor(dist)
	aimPreference := 0.80 - pressure*0.75 + s.profile.Skills.Discipline*0.18 + (1.0-pot)*0.08
	return aimPreference > 0.50
}

// fireModeParams bundles per-mode combat parameters.
type fireModeParams struct {
	shots       int     // bullets per trigger pull
	interval    int     // ticks between trigger pulls
	spreadRad   float64 // radians of arc spread applied to each round after the first
	accMul      float64 // accuracy multiplier for this mode
	damageMul   float64 // damage multiplier per bullet
	missScatter float64 // miss endpoint scatter radius
}

// fireModeTable maps each FireMode to its parameters.
var fireModeTable = map[FireMode]fireModeParams{
	FireModeSingle: {shots: 1, interval: fireIntervalSingle, spreadRad: 0, accMul: 1.0, damageMul: 1.0, missScatter: missScatterSingle},
	FireModeBurst:  {shots: 3, interval: fireIntervalBurst, spreadRad: 0.04, accMul: 0.85, damageMul: 1.0, missScatter: missScatterBurst},
	FireModeAuto:   {shots: 5, interval: fireIntervalAuto, spreadRad: 0.09, accMul: 0.70, damageMul: 1.0, missScatter: missScatterAuto},
}

// cqbDamageMul returns an extra damage multiplier for close-range fights.
// Represents higher hit probability on vital areas and terminal ballistics at short range.
// Uses a smooth fuzzy ramp: 1.0 at cqbRange, 1.8 at point-blank.
func cqbDamageMul(dist float64) float64 {
	if dist >= cqbRange {
		return 1.0
	}
	t := 1.0 - dist/cqbRange // 0 at cqbRange, 1 at 0
	return 1.0 + 0.8*t       // 1.0 → 1.8
}

// --- Tracer ---

// Tracer is a short-lived visual representing a bullet flight path.
// Uses sub-tick interpolation for smooth animation between game ticks.
type Tracer struct {
	fromX, fromY  float64
	toX, toY      float64
	hit           bool
	team          Team
	age           int     // ticks since spawn
	fractionalAge float64 // smooth sub-tick interpolation for rendering
}

// TracerDone returns true when the tracer should be removed.
func (t *Tracer) TracerDone() bool {
	return t.age >= tracerLifetime
}

// MuzzleFlash is a short-lived visual burst at the muzzle of a firing soldier.
type MuzzleFlash struct {
	x, y  float64
	angle float64 // firing direction
	team  Team
	age   int
}

// DrawTracer renders a bullet tracer with smooth sub-tick interpolation,
// thin anti-aliased lines, and soft gradient halos.
func (t *Tracer) DrawTracer(screen *ebiten.Image, offX, offY int) {
	if t.fractionalAge >= float64(tracerLifetime) {
		return
	}

	ox, oy := float32(offX), float32(offY)

	// Bullet travel uses eased progression to avoid visible stepping between ticks.
	travelProgress := clamp01(t.fractionalAge / tracerSpeed)
	travelProgress = travelProgress * travelProgress * (3.0 - 2.0*travelProgress)
	fadeProgress := math.Max(0.0, (t.fractionalAge-tracerSpeed)/(float64(tracerLifetime)-tracerSpeed))
	globalFade := float32(1.0 - fadeProgress)

	// Bullet position along flight path.
	headT := travelProgress
	tailLen := 0.22 // Slightly longer tail improves direction readability.
	tailT := math.Max(0.0, headT-tailLen)

	hx := float32(t.fromX + (t.toX-t.fromX)*headT)
	hy := float32(t.fromY + (t.toY-t.fromY)*headT)
	tx := float32(t.fromX + (t.toX-t.fromX)*tailT)
	ty := float32(t.fromY + (t.toY-t.fromY)*tailT)
	fx := float32(t.fromX)
	fy := float32(t.fromY)
	ex := float32(t.toX)
	ey := float32(t.toY)

	// Team colors.
	var coreR, coreG, coreB uint8
	var glowR, glowG, glowB uint8
	if t.team == TeamRed {
		coreR, coreG, coreB = 255, 210, 100
		glowR, glowG, glowB = 255, 180, 80
	} else {
		coreR, coreG, coreB = 100, 220, 255
		glowR, glowG, glowB = 80, 200, 255
	}
	pathAlpha := uint8(float32(34) * globalFade)

	// Faint full path guide helps read origin-to-endpoint instantly.
	vector.StrokeLine(screen, ox+fx, oy+fy, ox+ex, oy+ey, 0.7,
		color.RGBA{R: glowR, G: glowG, B: glowB, A: pathAlpha}, false)

	// Soft gradient halo - multiple layers with very low alpha for smooth falloff.
	// Layer 1: Outermost soft glow (widest, most transparent).
	vector.StrokeLine(screen, ox+tx, oy+ty, ox+hx, oy+hy, 4.0,
		color.RGBA{R: glowR, G: glowG, B: glowB, A: uint8(float32(15) * globalFade)}, false)
	// Layer 2: Mid glow.
	vector.StrokeLine(screen, ox+tx, oy+ty, ox+hx, oy+hy, 2.5,
		color.RGBA{R: glowR, G: glowG, B: glowB, A: uint8(float32(30) * globalFade)}, false)
	// Layer 3: Inner glow.
	vector.StrokeLine(screen, ox+tx, oy+ty, ox+hx, oy+hy, 1.5,
		color.RGBA{R: glowR, G: glowG, B: glowB, A: uint8(float32(60) * globalFade)}, false)

	// Core tracer line - thin and anti-aliased.
	vector.StrokeLine(screen, ox+tx, oy+ty, ox+hx, oy+hy, 0.8,
		color.RGBA{R: coreR, G: coreG, B: coreB, A: uint8(float32(220) * globalFade)}, false)

	// Bright tip - small and intense.
	// Soft outer glow.
	vector.FillCircle(screen, ox+hx, oy+hy, 2.0,
		color.RGBA{R: glowR, G: glowG, B: glowB, A: uint8(float32(40) * globalFade)}, false)
	// Bright core.
	vector.FillCircle(screen, ox+hx, oy+hy, 1.0,
		color.RGBA{R: 255, G: 255, B: 240, A: uint8(float32(240) * globalFade)}, false)

	// Origin cue makes it easier to see where the shot came from.
	vector.FillCircle(screen, ox+fx, oy+fy, 1.8,
		color.RGBA{R: coreR, G: coreG, B: coreB, A: uint8(float32(130) * globalFade)}, false)
	vector.FillCircle(screen, ox+fx, oy+fy, 0.9,
		color.RGBA{R: 255, G: 255, B: 230, A: uint8(float32(180) * globalFade)}, false)

	// Endpoint cue makes impact point easier to identify while the tracer is active.
	endA := uint8(float32(120) * globalFade)
	if t.hit {
		endA = uint8(float32(180) * globalFade)
	}
	vector.StrokeLine(screen, ox+ex-2.4, oy+ey, ox+ex+2.4, oy+ey, 0.9,
		color.RGBA{R: 255, G: 245, B: 210, A: endA}, false)
	vector.StrokeLine(screen, ox+ex, oy+ey-2.4, ox+ex, oy+ey+2.4, 0.9,
		color.RGBA{R: 255, G: 245, B: 210, A: endA}, false)

	// Impact flash - simple and visible.
	if t.hit && t.age <= 3 {
		impactX := float32(t.toX)
		impactY := float32(t.toY)
		flashFade := 1.0 - float32(t.age)/3.0

		// Soft outer flash.
		vector.FillCircle(screen, ox+impactX, oy+impactY, 6.0*flashFade+2.0,
			color.RGBA{R: 255, G: 240, B: 200, A: uint8(80 * flashFade)}, false)
		// Mid flash.
		vector.FillCircle(screen, ox+impactX, oy+impactY, 3.5*flashFade+1.0,
			color.RGBA{R: 255, G: 250, B: 220, A: uint8(160 * flashFade)}, false)
		// Bright core.
		vector.FillCircle(screen, ox+impactX, oy+impactY, 1.5*flashFade+0.5,
			color.RGBA{R: 255, G: 255, B: 255, A: uint8(240 * flashFade)}, false)
	}
}

type shooterTargetSelection struct {
	target      *Soldier
	targetX     float64
	targetY     float64
	firingAtLKP bool
	queuedBurst bool
}

func shouldSkipShooterCombatTick(s *Soldier) bool {
	if s.state == SoldierStateDead || s.state.IsIncapacitated() {
		return true
	}
	if s.magCapacity <= 0 {
		s.magCapacity = defaultMagazineCapacity
	}
	if s.magRounds < 0 {
		s.magRounds = 0
	}
	if handleShooterModeSwitchOrReload(s) {
		return true
	}
	if s.fireCooldown > 0 {
		s.fireCooldown--
		return true
	}
	if shooterCannotFireByState(s) {
		resetBurstState(s)
		resetAimingState(s)
		return true
	}
	return false
}

func handleShooterModeSwitchOrReload(s *Soldier) bool {
	if s.modeSwitchTimer > 0 {
		s.modeSwitchTimer--
		if s.modeSwitchTimer == 0 {
			s.currentFireMode = s.desiredFireMode
			s.think(fmt.Sprintf("fire mode → %s", s.currentFireMode))
		}
		resetBurstState(s)
		resetAimingState(s)
		return true
	}
	if s.reloadTimer > 0 {
		s.reloadTimer--
		if s.reloadTimer == 0 {
			s.magRounds = s.magCapacity
			s.think("reload complete")
		}
		resetBurstState(s)
		resetAimingState(s)
		return true
	}

	earlyReloadThreshold := int(float64(s.magCapacity) * (0.30 + s.profile.Preferences.ReloadEarly*0.40))
	needsReload := s.magRounds <= 0 || (s.magRounds <= earlyReloadThreshold && !s.blackboard.IsSuppressed())
	if !needsReload {
		return false
	}

	s.reloadTimer = s.reloadDurationTicks()
	s.fireCooldown = 0
	if s.magRounds <= 0 {
		s.think(fmt.Sprintf("reloading empty mag (%dt)", s.reloadTimer))
	} else {
		s.think(fmt.Sprintf("early reload %d/%d (%dt)", s.magRounds, s.magCapacity, s.reloadTimer))
	}
	resetBurstState(s)
	resetAimingState(s)
	return true
}

func shooterCannotFireByState(s *Soldier) bool {
	return s.blackboard.CurrentGoal == GoalSurvive || s.blackboard.Surrendered || s.blackboard.PanicRetreatActive
}

func resolveShooterTarget(cm *CombatManager, s *Soldier, allSoldiers []*Soldier) (shooterTargetSelection, bool) {
	confirmedContacts := visibleConfirmedContacts(s.vision.KnownContacts)
	lkpTargets := lkpThreats(s.blackboard.Threats)

	firingAtLKP := false
	var lkpTarget *ThreatFact
	switch {
	case len(confirmedContacts) > 0:
	case len(lkpTargets) > 0 && s.profile.Skills.Discipline > 0.3:
		firingAtLKP = true
		lkpTarget = closestLKPThreat(s, lkpTargets)
	default:
		resetBurstState(s)
		resetAimingState(s)
		return shooterTargetSelection{}, false
	}

	selection := shooterTargetSelection{queuedBurst: s.burstShotsRemaining > 0, firingAtLKP: firingAtLKP}
	if selection.firingAtLKP {
		if lkpTarget == nil {
			resetBurstState(s)
			resetAimingState(s)
			return shooterTargetSelection{}, false
		}
		selection.target = lkpTarget.Source
		selection.targetX = lkpTarget.X
		selection.targetY = lkpTarget.Y
		return selection, true
	}

	if selection.queuedBurst {
		selection.target = findSoldierByID(allSoldiers, s.burstTargetID)
	} else {
		selection.target = cm.closestContact(s)
	}
	if selection.target == nil || selection.target.state == SoldierStateDead || selection.target.state.IsIncapacitated() {
		resetBurstState(s)
		resetAimingState(s)
		return shooterTargetSelection{}, false
	}
	selection.targetX = selection.target.x
	selection.targetY = selection.target.y
	return selection, true
}

func closestLKPThreat(s *Soldier, lkpTargets []ThreatFact) *ThreatFact {
	bestDist := math.MaxFloat64
	var best *ThreatFact
	for i := range lkpTargets {
		dx := lkpTargets[i].X - s.x
		dy := lkpTargets[i].Y - s.y
		dist := math.Sqrt(dx*dx + dy*dy)
		if dist < bestDist {
			bestDist = dist
			best = &lkpTargets[i]
		}
	}
	return best
}

// --- Gunfire Events ---

// GunfireEvent records a shot being fired, for sound propagation.
type GunfireEvent struct {
	X, Y float64
	Team Team
	Tick int
}

// --- Combat Manager ---

// CombatManager handles firing resolution and tracer lifecycle.
type CombatManager struct {
	rng      *rand.Rand
	tracers  []*Tracer
	flashes  []*MuzzleFlash
	Gunfires []GunfireEvent
	tick     int
}

// NewCombatManager creates a combat manager with its own RNG.
func NewCombatManager(seed int64) *CombatManager {
	return &CombatManager{
		rng: rand.New(rand.NewSource(seed)), // #nosec G404 -- game only
	}
}

// ResetFireCounts zeros every soldier's per-tick fire counts and heard-gunfire flag.
// Combat memory (CombatMemoryStrength) is NOT reset here — it decays gradually.
func (cm *CombatManager) ResetFireCounts(soldiers []*Soldier) {
	for _, s := range soldiers {
		s.blackboard.IncomingFireCount = 0
		s.blackboard.HeardGunfire = false
		s.blackboard.DecayCombatMemory()
	}
	cm.Gunfires = cm.Gunfires[:0]
}

// BroadcastGunfire writes heard-gunfire info to enemy soldiers using a
// distance + occlusion + fieldcraft hearing model, then stamps persistent
// combat memory so they remain activated for ~60s.
func (cm *CombatManager) BroadcastGunfire(red, blue []*Soldier, tick int) {
	for _, ev := range cm.Gunfires {
		var listeners []*Soldier
		if ev.Team == TeamRed {
			listeners = blue
		} else {
			listeners = red
		}
		for _, s := range listeners {
			if s.state == SoldierStateDead {
				continue
			}
			heardStrength := gunfireHeardStrength(ev.X, ev.Y, s, red, blue)
			if heardStrength < gunfireMinHeardStrength {
				continue
			}

			// Phase 4: Add position jitter based on inverse fieldcraft
			fieldcraft := s.profile.Skills.Fieldcraft
			maxJitter := 80.0 // pixels
			jitterAmount := maxJitter * (1.0 - fieldcraft)
			angle := cm.rng.Float64() * 2 * math.Pi
			radius := cm.rng.Float64() * jitterAmount
			heardX := ev.X + math.Cos(angle)*radius
			heardY := ev.Y + math.Sin(angle)*radius

			// Single-tick flag — used by immediate decision logic.
			s.blackboard.HeardGunfireX = heardX
			s.blackboard.HeardGunfireY = heardY
			s.blackboard.HeardGunfire = true
			s.blackboard.HeardGunfireTick = tick
			// Persistent memory — keeps soldier activated for ~60s after last shot.
			s.blackboard.RecordGunfireWithStrength(heardX, heardY, heardStrength)
		}
		// Shooters also remember they fired (self-activation).
		var shooters []*Soldier
		if ev.Team == TeamRed {
			shooters = red
		} else {
			shooters = blue
		}
		for _, s := range shooters {
			if s.state == SoldierStateDead {
				continue
			}
			s.blackboard.RecordGunfireWithStrength(ev.X, ev.Y, 1.0)
		}
	}
}

// BroadcastGunfireSpatial uses spatial hashing for optimized gunfire propagation.
// Only checks soldiers within hearing range instead of all soldiers.
func (cm *CombatManager) BroadcastGunfireSpatial(redHash, blueHash *SpatialHash, red, blue []*Soldier, tick int) {
	for _, ev := range cm.Gunfires {
		var listenerHash *SpatialHash
		if ev.Team == TeamRed {
			listenerHash = blueHash
		} else {
			listenerHash = redHash
		}

		// Query only soldiers within hearing range using spatial hash.
		nearbyListeners := listenerHash.QueryRadius(ev.X, ev.Y, gunfireHearingMaxRange)

		for _, s := range nearbyListeners {
			if s.state == SoldierStateDead {
				continue
			}
			heardStrength := gunfireHeardStrength(ev.X, ev.Y, s, red, blue)
			if heardStrength < gunfireMinHeardStrength {
				continue
			}

			// Phase 4: Add position jitter based on inverse fieldcraft
			fieldcraft := s.profile.Skills.Fieldcraft
			maxJitter := 80.0 // pixels
			jitterAmount := maxJitter * (1.0 - fieldcraft)
			angle := cm.rng.Float64() * 2 * math.Pi
			radius := cm.rng.Float64() * jitterAmount
			heardX := ev.X + math.Cos(angle)*radius
			heardY := ev.Y + math.Sin(angle)*radius

			// Single-tick flag — used by immediate decision logic.
			s.blackboard.HeardGunfireX = heardX
			s.blackboard.HeardGunfireY = heardY
			s.blackboard.HeardGunfire = true
			s.blackboard.HeardGunfireTick = tick
			// Persistent memory — keeps soldier activated for ~60s after last shot.
			s.blackboard.RecordGunfireWithStrength(heardX, heardY, heardStrength)
		}

		// Shooters also remember they fired (self-activation).
		var shooters []*Soldier
		if ev.Team == TeamRed {
			shooters = red
		} else {
			shooters = blue
		}
		for _, s := range shooters {
			if s.state == SoldierStateDead {
				continue
			}
			s.blackboard.RecordGunfireWithStrength(ev.X, ev.Y, 1.0)
		}
	}
}

func gunfireHeardStrength(srcX, srcY float64, listener *Soldier, red, blue []*Soldier) float64 {
	dx := srcX - listener.x
	dy := srcY - listener.y
	dist := math.Hypot(dx, dy)
	if dist > gunfireHearingMaxRange {
		return 0
	}

	// Distance falloff with a small floor inside range.
	distanceFactor := 1.0 - dist/gunfireHearingMaxRange
	if distanceFactor < 0.10 {
		distanceFactor = 0.10
	}

	// Building occlusion muffles sound strongly.
	occlusionFactor := 1.0
	if !HasLineOfSightWithCoverIndexed(srcX, srcY, listener.x, listener.y, listener.buildings, listener.covers, listener.losIndex) {
		occlusionFactor = gunfireOccludedMul
	}

	// Fieldcraft slightly improves auditory cue extraction.
	fieldcraft := 0.0
	if listener.profile.Skills.Fieldcraft > 0 {
		fieldcraft = listener.profile.Skills.Fieldcraft
	}
	fieldcraftFactor := 0.85 + fieldcraft*0.30

	// Nearby allied listeners reinforce confidence in the heard direction.
	allies := red
	if listener.team == TeamBlue {
		allies = blue
	}
	nearbyAllies := 0
	for _, a := range allies {
		if a == nil || a == listener || a.state == SoldierStateDead {
			continue
		}
		if math.Hypot(a.x-listener.x, a.y-listener.y) <= 220 {
			nearbyAllies++
		}
	}
	allyBoost := 1.0 + math.Min(0.12, float64(nearbyAllies)*0.04)

	return clamp01(distanceFactor * occlusionFactor * fieldcraftFactor * allyBoost)
}

// ResolveCombat runs fire decisions for one set of shooters against a set of targets.
// AllFriendlies is the same-team list (for witness stress propagation).
// AllSoldiers is every soldier on the map (for ricochet near-miss stress).
func (cm *CombatManager) ResolveCombat(shooters, _, allFriendlies []*Soldier, buildings []rect, allSoldiers []*Soldier) { //nolint:gocognit,gocyclo
	for _, s := range shooters {
		if shouldSkipShooterCombatTick(s) {
			continue
		}

		selection, ok := resolveShooterTarget(cm, s, allSoldiers)
		if !ok {
			continue
		}
		target := selection.target
		targetX := selection.targetX
		targetY := selection.targetY
		firingAtLKP := selection.firingAtLKP
		queuedBurst := selection.queuedBurst

		// Range check.
		dx := targetX - s.x
		dy := targetY - s.y
		dist := math.Sqrt(dx*dx + dy*dy)
		if dist > potShotMaxFireRange {
			resetBurstState(s)
			resetAimingState(s)
			continue
		}

		// LOS check (buildings and tall walls block firing lines).
		// For LKP fire, we skip LOS check since we're firing at a remembered position
		if !firingAtLKP && !HasLineOfSightWithCoverIndexed(s.x, s.y, targetX, targetY, buildings, s.covers, s.losIndex) {
			resetBurstState(s)
			resetAimingState(s)
			continue
		}

		if !queuedBurst {
			// --- Fuzzy fire mode selection ---
			// Desired mode is determined by distance AND terrain sightline score.
			// Auto: CQB range AND enclosed terrain (low sightline) — true CQB conditions.
			// Burst: mid-range committed engagement.
			// Single: deliberate long-range fire.
			desired := cm.selectFireMode(s, dist)
			if desired != s.currentFireMode {
				// Fire-mode commitment: avoid flip-flopping right on range boundaries.
				// Under stress/suppression soldiers are more willing to force a switch.
				stress := clamp01(s.profile.Psych.EffectiveFear() + s.blackboard.SuppressLevel*0.7)
				switchPressure := clamp01(0.25 + stress*0.55 + s.blackboard.Internal.ShootDesire*0.20)
				if cm.rng.Float64() > switchPressure {
					desired = s.currentFireMode
				}
			}
			if desired != s.currentFireMode {
				// Trigger mode switch — pause firing while reconfiguring.
				s.desiredFireMode = desired
				s.modeSwitchTimer = modeSwitchTicks
				s.think(fmt.Sprintf("switching fire mode %s → %s", s.currentFireMode, desired))
				resetAimingState(s)
				continue
			}
		}

		// --- Build fire parameters from current mode ---
		params := fireModeTable[s.currentFireMode]

		// Turn to face target.
		targetH := math.Atan2(dy, dx)
		s.vision.UpdateHeading(targetH, math.Pi) // snap toward target

		// --- Physical fuzzy aim model ---
		// Total angular spread = soldier's accumulated aim spread (from movement)
		// + suppression jitter + fear spray + fire-mode base spread.
		suppressSpread := s.blackboard.SuppressLevel * 0.14
		fearSpread := s.profile.Psych.EffectiveFear() * 0.10
		// Stance multiplier: prone tightens spread, standing widens.
		stanceMul := 1.0 / math.Max(0.3, s.profile.Stance.Profile().AccuracyMul)
		woundAccMul := math.Max(0.1, s.body.AccuracyMul()) // floor to avoid divide-by-zero
		baseShooterSpread := (s.aimSpread + suppressSpread + fearSpread) * stanceMul / woundAccMul
		// Distance-dependent spread: pot-shot band becomes substantially inaccurate.
		baseShooterSpread += shotRangePenalty(dist) * 0.22

		// Aggression close combat bonus: aggressive soldiers get accuracy bonus at short range
		if dist < cqbRange {
			aggressionAccuracyBonus := s.profile.Personality.Aggression * 0.15 * (1.0 - dist/cqbRange)
			baseShooterSpread *= (1.0 - aggressionAccuracyBonus)
		}

		// Stealth ambush bonus: stealthy soldiers get accuracy bonus vs unaware targets
		if !firingAtLKP && target != nil {
			// Check if target has this soldier in their KnownContacts
			targetAware := false
			for _, contact := range target.vision.KnownContacts {
				if contact == s {
					targetAware = true
					break
				}
			}
			if !targetAware {
				// Target is unaware of this soldier - ambush bonus
				stealthAmbushBonus := s.profile.Survival.Stealth * 0.20
				baseShooterSpread *= (1.0 - stealthAmbushBonus)
			}
		}
		if queuedBurst && s.burstBaseSpread > 0 {
			baseShooterSpread = s.burstBaseSpread
		}

		// Phase 2: LKP suppression fire has reduced accuracy (-40% effective accuracy)
		if firingAtLKP {
			baseShooterSpread *= 1.67 // Inverse of 0.6 to reduce effective accuracy by 40%
		}

		// Effective target body radius reduced by cover and prone stance.
		// For LKP fire, use default radius since we don't know current stance
		baseBodyRadius := float64(soldierRadius)
		if target != nil {
			baseBodyRadius = float64(soldierRadius) * target.profile.Stance.Profile().ProfileMul
		}

		coverReduction := 0.0
		if !firingAtLKP && target != nil {
			if target.tileMap != nil {
				if inCover, defense := TileMapCoverBetween(target.tileMap, targetX, targetY, s.x, s.y); inCover {
					coverReduction = defense
				}
			}
			if coverReduction <= 0 {
				if inCover, defense := IsBehindCover(targetX, targetY, s.x, s.y, target.covers); inCover {
					coverReduction = defense
				}
			}
		}
		if coverReduction > 0.90 {
			coverReduction = 0.90
		}
		if coverReduction < 0 {
			coverReduction = 0
		}
		effBodyRadius := baseBodyRadius * (1.0 - coverReduction*0.7)

		// Angular half-size of target at this range.
		angularHalfSize := math.Atan2(effBodyRadius, math.Max(1, dist))

		// Expected hit probability for blackboard tracking (used by goal selection).
		hitChance := clamp01(angularHalfSize / math.Max(0.01, baseShooterSpread+params.spreadRad))

		if !queuedBurst && !firingAtLKP {
			// Long-range fire requires willingness to pull the trigger.
			// LKP fire skips deliberate aiming - it's suppression only
			if dist > accurateFireRange {
				pressure := clamp01(
					s.profile.Psych.EffectiveFear() +
						s.blackboard.SuppressLevel*0.85 +
						float64(s.blackboard.IncomingFireCount)*0.12,
				)
				pot := potShotFactor(dist)
				temptation := clamp01(
					0.34 +
						s.blackboard.Internal.ShootDesire*0.48 +
						s.blackboard.Internal.ShotMomentum*0.18 -
						s.blackboard.Internal.MoveDesire*0.20 +
						pot*0.22 +
						(1.0-pressure)*0.12,
				)
				if cm.rng.Float64() > temptation {
					resetAimingState(s)
					continue
				}

				if shouldDeliberatelyAimLongRange(s, dist, pressure) &&
					s.blackboard.IncomingFireCount == 0 && s.blackboard.SuppressLevel < aimingSuppressionBlock &&
					target != nil {
					requiredAimTicks := aimingTicksForDistance(dist)
					requiredAimTicks += int(math.Round(float64(aimingBaseTicks) * pot * (1.0 - pressure) * 0.8))
					if s.aimingTargetID != target.id {
						s.aimingTargetID = target.id
						s.aimingTicks = 0
						s.think("lining up long-range shot")
					}
					if s.aimingTicks < requiredAimTicks {
						s.aimingTicks++
						continue
					}
					aimProgress := 1.0
					if requiredAimTicks > 0 {
						aimProgress = clamp01(float64(s.aimingTicks) / float64(requiredAimTicks))
					}
					baseShooterSpread *= (1.0 - aimingMaxSpreadBonus*aimProgress)
				} else {
					resetAimingState(s)
				}
			} else {
				resetAimingState(s)
			}
		} else if firingAtLKP {
			// LKP fire doesn't use deliberate aiming
			resetAimingState(s)
		}

		if baseShooterSpread < 0.005 {
			baseShooterSpread = 0.005
		}

		// CQB damage multiplier — short range is much more lethal.
		dmgMul := cqbDamageMul(dist) * params.damageMul

		shotIdx := 0
		if queuedBurst {
			shotIdx = s.burstShotIndex
		}

		s.magRounds--
		if s.magRounds < 0 {
			s.magRounds = 0
		}

		cm.Gunfires = append(cm.Gunfires, GunfireEvent{X: s.x, Y: s.y, Team: s.team})
		cm.flashes = append(cm.flashes, &MuzzleFlash{x: s.x, y: s.y, angle: targetH, team: s.team})

		hit := cm.resolveBullet(s, target, shotIdx, baseShooterSpread, params, targetH, dist, angularHalfSize, dmgMul, allFriendlies, buildings, allSoldiers)

		if !queuedBurst {
			resetAimingState(s)
			if params.shots > 1 && target != nil {
				followUpRounds := params.shots - 1
				if followUpRounds > s.magRounds {
					followUpRounds = s.magRounds
				}
				if followUpRounds > 0 {
					s.burstShotsRemaining = followUpRounds
					s.burstShotIndex = 1
					s.burstTargetID = target.id
					s.burstAnyHit = hit
					s.burstHitChance = hitChance
					s.burstDist = dist
					s.burstBaseSpread = baseShooterSpread
					s.fireCooldown = burstInterShotGap
					continue
				}
			}

			// Aggression fire rate bonus: aggressive soldiers fire faster in sustained combat
			aggressionFireRateBonus := s.profile.Personality.Aggression * 0.25 // Up to 25% faster fire
			s.fireCooldown = int(float64(params.interval) * (1.0 - aggressionFireRateBonus))
			if s.fireCooldown < 1 {
				s.fireCooldown = 1 // Minimum 1 tick between shots
			}

			if forceReeval := s.blackboard.RecordShotOutcome(hit, hitChance, dist); forceReeval {
				s.blackboard.ShatterEvent = true
				s.think(fmt.Sprintf("3 consecutive misses (spread %.2f°) — changing approach",
					baseShooterSpread*180/math.Pi))
			}
			if firingAtLKP {
				if hit {
					s.think(fmt.Sprintf("suppression fire (%s) at LKP — HIT (spread %.1f°)", s.currentFireMode, baseShooterSpread*180/math.Pi))
				} else {
					s.think(fmt.Sprintf("suppression fire (%s) at LKP — miss (spread %.1f°)", s.currentFireMode, baseShooterSpread*180/math.Pi))
				}
			} else {
				if hit {
					s.think(fmt.Sprintf("fired (%s) — HIT (spread %.1f°)", s.currentFireMode, baseShooterSpread*180/math.Pi))
				} else {
					s.think(fmt.Sprintf("fired (%s) — miss (spread %.1f°)", s.currentFireMode, baseShooterSpread*180/math.Pi))
				}
			}
			continue
		}

		if hit {
			s.burstAnyHit = true
		}
		s.burstShotsRemaining--
		s.burstShotIndex++
		if s.burstShotsRemaining > 0 {
			s.fireCooldown = burstInterShotGap
			continue
		}

		s.fireCooldown = params.interval
		finalHitChance := s.burstHitChance
		if finalHitChance <= 0 {
			finalHitChance = hitChance
		}
		finalDist := s.burstDist
		if finalDist <= 0 {
			finalDist = dist
		}
		finalSpread := s.burstBaseSpread
		if finalSpread <= 0 {
			finalSpread = baseShooterSpread
		}
		if forceReeval := s.blackboard.RecordShotOutcome(s.burstAnyHit, finalHitChance, finalDist); forceReeval {
			s.blackboard.ShatterEvent = true
			s.think(fmt.Sprintf("3 consecutive misses (spread %.2f°) — changing approach",
				finalSpread*180/math.Pi))
		}
		if s.burstAnyHit {
			s.think(fmt.Sprintf("fired (%s) — HIT (spread %.1f°)", s.currentFireMode, finalSpread*180/math.Pi))
		} else {
			s.think(fmt.Sprintf("fired (%s) — miss (spread %.1f°)", s.currentFireMode, finalSpread*180/math.Pi))
		}
		resetBurstState(s)
	}
}

func resetBurstState(s *Soldier) {
	s.burstShotsRemaining = 0
	s.burstShotIndex = 0
	s.burstTargetID = -1
	s.burstAnyHit = false
	s.burstHitChance = 0
	s.burstDist = 0
	s.burstBaseSpread = 0
}

// visibleConfirmedContacts returns contacts that are currently visible (confirmed).
func visibleConfirmedContacts(contacts []*Soldier) []*Soldier {
	confirmed := make([]*Soldier, 0, len(contacts))
	for _, c := range contacts {
		if c.state != SoldierStateDead && !c.state.IsIncapacitated() {
			confirmed = append(confirmed, c)
		}
	}
	return confirmed
}

// lkpThreats returns threats that have LKP (not currently visible but confidence > threshold).
// These are targets for suppression fire.
func lkpThreats(threats []ThreatFact) []ThreatFact {
	const lkpConfidenceThreshold = 0.3
	lkp := make([]ThreatFact, 0)
	for _, t := range threats {
		if !t.IsVisible && t.Confidence > lkpConfidenceThreshold {
			if t.Source == nil || (t.Source.state != SoldierStateDead && !t.Source.state.IsIncapacitated()) {
				lkp = append(lkp, t)
			}
		}
	}
	return lkp
}

func resetAimingState(s *Soldier) {
	s.aimingTargetID = -1
	s.aimingTicks = 0
}

func aimingTicksForDistance(dist float64) int {
	if dist <= accurateFireRange {
		return 0
	}
	t := clamp01((dist - accurateFireRange) / (potShotMaxFireRange - accurateFireRange))
	return aimingBaseTicks + int(math.Round(float64(aimingExtraTicks)*t))
}

func findSoldierByID(all []*Soldier, id int) *Soldier {
	for _, s := range all {
		if s.id == id {
			return s
		}
	}
	return nil
}

func (cm *CombatManager) resolveBullet(
	shooter *Soldier,
	target *Soldier,
	shotIdx int,
	baseShooterSpread float64,
	params fireModeParams,
	targetH float64,
	dist float64,
	angularHalfSize float64,
	dmgMul float64,
	allFriendlies []*Soldier,
	buildings []rect,
	allSoldiers []*Soldier,
) bool {
	// Later shots in a burst have more muzzle climb.
	burstClimb := params.spreadRad * float64(shotIdx)
	totalSpread := baseShooterSpread + burstClimb
	if totalSpread < 0.005 {
		totalSpread = 0.005 // floor to avoid divide-by-zero
	}

	// Physical deflection: random angle within [-totalSpread, +totalSpread].
	// Using a triangular distribution (two uniform samples averaged)
	// for a more natural bell-shaped spread center.
	u1 := cm.rng.Float64()*2 - 1
	u2 := cm.rng.Float64()*2 - 1
	deflection := (u1 + u2) / 2.0 * totalSpread
	actualAngle := targetH + deflection

	// Hit if deflection falls within the target's angular body size.
	hit := math.Abs(deflection) <= angularHalfSize

	// Tracer endpoint follows actual bullet direction.
	toX := shooter.x + math.Cos(actualAngle)*(dist+30)
	toY := shooter.y + math.Sin(actualAngle)*(dist+30)
	if hit && target != nil {
		toX, toY = target.x, target.y
	}
	cm.tracers = append(cm.tracers, &Tracer{
		fromX: shooter.x, fromY: shooter.y,
		toX: toX, toY: toY,
		hit:  hit && target != nil, // Only show hit if we have a real target
		team: shooter.team,
	})

	// LKP fire: if target is nil, we're firing at a position, not a soldier
	// Hits only count if the target is actually there
	if hit && target != nil {
		damage := baseDamage * dmgMul

		// Roll hit region and create wound via body map.
		var coverMask [regionCount]float64 // TODO: populate from cover geometry
		wound, instantDeath := target.body.ApplyHit(damage, target.profile.Stance, coverMask, cm.tick, cm.rng)

		target.profile.Psych.ApplyStress(hitStress)
		target.blackboard.IncomingFireCount++
		target.blackboard.AccumulateSuppression(true, shooter.x, shooter.y, target.x, target.y)

		// Initialize casualty state on first wound.
		if target.body.WoundCount() == 1 {
			target.casualty = NewCasualtyState(cm.tick)
		}

		switch {
		case instantDeath:
			target.state = SoldierStateDead
			target.think(fmt.Sprintf("hit %s (%s) — killed instantly", wound.Region, wound.Severity))
		case target.body.HealthFraction() <= 0:
			target.state = SoldierStateDead
			target.think(fmt.Sprintf("hit %s (%s) — incapacitated", wound.Region, wound.Severity))
		default:
			target.think(fmt.Sprintf("hit %s (%s) — taking fire", wound.Region, wound.Severity))
		}
		cm.applyWitnessStress(target, allFriendlies)
		return true
	}

	// Near miss - only apply to actual target if present
	if target != nil {
		target.profile.Psych.ApplyStress(nearMissStress)
		target.blackboard.IncomingFireCount++
		target.blackboard.AccumulateSuppression(false, shooter.x, shooter.y, target.x, target.y)
		if shotIdx == 0 {
			target.think("near miss — incoming fire")
		}
	}
	cm.spawnRicochets(shooter.x, shooter.y, toX, toY, shooter.team, buildings, allSoldiers)
	return false
}

// selectFireMode uses fuzzy logic to choose the desired fire mode.
//
// Fuzzy rule set (priority order):
//  1. AUTO:   dist ≤ autoRange AND sightline < autoSightlineThresh
//     (CQB: cramped, close, no room to aim)
//  2. AUTO:   dist ≤ autoRange/2 regardless of terrain
//     (extreme point-blank — no choice but to spray)
//  3. BURST:  dist ≤ burstRange
//     (mid-range committed — controlled pairs/triples)
//  4. SINGLE: everything else — deliberate aimed fire
//
// Fuzzy blending: the transitions between modes aren't hard thresholds.
// A soft zone around each boundary lets randomness driven by the soldier's
// ShootDesire and fear create natural variation in when they switch.
func (cm *CombatManager) selectFireMode(s *Soldier, dist float64) FireMode {
	sightline := s.blackboard.LocalSightlineScore
	fear := s.profile.Psych.EffectiveFear()
	shootDesire := s.blackboard.Internal.ShootDesire
	stress := clamp01(fear + s.blackboard.SuppressLevel*0.7)

	if mode, ok := cm.tryStickCurrentFireMode(s, dist, sightline, fear, shootDesire); ok {
		return mode
	}

	if mode, ok := cm.selectCloseRangeMode(dist, sightline, fear, shootDesire, stress); ok {
		return mode
	}

	if mode, ok := cm.selectMidRangeMode(s, dist, fear, shootDesire, stress); ok {
		return mode
	}

	// --- Rule 4: long range — single shot.
	return FireModeSingle
}

func (cm *CombatManager) tryStickCurrentFireMode(
	s *Soldier,
	dist, sightline, fear, shootDesire float64,
) (FireMode, bool) {
	// Stickiness/hysteresis around mode boundaries to prevent chatter.
	// Current mode gets a deadband where it tends to persist.
	switch s.currentFireMode {
	case FireModeAuto:
		if dist <= float64(autoRange)*1.12 && (sightline < autoSightlineThresh+0.12 || fear > 0.45) && cm.rng.Float64() < 0.85 {
			return FireModeAuto, true
		}
	case FireModeBurst:
		if dist >= float64(autoRange)*0.92 && dist <= float64(burstRange)*1.08 {
			burstStick := clamp01(0.35 + fear*0.25 + shootDesire*0.20)
			if cm.rng.Float64() < burstStick {
				return FireModeBurst, true
			}
		}
	case FireModeSingle:
		if dist >= float64(burstRange)*0.88 {
			singleStick := clamp01(0.50 + s.profile.Skills.Marksmanship*0.20 - fear*0.15)
			if cm.rng.Float64() < singleStick {
				return FireModeSingle, true
			}
		}
	}
	return FireModeSingle, false
}

func (cm *CombatManager) selectCloseRangeMode(
	dist, sightline, fear, shootDesire, stress float64,
) (FireMode, bool) {
	pointBlankRange := float64(autoRange) / 2.0 // 5 tiles / 80px
	if dist <= pointBlankRange {
		return FireModeAuto, true
	}
	if dist > float64(autoRange) {
		return FireModeSingle, false
	}

	enclosedFactor := clamp01((autoSightlineThresh - sightline) / autoSightlineThresh)
	distFactor := clamp01(1.0 - dist/float64(autoRange))
	fearBoost := fear * 0.3
	autoMembership := enclosedFactor*0.5 + distFactor*0.35 + fearBoost + shootDesire*0.1
	if autoMembership > 0.45 {
		if stress > 0.58 && cm.rng.Float64() < (stress-0.58)*0.45 {
			return FireModeAuto, true
		}
		if cm.rng.Float64() < 0.80 {
			return FireModeAuto, true
		}
	}
	return FireModeBurst, true
}

func (cm *CombatManager) selectMidRangeMode(
	s *Soldier,
	dist, fear, shootDesire, stress float64,
) (FireMode, bool) {
	if dist > float64(burstRange) {
		return FireModeSingle, false
	}

	burstPressure := clamp01(0.4 + fear*0.3 + shootDesire*0.2 - s.profile.Skills.Marksmanship*0.2)
	if burstPressure <= 0.45 {
		return FireModeSingle, true
	}
	if stress > 0.60 && dist <= float64(autoRange)*1.05 && cm.rng.Float64() < (stress-0.60)*0.35 {
		return FireModeAuto, true
	}
	return FireModeBurst, true
}

// spawnRicochets checks if a missed shot's path intersects any wall, and if so,
// creates a reflected tracer that bounces off the wall. Ricochets apply near-miss
// stress to soldiers near the bounce path. Max 1 bounce per shot.
func (cm *CombatManager) spawnRicochets(fromX, fromY, toX, toY float64, shooterTeam Team, buildings []rect, allSoldiers []*Soldier) {
	// Find the first wall hit along the tracer path.
	bestT := 2.0 // >1 means no hit
	var hitWall rect
	for _, b := range buildings {
		t, hit := rayAABBHitT(fromX, fromY, toX, toY,
			float64(b.x), float64(b.y),
			float64(b.x+b.w), float64(b.y+b.h))
		if hit && t < bestT && t > 0.01 {
			bestT = t
			hitWall = b
		}
	}
	if bestT > 1.0 {
		return // no wall hit
	}

	// 40% chance of ricochet (not every bullet bounces).
	if cm.rng.Float64() > 0.40 {
		return
	}

	// Compute hit point on the wall.
	dx := toX - fromX
	dy := toY - fromY
	hitX := fromX + dx*bestT
	hitY := fromY + dy*bestT

	// Determine which face of the wall was hit to get the reflection normal.
	// Simple approach: check which edge of the AABB is closest to the hit point.
	wallCX := float64(hitWall.x) + float64(hitWall.w)/2
	wallCY := float64(hitWall.y) + float64(hitWall.h)/2
	relX := hitX - wallCX
	relY := hitY - wallCY

	var nx, ny float64
	if math.Abs(relX)/float64(hitWall.w) > math.Abs(relY)/float64(hitWall.h) {
		if relX > 0 {
			nx = 1
		} else {
			nx = -1
		}
	} else {
		if relY > 0 {
			ny = 1
		} else {
			ny = -1
		}
	}

	// Reflect the direction vector: r = d - 2(d·n)n
	dot := dx*nx + dy*ny
	rx := dx - 2*dot*nx
	ry := dy - 2*dot*ny

	// Ricochet travels a reduced distance (~60% of remaining path).
	remaining := math.Sqrt(dx*dx+dy*dy) * (1.0 - bestT) * 0.6
	rLen := math.Sqrt(rx*rx + ry*ry)
	if rLen < 1e-6 {
		return
	}
	endX := hitX + (rx/rLen)*remaining
	endY := hitY + (ry/rLen)*remaining

	// Spawn the ricochet tracer (always a miss visually).
	cm.tracers = append(cm.tracers, &Tracer{
		fromX: hitX, fromY: hitY,
		toX: endX, toY: endY,
		hit:  false,
		team: shooterTeam,
	})

	// Spawn a spark at the impact point.
	cm.flashes = append(cm.flashes, &MuzzleFlash{
		x: hitX, y: hitY,
		angle: math.Atan2(ry, rx),
		team:  shooterTeam,
	})

	// Apply near-miss stress to any soldier near the ricochet path.
	const ricochetStress = 0.06
	const ricochetRadius = 30.0
	for _, s := range allSoldiers {
		if s.state == SoldierStateDead {
			continue
		}
		// Point-to-segment distance from soldier to ricochet line.
		d := pointToSegmentDist(s.x, s.y, hitX, hitY, endX, endY)
		if d < ricochetRadius {
			s.profile.Psych.ApplyStress(ricochetStress)
			s.blackboard.IncomingFireCount++
		}
	}
}

// pointToSegmentDist is defined in roads.go (shared helper).

// closestContact returns the best target for a shooter based on distance and threat prioritization.
// High threat prioritization makes soldiers focus on the most dangerous targets first.
func (cm *CombatManager) closestContact(s *Soldier) *Soldier {
	var best *Soldier
	bestScore := -math.MaxFloat64
	threatPrio := s.profile.Survival.ThreatPrioritization
	coordFire := s.profile.Cooperation.CoordinatedFire
	squadmateTargets := buildSquadmateTargets(s, coordFire)

	for _, c := range s.vision.KnownContacts {
		if c.state == SoldierStateDead {
			continue
		}

		score := scoreContactTarget(s, c, coordFire, threatPrio, squadmateTargets)

		if score > bestScore {
			bestScore = score
			best = c
		}
	}
	return best
}

func buildSquadmateTargets(s *Soldier, coordFire float64) map[int]int {
	squadmateTargets := make(map[int]int)
	if coordFire <= 0.2 || s.squad == nil {
		return squadmateTargets
	}
	for _, squadmate := range s.squad.Members {
		if squadmate == nil || squadmate == s || squadmate.state == SoldierStateDead {
			continue
		}
		if squadmate.burstTargetID > 0 {
			squadmateTargets[squadmate.burstTargetID]++
		}
	}
	return squadmateTargets
}

func scoreContactTarget(
	shooter *Soldier,
	contact *Soldier,
	coordFire, threatPrio float64,
	squadmateTargets map[int]int,
) float64 {
	dx := contact.x - shooter.x
	dy := contact.y - shooter.y
	dist := math.Sqrt(dx*dx + dy*dy)

	score := 1000.0 / math.Max(dist, 1.0)

	if coordFire > 0.2 {
		if count, isEngaged := squadmateTargets[contact.id]; isEngaged {
			score += coordFire * float64(count) * 250.0
			score += shooter.profile.Personality.Teamwork * float64(count) * 150.0
		}
	}

	if threatPrio <= 0.3 {
		return score
	}

	if contact.fireCooldown > 0 || contact.burstShotsRemaining > 0 {
		score += threatPrio * 200.0
	}

	switch contact.profile.Stance {
	case StanceStanding:
		score += threatPrio * 100.0
	case StanceCrouching:
		score += threatPrio * 50.0
	}

	if dist < 150.0 {
		score += threatPrio * (150.0 - dist) * 2.0
	}

	return score
}

// applyWitnessStress adds stress to same-team soldiers near a hit target.
func (cm *CombatManager) applyWitnessStress(target *Soldier, friendlies []*Soldier) {
	for _, f := range friendlies {
		if f == target || f.state == SoldierStateDead {
			continue
		}
		dx := f.x - target.x
		dy := f.y - target.y
		if withinRadius2(dx, dy, witnessRadius*witnessRadius) {
			f.profile.Psych.ApplyStress(witnessStress)
		}
	}
}

// UpdateTracerInterpolation updates fractional ages for smooth sub-tick rendering.
// Called every frame with interpolation value [0, 1) representing progress to next tick.
func (cm *CombatManager) UpdateTracerInterpolation(interpolation float64) {
	for _, t := range cm.tracers {
		t.fractionalAge = float64(t.age) + interpolation
	}
}

// UpdateTracers ages and prunes tracers and muzzle flashes.
// Increments both integer age (for logic) and fractional age (for smooth rendering).
func (cm *CombatManager) UpdateTracers() {
	kept := cm.tracers[:0]
	for _, t := range cm.tracers {
		t.age++
		t.fractionalAge = float64(t.age)
		if !t.TracerDone() {
			kept = append(kept, t)
		}
	}
	cm.tracers = kept

	keptF := cm.flashes[:0]
	for _, f := range cm.flashes {
		f.age++
		if f.age < flashLifetime {
			keptF = append(keptF, f)
		}
	}
	cm.flashes = keptF
}

// DrawTracers renders all active tracers, offset by (offX, offY).
func (cm *CombatManager) DrawTracers(screen *ebiten.Image, offX, offY int) {
	for _, t := range cm.tracers {
		t.DrawTracer(screen, offX, offY)
	}
}

// DrawMuzzleFlashes renders muzzle flash effects with soft gradient halos
// and directional indicators showing where shots are coming from.
func (cm *CombatManager) DrawMuzzleFlashes(screen *ebiten.Image, offX, offY int) {
	ox, oy := float32(offX), float32(offY)
	for _, f := range cm.flashes {
		progress := float64(f.age) / float64(flashLifetime)
		fade := 1.0 - progress

		sx, sy := ox+float32(f.x), oy+float32(f.y)

		// Team-specific colors.
		var teamR, teamG, teamB uint8
		if f.team == TeamRed {
			teamR, teamG, teamB = 255, 180, 60
		} else {
			teamR, teamG, teamB = 100, 200, 255
		}

		// Soft gradient halo - multiple layers with low alpha for smooth falloff.
		// Outermost glow.
		glowR := float32(12.0) * float32(fade*0.7+0.3)
		vector.FillCircle(screen, sx, sy, glowR,
			color.RGBA{R: teamR, G: teamG, B: teamB, A: uint8(float64(40) * fade)}, false)
		// Mid glow.
		midR := float32(7.0) * float32(fade*0.6+0.4)
		vector.FillCircle(screen, sx, sy, midR,
			color.RGBA{R: teamR, G: teamG, B: teamB, A: uint8(float64(80) * fade)}, false)
		// Inner glow.
		innerR := float32(4.0) * float32(fade*0.5+0.5)
		vector.FillCircle(screen, sx, sy, innerR,
			color.RGBA{R: teamR, G: teamG, B: teamB, A: uint8(float64(140) * fade)}, false)
		// Bright core.
		coreR := float32(2.0) * float32(fade*0.4+0.6)
		vector.FillCircle(screen, sx, sy, coreR,
			color.RGBA{R: 255, G: 255, B: 240, A: uint8(float64(240) * fade)}, false)

		// Directional indicator - thin line showing firing direction.
		lineLen := float64(15.0) * fade
		ex := float32(f.x + math.Cos(f.angle)*lineLen)
		ey := float32(f.y + math.Sin(f.angle)*lineLen)
		// Soft outer line.
		vector.StrokeLine(screen, sx, sy, ox+ex, oy+ey, 2.5,
			color.RGBA{R: 255, G: 240, B: 200, A: uint8(float64(50) * fade)}, false)
		// Bright core line.
		vector.StrokeLine(screen, sx, sy, ox+ex, oy+ey, 1.0,
			color.RGBA{R: 255, G: 250, B: 220, A: uint8(float64(200) * fade)}, false)
	}
}

// ActiveFlashes returns the live muzzle flash slice for external lighting effects.
func (cm *CombatManager) ActiveFlashes() []*MuzzleFlash {
	return cm.flashes
}
