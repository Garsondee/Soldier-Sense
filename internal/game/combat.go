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
	soldierMaxHP      = 100.0 // starting health
	accurateFireRange = 300.0 // px, reliable engagement envelope
	maxFireRange      = 450.0 // px, includes low-accuracy long-range fire band (+50%)
	tracerLifetime    = 10    // ticks a tracer persists
	tracerSpeed       = 40.0  // px per tick (visual only)
	nearMissStress    = 0.08  // fear added to target on miss
	hitStress         = 0.20  // fear added to target on hit
	witnessStress     = 0.03  // fear added to nearby friendlies seeing a hit
	witnessRadius     = 80.0  // px radius for witness stress
	flashLifetime     = 4     // ticks a muzzle flash persists

	// Per fire-mode fire intervals (ticks between trigger pulls).
	// Single: deliberate, slow. Burst: semi-rapid. Auto: rapid.
	fireIntervalSingle = 40 // ~0.67s — aim, squeeze
	fireIntervalBurst  = 20 // ~0.33s — committed burst rhythm
	fireIntervalAuto   = 10 // ~0.17s — sustained fire

	// Base damage per bullet. Short-range bonus applied separately.
	baseDamage = 25.0

	// Scatter radius for misses (pixels). Auto has wider scatter.
	missScatterSingle = 14.0
	missScatterBurst  = 22.0
	missScatterAuto   = 34.0

	// CQB distance: below this, damage and stress are boosted.
	cqbRange = autoRange // 160px — inside this is lethal
)

// shotRangePenalty returns accuracy loss for the given shot distance.
// Short range has a BONUS (negative penalty — closer = easier to hit).
// Inside CQB range, the shooter simply cannot miss much.
// Beyond accurateFireRange, penalties ramp hard.
func shotRangePenalty(dist float64) float64 {
	if dist <= 0 {
		return -0.30 // point-blank: strong bonus
	}
	if dist <= cqbRange {
		// Smooth bonus: 0 at cqbRange, -0.30 at zero.
		return -0.30 * (1.0 - dist/cqbRange)
	}
	if dist <= accurateFireRange {
		// Gentle linear penalty across the accurate band.
		return 0.10 * ((dist - cqbRange) / (accurateFireRange - cqbRange))
	}
	if dist >= maxFireRange {
		return 0.45
	}
	t := (dist - accurateFireRange) / (maxFireRange - accurateFireRange)
	return 0.10 + 0.35*t
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
type Tracer struct {
	fromX, fromY float64
	toX, toY     float64
	hit          bool
	team         Team
	age          int // ticks since spawn
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

// DrawTracer renders a bullet tracer as a thin, fast line with a hot bright
// tip at the head and a cool dim tail that fades to transparent.
// Hits get a tiny impact flare; misses are purely the line.
func (t *Tracer) DrawTracer(screen *ebiten.Image, offX, offY int) {
	progress := float64(t.age) / float64(tracerLifetime)
	if progress > 1.0 {
		return
	}

	ox, oy := float32(offX), float32(offY)

	// Head advances quickly; short tight tail follows behind.
	headT := math.Min(1.0, progress*2.0)
	tailLen := 0.10 // very short tail — bullet moves fast
	tailT := math.Max(0.0, headT-tailLen)

	hx := float32(t.fromX + (t.toX-t.fromX)*headT)
	hy := float32(t.fromY + (t.toY-t.fromY)*headT)
	// Split the tail into 4 segments for a per-segment fade from hot→cold.
	const nSeg = 4
	globalFade := float32(1.0 - progress*progress)

	var hotR, hotG, hotB uint8 // team-tinted body colour
	if t.team == TeamRed {
		hotR, hotG, hotB = 255, 210, 100
	} else {
		hotR, hotG, hotB = 100, 220, 255
	}

	for i := 0; i < nSeg; i++ {
		t0 := tailT + (headT-tailT)*float64(i)/float64(nSeg)
		t1 := tailT + (headT-tailT)*float64(i+1)/float64(nSeg)
		sx0 := float32(t.fromX + (t.toX-t.fromX)*t0)
		sy0 := float32(t.fromY + (t.toY-t.fromY)*t0)
		sx1 := float32(t.fromX + (t.toX-t.fromX)*t1)
		sy1 := float32(t.fromY + (t.toY-t.fromY)*t1)

		// Intensity: head segment is brightest (i=nSeg-1), tail is dimmest (i=0).
		intensity := float32(i+1) / float32(nSeg) // 0.25→1.0
		a := uint8(float32(210) * intensity * globalFade)
		vector.StrokeLine(screen, ox+sx0, oy+sy0, ox+sx1, oy+sy1, 0.7,
			color.RGBA{R: hotR, G: hotG, B: hotB, A: a}, false)
	}

	// Bright white-hot tip dot at the head.
	tipA := uint8(float32(220) * globalFade)
	vector.FillCircle(screen, ox+hx, oy+hy, 0.9,
		color.RGBA{R: 255, G: 255, B: 230, A: tipA}, false)

	// On hit: tiny spark/flash at impact point (first tick only).
	if t.hit && t.age <= 1 {
		vector.FillCircle(screen, ox+float32(t.toX), oy+float32(t.toY), 2.5,
			color.RGBA{R: 255, G: 240, B: 180, A: 180}, false)
	}
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
	tracers  []*Tracer
	flashes  []*MuzzleFlash
	Gunfires []GunfireEvent // shots fired this tick, consumed by sound system
	rng      *rand.Rand
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

// BroadcastGunfire writes heard-gunfire info to all enemy soldiers AND
// stamps their persistent combat memory so they remain activated for ~60s.
// Sound is infinite range, no bouncing — every enemy hears every shot.
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
			// Single-tick flag — used by immediate decision logic.
			s.blackboard.HeardGunfireX = ev.X
			s.blackboard.HeardGunfireY = ev.Y
			s.blackboard.HeardGunfire = true
			s.blackboard.HeardGunfireTick = tick
			// Persistent memory — keeps soldier activated for ~60s after last shot.
			s.blackboard.RecordGunfire(ev.X, ev.Y)
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
			s.blackboard.RecordGunfire(ev.X, ev.Y)
		}
	}
}

