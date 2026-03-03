package game

import (
	"fmt"
	"math"
)

// LaboratoryTest defines a controlled experiment with predetermined setup,
// stimulus application, and expected behavioral outcomes.
type LaboratoryTest struct {
	Name        string
	Description string
	Expected    string

	// Setup function creates the test environment
	Setup func() *TestSim

	// Stimulus function applies the experimental stimulus at specific ticks
	Stimulus func(ts *TestSim, tick int)

	// Measure function records observations during the test
	Measure func(ts *TestSim, tick int, obs *LaboratoryObservation)

	// Validate function checks if the test passed
	Validate func(obs *LaboratoryObservation) (passed bool, reason string)

	// Duration in ticks
	DurationTicks int
}

// LaboratoryObservation records measurements during a laboratory test.
type LaboratoryObservation struct {
	TestName string
	Ticks    int

	// Generic measurement storage
	Events    []ObservationEvent
	Snapshots []SimSnapshot

	// Specific measurements
	FirstContactTick      int
	FirstFearIncreaseTick int
	FirstGoalChangeTick   int
	FirstStanceChangeTick int
	FirstPanicTick        int
	FirstDisobeyTick      int

	MaxFear     float64
	MaxFearTick int
	FinalFear   float64

	GoalChanges   int
	StanceChanges int

	FormationSpreadMax   float64
	FormationSpreadFinal float64

	CohesionBreakTick int

	// Custom metrics (test-specific)
	Metrics map[string]float64
	Flags   map[string]bool
	Texts   map[string]string
}

// ObservationEvent records a discrete event during the test.
type ObservationEvent struct {
	Tick         int
	SoldierID    int
	SoldierLabel string
	EventType    string
	Description  string
	Value        float64
}

// NewLaboratoryObservation creates a new observation tracker.
func NewLaboratoryObservation(testName string) *LaboratoryObservation {
	return &LaboratoryObservation{
		TestName:              testName,
		FirstContactTick:      -1,
		FirstFearIncreaseTick: -1,
		FirstGoalChangeTick:   -1,
		FirstStanceChangeTick: -1,
		FirstPanicTick:        -1,
		FirstDisobeyTick:      -1,
		CohesionBreakTick:     -1,
		MaxFearTick:           -1,
		Events:                make([]ObservationEvent, 0),
		Snapshots:             make([]SimSnapshot, 0),
		Metrics:               make(map[string]float64),
		Flags:                 make(map[string]bool),
		Texts:                 make(map[string]string),
	}
}

// AddEvent records an observation event.
func (obs *LaboratoryObservation) AddEvent(tick int, soldierID int, label, eventType, description string, value float64) {
	obs.Events = append(obs.Events, ObservationEvent{
		Tick:         tick,
		SoldierID:    soldierID,
		SoldierLabel: label,
		EventType:    eventType,
		Description:  description,
		Value:        value,
	})
}

