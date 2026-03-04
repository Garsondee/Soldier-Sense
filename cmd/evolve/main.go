// Package main runs evolutionary optimisation for Soldier-Sense soldier traits.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Garsondee/Soldier-Sense/internal/game"
)

func main() {
	// Command-line flags
	popSize := flag.Int("pop", 50, "Population size")
	generations := flag.Int("gen", 100, "Number of generations")
	battlesPerGenome := flag.Int("battles", 20, "Battles per genome for fitness evaluation")
	ticks := flag.Int("ticks", 15000, "Maximum ticks per battle")
	workers := flag.Int("workers", runtime.NumCPU(), "Number of parallel workers")
	seed := flag.Int64("seed", time.Now().UnixNano(), "Random seed for evolution")
	fitnessType := flag.String("fitness", "regular", "Fitness function: regular (recommended), default, aggressive, defensive, balanced, survival, operational, zero-casualty, cost-efficiency, recruit, elite")
	crossoverRate := flag.Float64("crossover", 0.8, "Crossover rate (0.0-1.0)")
	mutationRate := flag.Float64("mutation", 0.15, "Mutation rate per gene (0.0-1.0)")
	mutationSigma := flag.Float64("sigma", 0.1, "Mutation strength (standard deviation)")
	tournamentSize := flag.Int("tournament", 3, "Tournament selection size")
	eliteCount := flag.Int("elite", 5, "Number of elite genomes to preserve")
	logFile := flag.String("log", "evolution.log", "Log file for evolution history")

	reportFile := flag.String("report", "evolution_report.txt", "Detailed report file for post-analysis")

	flag.Parse()

	// Print header
	fmt.Printf("╔════════════════════════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║           PHASE 1: EVOLUTIONARY SOLDIER OPTIMIZATION                       ║\n")
	fmt.Printf("╠════════════════════════════════════════════════════════════════════════════╣\n")
	fmt.Printf("║ Evolution Configuration:                                                   ║\n")
	fmt.Printf("║   Population size:     %-3d                                                ║\n", *popSize)
	fmt.Printf("║   Generations:         %-3d                                                ║\n", *generations)
	fmt.Printf("║   Battles per genome:  %-3d                                                ║\n", *battlesPerGenome)
	fmt.Printf("║   Ticks per battle:    %-5d                                              ║\n", *ticks)
	fmt.Printf("║   Workers:             %-2d cores                                          ║\n", *workers)
	fmt.Printf("║   Fitness function:    %-10s                                         ║\n", *fitnessType)
	fmt.Printf("╠════════════════════════════════════════════════════════════════════════════╣\n")
	fmt.Printf("║ Genetic Operators:                                                         ║\n")
	fmt.Printf("║   Crossover rate:      %.2f                                                 ║\n", *crossoverRate)
	fmt.Printf("║   Mutation rate:       %.2f                                                 ║\n", *mutationRate)
	fmt.Printf("║   Mutation sigma:      %.2f                                                 ║\n", *mutationSigma)
	fmt.Printf("║   Tournament size:     %-2d                                                 ║\n", *tournamentSize)
	fmt.Printf("║   Elite count:         %-2d                                                 ║\n", *eliteCount)
	fmt.Printf("╠════════════════════════════════════════════════════════════════════════════╣\n")
	fmt.Printf("║ Seed: %-20d                                                 ║\n", *seed)
	fmt.Printf("║ Log:  %-68s ║\n", *logFile)
	fmt.Printf("╚════════════════════════════════════════════════════════════════════════════╝\n\n")

	// Select fitness function (cost-aware functions are applied during evaluation).
	fitnessFunc := selectFitnessFunction(*fitnessType)

	// Set up genetic operators
	operators := game.GeneticOperators{
		CrossoverRate:  *crossoverRate,
		MutationRate:   *mutationRate,
		MutationSigma:  *mutationSigma,
		TournamentSize: *tournamentSize,
		EliteCount:     *eliteCount,
	}

	// Open log file
	logF, err := os.Create(*logFile)
	if err != nil {
		fmt.Printf("Error creating log file: %v\n", err)
		return
	}
	defer func() {
		if closeErr := logF.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to close %s: %v\n", *logFile, closeErr)
		}
	}()

	// Write log header.
	if _, err = fmt.Fprintf(logF, "Generation,BestFitness,AvgFitness,BestSurvival,BestKD,BestWinRate,BestCost,BestCategory,BestCostAdjFitness,"); err != nil {
		fmt.Printf("Error writing log header: %v\n", err)
		return
	}
	for i, geneDef := range game.GeneDefinitions {
		if _, err = fmt.Fprintf(logF, "Best%s", geneDef.Name); err != nil {
			fmt.Printf("Error writing log header: %v\n", err)
			return
		}
		if i < len(game.GeneDefinitions)-1 {
			if _, err = fmt.Fprintf(logF, ","); err != nil {
				fmt.Printf("Error writing log header: %v\n", err)
				return
			}
		}
	}
	if _, err = fmt.Fprintf(logF, "\n"); err != nil {
		fmt.Printf("Error writing log header: %v\n", err)
		return
	}

	// Initialize population
	fmt.Printf("Initializing population of %d genomes...\n", *popSize)
	population := game.NewPopulation(*popSize, *seed)
	rng := rand.New(rand.NewSource(*seed))

	// Control genome for comparison
	control := game.ControlGenome()

	// Evolution loop
	startTime := time.Now()

	for gen := 0; gen < *generations; gen++ {
		genStart := time.Now()

		fmt.Printf("\n╔════════════════════════════════════════════════════════════════════════════╗\n")
		fmt.Printf("║ Generation %3d / %3d                                                        ║\n", gen+1, *generations)
		fmt.Printf("╚════════════════════════════════════════════════════════════════════════════╝\n")

		// Evaluate fitness for all genomes in parallel
		fmt.Printf("Evaluating fitness for %d genomes...\n", len(population.Genomes))
		evaluateFitnessParallelWithCost(&population, &control, *battlesPerGenome, *ticks, *workers, fitnessFunc, *fitnessType, rng.Int63())

		// Update statistics
		population.UpdateStatistics()

		printGenerationReport(gen+1, &population)

		// Log to file
		if err := logGeneration(logF, &population); err != nil {
			fmt.Printf("Error writing generation log: %v\n", err)
			return
		}

		// Time estimate and progress bar
		genDuration := time.Since(genStart)
		remaining := time.Duration((*generations - gen - 1)) * genDuration
		progress := float64(gen+1) / float64(*generations) * 100
		fmt.Printf("\nGeneration time: %v | Remaining: %v | Progress: %.1f%%\n",
			genDuration.Round(time.Second), remaining.Round(time.Second), progress)

		// Evolve to next generation (except on last generation)
		if gen < *generations-1 {
			fmt.Printf("Evolving to generation %d...\n", gen+2)
			population = game.Evolve(&population, operators, rng)
		}
	}

	totalTime := time.Since(startTime)

	// Final summary
	fmt.Printf("\n╔════════════════════════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║                         EVOLUTION COMPLETE                                 ║\n")
	fmt.Printf("╠════════════════════════════════════════════════════════════════════════════╣\n")
	fmt.Printf("║ Total time:        %v                                                  ║\n", totalTime.Round(time.Second))
	fmt.Printf("║ Generations:       %d                                                      ║\n", *generations)
	fmt.Printf("║ Total battles:     %d                                                   ║\n", *popSize**battlesPerGenome**generations)
	fmt.Printf("║ Final best fitness: %.4f                                                   ║\n", population.BestFitness)
	fmt.Printf("╚════════════════════════════════════════════════════════════════════════════╝\n\n")

	// Generate detailed report file
	fmt.Printf("\nGenerating detailed report...\n")
	if err := generateDetailedReport(*reportFile, &population, *generations, *popSize, *battlesPerGenome, *ticks, *workers, *fitnessType, totalTime); err != nil {
		fmt.Printf("Error generating report: %v\n", err)
	} else {
		fmt.Printf("Detailed report saved to: %s\n", *reportFile)
	}

	fmt.Printf("Evolution log saved to: %s\n", *logFile)
	fmt.Printf("You can visualize evolution progress with plotting tools.\n")
}

