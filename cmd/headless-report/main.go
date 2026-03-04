// Package main runs repeated headless simulations and aggregate reporting.
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
	proximityPartner string
	label            string
	goalChanges      []goalChange
	stateChanges     []stateChange
	stalledEvents    []stalledEvent
	detachedEvents   []detachedEvent

	immobilityPct float64
	separationPct float64
	proximityPct  float64

	team               game.Team
	totalTicks         int
	stationaryTicks    int
	sawEnemyTicks      int
	inRangeTicks       int
	farFromLeaderTicks int

	neverSawEnemy        bool
	neverInRange         bool
	immobile             bool
	excessivelySeparated bool
}

type scenarioPsychState struct {
	team      string
	disobey   bool
	panic     bool
	surrender bool
}

type scenarioEventStats struct {
	affected             map[string]struct{}
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
	firstDisobeyOnTick   int
	firstPanicOnTick     int
	firstSurrenderOnTick int
	peakRefusing         int
	peakRefusingRed      int
	peakRefusingBlue     int
	peakRefusingTick     int
	cohesionBreakEvents  int
	cohesionReformEvents int
}

func analyzeScenarioEvents(entries []game.SimLogEntry, firstDeathTick int) scenarioEventStats {
	stats := scenarioEventStats{
		affected:             map[string]struct{}{},
		firstDisobeyOnTick:   -1,
		firstPanicOnTick:     -1,
		firstSurrenderOnTick: -1,
		peakRefusingTick:     -1,
	}
	psychBySoldier := map[string]scenarioPsychState{}

	for _, e := range entries {
		switch e.Category {
		case "effectiveness":
			handleScenarioEffectivenessEvent(&e, &stats)
		case "psych":
			handleScenarioPsychEvent(&e, firstDeathTick, &stats, psychBySoldier)
		case "squad":
			if e.Key == "cohesion" {
				if strings.Contains(e.Value, "broken") {
					stats.cohesionBreakEvents++
				} else if strings.Contains(e.Value, "reformed") {
					stats.cohesionReformEvents++
				}
			}
		}
	}

	return stats
}

func handleScenarioEffectivenessEvent(e *game.SimLogEntry, stats *scenarioEventStats) {
	switch e.Key {
	case "stalled_in_combat":
		stats.stalledEvents++
		stats.affected[e.Soldier] = struct{}{}
	case "detached_from_engagement":
		stats.detachedEvents++
		stats.affected[e.Soldier] = struct{}{}
	}
}

