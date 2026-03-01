package game

import (
	"math"
	"math/rand"
)

// ---------------------------------------------------------------------------
// Medical Aid System — TCCC-Informed Casualty Response
// ---------------------------------------------------------------------------

// TreatmentAction represents a specific medical intervention.
type TreatmentAction int

const (
	TreatApplyTourniquet TreatmentAction = iota
	TreatPressureDressing
	TreatPackWound
)

func (ta TreatmentAction) String() string {
	switch ta {
	case TreatApplyTourniquet:
		return "tourniquet"
	case TreatPressureDressing:
		return "pressure_dressing"
	case TreatPackWound:
		return "pack_wound"
	default:
		return "unknown"
	}
}

// TreatmentAttempt tracks an ongoing medical action.
type TreatmentAttempt struct {
	Action      TreatmentAction
	TargetWound *Wound   // which wound is being addressed
	Provider    *Soldier // who is performing the treatment
	TicksLeft   int      // countdown to completion
	Interrupted bool     // set true if fire forces pause
	SkillLevel  float64  // provider's FirstAid skill
}

// TCCCPhase represents the tactical medical response phase.
type TCCCPhase int

const (
	PhaseCUF     TCCCPhase = iota // Care Under Fire
	PhaseTFC                      // Tactical Field Care
	PhaseTACEVAC                  // Tactical Evacuation Care
)

func (p TCCCPhase) String() string {
	switch p {
	case PhaseCUF:
		return "CUF"
	case PhaseTFC:
		return "TFC"
	case PhaseTACEVAC:
		return "TACEVAC"
	default:
		return "unknown"
	}
}

// CasualtyState tracks the medical-response status for one wounded soldier.
type CasualtyState struct {
	Phase         TCCCPhase
	PhaseTick     int  // tick when current phase began
	SelfAidActive bool // casualty is treating themselves
	CurrentTreat  *TreatmentAttempt
	Providers     []*Soldier // soldiers currently rendering aid (max 2)

	BuddyAidAttempts int
	BuddyAidSuccess  int
	MedicAidAttempts int
	MedicAidSuccess  int

	BeingDragged   bool
	Dragger        *Soldier // soldier dragging this casualty
	DragTargetX    float64
	DragTargetY    float64
	ReportSent     bool // casualty report transmitted to leader
	StabilizedTick int  // tick when all critical bleeds controlled (0 = not yet)
}

// NewCasualtyState initializes casualty state in Care Under Fire phase.
func NewCasualtyState(tick int) CasualtyState {
	return CasualtyState{
		Phase:     PhaseCUF,
		PhaseTick: tick,
	}
}

// ---------------------------------------------------------------------------
// Treatment Actions
// ---------------------------------------------------------------------------

// Treatment base durations in ticks (at 60 ticks/sec).
const (
	tourniquetBaseTicks       = 30 // ~0.5s
	pressureDressingBaseTicks = 60 // ~1.0s
	packWoundBaseTicks        = 90 // ~1.5s
)

// treatmentDuration returns the actual ticks needed for a treatment action,
// modified by provider skill and stress.
func treatmentDuration(action TreatmentAction, provider *Soldier) int {
	var base int
	switch action {
	case TreatApplyTourniquet:
		base = tourniquetBaseTicks
	case TreatPressureDressing:
		base = pressureDressingBaseTicks
	case TreatPackWound:
		base = packWoundBaseTicks
	default:
		base = 60
	}

	// Skill modifier: high FirstAid reduces time.
	skillMul := 1.3 - provider.profile.Skills.FirstAid*0.5
	if skillMul < 0.5 {
		skillMul = 0.5
	}

	// Stress modifier: fear increases time.
	stressMul := 1.0 + provider.profile.Psych.EffectiveFear()*0.4

	return int(float64(base) * skillMul * stressMul)
}

// treatmentSuccessChance returns the probability of successful treatment.
func treatmentSuccessChance(provider *Soldier, targetPain float64) float64 {
	base := provider.profile.Skills.FirstAid
	painPenalty := targetPain * 0.3
	fearPenalty := provider.profile.Psych.EffectiveFear() * 0.2
	return clamp01(base - painPenalty - fearPenalty)
}

// ---------------------------------------------------------------------------
// Self-Aid
// ---------------------------------------------------------------------------

// attemptSelfAid tries to apply a tourniquet to the worst limb bleed.
// Returns true if treatment was started.
func (s *Soldier) attemptSelfAid(tick int) bool {
	if !s.body.CanSelfAid(s.state != SoldierStateUnconscious) {
		return false
	}

	// Find worst untreated limb wound.
	var worstWound *Wound
	for i := range s.body.Wounds {
		w := &s.body.Wounds[i]
		if w.Treated {
			continue
		}
		// Only limbs can have tourniquets.
		if w.Region != RegionLegLeft && w.Region != RegionLegRight &&
			w.Region != RegionArmLeft && w.Region != RegionArmRight {
			continue
		}
		if worstWound == nil || w.BleedRate > worstWound.BleedRate {
			worstWound = w
		}
	}

	if worstWound == nil {
		return false // no limb wounds to treat
	}

	// Start treatment attempt.
	s.casualty.CurrentTreat = &TreatmentAttempt{
		Action:      TreatApplyTourniquet,
		TargetWound: worstWound,
		Provider:    s,
		TicksLeft:   treatmentDuration(TreatApplyTourniquet, s),
		SkillLevel:  s.profile.Skills.FirstAid,
	}
	s.casualty.SelfAidActive = true
	s.think("self-aid: applying tourniquet")
	return true
}