func selectFitnessFunction(fitnessType string) game.FitnessFunction {
	switch fitnessType {
	case "regular":
		return game.SurvivalFitnessFunction
	case "aggressive":
		return game.AggressiveFitnessFunction
	case "defensive":
		return game.DefensiveFitnessFunction
	case "balanced":
		return game.BalancedFitnessFunction
	case "survival":
		return game.SurvivalFitnessFunction
	case "operational":
		return game.OperationalFitnessFunction
	case "zero-casualty":
		return game.ZeroCasualtyFitnessFunction
	default:
		return game.DefaultFitnessFunction
	}
}

// evaluateFitnessParallelWithCost evaluates fitness for all genomes using parallel workers with cost awareness.
func evaluateFitnessParallelWithCost(pop *game.Population, control *game.TestGenome, battles, ticks, workers int, fitnessFunc game.FitnessFunction, fitnessType string, baseSeed int64) {
	type job struct {
		genome *game.Genome
		index  int
	}

	type result struct {
		evaluation game.GenomeEvaluation
		index      int
	}

	jobs := make(chan job, len(pop.Genomes))
	results := make(chan result, len(pop.Genomes))

	// Start workers
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				// Create test genome from evolved genome
				testGenome := game.TestGenome{
					Name:        fmt.Sprintf("Genome_%d", j.genome.ID),
					Description: "Evolved genome",
					Profile:     j.genome.ToProfile(),
				}

				// Run battles and calculate fitness
				seed := baseSeed + int64(j.index)*1000
				battleResults := runBattlesForGenome(&testGenome, control, battles, ticks, seed)
				baseFitness := fitnessFunc(&battleResults)

				// Calculate cost and cost-adjusted fitness
				cost := j.genome.CalculateCost()
				var costAdjustedFitness float64

				switch fitnessType {
				case "regular":
					costAdjustedFitness = game.RegularSoldierFitnessFunction(&battleResults, cost)
				case "cost-efficiency":
					costAdjustedFitness = game.CostEfficiencyFitnessFunction(&battleResults, cost)
				case "recruit":
					costAdjustedFitness = game.RecruitFitnessFunction(&battleResults, cost)
				case "elite":
					costAdjustedFitness = game.EliteFitnessFunction(&battleResults, cost)
				default:
					costAdjustedFitness = baseFitness
				}

				// Create detailed evaluation
				evaluation := game.GenomeEvaluation{
					Genome:              j.genome.Clone(),
					Results:             battleResults,
					Fitness:             baseFitness,
					Cost:                cost,
					CostAdjustedFitness: costAdjustedFitness,
				}

				results <- result{index: j.index, evaluation: evaluation}
			}
		}()
	}

	// Send jobs
	for i := range pop.Genomes {
		jobs <- job{index: i, genome: &pop.Genomes[i]}
	}
	close(jobs)

	// Wait for workers
	go func() {
		wg.Wait()
		close(results)
	}()

	// Initialize evaluations slice
	pop.Evaluations = make([]game.GenomeEvaluation, len(pop.Genomes))

	// Collect results
	for res := range results {
		// Store detailed evaluation
		pop.Evaluations[res.index] = res.evaluation

		// Use cost-adjusted fitness for evolution selection
		if fitnessType == "cost-efficiency" || fitnessType == "recruit" || fitnessType == "regular" {
			pop.Genomes[res.index].Fitness = res.evaluation.CostAdjustedFitness
		} else {
			pop.Genomes[res.index].Fitness = res.evaluation.Fitness
		}
	}
}

