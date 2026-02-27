package game

import "math"

// IntelMapKind identifies a specific heat layer within an IntelMap.
type IntelMapKind int

const (
	IntelContact          IntelMapKind = iota // active enemy visual contact this tick
	IntelRecentContact                        // enemy seen within recent window (from blackboard)
	IntelThreatDensity                        // accumulated contact heat — persistent hot zones
	IntelFriendlyPresence                     // where friendlies currently are
	IntelDangerZone                           // cells where friendlies are receiving fire
	IntelUnexplored                           // cells no friendly has ever seen (cleared on sight)
	intelMapCount                             // sentinel — total layer count
)

// IntelMapKindName returns a short display name for a layer.
func IntelMapKindName(k IntelMapKind) string {
	switch k {
	case IntelContact:
		return "Contact"
	case IntelRecentContact:
		return "RecentContact"
	case IntelThreatDensity:
		return "ThreatDensity"
	case IntelFriendlyPresence:
		return "Friendly"
	case IntelDangerZone:
		return "DangerZone"
	case IntelUnexplored:
		return "Unexplored"
	default:
		return "Unknown"
	}
}

// heatDecayRates are the per-tick decay rates for each layer (subtracted each tick).
// At 60 TPS:
//
//	IntelContact          0.05  → ~1.2s to zero from 1.0
//	IntelRecentContact    0.003 → ~30s
//	IntelThreatDensity    0.0005→ ~3.5 min
//	IntelFriendlyPresence 0.10  → ~0.7s
//	IntelDangerZone       0.002 → ~45s
//	IntelUnexplored       0.0   → never (cleared by sight)
var heatDecayRates = [intelMapCount]float32{
	0.05,
	0.003,
	0.0005,
	0.10,
	0.002,
	0.0,
}

const heatMaxValue float32 = 1.0

// HeatLayer is a 2-D float32 grid representing one type of spatial intelligence.
type HeatLayer struct {
	cells     []float32
	rows      int
	cols      int
	decayRate float32
}

func newHeatLayer(rows, cols int, decayRate float32) *HeatLayer {
	return &HeatLayer{
		cells:     make([]float32, rows*cols),
		rows:      rows,
		cols:      cols,
		decayRate: decayRate,
	}
}

// Add adds delta to cell (row, col), clamped to [0, heatMaxValue].
func (l *HeatLayer) Add(row, col int, delta float32) {
	if row < 0 || row >= l.rows || col < 0 || col >= l.cols {
		return
	}
	idx := row*l.cols + col
	v := l.cells[idx] + delta
	if v > heatMaxValue {
		v = heatMaxValue
	}
	if v < 0 {
		v = 0
	}
	l.cells[idx] = v
}

// Set forces a cell to exactly v, clamped to [0, heatMaxValue].
func (l *HeatLayer) Set(row, col int, v float32) {
	if row < 0 || row >= l.rows || col < 0 || col >= l.cols {
		return
	}
	if v > heatMaxValue {
		v = heatMaxValue
	}
	if v < 0 {
		v = 0
	}
	l.cells[row*l.cols+col] = v
}

// At returns the heat value at (row, col), or 0 if out of bounds.
func (l *HeatLayer) At(row, col int) float32 {
	if row < 0 || row >= l.rows || col < 0 || col >= l.cols {
		return 0
	}
	return l.cells[row*l.cols+col]
}

// SampleAt returns heat at a world-space position.
func (l *HeatLayer) SampleAt(wx, wy float64) float32 {
	col, row := WorldToCell(wx, wy)
	return l.At(row, col)
}

// SumInRadius returns the total heat within a world-space radius.
func (l *HeatLayer) SumInRadius(wx, wy, radius float64) float32 {
	col0, row0 := WorldToCell(wx-radius, wy-radius)
	col1, row1 := WorldToCell(wx+radius, wy+radius)
	if col0 < 0 {
		col0 = 0
	}
	if row0 < 0 {
		row0 = 0
	}
	if col1 >= l.cols {
		col1 = l.cols - 1
	}
	if row1 >= l.rows {
		row1 = l.rows - 1
	}

	r2 := radius * radius
	var sum float32
	for row := row0; row <= row1; row++ {
		for col := col0; col <= col1; col++ {
			cwx, cwy := CellToWorld(col, row)
			dx := cwx - wx
			dy := cwy - wy
			if dx*dx+dy*dy <= r2 {
				sum += l.cells[row*l.cols+col]
			}
		}
	}
	return sum
}

