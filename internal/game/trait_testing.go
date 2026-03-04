package game

import (
	"fmt"
	"math"
)

// TestGenome defines a soldier profile configuration for trait testing.
type TestGenome struct {
	Name        string
	Description string
	Profile     SoldierProfile
}

// TestGenomeResults holds aggregated statistics from multiple battle runs.
type TestGenomeResults struct {
	GenomeName    string
	SurvivalRates []float64
	KDRatios      []float64
	PanicCounts   []int

	AvgSurvivalRate      float64
	StdDevSurvival       float64
	AvgKills             float64
	AvgDeaths            float64
	AvgKDRatio           float64
	StdDevKDRatio        float64
	AvgPanicEvents       float64
	AvgSurrenders        float64
	AvgDisobediences     float64
	AvgBattleDuration    float64
	WinRate              float64
	AvgRedInjured        float64
	AvgBlueInjured       float64
	AvgRedKIA            float64
	AvgBlueKIA           float64
	NoContactRate        float64 // fraction of runs with no contact
	AvgAccuracy          float64 // hit rate %
	AvgEngageDist        float64 // average engagement distance
	AvgTimeInCover       float64 // % of time in cover
	AvgTimeSuppressed    float64 // % of time suppressed
	AvgRetreatCount      float64 // retreats per battle
	AvgFirstContact      float64 // ticks until first contact
	AvgFirstCasualty     float64 // ticks until first death
	AvgBattleIntensity   float64 // shots per tick
	AvgDetectionTime     float64 // average ticks to detect enemy
	AvgSpottingPower     float64 // average observer spotting effectiveness
	AvgConcealmentScore  float64 // average target concealment when detected
	AvgLKPRetentionTime  float64 // average ticks LKP is retained
	AvgAmbushSuccessRate float64 // % of first shots against unaware targets

	Runs int
}

// ControlGenome returns the baseline neutral soldier profile.
// This is used for the Blue team (control group).
func ControlGenome() TestGenome {
	return TestGenome{
		Name:        "Control",
		Description: "Baseline neutral stats - control group",
		Profile:     DefaultProfile(),
	}
}

// Phase0TestGenomes returns the experimental genome set for Phase 0 testing.
// These are designed to test if personality traits create measurable differences.
func Phase0TestGenomes() []TestGenome {
	base := DefaultProfile()

	return []TestGenome{
		{
			Name:        "Aggressive",
			Description: "High aggression, low caution - assault specialist",
			Profile: SoldierProfile{
				Physical: base.Physical,
				Skills:   base.Skills,
				Psych:    base.Psych,
				Personality: PersonalityTraits{
					Aggression:     0.9,
					Caution:        0.2,
					PanicThreshold: 0.5,
				},
				Stance: base.Stance,
			},
		},
		{
			Name:        "Cautious",
			Description: "Low aggression, high caution - defensive specialist",
			Profile: SoldierProfile{
				Physical: base.Physical,
				Skills:   base.Skills,
				Psych:    base.Psych,
				Personality: PersonalityTraits{
					Aggression:     0.2,
					Caution:        0.9,
					PanicThreshold: 0.5,
				},
				Stance: base.Stance,
			},
		},
		{
			Name:        "Fearless",
			Description: "High panic threshold - stress resistant",
			Profile: SoldierProfile{
				Physical: base.Physical,
				Skills:   base.Skills,
				Psych:    base.Psych,
				Personality: PersonalityTraits{
					Aggression:     0.6,
					Caution:        0.4,
					PanicThreshold: 0.95,
				},
				Stance: base.Stance,
			},
		},
		{
			Name:        "Panicky",
			Description: "Low panic threshold - breaks under pressure",
			Profile: SoldierProfile{
				Physical: base.Physical,
				Skills:   base.Skills,
				Psych:    base.Psych,
				Personality: PersonalityTraits{
					Aggression:     0.5,
					Caution:        0.5,
					PanicThreshold: 0.15,
				},
				Stance: base.Stance,
			},
		},
		{
			Name:        "Berserker",
			Description: "Maximum aggression, minimum caution - high risk",
			Profile: SoldierProfile{
				Physical: base.Physical,
				Skills:   base.Skills,
				Psych:    base.Psych,
				Personality: PersonalityTraits{
					Aggression:     1.0,
					Caution:        0.0,
					PanicThreshold: 0.7,
				},
				Stance: base.Stance,
			},
		},
		{
			Name:        "Balanced-Aggressive",
			Description: "Moderate aggression bias with good panic resistance",
			Profile: SoldierProfile{
				Physical: base.Physical,
				Skills:   base.Skills,
				Psych:    base.Psych,
				Personality: PersonalityTraits{
					Aggression:     0.7,
					Caution:        0.4,
					PanicThreshold: 0.7,
				},
				Stance: base.Stance,
			},
		},
		{
			Name:        "Balanced-Defensive",
			Description: "Moderate caution bias with good panic resistance",
			Profile: SoldierProfile{
				Physical: base.Physical,
				Skills:   base.Skills,
				Psych:    base.Psych,
				Personality: PersonalityTraits{
					Aggression:     0.4,
					Caution:        0.7,
					PanicThreshold: 0.7,
				},
				Stance: base.Stance,
			},
		},
		{
			Name:        "Elite-Aggressive",
			Description: "High skills + aggressive personality",
			Profile: SoldierProfile{
				Physical: PhysicalStats{
					FitnessBase: 0.8,
					Fatigue:     0.0,
					SprintPool:  10.0,
				},
				Skills: SkillStats{
					Marksmanship: 0.85,
					Fieldcraft:   0.7,
					Discipline:   0.8,
					FirstAid:     0.5,
				},
				Psych: PsychState{
					Experience: 0.6,
					Morale:     0.8,
					Fear:       0.0,
					Composure:  0.75,
				},
				Personality: PersonalityTraits{
					Aggression:     0.8,
					Caution:        0.3,
					PanicThreshold: 0.85,
				},
				Stance: base.Stance,
			},
		},
		{
			Name:        "Elite-Defensive",
			Description: "High skills + defensive personality",
			Profile: SoldierProfile{
				Physical: PhysicalStats{
					FitnessBase: 0.8,
					Fatigue:     0.0,
					SprintPool:  10.0,
				},
				Skills: SkillStats{
					Marksmanship: 0.85,
					Fieldcraft:   0.7,
					Discipline:   0.8,
					FirstAid:     0.5,
				},
				Psych: PsychState{
					Experience: 0.6,
					Morale:     0.8,
					Fear:       0.0,
					Composure:  0.75,
				},
				Personality: PersonalityTraits{
					Aggression:     0.3,
					Caution:        0.8,
					PanicThreshold: 0.85,
				},
				Stance: base.Stance,
			},
		},
	}
}

