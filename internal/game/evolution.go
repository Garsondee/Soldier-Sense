package game

import (
	"math"
	"math/rand"
	"sort"
)

// Genome represents a soldier's evolvable traits as a vector of floats.
// Each gene corresponds to a specific trait with defined bounds.
type Genome struct {
	Genes   []float64 // 20 genes (Phase 2): personality, skills, physical, preferences
	Fitness float64   // Calculated fitness score
	ID      int       // Unique identifier for tracking
}

// GeneBounds defines the valid range for each gene.
type GeneBounds struct {
	Min float64
	Max float64
}

// GeneDefinitions maps gene indices to their trait names and bounds.
var GeneDefinitions = []struct {
	Name   string
	Bounds GeneBounds
}{
	// Personality traits (0-5)
	{"Aggression", GeneBounds{0.0, 1.0}},
	{"Caution", GeneBounds{0.0, 1.0}},
	{"PanicThreshold", GeneBounds{0.0, 1.0}},
	{"Initiative", GeneBounds{0.0, 1.0}},
	{"Teamwork", GeneBounds{0.0, 1.0}},
	{"Adaptability", GeneBounds{0.0, 1.0}},

	// Skills (6-11)
	{"Marksmanship", GeneBounds{0.2, 0.8}},
	{"Fieldcraft", GeneBounds{0.2, 0.8}},
	{"Discipline", GeneBounds{0.3, 0.9}},
	{"FireControl", GeneBounds{0.2, 0.8}},
	{"TacticalAwareness", GeneBounds{0.2, 0.8}},
	{"FirstAid", GeneBounds{0.1, 0.7}},

	// Psychological (12-13)
	{"Experience", GeneBounds{0.0, 0.5}},
	{"Composure", GeneBounds{0.3, 0.8}},

	// Physical (14-16)
	{"Strength", GeneBounds{0.3, 0.9}},
	{"Agility", GeneBounds{0.3, 0.9}},
	{"Endurance", GeneBounds{0.3, 0.9}},

	// Tactical Preferences (17-19)
	{"ReloadEarly", GeneBounds{0.0, 1.0}},
	{"PreferCover", GeneBounds{0.0, 1.0}},
	{"PreferFlanking", GeneBounds{0.0, 1.0}},

	// Survival-Focused Traits (20-29) - New DNA for survival optimization
	{"SelfPreservation", GeneBounds{0.2, 0.9}},     // Instinctive danger avoidance
	{"SituationalAwareness", GeneBounds{0.1, 0.8}}, // Environmental threat detection
	{"MedicalKnowledge", GeneBounds{0.0, 0.7}},     // Self-aid and wound management
	{"MovementDiscipline", GeneBounds{0.3, 0.9}},   // Sound/visual discipline in movement
	{"RiskAssessment", GeneBounds{0.2, 0.8}},       // Danger evaluation before action
	{"CoverSeeking", GeneBounds{0.4, 1.0}},         // Active cover utilization
	{"ThreatPrioritization", GeneBounds{0.1, 0.8}}, // Focus on most dangerous targets first
	{"BreakContact", GeneBounds{0.2, 0.8}},         // Ability to disengage from bad situations
	{"Stealth", GeneBounds{0.0, 0.8}},              // Low-profile movement and positioning
	{"Survivalism", GeneBounds{0.1, 0.8}},          // General survival instincts and adaptability

	// Squad Cooperation Traits (30-37) - Teamwork and coordination
	{"CoordinatedFire", GeneBounds{0.0, 1.0}},      // Focus fire on same targets as squadmates
	{"BuddyAidPriority", GeneBounds{0.0, 1.0}},     // Willingness to help wounded squadmates
	{"MedicDedication", GeneBounds{0.0, 0.8}},      // Specialist medic trait for enhanced casualty care
	{"CasualtyEvacuation", GeneBounds{0.0, 0.8}},   // Willingness to drag/rescue wounded under fire
	{"CoverSharing", GeneBounds{0.0, 1.0}},         // Spatial awareness to avoid clustering
	{"SuppressiveSupport", GeneBounds{0.0, 0.8}},   // Covering fire for moving squadmates
	{"CommunicationClarity", GeneBounds{0.2, 0.9}}, // Radio discipline and clear reporting
	{"LeadershipFollowing", GeneBounds{0.0, 1.0}},  // Responsiveness to leader commands
}

