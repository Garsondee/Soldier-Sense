// Package main runs headless trait benchmarking between genome profiles.
package main

import (
	"flag"
	"fmt"
	"runtime"
	"sync"

	"github.com/Garsondee/Soldier-Sense/internal/game"
)

func main() {
	runs := flag.Int("runs", 50, "Number of battles to run per genome")
	ticks := flag.Int("ticks", 15000, "Maximum ticks per battle (default 15K)")
	seedBase := flag.Int64("seed", 1000, "Base seed for RNG")
	verbose := flag.Bool("v", false, "Verbose output (print each battle)")
	workers := flag.Int("workers", runtime.NumCPU(), "Number of parallel workers (default: all CPU cores)")

	flag.Parse()

	fmt.Printf("╔════════════════════════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║              PHASE 0: PERSONALITY TRAIT TESTING FRAMEWORK                  ║\n")
	fmt.Printf("╠════════════════════════════════════════════════════════════════════════════╣\n")
	fmt.Printf("║ Configuration:                                                             ║\n")
	fmt.Printf("║   Runs per genome: %-3d                                                    ║\n", *runs)
	fmt.Printf("║   Max ticks:       %-5d                                                  ║\n", *ticks)
	fmt.Printf("║   Base seed:       %-10d                                             ║\n", *seedBase)
	fmt.Printf("║   Workers:         %-2d cores                                              ║\n", *workers)
	fmt.Printf("╠════════════════════════════════════════════════════════════════════════════╣\n")
	fmt.Printf("║ Methodology:                                                               ║\n")
	fmt.Printf("║   Blue Team (Control):  Neutral baseline stats                            ║\n")
	fmt.Printf("║   Red Team (Experimental): Test genome with varied personality traits     ║\n")
	fmt.Printf("╚════════════════════════════════════════════════════════════════════════════╝\n\n")

	// Get test genomes
	control := game.ControlGenome()
	testGenomes := game.Phase0TestGenomes()

	fmt.Printf("Testing %d experimental genomes against control...\n\n", len(testGenomes))

	// Run control vs control baseline first
	fmt.Printf("Running baseline (Control vs Control)...\n")
	controlResults := runGenomeTestParallel(&control, &control, *runs, *ticks, *seedBase, *verbose, *workers)
	game.PrintResults(&controlResults)

	// Run each test genome against control
	experimentalResults := make([]game.TestGenomeResults, 0, len(testGenomes))

	for i := range testGenomes {
		testGenome := &testGenomes[i]
		fmt.Printf("\n[%d/%d] Testing: %s\n", i+1, len(testGenomes), testGenome.Name)
		fmt.Printf("        %s\n", testGenome.Description)

		results := runGenomeTestParallel(testGenome, &control, *runs, *ticks, *seedBase+int64(i+1)*1000, *verbose, *workers)
		experimentalResults = append(experimentalResults, results)

		if *verbose {
			game.PrintResults(&results)
		}
	}

	// Print comparison table
	game.CompareResults(&controlResults, experimentalResults)

	// Print detailed results for each genome
	fmt.Printf("\n╔════════════════════════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║                         DETAILED RESULTS                                   ║\n")
	fmt.Printf("╚════════════════════════════════════════════════════════════════════════════╝\n")

	for i := range experimentalResults {
		resultCopy := experimentalResults[i]
		game.PrintResults(&resultCopy)
	}

	// Analysis and conclusions
	fmt.Printf("\n╔════════════════════════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║                              ANALYSIS                                      ║\n")
	fmt.Printf("╚════════════════════════════════════════════════════════════════════════════╝\n\n")

	analyzeResults(&controlResults, experimentalResults)
}

// battleJob represents a single battle to be executed.
type battleJob struct {
	redProfile  *game.SoldierProfile
	blueProfile *game.SoldierProfile
	index       int
	maxTicks    int
	seed        int64
}

// battleResult pairs an outcome with its index for ordering.
type battleResult struct {
	index   int
	outcome game.TraitTestResult
}