// CalculateResults aggregates statistics from multiple battle outcomes.
func CalculateResults(genomeName string, outcomes []TraitTestResult) TestGenomeResults { //nolint:gocognit
	if len(outcomes) == 0 {
		return TestGenomeResults{GenomeName: genomeName, Runs: 0}
	}

	results := TestGenomeResults{
		GenomeName:    genomeName,
		Runs:          len(outcomes),
		SurvivalRates: make([]float64, len(outcomes)),
		KDRatios:      make([]float64, len(outcomes)),
		PanicCounts:   make([]int, len(outcomes)),
	}

	var totalSurvival, totalKills, totalDeaths, totalKDRatio float64
	var totalPanics, totalSurrenders, totalDisobey int
	var totalDuration float64
	var totalRedInjured, totalBlueInjured, totalRedKIA, totalBlueKIA float64
	var totalAccuracy, totalEngageDist, totalTimeInCover, totalTimeSuppressed float64
	var totalRetreats, totalFirstContact, totalFirstCasualty, totalIntensity float64
	var totalDetectionTime, totalSpottingPower, totalConcealment, totalLKPTime, totalAmbushRate float64
	var wins int
	contactSamples := 0
	casualtySamples := 0
	noContact := 0

	for i := range outcomes {
		outcome := &outcomes[i]
		// Survival rate (Red team only - experimental group)
		survivalRate := 0.0
		if outcome.RedTotal > 0 {
			survivalRate = float64(outcome.RedSurvived) / float64(outcome.RedTotal)
		}
		results.SurvivalRates[i] = survivalRate
		totalSurvival += survivalRate

		// K/D ratio
		kdRatio := 0.0
		if outcome.RedDeaths > 0 {
			kdRatio = float64(outcome.RedKills) / float64(outcome.RedDeaths)
		} else if outcome.RedKills > 0 {
			kdRatio = float64(outcome.RedKills) // No deaths = perfect ratio
		}
		results.KDRatios[i] = kdRatio
		totalKDRatio += kdRatio

		totalKills += float64(outcome.RedKills)
		totalDeaths += float64(outcome.RedDeaths)
		totalRedInjured += float64(outcome.RedInjured)
		totalBlueInjured += float64(outcome.BlueInjured)
		totalRedKIA += float64(outcome.RedDeaths)
		totalBlueKIA += float64(outcome.BlueDeaths)

		// Psychological events
		results.PanicCounts[i] = outcome.RedPanicEvents
		totalPanics += outcome.RedPanicEvents
		totalSurrenders += outcome.RedSurrenders
		totalDisobey += outcome.RedDisobediences

		// Battle metrics
		totalDuration += float64(outcome.Duration)
		if outcome.RedSurvived > outcome.BlueSurvived {
			wins++
		}

		// Enhanced telemetry
		if outcome.RedShotsFired > 0 {
			totalAccuracy += float64(outcome.RedShotsHit) / float64(outcome.RedShotsFired)
		}
		totalEngageDist += outcome.RedAvgEngageDist
		if outcome.Duration > 0 {
			totalTimeInCover += float64(outcome.RedTimeInCover) / float64(outcome.Duration)
			totalTimeSuppressed += float64(outcome.RedTimeSuppressed) / float64(outcome.Duration)
		}
		totalRetreats += float64(outcome.RedRetreatCount)
		if outcome.FirstContactTick >= 0 {
			totalFirstContact += float64(outcome.FirstContactTick)
			contactSamples++
		} else {
			noContact++
		}
		if outcome.FirstCasualtyTick >= 0 {
			totalFirstCasualty += float64(outcome.FirstCasualtyTick)
			casualtySamples++
		}
		totalIntensity += outcome.BattleIntensity

		// Detection and spotting telemetry aggregation
		if outcome.RedDetectionEvents > 0 {
			totalDetectionTime += float64(outcome.RedTotalDetectionTime) / float64(outcome.RedDetectionEvents)
		}
		if outcome.RedDetectionEvents > 0 {
			totalSpottingPower += outcome.RedTotalSpottingPower / float64(outcome.RedDetectionEvents)
		}
		if outcome.RedDetectionEvents > 0 {
			totalConcealment += outcome.RedTotalConcealment / float64(outcome.RedDetectionEvents)
		}
		if outcome.RedLKPEvents > 0 {
			totalLKPTime += float64(outcome.RedLKPTotalDuration) / float64(outcome.RedLKPEvents)
		}
		if outcome.RedAmbushShots > 0 {
			totalAmbushRate += float64(outcome.RedAmbushHits) / float64(outcome.RedAmbushShots)
		}
	}

	n := float64(len(outcomes))
	results.AvgSurvivalRate = totalSurvival / n
	results.AvgKills = totalKills / n
	results.AvgDeaths = totalDeaths / n
	results.AvgKDRatio = totalKDRatio / n
	results.AvgPanicEvents = float64(totalPanics) / n
	results.AvgSurrenders = float64(totalSurrenders) / n
	results.AvgDisobediences = float64(totalDisobey) / n
	results.AvgBattleDuration = totalDuration / n
	results.WinRate = float64(wins) / n
	results.AvgRedInjured = totalRedInjured / n
	results.AvgBlueInjured = totalBlueInjured / n
	results.AvgRedKIA = totalRedKIA / n
	results.AvgBlueKIA = totalBlueKIA / n
	results.NoContactRate = float64(noContact) / n

	// Enhanced telemetry averages
	results.AvgAccuracy = totalAccuracy / n
	results.AvgEngageDist = totalEngageDist / n
	results.AvgTimeInCover = totalTimeInCover / n
	results.AvgTimeSuppressed = totalTimeSuppressed / n
	results.AvgRetreatCount = totalRetreats / n
	if contactSamples > 0 {
		results.AvgFirstContact = totalFirstContact / float64(contactSamples)
	}
	if casualtySamples > 0 {
		results.AvgFirstCasualty = totalFirstCasualty / float64(casualtySamples)
	}
	results.AvgBattleIntensity = totalIntensity / n

	// Detection telemetry averages
	results.AvgDetectionTime = totalDetectionTime / n
	results.AvgSpottingPower = totalSpottingPower / n
	results.AvgConcealmentScore = totalConcealment / n
	results.AvgLKPRetentionTime = totalLKPTime / n
	results.AvgAmbushSuccessRate = totalAmbushRate / n

	// Calculate standard deviations
	var survivalVariance, kdVariance float64
	for i := range outcomes {
		survivalDiff := results.SurvivalRates[i] - results.AvgSurvivalRate
		survivalVariance += survivalDiff * survivalDiff

		kdDiff := results.KDRatios[i] - results.AvgKDRatio
		kdVariance += kdDiff * kdDiff
	}
	results.StdDevSurvival = math.Sqrt(survivalVariance / n)
	results.StdDevKDRatio = math.Sqrt(kdVariance / n)

	return results
}