// NumGenes is the total number of genes in a genome.
const NumGenes = 38

// TraitCosts defines the relative cost of each trait (0.1 = very cheap, 1.0 = very expensive).
// Updated based on sensitivity analysis - high-impact traits cost more to optimize.
var TraitCosts = []float64{
	// Personality traits (0-5) - costs updated based on performance impact
	0.3, // Aggression - moderate impact, moderate screening cost
	0.4, // Caution - high impact but negative correlation, costly to "reduce"
	0.8, // PanicThreshold - extremely high impact, very expensive to optimize (lower is better!)
	0.6, // Initiative - high variability, expensive to control
	0.5, // Teamwork - moderate impact, moderate cost to optimize
	0.7, // Adaptability - high negative impact, expensive to "reduce"

	// Skills (6-11) - costs adjusted based on actual performance correlation
	0.6, // Marksmanship - negative correlation! Less valuable than expected
	0.7, // Fieldcraft - strong negative impact, expensive to optimize
	0.5, // Discipline - moderate negative impact
	0.4, // FireControl - low impact, moderate cost
	0.3, // TacticalAwareness - surprisingly low impact
	0.6, // FirstAid - positive impact, moderate training cost

	// Psychological (12-13) - experience cheaper than expected, composure very valuable
	0.7, // Experience - moderate positive impact, but takes time to develop
	0.9, // Composure - extremely high positive impact, very expensive to select for

	// Physical (14-16) - endurance is by far the most valuable
	0.6, // Strength - moderate negative impact (surprising!)
	0.3, // Agility - very low impact, cheap
	1.0, // Endurance - strongest positive predictor! Extremely valuable

	// Tactical Preferences (17-19) - costs based on impact analysis
	0.3, // ReloadEarly - low impact
	0.5, // PreferCover - positive impact, moderate cost to train
	0.4, // PreferFlanking - strong negative impact, costly to "untrain"

	// Survival-Focused Traits (20-29) - high-value survival optimization traits
	0.9, // SelfPreservation - extremely valuable for survival, expensive to select for
	0.7, // SituationalAwareness - high-value skill, requires extensive training
	0.6, // MedicalKnowledge - moderate cost, specialized training required
	0.8, // MovementDiscipline - high-value survival skill, difficult to instill
	0.7, // RiskAssessment - critical for survival, requires experience and judgment
	0.5, // CoverSeeking - moderate cost, trainable behavior
	0.6, // ThreatPrioritization - moderate cost, tactical training required
	0.8, // BreakContact - high-value survival skill, requires training and nerve
	0.9, // Stealth - extremely valuable, difficult to master, expensive training
	0.8, // Survivalism - high-value general survival instincts, expensive to develop

	// Squad Cooperation Traits (30-37)
	0.5, // CoordinatedFire - moderate cost, tactical coordination training
	0.6, // BuddyAidPriority - requires overcoming self-preservation instinct
	0.9, // MedicDedication - specialist training, very expensive
	0.7, // CasualtyEvacuation - high-risk behavior, requires courage and training
	0.4, // CoverSharing - basic spatial awareness, relatively cheap
	0.6, // SuppressiveSupport - requires fire discipline and tactical awareness
	0.5, // CommunicationClarity - moderate training requirement
	0.3, // LeadershipFollowing - basic discipline, relatively cheap
}

// NewRandomGenome creates a genome with random values within bounds.
func NewRandomGenome(id int, rng *rand.Rand) Genome {
	genes := make([]float64, NumGenes)
	for i := 0; i < NumGenes; i++ {
		bounds := GeneDefinitions[i].Bounds
		genes[i] = bounds.Min + rng.Float64()*(bounds.Max-bounds.Min)
	}
	return Genome{
		Genes:   genes,
		Fitness: 0.0,
		ID:      id,
	}
}

