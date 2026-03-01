package game

// GroundType identifies the base surface of a tile.
type GroundType uint8

const (
	GroundGrass       GroundType = iota // Default open ground
	GroundGrassLong                     // Tall grass, minor concealment
	GroundScrub                         // Low bushes / bramble
	GroundMud                           // Wet / churned ground
	GroundSand                          // Sandy / arid patches
	GroundGravel                        // Loose stone, noisy
	GroundDirt                          // Packed earth path
	GroundTarmac                        // Road surface
	GroundPavement                      // Sidewalk / paved area
	GroundConcrete                      // Building interior floor
	GroundTile                          // Interior tile floor
	GroundWood                          // Interior wood floor
	GroundWater                         // Shallow puddle/stream
	GroundRubbleLight                   // Scattered small debris
	GroundRubbleHeavy                   // Dense rubble field
	GroundCrater                        // Shell crater
	groundTypeCount                     // sentinel
)

// groundMovementMul returns the movement speed multiplier for a ground type.
func groundMovementMul(g GroundType) float64 {
	switch g {
	case GroundGrass:
		return 1.0
	case GroundGrassLong:
		return 0.9
	case GroundScrub:
		return 0.8
	case GroundMud:
		return 0.6
	case GroundSand:
		return 0.75
	case GroundGravel:
		return 0.85
	case GroundDirt:
		return 0.95
	case GroundTarmac:
		return 1.0
	case GroundPavement:
		return 1.0
	case GroundConcrete:
		return 1.0
	case GroundTile:
		return 1.0
	case GroundWood:
		return 1.0
	case GroundWater:
		return 0.3
	case GroundRubbleLight:
		return 0.7
	case GroundRubbleHeavy:
		return 0.4
	case GroundCrater:
		return 0.5
	default:
		return 1.0
	}
}

// groundCoverValue returns the innate cover provided by ground type alone.
func groundCoverValue(g GroundType) float64 {
	switch g {
	case GroundGrassLong:
		return 0.05
	case GroundScrub:
		return 0.08
	case GroundRubbleLight:
		return 0.10
	case GroundRubbleHeavy:
		return 0.25
	case GroundCrater:
		return 0.15
	default:
		return 0.0
	}
}

// ObjectType identifies an object sitting on a tile.
type ObjectType uint8

const (
	ObjectNone         ObjectType = iota // Empty cell
	ObjectWall                           // Structural wall
	ObjectWallDamaged                    // Pre-damaged wall (holes)
	ObjectWindow                         // Intact window — see through, can't pass
	ObjectWindowBroken                   // Broken window — passable, minor snag
	ObjectDoor                           // Closed door — blocks move + LOS
	ObjectDoorOpen                       // Open door — passable
	ObjectDoorBroken                     // Destroyed door frame
	ObjectPillar                         // Structural column
	ObjectTable                          // Furniture — can be shot through
	ObjectChair                          // Light furniture
	ObjectCrate                          // Wooden crate — good cover
	ObjectSandbag                        // Chest-high sandbag wall
	ObjectChestWall                      // Low masonry wall
	ObjectTallWall                       // Freestanding tall wall
	ObjectHedgerow                       // Thick hedge
	ObjectBush                           // Decorative bush
	ObjectTreeTrunk                      // Tree base (1x1 cell)
	ObjectTreeCanopy                     // Overhead foliage
	ObjectRubblePile                     // Heaped debris
	ObjectWire                           // Barbed wire entanglement
	ObjectATBarrier                      // Concrete anti-tank block
	ObjectSlitTrench                     // Dug-in position
	ObjectVehicleWreck                   // Burnt-out vehicle hull
	ObjectFence                          // Chain-link or wooden fence
	objectTypeCount                      // sentinel
)

// objectBlocksMovement returns true if the object is impassable.
func objectBlocksMovement(o ObjectType) bool {
	switch o {
	case ObjectWall, ObjectWallDamaged, ObjectWindow, ObjectDoor,
		ObjectPillar, ObjectCrate, ObjectTallWall, ObjectTreeTrunk,
		ObjectATBarrier, ObjectVehicleWreck:
		return true
	default:
		return false
	}
}

// objectBlocksLOS returns true if the object fully blocks line of sight.
func objectBlocksLOS(o ObjectType) bool { //nolint:unused
	switch o {
	case ObjectWall, ObjectDoor, ObjectPillar, ObjectCrate,
		ObjectTallWall, ObjectTreeTrunk, ObjectVehicleWreck:
		return true
	default:
		return false
	}
}

// objectLOSOpacity returns a 0–1 opacity value for LOS ray-marching.
// 1.0 = fully opaque, 0.0 = transparent.
func objectLOSOpacity(o ObjectType) float64 {
	switch o {
	case ObjectWall, ObjectDoor, ObjectPillar, ObjectCrate,
		ObjectTallWall, ObjectTreeTrunk, ObjectVehicleWreck:
		return 1.0
	case ObjectWallDamaged:
		return 0.7
	case ObjectHedgerow:
		return 0.5
	case ObjectBush, ObjectTreeCanopy:
		return 0.3
	case ObjectFence:
		return 0.1
	default:
		return 0.0
	}
}

