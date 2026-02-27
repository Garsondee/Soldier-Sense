package game

import (
	"fmt"
	"math/rand"
)

// TestSim is a headless simulation harness used exclusively by tests.
// It mirrors Game.Update but has no Ebiten dependency and supports
// deterministic seeding and structured logging.
type TestSim struct {
	Width     int
	Height    int
	buildings []rect
	NavGrid   *NavGrid
	Soldiers  []*Soldier // all soldiers across both teams
	Squads    []*Squad
	SimLog    *SimLog
	Tick      int
	rng       *rand.Rand
	combat    *CombatManager

	// internal counters
	nextID int
	tick   int // pointer target for soldiers
}

// simOptionKind controls the pass in which an option is applied.
type simOptionKind int

const (
	simOptInfra   simOptionKind = iota // map size, buildings, seed, verbose — applied first
	simOptSoldier                      // add soldiers — applied after navgrid is built
	simOptSquad                        // form squads — applied after soldiers exist
)

// SimOption is a builder function applied to a TestSim during construction.
type SimOption struct {
	kind simOptionKind
	fn   func(*TestSim)
}

// WithMapSize sets the playfield dimensions.
func WithMapSize(w, h int) SimOption {
	return SimOption{simOptInfra, func(ts *TestSim) {
		ts.Width = w
		ts.Height = h
	}}
}

// WithBuilding adds an obstacle.
func WithBuilding(x, y, w, h int) SimOption {
	return SimOption{simOptInfra, func(ts *TestSim) {
		ts.buildings = append(ts.buildings, rect{x: x, y: y, w: w, h: h})
	}}
}

// WithSeed sets the RNG seed for deterministic runs.
func WithSeed(seed int64) SimOption {
	return SimOption{simOptInfra, func(ts *TestSim) {
		ts.rng = rand.New(rand.NewSource(seed)) // #nosec G404 -- test harness
	}}
}

// WithVerbose enables per-tick verbose logging.
func WithVerbose(v bool) SimOption {
	return SimOption{simOptInfra, func(ts *TestSim) {
		ts.SimLog = NewSimLog(v)
	}}
}

// WithRedSoldier adds a red team soldier advancing from (sx,sy) toward (tx,ty).
func WithRedSoldier(id int, sx, sy, tx, ty float64) SimOption {
	return SimOption{simOptSoldier, func(ts *TestSim) {
		ts.addSoldier(id, sx, sy, TeamRed, [2]float64{sx, sy}, [2]float64{tx, ty})
	}}
}

// WithBlueSoldier adds a blue team soldier advancing from (sx,sy) toward (tx,ty).
func WithBlueSoldier(id int, sx, sy, tx, ty float64) SimOption {
	return SimOption{simOptSoldier, func(ts *TestSim) {
		ts.addSoldier(id, sx, sy, TeamBlue, [2]float64{sx, sy}, [2]float64{tx, ty})
	}}
}

// WithRedSquad groups existing red soldiers (by ID) into a squad.
func WithRedSquad(ids ...int) SimOption {
	return SimOption{simOptSquad, func(ts *TestSim) {
		ts.formSquad(TeamRed, ids)
	}}
}

// WithBlueSquad groups existing blue soldiers (by ID) into a squad.
func WithBlueSquad(ids ...int) SimOption {
	return SimOption{simOptSquad, func(ts *TestSim) {
		ts.formSquad(TeamBlue, ids)
	}}
}

// NewTestSim constructs a TestSim from the given options in three ordered passes:
//  1. Infrastructure (map size, buildings, seed, verbose)
//  2. Build NavGrid
//  3. Soldiers
//  4. Squads
func NewTestSim(opts ...SimOption) *TestSim {
	ts := &TestSim{
		Width:  1280,
		Height: 720,
		SimLog: NewSimLog(false),
		rng:    rand.New(rand.NewSource(1)), // #nosec G404 -- test harness default
	}
	for _, o := range opts {
		if o.kind == simOptInfra {
			o.fn(ts)
		}
	}
	ts.buildNavGrid()
	for _, o := range opts {
		if o.kind == simOptSoldier {
			o.fn(ts)
		}
	}
	for _, o := range opts {
		if o.kind == simOptSquad {
			o.fn(ts)
		}
	}
	ts.combat = NewCombatManager(ts.rng.Int63())
	return ts
}

