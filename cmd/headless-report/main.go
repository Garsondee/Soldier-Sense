package main

import (
	"flag"
	"fmt"
	"sort"
	"strings"

	"github.com/Garsondee/Soldier-Sense/internal/game"
)

const (
	stalemateMinTeamSurvivalRate   = 0.50
	stalemateMinFrictionPerSoldier = 2.0
)

type runStats struct {
	runIndex int
	seed     int64
	ticks    int

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
	ts := game.NewTestSim(
		game.WithMapSize(1280, 720),
		game.WithSeed(seed),
		game.WithRedSoldier(0, 50, 350, 1200, 350),
		game.WithRedSoldier(1, 50, 322, 1200, 322),
		game.WithRedSoldier(2, 50, 378, 1200, 378),
		game.WithRedSoldier(3, 50, 294, 1200, 294),
		game.WithRedSoldier(4, 50, 406, 1200, 406),
		game.WithRedSoldier(5, 50, 266, 1200, 266),
		game.WithBlueSoldier(6, 1200, 350, 50, 350),
		game.WithBlueSoldier(7, 1200, 322, 50, 322),
		game.WithBlueSoldier(8, 1200, 378, 50, 378),
		game.WithBlueSoldier(9, 1200, 294, 50, 294),
		game.WithBlueSoldier(10, 1200, 406, 50, 406),
		game.WithBlueSoldier(11, 1200, 266, 50, 266),
		game.WithRedSquad(0, 1, 2, 3, 4, 5),
		game.WithBlueSquad(6, 7, 8, 9, 10, 11),
	)
	ts.RunTicks(ticks)

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

func printRun(rs runStats) {
	fmt.Printf("--- Run %d (seed=%d) ---\n", rs.runIndex, rs.seed)
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
	fmt.Println()
}

func printAggregate(all []runStats) {
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