// TraitTestResult captures the results of a single battle for trait testing.
type TraitTestResult struct {
	Seed     int64
	Duration int // ticks

	// Red team (experimental)
	RedTotal         int
	RedSurvived      int
	RedDeaths        int
	RedKills         int
	RedInjured       int
	RedPanicEvents   int
	RedSurrenders    int
	RedDisobediences int

	// Blue team (control)
	BlueTotal    int
	BlueSurvived int
	BlueDeaths   int
	BlueKills    int
	BlueInjured  int

	// Enhanced telemetry
	RedShotsFired     int
	RedShotsHit       int
	RedAvgEngageDist  float64
	RedTimeInCover    int     // ticks spent in cover
	RedTimeSuppressed int     // ticks spent suppressed
	RedRetreatCount   int     // number of retreat/fallback actions
	FirstContactTick  int     // when first enemy spotted
	FirstCasualtyTick int     // when first death occurred
	BattleIntensity   float64 // shots fired per tick

	// Detection and spotting telemetry
	RedDetectionEvents    int     // number of enemy detections
	RedTotalDetectionTime int     // cumulative ticks to detect enemies
	RedTotalSpottingPower float64 // cumulative spotting power measurements
	RedTotalConcealment   float64 // cumulative concealment scores when detected
	RedLKPEvents          int     // number of LKP created
	RedLKPTotalDuration   int     // cumulative ticks LKP retained
	RedAmbushShots        int     // shots fired at unaware targets
	RedAmbushHits         int     // successful ambush hits
}

