package game

import "math"

// SteeringBehavior computes movement forces for a soldier using flow fields
type SteeringBehavior struct {
	soldier *Soldier

	// Behavior weights
	flowWeight       float64
	separationWeight float64
	cohesionWeight   float64

	// Parameters
	separationRadius float64
	cohesionRadius   float64
	maxSpeed         float64

	// Unstuck tracking
	stuckTicks   int
	lastX, lastY float64
}

func NewSteeringBehavior(soldier *Soldier) *SteeringBehavior {
	return &SteeringBehavior{
		soldier:          soldier,
		flowWeight:       0.6,
		separationWeight: 0.3,
		cohesionWeight:   0.1,
		separationRadius: 30.0,
		cohesionRadius:   100.0,
		maxSpeed:         0.0, // Will be set based on stance
	}
}

// ComputeDesiredVelocity calculates the desired movement vector
func (sb *SteeringBehavior) ComputeDesiredVelocity(flowController *SquadFlowController, tacticalWeight float64) Vec2 {
	if flowController == nil {
		return Vec2{0, 0}
	}

	// Get blended flow from hierarchical layers
	flow := flowController.GetBlendedFlow(sb.soldier.x, sb.soldier.y, tacticalWeight)
	flowForce := flow.Scale(sb.flowWeight)

	// Compute separation force (avoid nearby soldiers)
	separationForce := sb.computeSeparation()
	separationForce = separationForce.Scale(sb.separationWeight)

	// Compute cohesion force (stay near squad)
	cohesionForce := sb.computeCohesion()
	cohesionForce = cohesionForce.Scale(sb.cohesionWeight)

	// Combine all forces
	desired := flowForce.Add(separationForce).Add(cohesionForce)

	// Apply obstacle avoidance if needed
	if obstacle := sb.detectImmediateObstacle(desired); obstacle != nil {
		desired = sb.avoidObstacle(obstacle, desired)
	}

	return desired
}

// computeSeparation pushes soldier away from nearby teammates
func (sb *SteeringBehavior) computeSeparation() Vec2 {
	if sb.soldier.squad == nil {
		return Vec2{0, 0}
	}

	force := Vec2{0, 0}

	for _, other := range sb.soldier.squad.Members {
		if other == sb.soldier || other.state == SoldierStateDead {
			continue
		}

		dx := sb.soldier.x - other.x
		dy := sb.soldier.y - other.y
		distSq := dx*dx + dy*dy
		dist := math.Sqrt(distSq)

		if dist < sb.separationRadius && dist > 0.1 {
			// Push away from nearby soldiers
			// Strength inversely proportional to distance
			strength := (sb.separationRadius - dist) / sb.separationRadius
			away := Vec2{dx / dist, dy / dist}
			force = force.Add(away.Scale(strength))
		}
	}

	return force
}

// computeCohesion pulls soldier toward squad center
func (sb *SteeringBehavior) computeCohesion() Vec2 {
	if sb.soldier.squad == nil {
		return Vec2{0, 0}
	}

	center := Vec2{0, 0}
	count := 0

	for _, other := range sb.soldier.squad.Members {
		if other == sb.soldier || other.state == SoldierStateDead {
			continue
		}

		dx := other.x - sb.soldier.x
		dy := other.y - sb.soldier.y
		dist := math.Sqrt(dx*dx + dy*dy)

		if dist < sb.cohesionRadius {
			center.X += other.x
			center.Y += other.y
			count++
		}
	}

	if count == 0 {
		return Vec2{0, 0}
	}

	center.X /= float64(count)
	center.Y /= float64(count)

	toward := Vec2{center.X - sb.soldier.x, center.Y - sb.soldier.y}
	if toward.Length() > 0.1 {
		return toward.Normalize()
	}
	return Vec2{0, 0}
}

// detectImmediateObstacle checks if movement will collide with obstacle
func (sb *SteeringBehavior) detectImmediateObstacle(velocity Vec2) *Vec2 {
	if velocity.Length() < 0.1 {
		return nil
	}

	// Check a few steps ahead
	lookAhead := 20.0 // pixels
	dir := velocity.Normalize()
	checkX := sb.soldier.x + dir.X*lookAhead
	checkY := sb.soldier.y + dir.Y*lookAhead

	// Check if that position is blocked
	cellX := int(checkX / cellSize)
	cellY := int(checkY / cellSize)

	if sb.soldier.navGrid != nil && sb.soldier.navGrid.IsBlocked(cellX, cellY) {
		return &Vec2{checkX, checkY}
	}

	return nil
}

