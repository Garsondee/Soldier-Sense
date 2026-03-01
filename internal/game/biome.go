package game

import (
	"math"
	"math/rand"
)

// biomeConfig holds tuneable noise and vegetation parameters.
type biomeConfig struct {
	// Noise layer scales (smaller = broader features).
	VegetationScale float64
	RoughnessScale  float64
	MoistureScale   float64

	// Vegetation density thresholds (noise value 0–1).
	TreeThreshold    float64 // above this → place tree
	BushThreshold    float64 // above this → place bush
	HedgeThreshold   float64 // above this → place hedgerow run
	LongGrassThresh  float64 // above this → long grass ground
	ScrubThreshold   float64 // above this → scrub ground

	// Roughness thresholds.
	GravelThreshold float64
	DirtThreshold   float64
	MudThreshold    float64
}

var defaultBiomeConfig = biomeConfig{
	VegetationScale: 0.04,
	RoughnessScale:  0.06,
	MoistureScale:   0.03,

	TreeThreshold:   0.72,
	BushThreshold:   0.62,
	HedgeThreshold:  0.78,
	LongGrassThresh: 0.45,
	ScrubThreshold:  0.55,

	GravelThreshold: 0.65,
	DirtThreshold:   0.50,
	MudThreshold:    0.70,
}

// generateBiome applies noise-driven ground variation and vegetation to the TileMap.
// Runs after roads and buildings are stamped, so it skips those tiles.
func generateBiome(tm *TileMap, rng *rand.Rand, cfg biomeConfig) {
	// Generate three independent noise seeds.
	vegSeed := rng.Int63()
	roughSeed := rng.Int63()
	moistSeed := rng.Int63()

	// Pre-compute noise for each tile.
	for row := 0; row < tm.Rows; row++ {
		for col := 0; col < tm.Cols; col++ {
			t := tm.At(col, row)
			if t == nil {
				continue
			}

			// Skip tiles already claimed by roads or buildings.
			if t.Flags&TileFlagIndoor != 0 {
				continue
			}
			if t.Ground == GroundTarmac || t.Ground == GroundPavement {
				continue
			}
			if t.Ground == GroundConcrete || t.Ground == GroundTile || t.Ground == GroundWood {
				continue
			}
			if t.Object != ObjectNone {
				continue
			}

			fx := float64(col) * cfg.VegetationScale
			fy := float64(row) * cfg.VegetationScale
			vegNoise := valueNoise2D(fx, fy, vegSeed)

			rx := float64(col) * cfg.RoughnessScale
			ry := float64(row) * cfg.RoughnessScale
			roughNoise := valueNoise2D(rx, ry, roughSeed)

			mx := float64(col) * cfg.MoistureScale
			my := float64(row) * cfg.MoistureScale
			moistNoise := valueNoise2D(mx, my, moistSeed)

			// --- Ground variation ---
			if moistNoise > cfg.MudThreshold && roughNoise < 0.4 {
				t.Ground = GroundMud
			} else if roughNoise > cfg.GravelThreshold {
				t.Ground = GroundGravel
			} else if roughNoise > cfg.DirtThreshold && vegNoise < 0.3 {
				t.Ground = GroundDirt
			} else if vegNoise > cfg.ScrubThreshold && vegNoise < cfg.BushThreshold {
				t.Ground = GroundScrub
			} else if vegNoise > cfg.LongGrassThresh {
				t.Ground = GroundGrassLong
			}
			// else stays GroundGrass (default)
		}
	}

	// --- Vegetation objects (second pass to avoid overwriting ground pass) ---
	for row := 0; row < tm.Rows; row++ {
		for col := 0; col < tm.Cols; col++ {
			t := tm.At(col, row)
			if t == nil || t.Object != ObjectNone {
				continue
			}
			if t.Flags&TileFlagIndoor != 0 {
				continue
			}
			if t.Ground == GroundTarmac || t.Ground == GroundPavement {
				continue
			}

			fx := float64(col) * cfg.VegetationScale
			fy := float64(row) * cfg.VegetationScale
			vegNoise := valueNoise2D(fx, fy, vegSeed)

			// Detail noise for sub-tile variation (prevents uniform blobs).
			detail := valueNoise2D(float64(col)*0.23, float64(row)*0.23, vegSeed+1)

			// Trees: high vegetation + detail filter.
			if vegNoise > cfg.TreeThreshold && detail > 0.5 {
				// Trees need a 3×3 clear area (trunk + canopy).
				if canPlaceTree(tm, col, row) {
					placeTree(tm, col, row)
					continue
				}
			}

			// Bushes: medium-high vegetation.
			if vegNoise > cfg.BushThreshold && detail > 0.4 && detail < 0.7 {
				tm.SetObject(col, row, ObjectBush)
				continue
			}

			// Hedgerow runs: very high vegetation in specific detail band.
			if vegNoise > cfg.HedgeThreshold && detail > 0.7 {
				placeHedgerowRun(tm, rng, col, row)
			}
		}
	}
}

