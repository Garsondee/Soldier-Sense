package game

import (
	"fmt"
	"math"
	"strings"
)

func (g *Game) soldierDebugReport(selected *Soldier, lastTicks int) string {
	if selected == nil {
		return ""
	}
	fromTick, toTick := debugTickWindow(g.tick, lastTicks)

	leader := (*Soldier)(nil)
	if selected.squad != nil {
		leader = selected.squad.Leader
	}

	var b strings.Builder
	fmt.Fprintf(&b, "--- SoldierSense debug report ---\n")
	fmt.Fprintf(&b, "seed=%d tick_range=[%d..%d] ticks=%d\n", g.mapSeed, fromTick, toTick, toTick-fromTick+1)
	fmt.Fprintf(&b, "selected=%s team=%d leader=%v\n\n", selected.label, selected.team, leaderLabel(leader))

	writeSoldierTimelineReport(&b, "SELECTED", selected, fromTick, toTick)
	if leader != nil && leader != selected {
		writeSoldierTimelineReport(&b, "LEADER", leader, fromTick, toTick)
	}

	return b.String()
}

func debugTickWindow(currentTick, lastTicks int) (fromTick, toTick int) {
	if lastTicks <= 0 {
		lastTicks = 120
	}
	toTick = currentTick
	fromTick = toTick - lastTicks + 1
	if fromTick < 0 {
		fromTick = 0
	}
	return fromTick, toTick
}

func writeSoldierTimelineReport(b *strings.Builder, title string, s *Soldier, fromTick, toTick int) {
	if s == nil {
		return
	}
	fmt.Fprintf(b, "== %s (%s) ==\n", title, s.label)
	snaps := s.debugSnapshots(fromTick, toTick)
	if len(snaps) == 0 {
		b.WriteString("(no snapshots recorded yet)\n\n")
		return
	}

	summary := summarizeSnapshots(snaps)
	writeSnapshotSummaryLine(b, &summary)
	writeStoryEventLines(b, storyEvents(snaps))
	writeStageLines(b, snaps, buildStages(snaps), s.label)
	b.WriteByte('\n')
}

func writeSnapshotSummaryLine(b *strings.Builder, summary *soldierSnapshotSummary) {
	fmt.Fprintf(b,
		"summary: idle=%d moving=%d cover=%d movedTicks=%d maxIdleRun=%d pathTerminal=%d suppressTicks=%d\n",
		summary.idleTicks,
		summary.movingTicks,
		summary.coverTicks,
		summary.movedTicks,
		summary.maxIdleRun,
		summary.pathTerminalTicks,
		summary.suppressedTicks,
	)
	fmt.Fprintf(b,
		"         dLeader[min/avg/max]=%.0f/%.0f/%.0f  dSlot[min/avg/max]=%.0f/%.0f/%.0f  recNoPathMax=%d stallMax=%d\n",
		summary.minLeaderDist,
		summary.avgLeaderDist,
		summary.maxLeaderDist,
		summary.minSlotDist,
		summary.avgSlotDist,
		summary.maxSlotDist,
		summary.maxNoPathStreak,
		summary.maxMobilityStall,
	)
}

func writeStoryEventLines(b *strings.Builder, events []string) {
	if len(events) == 0 {
		return
	}
	b.WriteString("events:\n")
	for _, e := range events {
		b.WriteString("  - ")
		b.WriteString(e)
		b.WriteByte('\n')
	}
}

func writeStageLines(b *strings.Builder, snaps []SoldierDebugSnapshot, stages []reportStage, label string) {
	b.WriteString("stages:\n")
	for i := range stages {
		st := &stages[i]
		tag := ""
		if st.onlyIdle {
			tag = " [IDLE-RUN]"
		}
		fmt.Fprintf(b,
			"  %02d) T=%d..%d (%dt)%s state:%s->%s goal:%s intent:%s ord:%s imm:%t path:%d/%d->%d/%d dLeader:%.0f->%.0f moved:%.0f\n",
			i+1,
			st.startTick,
			st.endTick,
			st.count,
			tag,
			st.first.State,
			st.last.State,
			st.first.Goal,
			st.first.SquadIntent,
			st.first.OfficerOrderKind,
			st.first.OfficerOrderImmediate,
			st.first.PathIndex,
			st.first.PathLen,
			st.last.PathIndex,
			st.last.PathLen,
			st.first.DistToLeader,
			st.last.DistToLeader,
			st.movedDistance,
		)
		writeStageSampleLines(b, snaps, st, label)
	}
}

