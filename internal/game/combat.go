package game

import (
	"image/color"
	"math"
	"math/rand"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// --- Combat constants ---

const (
	soldierMaxHP   = 100.0 // starting health
	fireInterval   = 30    // minimum ticks between shots (~0.5s at 60 TPS)
	maxFireRange   = 300.0 // px, same as vision range
	baseDamage     = 25.0  // HP per hit
	tracerLifetime = 8     // ticks a tracer persists
	tracerSpeed    = 40.0  // px per tick (visual only)
	nearMissStress = 0.08  // fear added to target on miss
	hitStress      = 0.20  // fear added to target on hit
	witnessStress  = 0.03  // fear added to nearby friendlies seeing a hit
	witnessRadius  = 80.0  // px radius for witness stress
	missScatter    = 20.0  // px random offset for miss endpoint
)

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

// DrawTracer renders a single tracer as a short bright line segment, offset by (offX, offY).
func (t *Tracer) DrawTracer(screen *ebiten.Image, offX, offY int) {
	progress := float64(t.age) / float64(tracerLifetime)
	if progress > 1.0 {
		return
	}

	// Head position along the path.
	headT := math.Min(1.0, progress*1.5) // head arrives before lifetime ends
	tailT := math.Max(0.0, headT-0.25)   // short tail behind the head

	hx := t.fromX + (t.toX-t.fromX)*headT
	hy := t.fromY + (t.toY-t.fromY)*headT
	tx := t.fromX + (t.toX-t.fromX)*tailT
	ty := t.fromY + (t.toY-t.fromY)*tailT

	// Team-tinted colour with fade.
	alpha := uint8(255 - uint8(progress*200))
	var c color.RGBA
	if t.team == TeamRed {
		c = color.RGBA{R: 255, G: 200, B: 60, A: alpha} // orange-yellow
	} else {
		c = color.RGBA{R: 60, G: 220, B: 255, A: alpha} // cyan
	}

	width := float32(1.5)
	if t.hit {
		width = 2.0
	}
	// Glow: wide translucent backing line.
	glowAlpha := uint8(float64(alpha) * 0.35)
	glowC := color.RGBA{R: c.R, G: c.G, B: c.B, A: glowAlpha}
	ox, oy := float32(offX), float32(offY)
	vector.StrokeLine(screen, ox+float32(tx), oy+float32(ty), ox+float32(hx), oy+float32(hy), width+3.0, glowC, false)
	// Bright core.
	vector.StrokeLine(screen, ox+float32(tx), oy+float32(ty), ox+float32(hx), oy+float32(hy), width, c, false)
}

// --- Combat Manager ---

// CombatManager handles firing resolution and tracer lifecycle.
type CombatManager struct {
	tracers []*Tracer
	rng     *rand.Rand
}

// NewCombatManager creates a combat manager with its own RNG.
func NewCombatManager(seed int64) *CombatManager {
	return &CombatManager{
		rng: rand.New(rand.NewSource(seed)), // #nosec G404 -- game only
	}
}

// ResetFireCounts zeros every soldier's IncomingFireCount for the new tick.
func (cm *CombatManager) ResetFireCounts(soldiers []*Soldier) {
	for _, s := range soldiers {
		s.blackboard.IncomingFireCount = 0
	}
}

// ResolveCombat runs fire decisions for one set of shooters against a set of targets.
// allFriendlies is the same-team list (for witness stress propagation).
func (cm *CombatManager) ResolveCombat(shooters, targets, allFriendlies []*Soldier, buildings []rect) {
	for _, s := range shooters {
		if s.state == SoldierStateDead {
			continue
		}
		// Tick down cooldown.
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

		// LOS check (buildings may have changed since vision scan).
		if !HasLineOfSight(s.x, s.y, target.x, target.y, buildings) {
			continue
		}

		// --- Fire! ---
		s.fireCooldown = fireInterval

		// Turn to face target.
		targetH := math.Atan2(dy, dx)
		s.vision.UpdateHeading(targetH, math.Pi) // snap toward target

		// Hit probability.
		// EffectiveAccuracy() already folds in shooter stance, fatigue, and fear.
		// A default soldier standing scores ~0.35; crouching ~0.45.
		// Range and target stance apply small additional penalties so skilled
		// soldiers at close range connect often, and extreme range is hard but
		// not impossible.
		accuracy := s.profile.EffectiveAccuracy()
		rangePenalty := 0.15 * (dist / maxFireRange)                               // max -0.15 at full range
		stancePenalty := (1.0 - target.profile.Stance.Profile().ProfileMul) * 0.15 // max -0.105 prone
		hitChance := accuracy - rangePenalty - stancePenalty
		hitChance = clamp01(hitChance)

		roll := cm.rng.Float64()
		hit := roll < hitChance

		// Spawn tracer.
		toX, toY := target.x, target.y
		if !hit {
			// Scatter the miss endpoint.
			angle := cm.rng.Float64() * 2 * math.Pi
			scatter := missScatter * (0.5 + cm.rng.Float64()*0.5)
			toX = target.x + math.Cos(angle)*scatter
			toY = target.y + math.Sin(angle)*scatter
		}
		cm.tracers = append(cm.tracers, &Tracer{
			fromX: s.x, fromY: s.y,
			toX: toX, toY: toY,
			hit:  hit,
			team: s.team,
		})

		// Log the shot.
		if hit {
			s.think("fired — HIT")
		} else {
			s.think("fired — miss")
		}

		// Apply effects.
		if hit {
			target.health -= baseDamage
			target.profile.Psych.ApplyStress(hitStress)
			target.blackboard.IncomingFireCount++
			if target.health <= 0 {
				target.health = 0
				target.state = SoldierStateDead
				target.think("incapacitated")
			} else {
				target.think("hit — taking fire")
			}
			// Witness stress to nearby friendlies.
			cm.applyWitnessStress(target, allFriendlies)
		} else {
			target.profile.Psych.ApplyStress(nearMissStress)
			target.blackboard.IncomingFireCount++
			target.think("near miss — incoming fire")
		}
	}
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

// UpdateTracers ages and prunes tracers.
func (cm *CombatManager) UpdateTracers() {
	kept := cm.tracers[:0]
	for _, t := range cm.tracers {
		t.age++
		if !t.TracerDone() {
			kept = append(kept, t)
		}
	}
	cm.tracers = kept
}

// DrawTracers renders all active tracers, offset by (offX, offY).
func (cm *CombatManager) DrawTracers(screen *ebiten.Image, offX, offY int) {
	for _, t := range cm.tracers {
		t.DrawTracer(screen, offX, offY)
	}
}