func printGenerationReport(gen int, population *game.Population) {
	fmt.Printf("\n┌─ Generation %d Report ─────────────────────────────────────────────────────┐\n", gen)
	fmt.Printf("│ Fitness:        Best %.4f  Avg %.4f                                        │\n", population.BestFitness, population.AvgFitness)
	if len(population.Evaluations) == 0 {
		fmt.Printf("└────────────────────────────────────────────────────────────────────────────┘\n")
		return
	}

	bestEval := population.BestEval
	fmt.Printf("│ Combat:         Survival %.1f%%  K/D %.2f  Wins %.0f%%                       │\n",
		bestEval.Results.AvgSurvivalRate*100, bestEval.Results.AvgKDRatio, bestEval.Results.WinRate*100)
	fmt.Printf("│ Squad:          Kills %.1f  Deaths %.1f  per battle                          │\n",
		bestEval.Results.AvgKills, bestEval.Results.AvgDeaths)

	// Generation-level aggregated telemetry (across all genomes).
	var genNoContact, genFirstContact float64
	var genRedInjured, genBlueInjured, genRedKIA, genBlueKIA float64
	contactSamples := 0
	for i := range population.Evaluations {
		ev := &population.Evaluations[i]
		genNoContact += ev.Results.NoContactRate
		genRedInjured += ev.Results.AvgRedInjured
		genBlueInjured += ev.Results.AvgBlueInjured
		genRedKIA += ev.Results.AvgRedKIA
		genBlueKIA += ev.Results.AvgBlueKIA
		if ev.Results.AvgFirstContact > 0 {
			genFirstContact += ev.Results.AvgFirstContact
			contactSamples++
		}
	}
	den := float64(len(population.Evaluations))
	avgNoContact := 0.0
	avgFirstContact := 0.0
	avgRedInjured := 0.0
	avgBlueInjured := 0.0
	avgRedKIA := 0.0
	avgBlueKIA := 0.0
	if den > 0 {
		avgNoContact = genNoContact / den
		avgRedInjured = genRedInjured / den
		avgBlueInjured = genBlueInjured / den
		avgRedKIA = genRedKIA / den
		avgBlueKIA = genBlueKIA / den
	}
	if contactSamples > 0 {
		avgFirstContact = genFirstContact / float64(contactSamples)
	}
	if avgFirstContact <= 0 {
		fmt.Printf("│ Detection:      NoContact %.1f%%                                             │\n", avgNoContact*100)
	} else {
		fmt.Printf("│ Detection:      NoContact %.1f%%  FirstContact %.0f ticks                     │\n",
			avgNoContact*100, avgFirstContact)
	}
	fmt.Printf("│ Casualties:     Red KIA %.2f  Injured %.2f | Blue KIA %.2f  Injured %.2f     │\n",
		avgRedKIA, avgRedInjured, avgBlueKIA, avgBlueInjured)

	genome := bestEval.Genome
	fmt.Printf("│ Best Genome:    ID %d  Cost %.2f (%s)  Cost-Adj %.4f                         │\n",
		genome.ID, bestEval.Cost, genome.GetCostCategory(), bestEval.CostAdjustedFitness)
	fmt.Printf("│ Traits (key):   Agg %.2f  Caut %.2f  Marks %.2f  Disc %.2f  Init %.2f        │\n",
		genome.Genes[0], genome.Genes[1], genome.Genes[6], genome.Genes[8], genome.Genes[3])
	fmt.Printf("│                CoverSeek %.2f  TacAware %.2f  SitAware %.2f  CoverShare %.2f │\n",
		genome.Genes[25], genome.Genes[10], genome.Genes[21], genome.Genes[34])
	fmt.Printf("└────────────────────────────────────────────────────────────────────────────┘\n")
}

