package game

// Feature flag for flow-field movement system
// Set to true to enable flow-field navigation, false to use A* pathfinding
const useFlowFieldMovement = true

// moveWithFlowField uses the flow-field navigation system for movement
// This is the new movement method that replaces A* pathfinding
func (s *Soldier) moveWithFlowField(dt float64) {
	if !useFlowFieldMovement || s.squad == nil || s.squad.flowController == nil || s.steeringBehavior == nil {
		// Fallback to existing movement if flow-field not enabled or not initialized
		s.moveAlongPath(dt)
		return
	}

	// Determine tactical weight (how much to blend tactical vs strategic flow)
	tacticalWeight := s.steeringBehavior.GetTacticalWeight()

	// Compute desired velocity from steering behaviors
	desired := s.steeringBehavior.ComputeDesiredVelocity(s.squad.flowController, tacticalWeight)

	// Apply movement
	s.steeringBehavior.ApplyMovement(desired, dt)
}

// updateSquadFlowFieldGoals updates the squad's strategic goal based on current objective
func (s *Soldier) updateSquadFlowFieldGoals() {
	if s.squad == nil || s.squad.flowController == nil || !s.isLeader {
		return
	}

	// Set strategic goal based on squad intent and blackboard
	bb := &s.blackboard

	// Strategic goal: where the squad is trying to go
	if bb.HasMoveOrder {
		// Officer order takes priority
		s.squad.flowController.SetStrategicGoal(bb.OrderMoveX, bb.OrderMoveY)
	} else if bb.SquadHasContact {
		// Move toward contact
		s.squad.flowController.SetStrategicGoal(bb.SquadContactX, bb.SquadContactY)
	} else if bb.HeardGunfire {
		// Move toward gunfire
		s.squad.flowController.SetStrategicGoal(bb.HeardGunfireX, bb.HeardGunfireY)
	} else {
		// Default: advance toward end target
		s.squad.flowController.SetStrategicGoal(s.endTarget[0], s.endTarget[1])
	}

	// Tactical goal: formation positioning
	// This will be set by formation update logic
}