// avoidObstacle adjusts velocity to slide along obstacle
func (sb *SteeringBehavior) avoidObstacle(obstacle *Vec2, velocity Vec2) Vec2 {
	// Compute vector to obstacle
	toObstacle := Vec2{obstacle.X - sb.soldier.x, obstacle.Y - sb.soldier.y}

	// Compute perpendicular vector (slide direction)
	perp := Vec2{-toObstacle.Y, toObstacle.X}
	perp = perp.Normalize()

	// Project velocity onto perpendicular
	dot := velocity.X*perp.X + velocity.Y*perp.Y
	slide := perp.Scale(dot)

	// If slide is too small, try opposite direction
	if slide.Length() < 0.1 {
		perp = Vec2{toObstacle.Y, -toObstacle.X}
		perp = perp.Normalize()
		dot = velocity.X*perp.X + velocity.Y*perp.Y
		slide = perp.Scale(dot)
	}

	return slide
}

// ApplyMovement updates soldier position based on desired velocity
func (sb *SteeringBehavior) ApplyMovement(desired Vec2, dt float64) {
	// Get max speed based on stance and state
	sb.maxSpeed = sb.getMaxSpeed()

	// Limit to max speed
	if desired.Length() > sb.maxSpeed {
		desired = desired.Normalize().Scale(sb.maxSpeed)
	}

	// Apply movement
	oldX, oldY := sb.soldier.x, sb.soldier.y
	sb.soldier.x += desired.X * dt
	sb.soldier.y += desired.Y * dt

	// Check if actually moved
	moved := math.Abs(sb.soldier.x-oldX) > 0.1 || math.Abs(sb.soldier.y-oldY) > 0.1

	if !moved {
		sb.stuckTicks++
		if sb.stuckTicks > 60 {
			sb.applyUnstuckBehavior()
		}
	} else {
		sb.stuckTicks = 0
		sb.lastX = sb.soldier.x
		sb.lastY = sb.soldier.y
	}

	// Update facing direction if moving
	if desired.Length() > 0.1 {
		targetHeading := math.Atan2(desired.Y, desired.X)
		sb.soldier.vision.UpdateHeading(targetHeading, 0.15) // Smooth turn rate
	}
}

// getMaxSpeed returns max speed based on soldier state and stance
func (sb *SteeringBehavior) getMaxSpeed() float64 {
	// Base speeds (pixels per second)
	baseSpeed := 100.0

	switch sb.soldier.profile.Stance {
	case StanceProne:
		baseSpeed = 20.0
	case StanceCrouching:
		baseSpeed = 60.0
	case StanceStanding:
		baseSpeed = 100.0
	}

	// Reduce speed if suppressed
	if sb.soldier.blackboard.IsSuppressed() {
		baseSpeed *= 0.5
	}

	// Reduce speed if wounded
	mobilityMul := sb.soldier.body.MobilityMul()
	baseSpeed *= mobilityMul

	return baseSpeed
}

// applyUnstuckBehavior attempts to free a stuck soldier
func (sb *SteeringBehavior) applyUnstuckBehavior() {
	// Apply random jitter using soldier's deterministic RNG
	jitterX := (sb.soldier.psychRoll(sb.stuckTicks*7) - 0.5) * 40.0
	jitterY := (sb.soldier.psychRoll(sb.stuckTicks*11) - 0.5) * 40.0

	sb.soldier.x += jitterX
	sb.soldier.y += jitterY

	// Reset stuck counter
	sb.stuckTicks = 0

	// Mark flow field as dirty to recompute
	if sb.soldier.squad != nil && sb.soldier.squad.flowController != nil {
		sb.soldier.squad.flowController.strategicDirty = true
		sb.soldier.squad.flowController.tacticalDirty = true
	}
}

// GetTacticalWeight determines how much to blend tactical vs strategic flow
// Returns 0.0 for pure strategic, 1.0 for pure tactical
func (sb *SteeringBehavior) GetTacticalWeight() float64 {
	if sb.soldier.squad == nil || sb.soldier.squad.flowController == nil {
		return 0.0
	}

	// Check distance to strategic goal
	strategicDist := sb.soldier.squad.flowController.GetDistanceToStrategicGoal(sb.soldier.x, sb.soldier.y)

	// If far from goal, use strategic flow
	if strategicDist > 200.0 {
		return 0.0
	}

	// If close to goal, blend in tactical
	if strategicDist < 50.0 {
		return 1.0
	}

	// Blend based on distance
	weight := 1.0 - (strategicDist-50.0)/150.0
	return weight
}