// objectCoverValue returns the cover defence fraction for the object.
func objectCoverValue(o ObjectType) float64 {
	switch o {
	case ObjectWall:
		return 0.90
	case ObjectWallDamaged:
		return 0.70
	case ObjectWindow:
		return 0.20
	case ObjectWindowBroken:
		return 0.05
	case ObjectDoor:
		return 0.60
	case ObjectPillar:
		return 0.85
	case ObjectTable:
		return 0.30
	case ObjectChair:
		return 0.15
	case ObjectCrate:
		return 0.65
	case ObjectSandbag:
		return 0.70
	case ObjectChestWall:
		return 0.70
	case ObjectTallWall:
		return 0.85
	case ObjectHedgerow:
		return 0.40
	case ObjectBush:
		return 0.25
	case ObjectTreeTrunk:
		return 0.80
	case ObjectRubblePile:
		return 0.55
	case ObjectATBarrier:
		return 0.50
	case ObjectSlitTrench:
		return 0.75
	case ObjectVehicleWreck:
		return 0.80
	case ObjectFence:
		return 0.05
	default:
		return 0.0
	}
}

// objectMovementMul returns the speed multiplier for objects that slow but don't block.
// Only meaningful when objectBlocksMovement returns false.
func objectMovementMul(o ObjectType) float64 {
	switch o {
	case ObjectTable:
		return 0.5
	case ObjectChair:
		return 0.7
	case ObjectSandbag:
		return 0.6
	case ObjectChestWall:
		return 0.6
	case ObjectHedgerow:
		return 0.4
	case ObjectBush:
		return 0.7
	case ObjectRubblePile:
		return 0.5
	case ObjectWire:
		return 0.2
	case ObjectFence:
		return 0.5
	case ObjectWindowBroken:
		return 0.8
	default:
		return 1.0
	}
}

// objectDefaultDurability returns the starting hit points for breakable objects.
// 0 means unbreakable.
func objectDefaultDurability(o ObjectType) int16 {
	switch o {
	case ObjectWindow:
		return 30
	case ObjectDoor:
		return 40
	case ObjectTable:
		return 20
	case ObjectChair:
		return 10
	case ObjectCrate:
		return 50
	case ObjectSandbag:
		return 80
	case ObjectHedgerow:
		return 60
	case ObjectBush:
		return 15
	case ObjectWire:
		return 25
	case ObjectFence:
		return 15
	default:
		return 0
	}
}

// TileFlags is a bitfield for per-tile metadata.
type TileFlags uint8

const (
	TileFlagIndoor   TileFlags = 1 << iota // inside a building footprint
	TileFlagRoadEdge                       // pavement / kerb bordering a road
	TileFlagDamaged                        // ground damaged by explosion
	TileFlagTrench                         // part of a trench system
	TileFlagRoof                           // has overhead cover
)

// Tile represents one cell of the battlefield.
type Tile struct {
	Ground     GroundType // base surface
	Object     ObjectType // furniture / obstacle / fortification (ObjectNone if empty)
	Flags      TileFlags  // bitfield: indoor, road-edge, damaged, etc.
	Elevation  int8       // relative height (0 = ground, -1 = trench, +1 = raised)
	Durability int16      // hit points for breakable objects
}

// TileMap is the authoritative per-cell terrain representation.
type TileMap struct {
	Cols  int
	Rows  int
	Tiles []Tile // row-major: index = row*Cols + col
}

// NewTileMap creates a tile map with default grass ground.
func NewTileMap(cols, rows int) *TileMap {
	tiles := make([]Tile, cols*rows)
	// Default: all grass, no objects.
	for i := range tiles {
		tiles[i].Ground = GroundGrass
	}
	return &TileMap{Cols: cols, Rows: rows, Tiles: tiles}
}

// inBounds returns true if (col, row) is within the tile map.
func (tm *TileMap) inBounds(col, row int) bool {
	return col >= 0 && col < tm.Cols && row >= 0 && row < tm.Rows
}

// At returns a pointer to the tile at (col, row), or nil if out of bounds.
func (tm *TileMap) At(col, row int) *Tile {
	if !tm.inBounds(col, row) {
		return nil
	}
	return &tm.Tiles[row*tm.Cols+col]
}

// Ground returns the ground type at (col, row).
func (tm *TileMap) Ground(col, row int) GroundType {
	if !tm.inBounds(col, row) {
		return GroundGrass
	}
	return tm.Tiles[row*tm.Cols+col].Ground
}

// ObjectAt returns the object type at (col, row).
func (tm *TileMap) ObjectAt(col, row int) ObjectType {
	if !tm.inBounds(col, row) {
		return ObjectNone
	}
	return tm.Tiles[row*tm.Cols+col].Object
}

// IsPassable returns true if a soldier can walk through (col, row).
func (tm *TileMap) IsPassable(col, row int) bool {
	if !tm.inBounds(col, row) {
		return false
	}
	t := &tm.Tiles[row*tm.Cols+col]
	return !objectBlocksMovement(t.Object)
}