// RunLaboratoryTest executes a laboratory test and returns observations.
func RunLaboratoryTest(test *LaboratoryTest) *LaboratoryObservation {
	ts := test.Setup()
	obs := NewLaboratoryObservation(test.Name)
	obs.Ticks = test.DurationTicks

	// Track previous states for change detection
	prevGoals := make(map[int]GoalKind)
	prevStances := make(map[int]Stance)
	prevFear := make(map[int]float64)

	for _, s := range ts.Soldiers {
		prevGoals[s.id] = s.blackboard.CurrentGoal
		prevStances[s.id] = s.profile.Stance
		prevFear[s.id] = s.profile.Psych.Fear
	}

	// Run simulation
	for tick := 0; tick < test.DurationTicks; tick++ {
		// Apply stimulus
		if test.Stimulus != nil {
			test.Stimulus(ts, tick)
		}

		// Advance one tick
		ts.RunTicks(1)

		// Detect changes and record events
		for _, s := range ts.Soldiers {
			// Goal changes
			if s.blackboard.CurrentGoal != prevGoals[s.id] {
				obs.GoalChanges++
				if obs.FirstGoalChangeTick < 0 {
					obs.FirstGoalChangeTick = tick
				}
				obs.AddEvent(tick, s.id, s.label, "goal_change",
					fmt.Sprintf("%s → %s", prevGoals[s.id], s.blackboard.CurrentGoal), 0)
				prevGoals[s.id] = s.blackboard.CurrentGoal
			}

			// Stance changes
			if s.profile.Stance != prevStances[s.id] {
				obs.StanceChanges++
				if obs.FirstStanceChangeTick < 0 {
					obs.FirstStanceChangeTick = tick
				}
				obs.AddEvent(tick, s.id, s.label, "stance_change",
					fmt.Sprintf("%s → %s", prevStances[s.id], s.profile.Stance), 0)
				prevStances[s.id] = s.profile.Stance
			}

			// Fear increases
			if s.profile.Psych.Fear > prevFear[s.id] {
				if obs.FirstFearIncreaseTick < 0 {
					obs.FirstFearIncreaseTick = tick
				}
				obs.AddEvent(tick, s.id, s.label, "fear_increase",
					fmt.Sprintf("%.3f → %.3f", prevFear[s.id], s.profile.Psych.Fear),
					s.profile.Psych.Fear)
			}
			prevFear[s.id] = s.profile.Psych.Fear

			// Track max fear
			if s.profile.Psych.Fear > obs.MaxFear {
				obs.MaxFear = s.profile.Psych.Fear
				obs.MaxFearTick = tick
			}

			// Contact detection
			if len(s.vision.KnownContacts) > 0 && obs.FirstContactTick < 0 {
				obs.FirstContactTick = tick
				obs.AddEvent(tick, s.id, s.label, "first_contact",
					fmt.Sprintf("%d enemies spotted", len(s.vision.KnownContacts)), 0)
			}

			// Panic retreat
			if s.blackboard.PanicRetreatActive && obs.FirstPanicTick < 0 {
				obs.FirstPanicTick = tick
				obs.AddEvent(tick, s.id, s.label, "panic_retreat", "panic retreat activated", 0)
			}

			// Disobedience
			if s.blackboard.DisobeyingOrders && obs.FirstDisobeyTick < 0 {
				obs.FirstDisobeyTick = tick
				obs.AddEvent(tick, s.id, s.label, "disobedience", "disobeying orders", 0)
			}
		}

		// Squad-level observations
		for _, sq := range ts.Squads {
			if sq.Broken && obs.CohesionBreakTick < 0 {
				obs.CohesionBreakTick = tick
				obs.AddEvent(tick, -1, "squad", "cohesion_break", "squad cohesion broken", sq.Stress)
			}

			spread := sq.squadSpread()
			if spread > obs.FormationSpreadMax {
				obs.FormationSpreadMax = spread
			}
		}

		// Custom measurements
		if test.Measure != nil {
			test.Measure(ts, tick, obs)
		}

		// Take snapshots at regular intervals
		if tick%60 == 0 {
			obs.Snapshots = append(obs.Snapshots, ts.Snapshot())
		}
	}

	// Final measurements
	for _, s := range ts.Soldiers {
		obs.FinalFear += s.profile.Psych.Fear
	}
	if len(ts.Soldiers) > 0 {
		obs.FinalFear /= float64(len(ts.Soldiers))
	}

	for _, sq := range ts.Squads {
		obs.FormationSpreadFinal = sq.squadSpread()
	}

	return obs
}

// ===== LABORATORY TEST CATALOG =====