func printPerfSummary(all []runStats) {
	if len(all) == 0 {
		return
	}
	setupMin, setupMax := all[0].setupDur, all[0].setupDur
	simMin, simMax := all[0].simDur, all[0].simDur
	postMin, postMax := all[0].postDur, all[0].postDur
	totalMin, totalMax := all[0].totalDur, all[0].totalDur
	var setupSum, simSum, postSum, totalSum time.Duration

	for i := range all {
		rs := &all[i]
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

func printAggregateSoldierPerformance(agg *aggregateData) {
	fmt.Println("\n=== Aggregate Soldier Performance ===")
	type labelScore struct {
		label    string
		topGood  string
		topBad   string
		avgScore float64
		survRate float64
	}
	rows := make([]labelScore, 0, len(agg.soldierAggs))
	for label, ag := range agg.soldierAggs {
		avgS := 0.0
		if ag.count > 0 {
			avgS = ag.scoreSum / float64(ag.count)
		}
		survR := 0.0
		if ag.count > 0 {
			survR = float64(ag.survived) / float64(ag.count) * 100
		}
		rows = append(rows, labelScore{label, topTrait(ag.good), topTrait(ag.bad), avgS, survR})
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
}

func handleScenarioPsychEvent(
	e *game.SimLogEntry,
	firstDeathTick int,
	stats *scenarioEventStats,
	psychBySoldier map[string]scenarioPsychState,
) {
	ps := psychBySoldier[e.Soldier]
	if ps.team == "" {
		ps.team = e.Team
	}

	switch e.Key {
	case "disobedience":
		handleDisobedienceTransition(e, firstDeathTick, stats, &ps)
	case "panic_retreat":
		handlePanicTransition(e, firstDeathTick, stats, &ps)
	case "surrender":
		handleSurrenderTransition(e, firstDeathTick, stats, &ps)
	}

	psychBySoldier[e.Soldier] = ps
	updatePeakRefusing(e.Tick, stats, psychBySoldier)
}

func handleDisobedienceTransition(
	e *game.SimLogEntry,
	firstDeathTick int,
	stats *scenarioEventStats,
	ps *scenarioPsychState,
) {
	stats.disobeyEvents++
	stats.affected[e.Soldier] = struct{}{}
	if strings.Contains(e.Value, "disobeying") {
		stats.disobeyOnEvents++
		ps.disobey = true
		if stats.firstDisobeyOnTick < 0 {
			stats.firstDisobeyOnTick = e.Tick
		}
		if firstDeathTick < 0 || e.Tick < firstDeathTick {
			stats.disobeyOnPreDeath++
		}
		return
	}
	if strings.Contains(e.Value, "obeying") {
		stats.disobeyOffEvents++
		ps.disobey = false
	}
}

func handlePanicTransition(
	e *game.SimLogEntry,
	firstDeathTick int,
	stats *scenarioEventStats,
	ps *scenarioPsychState,
) {
	stats.panicEvents++
	stats.affected[e.Soldier] = struct{}{}
	if strings.Contains(e.Value, "panic_retreat_on") {
		stats.panicOnEvents++
		ps.panic = true
		if stats.firstPanicOnTick < 0 {
			stats.firstPanicOnTick = e.Tick
		}
		if firstDeathTick < 0 || e.Tick < firstDeathTick {
			stats.panicOnPreDeath++
		}
		return
	}
	if strings.Contains(e.Value, "panic_retreat_off") {
		stats.panicOffEvents++
		ps.panic = false
	}
}

func handleSurrenderTransition(
	e *game.SimLogEntry,
	firstDeathTick int,
	stats *scenarioEventStats,
	ps *scenarioPsychState,
) {
	stats.surrenderEvents++
	stats.affected[e.Soldier] = struct{}{}
	if strings.Contains(e.Value, "surrender_on") {
		stats.surrenderOnEvents++
		ps.surrender = true
		if stats.firstSurrenderOnTick < 0 {
			stats.firstSurrenderOnTick = e.Tick
		}
		if firstDeathTick < 0 || e.Tick < firstDeathTick {
			stats.surrenderOnPreDeath++
		}
		return
	}
	if strings.Contains(e.Value, "surrender_off") {
		stats.surrenderOffEvents++
		ps.surrender = false
	}
}

func updatePeakRefusing(
	tick int,
	stats *scenarioEventStats,
	psychBySoldier map[string]scenarioPsychState,
) {
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
	if curRefusing <= stats.peakRefusing {
		return
	}
	stats.peakRefusing = curRefusing
	stats.peakRefusingRed = curRefusingRed
	stats.peakRefusingBlue = curRefusingBlue
	stats.peakRefusingTick = tick
}

func orderedProximityKey(a, b string) struct{ soldier1, soldier2 string } {
	if a < b {
		return struct{ soldier1, soldier2 string }{soldier1: a, soldier2: b}
	}
	return struct{ soldier1, soldier2 string }{soldier1: b, soldier2: a}
}

func countStallOverlap(left, right []stalledEvent) int {
	overlap := 0
	for i := range left {
		for j := range right {
			if abs(left[i].tick-right[j].tick) < 60 { // Within 1 second.
				overlap++
			}
		}
	}
	return overlap
}

type goalChange struct {
	fromGoal string
	toGoal   string
	tick     int
}

type stateChange struct {
	fromState string
	toState   string
	tick      int
}

type stalledEvent struct {
	goal   string
	intent string
	moved  float64
	tick   int
}

type detachedEvent struct {
	goal       string
	intent     string
	leaderDist float64
	tick       int
}

type runStats struct {
	outcomeReason       game.BattleOutcomeReason
	affected            map[string]struct{}
	windowSummary       *game.WindowReport
	stalemateReason     string
	grades              []game.SoldierGrade
	soldierPerf         []soldierPerformance
	problematicSoldiers []soldierPerformance

	setupDur time.Duration
	simDur   time.Duration
	postDur  time.Duration
	totalDur time.Duration

	pfCalls         int64
	pfFails         int64
	pfExpandedNodes int64
	pfAvgRuntimeMS  float64
	pfP95RuntimeMS  float64
	pfFailRatePct   float64

	seed int64

	runIndex             int
	ticks                int
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
	intentChanges        int
	goalChanges          int
	stateChanges         int
	contactNew           int
	contactLost          int
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
	peakRefusing         int
	peakRefusingRed      int
	peakRefusingBlue     int
	peakRefusingTick     int
	redTotal             int
	blueTotal            int
	redSurvivors         int
	blueSurvivors        int

	outcome   game.BattleOutcome
	stalemate bool
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

	for i := range grades {
		g := grades[i]
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
			handleEffectivenessEvent(&e, perf, stalledByLabel, detachedByLabel)
		case "vision":
			handleVisionEvent(&e, perf, contactByLabel)
		case "goal":
			handleGoalChangeEvent(&e, perf, prevGoal)
		case "state":
			handleStateChangeEvent(&e, perf, prevState)
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

func handleEffectivenessEvent(
	e *game.SimLogEntry,
	perf *soldierPerformance,
	stalledByLabel map[string]int,
	detachedByLabel map[string]int,
) {
	switch e.Key {
	case "stalled_in_combat":
		stalledByLabel[e.Soldier]++
		perf.stalledEvents = append(perf.stalledEvents, stalledEvent{
			tick:   e.Tick,
			goal:   extractField(e.Value, "goal="),
			intent: extractField(e.Value, "intent="),
			moved:  extractFloatField(e.Value, "moved="),
		})
	case "detached_from_engagement":
		detachedByLabel[e.Soldier]++
		perf.detachedEvents = append(perf.detachedEvents, detachedEvent{
			tick:       e.Tick,
			leaderDist: extractFloatField(e.Value, "leader_dist="),
			goal:       extractField(e.Value, "goal="),
			intent:     extractField(e.Value, "intent="),
		})
	}
}

func handleVisionEvent(e *game.SimLogEntry, perf *soldierPerformance, contactByLabel map[string]int) {
	if e.Key != "contact_new" {
		return
	}
	contactByLabel[e.Soldier]++
	perf.sawEnemyTicks++
}

func handleGoalChangeEvent(e *game.SimLogEntry, perf *soldierPerformance, prevGoal map[string]string) {
	if e.Key != "change" {
		return
	}
	from, to, ok := parseTransition(e.Value)
	if !ok {
		return
	}
	if prev, exists := prevGoal[e.Soldier]; exists && prev != from {
		from = prev
	}
	perf.goalChanges = append(perf.goalChanges, goalChange{tick: e.Tick, fromGoal: from, toGoal: to})
	prevGoal[e.Soldier] = to
}

func handleStateChangeEvent(e *game.SimLogEntry, perf *soldierPerformance, prevState map[string]string) {
	if e.Key != "change" {
		return
	}
	from, to, ok := parseTransition(e.Value)
	if !ok {
		return
	}
	if prev, exists := prevState[e.Soldier]; exists && prev != from {
		from = prev
	}
	perf.stateChanges = append(perf.stateChanges, stateChange{tick: e.Tick, fromState: from, toState: to})
	prevState[e.Soldier] = to
}

func parseTransition(value string) (string, string, bool) {
	parts := strings.Split(value, " → ")
	if len(parts) != 2 {
		return "", "", false
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), true
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
	_ = entries
	_ = ticks
	// Track which soldiers are frequently near each other during stalled periods
	// This helps identify soldiers bouncing off each other
	type proximityKey struct {
		soldier1 string
		soldier2 string
	}
	proximityCount := make(map[proximityKey]int)

	for label, perf := range perfMap {
		if len(perf.stalledEvents) == 0 {
			continue
		}

		// Check if this soldier's stalled events coincide with another soldier's
		for otherLabel, otherPerf := range perfMap {
			if label == otherLabel || perf.team != otherPerf.team {
				continue
			}

			overlap := countStallOverlap(perf.stalledEvents, otherPerf.stalledEvents)

			if overlap > 0 {
				key := orderedProximityKey(label, otherLabel)
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
		printRun(&stats)
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
	eventStats := analyzeScenarioEvents(entries, firstDeathTick)

	grades := ts.SoldierGrades()
	redTotal, blueTotal, redSurvivors, blueSurvivors := teamSurvivalCounts(grades)

	soldierPerf := analyzeSoldierPerformance(entries, grades, ticks)
	problematicSoldiers := make([]soldierPerformance, 0, len(soldierPerf))
	for i := range soldierPerf {
		perf := soldierPerf[i]
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
		firstDisobeyOnTick:   eventStats.firstDisobeyOnTick,
		firstPanicOnTick:     eventStats.firstPanicOnTick,
		firstSurrenderOnTick: eventStats.firstSurrenderOnTick,
		intentChanges:        ts.SimLog.CountCategory("squad", "intent_change"),
		goalChanges:          ts.SimLog.CountCategory("goal", "change"),
		stateChanges:         ts.SimLog.CountCategory("state", "change"),
		contactNew:           ts.SimLog.CountCategory("vision", "contact_new"),
		contactLost:          ts.SimLog.CountCategory("vision", "contact_lost"),
		stalledEvents:        eventStats.stalledEvents,
		detachedEvents:       eventStats.detachedEvents,
		disobeyEvents:        eventStats.disobeyEvents,
		panicEvents:          eventStats.panicEvents,
		surrenderEvents:      eventStats.surrenderEvents,
		disobeyOnEvents:      eventStats.disobeyOnEvents,
		disobeyOffEvents:     eventStats.disobeyOffEvents,
		panicOnEvents:        eventStats.panicOnEvents,
		panicOffEvents:       eventStats.panicOffEvents,
		surrenderOnEvents:    eventStats.surrenderOnEvents,
		surrenderOffEvents:   eventStats.surrenderOffEvents,
		disobeyOnPreDeath:    eventStats.disobeyOnPreDeath,
		panicOnPreDeath:      eventStats.panicOnPreDeath,
		surrenderOnPreDeath:  eventStats.surrenderOnPreDeath,
		cohesionBreakEvents:  eventStats.cohesionBreakEvents,
		cohesionReformEvents: eventStats.cohesionReformEvents,
		affected:             eventStats.affected,
		peakRefusing:         eventStats.peakRefusing,
		peakRefusingRed:      eventStats.peakRefusingRed,
		peakRefusingBlue:     eventStats.peakRefusingBlue,
		peakRefusingTick:     eventStats.peakRefusingTick,
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
	if ts.NavGrid != nil {
		pfStats := ts.NavGrid.PathfindingStats()
		rs.pfCalls = pfStats.CallCount
		rs.pfFails = pfStats.FailCount
		rs.pfExpandedNodes = pfStats.ExpandedNodes
		rs.pfAvgRuntimeMS = pfStats.AverageRuntimeMS
		rs.pfP95RuntimeMS = pfStats.P95RuntimeMS
		rs.pfFailRatePct = pfStats.FailureRatePercent
	}
	rs.stalemate, rs.stalemateReason = detectStalemate(&rs)

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

func printProblematicSoldiers(rs *runStats) {
	if len(rs.problematicSoldiers) == 0 {
		return
	}

	fmt.Printf("\n=== PROBLEMATIC SOLDIERS (Run %d, seed=%d) ===\n", rs.runIndex, rs.seed)
	fmt.Printf("Found %d soldiers with significant performance issues:\n\n", len(rs.problematicSoldiers))

	for i := range rs.problematicSoldiers {
		perf := &rs.problematicSoldiers[i]
		issues := performanceIssues(perf)

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

		printGoalPattern(perf)
		printStatePattern(perf)

		// Stalled event analysis
		if len(perf.stalledEvents) > 0 {
			fmt.Printf("  Stalled events: %d occurrences\n", len(perf.stalledEvents))
			samples := sampleStalledEvents(perf.stalledEvents)
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
			samples := sampleDetachedEvents(perf.detachedEvents)
			for _, de := range samples {
				fmt.Printf("    tick=%d leader_dist=%.1f goal=%s intent=%s\n", de.tick, de.leaderDist, de.goal, de.intent)
			}

			if len(perf.detachedEvents) > 3 {
				fmt.Printf("    ... (%d more detached events)\n", len(perf.detachedEvents)-3)
			}
		}

		fmt.Printf("  DIAGNOSIS: %s\n\n", strings.Join(buildDiagnoses(perf), "; "))
	}
}

func performanceIssues(perf *soldierPerformance) []string {
	issues := make([]string, 0, 4)
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
	return issues
}

func printGoalPattern(perf *soldierPerformance) {
	if len(perf.goalChanges) == 0 {
		return
	}
	goalFreq := make(map[string]int)
	for _, gc := range perf.goalChanges {
		goalFreq[gc.toGoal]++
	}
	topGoals := []string{}
	for goal, count := range goalFreq {
		if count > 2 || len(goalFreq) <= 3 {
			topGoals = append(topGoals, fmt.Sprintf("%s(%d)", goal, count))
		}
	}
	if len(topGoals) > 0 {
		fmt.Printf("  Goal pattern: %d changes, frequent: %s\n", len(perf.goalChanges), strings.Join(topGoals, ", "))
	}
	if len(perf.goalChanges) <= 10 {
		return
	}
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

func printStatePattern(perf *soldierPerformance) {
	if len(perf.stateChanges) == 0 {
		return
	}
	stateFreq := make(map[string]int)
	for _, sc := range perf.stateChanges {
		stateFreq[sc.toState]++
	}
	idleCount := stateFreq["idle"]
	coverCount := stateFreq["cover"]
	if idleCount > 5 && coverCount > 5 {
		fmt.Printf("  IDLE/COVER LOOP: %d idle, %d cover transitions (possible stuck behavior)\n", idleCount, coverCount)
	}
}

func sampleStalledEvents(events []stalledEvent) []stalledEvent {
	if len(events) <= 3 {
		return events
	}
	return []stalledEvent{events[0], events[len(events)/2], events[len(events)-1]}
}

func sampleDetachedEvents(events []detachedEvent) []detachedEvent {
	if len(events) <= 3 {
		return events
	}
	return []detachedEvent{events[0], events[len(events)/2], events[len(events)-1]}
}

func buildDiagnoses(perf *soldierPerformance) []string {
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
	if len(perf.stalledEvents) > 0 {
		switch perf.stalledEvents[0].goal {
		case "regroup":
			diagnoses = append(diagnoses, "Stuck during regroup")
		case "move_to_contact":
			diagnoses = append(diagnoses, "Pathfinding/movement failure")
		}
	}
	if len(diagnoses) == 0 {
		return []string{"Unknown - review event patterns above"}
	}
	return diagnoses
}

func teamLabel(team game.Team) string {
	if team == game.TeamRed {
		return "red"
	}
	return "blue"
}

func printRun(rs *runStats) {
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
	fmt.Printf("pathfinding: calls=%d expanded_nodes=%d fail_rate=%.1f%% avg_ms=%.3f p95_ms=%.3f\n",
		rs.pfCalls, rs.pfExpandedNodes, rs.pfFailRatePct, rs.pfAvgRuntimeMS, rs.pfP95RuntimeMS)
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

type soldierAgg struct {
	good     map[string]int
	bad      map[string]int
	scoreSum float64
	count    int
	survived int
}

type aggregateData struct {
	affectedGlobal map[string]struct{}
	soldierAggs    map[string]*soldierAgg
	contactTicks   []int
	engageTicks    []int
	deathTicks     []int
	panicTicks     []int
	surrenderTicks []int
	breakTicks     []int

	totalStalled             int
	totalDetached            int
	totalDisobey             int
	totalPanic               int
	totalSurrender           int
	totalDisobeyOn           int
	totalDisobeyOff          int
	totalPanicOn             int
	totalPanicOff            int
	totalSurrenderOn         int
	totalSurrenderOff        int
	totalDisobeyOnPreDeath   int
	totalPanicOnPreDeath     int
	totalSurrenderOnPreDeath int
	totalPeakRefusing        int
	totalPeakRefusingRed     int
	totalPeakRefusingBlue    int
	totalBreak               int
	totalReform              int
	totalIntent              int
	totalGoal                int
	totalState               int
	totalContactNew          int
	totalContactLost         int
	totalRedSurvivors        int
	totalBlueSurvivors       int
	totalRedSoldiers         int
	totalBlueSoldiers        int
	stalemateRuns            int
	redVictories             int
	blueVictories            int
	draws                    int
	inconclusives            int
}

func newAggregateData(runs int) *aggregateData {
	return &aggregateData{
		contactTicks:   make([]int, 0, runs),
		engageTicks:    make([]int, 0, runs),
		deathTicks:     make([]int, 0, runs),
		panicTicks:     make([]int, 0, runs),
		surrenderTicks: make([]int, 0, runs),
		breakTicks:     make([]int, 0, runs),
		affectedGlobal: map[string]struct{}{},
		soldierAggs:    map[string]*soldierAgg{},
	}
}

func accumulateRun(rs *runStats, agg *aggregateData) {
	accumulateCounters(rs, agg)
	accumulateOutcome(rs, agg)
	appendPhaseTicks(rs, agg)
	mergeAffected(rs, agg)
	accumulateGrades(rs, agg)
}

func accumulateCounters(rs *runStats, agg *aggregateData) {
	agg.totalStalled += rs.stalledEvents
	agg.totalDetached += rs.detachedEvents
	agg.totalDisobey += rs.disobeyEvents
	agg.totalPanic += rs.panicEvents
	agg.totalSurrender += rs.surrenderEvents
	agg.totalDisobeyOn += rs.disobeyOnEvents
	agg.totalDisobeyOff += rs.disobeyOffEvents
	agg.totalPanicOn += rs.panicOnEvents
	agg.totalPanicOff += rs.panicOffEvents
	agg.totalSurrenderOn += rs.surrenderOnEvents
	agg.totalSurrenderOff += rs.surrenderOffEvents
	agg.totalDisobeyOnPreDeath += rs.disobeyOnPreDeath
	agg.totalPanicOnPreDeath += rs.panicOnPreDeath
	agg.totalSurrenderOnPreDeath += rs.surrenderOnPreDeath
	agg.totalPeakRefusing += rs.peakRefusing
	agg.totalPeakRefusingRed += rs.peakRefusingRed
	agg.totalPeakRefusingBlue += rs.peakRefusingBlue
	agg.totalBreak += rs.cohesionBreakEvents
	agg.totalReform += rs.cohesionReformEvents
	agg.totalIntent += rs.intentChanges
	agg.totalGoal += rs.goalChanges
	agg.totalState += rs.stateChanges
	agg.totalContactNew += rs.contactNew
	agg.totalContactLost += rs.contactLost
	agg.totalRedSurvivors += rs.redSurvivors
	agg.totalBlueSurvivors += rs.blueSurvivors
	agg.totalRedSoldiers += rs.redTotal
	agg.totalBlueSoldiers += rs.blueTotal
}

func accumulateOutcome(rs *runStats, agg *aggregateData) {
	if rs.stalemate {
		agg.stalemateRuns++
	}
	switch rs.outcome {
	case game.OutcomeRedVictory:
		agg.redVictories++
	case game.OutcomeBlueVictory:
		agg.blueVictories++
	case game.OutcomeDraw:
		agg.draws++
	case game.OutcomeInconclusive:
		agg.inconclusives++
	}
}

func appendPhaseTicks(rs *runStats, agg *aggregateData) {
	if rs.firstContactTick >= 0 {
		agg.contactTicks = append(agg.contactTicks, rs.firstContactTick)
	}
	if rs.firstEngageTick >= 0 {
		agg.engageTicks = append(agg.engageTicks, rs.firstEngageTick)
	}
	if rs.firstDeathTick >= 0 {
		agg.deathTicks = append(agg.deathTicks, rs.firstDeathTick)
	}
	if rs.firstPanicTick >= 0 {
		agg.panicTicks = append(agg.panicTicks, rs.firstPanicTick)
	}
	if rs.firstSurrenderTick >= 0 {
		agg.surrenderTicks = append(agg.surrenderTicks, rs.firstSurrenderTick)
	}
	if rs.firstBreakTick >= 0 {
		agg.breakTicks = append(agg.breakTicks, rs.firstBreakTick)
	}
}

func mergeAffected(rs *runStats, agg *aggregateData) {
	for label := range rs.affected {
		agg.affectedGlobal[label] = struct{}{}
	}
}

func accumulateGrades(rs *runStats, agg *aggregateData) {
	for j := range rs.grades {
		g := rs.grades[j]
		sa, ok := agg.soldierAggs[g.Label]
		if !ok {
			sa = &soldierAgg{good: map[string]int{}, bad: map[string]int{}}
			agg.soldierAggs[g.Label] = sa
		}
		sa.scoreSum += g.Score
		sa.count++
		if g.Survived {
			sa.survived++
		}
		for _, t := range g.GoodTraits {
			sa.good[t]++
		}
		for _, t := range g.BadTraits {
			sa.bad[t]++
		}
	}
}

func printAggregateMetrics(agg *aggregateData, runs int) {
	fmt.Println("=== Aggregate AAR Inputs ===")
	fmt.Printf("runs=%d\n", runs)
	fmt.Printf("avg_events_per_run: intent_change=%.1f goal_change=%.1f state_change=%.1f contact_new=%.1f contact_lost=%.1f\n",
		avg(agg.totalIntent, runs), avg(agg.totalGoal, runs), avg(agg.totalState, runs), avg(agg.totalContactNew, runs), avg(agg.totalContactLost, runs))
	fmt.Printf("avg_effectiveness_per_run: stalled_in_combat=%.1f detached_from_engagement=%.1f\n",
		avg(agg.totalStalled, runs), avg(agg.totalDetached, runs))
	fmt.Printf("avg_psych_events_per_run: disobedience=%.1f panic_retreat=%.1f surrender=%.1f squad_break=%.1f squad_reform=%.1f\n",
		avg(agg.totalDisobey, runs), avg(agg.totalPanic, runs), avg(agg.totalSurrender, runs), avg(agg.totalBreak, runs), avg(agg.totalReform, runs))
	fmt.Printf("avg_psych_refusal_transitions_per_run: disobey_on=%.1f disobey_off=%.1f panic_on=%.1f panic_off=%.1f surrender_on=%.1f surrender_off=%.1f\n",
		avg(agg.totalDisobeyOn, runs), avg(agg.totalDisobeyOff, runs), avg(agg.totalPanicOn, runs), avg(agg.totalPanicOff, runs), avg(agg.totalSurrenderOn, runs), avg(agg.totalSurrenderOff, runs))
	fmt.Printf("avg_psych_refusal_onsets_pre_first_death_per_run: disobey_on=%.1f panic_on=%.1f surrender_on=%.1f\n",
		avg(agg.totalDisobeyOnPreDeath, runs), avg(agg.totalPanicOnPreDeath, runs), avg(agg.totalSurrenderOnPreDeath, runs))
	fmt.Printf("avg_peak_refusing_per_run: total=%.1f red=%.1f blue=%.1f\n",
		avg(agg.totalPeakRefusing, runs), avg(agg.totalPeakRefusingRed, runs), avg(agg.totalPeakRefusingBlue, runs))
	fmt.Printf("phase_marker_avg_ticks: first_contact=%s first_engage=%s first_death=%s first_panic=%s first_surrender=%s first_break=%s\n",
		avgTickString(agg.contactTicks), avgTickString(agg.engageTicks), avgTickString(agg.deathTicks), avgTickString(agg.panicTicks), avgTickString(agg.surrenderTicks), avgTickString(agg.breakTicks))
	fmt.Printf("unique_affected_labels=%d [%s]\n", len(agg.affectedGlobal), joinSet(agg.affectedGlobal))
	fmt.Printf("stalemate_runs=%d/%d (%.1f%%)\n", agg.stalemateRuns, runs, avg(agg.stalemateRuns*100, runs))
	fmt.Printf("battle_outcomes: red_victories=%d blue_victories=%d draws=%d inconclusive=%d\n",
		agg.redVictories, agg.blueVictories, agg.draws, agg.inconclusives)
	fmt.Printf("outcome_percentages: red=%.1f%% blue=%.1f%% draw=%.1f%% inconclusive=%.1f%%\n",
		avg(agg.redVictories*100, runs), avg(agg.blueVictories*100, runs), avg(agg.draws*100, runs), avg(agg.inconclusives*100, runs))
	if agg.totalRedSoldiers > 0 && agg.totalBlueSoldiers > 0 {
		fmt.Printf("survival_rate: red=%.1f%% blue=%.1f%%\n",
			float64(agg.totalRedSurvivors)/float64(agg.totalRedSoldiers)*100,
			float64(agg.totalBlueSurvivors)/float64(agg.totalBlueSoldiers)*100,
		)
	}
}

func printAggregate(all []runStats) {
	printPerfSummary(all)

	agg := newAggregateData(len(all))

	for i := range all {
		accumulateRun(&all[i], agg)
	}

	printAggregateMetrics(agg, len(all))

	printAggregateSoldierPerformance(agg)

	// Team-level aggregate from last run's grades as representative.
	if len(all) > 0 {
		fmt.Println("\n--- Team Summary (across all runs) ---")
		fmt.Print(game.FormatGradesSummary(collectAllGrades(all)))
	}
}

func avg(sum, n int) float64 {
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
	for i := range all {
		rs := all[i]
		out = append(out, rs.grades...)
	}
	return out
}

func teamSurvivalCounts(grades []game.SoldierGrade) (redTotal, blueTotal, redSurvivors, blueSurvivors int) {
	for i := range grades {
		g := grades[i]
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

func detectStalemate(rs *runStats) (bool, string) {
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
