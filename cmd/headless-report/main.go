package main

import (
	"flag"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Garsondee/Soldier-Sense/internal/game"
)

const (
	stalemateMinTeamSurvivalRate   = 0.50
	stalemateMinFrictionPerSoldier = 2.0
)

type soldierPerformance struct {
	label                string
	team                 game.Team
	startX               float64
	startY               float64
	lastX                float64
	lastY                float64
	totalTicks           int
	stationaryTicks      int
	movingTicks          int
	sawEnemyTicks        int
	inRangeTicks         int
	farFromLeaderTicks   int
	maxDistFromLeader    float64
	neverSawEnemy        bool
	neverInRange         bool
	immobile             bool
	excessivelySeparated bool
	immobilityPct        float64
	separationPct        float64
	positionSamples      int

	// Diagnostic information
	goalChanges      []goalChange
	stateChanges     []stateChange
	stalledEvents    []stalledEvent
	detachedEvents   []detachedEvent
	proximityPartner string
	proximityPct     float64
	pathFailures     int
	boundHoldTicks   int
}

type goalChange struct {
	tick     int
	fromGoal string
	toGoal   string
}

type stateChange struct {
	tick      int
	fromState string
	toState   string
}

type stalledEvent struct {
	tick   int
	goal   string
	intent string
	moved  float64
}

type detachedEvent struct {
	tick       int
	leaderDist float64
	goal       string
	intent     string
}

type runStats struct {
	runIndex int
	seed     int64
	ticks    int

	setupDur time.Duration
	simDur   time.Duration
	postDur  time.Duration
	totalDur time.Duration

	firstContactTick     int
	firstEngageTick      int
	firstRegroupTick     int
	firstDeathTick       int
	firstPanicTick       int
	firstSurrenderTick   int
	firstBreakTick       int
	firstDisobeyOnTick   int
	firstPanicOnTick     int
	firstSurrenderOnTick int

	intentChanges int
	goalChanges   int
	stateChanges  int
	contactNew    int
	contactLost   int

	stalledEvents        int
	detachedEvents       int
	disobeyEvents        int
	panicEvents          int
	surrenderEvents      int
	disobeyOnEvents      int
	disobeyOffEvents     int
	panicOnEvents        int
	panicOffEvents       int
	surrenderOnEvents    int
	surrenderOffEvents   int
	disobeyOnPreDeath    int
	panicOnPreDeath      int
	surrenderOnPreDeath  int
	cohesionBreakEvents  int
	cohesionReformEvents int
	affected             map[string]struct{}
	peakRefusing         int
	peakRefusingRed      int
	peakRefusingBlue     int
	peakRefusingTick     int

	windowSummary *game.WindowReport
	grades        []game.SoldierGrade

	redTotal      int
	blueTotal     int
	redSurvivors  int
	blueSurvivors int

	stalemate       bool
	stalemateReason string

	outcome       game.BattleOutcome
	outcomeReason game.BattleOutcomeReason

	soldierPerf         []soldierPerformance
	problematicSoldiers []soldierPerformance
}

const (
	immobilityThreshold        = 0.50
	separationThreshold        = 0.30
	maxLeaderDistance          = 300.0
	maxFireRange               = 800.0
	positionChangeThreshold    = 5.0
	effectivenessStalledTicks  = 180
	effectivenessDetachedTicks = 180
)

func analyzeSoldierPerformance(entries []game.SimLogEntry, grades []game.SoldierGrade, ticks int) []soldierPerformance {
	perfMap := make(map[string]*soldierPerformance)

	for _, g := range grades {
		perfMap[g.Label] = &soldierPerformance{
			label:      g.Label,
			team:       g.Team,
			totalTicks: ticks,
		}
	}

	stalledByLabel := make(map[string]int)
	detachedByLabel := make(map[string]int)
	contactByLabel := make(map[string]int)
	prevGoal := make(map[string]string)
	prevState := make(map[string]string)

	for _, e := range entries {
		perf, ok := perfMap[e.Soldier]
		if !ok {
			continue
		}

		switch e.Category {
		case "effectiveness":
			if e.Key == "stalled_in_combat" {
				stalledByLabel[e.Soldier]++
				// Parse stalled event details
				goal := extractField(e.Value, "goal=")
				intent := extractField(e.Value, "intent=")
				moved := extractFloatField(e.Value, "moved=")
				perf.stalledEvents = append(perf.stalledEvents, stalledEvent{
					tick:   e.Tick,
					goal:   goal,
					intent: intent,
					moved:  moved,
				})
			} else if e.Key == "detached_from_engagement" {
				detachedByLabel[e.Soldier]++
				goal := extractField(e.Value, "goal=")
				intent := extractField(e.Value, "intent=")
				leaderDist := extractFloatField(e.Value, "leader_dist=")
				perf.detachedEvents = append(perf.detachedEvents, detachedEvent{
					tick:       e.Tick,
					leaderDist: leaderDist,
					goal:       goal,
					intent:     intent,
				})
			}
		case "vision":
			if e.Key == "contact_new" {
				contactByLabel[e.Soldier]++
				perf.sawEnemyTicks++
			}
		case "goal":
			if e.Key == "change" {
				parts := strings.Split(e.Value, " → ")
				if len(parts) == 2 {
					from := strings.TrimSpace(parts[0])
					to := strings.TrimSpace(parts[1])
					if prev, exists := prevGoal[e.Soldier]; exists && prev != from {
						from = prev
					}
					perf.goalChanges = append(perf.goalChanges, goalChange{
						tick:     e.Tick,
						fromGoal: from,
						toGoal:   to,
					})
					prevGoal[e.Soldier] = to
				}
			}
		case "state":
			if e.Key == "change" {
				parts := strings.Split(e.Value, " → ")
				if len(parts) == 2 {
					from := strings.TrimSpace(parts[0])
					to := strings.TrimSpace(parts[1])
					if prev, exists := prevState[e.Soldier]; exists && prev != from {
						from = prev
					}
					perf.stateChanges = append(perf.stateChanges, stateChange{
						tick:      e.Tick,
						fromState: from,
						toState:   to,
					})
					prevState[e.Soldier] = to
				}
			}
		}
	}

	// Analyze proximity patterns
	analyzeProximity(entries, perfMap, ticks)

	result := make([]soldierPerformance, 0, len(perfMap))
	for label, perf := range perfMap {
		stalledEvents := stalledByLabel[label]
		detachedEvents := detachedByLabel[label]
		contactEvents := contactByLabel[label]

		perf.stationaryTicks = stalledEvents * effectivenessStalledTicks
		perf.farFromLeaderTicks = detachedEvents * effectivenessDetachedTicks

		if perf.totalTicks > 0 {
			perf.immobilityPct = float64(perf.stationaryTicks) / float64(perf.totalTicks) * 100
			perf.separationPct = float64(perf.farFromLeaderTicks) / float64(perf.totalTicks) * 100
		}

		perf.immobile = perf.immobilityPct >= immobilityThreshold*100
		perf.neverSawEnemy = contactEvents == 0
		perf.neverInRange = contactEvents == 0
		perf.excessivelySeparated = perf.separationPct >= separationThreshold*100

		result = append(result, *perf)
	}

	return result
}