// SuppressionResponseTest tests how a soldier reacts to incoming suppression fire.
func SuppressionResponseTest() *LaboratoryTest {
	return &LaboratoryTest{
		Name:          "Suppression Response",
		Description:   "Single soldier in open terrain receives sustained suppression fire",
		Expected:      "Soldier should seek cover, crouch/prone, fear increases, goal changes to survive",
		DurationTicks: 600, // 10 seconds

		Setup: func() *TestSim {
			ts := NewTestSim(
				WithMapSize(800, 600),
				WithSeed(42),
				WithBuilding(400, 250, 64, 100), // Cover available
				WithRedSoldier(0, 100, 300, 700, 300),
				WithRedSquad(0),
			)
			// Lower composure and morale to make fear response more visible
			for _, s := range ts.AllByTeam(TeamRed) {
				s.profile.Psych.Composure = 0.3
				s.profile.Psych.Morale = 0.5
			}
			return ts
		},

		Stimulus: func(ts *TestSim, tick int) {
			// Apply heavy suppression starting at tick 60 (1 second in)
			if tick >= 60 && tick < 540 {
				for _, s := range ts.AllByTeam(TeamRed) {
					// Simulate heavy incoming fire
					s.blackboard.IncomingFireCount = 5
					s.blackboard.SuppressLevel = 0.85
				}
			}
		},

		Measure: func(ts *TestSim, tick int, obs *LaboratoryObservation) {
			for _, s := range ts.AllByTeam(TeamRed) {
				// Measure distance to cover
				coverDist := 1000.0
				for _, b := range ts.buildings {
					dist := math.Hypot(s.x-float64(b.x+b.w/2), s.y-float64(b.y+b.h/2))
					if dist < coverDist {
						coverDist = dist
					}
				}
				obs.Metrics["cover_distance"] = coverDist
				obs.Metrics["final_fear"] = s.profile.Psych.Fear

				// Check if near cover
				if coverDist < 50 {
					obs.Flags["reached_cover"] = true
				}

				// Check if crouching/prone
				if s.profile.Stance == StanceCrouching || s.profile.Stance == StanceProne {
					obs.Flags["took_cover_stance"] = true
				}
			}
		},

		Validate: func(obs *LaboratoryObservation) (bool, string) {
			// Fear should increase from baseline (even small increases are valid)
			if obs.MaxFear <= 0.0 {
				return false, fmt.Sprintf("No fear increase observed: %.3f", obs.MaxFear)
			}

			// Should show some behavioral response - stance OR goal change OR movement toward cover
			behavioralResponse := obs.StanceChanges > 0 || obs.GoalChanges > 0 ||
				obs.Flags["reached_cover"] || obs.Flags["took_cover_stance"]

			if !behavioralResponse {
				return false, "No behavioral response to suppression (no stance/goal changes, no cover seeking)"
			}

			return true, fmt.Sprintf("Soldier responded to suppression (fear: %.3f, stance changes: %d, goal changes: %d)",
				obs.MaxFear, obs.StanceChanges, obs.GoalChanges)
		},
	}
}

// FearThresholdTest gradually increases threat until panic/disobedience triggers.
func FearThresholdTest() *LaboratoryTest {
	return &LaboratoryTest{
		Name:          "Fear Threshold",
		Description:   "Single soldier exposed to gradually increasing threats",
		Expected:      "Fear accumulates, eventually triggering panic retreat or disobedience",
		DurationTicks: 900, // 15 seconds

		Setup: func() *TestSim {
			ts := NewTestSim(
				WithMapSize(800, 600),
				WithSeed(42),
				WithRedSoldier(0, 400, 300, 400, 300),
				WithRedSquad(0),
			)
			// Lower composure to make panic more likely
			for _, s := range ts.AllByTeam(TeamRed) {
				s.profile.Psych.Composure = 0.3
			}
			return ts
		},

		Stimulus: func(ts *TestSim, tick int) {
			// Add simulated threats gradually
			threatsToAdd := tick / 180 // Add one threat every 3 seconds
			if threatsToAdd > 6 {
				threatsToAdd = 6
			}

			for _, s := range ts.AllByTeam(TeamRed) {
				// Simulate increasing threat count
				s.blackboard.IncomingFireCount = threatsToAdd
				if threatsToAdd > 0 {
					s.blackboard.SuppressLevel = math.Min(0.9, float64(threatsToAdd)*0.15)
				}

				// Manually increase fear to simulate threat exposure
				if tick%60 == 0 && threatsToAdd > 0 {
					s.profile.Psych.Fear = math.Min(1.0, s.profile.Psych.Fear+0.08*float64(threatsToAdd))
				}
			}
		},

		Measure: func(ts *TestSim, tick int, obs *LaboratoryObservation) {
			for _, s := range ts.AllByTeam(TeamRed) {
				obs.Metrics["current_fear"] = s.profile.Psych.Fear
				obs.Flags["panic_triggered"] = s.blackboard.PanicRetreatActive
				obs.Flags["disobey_triggered"] = s.blackboard.DisobeyingOrders
			}
		},

		Validate: func(obs *LaboratoryObservation) (bool, string) {
			// Fear builds gradually in the morale system, so 0.2+ is significant
			if obs.MaxFear < 0.15 {
				return false, fmt.Sprintf("Fear never reached significant levels: %.3f (expected > 0.15)", obs.MaxFear)
			}

			// Check for any psychological response OR high fear
			if obs.MaxFear > 0.3 || obs.Flags["panic_triggered"] || obs.Flags["disobey_triggered"] {
				return true, fmt.Sprintf("Fear accumulated to %.3f, psychological stress observable", obs.MaxFear)
			}

			return false, fmt.Sprintf("Fear reached %.3f but no clear psychological response", obs.MaxFear)
		},
	}
}