// MovementCost returns the movement cost multiplier at (col, row).
// Lower = faster. 0 means impassable (callers should check IsPassable first).
func (tm *TileMap) MovementCost(col, row int) float64 {
	if !tm.inBounds(col, row) {
		return 0
	}
	t := &tm.Tiles[row*tm.Cols+col]
	if objectBlocksMovement(t.Object) {
		return 0
	}
	gMul := groundMovementMul(t.Ground)
	oMul := objectMovementMul(t.Object)
	cost := gMul * oMul
	if cost < 0.1 {
		cost = 0.1
	}
	return cost
}

// LOSOpacity returns the opacity of this tile for line-of-sight checks.
// 0 = transparent, 1 = opaque.
func (tm *TileMap) LOSOpacity(col, row int) float64 {
	if !tm.inBounds(col, row) {
		return 0
	}
	return objectLOSOpacity(tm.Tiles[row*tm.Cols+col].Object)
}

// CoverValue returns the total cover defence fraction at (col, row).
func (tm *TileMap) CoverValue(col, row int) float64 {
	if !tm.inBounds(col, row) {
		return 0
	}
	t := &tm.Tiles[row*tm.Cols+col]
	gc := groundCoverValue(t.Ground)
	oc := objectCoverValue(t.Object)
	total := gc + oc
	if total > 0.90 {
		total = 0.90
	}
	return total
}

// IsIndoor returns true if the tile is flagged as indoor.
func (tm *TileMap) IsIndoor(col, row int) bool {
	if !tm.inBounds(col, row) {
		return false
	}
	return tm.Tiles[row*tm.Cols+col].Flags&TileFlagIndoor != 0
}

// SetGround sets the ground type for a tile.
func (tm *TileMap) SetGround(col, row int, g GroundType) {
	if !tm.inBounds(col, row) {
		return
	}
	tm.Tiles[row*tm.Cols+col].Ground = g
}

// SetObject places an object on a tile, initialising durability from defaults.
func (tm *TileMap) SetObject(col, row int, o ObjectType) {
	if !tm.inBounds(col, row) {
		return
	}
	t := &tm.Tiles[row*tm.Cols+col]
	t.Object = o
	t.Durability = objectDefaultDurability(o)
}

// AddFlag sets flag bits on a tile.
func (tm *TileMap) AddFlag(col, row int, f TileFlags) {
	if !tm.inBounds(col, row) {
		return
	}
	tm.Tiles[row*tm.Cols+col].Flags |= f
}

// groundBaseColour returns the base RGB colour for a ground type.
func groundBaseColour(g GroundType) (r, gr, b uint8) {
	switch g {
	case GroundGrass:
		return 30, 48, 30
	case GroundGrassLong:
		return 34, 58, 28
	case GroundScrub:
		return 40, 50, 30
	case GroundMud:
		return 50, 40, 28
	case GroundSand:
		return 70, 65, 48
	case GroundGravel:
		return 55, 52, 48
	case GroundDirt:
		return 48, 42, 34
	case GroundTarmac:
		return 48, 46, 42
	case GroundPavement:
		return 62, 60, 56
	case GroundConcrete:
		return 38, 36, 32
	case GroundTile:
		return 44, 40, 36
	case GroundWood:
		return 52, 40, 28
	case GroundWater:
		return 28, 38, 55
	case GroundRubbleLight:
		return 52, 48, 40
	case GroundRubbleHeavy:
		return 44, 40, 34
	case GroundCrater:
		return 36, 32, 28
	default:
		return 30, 45, 30
	}
}

// DamageTile reduces durability and transitions breakable objects when destroyed.
func (tm *TileMap) DamageTile(col, row int, dmg int) {
	if !tm.inBounds(col, row) {
		return
	}
	t := &tm.Tiles[row*tm.Cols+col]
	if t.Durability <= 0 {
		return // already broken or unbreakable
	}
	t.Durability -= int16(dmg) // #nosec G115 -- dmg is always a small game damage value, well within int16 range
	if t.Durability <= 0 {
		t.Durability = 0
		switch t.Object {
		case ObjectWindow:
			t.Object = ObjectWindowBroken
		case ObjectDoor:
			t.Object = ObjectDoorBroken
		case ObjectTable:
			t.Object = ObjectNone
			t.Ground = GroundRubbleLight
		case ObjectChair:
			t.Object = ObjectNone
		case ObjectCrate:
			t.Object = ObjectNone
			t.Ground = GroundRubbleLight
		case ObjectSandbag:
			t.Object = ObjectRubblePile
		case ObjectBush:
			t.Object = ObjectNone
		case ObjectHedgerow:
			t.Object = ObjectNone
			t.Ground = GroundScrub
		case ObjectWire:
			t.Object = ObjectNone
		case ObjectFence:
			t.Object = ObjectNone
			t.Ground = GroundRubbleLight
		}
		t.Flags |= TileFlagDamaged
	}
}
