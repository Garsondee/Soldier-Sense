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
func (sp *SoldierProfile) EffectiveAccuracy() float64 {
	base := sp.Skills.Marksmanship
	stanceMul := sp.Stance.Profile().AccuracyMul
	fatiguePen := 1.0 - sp.Physical.Fatigue*0.4
	fearPen := 1.0 - sp.Psych.EffectiveFear()*0.5
	return clamp01(base * stanceMul * fatiguePen * fearPen)
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