// Global shared battlefield for all evolution battles to eliminate map variance.
var sharedBattlefield *game.HeadlessBattlefield
var sharedBattlefieldInit uint32

// initSharedBattlefield creates the battlefield once for all evolution battles.
func initSharedBattlefield() {
	if atomic.LoadUint32(&sharedBattlefieldInit) == 1 {
		return
	}
	if atomic.CompareAndSwapUint32(&sharedBattlefieldInit, 0, 1) {
		// Use fixed seed for consistent map across all battles
		mapSeed := int64(42)
		sharedBattlefield = game.NewHeadlessBattlefield(mapSeed, 1200, 800)
	}
	// If another goroutine won the race, spin until it's ready.
	for sharedBattlefield == nil {
		runtime.Gosched()
	}
}

// runBattlesForGenome runs multiple battles for a genome and aggregates results.
func runBattlesForGenome(testGenome, control *game.TestGenome, battles, ticks int, baseSeed int64) game.TestGenomeResults {
	initSharedBattlefield() // Ensure battlefield is created once
	outcomes := make([]game.TraitTestResult, battles)

	for i := 0; i < battles; i++ {
		seed := baseSeed + int64(i)
		outcome := runSingleBattle(&testGenome.Profile, &control.Profile, ticks, seed)
		outcomes[i] = outcome
	}

	return game.CalculateResults(testGenome.Name, outcomes)
}