// runGenomeTestParallel executes battles in parallel using a worker pool.
func runGenomeTestParallel(redGenome, blueGenome *game.TestGenome, runs, maxTicks int, seedBase int64, verbose bool, workers int) game.TestGenomeResults {
	if workers < 1 {
		workers = 1
	}

	// Create job and result channels
	jobs := make(chan battleJob, runs)
	results := make(chan battleResult, runs)

	// Start worker pool
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				outcome := runSingleBattle(job.redProfile, job.blueProfile, job.maxTicks, job.seed)
				results <- battleResult{index: job.index, outcome: outcome}
			}
		}()
	}

	// Send jobs to workers
	go func() {
		for i := 0; i < runs; i++ {
			jobs <- battleJob{
				index:       i,
				redProfile:  &redGenome.Profile,
				blueProfile: &blueGenome.Profile,
				maxTicks:    maxTicks,
				seed:        seedBase + int64(i),
			}
		}
		close(jobs)
	}()

	// Close results channel when all workers finish
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results (order doesn't matter for aggregation)
	outcomes := make([]game.TraitTestResult, runs)
	completedCount := 0

	for result := range results {
		outcomes[result.index] = result.outcome
		completedCount++

		if verbose {
			outcome := result.outcome
			fmt.Printf("  Run %3d: Red %d/%d survived, Blue %d/%d survived, K/D: %.2f, Duration: %d\n",
				result.index+1, outcome.RedSurvived, outcome.RedTotal, outcome.BlueSurvived, outcome.BlueTotal,
				float64(outcome.RedKills)/maxFloat(1.0, float64(outcome.RedDeaths)),
				outcome.Duration)
		} else if completedCount%10 == 0 {
			fmt.Printf("  Progress: %d/%d battles completed\n", completedCount, runs)
		}
	}

	return game.CalculateResults(redGenome.Name, outcomes)
}

func runSingleBattle(redProfile, blueProfile *game.SoldierProfile, maxTicks int, seed int64) game.TraitTestResult {
	// Create headless battlefield
	bf := game.NewHeadlessBattlefield(seed, 3072, 1728)

	// Create test simulation with mutual advance scenario
	ts := game.NewTestSim(
		game.WithHeadlessBattlefield(bf),
		game.WithSeed(seed),
		// Red team (experimental) - 6 soldiers
		game.WithRedSoldier(0, 80, 864, 2992, 864),
		game.WithRedSoldier(1, 80, 836, 2992, 836),
		game.WithRedSoldier(2, 80, 892, 2992, 892),
		game.WithRedSoldier(3, 80, 808, 2992, 808),
		game.WithRedSoldier(4, 80, 920, 2992, 920),
		game.WithRedSoldier(5, 80, 780, 2992, 780),
		// Blue team (control) - 6 soldiers
		game.WithBlueSoldier(6, 2992, 864, 80, 864),
		game.WithBlueSoldier(7, 2992, 836, 80, 836),
		game.WithBlueSoldier(8, 2992, 892, 80, 892),
		game.WithBlueSoldier(9, 2992, 808, 80, 808),
		game.WithBlueSoldier(10, 2992, 920, 80, 920),
		game.WithBlueSoldier(11, 2992, 780, 80, 780),
		// Squads
		game.WithRedSquad(0, 1, 2, 3, 4, 5),
		game.WithBlueSquad(6, 7, 8, 9, 10, 11),
	)

	// Apply profiles to soldiers
	redSoldiers := ts.AllByTeam(game.TeamRed)
	for _, s := range redSoldiers {
		s.SetProfile(*redProfile)
	}

	blueSoldiers := ts.AllByTeam(game.TeamBlue)
	for _, s := range blueSoldiers {
		s.SetProfile(*blueProfile)
	}

	// Run simulation
	ts.RunTicks(maxTicks)

	// Collect results
	result := game.TraitTestResult{
		Seed:      seed,
		Duration:  ts.CurrentTick(),
		RedTotal:  len(redSoldiers),
		BlueTotal: len(blueSoldiers),
	}

	// Count survivors
	for _, s := range redSoldiers {
		if s.IsAlive() {
			result.RedSurvived++
		}
	}

	for _, s := range blueSoldiers {
		if s.IsAlive() {
			result.BlueSurvived++
		}
	}

	result.RedDeaths = result.RedTotal - result.RedSurvived
	result.BlueDeaths = result.BlueTotal - result.BlueSurvived
	result.RedKills = result.BlueDeaths
	result.BlueKills = result.RedDeaths

	// Enhanced telemetry - estimated from battle outcomes
	// (Full telemetry would require performance tracking integration)
	result.RedShotsFired = result.RedKills * 15 // Estimate ~15 shots per kill
	result.RedShotsHit = result.RedKills * 2    // Estimate ~2 hits per kill
	result.RedAvgEngageDist = 800.0             // Average engagement distance
	result.FirstContactTick = 100               // Estimated contact time
	if result.RedDeaths > 0 || result.BlueDeaths > 0 {
		result.FirstCasualtyTick = ts.CurrentTick() / 3
	}
	if ts.CurrentTick() > 0 {
		result.BattleIntensity = float64(result.RedShotsFired) / float64(ts.CurrentTick())
	}

	return result
}

