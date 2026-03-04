package game

import (
	"image/color"
	"math"
	"math/rand"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

func toUint8(v int) uint8 {
	if v < 0 {
		return 0
	}
	if v > math.MaxUint8 {
		return math.MaxUint8
	}

	return uint8(v)
}

// drawRubbleLight renders scattered small debris and dust.
func (g *Game) drawRubbleLight(screen *ebiten.Image, x0, y0, cs float32, worldX, worldY int) {
	// Use world coordinates as seed for consistent but varied appearance
	seed := int64(worldX*31 + worldY*37 + 12345)
	rng := rand.New(rand.NewSource(seed)) // #nosec G404 -- deterministic visual variation only

	// Base dust layer
	dustColor := color.RGBA{R: 45, G: 42, B: 35, A: 80}
	vector.FillRect(screen, x0, y0, cs, cs, dustColor, false)

	// Scatter small pebbles and fragments
	numPebbles := 8 + rng.Intn(6)
	for i := 0; i < numPebbles; i++ {
		px := x0 + float32(rng.Intn(int(cs-2))) + 1
		py := y0 + float32(rng.Intn(int(cs-2))) + 1
		size := 1 + rng.Intn(3)

		// Vary pebble colors - concrete grays and brick tones
		var pebbleColor color.RGBA
		if rng.Float32() < 0.6 {
			// Concrete gray
			gray := 60 + rng.Intn(40)
			pebbleColor = color.RGBA{R: toUint8(gray), G: toUint8(gray - 5), B: toUint8(gray - 10), A: 200}
		} else {
			// Brick/clay tones
			pebbleColor = color.RGBA{R: toUint8(80 + rng.Intn(30)), G: toUint8(50 + rng.Intn(25)), B: toUint8(35 + rng.Intn(20)), A: 190}
		}

		vector.FillRect(screen, px, py, float32(size), float32(size), pebbleColor, false)

		// Add highlight to some pebbles
		if rng.Float32() < 0.4 {
			highlight := color.RGBA{R: pebbleColor.R + 20, G: pebbleColor.G + 15, B: pebbleColor.B + 10, A: 150}
			vector.StrokeLine(screen, px, py, px+float32(size-1), py, 0.5, highlight, false)
		}
	}
}

// drawRubbleMedium renders moderate debris piles with mixed materials.
func (g *Game) drawRubbleMedium(screen *ebiten.Image, x0, y0, cs float32, worldX, worldY int) {
	seed := int64(worldX*41 + worldY*43 + 23456)
	rng := rand.New(rand.NewSource(seed)) // #nosec G404 -- deterministic visual variation only

	// Base shadow layer
	shadowColor := color.RGBA{R: 20, G: 18, B: 15, A: 100}
	vector.FillRect(screen, x0+1, y0+1, cs, cs, shadowColor, false)

	// Generate 3-5 medium-sized chunks in organic shapes
	numChunks := 3 + rng.Intn(3)
	for i := 0; i < numChunks; i++ {
		// Position with some overlap allowed
		centerX := x0 + float32(rng.Intn(int(cs-4))) + 2
		centerY := y0 + float32(rng.Intn(int(cs-4))) + 2

		// Irregular chunk size
		width := 4 + float32(rng.Intn(6))
		height := 3 + float32(rng.Intn(5))

		// Material type affects color
		var baseColor color.RGBA
		material := rng.Float32()
		switch {
		case material < 0.4:
			// Concrete - grays
			gray := 65 + rng.Intn(35)
			baseColor = color.RGBA{R: toUint8(gray), G: toUint8(gray - 3), B: toUint8(gray - 8), A: 220}
		case material < 0.7:
			// Brick/masonry - reds and browns
			baseColor = color.RGBA{R: toUint8(85 + rng.Intn(25)), G: toUint8(55 + rng.Intn(20)), B: toUint8(40 + rng.Intn(15)), A: 210}
		default:
			// Mixed debris - varied earth tones
			baseColor = color.RGBA{R: toUint8(70 + rng.Intn(30)), G: toUint8(65 + rng.Intn(25)), B: toUint8(50 + rng.Intn(20)), A: 205}
		}

		// Draw irregular chunk using multiple rectangles
		vector.FillRect(screen, centerX, centerY, width, height, baseColor, false)

		// Add broken edge detail
		if rng.Float32() < 0.6 {
			edgeX := centerX + width - 1
			edgeY := centerY + 1
			edgeHeight := height - 2
			edgeColor := color.RGBA{R: baseColor.R - 15, G: baseColor.G - 15, B: baseColor.B - 15, A: baseColor.A}
			vector.StrokeLine(screen, edgeX, edgeY, edgeX, edgeY+edgeHeight, 0.8, edgeColor, false)
		}

		// Top highlight
		if rng.Float32() < 0.5 {
			highlightColor := color.RGBA{R: baseColor.R + 25, G: baseColor.G + 20, B: baseColor.B + 15, A: 180}
			vector.StrokeLine(screen, centerX, centerY, centerX+width-1, centerY, 1.0, highlightColor, false)
		}

		// Add small fragments around chunks
		if rng.Float32() < 0.7 {
			fragX := centerX + width + 1
			fragY := centerY + float32(rng.Intn(int(height)))
			fragColor := color.RGBA{R: baseColor.R - 10, G: baseColor.G - 8, B: baseColor.B - 5, A: 160}
			vector.FillRect(screen, fragX, fragY, 1, 2, fragColor, false)
		}
	}
}

// drawRubbleHeavy renders large concrete/masonry chunks in dramatic formations.
func (g *Game) drawRubbleHeavy(screen *ebiten.Image, x0, y0, cs float32, worldX, worldY int) {
	seed := int64(worldX*53 + worldY*59 + 34567)
	rng := rand.New(rand.NewSource(seed)) // #nosec G404 -- deterministic visual variation only

	// Heavy shadow for large debris
	shadowColor := color.RGBA{R: 8, G: 6, B: 4, A: 140}
	vector.FillRect(screen, x0+2, y0+2, cs, cs, shadowColor, false)

	// Generate 1-2 large primary chunks
	numLargeChunks := 1 + rng.Intn(2)
	for i := 0; i < numLargeChunks; i++ {
		// Large irregular blocks
		chunkX := x0 + float32(i*int(cs/2)) + float32(rng.Intn(3))
		chunkY := y0 + float32(rng.Intn(4))
		chunkW := cs/2 + float32(rng.Intn(int(cs/3)))
		chunkH := cs/2 + float32(rng.Intn(int(cs/4)))

		// Heavy concrete colors - darker, more substantial
		gray := 55 + rng.Intn(25)
		concreteColor := color.RGBA{R: toUint8(gray), G: toUint8(gray - 5), B: toUint8(gray - 12), A: 255}

		// Main chunk body
		vector.FillRect(screen, chunkX, chunkY, chunkW, chunkH, concreteColor, false)

		// Add rebar/reinforcement lines
		if rng.Float32() < 0.6 {
			rebarColor := color.RGBA{R: 45, G: 35, B: 25, A: 200}
			rebarY := chunkY + chunkH/3
			vector.StrokeLine(screen, chunkX+2, rebarY, chunkX+chunkW-2, rebarY, 1.0, rebarColor, false)
		}

		// Cracking detail
		if rng.Float32() < 0.8 {
			crackColor := color.RGBA{R: 30, G: 25, B: 20, A: 180}
			crackStartX := chunkX + float32(rng.Intn(int(chunkW/2)))
			crackStartY := chunkY
			crackEndX := crackStartX + float32(rng.Intn(int(chunkW/3))) - chunkW/6
			crackEndY := chunkY + chunkH
			vector.StrokeLine(screen, crackStartX, crackStartY, crackEndX, crackEndY, 0.7, crackColor, false)
		}

		// Bright top edge to show 3D form
		topColor := color.RGBA{R: concreteColor.R + 35, G: concreteColor.G + 30, B: concreteColor.B + 25, A: 220}
		vector.StrokeLine(screen, chunkX, chunkY, chunkX+chunkW, chunkY, 1.5, topColor, false)

		// Dark right edge for depth
		rightColor := color.RGBA{R: concreteColor.R - 25, G: concreteColor.G - 25, B: concreteColor.B - 25, A: 240}
		vector.StrokeLine(screen, chunkX+chunkW, chunkY, chunkX+chunkW, chunkY+chunkH, 1.0, rightColor, false)

		// Add some concrete aggregate texture
		numSpeckles := 3 + rng.Intn(4)
		for j := 0; j < numSpeckles; j++ {
			speckleX := chunkX + 1 + float32(rng.Intn(int(chunkW-2)))
			speckleY := chunkY + 1 + float32(rng.Intn(int(chunkH-2)))
			speckleColor := color.RGBA{R: concreteColor.R + 15, G: concreteColor.G + 10, B: concreteColor.B + 5, A: 160}
			vector.FillRect(screen, speckleX, speckleY, 1, 1, speckleColor, false)
		}
	}

	// Add smaller broken fragments around the main chunks
	numFragments := 4 + rng.Intn(6)
	for i := 0; i < numFragments; i++ {
		fragX := x0 + float32(rng.Intn(int(cs-2))) + 1
		fragY := y0 + float32(rng.Intn(int(cs-2))) + 1
		fragSize := 2 + rng.Intn(3)

		fragGray := 50 + rng.Intn(30)
		fragColor := color.RGBA{R: toUint8(fragGray), G: toUint8(fragGray - 3), B: toUint8(fragGray - 8), A: 200}
		vector.FillRect(screen, fragX, fragY, float32(fragSize), float32(fragSize), fragColor, false)
	}
}

// drawRubbleMetal renders twisted steel beams and machinery debris.
func (g *Game) drawRubbleMetal(screen *ebiten.Image, x0, y0, cs float32, worldX, worldY int) {
	seed := int64(worldX*61 + worldY*67 + 45678)
	rng := rand.New(rand.NewSource(seed)) // #nosec G404 -- deterministic visual variation only

	// Base metallic debris bed
	baseColor := color.RGBA{R: 35, G: 35, B: 30, A: 120}
	vector.FillRect(screen, x0, y0, cs, cs, baseColor, false)

	// Generate twisted metal beams - angular, linear elements
	numBeams := 2 + rng.Intn(3)
	for i := 0; i < numBeams; i++ {
		// Beam can be diagonal or bent
		angle := float64(rng.Intn(8)) * math.Pi / 4 // 8 directions
		startX := x0 + float32(rng.Intn(int(cs/2)))
		startY := y0 + float32(rng.Intn(int(cs/2)))
		length := cs/2 + float32(rng.Intn(int(cs/2)))

		endX := startX + float32(math.Cos(angle))*length
		endY := startY + float32(math.Sin(angle))*length

		// Clamp to cell boundaries
		if endX < x0 {
			endX = x0
		}
		if endX > x0+cs {
			endX = x0 + cs
		}
		if endY < y0 {
			endY = y0
		}
		if endY > y0+cs {
			endY = y0 + cs
		}

		// Metal beam colors - steels and rusted metal
		var beamColor color.RGBA
		beamType := rng.Float32()
		switch {
		case beamType < 0.4:
			// Clean steel - bluish gray
			beamColor = color.RGBA{R: 70, G: 75, B: 80, A: 240}
		case beamType < 0.7:
			// Rusted steel - browns and oranges
			beamColor = color.RGBA{R: toUint8(85 + rng.Intn(20)), G: toUint8(55 + rng.Intn(15)), B: toUint8(35 + rng.Intn(10)), A: 230}
		default:
			// Painted metal - varied colors with wear
			colors := []color.RGBA{
				{R: 60, G: 80, B: 95, A: 220}, // blue
				{R: 85, G: 70, B: 60, A: 220}, // brown
				{R: 75, G: 85, B: 65, A: 220}, // green
			}
			beamColor = colors[rng.Intn(len(colors))]
		}

		// Draw beam with thickness
		thickness := 1.5 + float32(rng.Intn(2))
		vector.StrokeLine(screen, startX, startY, endX, endY, thickness, beamColor, false)

		// Add metallic shine
		shineColor := color.RGBA{R: beamColor.R + 30, G: beamColor.G + 25, B: beamColor.B + 20, A: 150}
		vector.StrokeLine(screen, startX+1, startY, endX+1, endY, 0.7, shineColor, false)

		// Add connection points/bolts
		if rng.Float32() < 0.6 {
			boltX := startX + (endX-startX)/2
			boltY := startY + (endY-startY)/2
			boltColor := color.RGBA{R: 40, G: 40, B: 35, A: 200}
			vector.FillRect(screen, boltX, boltY, 2, 2, boltColor, false)
		}
	}

	// Add scattered metal fragments and sharp pieces
	numFragments := 5 + rng.Intn(8)
	for i := 0; i < numFragments; i++ {
		fragX := x0 + float32(rng.Intn(int(cs-3))) + 1
		fragY := y0 + float32(rng.Intn(int(cs-3))) + 1

		// Sharp, angular fragments
		if rng.Float32() < 0.5 {
			// Triangular sharp piece
			points := []float32{
				fragX, fragY + 3,
				fragX + 3, fragY + 3,
				fragX + 1, fragY,
			}
			fragColor := color.RGBA{R: 55, G: 60, B: 65, A: 190}

			// Draw triangle using lines
			vector.StrokeLine(screen, points[0], points[1], points[2], points[3], 0.8, fragColor, false)
			vector.StrokeLine(screen, points[2], points[3], points[4], points[5], 0.8, fragColor, false)
			vector.StrokeLine(screen, points[4], points[5], points[0], points[1], 0.8, fragColor, false)
		} else {
			// Rectangular metal plate fragment
			plateW := 2 + float32(rng.Intn(3))
			plateH := 1 + float32(rng.Intn(2))
			plateColor := color.RGBA{R: toUint8(65 + rng.Intn(15)), G: toUint8(65 + rng.Intn(15)), B: toUint8(70 + rng.Intn(15)), A: 200}
			vector.FillRect(screen, fragX, fragY, plateW, plateH, plateColor, false)

			// Add rivet detail
			if plateW > 2 && plateH > 1 {
				rivetColor := color.RGBA{R: 45, G: 45, B: 40, A: 180}
				vector.FillRect(screen, fragX+1, fragY, 1, 1, rivetColor, false)
			}
		}
	}
}

// drawRubbleWood renders splintered timber and organic debris.
func (g *Game) drawRubbleWood(screen *ebiten.Image, x0, y0, cs float32, worldX, worldY int) {
	seed := int64(worldX*71 + worldY*73 + 56789)
	rng := rand.New(rand.NewSource(seed)) // #nosec G404 -- deterministic visual variation only

	// Organic debris base - dirt and leaf litter
	baseColor := color.RGBA{R: 55, G: 50, B: 35, A: 100}
	vector.FillRect(screen, x0, y0, cs, cs, baseColor, false)

	// Generate splintered wood planks and beams
	numPlanks := 3 + rng.Intn(4)
	for i := 0; i < numPlanks; i++ {
		// Wood pieces can be at various angles
		angle := float64(rng.Intn(12)) * math.Pi / 6 // More varied angles than metal
		startX := x0 + float32(rng.Intn(int(cs/2)))
		startY := y0 + float32(rng.Intn(int(cs/2)))
		length := cs/3 + float32(rng.Intn(int(cs/2)))

		endX := startX + float32(math.Cos(angle))*length
		endY := startY + float32(math.Sin(angle))*length

		// Clamp to boundaries
		if endX < x0 {
			endX = x0
		}
		if endX > x0+cs {
			endX = x0 + cs
		}
		if endY < y0 {
			endY = y0
		}
		if endY > y0+cs {
			endY = y0 + cs
		}

		// Wood colors - various timber shades
		var woodColor color.RGBA
		woodType := rng.Float32()
		switch {
		case woodType < 0.4:
			// Light wood - pine, birch
			woodColor = color.RGBA{R: toUint8(120 + rng.Intn(20)), G: toUint8(100 + rng.Intn(15)), B: toUint8(70 + rng.Intn(15)), A: 210}
		case woodType < 0.7:
			// Dark wood - oak, mahogany
			woodColor = color.RGBA{R: toUint8(85 + rng.Intn(15)), G: toUint8(65 + rng.Intn(12)), B: toUint8(45 + rng.Intn(10)), A: 220}
		default:
			// Weathered/charred wood
			woodColor = color.RGBA{R: toUint8(60 + rng.Intn(20)), G: toUint8(55 + rng.Intn(15)), B: toUint8(40 + rng.Intn(10)), A: 200}
		}

		// Draw plank with wood grain
		thickness := 2.0 + float32(rng.Intn(2))
		vector.StrokeLine(screen, startX, startY, endX, endY, thickness, woodColor, false)

		// Add wood grain lines
		if rng.Float32() < 0.7 {
			grainColor := color.RGBA{R: woodColor.R - 15, G: woodColor.G - 10, B: woodColor.B - 8, A: 120}
			grainOffset := 0.5 + float32(rng.Intn(2))
			vector.StrokeLine(screen, startX, startY+grainOffset, endX, endY+grainOffset, 0.5, grainColor, false)
		}

		// Splintered ends
		if rng.Float32() < 0.5 {
			splinterColor := color.RGBA{R: woodColor.R + 15, G: woodColor.G + 10, B: woodColor.B + 5, A: 160}
			splinterLen := 2 + float32(rng.Intn(3))
			splinterAngle := angle + (float64(rng.Intn(60))-30)*math.Pi/180 // +/- 30 degrees

			splinterEndX := endX + float32(math.Cos(splinterAngle))*splinterLen
			splinterEndY := endY + float32(math.Sin(splinterAngle))*splinterLen
			vector.StrokeLine(screen, endX, endY, splinterEndX, splinterEndY, 0.8, splinterColor, false)
		}
	}

	// Add wood chips, sawdust, and organic debris
	numChips := 8 + rng.Intn(10)
	for i := 0; i < numChips; i++ {
		chipX := x0 + float32(rng.Intn(int(cs-2))) + 1
		chipY := y0 + float32(rng.Intn(int(cs-2))) + 1

		if rng.Float32() < 0.6 {
			// Wood chip - small irregular piece
			chipSize := 1 + rng.Intn(2)
			chipColor := color.RGBA{
				R: toUint8(90 + rng.Intn(30)),
				G: toUint8(80 + rng.Intn(20)),
				B: toUint8(55 + rng.Intn(15)),
				A: 180,
			}
			vector.FillRect(screen, chipX, chipY, float32(chipSize), float32(chipSize), chipColor, false)
		} else {
			// Leaf/organic matter
			leafColor := color.RGBA{
				R: toUint8(65 + rng.Intn(20)),
				G: toUint8(70 + rng.Intn(25)),
				B: toUint8(35 + rng.Intn(15)),
				A: 150,
			}
			vector.FillRect(screen, chipX, chipY, 2, 1, leafColor, false)
		}
	}

	// Add some furniture fragments if it's building debris
	if rng.Float32() < 0.3 {
		// Chair leg, table fragment, etc.
		furnX := x0 + float32(rng.Intn(int(cs-4))) + 2
		furnY := y0 + float32(rng.Intn(int(cs-6))) + 2
		furnColor := color.RGBA{R: 100, G: 85, B: 65, A: 200}

		// Simple L-shaped furniture fragment
		vector.FillRect(screen, furnX, furnY, 4, 2, furnColor, false)
		vector.FillRect(screen, furnX, furnY+2, 2, 3, furnColor, false)

		// Add furniture detail - screw or joint
		jointColor := color.RGBA{R: 50, G: 45, B: 35, A: 180}
		vector.FillRect(screen, furnX+1, furnY+1, 1, 1, jointColor, false)
	}
}
