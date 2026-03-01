package game

import "math"

// --- Stance ---

// Stance represents a soldier's body posture.
type Stance int

const (
	StanceStanding Stance = iota
	StanceCrouching
	StanceProne
)

// StanceProfile holds the gameplay modifiers for a stance.
type StanceProfile struct {
	SpeedMul     float64 // multiplier on base move speed
	AccuracyMul  float64 // multiplier on base accuracy (higher = better)
	ProfileMul   float64 // visual/hit profile size multiplier (lower = harder to hit/see)
	TransitionMs int     // milliseconds to switch INTO this stance
}

var stanceProfiles = map[Stance]StanceProfile{
	StanceStanding:  {SpeedMul: 1.0, AccuracyMul: 0.7, ProfileMul: 1.0, TransitionMs: 0},
	StanceCrouching: {SpeedMul: 0.5, AccuracyMul: 0.9, ProfileMul: 0.6, TransitionMs: 300},
	StanceProne:     {SpeedMul: 0.15, AccuracyMul: 1.0, ProfileMul: 0.3, TransitionMs: 700},
}

// Profile returns the gameplay modifiers for this stance.
func (s Stance) Profile() StanceProfile {
	return stanceProfiles[s]
}

func (s Stance) String() string {
	switch s {
	case StanceStanding:
		return "standing"
	case StanceCrouching:
		return "crouching"
	case StanceProne:
		return "prone"
	default:
		return "unknown"
	}
}

// --- Physical Stats ---

// PhysicalStats represents a soldier's physical condition.
type PhysicalStats struct {
	FitnessBase float64 // 0-1, innate physical capability
	Fatigue     float64 // 0-1, current exhaustion (0 = fresh, 1 = collapsed)
	SprintPool  float64 // seconds of sprint remaining
}

// EffectiveFitness returns fitness degraded by fatigue.
func (p *PhysicalStats) EffectiveFitness() float64 {
	return p.FitnessBase * (1.0 - p.Fatigue*0.8)
}

// AccumulateFatigue adds fatigue based on exertion level (0-1) per tick.
// Recovery happens at a slower rate when exertion is 0.
func (p *PhysicalStats) AccumulateFatigue(exertion float64, dt float64) {
	if exertion > 0 {
		rate := 0.01 * exertion / p.FitnessBase // less fit soldiers tire faster
		p.Fatigue = math.Min(1.0, p.Fatigue+rate*dt)
	} else {
		recovery := 0.003 * p.FitnessBase // fitter soldiers recover faster
		p.Fatigue = math.Max(0.0, p.Fatigue-recovery*dt)
	}
}

// --- Skill Stats ---

// SkillStats represents trained abilities.
type SkillStats struct {
	Marksmanship float64 // 0-1, shooting accuracy
	Fieldcraft   float64 // 0-1, ability to use terrain/cover
	Discipline   float64 // 0-1, order compliance under pressure
	FirstAid     float64 // 0-1, medical competence
}

// --- Psychological State ---

// PsychState represents the soldier's current mental/emotional state.
type PsychState struct {
	Experience float64 // 0-1, combat exposure (permanent, grows slowly)
	Morale     float64 // 0-1, confidence (fluctuates)
	Fear       float64 // 0-1, acute stress (spikes under fire, decays)
	Composure  float64 // 0-1, innate ability to manage fear (trait)
}

// MoraleContext captures the social and tactical conditions that shape morale.
type MoraleContext struct {
	UnderFire         bool
	IncomingFireCount int
	SuppressLevel     float64
	VisibleThreats    int
	VisibleAllies     int
	IsolatedTicks     int
	SquadCasualtyRate float64
	SquadStress       float64
	SquadAvgFear      float64
	SquadFearDelta    float64
	CloseAllyPressure float64
	ShotMomentum      float64
	LocalSightline    float64
	HasContact        bool
}

// EffectiveFear returns fear modulated by composure and experience.
// A composed veteran feels fear but acts despite it.
func (ps *PsychState) EffectiveFear() float64 {
	dampening := 0.5*ps.Composure + 0.5*ps.Experience
	return ps.Fear * (1.0 - dampening*0.6)
}

// WillComply returns true if the soldier will follow an order given current state.
// Based on: discipline + morale + trust - effective_fear - fatigue_penalty
func (ps *PsychState) WillComply(discipline, fatigue float64) bool {
	score := discipline + ps.Morale*0.4 - ps.EffectiveFear()*0.6 - fatigue*0.2
	return score > 0.3
}

// ApplyStress increases fear. Capped at 1.0.
func (ps *PsychState) ApplyStress(amount float64) {
	ps.Fear = math.Min(1.0, ps.Fear+amount)
}

// RecoverFear decays fear over time. Rate affected by composure and morale.
func (ps *PsychState) RecoverFear(dt float64) {
	rate := 0.02 * (0.5 + 0.3*ps.Composure + 0.2*ps.Morale)
	ps.Fear = math.Max(0.0, ps.Fear-rate*dt)
}

// RecoverMorale slowly restores morale during calm periods.
func (ps *PsychState) RecoverMorale(dt float64) {
	if ps.Fear < 0.2 {
		rate := 0.005
		ps.Morale = math.Min(1.0, ps.Morale+rate*dt)
	}
}