func writeStageSampleLines(b *strings.Builder, snaps []SoldierDebugSnapshot, st *reportStage, label string) {
	if st.count <= 3 {
		for j := st.startIdx; j <= st.endIdx; j++ {
			ss := &snaps[j]
			b.WriteString("      ")
			b.WriteString(ss.CompactString(label))
			b.WriteByte('\n')
		}
		return
	}
	b.WriteString("      first: ")
	b.WriteString(st.first.CompactString(label))
	b.WriteByte('\n')
	b.WriteString("      last:  ")
	b.WriteString(st.last.CompactString(label))
	b.WriteByte('\n')
}

type soldierSnapshotSummary struct {
	idleTicks         int
	movingTicks       int
	coverTicks        int
	movedTicks        int
	maxIdleRun        int
	pathTerminalTicks int
	suppressedTicks   int
	minLeaderDist     float64
	avgLeaderDist     float64
	maxLeaderDist     float64
	minSlotDist       float64
	avgSlotDist       float64
	maxSlotDist       float64
	maxNoPathStreak   int
	maxMobilityStall  int
}

func summarizeSnapshots(snaps []SoldierDebugSnapshot) soldierSnapshotSummary {
	if len(snaps) == 0 {
		return soldierSnapshotSummary{}
	}
	res := soldierSnapshotSummary{
		minLeaderDist: math.MaxFloat64,
		minSlotDist:   math.MaxFloat64,
	}
	idleRun := 0
	leaderSum := 0.0
	slotSum := 0.0
	for i := range snaps {
		s := &snaps[i]
		idleRun = updateStateRunMetrics(&res, s, idleRun)
		if i > 0 {
			if math.Hypot(s.X-snaps[i-1].X, s.Y-snaps[i-1].Y) > 0.75 {
				res.movedTicks++
			}
		}
		updateSuppressionAndPathMetrics(&res, s)
		if s.DistToLeader < res.minLeaderDist {
			res.minLeaderDist = s.DistToLeader
		}
		if s.DistToLeader > res.maxLeaderDist {
			res.maxLeaderDist = s.DistToLeader
		}
		leaderSum += s.DistToLeader
		updateSlotDistMetrics(&res, s)
		slotSum += s.DistToSlot
		updateRecoveryMetrics(&res, s)
	}
	if len(snaps) > 0 {
		res.avgLeaderDist = leaderSum / float64(len(snaps))
		res.avgSlotDist = slotSum / float64(len(snaps))
	}
	if res.minLeaderDist == math.MaxFloat64 {
		res.minLeaderDist = 0
	}
	if res.minSlotDist == math.MaxFloat64 {
		res.minSlotDist = 0
	}
	return res
}

func updateStateRunMetrics(res *soldierSnapshotSummary, s *SoldierDebugSnapshot, idleRun int) int {
	switch s.State {
	case SoldierStateIdle:
		res.idleTicks++
		idleRun++
		if idleRun > res.maxIdleRun {
			res.maxIdleRun = idleRun
		}
	case SoldierStateMoving:
		res.movingTicks++
		idleRun = 0
	case SoldierStateCover:
		res.coverTicks++
		idleRun = 0
	default:
		idleRun = 0
	}
	return idleRun
}

func updateSuppressionAndPathMetrics(res *soldierSnapshotSummary, s *SoldierDebugSnapshot) {
	if s.PathLen == 0 || s.PathIndex >= s.PathLen {
		res.pathTerminalTicks++
	}
	if s.IncomingFireCount > 0 || s.SuppressLevel > 0.001 {
		res.suppressedTicks++
	}
}

func updateSlotDistMetrics(res *soldierSnapshotSummary, s *SoldierDebugSnapshot) {
	if s.DistToSlot < res.minSlotDist {
		res.minSlotDist = s.DistToSlot
	}
	if s.DistToSlot > res.maxSlotDist {
		res.maxSlotDist = s.DistToSlot
	}
}

func updateRecoveryMetrics(res *soldierSnapshotSummary, s *SoldierDebugSnapshot) {
	if s.RecoveryNoPathStreak > res.maxNoPathStreak {
		res.maxNoPathStreak = s.RecoveryNoPathStreak
	}
	if s.MobilityStallTicks > res.maxMobilityStall {
		res.maxMobilityStall = s.MobilityStallTicks
	}
}

type reportStage struct {
	startIdx      int
	endIdx        int
	startTick     int
	endTick       int
	count         int
	first         SoldierDebugSnapshot
	last          SoldierDebugSnapshot
	movedDistance float64
	onlyIdle      bool
}

