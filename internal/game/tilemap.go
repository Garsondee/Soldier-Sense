package game

// GroundType identifies the base surface of a tile.
type GroundType uint8

// GroundType enumerates the base surface materials for tiles.
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
	if int(g) < len(groundMovementMulLUT) {
		return groundMovementMulLUT[g]
	}
	return 1.0
}

var groundMovementMulLUT = [groundTypeCount]float64{
	GroundGrass:       1.0,
	GroundGrassLong:   0.9,
	GroundScrub:       0.8,
	GroundMud:         0.6,
	GroundSand:        0.75,
	GroundGravel:      0.85,
	GroundDirt:        0.95,
	GroundTarmac:      1.0,
	GroundPavement:    1.0,
	GroundConcrete:    1.0,
	GroundTile:        1.0,
	GroundWood:        1.0,
	GroundWater:       0.3,
	GroundRubbleLight: 0.7,
	GroundRubbleHeavy: 0.4,
	GroundCrater:      0.5,
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

// WallType represents different wall construction types with varying properties.
type WallType uint8

// WallType enumerates wall construction types.
const (
	WallTypeNone                 WallType = iota // No wall
	WallTypeResidentialExterior                  // Wood frame, drywall, siding (thin)
	WallTypeResidentialInterior                  // Drywall partition (very thin)
	WallTypeCommercialExterior                   // Concrete block, brick veneer (medium)
	WallTypeCommercialInterior                   // Steel stud, drywall (thin)
	WallTypeIndustrialExterior                   // Reinforced concrete, steel (thick)
	WallTypeIndustrialInterior                   // Steel frame, metal panels (medium)
	WallTypeMilitaryExterior                     // Reinforced concrete, armor plating (very thick)
	WallTypeMilitaryInterior                     // Steel reinforced concrete (thick)
	WallTypeAgriculturalExterior                 // Wood frame, metal siding (thin-medium)
	WallTypeAgriculturalInterior                 // Wood frame partition (thin)
)

// WallProperties defines physical characteristics of wall types.
type WallProperties struct {
	Thickness        float32 // Wall thickness in meters
	PenetrationValue float32 // Resistance to bullet penetration (0.0-1.0, higher = more resistant)
	FragmentationMod float32 // How much the wall fragments when hit (0.0-1.0, higher = more fragments)
	RicochetChance   float32 // Probability of bullets ricocheting (0.0-1.0)
	MaterialDensity  float32 // Material density for blast/explosion resistance
	VisualThickness  int     // Visual thickness in pixels for rendering
}

// ObjectType represents the type of object on a tile.
type ObjectType uint8

// ObjectType enumerates placeable objects that affect movement and cover.
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
	ObjectBed                            // Bed (2×1, blocks movement, soft cover)
	ObjectSofa                           // Sofa (2×1, blocks movement, soft cover)
	ObjectDesk                           // Desk (2×1, like table but rectangular)
	ObjectBookshelf                      // Bookshelf (1×1, blocks movement, medium cover)
	ObjectCounter                        // Kitchen counter (1×1, blocks movement, hard cover)
	ObjectRefrigerator                   // Refrigerator (1×1, blocks movement, hard cover)
	ObjectPallet                         // Wooden pallet (2×2, blocks movement, soft cover)
	ObjectMachinery                      // Industrial machinery (2×2, blocks movement, hard cover)
	ObjectWorkbench                      // Workbench (3×1, blocks movement, medium cover)
	ObjectShelving                       // Metal shelving unit (1×1, blocks movement, medium cover)
	ObjectBarrel                         // Industrial barrel (1×1, blocks movement, medium cover)
	objectTypeCount                      // sentinel
)

// objectBlocksMovement returns true if the object is impassable.
func objectBlocksMovement(o ObjectType) bool {
	switch o {
	case ObjectWall, ObjectWallDamaged, ObjectWindow, ObjectDoor,
		ObjectPillar, ObjectCrate, ObjectTallWall, ObjectTreeTrunk,
		ObjectATBarrier, ObjectVehicleWreck, ObjectBed, ObjectSofa,
		ObjectDesk, ObjectBookshelf, ObjectCounter, ObjectRefrigerator,
		ObjectPallet, ObjectMachinery, ObjectWorkbench, ObjectShelving,
		ObjectBarrel:
		return true
	default:
		return false
	}
}

