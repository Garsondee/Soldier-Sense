package game

import "math"

// executeHelpCasualty implements the GoalHelpCasualty behavior:
// navigate to nearest wounded squad member and provide medical aid.
func (s *Soldier) executeHelpCasualty(dt float64) {
	if !s.canProvideCare() {
		s.state = SoldierStateIdle
		return
	}

	// Find nearest casualty needing aid.
	casualty := s.findNearestCasualty()
	if casualty == nil {
		// No casualties to help - idle.
		s.state = SoldierStateIdle
		s.faceNearestThreatOrContact()
		return
	}

	dx := casualty.x - s.x
	dy := casualty.y - s.y
	dist := math.Sqrt(dx*dx + dy*dy)

	// If close enough, provide aid.
	if dist < 35.0 {
		s.state = SoldierStateIdle
		s.requestStance(StanceCrouching, false)

		// Check if already providing aid to this casualty.
		alreadyProviding := false
		for _, p := range casualty.casualty.Providers {
			if p == s {
				alreadyProviding = true
				break
			}
		}

		if !alreadyProviding {
			s.startProvidingAid(casualty, s.tickVal())
		}

		// Face the casualty.
		heading := math.Atan2(dy, dx)
		s.vision.UpdateHeading(heading, turnRate)
		return
	}

	// Navigate to casualty.
	s.state = SoldierStateMoving
	s.requestStance(StanceCrouching, false)

	// Ensure we keep redirecting toward the casualty even if we had a prior path
	// from another goal (advance/formation/etc).
	if s.navGrid != nil {
		tick := s.tickVal()
		if s.path == nil || s.pathIndex >= len(s.path) || tick%15 == 0 {
			s.path = s.navGrid.FindPath(s.x, s.y, casualty.x, casualty.y)
			s.pathIndex = 0
		}
	}

	s.moveAlongPath(dt)
}

// integrateWoundedSelfAid runs self-aid attempts for wounded soldiers.
// Called from the soldier's Update loop.
func (s *Soldier) integrateWoundedSelfAid() {
	// Only attempt self-aid if wounded and can act.
	if !s.body.IsInjured() {
		return
	}
	if s.state == SoldierStateDead || s.state.IsIncapacitated() {
		return
	}

	// If already treating self, tick the treatment.
	if s.casualty.SelfAidActive {
		s.tickSelfAid(s.tickVal())
		return
	}

	// Attempt to start self-aid if not under heavy fire.
	bb := &s.blackboard
	if bb.IsSuppressed() || bb.IncomingFireCount > 0 {
		return // too dangerous to self-aid
	}

	// Try to apply tourniquet to worst limb bleed.
	s.attemptSelfAid(s.tickVal())
}

// integrateBuddyAidTick advances treatment being provided to casualties.
// Called from the game loop after soldier updates.
func integrateBuddyAidTick(soldiers []*Soldier, tick int) {
	for _, s := range soldiers {
		if s.state == SoldierStateDead {
			continue
		}
		if !s.body.IsInjured() {
			continue
		}

		// Tick any active treatment.
		tickProvidedAid(s, tick)

		// Check if providers should stop (under fire, etc).
		for i := len(s.casualty.Providers) - 1; i >= 0; i-- {
			provider := s.casualty.Providers[i]
			if provider.state == SoldierStateDead || provider.state.IsIncapacitated() {
				stopProvidingAid(provider, s)
				continue
			}
			// Stop if provider is under heavy fire.
			if provider.blackboard.IsSuppressed() || provider.blackboard.IncomingFireCount > 0 {
				stopProvidingAid(provider, s)
				provider.think("stopping aid - under fire")
			}
		}
	}
}