// UpdateMorale applies a richer morale model that accounts for combat pressure,
// social context, confidence feedback, and recovery conditions.
func (ps *PsychState) UpdateMorale(dt, discipline float64, ctx MoraleContext) {
	if dt <= 0 {
		return
	}

	ef := clamp01(ps.EffectiveFear())

	threatLoad := 0.0
	if ctx.UnderFire {
		incoming := math.Min(3.0, float64(ctx.IncomingFireCount))
		threatLoad += 0.035 + incoming*0.020
	}
	threatLoad += clamp01(ctx.SuppressLevel) * 0.030
	threatLoad += ef * 0.018
	if ctx.VisibleThreats > 0 && ctx.VisibleAllies == 0 {
		threatLoad += 0.012
	}
	if ctx.VisibleThreats == 0 && ctx.IsolatedTicks > 0 {
		threatLoad += clamp01(float64(ctx.IsolatedTicks)/180.0) * 0.010
	}
	if ctx.SquadFearDelta > 0.01 {
		threatLoad += clamp01(ctx.SquadFearDelta*8.0) * 0.012
	}
	threatLoad += clamp01(ctx.CloseAllyPressure) * 0.008
	if ctx.ShotMomentum < 0 {
		threatLoad += clamp01(-ctx.ShotMomentum) * 0.006
	}
	threatLoad += clamp01(ctx.SquadCasualtyRate) * 0.020
	threatLoad += clamp01(ctx.SquadStress) * 0.016

	supportGain := 0.0
	if ctx.VisibleAllies > 0 {
		allySupport := clamp01(float64(ctx.VisibleAllies) / 4.0)
		calmSquad := clamp01(1.0 - ctx.SquadAvgFear)
		supportGain += allySupport * (0.010 + calmSquad*0.010)
	}
	if !ctx.UnderFire && ctx.SuppressLevel < 0.2 && ctx.VisibleThreats == 0 {
		calmness := clamp01((0.35 - ps.Fear) / 0.35)
		recoveryGain := 0.006 + calmness*0.008
		if ctx.HasContact {
			recoveryGain *= 0.80
		}
		supportGain += recoveryGain
	}
	if ctx.ShotMomentum > 0 {
		supportGain += clamp01(ctx.ShotMomentum) * 0.009
	}
	if ctx.LocalSightline > 0.55 && !ctx.UnderFire {
		supportGain += (ctx.LocalSightline - 0.55) * 0.008
	}

	resilience := 0.55 + discipline*0.25 + ps.Composure*0.20 + ps.Experience*0.15
	threatLoad *= clamp01(1.35-resilience) + 0.45
	supportGain *= 0.70 + clamp01(resilience)*0.55

	delta := (supportGain - threatLoad) * dt
	ps.Morale = clamp01(ps.Morale + delta)

	if ps.Morale < 0.25 && (ctx.UnderFire || ctx.SuppressLevel > 0.4) {
		ps.ApplyStress((0.25 - ps.Morale) * 0.020 * dt)
	}
	if ps.Morale > 0.75 && !ctx.UnderFire && ctx.VisibleThreats == 0 {
		ps.RecoverFear(0.20 * dt)
	}
}

// --- Soldier Profile (immutable template + mutable state) ---

// SoldierProfile bundles all stats for a single soldier.
type SoldierProfile struct {
	Physical PhysicalStats
	Skills   SkillStats
	Psych    PsychState
	Stance   Stance
}

// EffectiveSpeed returns the current movement speed in pixels/tick,
// factoring in stance, fitness, and fatigue.
func (sp *SoldierProfile) EffectiveSpeed(baseSpeed float64) float64 {
	stanceMul := sp.Stance.Profile().SpeedMul
	fitnessMul := 0.6 + 0.4*sp.Physical.EffectiveFitness() // floor at 60% speed
	fearPenalty := 1.0 - sp.Psych.EffectiveFear()*0.3      // fear slows deliberate movement
	return baseSpeed * stanceMul * fitnessMul * fearPenalty
}

// EffectiveAccuracy returns a 0-1 accuracy score.
// An optional suppressLevel (0-1) can be passed to apply suppression degradation.
// Call with no arguments for the base accuracy (ignores suppression).
func (sp *SoldierProfile) EffectiveAccuracy(suppressLevel ...float64) float64 {
	base := sp.Skills.Marksmanship
	stanceMul := sp.Stance.Profile().AccuracyMul
	fatiguePen := 1.0 - sp.Physical.Fatigue*0.4
	fearPen := 1.0 - sp.Psych.EffectiveFear()*0.5
	acc := clamp01(base * stanceMul * fatiguePen * fearPen)
	if len(suppressLevel) > 0 && suppressLevel[0] > 0 {
		// Suppression degrades accuracy: at suppress=1.0 a baseline soldier
		// loses ~40% accuracy. Discipline resists — veterans hold their shots.
		pen := suppressLevel[0] * (0.40 - sp.Skills.Discipline*0.20)
		acc = clamp01(acc * (1.0 - pen))
	}
	return acc
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
