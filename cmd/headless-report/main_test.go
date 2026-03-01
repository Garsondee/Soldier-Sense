package main

import (
	"strings"
	"testing"

	"github.com/Garsondee/Soldier-Sense/internal/game"
)

func TestTeamSurvivalCounts(t *testing.T) {
	grades := []game.SoldierGrade{
		{Team: game.TeamRed, Survived: true},
		{Team: game.TeamRed, Survived: false},
		{Team: game.TeamBlue, Survived: true},
		{Team: game.TeamBlue, Survived: true},
	}

	redTotal, blueTotal, redSurvivors, blueSurvivors := teamSurvivalCounts(grades)
	if redTotal != 2 || blueTotal != 2 {
		t.Fatalf("expected totals red=2 blue=2, got red=%d blue=%d", redTotal, blueTotal)
	}
	if redSurvivors != 1 || blueSurvivors != 2 {
		t.Fatalf("expected survivors red=1 blue=2, got red=%d blue=%d", redSurvivors, blueSurvivors)
	}
}

func TestDetectStalemate_TrueWhenMutualSurvivalAndFrictionHigh(t *testing.T) {
	rs := runStats{
		redTotal:            6,
		blueTotal:           6,
		redSurvivors:        4,
		blueSurvivors:       4,
		stalledEvents:       28,
		detachedEvents:      4,
		cohesionBreakEvents: 0,
	}

	isStalemate, reason := detectStalemate(rs)
	if !isStalemate {
		t.Fatalf("expected stalemate=true, got false (reason=%s)", reason)
	}
	if !strings.Contains(reason, "high_mutual_survival") {
		t.Fatalf("expected reason to mention high_mutual_survival, got: %s", reason)
	}
}

func TestDetectStalemate_FalseWhenSquadBreakOccurs(t *testing.T) {
	rs := runStats{
		redTotal:            6,
		blueTotal:           6,
		redSurvivors:        4,
		blueSurvivors:       4,
		stalledEvents:       28,
		detachedEvents:      4,
		cohesionBreakEvents: 1,
	}

	isStalemate, reason := detectStalemate(rs)
	if isStalemate {
		t.Fatalf("expected stalemate=false when cohesion breaks occur (reason=%s)", reason)
	}
}

func TestDetectStalemate_FalseWhenAttritionDecisive(t *testing.T) {
	rs := runStats{
		redTotal:            6,
		blueTotal:           6,
		redSurvivors:        2,
		blueSurvivors:       5,
		stalledEvents:       28,
		detachedEvents:      4,
		cohesionBreakEvents: 0,
	}

	isStalemate, reason := detectStalemate(rs)
	if isStalemate {
		t.Fatalf("expected stalemate=false under decisive attrition (reason=%s)", reason)
	}
}