// runSingleBattle executes a single battle simulation.
func runSingleBattle(redProfile, blueProfile *game.SoldierProfile, maxTicks int, seed int64) game.TraitTestResult {
	// Use the shared battlefield to eliminate map generation variance
	bf := sharedBattlefield

	// Red team starts on left (x=150), Blue team on right (x=1050)
	// Teams are 900px apart - close enough for quick engagement
	// Procedurally generated buildings/cover between them block initial LOS
	centerY := 400.0
	ts := game.NewTestSim(
		game.WithHeadlessBattlefield(bf),
		game.WithSeed(seed),
		// Red squad: left side, spread vertically around center
		game.WithRedSoldier(0, 150, centerY, 1050, centerY),
		game.WithRedSoldier(1, 150, centerY-30, 1050, centerY-30),
		game.WithRedSoldier(2, 150, centerY+30, 1050, centerY+30),
		game.WithRedSoldier(3, 150, centerY-60, 1050, centerY-60),
		game.WithRedSoldier(4, 150, centerY+60, 1050, centerY+60),
		game.WithRedSoldier(5, 150, centerY-90, 1050, centerY-90),
		// Blue squad: right side, spread vertically around center
		game.WithBlueSoldier(6, 1050, centerY, 150, centerY),
		game.WithBlueSoldier(7, 1050, centerY-30, 150, centerY-30),
		game.WithBlueSoldier(8, 1050, centerY+30, 150, centerY+30),
		game.WithBlueSoldier(9, 1050, centerY-60, 150, centerY-60),
		game.WithBlueSoldier(10, 1050, centerY+60, 150, centerY+60),
		game.WithBlueSoldier(11, 1050, centerY-90, 150, centerY-90),
		game.WithRedSquad(0, 1, 2, 3, 4, 5),
		game.WithBlueSquad(6, 7, 8, 9, 10, 11),
	)

	redSoldiers := ts.AllByTeam(game.TeamRed)
	for _, s := range redSoldiers {
		s.SetProfile(*redProfile)
	}

	blueSoldiers := ts.AllByTeam(game.TeamBlue)
	for _, s := range blueSoldiers {
		s.SetProfile(*blueProfile)
	}

	firstContactTick := -1
	firstCasualtyTick := -1

	// Run simulation (no per-battle logging; collect telemetry only).
	for tick := 0; tick < maxTicks; tick++ {
		ts.RunTicks(1)

		if firstContactTick < 0 && hasVisibleThreat(redSoldiers, blueSoldiers) {
			firstContactTick = tick
		}

		if firstCasualtyTick < 0 && hasAnyDead(redSoldiers, blueSoldiers) {
			firstCasualtyTick = tick
		}
	}

	// Final battle summary
	finalRedAlive := 0
	finalBlueAlive := 0
	for _, s := range redSoldiers {
		if s.IsAlive() {
			finalRedAlive++
		}
	}
	for _, s := range blueSoldiers {
		if s.IsAlive() {
			finalBlueAlive++
		}
	}

	redInjured := countAliveInjured(redSoldiers)
	blueInjured := countAliveInjured(blueSoldiers)

	result := game.TraitTestResult{
		Seed:         seed,
		Duration:     ts.CurrentTick(),
		RedTotal:     len(redSoldiers),
		BlueTotal:    len(blueSoldiers),
		RedSurvived:  finalRedAlive,
		BlueSurvived: finalBlueAlive,
		RedDeaths:    len(redSoldiers) - finalRedAlive,
		BlueDeaths:   len(blueSoldiers) - finalBlueAlive,
		RedInjured:   redInjured,
		BlueInjured:  blueInjured,
	}

	result.RedKills = result.BlueDeaths
	result.BlueKills = result.RedDeaths

	// Enhanced telemetry with lightweight estimates
	result.RedShotsFired = result.RedKills * 15
	result.RedShotsHit = result.RedKills * 2
	result.RedAvgEngageDist = 800.0
	result.FirstContactTick = firstContactTick
	result.FirstCasualtyTick = firstCasualtyTick
	if ts.CurrentTick() > 0 {
		result.BattleIntensity = float64(result.RedShotsFired) / float64(ts.CurrentTick())
	}

	return result
}