// tickSelfAid advances self-aid treatment and applies on completion.
func (s *Soldier) tickSelfAid(tick int) {
	if !s.casualty.SelfAidActive || s.casualty.CurrentTreat == nil {
		return
	}

	treat := s.casualty.CurrentTreat
	treat.TicksLeft--

	if treat.TicksLeft <= 0 {
		// Treatment complete — check success.
		pain := s.body.TotalPain()
		successChance := treatmentSuccessChance(s, pain)

		if rand.Float64() < successChance {
			// Success: mark wound as treated.
			treat.TargetWound.Treated = true
			treat.TargetWound.TreatedTick = tick
			s.think("tourniquet applied successfully")
		} else {
			// Failure: can retry.
			s.think("tourniquet failed — will retry")
		}

		s.casualty.CurrentTreat = nil
		s.casualty.SelfAidActive = false
	}
}

// ---------------------------------------------------------------------------
// Buddy Aid & Medic Aid
// ---------------------------------------------------------------------------

// canProvideCare returns true if this soldier can render aid to casualties.
func (s *Soldier) canProvideCare() bool {
	if s.state == SoldierStateDead || s.state.IsIncapacitated() {
		return false
	}
	// Must have at least minimal first aid skill.
	return s.profile.Skills.FirstAid > 0.1
}

// findNearestCasualty returns the closest wounded soldier who needs aid.
func (s *Soldier) findNearestCasualty() *Soldier {
	if s.squad == nil {
		return nil
	}

	var nearest *Soldier
	minDist := math.MaxFloat64

	for _, m := range s.squad.Members {
		if m == s || m.state == SoldierStateDead {
			continue
		}
		if !m.body.IsInjured() {
			continue
		}
		// Skip if already being treated by 2 providers.
		if len(m.casualty.Providers) >= 2 {
			continue
		}
		// Skip if all wounds are treated (unless unconscious - they need monitoring).
		if !m.body.HasUntreatedWounds() && m.state != SoldierStateUnconscious {
			continue
		}

		dx := m.x - s.x
		dy := m.y - s.y
		dist := math.Sqrt(dx*dx + dy*dy)
		if dist < minDist {
			minDist = dist
			nearest = m
		}
	}

	return nearest
}

// startProvidingAid begins rendering aid to a casualty.
func (s *Soldier) startProvidingAid(casualty *Soldier, tick int) {
	// Allow retries by an existing provider even if max providers already present.
	alreadyProvider := false
	for _, p := range casualty.casualty.Providers {
		if p == s {
			alreadyProvider = true
			break
		}
	}
	if len(casualty.casualty.Providers) >= 2 && !alreadyProvider {
		return // already has max providers
	}

	// Add self as provider (no duplicates).
	if !alreadyProvider {
		casualty.casualty.Providers = append(casualty.casualty.Providers, s)
	}
	if s.isMedic {
		casualty.casualty.MedicAidAttempts++
	} else {
		casualty.casualty.BuddyAidAttempts++
	}

	// Find worst untreated wound.
	worst := casualty.body.WorstUntreatedWound()
	if worst == nil {
		return
	}

	// Determine treatment action.
	action := TreatApplyTourniquet
	if worst.Region == RegionTorso || worst.Region == RegionAbdomen {
		if s.isMedic {
			action = TreatPackWound
		} else {
			action = TreatPressureDressing
		}
	}

	// Start treatment.
	casualty.casualty.CurrentTreat = &TreatmentAttempt{
		Action:      action,
		TargetWound: worst,
		Provider:    s,
		TicksLeft:   treatmentDuration(action, s),
		SkillLevel:  s.profile.Skills.FirstAid,
	}

	s.think("providing aid: " + action.String())
}

// tickProvidedAid advances treatment being provided to a casualty.
func tickProvidedAid(casualty *Soldier, tick int) {
	if casualty.casualty.CurrentTreat == nil {
		return
	}

	treat := casualty.casualty.CurrentTreat
	treat.TicksLeft--

	if treat.TicksLeft <= 0 {
		// Treatment complete — check success.
		pain := casualty.body.TotalPain()
		successChance := treatmentSuccessChance(treat.Provider, pain)

		// Medics get a bonus for wound packing.
		if treat.Action == TreatPackWound && treat.Provider.isMedic {
			successChance = clamp01(successChance + 0.2)
		}

		if rand.Float64() < successChance {
			// Success: apply treatment effect.
			switch treat.Action {
			case TreatApplyTourniquet:
				treat.TargetWound.Treated = true
				treat.TargetWound.TreatedTick = tick
				treat.TargetWound.BleedRate = 0 // tourniquet stops bleed completely
			case TreatPressureDressing:
				treat.TargetWound.Treated = true
				treat.TargetWound.TreatedTick = tick
				treat.TargetWound.BleedRate *= 0.2 // pressure dressing reduces bleed
			case TreatPackWound:
				treat.TargetWound.Treated = true
				treat.TargetWound.TreatedTick = tick
				treat.TargetWound.BleedRate *= 0.1 // wound packing nearly stops bleed
			}
			if treat.Provider != nil {
				if treat.Provider.isMedic {
					casualty.casualty.MedicAidSuccess++
				} else {
					casualty.casualty.BuddyAidSuccess++
				}
			}
			treat.Provider.think("treatment successful: " + treat.Action.String())
		} else {
			treat.Provider.think("treatment failed — will retry")
		}

		casualty.casualty.CurrentTreat = nil
	}
}

// stopProvidingAid removes a provider from a casualty's provider list.
func stopProvidingAid(provider *Soldier, casualty *Soldier) {
	for i, p := range casualty.casualty.Providers {
		if p == provider {
			casualty.casualty.Providers = append(casualty.casualty.Providers[:i], casualty.casualty.Providers[i+1:]...)
			break
		}
	}
}

// ---------------------------------------------------------------------------
// Casualty Drag
// ---------------------------------------------------------------------------
