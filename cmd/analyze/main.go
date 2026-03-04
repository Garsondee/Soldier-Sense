// Package main provides a CLI for evolution parameter sensitivity analysis.
package main

import (
	"bufio"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/Garsondee/Soldier-Sense/internal/game"
)

// EvolutionData represents a single generation's data from the evolution log.
type EvolutionData struct {
	BestCategory       string
	BestGenes          []float64
	Generation         int
	BestFitness        float64
	AvgFitness         float64
	BestSurvival       float64
	BestKD             float64
	BestWinRate        float64
	BestCost           float64
	BestCostAdjFitness float64
}

// GeneAnalysis holds statistical analysis for a single gene.
type GeneAnalysis struct {
	GeneName string
	Index    int

	// Statistical measures
	CorrelationWithFitness float64
	VarianceAcrossGens     float64
	ConvergenceRate        float64
	FinalValue             float64
	InitialValue           float64

	// Importance ranking
	ImportanceScore float64
	Rank            int
}

func main() {
	logFile := flag.String("log", "evolution.log", "Evolution log file to analyze")
	outputFile := flag.String("output", "sensitivity_analysis.txt", "Output file for analysis results")
	flag.Parse()

	fmt.Printf("🔬 PARAMETER SENSITIVITY ANALYSIS\n")
	fmt.Printf("═════════════════════════════════════════════════\n")
	fmt.Printf("Analyzing: %s\n", *logFile)
	fmt.Printf("Output: %s\n\n", *outputFile)

	// Load evolution data
	data, err := loadEvolutionData(*logFile)
	if err != nil {
		fmt.Printf("Error loading data: %v\n", err)
		return
	}

	if len(data) < 3 {
		fmt.Printf("Error: Need at least 3 generations for analysis\n")
		return
	}

	fmt.Printf("Loaded %d generations of data\n", len(data))

	// Perform sensitivity analysis
	geneAnalyses := performSensitivityAnalysis(data)

	// Sort by importance
	sort.Slice(geneAnalyses, func(i, j int) bool {
		return geneAnalyses[i].ImportanceScore > geneAnalyses[j].ImportanceScore
	})

	// Assign ranks
	for i := range geneAnalyses {
		geneAnalyses[i].Rank = i + 1
	}

	// Generate report
	if err := generateReport(geneAnalyses, data, *outputFile); err != nil {
		fmt.Printf("Error writing report: %v\n", err)
		return
	}

	// Print summary to console
	printSummary(geneAnalyses, len(data))
}

func loadEvolutionData(filename string) ([]EvolutionData, error) {
	cleanPath := filepath.Clean(filename)

	// #nosec G304 -- The path is intentionally provided by a trusted local CLI flag.
	file, err := os.Open(cleanPath)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to close %s: %v\n", cleanPath, closeErr)
		}
	}()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	if len(records) < 2 {
		return nil, fmt.Errorf("insufficient data in log file")
	}

	data := make([]EvolutionData, 0, len(records)-1)

	// Skip header row.
	for _, record := range records[1:] {
		entry, ok := parseEvolutionRecord(record)
		if !ok {
			continue
		}
		data = append(data, entry)
	}

	return data, nil
}

func parseEvolutionRecord(record []string) (EvolutionData, bool) {
	if len(record) < 29 { // Minimum required columns.
		return EvolutionData{}, false
	}

	entry := EvolutionData{BestCategory: record[7]}

	var parseErr error
	if entry.Generation, parseErr = strconv.Atoi(record[0]); parseErr != nil {
		return EvolutionData{}, false
	}
	if entry.BestFitness, parseErr = strconv.ParseFloat(record[1], 64); parseErr != nil {
		return EvolutionData{}, false
	}
	if entry.AvgFitness, parseErr = strconv.ParseFloat(record[2], 64); parseErr != nil {
		return EvolutionData{}, false
	}
	if entry.BestSurvival, parseErr = strconv.ParseFloat(record[3], 64); parseErr != nil {
		return EvolutionData{}, false
	}
	if entry.BestKD, parseErr = strconv.ParseFloat(record[4], 64); parseErr != nil {
		return EvolutionData{}, false
	}
	if entry.BestWinRate, parseErr = strconv.ParseFloat(record[5], 64); parseErr != nil {
		return EvolutionData{}, false
	}
	if entry.BestCost, parseErr = strconv.ParseFloat(record[6], 64); parseErr != nil {
		return EvolutionData{}, false
	}
	if entry.BestCostAdjFitness, parseErr = strconv.ParseFloat(record[8], 64); parseErr != nil {
		return EvolutionData{}, false
	}

	// Parse genes (columns 9 onwards).
	entry.BestGenes = make([]float64, game.NumGenes)
	for j := 0; j < game.NumGenes && j+9 < len(record); j++ {
		entry.BestGenes[j], parseErr = strconv.ParseFloat(record[j+9], 64)
		if parseErr != nil {
			entry.BestGenes[j] = 0
		}
	}

	return entry, true
}