func hasVisibleThreat(redSoldiers, blueSoldiers []*game.Soldier) bool {
	for _, s := range redSoldiers {
		if s.IsAlive() && s.VisibleThreatCount() > 0 {
			return true
		}
	}
	for _, s := range blueSoldiers {
		if s.IsAlive() && s.VisibleThreatCount() > 0 {
			return true
		}
	}

	return false
}

func hasAnyDead(redSoldiers, blueSoldiers []*game.Soldier) bool {
	for _, s := range redSoldiers {
		if !s.IsAlive() {
			return true
		}
	}
	for _, s := range blueSoldiers {
		if !s.IsAlive() {
			return true
		}
	}

	return false
}

func countAliveInjured(soldiers []*game.Soldier) int {
	injured := 0
	for _, s := range soldiers {
		if s.IsAlive() && s.IsInjured() {
			injured++
		}
	}

	return injured
}

type reportWriter struct {
	writer io.Writer
	err    error
}

func (rw *reportWriter) linef(format string, args ...any) {
	if rw.err != nil {
		return
	}
	_, rw.err = fmt.Fprintf(rw.writer, format, args...)
}

// generateDetailedReport creates a comprehensive report file for post-analysis.
func generateDetailedReport(filename string, pop *game.Population, generations, popSize, battles, ticks, workers int, fitnessType string, totalTime time.Duration) error {
	cleanPath := filepath.Clean(filename)

	// #nosec G304 -- The path is intentionally provided by a trusted local CLI flag.
	f, err := os.Create(cleanPath)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to close %s: %v\n", cleanPath, closeErr)
		}
	}()

	bw := bufio.NewWriter(f)
	rw := &reportWriter{writer: bw}

	writeDetailedReportHeader(rw)
	writeDetailedReportConfig(rw, generations, popSize, battles, ticks, workers, fitnessType, totalTime)
	writeDetailedReportStats(rw, pop)
	writeDetailedReportTopGenomes(rw, pop)
	writeDetailedReportTraitAnalysis(rw, pop)
	writeDetailedReportDiversity(rw, pop)
	rw.linef("\n═══════════════════════════════════════════════════════════════════════════════\n")
	rw.linef("                         END OF REPORT                                         \n")
	rw.linef("═══════════════════════════════════════════════════════════════════════════════\n")

	if rw.err != nil {
		return rw.err
	}

	return bw.Flush()
}

func writeDetailedReportHeader(rw *reportWriter) {
	rw.linef("═══════════════════════════════════════════════════════════════════════════════\n")
	rw.linef("                    EVOLUTION DETAILED REPORT                                  \n")
	rw.linef("═══════════════════════════════════════════════════════════════════════════════\n\n")
}