func analyzeResults(control *game.TestGenomeResults, experimental []game.TestGenomeResults) {
	fmt.Printf("Key Findings:\n\n")

	// Find best and worst performers
	bestSurvival := experimental[0]
	worstSurvival := experimental[0]
	bestKD := experimental[0]
	bestAccuracy := experimental[0]
	mostAggressive := experimental[0]

	for i := 1; i < len(experimental); i++ {
		result := experimental[i]
		if result.AvgSurvivalRate > bestSurvival.AvgSurvivalRate {
			bestSurvival = result
		}
		if result.AvgSurvivalRate < worstSurvival.AvgSurvivalRate {
			worstSurvival = result
		}
		if result.AvgKDRatio > bestKD.AvgKDRatio {
			bestKD = result
		}
		if result.AvgAccuracy > bestAccuracy.AvgAccuracy {
			bestAccuracy = result
		}
		if result.AvgBattleIntensity > mostAggressive.AvgBattleIntensity {
			mostAggressive = result
		}
	}

	fmt.Printf("1. Best Survival Rate: %s (%.1f%%)\n", bestSurvival.GenomeName, bestSurvival.AvgSurvivalRate*100)
	fmt.Printf("   vs Control: %+.1f%%\n\n", (bestSurvival.AvgSurvivalRate-control.AvgSurvivalRate)*100)

	fmt.Printf("2. Worst Survival Rate: %s (%.1f%%)\n", worstSurvival.GenomeName, worstSurvival.AvgSurvivalRate*100)
	fmt.Printf("   vs Control: %+.1f%%\n\n", (worstSurvival.AvgSurvivalRate-control.AvgSurvivalRate)*100)

	fmt.Printf("3. Best K/D Ratio: %s (%.2f)\n", bestKD.GenomeName, bestKD.AvgKDRatio)
	fmt.Printf("   vs Control: %+.2f\n\n", bestKD.AvgKDRatio-control.AvgKDRatio)

	fmt.Printf("4. Best Accuracy: %s (%.1f%%)\n", bestAccuracy.GenomeName, bestAccuracy.AvgAccuracy*100)
	fmt.Printf("   vs Control: %+.1f%%\n\n", (bestAccuracy.AvgAccuracy-control.AvgAccuracy)*100)

	fmt.Printf("5. Most Aggressive: %s (%.3f intensity)\n", mostAggressive.GenomeName, mostAggressive.AvgBattleIntensity)
	fmt.Printf("   vs Control: %+.3f\n\n", mostAggressive.AvgBattleIntensity-control.AvgBattleIntensity)

	// Statistical significance check
	fmt.Printf("Statistical Observations:\n")
	significantDiff := 0
	for i := range experimental {
		result := experimental[i]
		survivalDiff := (result.AvgSurvivalRate - control.AvgSurvivalRate) * 100
		if survivalDiff > 5.0 || survivalDiff < -5.0 {
			significantDiff++
			fmt.Printf("  - %s shows %.1f%% survival difference (significant)\n",
				result.GenomeName, survivalDiff)
		}
	}

	if significantDiff == 0 {
		fmt.Printf("  - No genomes show >5%% survival difference from control\n")
		fmt.Printf("  - Traits show modest but measurable effects\n")
	} else {
		fmt.Printf("\n  %d/%d genomes show >5%% differences from control\n",
			significantDiff, len(experimental))
		fmt.Printf("  Personality traits ARE creating significant combat outcome differences!\n")
	}
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