// MaxInRadius returns the peak heat value within a world-space radius.
func (l *HeatLayer) MaxInRadius(wx, wy, radius float64) float32 {
	col0, row0 := WorldToCell(wx-radius, wy-radius)
	col1, row1 := WorldToCell(wx+radius, wy+radius)
	if col0 < 0 {
		col0 = 0
	}
	if row0 < 0 {
		row0 = 0
	}
	if col1 >= l.cols {
		col1 = l.cols - 1
	}
	if row1 >= l.rows {
		row1 = l.rows - 1
	}

	r2 := radius * radius
	var best float32
	for row := row0; row <= row1; row++ {
		for col := col0; col <= col1; col++ {
			cwx, cwy := CellToWorld(col, row)
			dx := cwx - wx
			dy := cwy - wy
			if dx*dx+dy*dy <= r2 {
				if v := l.cells[row*l.cols+col]; v > best {
					best = v
				}
			}
		}
	}
	return best
}

// Centroid returns the heat-weighted centroid in world space.
// ok is false when the layer has no heat (all zeros).
func (l *HeatLayer) Centroid() (wx, wy float64, ok bool) {
	var sumW, sumX, sumY float64
	for row := 0; row < l.rows; row++ {
		for col := 0; col < l.cols; col++ {
			v := float64(l.cells[row*l.cols+col])
			if v <= 0 {
				continue
			}
			cwx, cwy := CellToWorld(col, row)
			sumW += v
			sumX += cwx * v
			sumY += cwy * v
		}
	}
	if sumW < 1e-9 {
		return 0, 0, false
	}
	return sumX / sumW, sumY / sumW, true
}

// Decay subtracts decayRate from every cell, clamping at 0.
func (l *HeatLayer) Decay() {
	if l.decayRate <= 0 {
		return
	}
	for i := range l.cells {
		v := l.cells[i] - l.decayRate
		if v < 0 {
			v = 0
		}
		l.cells[i] = v
	}
}

// Fill sets every cell to v (used for initialising IntelUnexplored).
func (l *HeatLayer) Fill(v float32) {
	if v > heatMaxValue {
		v = heatMaxValue
	}
	for i := range l.cells {
		l.cells[i] = v
	}
}

// --- IntelMap ---

// IntelMap holds all heat layers for one team.
type IntelMap struct {
	Team   Team
	layers [intelMapCount]*HeatLayer
}

func newIntelMap(team Team, rows, cols int) *IntelMap {
	m := &IntelMap{Team: team}
	for k := IntelMapKind(0); k < intelMapCount; k++ {
		m.layers[k] = newHeatLayer(rows, cols, heatDecayRates[k])
	}
	// Unexplored starts fully opaque — cleared as cells are seen.
	m.layers[IntelUnexplored].Fill(heatMaxValue)
	return m
}

// Layer returns the HeatLayer for kind k.
func (m *IntelMap) Layer(k IntelMapKind) *HeatLayer {
	return m.layers[k]
}

// Decay applies per-tick decay to all layers.
func (m *IntelMap) Decay() {
	for _, l := range m.layers {
		l.Decay()
	}
}

// WriteContact stamps enemy contact heat at world position (wx, wy).
func (m *IntelMap) WriteContact(wx, wy float64) {
	col, row := WorldToCell(wx, wy)
	m.layers[IntelContact].Add(row, col, 1.0)
}

// WriteRecentContact stamps recent-contact heat weighted by confidence.
func (m *IntelMap) WriteRecentContact(wx, wy float64, confidence float64) {
	col, row := WorldToCell(wx, wy)
	m.layers[IntelRecentContact].Add(row, col, float32(confidence*0.8))
}

// WriteFriendlyPresence marks a friendly soldier's current position.
func (m *IntelMap) WriteFriendlyPresence(wx, wy float64) {
	col, row := WorldToCell(wx, wy)
	m.layers[IntelFriendlyPresence].Set(row, col, heatMaxValue)
}

// WriteDangerZone adds danger heat at a friendly soldier's position
// when they are receiving fire, weighted by fire count.
func (m *IntelMap) WriteDangerZone(wx, wy float64, fireCounts int) {
	col, row := WorldToCell(wx, wy)
	m.layers[IntelDangerZone].Add(row, col, float32(fireCounts)*0.5)
}

// ClearSeen marks a cell as seen (clears Unexplored heat).
func (m *IntelMap) ClearSeen(wx, wy float64) {
	col, row := WorldToCell(wx, wy)
	m.layers[IntelUnexplored].Set(row, col, 0)
}

// AccumulateThreatDensity bleeds ContactHeat into ThreatDensity each tick.
func (m *IntelMap) AccumulateThreatDensity() {
	contact := m.layers[IntelContact]
	density := m.layers[IntelThreatDensity]
	for i := range contact.cells {
		if contact.cells[i] > 0 {
			v := density.cells[i] + contact.cells[i]*0.5
			if v > heatMaxValue {
				v = heatMaxValue
			}
			density.cells[i] = v
		}
	}
}