// canPlaceTree checks if a tree can be placed at (col,row) — needs the cell
// and its cardinal neighbours to be clear outdoor ground.
func canPlaceTree(tm *TileMap, col, row int) bool {
	for dr := -1; dr <= 1; dr++ {
		for dc := -1; dc <= 1; dc++ {
			c, r := col+dc, row+dr
			if !tm.inBounds(c, r) {
				return false
			}
			t := &tm.Tiles[r*tm.Cols+c]
			if t.Flags&TileFlagIndoor != 0 {
				return false
			}
			if t.Ground == GroundTarmac || t.Ground == GroundPavement {
				return false
			}
			// Centre cell must be empty; neighbours must not have blocking objects.
			if dc == 0 && dr == 0 {
				if t.Object != ObjectNone {
					return false
				}
			} else {
				if t.Object == ObjectTreeTrunk || t.Object == ObjectWall || t.Object == ObjectPillar {
					return false
				}
			}
		}
	}
	return true
}

// placeTree stamps a tree trunk at (col,row) and canopy on the 8 surrounding tiles.
func placeTree(tm *TileMap, col, row int) {
	tm.SetObject(col, row, ObjectTreeTrunk)
	for dr := -1; dr <= 1; dr++ {
		for dc := -1; dc <= 1; dc++ {
			if dc == 0 && dr == 0 {
				continue
			}
			c, r := col+dc, row+dr
			if tm.inBounds(c, r) && tm.ObjectAt(c, r) == ObjectNone {
				tm.SetObject(c, r, ObjectTreeCanopy)
			}
		}
	}
}

// placeHedgerowRun places a short hedgerow line (2-5 tiles) starting from (col,row).
func placeHedgerowRun(tm *TileMap, rng *rand.Rand, col, row int) {
	horizontal := rng.Intn(2) == 0
	length := 2 + rng.Intn(4) // 2-5 tiles
	for i := 0; i < length; i++ {
		c, r := col, row
		if horizontal {
			c = col + i
		} else {
			r = row + i
		}
		if !tm.inBounds(c, r) {
			break
		}
		t := &tm.Tiles[r*tm.Cols+c]
		if t.Object != ObjectNone || t.Flags&TileFlagIndoor != 0 {
			break
		}
		if t.Ground == GroundTarmac || t.Ground == GroundPavement {
			break
		}
		tm.SetObject(c, r, ObjectHedgerow)
	}
}

// --- Value noise implementation (no external deps) ---

// valueNoise2D returns a smooth noise value in [0,1] for the given coordinates.
// Uses lattice-based value noise with hermite interpolation.
func valueNoise2D(x, y float64, seed int64) float64 {
	xi := int(math.Floor(x))
	yi := int(math.Floor(y))
	xf := x - float64(xi)
	yf := y - float64(yi)

	// Hermite smoothstep.
	u := xf * xf * (3 - 2*xf)
	v := yf * yf * (3 - 2*yf)

	n00 := latticeValue(xi, yi, seed)
	n10 := latticeValue(xi+1, yi, seed)
	n01 := latticeValue(xi, yi+1, seed)
	n11 := latticeValue(xi+1, yi+1, seed)

	nx0 := n00*(1-u) + n10*u
	nx1 := n01*(1-u) + n11*u
	return nx0*(1-v) + nx1*v
}

// latticeValue returns a pseudo-random value in [0,1] for integer coordinates.
func latticeValue(x, y int, seed int64) float64 {
	// Hash combine x, y, seed into a deterministic value.
	h := uint64(seed)
	h ^= uint64(x) * 0x517cc1b727220a95
	h ^= uint64(y) * 0x6c62272e07bb0142
	h = h*0x2545f4914f6cdd1d + 0x14057b7ef767814f
	h ^= h >> 16
	h *= 0xd6e8feb86659fd93
	h ^= h >> 16
	return float64(h&0xFFFFFFFF) / float64(0xFFFFFFFF)
}