func performSensitivityAnalysis(data []EvolutionData) []GeneAnalysis {
	geneAnalyses := make([]GeneAnalysis, game.NumGenes)

	for i := 0; i < game.NumGenes; i++ {
		analysis := GeneAnalysis{
			GeneName: game.GeneDefinitions[i].Name,
			Index:    i,
		}

		// Extract gene values and fitness values across generations
		geneValues := make([]float64, len(data))
		fitnessValues := make([]float64, len(data))

		for j, entry := range data {
			if i < len(entry.BestGenes) {
				geneValues[j] = entry.BestGenes[i]
			}
			fitnessValues[j] = entry.BestFitness
		}

		// Calculate correlation with fitness
		analysis.CorrelationWithFitness = calculateCorrelation(geneValues, fitnessValues)

		// Calculate variance across generations
		analysis.VarianceAcrossGens = calculateVariance(geneValues)

		// Calculate convergence rate (how quickly the gene value stabilizes)
		analysis.ConvergenceRate = calculateConvergenceRate(geneValues)

		// Store initial and final values
		if len(geneValues) > 0 {
			analysis.InitialValue = geneValues[0]
			analysis.FinalValue = geneValues[len(geneValues)-1]
		}

		// Calculate importance score (combination of correlation and convergence)
		analysis.ImportanceScore = calculateImportanceScore(&analysis)

		geneAnalyses[i] = analysis
	}

	return geneAnalyses
}

func calculateCorrelation(x, y []float64) float64 {
	if len(x) != len(y) || len(x) < 2 {
		return 0.0
	}

	n := float64(len(x))

	// Calculate means
	meanX := 0.0
	meanY := 0.0
	for i := 0; i < len(x); i++ {
		meanX += x[i]
		meanY += y[i]
	}
	meanX /= n
	meanY /= n

	// Calculate correlation coefficient
	numerator := 0.0
	denomX := 0.0
	denomY := 0.0

	for i := 0; i < len(x); i++ {
		dx := x[i] - meanX
		dy := y[i] - meanY
		numerator += dx * dy
		denomX += dx * dx
		denomY += dy * dy
	}

	if denomX == 0 || denomY == 0 {
		return 0.0
	}

	return numerator / math.Sqrt(denomX*denomY)
}

func calculateVariance(values []float64) float64 {
	if len(values) < 2 {
		return 0.0
	}

	mean := 0.0
	for _, v := range values {
		mean += v
	}
	mean /= float64(len(values))

	variance := 0.0
	for _, v := range values {
		diff := v - mean
		variance += diff * diff
	}
	variance /= float64(len(values) - 1)

	return variance
}

func calculateConvergenceRate(values []float64) float64 {
	if len(values) < 3 {
		return 0.0
	}

	// Calculate rate of change between consecutive generations
	changes := make([]float64, len(values)-1)
	for i := 1; i < len(values); i++ {
		changes[i-1] = math.Abs(values[i] - values[i-1])
	}

	// Return inverse of average change (higher = more convergence)
	avgChange := 0.0
	for _, change := range changes {
		avgChange += change
	}
	avgChange /= float64(len(changes))

	if avgChange == 0 {
		return 1.0 // Perfect convergence
	}

	return 1.0 / (1.0 + avgChange)
}

func calculateImportanceScore(analysis *GeneAnalysis) float64 {
	// Combine multiple factors to determine overall importance:
	// 1. Correlation with fitness (how much it affects performance)
	// 2. Convergence rate (how much evolution cares about it)
	// 3. Variance (how much it changes)

	corrWeight := 0.5
	convWeight := 0.3
	varWeight := 0.2

	// Normalize correlation (absolute value, since negative correlation is also important)
	corrScore := math.Abs(analysis.CorrelationWithFitness)

	// Convergence score
	convScore := analysis.ConvergenceRate

	// Variance score (normalized by typical gene range of 0-1)
	varScore := math.Min(analysis.VarianceAcrossGens, 1.0)

	return corrScore*corrWeight + convScore*convWeight + varScore*varWeight
}