func writeDetailedReportConfig(rw *reportWriter, generations, popSize, battles, ticks, workers int, fitnessType string, totalTime time.Duration) {
	rw.linef("CONFIGURATION:\n")
	rw.linef("  Generations:        %d\n", generations)
	rw.linef("  Population size:    %d\n", popSize)
	rw.linef("  Battles per genome: %d\n", battles)
	rw.linef("  Ticks per battle:   %d\n", ticks)
	rw.linef("  Workers:            %d\n", workers)
	rw.linef("  Fitness function:   %s\n", fitnessType)
	rw.linef("  Total time:         %v\n", totalTime.Round(time.Second))
	rw.linef("  Total battles:      %d\n\n", popSize*battles*generations)
}

func writeDetailedReportStats(rw *reportWriter, pop *game.Population) {
	rw.linef("FINAL GENERATION STATISTICS:\n")
	rw.linef("  Best Fitness:       %.6f\n", pop.BestFitness)
	rw.linef("  Average Fitness:    %.6f\n\n", pop.AvgFitness)

	if len(pop.Evaluations) == 0 || pop.BestEval.Results.GenomeName == "" {
		return
	}

	bestEval := pop.BestEval
	rw.linef("BEST GENOME ANALYSIS (ID: %d):\n", bestEval.Genome.ID)
	rw.linef("  Fitness:            %.6f\n", bestEval.Fitness)
	rw.linef("  Cost:               %.2f (%s)\n", bestEval.Cost, bestEval.Genome.GetCostCategory())
	rw.linef("  Cost-Adj Fitness:   %.6f\n\n", bestEval.CostAdjustedFitness)
	rw.linef("  Battle Performance:\n")
	rw.linef("    Survival Rate:    %.2f%%\n", bestEval.Results.AvgSurvivalRate*100)
	rw.linef("    K/D Ratio:        %.3f\n", bestEval.Results.AvgKDRatio)
	rw.linef("    Win Rate:         %.2f%%\n\n", bestEval.Results.WinRate*100)
	rw.linef("  Genome Traits (38 genes):\n")
	for i, geneDef := range game.GeneDefinitions {
		rw.linef("    %-25s %.4f  (bounds: %.2f-%.2f)\n", geneDef.Name+":", bestEval.Genome.Genes[i], geneDef.Bounds.Min, geneDef.Bounds.Max)
	}
	rw.linef("\n")
}

func writeDetailedReportTopGenomes(rw *reportWriter, pop *game.Population) {
	rw.linef("TOP 10 GENOMES FROM FINAL GENERATION:\n")
	rw.linef("───────────────────────────────────────────────────────────────────────────────\n")
	rw.linef("Rank  ID     Fitness   Survival  K/D     WinRate  Cost    Category\n")
	rw.linef("───────────────────────────────────────────────────────────────────────────────\n")

	topEvals := make([]game.GenomeEvaluation, len(pop.Evaluations))
	copy(topEvals, pop.Evaluations)
	for i := 0; i < len(topEvals)-1; i++ {
		for j := i + 1; j < len(topEvals); j++ {
			if topEvals[j].Fitness > topEvals[i].Fitness {
				topEvals[i], topEvals[j] = topEvals[j], topEvals[i]
			}
		}
	}

	for rank := 0; rank < 10 && rank < len(topEvals); rank++ {
		eval := topEvals[rank]
		rw.linef("%-5d %-6d %.4f    %.2f%%     %.3f   %.2f%%    %.2f    %-8s\n",
			rank+1,
			eval.Genome.ID,
			eval.Fitness,
			eval.Results.AvgSurvivalRate*100,
			eval.Results.AvgKDRatio,
			eval.Results.WinRate*100,
			eval.Cost,
			eval.Genome.GetCostCategory(),
		)
	}
	rw.linef("\n")
}