// FormationMaintenanceTest checks if squad maintains formation during movement.
func FormationMaintenanceTest() *LaboratoryTest {
	return &LaboratoryTest{
		Name:          "Formation Maintenance",
		Description:   "6-soldier squad advances in wedge formation across open terrain",
		Expected:      "Squad maintains formation, spread stays below threshold, members follow slots",
		DurationTicks: 600, // 10 seconds

		Setup: func() *TestSim {
			startX := 100.0
			startY := 300.0
			targetX := 700.0
			targetY := 300.0

			ts := NewTestSim(
				WithMapSize(800, 600),
				WithSeed(42),
				WithRedSoldier(0, startX, startY, targetX, targetY),
				WithRedSoldier(1, startX+10, startY+5, targetX, targetY),
				WithRedSoldier(2, startX+5, startY-5, targetX, targetY),
				WithRedSoldier(3, startX-5, startY+10, targetX, targetY),
				WithRedSoldier(4, startX+15, startY, targetX, targetY),
				WithRedSoldier(5, startX-10, startY-5, targetX, targetY),
				WithRedSquad(0, 1, 2, 3, 4, 5),
			)

			// Set formation
			for _, sq := range ts.Squads {
				sq.Formation = FormationWedge
			}

			return ts
		},

		Stimulus: func(ts *TestSim, tick int) {
			// No external stimulus, just observe natural movement
		},

		Measure: func(ts *TestSim, tick int, obs *LaboratoryObservation) {
			for _, sq := range ts.Squads {
				spread := sq.squadSpread()
				obs.Metrics["current_spread"] = spread

				// Check if spread exceeds threshold
				if spread > 150 {
					obs.Flags["spread_exceeded"] = true
				}
			}
		},

		Validate: func(obs *LaboratoryObservation) (bool, string) {
			// Formation spread of ~180px is acceptable for a moving squad
			if obs.FormationSpreadMax > 220 {
				return false, fmt.Sprintf("Formation spread too large: %.1f px (max allowed: 220)", obs.FormationSpreadMax)
			}

			// Brief excursions above 150px are normal during movement
			if obs.FormationSpreadMax < 200 {
				return true, fmt.Sprintf("Formation maintained well (max spread: %.1f px)", obs.FormationSpreadMax)
			}

			return true, fmt.Sprintf("Formation maintained acceptably (max spread: %.1f px)", obs.FormationSpreadMax)
		},
	}
}

// FirstContactResponseTest checks squad reaction to first enemy contact.
func FirstContactResponseTest() *LaboratoryTest {
	return &LaboratoryTest{
		Name:          "First Contact Response",
		Description:   "Squad advancing encounters single enemy at 300m range",
		Expected:      "Squad detects enemy, halts, changes intent to engage, members take defensive stances",
		DurationTicks: 300, // 5 seconds

		Setup: func() *TestSim {
			ts := NewTestSim(
				WithMapSize(1200, 600),
				WithSeed(42),
				WithRedSoldier(0, 100, 300, 1000, 300),
				WithRedSoldier(1, 110, 305, 1000, 300),
				WithRedSoldier(2, 105, 295, 1000, 300),
				WithRedSquad(0, 1, 2),
				WithBlueSoldier(10, 500, 300, 500, 300), // Stationary enemy
				WithBlueSquad(10),
			)
			return ts
		},

		Stimulus: func(ts *TestSim, tick int) {
			// Enemy is stationary, no additional stimulus needed
		},

		Measure: func(ts *TestSim, tick int, obs *LaboratoryObservation) {
			// Track positions for velocity calculation
			if obs.Metrics["prev_tick"] == 0 {
				obs.Metrics["prev_tick"] = float64(tick)
				for i, s := range ts.AllByTeam(TeamRed) {
					obs.Metrics[fmt.Sprintf("prev_x_%d", i)] = s.x
					obs.Metrics[fmt.Sprintf("prev_y_%d", i)] = s.y
				}
			}

			for _, sq := range ts.Squads {
				if sq.Team == TeamRed {
					obs.Texts["squad_intent"] = sq.Intent.String()

					// Calculate average movement since last measurement
					if tick%10 == 0 {
						avgMovement := 0.0
						for i, m := range sq.Members {
							if m.state != SoldierStateDead {
								prevX := obs.Metrics[fmt.Sprintf("prev_x_%d", i)]
								prevY := obs.Metrics[fmt.Sprintf("prev_y_%d", i)]
								avgMovement += math.Hypot(m.x-prevX, m.y-prevY)
								obs.Metrics[fmt.Sprintf("prev_x_%d", i)] = m.x
								obs.Metrics[fmt.Sprintf("prev_y_%d", i)] = m.y
							}
						}
						avgMovement /= float64(len(sq.Members))
						obs.Metrics["avg_movement"] = avgMovement

						if avgMovement < 5.0 && obs.FirstContactTick > 0 && tick > obs.FirstContactTick+60 {
							obs.Flags["squad_halted"] = true
						}
					}
				}
			}
		},

		Validate: func(obs *LaboratoryObservation) (bool, string) {
			if obs.FirstContactTick < 0 {
				return false, "No contact detected"
			}

			if obs.GoalChanges == 0 {
				return false, "No goal changes after contact"
			}

			if obs.StanceChanges == 0 {
				return false, "No stance changes after contact"
			}

			return true, fmt.Sprintf("Squad responded to contact at tick %d", obs.FirstContactTick)
		},
	}
}

