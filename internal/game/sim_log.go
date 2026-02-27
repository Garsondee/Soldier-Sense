package game

import (
	"fmt"
	"strings"
)

// SimLogEntry is one recorded event during a headless test simulation.
type SimLogEntry struct {
	Tick     int
	Soldier  string  // label e.g. "R0", "B3", or "--" for global events
	Team     string  // "red", "blue", or "--"
	Category string  // goal, squad, move, vision, threat, stance, state, stats, formation
	Key      string  // specific event name within the category
	Value    string  // human-readable detail
	NumVal   float64 // optional numeric value for threshold checks
}

// String formats the entry as a fixed-width log line.
//
//	[T=042] R0   squad   intent_change   advance → hold
func (e SimLogEntry) String() string {
	return fmt.Sprintf("[T=%03d] %-4s %-9s %-16s %s",
		e.Tick, e.Soldier, e.Category, e.Key, e.Value)
}

// SimLog collects structured events during a headless test simulation.
// Unlike ThoughtLog (UI ring-buffer), SimLog is unbounded and machine-readable.
type SimLog struct {
	entries []SimLogEntry
	verbose bool
}

// NewSimLog creates a SimLog. If verbose is true, per-tick position/speed/stat
// entries are also recorded (useful for detailed debugging).
func NewSimLog(verbose bool) *SimLog {
	return &SimLog{verbose: verbose}
}

// Add records a new entry.
func (sl *SimLog) Add(tick int, soldier, team, category, key, value string, numVal float64) {
	sl.entries = append(sl.entries, SimLogEntry{
		Tick:     tick,
		Soldier:  soldier,
		Team:     team,
		Category: category,
		Key:      key,
		Value:    value,
		NumVal:   numVal,
	})
}

// AddVerbose records an entry only when verbose mode is on.
func (sl *SimLog) AddVerbose(tick int, soldier, team, category, key, value string, numVal float64) {
	if !sl.verbose {
		return
	}
	sl.Add(tick, soldier, team, category, key, value, numVal)
}

// Entries returns all recorded entries.
func (sl *SimLog) Entries() []SimLogEntry {
	return sl.entries
}

// Filter returns entries matching the given category and/or key.
// Pass empty string to match any value for that field.
func (sl *SimLog) Filter(category, key string) []SimLogEntry {
	var out []SimLogEntry
	for _, e := range sl.entries {
		if category != "" && e.Category != category {
			continue
		}
		if key != "" && e.Key != key {
			continue
		}
		out = append(out, e)
	}
	return out
}

// FilterSoldier returns entries for a specific soldier label.
func (sl *SimLog) FilterSoldier(label string) []SimLogEntry {
	var out []SimLogEntry
	for _, e := range sl.entries {
		if e.Soldier == label {
			out = append(out, e)
		}
	}
	return out
}

// FilterTickRange returns entries within [fromTick, toTick] inclusive.
func (sl *SimLog) FilterTickRange(fromTick, toTick int) []SimLogEntry {
	var out []SimLogEntry
	for _, e := range sl.entries {
		if e.Tick >= fromTick && e.Tick <= toTick {
			out = append(out, e)
		}
	}
	return out
}

// CountCategory returns how many entries match the given category and key.
func (sl *SimLog) CountCategory(category, key string) int {
	return len(sl.Filter(category, key))
}

// LastOf returns the most recent entry matching category+key, or false if none.
func (sl *SimLog) LastOf(category, key string) (SimLogEntry, bool) {
	entries := sl.Filter(category, key)
	if len(entries) == 0 {
		return SimLogEntry{}, false
	}
	return entries[len(entries)-1], true
}

// HasEntry returns true if at least one entry matches category, key, and value substring.
func (sl *SimLog) HasEntry(category, key, valueSubstr string) bool {
	for _, e := range sl.entries {
		if category != "" && e.Category != category {
			continue
		}
		if key != "" && e.Key != key {
			continue
		}
		if valueSubstr != "" && !strings.Contains(e.Value, valueSubstr) {
			continue
		}
		return true
	}
	return false
}

// Format returns the full log as a single string for t.Log output.
func (sl *SimLog) Format() string {
	var sb strings.Builder
	for _, e := range sl.entries {
		sb.WriteString(e.String())
		sb.WriteByte('\n')
	}
	return sb.String()
}

// FormatRange returns a log string filtered to a tick range.
func (sl *SimLog) FormatRange(fromTick, toTick int) string {
	var sb strings.Builder
	for _, e := range sl.FilterTickRange(fromTick, toTick) {
		sb.WriteString(e.String())
		sb.WriteByte('\n')
	}
	return sb.String()
}

// Summary returns a short human-readable summary of the simulation state.
// soldiers is the full list of all soldiers in the simulation.
func (sl *SimLog) Summary(tick int, soldiers []*Soldier, squads []*Squad) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "--- Summary at T=%03d ---\n", tick)

	// Goal distribution per team.
	goalCount := map[Team]map[GoalKind]int{}
	for _, s := range soldiers {
		if _, ok := goalCount[s.team]; !ok {
			goalCount[s.team] = map[GoalKind]int{}
		}
		goalCount[s.team][s.blackboard.CurrentGoal]++
	}
	for _, team := range []Team{TeamRed, TeamBlue} {
		counts, ok := goalCount[team]
		if !ok {
			continue
		}
		label := "Red"
		if team == TeamBlue {
			label = "Blue"
		}
		fmt.Fprintf(&sb, "%s goals: ", label)
		goals := []GoalKind{GoalAdvance, GoalMaintainFormation, GoalRegroup, GoalHoldPosition, GoalSurvive, GoalEngage, GoalMoveToContact, GoalFallback}
		for _, g := range goals {
			if n := counts[g]; n > 0 {
				fmt.Fprintf(&sb, "%s=%d  ", g, n)
			}
		}
		sb.WriteByte('\n')
	}

	// Squad spread.
	for _, sq := range squads {
		if sq.Leader == nil {
			continue
		}
		teamLabel := "Red"
		if sq.Team == TeamBlue {
			teamLabel = "Blue"
		}
		spread := sq.squadSpread()
		fmt.Fprintf(&sb, "%s squad spread: %.1fpx  intent: %s\n", teamLabel, spread, sq.Intent)
	}

	// Alive counts.
	redAlive, blueAlive := 0, 0
	for _, s := range soldiers {
		if s.state != SoldierStateDead {
			if s.team == TeamRed {
				redAlive++
			} else {
				blueAlive++
			}
		}
	}
	fmt.Fprintf(&sb, "Alive: red=%d  blue=%d\n", redAlive, blueAlive)

	// Active contacts.
	contactLines := 0
	for _, s := range soldiers {
		if len(s.vision.KnownContacts) > 0 {
			targets := make([]string, len(s.vision.KnownContacts))
			for i, c := range s.vision.KnownContacts {
				targets[i] = c.label
			}
			fmt.Fprintf(&sb, "Contacts: %s → [%s]\n", s.label, strings.Join(targets, ", "))
			contactLines++
		}
	}
	if contactLines == 0 {
		sb.WriteString("Contacts: none\n")
	}

	return sb.String()
}
