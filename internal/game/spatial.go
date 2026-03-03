package game

import "math"

// SpatialHash provides O(1) spatial queries for nearby soldiers.
// Uses a grid-based hash to bucket soldiers by position.
type SpatialHash struct {
	cellSize float64
	buckets  map[int64][]*Soldier
}

// NewSpatialHash creates a spatial hash with the given cell size.
// cellSize should be roughly the maximum query radius for best performance.
func NewSpatialHash(cellSize float64) *SpatialHash {
	return &SpatialHash{
		cellSize: cellSize,
		buckets:  make(map[int64][]*Soldier),
	}
}

// Clear removes all soldiers from the hash.
func (sh *SpatialHash) Clear() {
	for k := range sh.buckets {
		delete(sh.buckets, k)
	}
}

// Insert adds a soldier to the spatial hash at their current position.
func (sh *SpatialHash) Insert(s *Soldier) {
	key := sh.cellKey(s.x, s.y)
	sh.buckets[key] = append(sh.buckets[key], s)
}

// QueryRadius returns all soldiers within radius of (x, y).
// This checks a square region, so some results may be slightly outside radius.
func (sh *SpatialHash) QueryRadius(x, y, radius float64) []*Soldier {
	var result []*Soldier

	// Calculate cell range to check
	minCX := int(math.Floor((x - radius) / sh.cellSize))
	maxCX := int(math.Floor((x + radius) / sh.cellSize))
	minCY := int(math.Floor((y - radius) / sh.cellSize))
	maxCY := int(math.Floor((y + radius) / sh.cellSize))

	radiusSq := radius * radius

	// Check all cells in range
	for cy := minCY; cy <= maxCY; cy++ {
		for cx := minCX; cx <= maxCX; cx++ {
			key := sh.cellKeyFromCoords(cx, cy)
			soldiers := sh.buckets[key]

			// Filter by actual distance
			for _, s := range soldiers {
				dx := s.x - x
				dy := s.y - y
				if dx*dx+dy*dy <= radiusSq {
					result = append(result, s)
				}
			}
		}
	}

	return result
}

// QueryRadiusNoFilter returns all soldiers in cells overlapping the radius.
// Does NOT filter by exact distance - faster but less precise.
func (sh *SpatialHash) QueryRadiusNoFilter(x, y, radius float64) []*Soldier {
	var result []*Soldier

	minCX := int(math.Floor((x - radius) / sh.cellSize))
	maxCX := int(math.Floor((x + radius) / sh.cellSize))
	minCY := int(math.Floor((y - radius) / sh.cellSize))
	maxCY := int(math.Floor((y + radius) / sh.cellSize))

	for cy := minCY; cy <= maxCY; cy++ {
		for cx := minCX; cx <= maxCX; cx++ {
			key := sh.cellKeyFromCoords(cx, cy)
			result = append(result, sh.buckets[key]...)
		}
	}

	return result
}

// cellKey computes the hash key for a world position.
func (sh *SpatialHash) cellKey(x, y float64) int64 {
	cx := int(math.Floor(x / sh.cellSize))
	cy := int(math.Floor(y / sh.cellSize))
	return sh.cellKeyFromCoords(cx, cy)
}

// cellKeyFromCoords computes the hash key from cell coordinates.
func (sh *SpatialHash) cellKeyFromCoords(cx, cy int) int64 {
	// Simple pairing function for 2D -> 1D mapping
	return int64(cy)<<32 | (int64(cx) & 0xffffffff)
}