func extractField(value, prefix string) string {
	idx := strings.Index(value, prefix)
	if idx < 0 {
		return ""
	}
	start := idx + len(prefix)
	end := strings.IndexAny(value[start:], " \t")
	if end < 0 {
		return value[start:]
	}
	return value[start : start+end]
}

func extractFloatField(value, prefix string) float64 {
	s := extractField(value, prefix)
	if s == "" {
		return 0
	}
	var f float64
	if _, err := fmt.Sscanf(s, "%f", &f); err != nil {
		return 0
	}
	return f
}

func analyzeProximity(entries []game.SimLogEntry, perfMap map[string]*soldierPerformance, ticks int) {
	// Track which soldiers are frequently near each other during stalled periods
	// This helps identify soldiers bouncing off each other
	type proximityKey struct {
		soldier1 string
		soldier2 string
	}
	proximityCount := make(map[proximityKey]int)

	// For now, we'll use stalled events as a proxy for proximity issues
	// In a full implementation, we'd track actual position data
	for label, perf := range perfMap {
		if len(perf.stalledEvents) == 0 {
			continue
		}

		// Check if this soldier's stalled events coincide with another soldier's
		for otherLabel, otherPerf := range perfMap {
			if label == otherLabel || perf.team != otherPerf.team {
				continue
			}

			// Count overlapping stalled periods
			overlap := 0
			for _, se := range perf.stalledEvents {
				for _, ose := range otherPerf.stalledEvents {
					if abs(se.tick-ose.tick) < 60 { // Within 1 second
						overlap++
					}
				}
			}

			if overlap > 0 {
				key := proximityKey{soldier1: label, soldier2: otherLabel}
				if label > otherLabel {
					key = proximityKey{soldier1: otherLabel, soldier2: label}
				}
				proximityCount[key] += overlap
			}
		}
	}

	// Assign proximity partners to soldiers
	for key, count := range proximityCount {
		perf1 := perfMap[key.soldier1]
		perf2 := perfMap[key.soldier2]

		pct1 := float64(count) / float64(len(perf1.stalledEvents)) * 100
		pct2 := float64(count) / float64(len(perf2.stalledEvents)) * 100

		if pct1 > 50 && pct1 > perf1.proximityPct {
			perf1.proximityPartner = key.soldier2
			perf1.proximityPct = pct1
		}
		if pct2 > 50 && pct2 > perf2.proximityPct {
			perf2.proximityPartner = key.soldier1
			perf2.proximityPct = pct2
		}
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func main() {
	var runs int
	var ticks int
	var seedBase int64
	var seedStep int64
	var scenario string

	flag.IntVar(&runs, "runs", 5, "number of headless simulation runs")
	flag.IntVar(&ticks, "ticks", 3600, "ticks per run")
	flag.Int64Var(&seedBase, "seed-base", 42, "base RNG seed for run 1")
	flag.Int64Var(&seedStep, "seed-step", 1, "seed increment between runs")
	flag.StringVar(&scenario, "scenario", "mutual-advance", "scenario name")
	flag.Parse()

	if runs <= 0 {
		fmt.Println("error: -runs must be > 0")
		return
	}
	if ticks <= 0 {
		fmt.Println("error: -ticks must be > 0")
		return
	}
	if scenario != "mutual-advance" {
		fmt.Printf("error: unsupported scenario %q (supported: mutual-advance)\n", scenario)
		return
	}

	fmt.Printf("=== Headless Combat Report ===\n")
	fmt.Printf("scenario=%s runs=%d ticks=%d seed_base=%d seed_step=%d\n\n", scenario, runs, ticks, seedBase, seedStep)

	all := make([]runStats, 0, runs)
	for i := 0; i < runs; i++ {
		seed := seedBase + int64(i)*seedStep
		stats := runScenarioMutualAdvance(i+1, seed, ticks)
		all = append(all, stats)
		printRun(stats)
	}

	printAggregate(all)
}

func runScenarioMutualAdvance(runIndex int, seed int64, ticks int) runStats {
	t0 := time.Now()
	setupStart := time.Now()
	bf := game.NewHeadlessBattlefield(seed, 3072, 1728)
	ts := game.NewTestSim(
		game.WithHeadlessBattlefield(bf),
		game.WithSeed(seed),
		game.WithRedSoldier(0, 80, 864, 2992, 864),
		game.WithRedSoldier(1, 80, 836, 2992, 836),
		game.WithRedSoldier(2, 80, 892, 2992, 892),
		game.WithRedSoldier(3, 80, 808, 2992, 808),
		game.WithRedSoldier(4, 80, 920, 2992, 920),
		game.WithRedSoldier(5, 80, 780, 2992, 780),
		game.WithBlueSoldier(6, 2992, 864, 80, 864),
		game.WithBlueSoldier(7, 2992, 836, 80, 836),
		game.WithBlueSoldier(8, 2992, 892, 80, 892),
		game.WithBlueSoldier(9, 2992, 808, 80, 808),
		game.WithBlueSoldier(10, 2992, 920, 80, 920),
		game.WithBlueSoldier(11, 2992, 780, 80, 780),
		game.WithRedSquad(0, 1, 2, 3, 4, 5),
		game.WithBlueSquad(6, 7, 8, 9, 10, 11),
	)
	setupDur := time.Since(setupStart)

	simStart := time.Now()
	ts.RunTicks(ticks)
	simDur := time.Since(simStart)

	postStart := time.Now()

	entries := ts.SimLog.Entries()
	firstDeathTick := firstTick(entries, "state", "change", "→ dead")
	affected := map[string]struct{}{}
	stalledEvents := 0
	detachedEvents := 0
	disobeyEvents := 0
	panicEvents := 0
	surrenderEvents := 0
	disobeyOnEvents := 0
	disobeyOffEvents := 0
	panicOnEvents := 0
	panicOffEvents := 0
	surrenderOnEvents := 0
	surrenderOffEvents := 0
	disobeyOnPreDeath := 0
	panicOnPreDeath := 0
	surrenderOnPreDeath := 0
	firstDisobeyOnTick := -1
	firstPanicOnTick := -1
	firstSurrenderOnTick := -1

	type psychState struct {
		team      string
		disobey   bool
		panic     bool
		surrender bool
	}
	psychBySoldier := map[string]psychState{}
	peakRefusing := 0
	peakRefusingRed := 0
	peakRefusingBlue := 0
	peakRefusingTick := -1
	cohesionBreakEvents := 0
	cohesionReformEvents := 0
	for _, e := range entries {
		switch e.Category {
		case "effectiveness":
			switch e.Key {
			case "stalled_in_combat":
				stalledEvents++
				affected[e.Soldier] = struct{}{}
			case "detached_from_engagement":
				detachedEvents++
				affected[e.Soldier] = struct{}{}
			}
		case "psych":
			ps := psychBySoldier[e.Soldier]
			if ps.team == "" {
				ps.team = e.Team
			}
			switch e.Key {
			case "disobedience":
				disobeyEvents++
				affected[e.Soldier] = struct{}{}
				if strings.Contains(e.Value, "disobeying") {
					disobeyOnEvents++
					ps.disobey = true
					if firstDisobeyOnTick < 0 {
						firstDisobeyOnTick = e.Tick
					}
					if firstDeathTick < 0 || e.Tick < firstDeathTick {
						disobeyOnPreDeath++
					}
				} else if strings.Contains(e.Value, "obeying") {
					disobeyOffEvents++
					ps.disobey = false
				}
			case "panic_retreat":
				panicEvents++
				affected[e.Soldier] = struct{}{}
				if strings.Contains(e.Value, "panic_retreat_on") {
					panicOnEvents++
					ps.panic = true
					if firstPanicOnTick < 0 {
						firstPanicOnTick = e.Tick
					}
					if firstDeathTick < 0 || e.Tick < firstDeathTick {
						panicOnPreDeath++
					}
				} else if strings.Contains(e.Value, "panic_retreat_off") {
					panicOffEvents++
					ps.panic = false
				}
			case "surrender":
				surrenderEvents++
				affected[e.Soldier] = struct{}{}
				if strings.Contains(e.Value, "surrender_on") {
					surrenderOnEvents++
					ps.surrender = true
					if firstSurrenderOnTick < 0 {
						firstSurrenderOnTick = e.Tick
					}
					if firstDeathTick < 0 || e.Tick < firstDeathTick {
						surrenderOnPreDeath++
					}
				} else if strings.Contains(e.Value, "surrender_off") {
					surrenderOffEvents++
					ps.surrender = false
				}
			}
			psychBySoldier[e.Soldier] = ps

			curRefusing := 0
			curRefusingRed := 0
			curRefusingBlue := 0
			for _, st := range psychBySoldier {
				if !(st.disobey || st.panic || st.surrender) {
					continue
				}
				curRefusing++
				if st.team == "red" {
					curRefusingRed++
				} else if st.team == "blue" {
					curRefusingBlue++
				}
			}
			if curRefusing > peakRefusing {
				peakRefusing = curRefusing
				peakRefusingRed = curRefusingRed
				peakRefusingBlue = curRefusingBlue
				peakRefusingTick = e.Tick
			}
		case "squad":
			if e.Key == "cohesion" {
				if strings.Contains(e.Value, "broken") {
					cohesionBreakEvents++
				} else if strings.Contains(e.Value, "reformed") {
					cohesionReformEvents++
				}
			}
		}
	}

	grades := ts.SoldierGrades()
	redTotal, blueTotal, redSurvivors, blueSurvivors := teamSurvivalCounts(grades)

	soldierPerf := analyzeSoldierPerformance(entries, grades, ticks)
	var problematicSoldiers []soldierPerformance
	for _, perf := range soldierPerf {
		if perf.immobile || perf.neverSawEnemy || perf.neverInRange || perf.excessivelySeparated {
			problematicSoldiers = append(problematicSoldiers, perf)
		}
	}

	rs := runStats{
		runIndex:             runIndex,
		seed:                 seed,
		ticks:                ticks,
		firstContactTick:     firstTick(entries, "vision", "contact_new", ""),
		firstEngageTick:      firstTick(entries, "squad", "intent_change", "engage"),
		firstRegroupTick:     firstTick(entries, "squad", "intent_change", "regroup"),
		firstDeathTick:       firstDeathTick,
		firstPanicTick:       firstTick(entries, "psych", "panic_retreat", "panic_retreat_on"),
		firstSurrenderTick:   firstTick(entries, "psych", "surrender", "surrender_on"),
		firstBreakTick:       firstTick(entries, "squad", "cohesion", "broken"),
		firstDisobeyOnTick:   firstDisobeyOnTick,
		firstPanicOnTick:     firstPanicOnTick,
		firstSurrenderOnTick: firstSurrenderOnTick,
		intentChanges:        ts.SimLog.CountCategory("squad", "intent_change"),
		goalChanges:          ts.SimLog.CountCategory("goal", "change"),
		stateChanges:         ts.SimLog.CountCategory("state", "change"),
		contactNew:           ts.SimLog.CountCategory("vision", "contact_new"),
		contactLost:          ts.SimLog.CountCategory("vision", "contact_lost"),
		stalledEvents:        stalledEvents,
		detachedEvents:       detachedEvents,
		disobeyEvents:        disobeyEvents,
		panicEvents:          panicEvents,
		surrenderEvents:      surrenderEvents,
		disobeyOnEvents:      disobeyOnEvents,
		disobeyOffEvents:     disobeyOffEvents,
		panicOnEvents:        panicOnEvents,
		panicOffEvents:       panicOffEvents,
		surrenderOnEvents:    surrenderOnEvents,
		surrenderOffEvents:   surrenderOffEvents,
		disobeyOnPreDeath:    disobeyOnPreDeath,
		panicOnPreDeath:      panicOnPreDeath,
		surrenderOnPreDeath:  surrenderOnPreDeath,
		cohesionBreakEvents:  cohesionBreakEvents,
		cohesionReformEvents: cohesionReformEvents,
		affected:             affected,
		peakRefusing:         peakRefusing,
		peakRefusingRed:      peakRefusingRed,
		peakRefusingBlue:     peakRefusingBlue,
		peakRefusingTick:     peakRefusingTick,
		windowSummary:        ts.Reporter.WindowSummary(),
		grades:               grades,
		redTotal:             redTotal,
		blueTotal:            blueTotal,
		redSurvivors:         redSurvivors,
		blueSurvivors:        blueSurvivors,
		soldierPerf:          soldierPerf,
		problematicSoldiers:  problematicSoldiers,
		setupDur:             setupDur,
		simDur:               simDur,
	}
	rs.stalemate, rs.stalemateReason = detectStalemate(rs)

	// Determine battle outcome
	redSoldiers := ts.AllByTeam(game.TeamRed)
	blueSoldiers := ts.AllByTeam(game.TeamBlue)
	redSquads := []*game.Squad{}
	blueSquads := []*game.Squad{}
	for _, sq := range ts.Squads {
		if sq.Team == game.TeamRed {
			redSquads = append(redSquads, sq)
		} else if sq.Team == game.TeamBlue {
			blueSquads = append(blueSquads, sq)
		}
	}
	rs.outcomeReason = game.DetermineBattleOutcome(redSoldiers, blueSoldiers, redSquads, blueSquads)
	rs.outcome = rs.outcomeReason.Outcome

	rs.postDur = time.Since(postStart)
	rs.totalDur = time.Since(t0)

	return rs
}

func firstTick(entries []game.SimLogEntry, category, key, contains string) int {
	for _, e := range entries {
		if e.Category != category || e.Key != key {
			continue
		}
		if contains == "" || strings.Contains(e.Value, contains) {
			return e.Tick
		}
	}
	return -1
}

func printProblematicSoldiers(rs runStats) {
	if len(rs.problematicSoldiers) == 0 {
		return
	}

	fmt.Printf("\n=== PROBLEMATIC SOLDIERS (Run %d, seed=%d) ===\n", rs.runIndex, rs.seed)
	fmt.Printf("Found %d soldiers with significant performance issues:\n\n", len(rs.problematicSoldiers))

	for i, perf := range rs.problematicSoldiers {
		issues := []string{}
		if perf.immobile {
			issues = append(issues, fmt.Sprintf("IMMOBILE(%.1f%%)", perf.immobilityPct))
		}
		if perf.neverSawEnemy {
			issues = append(issues, "NEVER_SAW_ENEMY")
		}
		if perf.neverInRange {
			issues = append(issues, "NEVER_IN_RANGE")
		}
		if perf.excessivelySeparated {
			issues = append(issues, fmt.Sprintf("SEPARATED(%.1f%%)", perf.separationPct))
		}

		fmt.Printf("[%d] %s [%s] - %s\n", i+1, perf.label, teamLabel(perf.team), strings.Join(issues, ", "))

		// Basic metrics
		fmt.Printf("  Engagement: saw_enemy=%dt in_range=%dt stationary=%dt (%.1f%%) far_from_leader=%dt (%.1f%%)\n",
			perf.sawEnemyTicks, perf.inRangeTicks, perf.stationaryTicks, perf.immobilityPct,
			perf.farFromLeaderTicks, perf.separationPct)

		// Proximity analysis - soldiers stuck together
		if perf.proximityPartner != "" {
			fmt.Printf("  PROXIMITY ISSUE: Spent %.1f%% of stalled time near %s (possible collision/bouncing)\n",
				perf.proximityPct, perf.proximityPartner)
		}

		// Goal pattern analysis
		if len(perf.goalChanges) > 0 {
			goalFreq := make(map[string]int)
			for _, gc := range perf.goalChanges {
				goalFreq[gc.toGoal]++
			}

			// Find most common goals
			topGoals := []string{}
			for goal, count := range goalFreq {
				if count > 2 || len(goalFreq) <= 3 {
					topGoals = append(topGoals, fmt.Sprintf("%s(%d)", goal, count))
				}
			}

			if len(topGoals) > 0 {
				fmt.Printf("  Goal pattern: %d changes, frequent: %s\n", len(perf.goalChanges), strings.Join(topGoals, ", "))
			}

			// Detect thrashing (rapid goal changes)
			if len(perf.goalChanges) > 10 {
				thrashCount := 0
				for j := 1; j < len(perf.goalChanges); j++ {
					if perf.goalChanges[j].tick-perf.goalChanges[j-1].tick < 30 {
						thrashCount++
					}
				}
				if thrashCount > 5 {
					fmt.Printf("  GOAL THRASHING: %d rapid goal changes detected (possible decision loop)\n", thrashCount)
				}
			}
		}

		// State pattern analysis
		if len(perf.stateChanges) > 0 {
			stateFreq := make(map[string]int)
			for _, sc := range perf.stateChanges {
				stateFreq[sc.toState]++
			}

			// Detect idle/cover loops
			idleCount := stateFreq["idle"]
			coverCount := stateFreq["cover"]
			if idleCount > 5 && coverCount > 5 {
				fmt.Printf("  IDLE/COVER LOOP: %d idle, %d cover transitions (possible stuck behavior)\n", idleCount, coverCount)
			}
		}

		// Stalled event analysis
		if len(perf.stalledEvents) > 0 {
			fmt.Printf("  Stalled events: %d occurrences\n", len(perf.stalledEvents))

			// Sample first, middle, and last stalled events
			samples := []stalledEvent{}
			if len(perf.stalledEvents) <= 3 {
				samples = perf.stalledEvents
			} else {
				samples = append(samples, perf.stalledEvents[0])
				samples = append(samples, perf.stalledEvents[len(perf.stalledEvents)/2])
				samples = append(samples, perf.stalledEvents[len(perf.stalledEvents)-1])
			}

			for _, se := range samples {
				fmt.Printf("    tick=%d goal=%s intent=%s moved=%.2f\n", se.tick, se.goal, se.intent, se.moved)
			}

			if len(perf.stalledEvents) > 3 {
				fmt.Printf("    ... (%d more stalled events)\n", len(perf.stalledEvents)-3)
			}
		}

		// Detached event analysis
		if len(perf.detachedEvents) > 0 {
			fmt.Printf("  Detached events: %d occurrences\n", len(perf.detachedEvents))

			// Sample detached events
			samples := []detachedEvent{}
			if len(perf.detachedEvents) <= 3 {
				samples = perf.detachedEvents
			} else {
				samples = append(samples, perf.detachedEvents[0])
				samples = append(samples, perf.detachedEvents[len(perf.detachedEvents)/2])
				samples = append(samples, perf.detachedEvents[len(perf.detachedEvents)-1])
			}

			for _, de := range samples {
				fmt.Printf("    tick=%d leader_dist=%.1f goal=%s intent=%s\n", de.tick, de.leaderDist, de.goal, de.intent)
			}

			if len(perf.detachedEvents) > 3 {
				fmt.Printf("    ... (%d more detached events)\n", len(perf.detachedEvents)-3)
			}
		}

		// Root cause summary
		fmt.Printf("  DIAGNOSIS: ")
		diagnoses := []string{}

		if perf.neverSawEnemy && len(perf.stalledEvents) > 0 {
			diagnoses = append(diagnoses, "Stuck before reaching combat")
		}
		if perf.proximityPartner != "" {
			diagnoses = append(diagnoses, fmt.Sprintf("Collision with %s", perf.proximityPartner))
		}
		if len(perf.goalChanges) > 15 {
			diagnoses = append(diagnoses, "Decision thrashing")
		}
		if len(perf.stalledEvents) > 0 && perf.stalledEvents[0].goal == "regroup" {
			diagnoses = append(diagnoses, "Stuck during regroup")
		}
		if len(perf.stalledEvents) > 0 && perf.stalledEvents[0].goal == "move_to_contact" {
			diagnoses = append(diagnoses, "Pathfinding/movement failure")
		}
		if len(diagnoses) == 0 {
			diagnoses = append(diagnoses, "Unknown - review event patterns above")
		}

		fmt.Printf("%s\n\n", strings.Join(diagnoses, "; "))
	}
}

func teamLabel(team game.Team) string {
	if team == game.TeamRed {
		return "red"
	}
	return "blue"
}

func printRun(rs runStats) {
	fmt.Printf("--- Run %d (seed=%d) ---\n", rs.runIndex, rs.seed)
	if rs.simDur > 0 {
		ticksPerSec := float64(rs.ticks) / rs.simDur.Seconds()
		usPerTick := rs.simDur.Seconds() * 1_000_000 / float64(rs.ticks)
		fmt.Printf("perf: setup=%s sim=%s (%.0f ticks/sec, %.2f us/tick) post=%s total=%s\n",
			rs.setupDur, rs.simDur, ticksPerSec, usPerTick, rs.postDur, rs.totalDur)
	} else {
		fmt.Printf("perf: setup=%s sim=%s post=%s total=%s\n", rs.setupDur, rs.simDur, rs.postDur, rs.totalDur)
	}
	fmt.Printf("phase_markers: contact=%d engage=%d regroup=%d first_death=%d first_panic=%d first_surrender=%d first_break=%d\n",
		rs.firstContactTick, rs.firstEngageTick, rs.firstRegroupTick, rs.firstDeathTick, rs.firstPanicTick, rs.firstSurrenderTick, rs.firstBreakTick)
	fmt.Printf("event_totals: intent_change=%d goal_change=%d state_change=%d contact_new=%d contact_lost=%d\n",
		rs.intentChanges, rs.goalChanges, rs.stateChanges, rs.contactNew, rs.contactLost)
	fmt.Printf("effectiveness_events: stalled_in_combat=%d detached_from_engagement=%d affected_soldiers=%d\n",
		rs.stalledEvents, rs.detachedEvents, len(rs.affected))
	fmt.Printf("survivors: red=%d/%d blue=%d/%d\n", rs.redSurvivors, rs.redTotal, rs.blueSurvivors, rs.blueTotal)
	fmt.Printf("stalemate_check: verdict=%t reason=%s\n", rs.stalemate, rs.stalemateReason)
	fmt.Printf("battle_outcome: %s (%s) red_squads_broken=%d/%d blue_squads_broken=%d/%d\n",
		rs.outcome, rs.outcomeReason.Description,
		rs.outcomeReason.RedSquadsBroken, rs.outcomeReason.RedSquadsTotal,
		rs.outcomeReason.BlueSquadsBroken, rs.outcomeReason.BlueSquadsTotal)
	fmt.Printf("psych_events: disobedience=%d panic_retreat=%d surrender=%d squad_break=%d squad_reform=%d\n",
		rs.disobeyEvents, rs.panicEvents, rs.surrenderEvents, rs.cohesionBreakEvents, rs.cohesionReformEvents)
	fmt.Printf("psych_refusal_transitions: disobey_on=%d disobey_off=%d panic_on=%d panic_off=%d surrender_on=%d surrender_off=%d\n",
		rs.disobeyOnEvents, rs.disobeyOffEvents, rs.panicOnEvents, rs.panicOffEvents, rs.surrenderOnEvents, rs.surrenderOffEvents)
	fmt.Printf("psych_refusal_early: disobey_on_pre_first_death=%d panic_on_pre_first_death=%d surrender_on_pre_first_death=%d\n",
		rs.disobeyOnPreDeath, rs.panicOnPreDeath, rs.surrenderOnPreDeath)
	fmt.Printf("psych_refusal_first_onsets: disobey_on=%d panic_on=%d surrender_on=%d\n",
		rs.firstDisobeyOnTick, rs.firstPanicOnTick, rs.firstSurrenderOnTick)
	fmt.Printf("psych_refusal_peak: total=%d red=%d blue=%d tick=%d\n",
		rs.peakRefusing, rs.peakRefusingRed, rs.peakRefusingBlue, rs.peakRefusingTick)
	fmt.Printf("affected_labels: %s\n", joinSet(rs.affected))
	if rs.windowSummary != nil {
		fmt.Printf("window_samples=%d window_tick_range=%d..%d\n",
			rs.windowSummary.SampleCount, rs.windowSummary.FromTick, rs.windowSummary.ToTick)
		fmt.Printf("window_effectiveness_avg: red_stalled=%.1f red_detached=%.1f blue_stalled=%.1f blue_detached=%.1f\n",
			rs.windowSummary.AvgRedStalledInCombat,
			rs.windowSummary.AvgRedDetached,
			rs.windowSummary.AvgBlueStalledInCombat,
			rs.windowSummary.AvgBlueDetached,
		)
		fmt.Printf("window_psych_avg: red_disobey=%.1f red_panic=%.1f red_surrender=%.1f red_broken=%.1f red_stress=%.2f red_casualty=%.2f\n",
			rs.windowSummary.AvgRedDisobeying,
			rs.windowSummary.AvgRedPanicRetreat,
			rs.windowSummary.AvgRedSurrendered,
			rs.windowSummary.AvgRedSquadBrokenMembers,
			rs.windowSummary.AvgRedSquadStress,
			rs.windowSummary.AvgRedCasualtyRate,
		)
		fmt.Printf("window_psych_avg_blue: blue_disobey=%.1f blue_panic=%.1f blue_surrender=%.1f blue_broken=%.1f blue_stress=%.2f blue_casualty=%.2f\n",
			rs.windowSummary.AvgBlueDisobeying,
			rs.windowSummary.AvgBluePanicRetreat,
			rs.windowSummary.AvgBlueSurrendered,
			rs.windowSummary.AvgBlueSquadBrokenMembers,
			rs.windowSummary.AvgBlueSquadStress,
			rs.windowSummary.AvgBlueCasualtyRate,
		)
		redAvgRefusing := rs.windowSummary.AvgRedDisobeying + rs.windowSummary.AvgRedPanicRetreat + rs.windowSummary.AvgRedSurrendered
		blueAvgRefusing := rs.windowSummary.AvgBlueDisobeying + rs.windowSummary.AvgBluePanicRetreat + rs.windowSummary.AvgBlueSurrendered
		fmt.Printf("window_refusal_pressure: red_refusing=%.1f/%.1f (%.1f%%) blue_refusing=%.1f/%.1f (%.1f%%)\n",
			redAvgRefusing,
			rs.windowSummary.AvgRedAlive,
			pct(redAvgRefusing, rs.windowSummary.AvgRedAlive),
			blueAvgRefusing,
			rs.windowSummary.AvgBlueAlive,
			pct(blueAvgRefusing, rs.windowSummary.AvgBlueAlive),
		)
	}
	fmt.Print(game.FormatGrades(rs.grades))
	printProblematicSoldiers(rs)
	fmt.Println()
}

func printAggregate(all []runStats) {
	if len(all) > 0 {
		setupMin, setupMax := all[0].setupDur, all[0].setupDur
		simMin, simMax := all[0].simDur, all[0].simDur
		postMin, postMax := all[0].postDur, all[0].postDur
		totalMin, totalMax := all[0].totalDur, all[0].totalDur
		var setupSum, simSum, postSum, totalSum time.Duration

		for _, rs := range all {
			setupSum += rs.setupDur
			simSum += rs.simDur
			postSum += rs.postDur
			totalSum += rs.totalDur

			if rs.setupDur < setupMin {
				setupMin = rs.setupDur
			}
			if rs.setupDur > setupMax {
				setupMax = rs.setupDur
			}
			if rs.simDur < simMin {
				simMin = rs.simDur
			}
			if rs.simDur > simMax {
				simMax = rs.simDur
			}
			if rs.postDur < postMin {
				postMin = rs.postDur
			}
			if rs.postDur > postMax {
				postMax = rs.postDur
			}
			if rs.totalDur < totalMin {
				totalMin = rs.totalDur
			}
			if rs.totalDur > totalMax {
				totalMax = rs.totalDur
			}
		}

		setupAvg := setupSum / time.Duration(len(all))
		simAvg := simSum / time.Duration(len(all))
		postAvg := postSum / time.Duration(len(all))
		totalAvg := totalSum / time.Duration(len(all))

		fmt.Printf("=== Perf Summary (avg/min/max over %d runs) ===\n", len(all))
		fmt.Printf("setup: %s / %s / %s\n", setupAvg, setupMin, setupMax)
		fmt.Printf("sim:   %s / %s / %s\n", simAvg, simMin, simMax)
		fmt.Printf("post:  %s / %s / %s\n", postAvg, postMin, postMax)
		fmt.Printf("total: %s / %s / %s\n\n", totalAvg, totalMin, totalMax)
	}

	totalStalled := 0
	totalDetached := 0
	totalDisobey := 0
	totalPanic := 0
	totalSurrender := 0
	totalDisobeyOn := 0
	totalDisobeyOff := 0
	totalPanicOn := 0
	totalPanicOff := 0
	totalSurrenderOn := 0
	totalSurrenderOff := 0
	totalDisobeyOnPreDeath := 0
	totalPanicOnPreDeath := 0
	totalSurrenderOnPreDeath := 0
	totalPeakRefusing := 0
	totalPeakRefusingRed := 0
	totalPeakRefusingBlue := 0
	totalBreak := 0
	totalReform := 0
	totalIntent := 0
	totalGoal := 0
	totalState := 0
	totalContactNew := 0
	totalContactLost := 0
	totalRedSurvivors := 0
	totalBlueSurvivors := 0
	totalRedSoldiers := 0
	totalBlueSoldiers := 0
	stalemateRuns := 0
	redVictories := 0
	blueVictories := 0
	draws := 0
	inconclusives := 0

	contactTicks := make([]int, 0, len(all))
	engageTicks := make([]int, 0, len(all))
	deathTicks := make([]int, 0, len(all))
	panicTicks := make([]int, 0, len(all))
	surrenderTicks := make([]int, 0, len(all))
	breakTicks := make([]int, 0, len(all))
	affectedGlobal := map[string]struct{}{}

	// Aggregate per-soldier scores across runs.
	type soldierAgg struct {
		scoreSum float64
		count    int
		survived int
		good     map[string]int
		bad      map[string]int
	}
	soldierAggs := map[string]*soldierAgg{}

	for _, rs := range all {
		totalStalled += rs.stalledEvents
		totalDetached += rs.detachedEvents
		totalDisobey += rs.disobeyEvents
		totalPanic += rs.panicEvents
		totalSurrender += rs.surrenderEvents
		totalDisobeyOn += rs.disobeyOnEvents
		totalDisobeyOff += rs.disobeyOffEvents
		totalPanicOn += rs.panicOnEvents
		totalPanicOff += rs.panicOffEvents
		totalSurrenderOn += rs.surrenderOnEvents
		totalSurrenderOff += rs.surrenderOffEvents
		totalDisobeyOnPreDeath += rs.disobeyOnPreDeath
		totalPanicOnPreDeath += rs.panicOnPreDeath
		totalSurrenderOnPreDeath += rs.surrenderOnPreDeath
		totalPeakRefusing += rs.peakRefusing
		totalPeakRefusingRed += rs.peakRefusingRed
		totalPeakRefusingBlue += rs.peakRefusingBlue
		totalBreak += rs.cohesionBreakEvents
		totalReform += rs.cohesionReformEvents
		totalIntent += rs.intentChanges
		totalGoal += rs.goalChanges
		totalState += rs.stateChanges
		totalContactNew += rs.contactNew
		totalContactLost += rs.contactLost
		totalRedSurvivors += rs.redSurvivors
		totalBlueSurvivors += rs.blueSurvivors
		totalRedSoldiers += rs.redTotal
		totalBlueSoldiers += rs.blueTotal
		if rs.stalemate {
			stalemateRuns++
		}
		switch rs.outcome {
		case game.OutcomeRedVictory:
			redVictories++
		case game.OutcomeBlueVictory:
			blueVictories++
		case game.OutcomeDraw:
			draws++
		case game.OutcomeInconclusive:
			inconclusives++
		}
		if rs.firstContactTick >= 0 {
			contactTicks = append(contactTicks, rs.firstContactTick)
		}
		if rs.firstEngageTick >= 0 {
			engageTicks = append(engageTicks, rs.firstEngageTick)
		}
		if rs.firstDeathTick >= 0 {
			deathTicks = append(deathTicks, rs.firstDeathTick)
		}
		if rs.firstPanicTick >= 0 {
			panicTicks = append(panicTicks, rs.firstPanicTick)
		}
		if rs.firstSurrenderTick >= 0 {
			surrenderTicks = append(surrenderTicks, rs.firstSurrenderTick)
		}
		if rs.firstBreakTick >= 0 {
			breakTicks = append(breakTicks, rs.firstBreakTick)
		}
		for label := range rs.affected {
			affectedGlobal[label] = struct{}{}
		}
		for _, g := range rs.grades {
			ag, ok := soldierAggs[g.Label]
			if !ok {
				ag = &soldierAgg{good: map[string]int{}, bad: map[string]int{}}
				soldierAggs[g.Label] = ag
			}
			ag.scoreSum += g.Score
			ag.count++
			if g.Survived {
				ag.survived++
			}
			for _, t := range g.GoodTraits {
				ag.good[t]++
			}
			for _, t := range g.BadTraits {
				ag.bad[t]++
			}
		}
	}

	fmt.Println("=== Aggregate AAR Inputs ===")
	fmt.Printf("runs=%d\n", len(all))
	fmt.Printf("avg_events_per_run: intent_change=%.1f goal_change=%.1f state_change=%.1f contact_new=%.1f contact_lost=%.1f\n",
		avg(totalIntent, len(all)), avg(totalGoal, len(all)), avg(totalState, len(all)), avg(totalContactNew, len(all)), avg(totalContactLost, len(all)))
	fmt.Printf("avg_effectiveness_per_run: stalled_in_combat=%.1f detached_from_engagement=%.1f\n",
		avg(totalStalled, len(all)), avg(totalDetached, len(all)))
	fmt.Printf("avg_psych_events_per_run: disobedience=%.1f panic_retreat=%.1f surrender=%.1f squad_break=%.1f squad_reform=%.1f\n",
		avg(totalDisobey, len(all)), avg(totalPanic, len(all)), avg(totalSurrender, len(all)), avg(totalBreak, len(all)), avg(totalReform, len(all)))
	fmt.Printf("avg_psych_refusal_transitions_per_run: disobey_on=%.1f disobey_off=%.1f panic_on=%.1f panic_off=%.1f surrender_on=%.1f surrender_off=%.1f\n",
		avg(totalDisobeyOn, len(all)), avg(totalDisobeyOff, len(all)), avg(totalPanicOn, len(all)), avg(totalPanicOff, len(all)), avg(totalSurrenderOn, len(all)), avg(totalSurrenderOff, len(all)))
	fmt.Printf("avg_psych_refusal_onsets_pre_first_death_per_run: disobey_on=%.1f panic_on=%.1f surrender_on=%.1f\n",
		avg(totalDisobeyOnPreDeath, len(all)), avg(totalPanicOnPreDeath, len(all)), avg(totalSurrenderOnPreDeath, len(all)))
	fmt.Printf("avg_peak_refusing_per_run: total=%.1f red=%.1f blue=%.1f\n",
		avg(totalPeakRefusing, len(all)), avg(totalPeakRefusingRed, len(all)), avg(totalPeakRefusingBlue, len(all)))
	fmt.Printf("phase_marker_avg_ticks: first_contact=%s first_engage=%s first_death=%s first_panic=%s first_surrender=%s first_break=%s\n",
		avgTickString(contactTicks), avgTickString(engageTicks), avgTickString(deathTicks), avgTickString(panicTicks), avgTickString(surrenderTicks), avgTickString(breakTicks))
	fmt.Printf("unique_affected_labels=%d [%s]\n", len(affectedGlobal), joinSet(affectedGlobal))
	fmt.Printf("stalemate_runs=%d/%d (%.1f%%)\n", stalemateRuns, len(all), avg(stalemateRuns*100, len(all)))
	fmt.Printf("battle_outcomes: red_victories=%d blue_victories=%d draws=%d inconclusive=%d\n",
		redVictories, blueVictories, draws, inconclusives)
	fmt.Printf("outcome_percentages: red=%.1f%% blue=%.1f%% draw=%.1f%% inconclusive=%.1f%%\n",
		avg(redVictories*100, len(all)), avg(blueVictories*100, len(all)),
		avg(draws*100, len(all)), avg(inconclusives*100, len(all)))
	if totalRedSoldiers > 0 && totalBlueSoldiers > 0 {
		fmt.Printf("survival_rate: red=%.1f%% blue=%.1f%%\n",
			float64(totalRedSurvivors)/float64(totalRedSoldiers)*100,
			float64(totalBlueSurvivors)/float64(totalBlueSoldiers)*100,
		)
	}

	// Per-soldier aggregate performance.
	fmt.Println("\n=== Aggregate Soldier Performance ===")
	type labelScore struct {
		label    string
		avgScore float64
		survRate float64
		topGood  string
		topBad   string
	}
	var rows []labelScore
	for label, ag := range soldierAggs {
		avgS := 0.0
		if ag.count > 0 {
			avgS = ag.scoreSum / float64(ag.count)
		}
		survR := 0.0
		if ag.count > 0 {
			survR = float64(ag.survived) / float64(ag.count) * 100
		}
		tg := topTrait(ag.good)
		tb := topTrait(ag.bad)
		rows = append(rows, labelScore{label, avgS, survR, tg, tb})
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].label < rows[j].label
	})
	for _, r := range rows {
		grade := game.PerfLetterGrade(r.avgScore)
		fmt.Printf("  %s  %s (avg=%.1f)  survival=%.0f%%", r.label, grade, r.avgScore, r.survRate)
		if r.topGood != "" {
			fmt.Printf("  good=%s", r.topGood)
		}
		if r.topBad != "" {
			fmt.Printf("  bad=%s", r.topBad)
		}
		fmt.Println()
	}

	// Team-level aggregate from last run's grades as representative.
	if len(all) > 0 {
		fmt.Println("\n--- Team Summary (across all runs) ---")
		fmt.Print(game.FormatGradesSummary(collectAllGrades(all)))
	}
}

