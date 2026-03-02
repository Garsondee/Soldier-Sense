package game

import (
	"math/rand"
)

type HeadlessBattlefield struct {
	Width  int
	Height int

	TileMap            *TileMap
	Buildings          []rect
	BuildingFootprints []rect
	Windows            []rect
	Covers             []*CoverObject

	NavGrid     *NavGrid
	TacticalMap *TacticalMap
	MapSeed     int64
}

func NewHeadlessBattlefield(mapSeed int64, battleW, battleH int) *HeadlessBattlefield {
	g := &Game{
		gameWidth:  battleW,
		gameHeight: battleH,
		mapSeed:    mapSeed,
	}

	mapRng := rand.New(rand.NewSource(mapSeed)) // #nosec G404 -- deterministic sim
	g.tileMap = NewTileMap(battleW/cellSize, battleH/cellSize)
	generateGridRoads(g.tileMap, mapRng, defaultRoadConfig)
	g.initBuildings(mapRng)

	coverRng := rand.New(rand.NewSource(mapSeed + 12345)) // #nosec G404 -- deterministic sim
	var rubble []*CoverObject
	g.covers, rubble = GenerateCover(g.gameWidth, g.gameHeight, g.buildingFootprints, g.buildings, coverRng, g.tileMap)
	g.applyBuildingDamage(rubble)

	g.initTileMap()
	generateBiome(g.tileMap, mapRng, defaultBiomeConfig)
	generateFortifications(g.tileMap, mapRng, defaultFortConfig)

	g.navGrid = NewNavGrid(g.gameWidth, g.gameHeight, g.buildings, soldierRadius, g.covers, g.windows)
	g.tacticalMap = NewTacticalMap(g.gameWidth, g.gameHeight, g.buildings, g.windows, g.buildingFootprints)
	g.buildingQualities = ComputeBuildingQualities(g.buildingFootprints, g.buildings, g.windows, g.gameWidth, g.gameHeight, g.navGrid)

	return &HeadlessBattlefield{
		Width:              battleW,
		Height:             battleH,
		TileMap:            g.tileMap,
		Buildings:          append([]rect(nil), g.buildings...),
		BuildingFootprints: append([]rect(nil), g.buildingFootprints...),
		Windows:            append([]rect(nil), g.windows...),
		Covers:             append([]*CoverObject(nil), g.covers...),
		NavGrid:            g.navGrid,
		TacticalMap:        g.tacticalMap,
		MapSeed:            mapSeed,
	}
}