// ToProfile converts a genome into a SoldierProfile for simulation.
func (g *Genome) ToProfile() SoldierProfile {
	base := DefaultProfile()

	// Personality traits (0-5)
	base.Personality = PersonalityTraits{
		Aggression:     g.Genes[0],
		Caution:        g.Genes[1],
		PanicThreshold: g.Genes[2],
		Initiative:     g.Genes[3],
		Teamwork:       g.Genes[4],
		Adaptability:   g.Genes[5],
	}

	// Skills (6-11)
	base.Skills = SkillStats{
		Marksmanship:      g.Genes[6],
		Fieldcraft:        g.Genes[7],
		Discipline:        g.Genes[8],
		FireControl:       g.Genes[9],
		TacticalAwareness: g.Genes[10],
		FirstAid:          g.Genes[11],
	}

	// Psychological (12-13)
	base.Psych.Experience = g.Genes[12]
	base.Psych.Composure = g.Genes[13]

	// Physical (14-16)
	base.Physical.Strength = g.Genes[14]
	base.Physical.Agility = g.Genes[15]
	base.Physical.Endurance = g.Genes[16]

	// Tactical Preferences (17-19)
	base.Preferences = TacticalPreferences{
		ReloadEarly:    g.Genes[17],
		PreferCover:    g.Genes[18],
		PreferFlanking: g.Genes[19],
	}

	// Survival Traits (20-29)
	base.Survival = SurvivalTraits{
		SelfPreservation:     g.Genes[20],
		SituationalAwareness: g.Genes[21],
		MedicalKnowledge:     g.Genes[22],
		MovementDiscipline:   g.Genes[23],
		RiskAssessment:       g.Genes[24],
		CoverSeeking:         g.Genes[25],
		ThreatPrioritization: g.Genes[26],
		BreakContact:         g.Genes[27],
		Stealth:              g.Genes[28],
		Survivalism:          g.Genes[29],
	}

	// Squad Cooperation Traits (30-37)
	base.Cooperation = SquadCooperation{
		CoordinatedFire:      g.Genes[30],
		BuddyAidPriority:     g.Genes[31],
		MedicDedication:      g.Genes[32],
		CasualtyEvacuation:   g.Genes[33],
		CoverSharing:         g.Genes[34],
		SuppressiveSupport:   g.Genes[35],
		CommunicationClarity: g.Genes[36],
		LeadershipFollowing:  g.Genes[37],
	}

	return base
}

// CalculateCost returns the total cost of this genome's traits.
// Cost represents training time, selection difficulty, and rarity.
// Range: ~2.0 (very cheap recruit) to ~12.0 (elite veteran).
func (g *Genome) CalculateCost() float64 {
	totalCost := 0.0
	for i := 0; i < NumGenes; i++ {
		// Cost scales with trait value: higher traits are more expensive
		traitCost := TraitCosts[i] * g.Genes[i]
		totalCost += traitCost
	}
	return totalCost
}

// GetCostCategory returns a human-readable cost category for this genome.
func (g *Genome) GetCostCategory() string {
	cost := g.CalculateCost()
	switch {
	case cost < 4.0:
		return "Recruit" // Very cheap
	case cost < 6.0:
		return "Regular" // Average cost
	case cost < 8.0:
		return "Veteran" // Expensive
	case cost < 10.0:
		return "Elite" // Very expensive
	default:
		return "Special Forces" // Extremely expensive
	}
}

// Clone creates a deep copy of the genome.
func (g *Genome) Clone() Genome {
	genes := make([]float64, len(g.Genes))
	copy(genes, g.Genes)
	return Genome{
		Genes:   genes,
		Fitness: g.Fitness,
		ID:      g.ID,
	}
}