func writeDetailedReportTraitAnalysis(rw *reportWriter, pop *game.Population) {
	rw.linef("TRAIT CATEGORY ANALYSIS (Best Genome):\n")
	rw.linef("───────────────────────────────────────────────────────────────────────────────\n")
	rw.linef("Survival Traits:\n")
	rw.linef("─────────────────\n")
	if len(pop.Evaluations) == 0 {
		return
	}

	bestGenome := pop.BestEval.Genome
	rw.linef("  SelfPreservation:     %.4f\n", bestGenome.Genes[20])
	rw.linef("  SituationalAwareness: %.4f\n", bestGenome.Genes[21])
	rw.linef("  MedicalKnowledge:     %.4f\n", bestGenome.Genes[22])
	rw.linef("  MovementDiscipline:   %.4f\n", bestGenome.Genes[23])
	rw.linef("  RiskAssessment:       %.4f\n", bestGenome.Genes[24])
	rw.linef("  CoverSeeking:         %.4f\n", bestGenome.Genes[25])
	rw.linef("  ThreatPrioritization: %.4f\n", bestGenome.Genes[26])
	rw.linef("  BreakContact:         %.4f\n", bestGenome.Genes[27])
	rw.linef("  Stealth:              %.4f\n", bestGenome.Genes[28])
	rw.linef("  Survivalism:          %.4f\n\n", bestGenome.Genes[29])
	rw.linef("  Squad Cooperation Traits:\n")
	rw.linef("    CoordinatedFire:      %.4f\n", bestGenome.Genes[30])
	rw.linef("    BuddyAidPriority:     %.4f\n", bestGenome.Genes[31])
	rw.linef("    MedicDedication:      %.4f\n", bestGenome.Genes[32])
	rw.linef("    CasualtyEvacuation:   %.4f\n", bestGenome.Genes[33])
	rw.linef("    CoverSharing:         %.4f\n", bestGenome.Genes[34])
	rw.linef("    SuppressiveSupport:   %.4f\n", bestGenome.Genes[35])
	rw.linef("    CommunicationClarity: %.4f\n", bestGenome.Genes[36])
	rw.linef("    LeadershipFollowing:  %.4f\n\n", bestGenome.Genes[37])
}

func writeDetailedReportDiversity(rw *reportWriter, pop *game.Population) {
	rw.linef("POPULATION DIVERSITY:\n")
	rw.linef("───────────────────────────────────────────────────────────────────────────────\n")
	for i, geneDef := range game.GeneDefinitions {
		var sum float64
		var sumSq float64
		for _, genome := range pop.Genomes {
			val := genome.Genes[i]
			sum += val
			sumSq += val * val
		}
		mean := sum / float64(len(pop.Genomes))
		variance := (sumSq / float64(len(pop.Genomes))) - (mean * mean)
		stdDev := 0.0
		if variance > 0 {
			stdDev = variance
			for j := 0; j < 10; j++ {
				stdDev = (stdDev + variance/stdDev) / 2
			}
		}
		rw.linef("  %-25s mean: %.4f  stddev: %.4f\n", geneDef.Name, mean, stdDev)
	}
}

// logGeneration writes generation statistics to log file.
func logGeneration(f *os.File, pop *game.Population) error {
	fields := []string{
		strconv.Itoa(pop.Generation),
		fmt.Sprintf("%.6f", pop.BestFitness),
		fmt.Sprintf("%.6f", pop.AvgFitness),
	}

	if len(pop.Evaluations) > 0 && pop.BestEval.Results.GenomeName != "" {
		bestEval := pop.BestEval
		fields = append(fields,
			fmt.Sprintf("%.6f", bestEval.Results.AvgSurvivalRate),
			fmt.Sprintf("%.6f", bestEval.Results.AvgKDRatio),
			fmt.Sprintf("%.6f", bestEval.Results.WinRate),
			fmt.Sprintf("%.6f", bestEval.Cost),
			bestEval.Genome.GetCostCategory(),
			fmt.Sprintf("%.6f", bestEval.CostAdjustedFitness),
		)
	} else {
		fields = append(fields, "0.0", "0.0", "0.0", "0.0", "Unknown", "0.0")
	}

	for _, gene := range pop.BestGenome.Genes {
		fields = append(fields, fmt.Sprintf("%.6f", gene))
	}

	_, err := f.WriteString(strings.Join(fields, ",") + "\n")
	return err
}
