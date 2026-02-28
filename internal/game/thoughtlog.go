package game

import (
	"fmt"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

const (
	logPanelWidth     = 480
	logMaxEntries     = 120
	logLineHeight     = 14
	squadPollInterval = 300 // ticks between squad summary polls (~5s at 60TPS)
)

// ThoughtEntry is a single line in the thought log.
type ThoughtEntry struct {
	Tick    int
	Label   string // e.g. "R1", "B3", "SQ-R"
	Team    Team
	Message string
}

// ThoughtLog is a ring buffer of soldier thought entries rendered on-screen.
type ThoughtLog struct {
	entries []ThoughtEntry
	head    int
	count   int
}

// NewThoughtLog creates a thought log with a fixed capacity.
func NewThoughtLog() *ThoughtLog {
	return &ThoughtLog{
		entries: make([]ThoughtEntry, logMaxEntries),
	}
}

// Add appends an entry to the log. Individual soldier calls are now throttled:
// only goal changes and combat events pass through. Squad summaries are always added.
func (tl *ThoughtLog) Add(tick int, label string, team Team, msg string) {
	tl.entries[tl.head] = ThoughtEntry{
		Tick:    tick,
		Label:   label,
		Team:    team,
		Message: msg,
	}
	tl.head = (tl.head + 1) % logMaxEntries
	if tl.count < logMaxEntries {
		tl.count++
	}
}

// Recent returns entries in chronological order (oldest first).
func (tl *ThoughtLog) Recent() []ThoughtEntry {
	result := make([]ThoughtEntry, tl.count)
	for i := 0; i < tl.count; i++ {
		idx := (tl.head - tl.count + i + logMaxEntries) % logMaxEntries
		result[i] = tl.entries[idx]
	}
	return result
}

// AddSquadPoll generates a compact squad status summary showing goal distribution
// and key stats. Called every squadPollInterval ticks from the game loop.
func (tl *ThoughtLog) AddSquadPoll(tick int, sq *Squad) {
	if sq.Leader == nil {
		return
	}

	label := "SQ-R"
	if sq.Team == TeamBlue {
		label = "SQ-B"
	}

	// Count goals across alive members.
	goalCounts := map[GoalKind]int{}
	alive := 0
	totalFear := 0.0
	activated := 0
	for _, m := range sq.Members {
		if m.state == SoldierStateDead {
			continue
		}
		alive++
		goalCounts[m.blackboard.CurrentGoal]++
		totalFear += m.profile.Psych.EffectiveFear()
		if m.blackboard.IsActivated() {
			activated++
		}
	}
	if alive == 0 {
		tl.Add(tick, label, sq.Team, "WIPED OUT")
		return
	}

	avgFear := totalFear / float64(alive)

	// Build compact goal distribution string.
	goalStr := ""
	order := []GoalKind{
		GoalAdvance, GoalMaintainFormation, GoalMoveToContact, GoalEngage,
		GoalFlank, GoalOverwatch, GoalFallback, GoalSurvive,
		GoalHoldPosition, GoalRegroup,
	}
	abbrev := map[GoalKind]string{
		GoalAdvance:           "ADV",
		GoalMaintainFormation: "FRM",
		GoalMoveToContact:     "MTC",
		GoalEngage:            "ENG",
		GoalFlank:             "FLK",
		GoalOverwatch:         "OVW",
		GoalFallback:          "FBK",
		GoalSurvive:           "SRV",
		GoalHoldPosition:      "HLD",
		GoalRegroup:           "RGP",
	}
	for _, g := range order {
		if n, ok := goalCounts[g]; ok && n > 0 {
			if goalStr != "" {
				goalStr += " "
			}
			goalStr += fmt.Sprintf("%s:%d", abbrev[g], n)
		}
	}

	// Status line: intent + alive + fear + activation.
	statusLine := fmt.Sprintf("%s %d/%d f:%.0f%%",
		sq.Intent, alive, len(sq.Members), avgFear*100)
	if activated > 0 {
		statusLine += fmt.Sprintf(" act:%d", activated)
	}

	tl.Add(tick, label, sq.Team, statusLine)
	tl.Add(tick, label, sq.Team, goalStr)
}

// AddSpeech logs a soldier speech event. Prefixed with speech marks for visual distinction.
func (tl *ThoughtLog) AddSpeech(tick int, label string, team Team, msg string) {
	tl.Add(tick, label, team, msg)
}

// Draw renders the thought log panel on the right side of the screen.
func (tl *ThoughtLog) Draw(screen *ebiten.Image, panelX int, panelH int) {
	// Panel background.
	vector.FillRect(screen, float32(panelX), 0, float32(logPanelWidth), float32(panelH), color.RGBA{R: 10, G: 12, B: 10, A: 248}, false)
	// Left separator line.
	vector.StrokeLine(screen, float32(panelX), 0, float32(panelX), float32(panelH), 1.0, color.RGBA{R: 50, G: 70, B: 50, A: 255}, false)

	// Title bar background.
	vector.FillRect(screen, float32(panelX), 0, float32(logPanelWidth), 16, color.RGBA{R: 20, G: 30, B: 20, A: 255}, false)
	ebitenutil.DebugPrintAt(screen, "SQUAD LOG", panelX+8, 2)
	// Title separator.
	vector.StrokeLine(screen, float32(panelX), 16, float32(panelX+logPanelWidth), 16, 1.0, color.RGBA{R: 50, G: 80, B: 50, A: 200}, false)

	entries := tl.Recent()

	// Draw from bottom up so newest is at bottom.
	maxVisible := (panelH - 24) / logLineHeight
	startIdx := 0
	if len(entries) > maxVisible {
		startIdx = len(entries) - maxVisible
	}

	visible := entries[startIdx:]
	recent := 5 // how many latest entries to highlight

	y := 20
	for i, e := range visible {
		isRecent := i >= len(visible)-recent

		// Team colour dot.
		var dotCol color.RGBA
		if e.Team == TeamRed {
			dotCol = color.RGBA{R: 210, G: 70, B: 70, A: 255}
		} else {
			dotCol = color.RGBA{R: 70, G: 110, B: 210, A: 255}
		}

		// Highlight row background for recent entries.
		if isRecent {
			vector.FillRect(screen, float32(panelX+2), float32(y), float32(logPanelWidth-4), float32(logLineHeight), color.RGBA{R: 30, G: 40, B: 30, A: 160}, false)
		}

		// Team colour indicator dot.
		vector.FillRect(screen, float32(panelX+5), float32(y+3), 3, 5, dotCol, false)

		// Tick + label + message.
		line := fmt.Sprintf("%4d [%s] %s", e.Tick, e.Label, e.Message)
		ebitenutil.DebugPrintAt(screen, line, panelX+12, y)
		y += logLineHeight
	}
}