// PrintResults outputs formatted test results for analysis.
func PrintResults(results *TestGenomeResults) {
	fmt.Printf("\n=== %s (%d runs) ===\n", results.GenomeName, results.Runs)
	fmt.Printf("Survival:  %.1f%% (±%.1f%%)\n", results.AvgSurvivalRate*100, results.StdDevSurvival*100)
	fmt.Printf("K/D Ratio: %.2f (±%.2f)\n", results.AvgKDRatio, results.StdDevKDRatio)
	fmt.Printf("Kills:     %.1f avg\n", results.AvgKills)
	fmt.Printf("Deaths:    %.1f avg\n", results.AvgDeaths)
	fmt.Printf("Win Rate:  %.1f%%\n", results.WinRate*100)
	fmt.Printf("Duration:  %.0f ticks avg\n", results.AvgBattleDuration)

	fmt.Printf("\nCombat Performance:\n")
	fmt.Printf("  Accuracy:       %.1f%%\n", results.AvgAccuracy*100)
	fmt.Printf("  Engage Dist:    %.0f units\n", results.AvgEngageDist)
	fmt.Printf("  Time in Cover:  %.1f%%\n", results.AvgTimeInCover*100)
	fmt.Printf("  Suppressed:     %.1f%%\n", results.AvgTimeSuppressed*100)
	fmt.Printf("  Retreats:       %.1f avg\n", results.AvgRetreatCount)

	fmt.Printf("\nBattle Dynamics:\n")
	fmt.Printf("  First Contact:  %.0f ticks\n", results.AvgFirstContact)
	fmt.Printf("  First Casualty: %.0f ticks\n", results.AvgFirstCasualty)
	fmt.Printf("  Intensity:      %.3f shots/tick\n", results.AvgBattleIntensity)

	fmt.Printf("\nPsychological:\n")
	fmt.Printf("  Panics:      %.1f avg\n", results.AvgPanicEvents)
	fmt.Printf("  Surrenders:  %.1f avg\n", results.AvgSurrenders)
	fmt.Printf("  Disobeys:    %.1f avg\n", results.AvgDisobediences)
}

// CompareResults prints a comparison table of multiple genome results.
func CompareResults(control *TestGenomeResults, experimental []TestGenomeResults) {
	fmt.Printf("\n╔════════════════════════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║                    PHASE 0 TRAIT TESTING RESULTS                           ║\n")
	fmt.Printf("╠════════════════════════════════════════════════════════════════════════════╣\n")
	fmt.Printf("║ Control Group: %s (Blue Team)                                          ║\n", control.GenomeName)
	fmt.Printf("║ Experimental: Red Team with varying personality traits                    ║\n")
	fmt.Printf("╚════════════════════════════════════════════════════════════════════════════╝\n\n")

	fmt.Printf("%-20s %8s %8s %8s %8s %8s %8s\n",
		"Genome", "Survival", "K/D", "Wins", "Panics", "Surrend", "Duration")
	fmt.Printf("%-20s %8s %8s %8s %8s %8s %8s\n",
		"--------------------", "--------", "--------", "--------", "--------", "--------", "--------")

	for i := range experimental {
		result := &experimental[i]
		survivalDelta := (result.AvgSurvivalRate - control.AvgSurvivalRate) * 100
		survivalSign := ""
		if survivalDelta > 0 {
			survivalSign = "+"
		}

		fmt.Printf("%-20s %6.1f%% %8.2f %7.0f%% %8.1f %8.1f %8.0f\n",
			result.GenomeName,
			result.AvgSurvivalRate*100,
			result.AvgKDRatio,
			result.WinRate*100,
			result.AvgPanicEvents,
			result.AvgSurrenders,
			result.AvgBattleDuration)

		fmt.Printf("  vs Control:        %s%5.1f%%\n", survivalSign, survivalDelta)
	}

	fmt.Printf("\n")
}