// ResolveCombat runs fire decisions for one set of shooters against a set of targets.
// allFriendlies is the same-team list (for witness stress propagation).
// allSoldiers is every soldier on the map (for ricochet near-miss stress).
func (cm *CombatManager) ResolveCombat(shooters, targets, allFriendlies []*Soldier, buildings []rect, allSoldiers []*Soldier) {
	for _, s := range shooters {
		if s.state == SoldierStateDead {
			continue
		}

		// Tick down mode-switch timer first — soldier is busy changing modes.
		if s.modeSwitchTimer > 0 {
			s.modeSwitchTimer--
			if s.modeSwitchTimer == 0 {
				// Mode switch complete.
				s.currentFireMode = s.desiredFireMode
				s.think(fmt.Sprintf("fire mode → %s", s.currentFireMode))
			}
			continue
		}

		// Tick down fire cooldown.
		if s.fireCooldown > 0 {
			s.fireCooldown--
			continue
		}

		// Don't shoot while panicked.
		if s.blackboard.CurrentGoal == GoalSurvive {
			continue
		}

		// Need a visible target.
		if len(s.vision.KnownContacts) == 0 {
			continue
		}

		target := cm.closestContact(s)
		if target == nil || target.state == SoldierStateDead {
			continue
		}

		// Range check.
		dx := target.x - s.x
		dy := target.y - s.y
		dist := math.Sqrt(dx*dx + dy*dy)
		if dist > maxFireRange {
			continue
		}

		// LOS check (buildings and tall walls block firing lines).
		if !HasLineOfSightWithCover(s.x, s.y, target.x, target.y, buildings, s.covers) {
			continue
		}

		// --- Fuzzy fire mode selection ---
		// Desired mode is determined by distance AND terrain sightline score.
		// Auto: CQB range AND enclosed terrain (low sightline) — true CQB conditions.
		// Burst: mid-range committed engagement.
		// Single: deliberate long-range fire.
		desired := cm.selectFireMode(s, dist)
		if desired != s.currentFireMode {
			// Trigger mode switch — pause firing while reconfiguring.
			s.desiredFireMode = desired
			s.modeSwitchTimer = modeSwitchTicks
			s.think(fmt.Sprintf("switching fire mode %s → %s", s.currentFireMode, desired))
			continue
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
		baseShooterSpread := (s.aimSpread + suppressSpread + fearSpread) * stanceMul

		// Effective target body radius reduced by cover and prone stance.
		baseBodyRadius := float64(soldierRadius) * target.profile.Stance.Profile().ProfileMul
		coverReduction := 0.0
		if inCover, defence := IsBehindCover(target.x, target.y, s.x, s.y, target.covers); inCover {
			coverReduction = defence
		}
		effBodyRadius := baseBodyRadius * (1.0 - coverReduction*0.7)

		// Angular half-size of target at this range.
		angularHalfSize := math.Atan2(effBodyRadius, math.Max(1, dist))

		// Expected hit probability for blackboard tracking (used by goal selection).
		hitChance := clamp01(angularHalfSize / math.Max(0.01, baseShooterSpread+params.spreadRad))

		// Long-range fire requires willingness to pull the trigger.
		if dist > accurateFireRange {
			temptation := clamp01(
				0.25 +
					s.blackboard.Internal.ShootDesire*0.55 +
					s.blackboard.Internal.ShotMomentum*0.20 -
					s.blackboard.Internal.MoveDesire*0.35,
			)
			if cm.rng.Float64() > temptation {
				continue
			}
		}

		// CQB damage multiplier — short range is much more lethal.
		dmgMul := cqbDamageMul(dist) * params.damageMul

		// Set cooldown for the next burst/pull.
		s.fireCooldown = params.interval

		// Register one gunfire event per trigger pull (sound propagation).
		cm.Gunfires = append(cm.Gunfires, GunfireEvent{
			X: s.x, Y: s.y, Team: s.team,
		})
		cm.flashes = append(cm.flashes, &MuzzleFlash{
			x: s.x, y: s.y,
			angle: targetH,
			team:  s.team,
		})

		// --- Resolve each bullet in the burst/salvo ---
		anyHit := false
		for shotIdx := 0; shotIdx < params.shots; shotIdx++ {
			// Total spread for this specific shot in the burst.
			// Later shots in a burst have more muzzle climb.
			burstClimb := params.spreadRad * float64(shotIdx)
			totalSpread := baseShooterSpread + burstClimb
			if totalSpread < 0.005 {
				totalSpread = 0.005 // floor to avoid divide-by-zero
			}

			// Physical deflection: random angle within [-totalSpread, +totalSpread].
			// Using a triangular distribution (two uniform samples averaged)
			// for a more natural bell-shaped spread centre.
			u1 := cm.rng.Float64()*2 - 1
			u2 := cm.rng.Float64()*2 - 1
			deflection := (u1 + u2) / 2.0 * totalSpread
			actualAngle := targetH + deflection

			// Hit if deflection falls within the target's angular body size.
			hit := math.Abs(deflection) <= angularHalfSize

			// Tracer endpoint follows actual bullet direction.
			toX := s.x + math.Cos(actualAngle)*(dist+30)
			toY := s.y + math.Sin(actualAngle)*(dist+30)
			if hit {
				toX, toY = target.x, target.y
			}
			cm.tracers = append(cm.tracers, &Tracer{
				fromX: s.x, fromY: s.y,
				toX: toX, toY: toY,
				hit:  hit,
				team: s.team,
			})

			if hit {
				anyHit = true
				damage := baseDamage * dmgMul
				target.health -= damage
				target.profile.Psych.ApplyStress(hitStress)
				target.blackboard.IncomingFireCount++
				target.blackboard.AccumulateSuppression(true, s.x, s.y, target.x, target.y)
				if target.health <= 0 {
					target.health = 0
					target.state = SoldierStateDead
					target.think("incapacitated")
				} else {
					target.think("hit — taking fire")
				}
				cm.applyWitnessStress(target, allFriendlies)
			} else {
				target.profile.Psych.ApplyStress(nearMissStress)
				target.blackboard.IncomingFireCount++
				target.blackboard.AccumulateSuppression(false, s.x, s.y, target.x, target.y)
				if shotIdx == 0 {
					target.think("near miss — incoming fire")
				}
				cm.spawnRicochets(s.x, s.y, toX, toY, s.team, buildings, allSoldiers)
			}
		}

		// Record outcome for blackboard momentum tracking.
		// RecordShotOutcome returns true when 3+ consecutive misses force a re-evaluation.
		firstShotHit := anyHit
		if forceReeval := s.blackboard.RecordShotOutcome(firstShotHit, hitChance, dist); forceReeval {
			s.blackboard.ShatterEvent = true
			s.think(fmt.Sprintf("3 consecutive misses (spread %.2f°) — changing approach",
				baseShooterSpread*180/math.Pi))
		}
		if anyHit {
			s.think(fmt.Sprintf("fired (%s) — HIT (spread %.1f°)", s.currentFireMode, baseShooterSpread*180/math.Pi))
		} else {
			s.think(fmt.Sprintf("fired (%s) — miss (spread %.1f°)", s.currentFireMode, baseShooterSpread*180/math.Pi))
		}
	}
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

	// --- Rule 1: extreme CQB — always auto regardless of terrain.
	pointBlankRange := float64(autoRange) / 2.0 // 5 tiles / 80px
	if dist <= pointBlankRange {
		return FireModeAuto
	}

	// --- Rule 2: CQB range + enclosed terrain (fuzzy).
	if dist <= float64(autoRange) {
		// How enclosed is the terrain? Low sightline = enclosed.
		// How close are they? Closer = more auto pressure.
		// How scared are they? Fear drives spray.
		enclosedFactor := clamp01((autoSightlineThresh - sightline) / autoSightlineThresh)
		distFactor := clamp01(1.0 - dist/float64(autoRange))
		fearBoost := fear * 0.3
		autoMembership := enclosedFactor*0.5 + distFactor*0.35 + fearBoost + shootDesire*0.1
		if autoMembership > 0.45 {
			return FireModeAuto
		}
		// Falls through to burst if not quite enclosed enough.
		return FireModeBurst
	}

	// --- Rule 3: mid-range burst zone (fuzzy boundary with single).
	if dist <= float64(burstRange) {
		// Prefer burst, but experienced calm soldiers may stay on single
		// for better accuracy. Fear pushes toward burst (spray under stress).
		burstPressure := clamp01(
			0.4 +
				fear*0.3 +
				shootDesire*0.2 -
				s.profile.Skills.Marksmanship*0.2,
		)
		if burstPressure > 0.45 {
			return FireModeBurst
		}
		return FireModeSingle
	}

	// --- Rule 4: long range — single shot.
	return FireModeSingle
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

// pointToSegmentDist returns the distance from point (px,py) to the closest
// point on line segment (ax,ay)-(bx,by).
func pointToSegmentDist(px, py, ax, ay, bx, by float64) float64 {
	abx := bx - ax
	aby := by - ay
	apx := px - ax
	apy := py - ay
	ab2 := abx*abx + aby*aby
	if ab2 < 1e-12 {
		return math.Sqrt(apx*apx + apy*apy)
	}
	t := (apx*abx + apy*aby) / ab2
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	cx := ax + t*abx - px
	cy := ay + t*aby - py
	return math.Sqrt(cx*cx + cy*cy)
}

// closestContact returns the nearest visible contact for a shooter.
func (cm *CombatManager) closestContact(s *Soldier) *Soldier {
	var best *Soldier
	bestDist := math.MaxFloat64
	for _, c := range s.vision.KnownContacts {
		if c.state == SoldierStateDead {
			continue
		}
		dx := c.x - s.x
		dy := c.y - s.y
		d := dx*dx + dy*dy
		if d < bestDist {
			bestDist = d
			best = c
		}
	}
	return best
}

// applyWitnessStress adds stress to same-team soldiers near a hit target.
func (cm *CombatManager) applyWitnessStress(target *Soldier, friendlies []*Soldier) {
	for _, f := range friendlies {
		if f == target || f.state == SoldierStateDead {
			continue
		}
		dx := f.x - target.x
		dy := f.y - target.y
		if math.Sqrt(dx*dx+dy*dy) <= witnessRadius {
			f.profile.Psych.ApplyStress(witnessStress)
		}
	}
}

// UpdateTracers ages and prunes tracers and muzzle flashes.
func (cm *CombatManager) UpdateTracers() {
	kept := cm.tracers[:0]
	for _, t := range cm.tracers {
		t.age++
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

// DrawMuzzleFlashes renders muzzle flash effects at the firing position.
func (cm *CombatManager) DrawMuzzleFlashes(screen *ebiten.Image, offX, offY int) {
	ox, oy := float32(offX), float32(offY)
	for _, f := range cm.flashes {
		progress := float64(f.age) / float64(flashLifetime)
		alpha := uint8(255 * (1.0 - progress))

		sx, sy := ox+float32(f.x), oy+float32(f.y)

		// Outer glow.
		glowR := float32(8.0) * float32(1.0-progress*0.6)
		var glowCol color.RGBA
		if f.team == TeamRed {
			glowCol = color.RGBA{R: 255, G: 180, B: 40, A: uint8(float64(alpha) * 0.3)}
		} else {
			glowCol = color.RGBA{R: 100, G: 200, B: 255, A: uint8(float64(alpha) * 0.3)}
		}
		vector.FillCircle(screen, sx, sy, glowR, glowCol, false)

		// Bright core flash.
		coreR := float32(3.5) * float32(1.0-progress*0.5)
		coreCol := color.RGBA{R: 255, G: 255, B: 220, A: alpha}
		vector.FillCircle(screen, sx, sy, coreR, coreCol, false)

		// Short flash line in firing direction.
		lineLen := float64(12.0) * (1.0 - progress*0.7)
		ex := float32(f.x + math.Cos(f.angle)*lineLen)
		ey := float32(f.y + math.Sin(f.angle)*lineLen)
		vector.StrokeLine(screen, sx, sy, ox+ex, oy+ey, 1.5,
			color.RGBA{R: 255, G: 240, B: 160, A: uint8(float64(alpha) * 0.7)}, false)
	}
}

// ActiveFlashes returns the live muzzle flash slice for external lighting effects.
func (cm *CombatManager) ActiveFlashes() []*MuzzleFlash {
	return cm.flashes
}
