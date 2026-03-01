package game

type BattleOutcome int

const (
	OutcomeInconclusive BattleOutcome = iota
	OutcomeRedVictory
	OutcomeBlueVictory
	OutcomeDraw
)

func (o BattleOutcome) String() string {
	switch o {
	case OutcomeRedVictory:
		return "red_victory"
	case OutcomeBlueVictory:
		return "blue_victory"
	case OutcomeDraw:
		return "draw"
	case OutcomeInconclusive:
		return "inconclusive"
	default:
		return "unknown"
	}
}

type BattleOutcomeReason struct {
	Outcome          BattleOutcome
	RedSurvivors     int
	RedTotal         int
	BlueSurvivors    int
	BlueTotal        int
	RedSquadsBroken  int
	RedSquadsTotal   int
	BlueSquadsBroken int
	BlueSquadsTotal  int
	RedFled          int
	BlueFled         int
	Description      string
}

func DetermineBattleOutcome(redSoldiers, blueSoldiers []*Soldier, redSquads, blueSquads []*Squad) BattleOutcomeReason {
	redTotal := len(redSoldiers)
	blueTotal := len(blueSoldiers)
	redSurvivors := 0
	blueSurvivors := 0
	redFled := 0
	blueFled := 0

	for _, s := range redSoldiers {
		if s.state != SoldierStateDead {
			redSurvivors++
		}
	}
	for _, s := range blueSoldiers {
		if s.state != SoldierStateDead {
			blueSurvivors++
		}
	}

	redSquadsTotal := len(redSquads)
	blueSquadsTotal := len(blueSquads)
	redSquadsBroken := 0
	blueSquadsBroken := 0

	for _, sq := range redSquads {
		if sq.Broken {
			redSquadsBroken++
		}
	}
	for _, sq := range blueSquads {
		if sq.Broken {
			blueSquadsBroken++
		}
	}

	// Calculate casualty rates
	redCasualtyRate := 0.0
	blueCasualtyRate := 0.0
	if redTotal > 0 {
		redCasualtyRate = float64(redTotal-redSurvivors) / float64(redTotal)
	}
	if blueTotal > 0 {
		blueCasualtyRate = float64(blueTotal-blueSurvivors) / float64(blueTotal)
	}

	extremeSquadCollapseVictory := func(winner BattleOutcome, desc string) BattleOutcomeReason {
		return BattleOutcomeReason{
			Outcome:          winner,
			RedSurvivors:     redSurvivors,
			RedTotal:         redTotal,
			BlueSurvivors:    blueSurvivors,
			BlueTotal:        blueTotal,
			RedSquadsBroken:  redSquadsBroken,
			RedSquadsTotal:   redSquadsTotal,
			BlueSquadsBroken: blueSquadsBroken,
			BlueSquadsTotal:  blueSquadsTotal,
			RedFled:          redFled,
			BlueFled:         blueFled,
			Description:      desc,
		}
	}

	if redSurvivors == 0 && blueSurvivors > 0 {
		return BattleOutcomeReason{
			Outcome:          OutcomeBlueVictory,
			RedSurvivors:     redSurvivors,
			RedTotal:         redTotal,
			BlueSurvivors:    blueSurvivors,
			BlueTotal:        blueTotal,
			RedSquadsBroken:  redSquadsBroken,
			RedSquadsTotal:   redSquadsTotal,
			BlueSquadsBroken: blueSquadsBroken,
			BlueSquadsTotal:  blueSquadsTotal,
			RedFled:          redFled,
			BlueFled:         blueFled,
			Description:      "decisive_blue_victory_red_eliminated",
		}
	}
	if blueSurvivors == 0 && redSurvivors > 0 {
		return BattleOutcomeReason{
			Outcome:          OutcomeRedVictory,
			RedSurvivors:     redSurvivors,
			RedTotal:         redTotal,
			BlueSurvivors:    blueSurvivors,
			BlueTotal:        blueTotal,
			RedSquadsBroken:  redSquadsBroken,
			RedSquadsTotal:   redSquadsTotal,
			BlueSquadsBroken: blueSquadsBroken,
			BlueSquadsTotal:  blueSquadsTotal,
			RedFled:          redFled,
			BlueFled:         blueFled,
			Description:      "decisive_red_victory_blue_eliminated",
		}
	}

	if redSurvivors == 0 && blueSurvivors == 0 {
		return BattleOutcomeReason{
			Outcome:          OutcomeDraw,
			RedSurvivors:     redSurvivors,
			RedTotal:         redTotal,
			BlueSurvivors:    blueSurvivors,
			BlueTotal:        blueTotal,
			RedSquadsBroken:  redSquadsBroken,
			RedSquadsTotal:   redSquadsTotal,
			BlueSquadsBroken: blueSquadsBroken,
			BlueSquadsTotal:  blueSquadsTotal,
			RedFled:          redFled,
			BlueFled:         blueFled,
			Description:      "mutual_annihilation",
		}
	}

	if redSquadsTotal > 0 && blueSquadsTotal > 0 {
		if blueSquadsBroken == blueSquadsTotal && redSquadsBroken < redSquadsTotal {
			if blueSurvivors == 0 {
				return extremeSquadCollapseVictory(OutcomeRedVictory, "decisive_red_victory_blue_eliminated")
			}
			if blueCasualtyRate >= 0.80 && blueSurvivors <= max(1, blueTotal/4) {
				return extremeSquadCollapseVictory(OutcomeRedVictory, "red_victory_blue_combat_ineffective")
			}
		}
		if redSquadsBroken == redSquadsTotal && blueSquadsBroken < blueSquadsTotal {
			if redSurvivors == 0 {
				return extremeSquadCollapseVictory(OutcomeBlueVictory, "decisive_blue_victory_red_eliminated")
			}
			if redCasualtyRate >= 0.80 && redSurvivors <= max(1, redTotal/4) {
				return extremeSquadCollapseVictory(OutcomeBlueVictory, "blue_victory_red_combat_ineffective")
			}
		}
		if redSquadsBroken == redSquadsTotal && blueSquadsBroken == blueSquadsTotal {
			if redCasualtyRate >= 0.70 && blueCasualtyRate >= 0.70 {
				return extremeSquadCollapseVictory(OutcomeDraw, "draw_mutual_rout")
			}
		}
	}

	casualtyDiff := blueCasualtyRate - redCasualtyRate
	if casualtyDiff > 0.30 && redCasualtyRate < 0.50 {
		return BattleOutcomeReason{
			Outcome:          OutcomeRedVictory,
			RedSurvivors:     redSurvivors,
			RedTotal:         redTotal,
			BlueSurvivors:    blueSurvivors,
			BlueTotal:        blueTotal,
			RedSquadsBroken:  redSquadsBroken,
			RedSquadsTotal:   redSquadsTotal,
			BlueSquadsBroken: blueSquadsBroken,
			BlueSquadsTotal:  blueSquadsTotal,
			RedFled:          redFled,
			BlueFled:         blueFled,
			Description:      "marginal_red_victory_casualty_advantage",
		}
	}
	if casualtyDiff < -0.30 && blueCasualtyRate < 0.50 {
		return BattleOutcomeReason{
			Outcome:          OutcomeBlueVictory,
			RedSurvivors:     redSurvivors,
			RedTotal:         redTotal,
			BlueSurvivors:    blueSurvivors,
			BlueTotal:        blueTotal,
			RedSquadsBroken:  redSquadsBroken,
			RedSquadsTotal:   redSquadsTotal,
			BlueSquadsBroken: blueSquadsBroken,
			BlueSquadsTotal:  blueSquadsTotal,
			RedFled:          redFled,
			BlueFled:         blueFled,
			Description:      "marginal_blue_victory_casualty_advantage",
		}
	}

	if casualtyDiff >= -0.20 && casualtyDiff <= 0.20 && (redCasualtyRate > 0.30 || blueCasualtyRate > 0.30) {
		return BattleOutcomeReason{
			Outcome:          OutcomeDraw,
			RedSurvivors:     redSurvivors,
			RedTotal:         redTotal,
			BlueSurvivors:    blueSurvivors,
			BlueTotal:        blueTotal,
			RedSquadsBroken:  redSquadsBroken,
			RedSquadsTotal:   redSquadsTotal,
			BlueSquadsBroken: blueSquadsBroken,
			BlueSquadsTotal:  blueSquadsTotal,
			RedFled:          redFled,
			BlueFled:         blueFled,
			Description:      "draw_similar_casualties",
		}
	}

	return BattleOutcomeReason{
		Outcome:          OutcomeInconclusive,
		RedSurvivors:     redSurvivors,
		RedTotal:         redTotal,
		BlueSurvivors:    blueSurvivors,
		BlueTotal:        blueTotal,
		RedSquadsBroken:  redSquadsBroken,
		RedSquadsTotal:   redSquadsTotal,
		BlueSquadsBroken: blueSquadsBroken,
		BlueSquadsTotal:  blueSquadsTotal,
		RedFled:          redFled,
		BlueFled:         blueFled,
		Description:      "inconclusive_insufficient_resolution",
	}
}