// --- IntelStore ---

// IntelStore owns all intelligence maps for all teams.
type IntelStore struct {
	maps    map[Team]*IntelMap
	rows    int
	cols    int
	mapW    int // playfield width in pixels
	mapH    int // playfield height in pixels
}

// NewIntelStore creates maps for TeamRed and TeamBlue sized to the given
// playfield pixel dimensions.
func NewIntelStore(mapW, mapH int) *IntelStore {
	cols := mapW / cellSize
	rows := mapH / cellSize
	s := &IntelStore{
		maps: make(map[Team]*IntelMap),
		rows: rows,
		cols: cols,
		mapW: mapW,
		mapH: mapH,
	}
	s.maps[TeamRed] = newIntelMap(TeamRed, rows, cols)
	s.maps[TeamBlue] = newIntelMap(TeamBlue, rows, cols)
	return s
}

// For returns the IntelMap for the given team. Returns nil for unknown teams.
func (s *IntelStore) For(team Team) *IntelMap {
	return s.maps[team]
}

// Decay applies per-tick decay to all team maps.
func (s *IntelStore) Decay() {
	for _, m := range s.maps {
		m.Decay()
	}
}

// Update is called each tick. It drives decay, accumulates derived layers,
// and performs all writes from the given soldier slices.
//
// redSoldiers  — friendly (red) agents
// blueSoldiers — OpFor (blue) agents
// buildings    — for computing visible cells in the unexplored layer
func (s *IntelStore) Update(redSoldiers, blueSoldiers []*Soldier, buildings []rect) {
	// Decay all layers first.
	s.Decay()

	// Write from both sides.
	s.writeSoldiers(redSoldiers, blueSoldiers, buildings)
	s.writeSoldiers(blueSoldiers, redSoldiers, buildings)

	// Accumulate derived ThreatDensity from contact heat.
	for _, m := range s.maps {
		m.AccumulateThreatDensity()
	}
}

// writeSoldiers writes all intel for one team's soldiers.
// soldiers = the team being written for; enemies = the opposing side.
func (s *IntelStore) writeSoldiers(soldiers, _ []*Soldier, _ []rect) {
	if len(soldiers) == 0 {
		return
	}
	team := soldiers[0].team
	m := s.maps[team]
	if m == nil {
		return
	}

	for _, sol := range soldiers {
		if sol.state == SoldierStateDead {
			continue
		}

		// Friendly presence — stamp soldier's own position.
		m.WriteFriendlyPresence(sol.x, sol.y)

		// Danger zone — soldier is being shot at.
		if sol.blackboard.IncomingFireCount > 0 {
			m.WriteDangerZone(sol.x, sol.y, sol.blackboard.IncomingFireCount)
		}

		// Contact heat — from live vision contacts.
		for _, c := range sol.vision.KnownContacts {
			m.WriteContact(c.x, c.y)

			// Also clear unexplored at the contact's cell (we can see it).
			m.ClearSeen(c.x, c.y)
		}

		// Recent contact heat — from blackboard threat facts.
		for _, t := range sol.blackboard.Threats {
			if t.Confidence > 0.15 {
				m.WriteRecentContact(t.X, t.Y, float64(t.Confidence))
			}
		}

		// Unexplored: clear cells within the soldier's approximate vision cone.
		// We sample a grid of points inside the cone and clear each one.
		s.clearVisibleCells(m, sol)
	}
}

// clearVisibleCells clears IntelUnexplored for cells visible to a soldier.
// Uses a coarse grid sample of the vision fan — accurate enough for fog-of-war.
func (s *IntelStore) clearVisibleCells(m *IntelMap, sol *Soldier) {
	v := &sol.vision
	halfFOV := v.FOV / 2.0
	sampleRange := v.MaxRange
	steps := int(sampleRange / float64(cellSize))
	if steps > 24 {
		steps = 24
	}
	angularSteps := 12

	for ai := 0; ai <= angularSteps; ai++ {
		angle := v.Heading - halfFOV + (v.FOV/float64(angularSteps))*float64(ai)
		cosA := math.Cos(angle)
		sinA := math.Sin(angle)
		for ri := 1; ri <= steps; ri++ {
			dist := float64(ri) * float64(cellSize)
			wx := sol.x + cosA*dist
			wy := sol.y + sinA*dist
			m.ClearSeen(wx, wy)
		}
	}
	// Always clear the soldier's own cell.
	m.ClearSeen(sol.x, sol.y)
}

// Rows returns the grid row count.
func (s *IntelStore) Rows() int { return s.rows }

// Cols returns the grid column count.
func (s *IntelStore) Cols() int { return s.cols }