// buildNavGrid (re)builds the nav grid from current buildings. Called after all
// buildings have been added. Must be called before soldiers are added if you
// want their initial paths to be correct — NewTestSim handles this automatically.
func (ts *TestSim) buildNavGrid() {
	ts.NavGrid = NewNavGrid(ts.Width, ts.Height, ts.buildings, soldierRadius)
	// Re-path any soldiers that were added before the grid was built.
	for _, s := range ts.Soldiers {
		s.navGrid = ts.NavGrid
		s.recomputePath()
	}
}

// addSoldier is the internal helper used by WithRedSoldier / WithBlueSoldier.
func (ts *TestSim) addSoldier(id int, x, y float64, team Team, start, end [2]float64) {
	tl := NewThoughtLog() // per-sim log; not rendered
	s := NewSoldier(id, x, y, team, start, end, ts.NavGrid, tl, &ts.tick)
	ts.Soldiers = append(ts.Soldiers, s)
	if id >= ts.nextID {
		ts.nextID = id + 1
	}
}

// formSquad groups soldiers into a squad. indices are soldier IDs (not slice indices).
func (ts *TestSim) formSquad(team Team, ids []int) {
	var members []*Soldier
	for _, id := range ids {
		for _, s := range ts.Soldiers {
			if s.id == id && s.team == team {
				members = append(members, s)
				break
			}
		}
	}
	if len(members) == 0 {
		return
	}
	sqID := len(ts.Squads)
	sq := NewSquad(sqID, team, members)
	ts.Squads = append(ts.Squads, sq)
}

// AllByTeam returns all soldiers for a given team.
func (ts *TestSim) AllByTeam(team Team) []*Soldier {
	var out []*Soldier
	for _, s := range ts.Soldiers {
		if s.team == team {
			out = append(out, s)
		}
	}
	return out
}

// RunTicks advances the simulation n ticks, logging events to SimLog.
func (ts *TestSim) RunTicks(n int) {
	reds := ts.AllByTeam(TeamRed)
	blues := ts.AllByTeam(TeamBlue)

	for i := 0; i < n; i++ {
		ts.tick++
		ts.runOneTick(reds, blues)
	}
}

// RunUntil advances the simulation up to maxTicks, stopping early if predicate
// returns true. Returns the tick at which the predicate was satisfied, or -1.
func (ts *TestSim) RunUntil(predicate func(*TestSim) bool, maxTicks int) int {
	reds := ts.AllByTeam(TeamRed)
	blues := ts.AllByTeam(TeamBlue)

	for i := 0; i < maxTicks; i++ {
		ts.tick++
		ts.runOneTick(reds, blues)
		if predicate(ts) {
			return ts.tick
		}
	}
	return -1
}