func generateReport(geneAnalyses []GeneAnalysis, data []EvolutionData, filename string) error {
	cleanPath := filepath.Clean(filename)

	// #nosec G304 -- The path is intentionally provided by a trusted local CLI flag.
	file, err := os.Create(cleanPath)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to close %s: %v\n", cleanPath, closeErr)
		}
	}()

	writer := bufio.NewWriter(file)
	report := newReportWriter(writer)

	// Write header.
	report.linef("PARAMETER SENSITIVITY ANALYSIS REPORT\n")
	report.linef("═════════════════════════════════════\n\n")
	report.linef("Analysis of %d generations of evolution data\n\n", len(data))

	// Write methodology.
	report.linef("METHODOLOGY:\n")
	report.linef("- Correlation: How gene value correlates with fitness (-1 to +1)\n")
	report.linef("- Convergence: How quickly gene value stabilizes (0 to 1)\n")
	report.linef("- Variance: How much gene value varies across generations\n")
	report.linef("- Importance: Combined score (50%% correlation, 30%% convergence, 20%% variance)\n\n")

	// Write top 10 most important genes.
	report.linef("TOP 10 MOST IMPORTANT GENES:\n")
	report.linef("═══════════════════════════════\n")
	report.linef("Rank | Gene Name          | Importance | Correlation | Convergence | Variance | Change\n")
	report.linef("-----|--------------------|-----------:|------------:|------------:|---------:|-------:\n")

	for i := 0; i < 10 && i < len(geneAnalyses); i++ {
		a := geneAnalyses[i]
		change := a.FinalValue - a.InitialValue
		report.linef("%4d | %-18s | %10.4f | %11.4f | %11.4f | %8.4f | %+6.3f\n",
			a.Rank, a.GeneName, a.ImportanceScore, a.CorrelationWithFitness,
			a.ConvergenceRate, a.VarianceAcrossGens, change)
	}

	// Write full detailed analysis.
	report.linef("\n\nFULL GENE ANALYSIS:\n")
	report.linef("══════════════════\n")

	for _, a := range geneAnalyses {
		report.linef("\n%d. %s (Rank %d)\n", a.Index+1, a.GeneName, a.Rank)
		report.linef("   Importance Score: %.4f\n", a.ImportanceScore)
		report.linef("   Correlation with Fitness: %.4f\n", a.CorrelationWithFitness)
		report.linef("   Convergence Rate: %.4f\n", a.ConvergenceRate)
		report.linef("   Variance: %.4f\n", a.VarianceAcrossGens)
		report.linef("   Evolution: %.3f -> %.3f (delta%+.3f)\n", a.InitialValue, a.FinalValue, a.FinalValue-a.InitialValue)
	}

	if report.err != nil {
		return report.err
	}

	if err := writer.Flush(); err != nil {
		return err
	}

	fmt.Printf("✅ Detailed analysis saved to %s\n", filename)

	return nil
}

type reportWriter struct {
	writer io.Writer
	err    error
}

func newReportWriter(writer io.Writer) *reportWriter {
	return &reportWriter{writer: writer}
}

func (rw *reportWriter) linef(format string, args ...any) {
	if rw.err != nil {
		return
	}
	_, rw.err = fmt.Fprintf(rw.writer, format, args...)
}

func correlationLabel(analysis *GeneAnalysis) string {
	absCorrelation := math.Abs(analysis.CorrelationWithFitness)

	switch {
	case absCorrelation > 0.3 && analysis.CorrelationWithFitness > 0:
		return "📈 Strong +"
	case absCorrelation > 0.3:
		return "📉 Strong -"
	case absCorrelation > 0.1:
		return "📊 Moderate"
	default:
		return "➖ Weak"
	}
}

func printSummary(geneAnalyses []GeneAnalysis, numGens int) {
	fmt.Printf("\n📊 SENSITIVITY ANALYSIS SUMMARY\n")
	fmt.Printf("═════════════════════════════════════\n")
	fmt.Printf("Analyzed %d generations across %d genes\n\n", numGens, len(geneAnalyses))

	fmt.Printf("🏆 TOP 5 MOST IMPORTANT GENES:\n")
	for i := 0; i < 5 && i < len(geneAnalyses); i++ {
		a := geneAnalyses[i]
		fmt.Printf("%d. %-18s | Score: %5.3f | %s\n",
			i+1, a.GeneName, a.ImportanceScore, correlationLabel(&a))
	}

	fmt.Printf("\n🔍 KEY INSIGHTS:\n")

	// Find genes with strong positive correlation
	var strongPos []string
	var strongNeg []string
	var highVar []string
	var lowVar []string

	for _, a := range geneAnalyses {
		if a.CorrelationWithFitness > 0.3 {
			strongPos = append(strongPos, a.GeneName)
		} else if a.CorrelationWithFitness < -0.3 {
			strongNeg = append(strongNeg, a.GeneName)
		}

		if a.VarianceAcrossGens > 0.1 {
			highVar = append(highVar, a.GeneName)
		} else if a.VarianceAcrossGens < 0.01 {
			lowVar = append(lowVar, a.GeneName)
		}
	}

	if len(strongPos) > 0 {
		fmt.Printf("• Traits that boost performance: %s\n", strings.Join(strongPos, ", "))
	}
	if len(strongNeg) > 0 {
		fmt.Printf("• Traits that hurt performance: %s\n", strings.Join(strongNeg, ", "))
	}
	if len(highVar) > 0 {
		fmt.Printf("• Highly variable traits: %s\n", strings.Join(highVar, ", "))
	}
	if len(lowVar) > 0 {
		fmt.Printf("• Converged traits: %s\n", strings.Join(lowVar, ", "))
	}

	fmt.Printf("\n📋 For detailed analysis, see the output file.\n")
}