// Clamp ensures all genes are within their valid bounds.
func (g *Genome) Clamp() {
	for i := 0; i < NumGenes; i++ {
		bounds := GeneDefinitions[i].Bounds
		if g.Genes[i] < bounds.Min {
			g.Genes[i] = bounds.Min
		} else if g.Genes[i] > bounds.Max {
			g.Genes[i] = bounds.Max
		}
	}
}

// GenomeEvaluation holds detailed evaluation results for a genome.
type GenomeEvaluation struct {
	Genome              Genome
	Results             TestGenomeResults
	Fitness             float64
	Cost                float64
	CostAdjustedFitness float64
}

// Population represents a collection of genomes in a generation.
type Population struct {
	Genomes     []Genome
	Evaluations []GenomeEvaluation
	BestGenome  Genome
	BestEval    GenomeEvaluation
	Generation  int

	BestFitness float64
	AvgFitness  float64
}

// NewPopulation creates a random initial population.
func NewPopulation(size int, seed int64) Population {
	rng := rand.New(rand.NewSource(seed))
	genomes := make([]Genome, size)
	for i := 0; i < size; i++ {
		genomes[i] = NewRandomGenome(i, rng)
	}
	return Population{
		Genomes:    genomes,
		Generation: 0,
	}
}

// UpdateStatistics calculates population-wide fitness statistics.
func (p *Population) UpdateStatistics() {
	if len(p.Genomes) == 0 {
		return
	}

	totalFitness := 0.0
	bestFitness := p.Genomes[0].Fitness
	bestIdx := 0

	for i, genome := range p.Genomes {
		totalFitness += genome.Fitness
		if genome.Fitness > bestFitness {
			bestFitness = genome.Fitness
			bestIdx = i
		}
	}

	p.AvgFitness = totalFitness / float64(len(p.Genomes))
	p.BestFitness = bestFitness
	p.BestGenome = p.Genomes[bestIdx].Clone()

	// Update best evaluation if we have detailed results
	if len(p.Evaluations) > 0 {
		bestEvalIdx := 0
		bestEvalFitness := p.Evaluations[0].Fitness
		for i := range p.Evaluations {
			eval := &p.Evaluations[i]
			if eval.Fitness > bestEvalFitness {
				bestEvalFitness = eval.Fitness
				bestEvalIdx = i
			}
		}
		p.BestEval = p.Evaluations[bestEvalIdx]
	}
}

// SortByFitness sorts genomes in descending order of fitness.
func (p *Population) SortByFitness() {
	sort.Slice(p.Genomes, func(i, j int) bool {
		return p.Genomes[i].Fitness > p.Genomes[j].Fitness
	})
}

// FitnessFunction defines how to calculate fitness from battle results.
type FitnessFunction func(results *TestGenomeResults) float64

// DefaultFitnessFunction balances survival, K/D ratio, and win rate.
func DefaultFitnessFunction(results *TestGenomeResults) float64 {
	// Normalize components to 0-1 range
	survival := results.AvgSurvivalRate // Already 0-1

	// K/D ratio: cap at 3.0 for normalization
	kdNorm := math.Min(results.AvgKDRatio/3.0, 1.0)

	// Win rate: already 0-1
	winRate := results.WinRate

	// Weighted combination
	fitness := (survival * 0.35) + (kdNorm * 0.35) + (winRate * 0.30)

	return fitness
}

// AggressiveFitnessFunction prioritizes kills and aggression.
func AggressiveFitnessFunction(results *TestGenomeResults) float64 {
	survival := results.AvgSurvivalRate
	kdNorm := math.Min(results.AvgKDRatio/3.0, 1.0)
	winRate := results.WinRate

	// Heavily weight K/D and wins
	fitness := (survival * 0.20) + (kdNorm * 0.50) + (winRate * 0.30)

	return fitness
}

// DefensiveFitnessFunction prioritizes survival.
func DefensiveFitnessFunction(results *TestGenomeResults) float64 {
	survival := results.AvgSurvivalRate
	kdNorm := math.Min(results.AvgKDRatio/3.0, 1.0)
	winRate := results.WinRate

	// Heavily weight survival
	fitness := (survival * 0.60) + (kdNorm * 0.20) + (winRate * 0.20)

	return fitness
}