// runOneTick mirrors Game.Update for the headless harness.
func (ts *TestSim) runOneTick(reds, blues []*Soldier) {
	tick := ts.tick

	// Snapshot previous goals/intents for change detection.
	prevGoals := make(map[int]GoalKind, len(ts.Soldiers))
	for _, s := range ts.Soldiers {
		prevGoals[s.id] = s.blackboard.CurrentGoal
	}
	prevIntents := make(map[int]SquadIntentKind, len(ts.Squads))
	for _, sq := range ts.Squads {
		prevIntents[sq.ID] = sq.Intent
	}
	prevStances := make(map[int]Stance, len(ts.Soldiers))
	for _, s := range ts.Soldiers {
		prevStances[s.id] = s.profile.Stance
	}
	prevStates := make(map[int]SoldierState, len(ts.Soldiers))
	for _, s := range ts.Soldiers {
		prevStates[s.id] = s.state
	}
	prevContacts := make(map[int]int, len(ts.Soldiers))
	for _, s := range ts.Soldiers {
		prevContacts[s.id] = len(s.vision.KnownContacts)
	}

	// 1. SENSE
	for _, s := range reds {
		s.UpdateVision(blues, ts.buildings)
	}
	for _, s := range blues {
		s.UpdateVision(reds, ts.buildings)
	}

	// 2. COMBAT
	ts.combat.ResetFireCounts(ts.Soldiers)
	ts.combat.ResolveCombat(reds, blues, reds, ts.buildings)
	ts.combat.ResolveCombat(blues, reds, blues, ts.buildings)
	ts.combat.UpdateTracers()

	// 3. SQUAD THINK
	for _, sq := range ts.Squads {
		sq.SquadThink(nil)
	}

	// Formation pass
	for _, sq := range ts.Squads {
		sq.UpdateFormation()
	}

	// 4+5. INDIVIDUAL THINK + ACT
	for _, s := range ts.Soldiers {
		s.Update()
	}

	// --- Post-tick logging ---

	for _, sq := range ts.Squads {
		if sq.Intent != prevIntents[sq.ID] {
			label := "--"
			teamStr := "red"
			if sq.Leader != nil {
				label = sq.Leader.label
				if sq.Team == TeamBlue {
					teamStr = "blue"
				}
			}
			ts.SimLog.Add(tick, label, teamStr, "squad", "intent_change",
				fmt.Sprintf("%s → %s", prevIntents[sq.ID], sq.Intent), 0)
		}
		// Verbose: log squad spread every tick.
		if sq.Leader != nil {
			spread := sq.squadSpread()
			ts.SimLog.AddVerbose(tick, sq.Leader.label, teamLabel(sq.Team),
				"squad", "spread", fmt.Sprintf("%.1fpx", spread), spread)
		}
	}

	for _, s := range ts.Soldiers {
		tStr := teamLabel(s.team)

		// Goal changes.
		if s.blackboard.CurrentGoal != prevGoals[s.id] {
			ts.SimLog.Add(tick, s.label, tStr, "goal", "change",
				fmt.Sprintf("%s → %s", prevGoals[s.id], s.blackboard.CurrentGoal), 0)
		}
		// Verbose: goal every tick with util hint.
		ts.SimLog.AddVerbose(tick, s.label, tStr, "goal", "current",
			s.blackboard.CurrentGoal.String(), 0)

		// Stance changes.
		if s.profile.Stance != prevStances[s.id] {
			ts.SimLog.Add(tick, s.label, tStr, "stance", "change",
				fmt.Sprintf("%s → %s", prevStances[s.id], s.profile.Stance), 0)
		}

		// State changes.
		if s.state != prevStates[s.id] {
			ts.SimLog.Add(tick, s.label, tStr, "state", "change",
				fmt.Sprintf("%s → %s", prevStates[s.id], s.state), 0)
		}

		// Vision contact changes.
		nowContacts := len(s.vision.KnownContacts)
		if nowContacts > prevContacts[s.id] {
			for _, c := range s.vision.KnownContacts {
				ts.SimLog.Add(tick, s.label, tStr, "vision", "contact_new",
					fmt.Sprintf("spotted %s at (%.0f,%.0f)", c.label, c.x, c.y), 0)
			}
		} else if nowContacts < prevContacts[s.id] {
			ts.SimLog.Add(tick, s.label, tStr, "vision", "contact_lost",
				fmt.Sprintf("%d → %d contacts", prevContacts[s.id], nowContacts), 0)
		}

		// Verbose: position.
		ts.SimLog.AddVerbose(tick, s.label, tStr, "move", "position",
			fmt.Sprintf("(%.1f,%.1f)", s.x, s.y), 0)

		// Verbose: fear.
		ts.SimLog.AddVerbose(tick, s.label, tStr, "stats", "fear",
			fmt.Sprintf("%.3f", s.profile.Psych.Fear), s.profile.Psych.Fear)
	}
}

// teamLabel returns a short string for a team.
func teamLabel(t Team) string {
	if t == TeamRed {
		return "red"
	}
	return "blue"
}

// CurrentTick returns the current simulation tick.
func (ts *TestSim) CurrentTick() int {
	return ts.tick
}

// Snapshot captures a lightweight state summary.
type SimSnapshot struct {
	Tick     int
	Soldiers []SoldierSnapshot
}

// SoldierSnapshot is a lightweight copy of a soldier's state at a tick.
type SoldierSnapshot struct {
	ID    int
	Label string
	Team  Team
	X, Y  float64
	State SoldierState
	Goal  GoalKind
	Fear  float64
}

// Snapshot returns the current state of all soldiers.
func (ts *TestSim) Snapshot() SimSnapshot {
	snap := SimSnapshot{Tick: ts.tick}
	for _, s := range ts.Soldiers {
		snap.Soldiers = append(snap.Soldiers, SoldierSnapshot{
			ID:    s.id,
			Label: s.label,
			Team:  s.team,
			X:     s.x,
			Y:     s.y,
			State: s.state,
			Goal:  s.blackboard.CurrentGoal,
			Fear:  s.profile.Psych.Fear,
		})
	}
	return snap
}
