package game

import (
	"fmt"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

const (
	logPanelWidth     = 720
	logMaxEntries     = 200
	logLineHeight     = 14
	logScale          = 3   // text rendered at 1x into buffer, blitted at 3× for readability
	squadPollInterval = 300 // ticks between squad summary polls (~5s at 60TPS)
)

// LogCategory tags each thought log entry for filtering.
type LogCategory uint8

const (
	LogCatRadio   LogCategory = iota // radio transmissions (clear/garbled/drop/timeout)
	LogCatSquad                      // periodic squad status polls
	LogCatSpeech                     // soldier speech bubbles
	LogCatThought                    // individual soldier goal/state changes
	logCatCount                      // sentinel — must be last
)

func (c LogCategory) ShortName() string {
	switch c {
	case LogCatRadio:
		return "RAD"
	case LogCatSquad:
		return "SQD"
	case LogCatSpeech:
		return "SPK"
	case LogCatThought:
		return "THK"
	default:
		return "???"
	}
}

// ThoughtEntry is a single line in the thought log.
type ThoughtEntry struct {
	Tick     int
	Label    string // e.g. "R1", "B3", "SQ-R"
	Team     Team
	Message  string
	Category LogCategory
}

// ThoughtLog is a ring buffer of soldier thought entries rendered on-screen.
type ThoughtLog struct {
	entries []ThoughtEntry
	head    int
	count   int
	// Filter state: true = category is visible.
	filters [logCatCount]bool
}

// NewThoughtLog creates a thought log with a fixed capacity.
func NewThoughtLog() *ThoughtLog {
	tl := &ThoughtLog{
		entries: make([]ThoughtEntry, logMaxEntries),
	}
	// All categories visible by default.
	for i := range tl.filters {
		tl.filters[i] = true
	}
	return tl
}

// ToggleFilter flips the visibility of a log category.
func (tl *ThoughtLog) ToggleFilter(cat LogCategory) {
	if int(cat) < len(tl.filters) {
		tl.filters[cat] = !tl.filters[cat]
	}
}

// FilterEnabled returns whether a category is currently shown.
func (tl *ThoughtLog) FilterEnabled(cat LogCategory) bool {
	if int(cat) < len(tl.filters) {
		return tl.filters[cat]
	}
	return true
}

// Add appends an entry to the log with the given category.
func (tl *ThoughtLog) Add(tick int, label string, team Team, msg string, cat LogCategory) {
	tl.entries[tl.head] = ThoughtEntry{
		Tick:     tick,
		Label:    label,
		Team:     team,
		Message:  msg,
		Category: cat,
	}
	tl.head = (tl.head + 1) % logMaxEntries
	if tl.count < logMaxEntries {
		tl.count++
	}
}

// Recent returns entries in chronological order (oldest first).
func (tl *ThoughtLog) Recent() []ThoughtEntry {
	result := make([]ThoughtEntry, 0, tl.count)
	for i := 0; i < tl.count; i++ {
		idx := (tl.head - tl.count + i + logMaxEntries) % logMaxEntries
		e := tl.entries[idx]
		if tl.filters[e.Category] {
			result = append(result, e)
		}
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
		tl.Add(tick, label, sq.Team, "WIPED OUT", LogCatSquad)
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
	if sq.ActiveOrder.IsActiveAt(tick) {
		statusLine += fmt.Sprintf(" ord:%s", sq.ActiveOrder.Kind)
	}
	if activated > 0 {
		statusLine += fmt.Sprintf(" act:%d", activated)
	}

	tl.Add(tick, label, sq.Team, statusLine, LogCatSquad)
	tl.Add(tick, label, sq.Team, goalStr, LogCatSquad)
}

// AddSpeech logs a soldier speech event.
func (tl *ThoughtLog) AddSpeech(tick int, label string, team Team, msg string) {
	tl.Add(tick, label, team, msg, LogCatSpeech)
}

// logCategoryColor returns the tint for a category indicator.
func logCategoryColor(cat LogCategory) color.RGBA {
	switch cat {
	case LogCatRadio:
		return color.RGBA{R: 100, G: 220, B: 160, A: 255}
	case LogCatSquad:
		return color.RGBA{R: 200, G: 200, B: 100, A: 255}
	case LogCatSpeech:
		return color.RGBA{R: 180, G: 140, B: 220, A: 255}
	case LogCatThought:
		return color.RGBA{R: 140, G: 180, B: 140, A: 255}
	default:
		return color.RGBA{R: 128, G: 128, B: 128, A: 255}
	}
}

// Draw renders the thought log into the provided offscreen buffer at 1× scale.
// The caller is responsible for blitting this buffer onto the screen at logScale.
func (tl *ThoughtLog) Draw(buf *ebiten.Image, bufW, bufH int) {
	buf.Clear()

	// Panel background.
	vector.FillRect(buf, 0, 0, float32(bufW), float32(bufH), color.RGBA{R: 10, G: 12, B: 10, A: 248}, false)
	// Left separator line.
	vector.StrokeLine(buf, 0, 0, 0, float32(bufH), 1.0, color.RGBA{R: 60, G: 90, B: 60, A: 255}, false)

	// Title bar background.
	vector.FillRect(buf, 0, 0, float32(bufW), 14, color.RGBA{R: 22, G: 34, B: 22, A: 255}, false)
	ebitenutil.DebugPrintAt(buf, "LOG", 4, 2)

	// Filter toggle indicators in the title bar.
	filterX := 30
	filterKeys := [logCatCount]string{"F5", "F6", "F7", "F8"}
	for i := LogCategory(0); i < logCatCount; i++ {
		label := fmt.Sprintf("[%s]%s", filterKeys[i], i.ShortName())
		if tl.filters[i] {
			ebitenutil.DebugPrintAt(buf, label, filterX, 2)
		} else {
			// Dim the label when the filter is off.
			ebitenutil.DebugPrintAt(buf, label, filterX, 2)
			// Overdraw a dark rect to dim it.
			labelW := float32(len(label) * 6)
			vector.FillRect(buf, float32(filterX), 2, labelW, 12, color.RGBA{R: 10, G: 12, B: 10, A: 180}, false)
		}
		filterX += len(label)*6 + 6
	}

	// Title separator.
	vector.StrokeLine(buf, 0, 14, float32(bufW), 14, 1.0, color.RGBA{R: 60, G: 100, B: 60, A: 220}, false)

	entries := tl.Recent()

	// Draw from bottom up so newest is at bottom.
	topMargin := 17
	maxVisible := (bufH - topMargin - 2) / logLineHeight
	startIdx := 0
	if len(entries) > maxVisible {
		startIdx = len(entries) - maxVisible
	}

	visible := entries[startIdx:]
	recent := 5 // how many latest entries to highlight

	y := topMargin
	for i, e := range visible {
		isRecent := i >= len(visible)-recent

		// Team colour dot.
		var dotCol color.RGBA
		if e.Team == TeamRed {
			dotCol = color.RGBA{R: 230, G: 70, B: 60, A: 255}
		} else {
			dotCol = color.RGBA{R: 70, G: 120, B: 230, A: 255}
		}

		// Highlight row background for recent entries.
		if isRecent {
			vector.FillRect(buf, 2, float32(y), float32(bufW-4), float32(logLineHeight), color.RGBA{R: 30, G: 45, B: 30, A: 180}, false)
		}

		// Team colour indicator stripe.
		vector.FillRect(buf, 2, float32(y+1), 2, float32(logLineHeight-2), dotCol, false)

		// Category colour pip.
		catCol := logCategoryColor(e.Category)
		vector.FillRect(buf, 5, float32(y+1), 2, float32(logLineHeight-2), catCol, false)

		// Tick + label + message.
		line := fmt.Sprintf("%4d [%s] %s", e.Tick, e.Label, e.Message)
		ebitenutil.DebugPrintAt(buf, line, 10, y)
		y += logLineHeight
	}
}