// BalancedFitnessFunction emphasizes win rate.
func BalancedFitnessFunction(results *TestGenomeResults) float64 {
	survival := results.AvgSurvivalRate
	kdNorm := math.Min(results.AvgKDRatio/3.0, 1.0)
	winRate := results.WinRate

	// Prioritize winning
	fitness := (survival * 0.25) + (kdNorm * 0.25) + (winRate * 0.50)

	return fitness
}

// CostEfficiencyFitnessFunction rewards performance per unit cost.
// This discovers effective soldiers regardless of their training expense.
func CostEfficiencyFitnessFunction(results *TestGenomeResults, cost float64) float64 {
	// Base fitness using default weighting
	baseFitness := DefaultFitnessFunction(results)

	// Cost efficiency: performance per unit cost
	// Average cost is ~6.0, so we normalize around that
	avgCost := 6.0
	costEfficiency := baseFitness * (avgCost / math.Max(cost, 1.0))

	return costEfficiency
}

// RecruitFitnessFunction heavily rewards cheap soldiers who perform well.
// This finds the best bang-for-buck among low-cost troops.
func RecruitFitnessFunction(results *TestGenomeResults, cost float64) float64 {
	baseFitness := DefaultFitnessFunction(results)

	// Heavy cost penalty for expensive troops, bonus for cheap troops
	var costMultiplier float64
	switch {
	case cost < 4.0:
		costMultiplier = 1.5 // 50% bonus for recruits
	case cost < 6.0:
		costMultiplier = 1.2 // 20% bonus for regulars
	case cost < 8.0:
		costMultiplier = 0.8 // 20% penalty for veterans
	default:
		costMultiplier = 0.5 // 50% penalty for elites
	}

	return baseFitness * costMultiplier
}

// EliteFitnessFunction seeks maximum absolute performance regardless of cost.
// This is the traditional fitness function for special operations.
func EliteFitnessFunction(results *TestGenomeResults, _ float64) float64 {
	// Pure performance, cost doesn't matter
	return DefaultFitnessFunction(results)
}

// SurvivalFitnessFunction prioritizes soldier survival above all else.
// Uses exponential penalty for casualties - dead soldiers ruin missions.
// Also penalizes passive/disengaged behavior to prevent hiding strategies.
// NOTE: This is the cost-unaware version. Use RegularSoldierFitnessFunction for cost-balanced evolution.
func SurvivalFitnessFunction(results *TestGenomeResults) float64 {
	survival := results.AvgSurvivalRate
	kdNorm := math.Min(results.AvgKDRatio/3.0, 1.0)
	winRate := results.WinRate

	// Exponential survival penalty: each casualty dramatically reduces fitness
	// 100% survival = 1.0, 90% survival = 0.81, 80% survival = 0.64, etc.
	survivalBonus := survival * survival

	// Mission failure penalty: if survival < 50%, fitness approaches zero
	if survival < 0.5 {
		survivalBonus *= survival // Additional penalty for catastrophic losses
	}

	// Engagement penalty: penalize passive behavior (low K/D + low win rate)
	// This prevents "hide and survive" strategies that never engage
	engagementScore := (kdNorm * 0.4) + (winRate * 0.6)
	passivityPenalty := 0.0
	if survival > 0.95 && engagementScore < 0.3 {
		// Perfect survival but terrible engagement = passive hiding
		passivityPenalty = (0.3 - engagementScore) * 0.15 // Up to -4.5% penalty
	}

	// 85% survival weight, 15% engagement weight (when soldiers survive)
	// Engagement rewards scale with survival to avoid rewarding suicidal aggression
	fitness := (survivalBonus * 0.85) + (engagementScore * survival * 0.15) - passivityPenalty

	return fitness
}