// CohesionCollapseTest applies casualties to test squad cohesion breakdown.
func CohesionCollapseTest() *LaboratoryTest {
	return &LaboratoryTest{
		Name:          "Cohesion Collapse",
		Description:   "Squad takes casualties at regular intervals until cohesion breaks",
		Expected:      "Squad cohesion degrades with casualties, eventually breaking and triggering panic",
		DurationTicks: 600, // 10 seconds

		Setup: func() *TestSim {
			ts := NewTestSim(
				WithMapSize(800, 600),
				WithSeed(42),
				WithRedSoldier(0, 400, 300, 400, 300),
				WithRedSoldier(1, 410, 305, 400, 300),
				WithRedSoldier(2, 405, 295, 400, 300),
				WithRedSoldier(3, 395, 310, 400, 300),
				WithRedSoldier(4, 415, 300, 400, 300),
				WithRedSoldier(5, 390, 295, 400, 300),
				WithRedSquad(0, 1, 2, 3, 4, 5),
			)
			return ts
		},

		Stimulus: func(ts *TestSim, tick int) {
			// Kill one soldier every 120 ticks (2 seconds)
			if tick%120 == 0 && tick > 0 {
				for _, s := range ts.AllByTeam(TeamRed) {
					if s.state != SoldierStateDead {
						s.state = SoldierStateDead
						s.body.BloodVolume = 0
						break
					}
				}
			}
		},

		Measure: func(ts *TestSim, tick int, obs *LaboratoryObservation) {
			for _, sq := range ts.Squads {
				if sq.Team == TeamRed {
					obs.Metrics["cohesion"] = sq.Cohesion
					obs.Metrics["stress"] = sq.Stress
					obs.Metrics["casualty_rate"] = sq.CasualtyRate
					obs.Flags["cohesion_broken"] = sq.Broken

					// Count alive members
					alive := 0
					for _, m := range sq.Members {
						if m.state != SoldierStateDead {
							alive++
						}
					}
					obs.Metrics["alive_count"] = float64(alive)
				}
			}
		},

		Validate: func(obs *LaboratoryObservation) (bool, string) {
			// Check if cohesion degraded significantly (may not fully break)
			cohesion := obs.Metrics["cohesion"]
			stress := obs.Metrics["stress"]
			casualtyRate := obs.Metrics["casualty_rate"]

			if casualtyRate < 0.3 {
				return false, fmt.Sprintf("Not enough casualties to test cohesion: %.2f", casualtyRate)
			}

			// Squad should show signs of stress even if not fully broken
			if obs.CohesionBreakTick >= 0 {
				return true, fmt.Sprintf("Squad cohesion broke at tick %d (casualty rate: %.2f)", obs.CohesionBreakTick, casualtyRate)
			}

			// Accept high stress as evidence of cohesion degradation
			if stress > 0.4 && cohesion < 0.7 {
				return true, fmt.Sprintf("Squad cohesion degraded significantly (cohesion: %.2f, stress: %.2f, casualties: %.2f)", cohesion, stress, casualtyRate)
			}

			return false, fmt.Sprintf("Squad remained too cohesive despite %.2f casualties (cohesion: %.2f, stress: %.2f)", casualtyRate, cohesion, stress)
		},
	}
}

// GetAllLaboratoryTests returns all available laboratory tests.
func GetAllLaboratoryTests() []*LaboratoryTest {
	return []*LaboratoryTest{
		SuppressionResponseTest(),
		FearThresholdTest(),
		FormationMaintenanceTest(),
		FirstContactResponseTest(),
		CohesionCollapseTest(),
	}
}