func avg(sum int, n int) float64 {
	if n <= 0 {
		return 0
	}
	return float64(sum) / float64(n)
}

func pct(part, whole float64) float64 {
	if whole <= 0 {
		return 0
	}
	return part / whole * 100
}

func avgTickString(vals []int) string {
	if len(vals) == 0 {
		return "n/a"
	}
	sum := 0
	for _, v := range vals {
		sum += v
	}
	return fmt.Sprintf("%.1f", float64(sum)/float64(len(vals)))
}

func topTrait(counts map[string]int) string {
	if len(counts) == 0 {
		return ""
	}
	best := ""
	bestN := 0
	for k, v := range counts {
		if v > bestN {
			best = k
			bestN = v
		}
	}
	return fmt.Sprintf("%s(%d)", best, bestN)
}

func collectAllGrades(all []runStats) []game.SoldierGrade {
	var out []game.SoldierGrade
	for _, rs := range all {
		out = append(out, rs.grades...)
	}
	return out
}

func teamSurvivalCounts(grades []game.SoldierGrade) (redTotal, blueTotal, redSurvivors, blueSurvivors int) {
	for _, g := range grades {
		switch g.Team {
		case game.TeamRed:
			redTotal++
			if g.Survived {
				redSurvivors++
			}
		case game.TeamBlue:
			blueTotal++
			if g.Survived {
				blueSurvivors++
			}
		}
	}
	return redTotal, blueTotal, redSurvivors, blueSurvivors
}