// RegularSoldierFitnessFunction seeks effective "average" soldiers who are cost-efficient.
// Goal: Active engagement, intelligent decisions (including tactical retreats), high survival,
// but NOT expensive special forces. Favors Regular soldiers (cost 4-6) over elites.
func RegularSoldierFitnessFunction(results *TestGenomeResults, cost float64) float64 {
	survival := results.AvgSurvivalRate
	kdNorm := math.Min(results.AvgKDRatio/3.0, 1.0)
	winRate := results.WinRate

	// Base performance score (same as SurvivalFitnessFunction)
	survivalBonus := survival * survival
	if survival < 0.5 {
		survivalBonus *= survival
	}

	// Engagement score: active participation is critical
	engagementScore := (kdNorm * 0.4) + (winRate * 0.6)

	// Passivity penalty: punish hiding behavior
	passivityPenalty := 0.0
	if survival > 0.95 && engagementScore < 0.3 {
		passivityPenalty = (0.3 - engagementScore) * 0.20 // Stronger than before
	}

	// Base fitness: 80% survival, 20% engagement
	baseFitness := (survivalBonus * 0.80) + (engagementScore * survival * 0.20) - passivityPenalty

	// Cost penalty/bonus: STRONGLY favor Regular soldiers (4-6 cost)
	// This is the key to preventing Special Forces evolution
	var costMultiplier float64
	switch {
	case cost < 3.0:
		// Too cheap = undertrained, risky
		costMultiplier = 0.6 // 40% penalty
	case cost < 4.0:
		// Recruit: acceptable but not ideal
		costMultiplier = 0.85 // 15% penalty
	case cost >= 4.0 && cost <= 6.0:
		// SWEET SPOT: Regular soldiers - cost-effective and capable
		// Give bonus for being in the ideal range
		centerBonus := 1.0 - math.Abs(cost-5.0)*0.05 // Peak at cost=5.0
		costMultiplier = 1.0 + centerBonus*0.3       // Up to 30% bonus at cost=5.0
	case cost < 8.0:
		// Veteran: too expensive for marginal gains
		costMultiplier = 0.7 // 30% penalty
	case cost < 10.0:
		// Elite: way too expensive
		costMultiplier = 0.4 // 60% penalty
	default:
		// Special Forces: completely unacceptable cost
		costMultiplier = 0.2 // 80% penalty
	}

	return baseFitness * costMultiplier
}

// OperationalFitnessFunction balances survival with mission effectiveness.
// Still heavily survival-focused but rewards tactical success when achieved safely.
func OperationalFitnessFunction(results *TestGenomeResults) float64 {
	survival := results.AvgSurvivalRate
	kdNorm := math.Min(results.AvgKDRatio/3.0, 1.0)
	winRate := results.WinRate

	// Survival is primary, but tactical effectiveness matters when soldiers live
	survivalWeight := 0.75
	tacticalWeight := 0.25 * survival // Tactical rewards scale with survival rate

	fitness := (survival * survivalWeight) + ((kdNorm*0.4 + winRate*0.6) * tacticalWeight)

	return fitness
}

// ZeroCasualtyFitnessFunction aims for perfect survival with secondary objectives.
// Massive penalties for any casualties, designed for training scenarios.
func ZeroCasualtyFitnessFunction(results *TestGenomeResults) float64 {
	survival := results.AvgSurvivalRate
	kdNorm := math.Min(results.AvgKDRatio/3.0, 1.0)
	winRate := results.WinRate

	var fitness float64

	// Perfect survival gets full score, any casualties get huge penalty
	switch {
	case survival >= 0.95:
		// Near-perfect survival: reward tactical performance
		fitness = 0.7 + (kdNorm * 0.15) + (winRate * 0.15)
	case survival >= 0.8:
		// Moderate casualties: acceptable but penalized
		fitness = survival*0.6 + (kdNorm * survival * 0.2) + (winRate * survival * 0.2)
	default:
		// High casualties: massive penalty, fitness approaches zero
		fitness = survival * survival * 0.3
	}

	return fitness
}