func buildStages(snaps []SoldierDebugSnapshot) []reportStage {
	if len(snaps) == 0 {
		return nil
	}
	keyOf := func(s SoldierDebugSnapshot) string {
		pathTerminal := s.PathLen == 0 || s.PathIndex >= s.PathLen
		stallBand := s.MobilityStallTicks / 5
		if stallBand > 6 {
			stallBand = 6
		}
		npBand := s.RecoveryNoPathStreak / 10
		if npBand > 8 {
			npBand = 8
		}
		return fmt.Sprintf("st=%d|g=%d|si=%d|ord=%d|imm=%t|b=%t|dashOn=%t|term=%t|supAbort=%t|ra=%d|stallBand=%d|npBand=%d|gpOn=%t|cpOn=%t|paOn=%t",
			s.State,
			s.Goal,
			s.SquadIntent,
			s.OfficerOrderKind,
			s.OfficerOrderImmediate,
			s.BoundMover,
			s.DashOverwatchTimer > 0,
			pathTerminal,
			s.SuppressionAbort,
			s.RecoveryAction,
			stallBand,
			npBand,
			s.GoalPauseTimer > 0,
			s.CognitionPauseTimer > 0,
			s.PostArrivalTimer > 0,
		)
	}

	stages := make([]reportStage, 0, 16)
	start := 0
	curKey := keyOf(snaps[0])
	for i := 1; i < len(snaps); i++ {
		k := keyOf(snaps[i])
		if k == curKey {
			continue
		}
		stages = append(stages, makeStage(snaps, start, i-1))
		start = i
		curKey = k
	}
	stages = append(stages, makeStage(snaps, start, len(snaps)-1))
	return stages
}

func makeStage(snaps []SoldierDebugSnapshot, start, end int) reportStage {
	first := snaps[start]
	last := snaps[end]
	moved := math.Hypot(last.X-first.X, last.Y-first.Y)
	allIdle := true
	for i := start; i <= end; i++ {
		if snaps[i].State != SoldierStateIdle {
			allIdle = false
			break
		}
	}
	return reportStage{
		startIdx:      start,
		endIdx:        end,
		startTick:     first.Tick,
		endTick:       last.Tick,
		count:         end - start + 1,
		first:         first,
		last:          last,
		movedDistance: moved,
		onlyIdle:      allIdle,
	}
}

func storyEvents(snaps []SoldierDebugSnapshot) []string {
	if len(snaps) == 0 {
		return nil
	}
	var out []string
	prev := snaps[0]
	for i := 1; i < len(snaps); i++ {
		cur := &snaps[i]
		out = appendTransitionEvents(out, &prev, cur)
		prev = *cur
	}
	if len(out) > 24 {
		out = append(out[:24], fmt.Sprintf("... (%d more events)", len(out)-24))
	}
	return out
}

func appendTransitionEvents(out []string, prev, cur *SoldierDebugSnapshot) []string {
	if cur.State != prev.State {
		out = append(out, fmt.Sprintf("T=%d state %s -> %s", cur.Tick, prev.State, cur.State))
	}
	if cur.Goal != prev.Goal {
		out = append(out, fmt.Sprintf("T=%d goal %s -> %s", cur.Tick, prev.Goal, cur.Goal))
	}
	if cur.SquadIntent != prev.SquadIntent {
		out = append(out, fmt.Sprintf("T=%d squad_intent %s -> %s", cur.Tick, prev.SquadIntent, cur.SquadIntent))
	}
	if cur.OfficerOrderKind != prev.OfficerOrderKind || cur.OfficerOrderImmediate != prev.OfficerOrderImmediate {
		out = append(out, fmt.Sprintf("T=%d order %s(imm=%t) -> %s(imm=%t)",
			cur.Tick,
			prev.OfficerOrderKind, prev.OfficerOrderImmediate,
			cur.OfficerOrderKind, cur.OfficerOrderImmediate,
		))
	}
	prevPathTerminal := prev.PathLen == 0 || prev.PathIndex >= prev.PathLen
	curPathTerminal := cur.PathLen == 0 || cur.PathIndex >= cur.PathLen
	if prevPathTerminal != curPathTerminal {
		out = append(out, fmt.Sprintf("T=%d path %s -> %s", cur.Tick, pathStateLabel(prevPathTerminal), pathStateLabel(curPathTerminal)))
	}
	if cur.RecoveryNoPathStreak != prev.RecoveryNoPathStreak {
		out = append(out, fmt.Sprintf("T=%d no_path_streak %d -> %d", cur.Tick, prev.RecoveryNoPathStreak, cur.RecoveryNoPathStreak))
	}
	if cur.MobilityStallTicks != prev.MobilityStallTicks && (cur.MobilityStallTicks == 0 || cur.MobilityStallTicks >= 10) {
		out = append(out, fmt.Sprintf("T=%d mobility_stall %d -> %d", cur.Tick, prev.MobilityStallTicks, cur.MobilityStallTicks))
	}
	return out
}

func pathStateLabel(isTerminal bool) string {
	if isTerminal {
		return "terminal"
	}
	return "active"
}

func leaderLabel(s *Soldier) string {
	if s == nil {
		return "<none>"
	}
	return s.label
}