// objectBlocksLOS returns true if the object fully blocks line of sight.
func objectBlocksLOS(o ObjectType) bool { //nolint:unused
	switch o {
	case ObjectWall, ObjectDoor, ObjectPillar, ObjectCrate,
		ObjectTallWall, ObjectTreeTrunk, ObjectVehicleWreck,
		ObjectBookshelf, ObjectCounter, ObjectRefrigerator,
		ObjectMachinery, ObjectWorkbench, ObjectShelving:
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
		ObjectTallWall, ObjectTreeTrunk, ObjectVehicleWreck,
		ObjectBookshelf, ObjectCounter, ObjectRefrigerator,
		ObjectMachinery, ObjectWorkbench, ObjectShelving:
		return 1.0
	case ObjectWallDamaged:
		return 0.7
	case ObjectBed, ObjectSofa, ObjectDesk: // Soft cover, partial opacity
		return 0.6
	case ObjectHedgerow:
		return 0.5
	case ObjectPallet, ObjectBarrel: // Medium cover
		return 0.4
	case ObjectBush, ObjectTreeCanopy:
		return 0.3
	case ObjectFence:
		return 0.1
	default:
		return 0.0
	}
}

// objectCoverValue returns the cover defense fraction for the object.
func objectCoverValue(o ObjectType) float64 {
	if int(o) < len(objectCoverValueLUT) {
		return objectCoverValueLUT[o]
	}
	return 0.0
}

var objectCoverValueLUT = [objectTypeCount]float64{
	ObjectNone:         0.0,
	ObjectWall:         0.90,
	ObjectWallDamaged:  0.70,
	ObjectWindow:       0.20,
	ObjectWindowBroken: 0.05,
	ObjectDoor:         0.60,
	ObjectDoorOpen:     0.0,
	ObjectDoorBroken:   0.0,
	ObjectPillar:       0.85,
	ObjectTable:        0.30,
	ObjectChair:        0.15,
	ObjectCrate:        0.65,
	ObjectSandbag:      0.70,
	ObjectChestWall:    0.70,
	ObjectTallWall:     0.85,
	ObjectHedgerow:     0.40,
	ObjectBush:         0.25,
	ObjectTreeTrunk:    0.80,
	ObjectTreeCanopy:   0.0,
	ObjectRubblePile:   0.55,
	ObjectWire:         0.0,
	ObjectATBarrier:    0.50,
	ObjectSlitTrench:   0.75,
	ObjectVehicleWreck: 0.80,
	ObjectFence:        0.05,
	ObjectBed:          0.0,
	ObjectSofa:         0.0,
	ObjectDesk:         0.0,
	ObjectBookshelf:    0.0,
	ObjectCounter:      0.0,
	ObjectRefrigerator: 0.0,
	ObjectPallet:       0.0,
	ObjectMachinery:    0.0,
	ObjectWorkbench:    0.0,
	ObjectShelving:     0.0,
	ObjectBarrel:       0.0,
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

// TileFlags stores per-tile metadata such as indoor/road-edge/damage.
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
	Tiles []Tile // row-major: index = row*Cols + col
	Cols  int
	Rows  int
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

// CoverValue returns the total cover defense fraction at (col, row).
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

// SetObject places an object on a tile, initializing durability from defaults.
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

// groundBaseColor returns the base RGB color for a ground type.
func groundBaseColor(g GroundType) (r, gr, b uint8) {
	if int(g) >= len(groundBaseColorLUT) {
		return 30, 45, 30
	}
	c := groundBaseColorLUT[g]
	return c[0], c[1], c[2]
}

var groundBaseColorLUT = [groundTypeCount][3]uint8{
	GroundGrass:       {30, 48, 30},
	GroundGrassLong:   {34, 58, 28},
	GroundScrub:       {40, 50, 30},
	GroundMud:         {50, 40, 28},
	GroundSand:        {70, 65, 48},
	GroundGravel:      {55, 52, 48},
	GroundDirt:        {48, 42, 34},
	GroundTarmac:      {48, 46, 42},
	GroundPavement:    {62, 60, 56},
	GroundConcrete:    {38, 36, 32},
	GroundTile:        {44, 40, 36},
	GroundWood:        {52, 40, 28},
	GroundWater:       {28, 38, 55},
	GroundRubbleLight: {52, 48, 40},
	GroundRubbleHeavy: {44, 40, 34},
	GroundCrater:      {36, 32, 28},
}

// DamageTile reduces durability and transitions breakable objects when destroyed.
func (tm *TileMap) DamageTile(col, row, dmg int) {
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