func detectStalemate(rs runStats) (bool, string) {
	if rs.redTotal <= 0 || rs.blueTotal <= 0 {
		return false, "insufficient-team-data"
	}
	redSurvival := float64(rs.redSurvivors) / float64(rs.redTotal)
	blueSurvival := float64(rs.blueSurvivors) / float64(rs.blueTotal)
	highMutualSurvival := redSurvival >= stalemateMinTeamSurvivalRate && blueSurvival >= stalemateMinTeamSurvivalRate

	totalSoldiers := rs.redTotal + rs.blueTotal
	frictionPerSoldier := 0.0
	if totalSoldiers > 0 {
		frictionPerSoldier = float64(rs.stalledEvents+rs.detachedEvents) / float64(totalSoldiers)
	}
	highFriction := frictionPerSoldier >= stalemateMinFrictionPerSoldier

	noSquadBreak := rs.cohesionBreakEvents == 0

	if highMutualSurvival && highFriction && noSquadBreak {
		return true, fmt.Sprintf("high_mutual_survival(%.2f/%.2f)+high_friction_per_soldier(%.2f)+no_squad_break", redSurvival, blueSurvival, frictionPerSoldier)
	}

	return false, fmt.Sprintf("mutual_survival=%.2f/%.2f friction_per_soldier=%.2f no_squad_break=%t", redSurvival, blueSurvival, frictionPerSoldier, noSquadBreak)
}

func joinSet(s map[string]struct{}) string {
	if len(s) == 0 {
		return "none"
	}
	labels := make([]string, 0, len(s))
	for k := range s {
		labels = append(labels, k)
	}
	sort.Strings(labels)
	return strings.Join(labels, ",")
}