// GeneticOperators contains the parameters for genetic algorithm operations.
type GeneticOperators struct {
	CrossoverRate  float64 // Probability of crossover (typically 0.7-0.9)
	MutationRate   float64 // Probability of mutation per gene (typically 0.01-0.15)
	MutationSigma  float64 // Standard deviation for Gaussian mutation (typically 0.05-0.15)
	TournamentSize int     // Number of genomes in tournament selection (typically 3-5)
	EliteCount     int     // Number of top genomes to preserve (typically 2-10)
}

// DefaultGeneticOperators returns recommended GA parameters.
func DefaultGeneticOperators() GeneticOperators {
	return GeneticOperators{
		CrossoverRate:  0.8,
		MutationRate:   0.15,
		MutationSigma:  0.1,
		TournamentSize: 3,
		EliteCount:     5,
	}
}

// TournamentSelection selects a genome using tournament selection.
func TournamentSelection(population []Genome, tournamentSize int, rng *rand.Rand) Genome {
	best := population[rng.Intn(len(population))]
	for i := 1; i < tournamentSize; i++ {
		candidate := population[rng.Intn(len(population))]
		if candidate.Fitness > best.Fitness {
			best = candidate
		}
	}
	return best.Clone()
}

// UniformCrossover performs uniform crossover between two parent genomes.
func UniformCrossover(parent1, parent2 Genome, rng *rand.Rand) (Genome, Genome) {
	child1 := parent1.Clone()
	child2 := parent2.Clone()

	for i := 0; i < NumGenes; i++ {
		if rng.Float64() < 0.5 {
			// Swap genes
			child1.Genes[i], child2.Genes[i] = child2.Genes[i], child1.Genes[i]
		}
	}

	return child1, child2
}

// GaussianMutate applies Gaussian mutation to a genome.
func GaussianMutate(genome Genome, mutationRate, sigma float64, rng *rand.Rand) Genome {
	mutated := genome.Clone()

	for i := 0; i < NumGenes; i++ {
		if rng.Float64() < mutationRate {
			// Add Gaussian noise
			noise := rng.NormFloat64() * sigma
			mutated.Genes[i] += noise
		}
	}

	// Ensure genes stay within bounds
	mutated.Clamp()

	return mutated
}

// Evolve creates the next generation using genetic operators.
func Evolve(population *Population, operators GeneticOperators, rng *rand.Rand) Population {
	// Sort by fitness
	population.SortByFitness()

	nextGen := make([]Genome, 0, len(population.Genomes))

	// Elitism: preserve top genomes
	for i := 0; i < operators.EliteCount && i < len(population.Genomes); i++ {
		elite := population.Genomes[i].Clone()
		elite.ID = i // Reassign IDs for next generation
		nextGen = append(nextGen, elite)
	}

	// Generate offspring to fill the rest of the population
	nextID := operators.EliteCount
	for len(nextGen) < len(population.Genomes) {
		// Selection
		parent1 := TournamentSelection(population.Genomes, operators.TournamentSize, rng)
		parent2 := TournamentSelection(population.Genomes, operators.TournamentSize, rng)

		// Crossover
		var child1, child2 Genome
		if rng.Float64() < operators.CrossoverRate {
			child1, child2 = UniformCrossover(parent1, parent2, rng)
		} else {
			child1 = parent1.Clone()
			child2 = parent2.Clone()
		}

		// Mutation
		child1 = GaussianMutate(child1, operators.MutationRate, operators.MutationSigma, rng)
		child2 = GaussianMutate(child2, operators.MutationRate, operators.MutationSigma, rng)

		// Assign IDs and add to next generation
		child1.ID = nextID
		nextID++
		nextGen = append(nextGen, child1)

		if len(nextGen) < len(population.Genomes) {
			child2.ID = nextID
			nextID++
			nextGen = append(nextGen, child2)
		}
	}

	return Population{
		Genomes:    nextGen,
		Generation: population.Generation + 1,
	}
}
