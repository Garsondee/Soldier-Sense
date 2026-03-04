# Building Generation Improvements: Tactical Complexity Brainstorm

## Executive Summary

Current building generation creates functional but simplistic structures: rectangular footprints with BSP room subdivision, evenly-spaced windows, 1-2 doors, and minimal furniture. While tactically usable, buildings lack the **architectural variety**, **interior complexity**, and **tactical richness** that would make urban combat truly engaging.

This document brainstorms comprehensive improvements to building generation, focusing on:
- **Architectural diversity** (L-shapes, courtyards, multi-wing structures)
- **Interior complexity** (more rooms, hallways, realistic layouts)
- **Window/door intelligence** (functional placement, not just spacing)
- **Furniture density** (clutter, cover, obstacles)
- **Building types** (residential, industrial, commercial, military)
- **Larger structures** (warehouses, factories, compounds)

---

## Current System Analysis

### What We Have Now

**Building Sizes** (`game.go:532-541`):
- Small: 3×3 to 4×3 units (48×48 to 64×48 pixels)
- Medium: 4×4 to 5×5 units (64×64 to 80×80 pixels)
- Large: 5×6 to 8×8 units (80×96 to 128×128 pixels)
- **Max size**: 8×8 units (128×128 pixels)

**Perimeter Features** (`game.go:585-737`):
- 1-2 exterior doors (occasionally 3)
- Windows every ~2 units along walls
- Doors placed randomly on selected faces
- Windows skip corners and door positions

**Interior Subdivision** (`game.go:758-852`):
- Recursive BSP splits (minimum 2×2 unit rooms)
- Vertical or horizontal partitions
- One doorway per partition wall
- Stops at depth or probabilistically (15% chance)
- **Result**: 2-6 rooms per building

**Furniture** (`interiors.go:38-94`):
- 30% chance: table cluster (1-2 tables, 1-3 chairs)
- 15% chance: crates along wall (1-3 crates)
- 40% chance: pillar in large rooms (≥5×5 tiles)
- Random floor type (concrete/tile/wood)

### Limitations

1. **All buildings are rectangles** — no L-shapes, T-shapes, courtyards
2. **Small maximum size** — 8×8 units (128px) is tiny for warehouses/factories
3. **Uniform room density** — BSP creates similar-sized rooms, no variety
4. **Sparse furniture** — rooms feel empty, not lived-in
5. **No hallways** — all rooms connect directly via partition walls
6. **Window placement is mechanical** — every 2 units, no functional logic
7. **No building archetypes** — house looks like warehouse looks like bunker
8. **No exterior features** — no porches, loading docks, annexes
9. **Single-story only** — no vertical dimension
10. **No interior detail** — no bathrooms, kitchens, offices, storage rooms

---

## Improvement Categories

### 1. Architectural Shapes & Footprints

#### 1.1 Non-Rectangular Footprints

**L-Shaped Buildings**:
```
┌─────┐
│     │
│     ├─────┐
│     │     │
└─────┴─────┘
```
- Two perpendicular wings
- Natural courtyard/corner formation
- Excellent for defensive positions (enfilade fire)
- **Implementation**: Generate two overlapping rectangles at 90° angle

**T-Shaped Buildings**:
```
  ┌───┐
  │   │
┌─┴───┴─┐
│       │
└───────┘
```
- Central wing + perpendicular extension
- Good for command posts, admin buildings
- **Implementation**: Three rectangles (base + two wings)

**U-Shaped Buildings** (Courtyard):
```
┌───┐ ┌───┐
│   │ │   │
│   └─┘   │
│         │
└─────────┘
```
- Three wings around open courtyard
- Excellent for compounds, barracks
- Courtyard provides protected rally point
- **Implementation**: Three rectangles around void

**Compound Structures**:
```
┌───┐   ┌───┐
│ A │   │ B │
└───┘   └───┘
    ┌───┐
    │ C │
    └───┘
```
- Multiple separate buildings within perimeter wall
- Industrial complexes, military bases
- **Implementation**: Generate cluster, add perimeter fence/wall

#### 1.2 Larger Buildings

**Current max**: 8×8 units (128×128 pixels)

**Proposed sizes**:
- **Warehouse**: 12×8 to 16×12 units (192×128 to 256×192 pixels)
- **Factory**: 14×10 to 20×14 units (224×160 to 320×224 pixels)
- **Hangar**: 16×16 to 24×16 units (256×256 to 384×256 pixels)
- **Apartment block**: 10×6 to 14×8 units (160×96 to 224×128 pixels)

**Challenges**:
- Navmesh performance (larger interiors)
- LOS raycasting (more walls)
- Memory (more rooms, furniture)

**Solutions**:
- Limit count (1-2 large buildings per map)
- Simplify interior (open floor plans for warehouses)
- Optimize spatial queries (grid acceleration)

#### 1.3 Exterior Features

**Porches/Verandas**:
- Small 1-2 unit extension from door
- Provides covered approach
- Tactical: Pre-entry staging area

**Loading Docks**:
- Raised platform on warehouse side
- Ramp or stairs
- Tactical: Height advantage, vehicle cover

**Annexes**:
- Small attached structures (sheds, garages)
- 2×2 to 3×3 units
- Connected by door or separate

**Balconies** (if multi-story):
- Overhangs on upper floors
- Firing positions with cover
- Accessible from interior

---

### 2. Interior Complexity

#### 2.1 Room Types & Layouts

**Current**: Generic rooms, no specialization

**Proposed room types**:

**Residential**:
- **Living room**: Large, central, table cluster + chairs
- **Kitchen**: Counters (crate-like objects), table, narrow
- **Bedroom**: Bed (2×1 table), dresser (crate), small
- **Bathroom**: Tiny (1×1 or 1×2 units), sink/toilet objects
- **Hallway**: Narrow (1 unit wide), connects rooms

**Industrial**:
- **Open floor**: Large (6×6+), minimal partitions, scattered crates
- **Storage room**: Dense crates, shelving
- **Office**: Desks (tables), filing cabinets (crates), chairs
- **Workshop**: Workbenches, tool racks, machinery (pillars)
- **Loading bay**: Large door, open space, pallets

**Commercial**:
- **Shop floor**: Open, counters, display shelves
- **Stockroom**: Dense storage, narrow aisles
- **Break room**: Tables, chairs, vending machines (crates)

**Military**:
- **Barracks**: Rows of beds (tables), lockers (crates)
- **Armory**: Weapon racks (wall-mounted), ammo crates
- **Command center**: Large table (map table), chairs, radios
- **Guard post**: Small, windows on all sides, minimal furniture

#### 2.2 Hallway System

**Current**: No hallways, all rooms connect directly

**Proposed**:
- **Central hallway**: 1-unit wide corridor, rooms branch off
- **L-shaped hallway**: Follows building perimeter
- **Double-loaded corridor**: Hallway with rooms on both sides

**Implementation**:
```
Instead of:
┌─────┬─────┐
│  A  │  B  │
├─────┼─────┤
│  C  │  D  │
└─────┴─────┘

Generate:
┌─────┬───┬─────┐
│  A  │ H │  B  │
├─────┤ A ├─────┤
│  C  │ L │  D  │
└─────┴─L─┴─────┘
```

**Tactical impact**:
- Hallways are **fatal funnels** (high danger)
- Rooms off hallways are **strongpoints**
- Clearing requires **slice-the-pie** tactics
- Defenders can **cover hallway** from room doorways

#### 2.3 Room Count & Density

**Current**: 2-6 rooms per building (BSP stops early)

**Proposed**:
- **Small buildings** (3×3 to 4×4): 1-2 rooms (current is fine)
- **Medium buildings** (5×5 to 6×6): 3-5 rooms (increase from 2-4)
- **Large buildings** (7×7 to 8×8): 5-8 rooms (increase from 3-6)
- **Very large buildings** (10×10+): 8-15 rooms (new category)

**Implementation tweaks**:
- Reduce BSP early-stop probability (15% → 5%)
- Increase minimum room size variety (some 1×2, not just 2×2)
- Add hallway generation pass after BSP

---

### 3. Window & Door Intelligence

#### 3.1 Functional Window Placement

**Current**: Windows every 2 units, mechanical spacing

**Proposed logic**:

**Residential**:
- **Living room**: 2-3 windows on exterior walls
- **Bedroom**: 1 window (natural light)
- **Kitchen**: 1-2 windows (ventilation)
- **Bathroom**: Small/high window or none (privacy)
- **Hallway**: No windows (interior)

**Industrial**:
- **Warehouse**: High windows (clerestory), sparse
- **Office**: Regular windows (every 2-3 units)
- **Workshop**: Ventilation windows, irregular spacing
- **Loading bay**: Large opening (garage door size)

**Military**:
- **Barracks**: Regular windows (every 2 units)
- **Guard post**: Windows on all sides (360° observation)
- **Bunker**: Firing slits (narrow windows), strategic placement

**Implementation**:
- Room type determines window count/placement
- Windows face **outward** (exterior walls only)
- Corner rooms get windows on **two faces**
- Interior rooms (hallway-accessed) get **no windows**

#### 3.2 Door Placement Logic

**Current**: 1-2 random exterior doors, one door per partition

**Proposed**:

**Exterior doors**:
- **Front door**: Main entrance, faces road/approach
- **Back door**: Rear exit, opposite side from front
- **Side door**: Service entrance (industrial/commercial)
- **Multiple doors**: Large buildings get 2-4 doors

**Interior doors**:
- **Hallway doors**: Every room off hallway has door
- **Connecting doors**: Adjacent rooms may share door (bedroom→bathroom)
- **Double doors**: Large rooms (warehouses) get wide openings

**Door types**:
- **Standard door**: 1 unit wide (current)
- **Double door**: 2 units wide (warehouses, hangars)
- **Sliding door**: Industrial (same mechanics, different visual)
- **No door**: Some interior partitions are open archways

**Tactical impact**:
- **Front door**: Obvious entry, likely defended
- **Back door**: Flanking route, less defended
- **Multiple doors**: More entry options, harder to defend
- **Chokepoints**: Single door rooms are strongpoints

#### 3.3 Window Variety

**Current**: All windows identical (ObjectWindow)

**Proposed types**:
- **Standard window**: 1 unit, blocks movement, transparent LOS
- **Large window**: 2 units wide (living rooms, shops)
- **Small/high window**: Half-height, harder to shoot through
- **Firing slit**: Narrow (0.5 unit), military structures
- **Broken window**: Pre-damaged, passable with penalty
- **Barred window**: Reinforced, extra cover, blocks entry

---

### 4. Furniture & Clutter

#### 4.1 Increased Furniture Density

**Current**: 30% table, 15% crates, 40% pillar (sparse)

**Proposed density by room type**:

**Living room** (80% furnished):
- Sofa (2×1 table)
- Coffee table (1×1)
- TV stand (1×1 crate)
- Bookshelf (1×1 crate)
- 2-4 chairs

**Kitchen** (90% furnished):
- Counter (3-4 crate-like objects along wall)
- Table (1×1 or 2×1)
- 2-4 chairs
- Refrigerator (1×1 crate)
- Stove (1×1 crate)

**Bedroom** (70% furnished):
- Bed (2×1 table)
- Nightstand (1×1 crate)
- Dresser (2×1 crate)
- Wardrobe (1×1 crate)

**Office** (85% furnished):
- Desk (2×1 table)
- Office chair (1×1)
- Filing cabinet (1×1 crate)
- Bookshelf (1×1 crate)

**Warehouse** (40% furnished):
- Scattered crate clusters (3-8 crates)
- Pallets (2×2 crate groups)
- Machinery (pillars)
- Shelving units (crate rows)

**Workshop** (60% furnished):
- Workbenches (3×1 table)
- Tool racks (wall-mounted crates)
- Machinery (pillars)
- Parts bins (crate clusters)

#### 4.2 New Furniture Objects

**Expand ObjectType enum** (`tilemap.go:87-113`):

**Residential**:
- `ObjectBed` (2×1, blocks movement, soft cover)
- `ObjectSofa` (2×1, blocks movement, soft cover)
- `ObjectDesk` (2×1, like table)
- `ObjectBookshelf` (1×1, blocks movement, medium cover)
- `ObjectCounter` (1×1, blocks movement, hard cover)
- `ObjectRefrigerator` (1×1, blocks movement, hard cover)

**Industrial**:
- `ObjectPallet` (2×2, blocks movement, soft cover)
- `ObjectMachinery` (2×2, blocks movement, hard cover)
- `ObjectWorkbench` (3×1, blocks movement, medium cover)
- `ObjectShelving` (1×1, blocks movement, medium cover)
- `ObjectBarrel` (1×1, blocks movement, medium cover)

**Military**:
- `ObjectBunk` (2×1, like bed)
- `ObjectLocker` (1×1, like crate)
- `ObjectWeaponRack` (1×1, wall-mounted)
- `ObjectRadio` (1×1, like crate)

#### 4.3 Clutter & Scatter

**Small decorative objects** (don't block movement):
- Chairs (current, keep)
- Trash bins
- Small boxes
- Debris piles (rubble-like)

**Placement strategy**:
- **Clustered**: Furniture in logical groups (desk + chair + filing cabinet)
- **Wall-aligned**: Shelves, counters along walls
- **Scattered**: Crates, barrels in warehouse open spaces
- **Pathways**: Leave 1-unit wide paths through dense rooms

**Tactical impact**:
- **Cover variety**: Mix of soft/medium/hard cover
- **Movement impediment**: Dense rooms slow movement
- **LOS obstruction**: Furniture creates blind spots
- **Firefight complexity**: Can't shoot straight across rooms

---

### 5. Building Archetypes

#### 5.1 Building Type System

**Add building type classification**:

```go
type BuildingType uint8

const (
    BuildingTypeResidential BuildingType = iota
    BuildingTypeIndustrial
    BuildingTypeCommercial
    BuildingTypeMilitary
    BuildingTypeAgricultural
    buildingTypeCount
)

type BuildingFootprint struct {
    rect
    Type BuildingType
    RoomCount int
    DoorCount int
    WindowCount int
}
```

**Type determines**:
- Footprint shape (residential = rectangle, industrial = L-shape)
- Size range (residential = small-medium, industrial = large-huge)
- Interior layout (residential = rooms+hallway, warehouse = open)
- Furniture density (residential = high, warehouse = low)
- Window placement (residential = regular, bunker = slits)
- Material/cover quality (residential = medium, bunker = high)

#### 5.2 Residential Buildings

**Characteristics**:
- **Size**: 4×4 to 7×6 units
- **Shape**: Rectangle or L-shape
- **Rooms**: 3-6 (living, kitchen, 1-2 bedrooms, bathroom)
- **Hallway**: Central or L-shaped
- **Windows**: Regular, every room except bathroom
- **Doors**: 1-2 exterior (front + back)
- **Furniture**: High density (70-90% per room)
- **Floor**: Wood or tile
- **Tactical value**: Medium (good cover, multiple rooms)

**Variants**:
- **Small house**: 4×4, 2-3 rooms, single door
- **Large house**: 6×6, 5-6 rooms, hallway, 2 doors
- **Duplex**: 8×5, mirrored layout, 2 front doors

#### 5.3 Industrial Buildings

**Warehouse**:
- **Size**: 10×8 to 16×12 units (large)
- **Shape**: Rectangle or L-shape
- **Rooms**: 1-3 (open floor + office + storage)
- **Hallway**: None (direct access)
- **Windows**: High/sparse (clerestory)
- **Doors**: 2-3 (large loading door + side doors)
- **Furniture**: Low density (20-40%), clustered crates
- **Floor**: Concrete
- **Tactical value**: High (size, multiple firing positions)

**Factory**:
- **Size**: 14×10 to 20×14 units (very large)
- **Shape**: Rectangle or compound (multiple wings)
- **Rooms**: 4-8 (production floor, offices, storage, workshop)
- **Hallway**: Perimeter or central
- **Windows**: Regular on offices, sparse on production
- **Doors**: 3-4 (loading bays, personnel doors)
- **Furniture**: Medium density (50%), machinery + workbenches
- **Floor**: Concrete
- **Tactical value**: Very high (dominant position, many rooms)

**Workshop**:
- **Size**: 6×5 to 8×6 units (medium)
- **Shape**: Rectangle
- **Rooms**: 2-3 (main shop + office + storage)
- **Windows**: Regular
- **Doors**: 2 (large bay + side door)
- **Furniture**: High density (60%), workbenches + tools
- **Floor**: Concrete
- **Tactical value**: Medium-high (good cover, multiple positions)

#### 5.4 Commercial Buildings

**Shop**:
- **Size**: 5×4 to 7×5 units (small-medium)
- **Shape**: Rectangle
- **Rooms**: 2-3 (shop floor + stockroom + office)
- **Windows**: Large front windows (display)
- **Doors**: 1-2 (front entrance + back door)
- **Furniture**: Medium-high (counters, shelves, displays)
- **Floor**: Tile
- **Tactical value**: Medium (good windows, limited depth)

**Office building**:
- **Size**: 8×6 to 10×8 units (medium-large)
- **Shape**: Rectangle or L-shape
- **Rooms**: 6-10 (offices, conference room, break room)
- **Hallway**: Central corridor
- **Windows**: Regular, all offices
- **Doors**: 2-3 (main + side + emergency)
- **Furniture**: High density (desks, chairs, filing cabinets)
- **Floor**: Tile or carpet (wood)
- **Tactical value**: High (many rooms, good windows)

#### 5.5 Military Buildings

**Bunker**:
- **Size**: 4×4 to 6×5 units (small-medium)
- **Shape**: Rectangle, thick walls
- **Rooms**: 1-2 (main room + storage)
- **Windows**: Firing slits only (narrow, strategic)
- **Doors**: 1 (reinforced, single entry)
- **Furniture**: Low (ammo crates, radio, table)
- **Floor**: Concrete
- **Walls**: Extra thick (2-cell walls)
- **Tactical value**: Very high (excellent cover, defensible)

**Barracks**:
- **Size**: 8×5 to 12×6 units (medium-large)
- **Shape**: Rectangle
- **Rooms**: 2-4 (sleeping bay + latrine + office)
- **Windows**: Regular, high placement
- **Doors**: 2 (front + back)
- **Furniture**: High density (rows of bunks, lockers)
- **Floor**: Concrete
- **Tactical value**: High (size, multiple rooms)

**Guard post**:
- **Size**: 3×3 to 4×4 units (small)
- **Shape**: Square
- **Rooms**: 1 (single room)
- **Windows**: All four sides (360° observation)
- **Doors**: 1-2 (entry + emergency)
- **Furniture**: Minimal (chair, table, radio)
- **Floor**: Concrete
- **Tactical value**: Very high (observation, all-around defense)

#### 5.6 Agricultural Buildings

**Barn**:
- **Size**: 10×8 to 14×10 units (large)
- **Shape**: Rectangle
- **Rooms**: 1-2 (open floor + loft/storage)
- **Windows**: Sparse, high
- **Doors**: 2-3 (large barn doors + side door)
- **Furniture**: Low (hay bales, equipment)
- **Floor**: Dirt or wood
- **Tactical value**: Medium-high (size, limited entry)

---

### 6. Exterior Objects & Defensive Structures

#### 6.1 Perimeter Fencing & Walls

**Wire Fence** (Chain-link):
- **Placement**: Around industrial buildings, compounds, military areas
- **Height**: Standard (blocks movement, transparent LOS)
- **Length**: 3-8 unit runs along property lines
- **Gates**: 1-2 unit gaps with gate objects (open/closed)
- **Tactical**: Channelizes movement, slows entry, provides no cover
- **Destructible**: Can be cut/breached (future)

**Wooden Fence**:
- **Placement**: Around residential properties, farms
- **Height**: Low (blocks movement, partial LOS opacity 0.3)
- **Length**: 4-10 unit runs along property boundaries
- **Gates**: Wooden gate objects
- **Tactical**: Light concealment, minimal cover
- **Condition**: Can be damaged/broken sections

**Concrete Wall** (Perimeter):
- **Placement**: Military compounds, secure facilities
- **Height**: Tall (blocks movement, blocks LOS)
- **Length**: 6-15 unit runs, forms perimeter
- **Gates**: Reinforced gate, guard post adjacent
- **Tactical**: Hard cover, strong defensive line
- **Thickness**: 1-2 cells (thicker than building walls)

**Brick Wall** (Property):
- **Placement**: Around estates, gardens, commercial properties
- **Height**: Medium (blocks movement, blocks LOS)
- **Length**: 5-12 unit runs
- **Tactical**: Good cover, durable
- **Features**: May have decorative elements, gates

#### 6.2 Defensive Fortifications

**Trench Line**:
- **Placement**: Military defensive positions, field fortifications
- **Layout**: Zigzag or straight runs, 5-20 units long
- **Width**: 1-2 cells
- **Depth**: Provides excellent cover (prone stance bonus)
- **Tactical**: Firing positions, protected movement
- **Features**:
  - Firing steps (elevated positions)
  - Traverses (perpendicular sections to limit enfilade)
  - Communication trenches (connecting parallel trenches)
  - Dugouts (small covered sections, 2×2 units)

**Barbed Wire Entanglement**:
- **Placement**: In front of defensive lines, around perimeters
- **Layout**: 2-4 parallel rows, 3-10 units wide
- **Effect**: Severe movement penalty (0.2× speed), damage over time
- **Tactical**: Area denial, channelizes attackers
- **Breaching**: Can be cut (slow), blown (explosives), or bypassed
- **Types**:
  - **Concertina wire**: Coiled, 1-2 unit depth
  - **Wire fence**: Staked, 2-3 unit depth
  - **Tanglefoot**: Low, trip hazard

**Sandbag Walls** (already exists as ObjectSandbag):
- **Enhanced placement**: Defensive lines, not just scattered
- **Layouts**:
  - **Straight wall**: 3-6 sandbags in line
  - **L-shaped**: Corner positions
  - **Firing position**: U-shaped (3 sides)
  - **Bunker entrance**: Flanking sandbag walls

**Dragon's Teeth** (Anti-tank obstacles):
- **Placement**: Road blocks, defensive lines
- **Layout**: Staggered rows, 3-5 deep
- **Effect**: Blocks vehicle movement (future), provides cover
- **Tactical**: Infantry can use as cover, channels movement
- **Material**: Concrete pyramids (ObjectATBarrier already exists)

**Slit Trenches** (already exists as ObjectSlitTrench):
- **Enhanced placement**: Individual fighting positions
- **Layout**: Scattered along defensive lines, 1-2 per sector
- **Effect**: Excellent cover for prone soldier
- **Tactical**: Sniper positions, overwatch

#### 6.3 Mundane Scatter Objects

**Vehicles & Wrecks**:
- **Parked vehicles**:
  - **Car**: 2×1 object, medium cover, near residential
  - **Truck**: 3×2 object, good cover, near industrial/commercial
  - **Van**: 2×2 object, good cover, scattered
- **Vehicle wrecks** (ObjectVehicleWreck already exists):
  - **Burnt-out car**: 2×1, hard cover, scattered in combat zones
  - **Destroyed truck**: 3×2, excellent cover
  - **Abandoned tank**: 4×3, dominant cover (rare)

**Storage & Containers**:
- **Shipping container**: 4×2 object, blocks movement/LOS, excellent cover
  - Placement: Industrial areas, warehouses, ports
  - Can be stacked (future: 2-high)
- **Dumpster**: 2×1 object, good cover, behind commercial buildings
- **Fuel tank**: 2×2 cylindrical, medium cover, industrial/military
  - Tactical: Explosive hazard (future)
- **Water tank**: 2×2 or 3×3, hard cover, rooftops/industrial

**Agricultural**:
- **Hay bales**: 2×1 object, soft cover, farms/fields
  - Layout: Stacks of 2-4 bales
  - Tactical: Concealment, minimal protection
- **Tractor**: 2×2 object, good cover, farms
- **Plow/equipment**: 2×1 object, light cover, scattered in fields
- **Silo**: 3×3 tall cylinder, blocks LOS, farms
  - Tactical: Landmark, no entry

**Urban Clutter**:
- **Street furniture**:
  - **Bench**: 2×1, light cover, parks/streets
  - **Trash bin**: 1×1, minimal cover, streets
  - **Mailbox**: 1×1, decorative, residential
  - **Street sign**: 1×1, decorative, intersections
  - **Fire hydrant**: 1×1, decorative, streets
- **Utility boxes**:
  - **Electrical box**: 1×1, light cover, building exteriors
  - **Generator**: 2×1, medium cover, industrial/military
  - **AC unit**: 1×1, light cover, building exteriors

**Construction Materials**:
- **Lumber pile**: 2×1 or 3×1, medium cover, construction sites
- **Pipe stack**: 2×1, medium cover, industrial
- **Concrete blocks**: 1×1 scattered, light cover
- **Scaffolding**: 2×3 or 3×3, passable, light cover, construction

**Natural Features** (enhanced):
- **Rock pile**: 2×2 or 3×2, good cover, rough terrain
- **Boulder**: 1×1 or 2×2, excellent cover, scattered
- **Log pile**: 2×1 or 3×1, medium cover, forests/lumber areas
- **Stump**: 1×1, light cover, cleared forests

#### 6.4 Compound & Perimeter Generation

**Residential Property**:
```
┌─────────────────┐  ← Wooden fence
│  🏠 House       │
│                 │
│  🚗 Car  🌳    │  ← Scattered objects
│                 │
│  [Gate]         │  ← Front gate
└─────────────────┘
```
- Wooden fence perimeter (70% chance)
- Front gate aligned with door
- Driveway (paved ground)
- 1-2 trees, car (50% chance), trash bins

**Industrial Compound**:
```
┌──────────────────────┐  ← Chain-link fence
│ [Gate]               │
│                      │
│  🏭 Warehouse        │
│                      │
│  📦📦 Containers      │  ← Shipping containers
│  🚛 Truck            │
│                      │
└──────────────────────┘
```
- Chain-link fence perimeter (90% chance)
- Security gate with guard post (small building)
- Shipping containers (2-6)
- Parked trucks (1-2)
- Fuel tanks, dumpsters

**Military Base**:
```
┌────────────────────────┐  ← Concrete wall
│ [Gate] 🏠              │
│  Guard                 │
│                        │
│  🏢 Barracks  🏢       │
│                        │
│  ╱╱╱ Barbed wire       │
│  ▓▓▓ Sandbags          │
│  ═══ Trench line       │
│                        │
└────────────────────────┘
```
- Concrete perimeter wall (100%)
- Guard post at gate
- Barbed wire inner perimeter
- Sandbag defensive positions
- Trench lines (outer defense)
- Vehicle wrecks (training/combat damage)

**Farm Complex**:
```
┌─────────────────┐  ← Wooden fence (partial)
│                 │
│  🏠 Farmhouse   │
│                 │
│  🌾🌾 Hay bales │
│  🚜 Tractor     │
│                 │
│  🏚️ Barn        │
│                 │
└─────────────────┘
```
- Wooden fence (partial, 40% coverage)
- Hay bales scattered
- Tractor, plow equipment
- Barn (large building)
- Silo (landmark)

#### 6.5 Defensive Line Generation

**Hasty Defense** (squad-level):
- **Sandbag positions**: 2-4 U-shaped emplacements
- **Spacing**: 20-40 units apart
- **Placement**: Along likely enemy approach
- **Depth**: Single line

**Prepared Defense** (platoon-level):
- **Trench line**: 30-60 unit zigzag trench
- **Firing positions**: Every 10-15 units
- **Barbed wire**: 10-20 units in front of trench
- **Depth**: Trench + wire obstacle
- **Flanks**: Sandbag positions on ends

**Fortified Defense** (company-level):
- **Primary trench**: 60-100 unit main line
- **Secondary trench**: 40-60 unit fallback line
- **Communication trenches**: Connecting lines
- **Barbed wire**: Multiple belts, 20-40 units deep
- **Bunkers**: 2-4 reinforced positions (small buildings)
- **AT obstacles**: Dragon's teeth on roads/approaches
- **Depth**: Multi-layered (100+ units)

#### 6.6 Placement Algorithms

**Fence generation**:
```
1. Identify building footprint
2. Expand perimeter by 2-4 units (property boundary)
3. Place fence along boundary (skip front for gate)
4. Add gate aligned with main door
5. Corner posts at 90° turns
```

**Defensive line generation**:
```
1. Identify defensive sector (e.g., east side of map)
2. Generate trench centerline (zigzag or straight)
3. Place barbed wire 5-10 units in front
4. Add sandbag positions on flanks
5. Place slit trenches for overwatch (behind main line)
6. Add dugouts every 20-30 units (covered positions)
```

**Scatter object placement**:
```
1. Identify terrain type (urban, industrial, rural)
2. Select object pool (vehicles for urban, hay for rural)
3. Place objects with spacing constraints (min 5 units apart)
4. Cluster related objects (crates near warehouse, cars near houses)
5. Avoid blocking critical paths (roads, building doors)
```

**Tactical considerations**:
- **Cover lanes**: Leave gaps in obstacles for movement
- **Interlocking fields**: Defensive positions cover each other
- **Depth**: Multiple layers of obstacles/positions
- **Channelization**: Obstacles funnel attackers into kill zones

---

### 7. Building Identification & UI

#### 7.1 Building Labels

**Subtle text overlay**:
- **Rendering**: Small text above building center
- **Font**: 8-10pt, semi-transparent (alpha 0.6)
- **Color**: White with dark outline (readable on any background)
- **Content**: Building type name
  - "House" / "Warehouse" / "Bunker" / "Office"
  - "Factory" / "Shop" / "Barracks" / "Guard Post"
- **Toggle**: Keyboard shortcut to show/hide labels (e.g., 'L' key)
- **Zoom-dependent**: Only show when zoomed in (scale > 1.5×)

**Implementation**:
```go
// In Game struct
showBuildingLabels bool

// In drawWorld()
if g.showBuildingLabels && g.scale > 1.5 {
    for i, fp := range g.buildingFootprints {
        label := g.buildingTypes[i].String()
        x := fp.x + fp.w/2
        y := fp.y - 8  // Above building
        drawTextWithOutline(screen, label, x, y, colorWhite, colorBlack)
    }
}
```

#### 7.2 Click-to-Inspect System

**Mouse interaction**:
- **Click building**: Show info panel
- **Panel location**: Top-right corner or mouse cursor
- **Panel content**:
  ```
  ┌─────────────────────┐
  │ WAREHOUSE           │
  │─────────────────────│
  │ Size: 12×10 (Large) │
  │ Rooms: 3            │
  │ Doors: 3 (2 loading)│
  │ Windows: 8          │
  │ Quality: ████░ 0.82 │
  │─────────────────────│
  │ Tactical Value: High│
  │ Cover: Excellent    │
  │ Sightlines: Good    │
  │ Accessibility: High │
  │─────────────────────│
  │ Status: Unoccupied  │
  │ [or]                │
  │ Occupied: Blue Sq 1 │
  │ [or]                │
  │ Enemy Presence: 60% │
  └─────────────────────┘
  ```

**Data structure**:
```go
type BuildingInfo struct {
    Type          BuildingType
    Name          string  // "Warehouse", "Small House", etc.
    SizeCategory  string  // "Small", "Medium", "Large", "Huge"
    RoomCount     int
    DoorCount     int
    WindowCount   int
    Quality       BuildingQuality  // Already exists
    OccupiedBy    *Squad           // nil if unoccupied
    EnemyPresence float64          // From BuildingIntel
}
```

**Click detection**:
```go
// In Game.Update()
if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
    mx, my := ebiten.CursorPosition()
    worldX, worldY := g.screenToWorld(mx, my)

    buildingIdx := g.findBuildingAtPosition(worldX, worldY)
    if buildingIdx >= 0 {
        g.selectedBuilding = buildingIdx
        g.showBuildingInfo = true
    }
}

// In Game.Draw()
if g.showBuildingInfo && g.selectedBuilding >= 0 {
    g.drawBuildingInfoPanel(screen, g.selectedBuilding)
}
```

#### 7.3 Building Naming System

**Procedural names** (more immersive than just "Warehouse"):

**Residential**:
- "Small House" / "Large House" / "Duplex"
- "Cottage" / "Bungalow" / "Villa"
- Street address: "House #12" / "Residence 4A"

**Industrial**:
- "Warehouse A" / "Warehouse B" (lettered)
- "Storage Facility" / "Distribution Center"
- "Factory Building 1" / "Production Hall"
- "Workshop" / "Maintenance Shed"

**Commercial**:
- "Shop" / "Store" / "Market"
- "Office Building" / "Admin Block"
- Business type: "Bakery" / "Hardware Store" / "Cafe"

**Military**:
- "Bunker 3" / "Pillbox A"
- "Barracks 1" / "Barracks 2"
- "Command Post" / "HQ Building"
- "Guard Post North" / "Guard Post East"
- "Armory" / "Supply Depot"

**Agricultural**:
- "Farmhouse" / "Main Barn" / "Equipment Shed"
- "Silo Complex" / "Grain Storage"

**Implementation**:
```go
func generateBuildingName(bType BuildingType, index int) string {
    switch bType {
    case BuildingTypeIndustrial:
        if size > largeThreshold {
            return fmt.Sprintf("Warehouse %c", 'A'+index)
        }
        return "Workshop"
    case BuildingTypeMilitary:
        return fmt.Sprintf("Bunker %d", index+1)
    case BuildingTypeResidential:
        if roomCount <= 3 {
            return "Small House"
        }
        return "Large House"
    // ... etc
    }
}
```

#### 7.4 Visual Indicators

**Building state overlays**:
- **Unoccupied**: Default rendering
- **Friendly occupied**: Green tint on roof/outline
- **Enemy occupied**: Red tint (if known via intel)
- **Contested**: Yellow/orange tint
- **Cleared**: Checkmark icon above building

**Icon system**:
```
🏠 Residential
🏭 Industrial
🏢 Commercial
🛡️ Military
🌾 Agricultural
```

**Minimap representation**:
- Buildings shown as colored rectangles
- Color indicates type (gray=residential, brown=industrial, green=military)
- Occupied buildings have border color (blue=friendly, red=enemy)

#### 7.5 Accessibility Features

**Keyboard shortcuts**:
- `L`: Toggle building labels
- `I`: Toggle building info panel
- `B`: Cycle through buildings (select next)
- `Shift+B`: Cycle backwards
- `Esc`: Close building info panel

**Mouse interactions**:
- **Left-click**: Select building, show info
- **Right-click**: Deselect, close info
- **Hover**: Highlight building outline (subtle glow)

**Screen reader support** (future):
- Building info panel text is accessible
- Keyboard navigation through buildings
- Audio cues for building selection

---

### 8. Road Generation & Organic Lot Placement

#### 8.1 Current Road System Analysis

**Existing implementation** (`gridroads.go`):
- **Grid-aligned roads**: 2 horizontal + 1-2 vertical main roads
- **90° turns**: Occasional perpendicular shifts
- **Side streets**: Short stubs branching from main roads
- **Width**: 5 tiles (main), 3 tiles (side streets)
- **Pavement**: 70% chance of sidewalks on road edges
- **Building placement**: `buildingCandidatesAlongGridRoads()` samples road tiles, places buildings adjacent

**Limitations**:
- **Rigid grid**: All roads are axis-aligned (no curves, diagonals)
- **Uniform spacing**: Roads evenly distributed across map
- **No lot system**: Buildings placed directly adjacent to roads
- **Square footprints**: No organic property boundaries
- **No road hierarchy**: All roads look similar (no highways vs alleys)
- **No cul-de-sacs**: Dead-end streets not supported
- **No intersections**: T-junctions and crossroads are just overlapping tiles

#### 8.2 Organic Lot Generation System

**Concept**: Generate property boundaries first, then fit buildings within lots.

**Lot types**:
- **Residential lot**: 60-120 unit area, setback from road
- **Industrial lot**: 150-400 unit area, road frontage + rear access
- **Commercial lot**: 80-150 unit area, maximum road frontage
- **Military compound**: 300-600 unit area, perimeter wall
- **Agricultural parcel**: 200-800 unit area, irregular shape

**Lot generation algorithm**:
```
1. Generate road network (enhanced system)
2. Identify road segments and intersections
3. Subdivide spaces between roads into lots:
   a. Voronoi-based subdivision (organic boundaries)
   b. Constrained by road network
   c. Minimum lot size enforcement
4. Classify lot type (based on location, road type, neighbors)
5. Generate building within lot:
   a. Respect setbacks (front, side, rear)
   b. Building size proportional to lot size
   c. Orientation toward road
   d. Driveway/path from building to road
6. Add lot features (fence, landscaping, parking)
```

**Voronoi subdivision**:
```
1. Place seed points in spaces between roads
2. Compute Voronoi cells (each cell = one lot)
3. Clip cells to road boundaries
4. Merge small cells (< minimum lot size)
5. Regularize boundaries (smooth jagged edges)
```

**Setback rules**:
- **Front setback**: 2-4 units from road (residential), 1-2 units (commercial)
- **Side setback**: 1-2 units from lot boundary
- **Rear setback**: 2-3 units from rear lot line
- **Zero lot line**: Industrial/commercial may build to side boundary

**Building-to-road connection**:
```
1. Identify building front door position
2. Trace path from door to nearest road
3. Path types:
   - Driveway (paved, 1-2 units wide)
   - Footpath (narrow, 1 unit)
   - Shared access (multiple buildings)
4. Stamp path into TileMap (GroundPavement or GroundDirt)
```

#### 8.3 Enhanced Road Network Generation

**Road hierarchy**:

**Highway** (rare, 1 per large map):
- **Width**: 7-9 tiles
- **Function**: Main thoroughfare, map crossing
- **Features**: Wide, straight, minimal turns
- **Speed**: Fast movement
- **Buildings**: Set back 4-6 units, industrial/commercial

**Main road** (current system):
- **Width**: 5 tiles
- **Function**: Primary streets, grid structure
- **Features**: Occasional turns, intersections
- **Buildings**: Mixed use, 2-4 unit setback

**Side street**:
- **Width**: 3 tiles
- **Function**: Residential access
- **Features**: Shorter runs, branches from main roads
- **Buildings**: Residential, close setback

**Alley**:
- **Width**: 2 tiles
- **Function**: Rear access, service
- **Features**: Narrow, unpaved (GroundDirt or GroundGravel)
- **Buildings**: Rear entrances, loading docks

**Cul-de-sac**:
- **Width**: 3 tiles
- **Function**: Dead-end residential street
- **Features**: Circular turnaround at end
- **Buildings**: Residential, arranged around circle

**Dirt road/path**:
- **Width**: 2 tiles
- **Function**: Rural access, farm roads
- **Features**: Unpaved, winding
- **Buildings**: Farms, scattered rural structures

**Curved roads**:
```
1. Generate road centerline as Bezier curve or arc
2. Sample points along curve (every 2-3 tiles)
3. Expand to road width perpendicular to tangent
4. Stamp tiles (smooth transitions at curves)
```

**Intersection improvements**:
```
1. Detect road crossings (T-junction, 4-way, Y-junction)
2. Expand intersection area (wider than road width)
3. Add corner rounding (chamfer or arc)
4. Optional: Traffic features (painted lines, stop signs)
```

**Roundabout generation**:
```
1. Identify suitable intersection (3+ roads)
2. Generate circular center island (3-5 unit radius)
3. Outer ring road (2-3 tiles wide)
4. Connect incoming roads tangentially
5. Center island: decorative (grass, monument)
```

#### 8.4 Context-Aware Road Placement

**Urban pattern** (dense, grid):
- Main roads: 40-60 unit spacing
- Side streets: 20-30 unit spacing
- Lots: Small (60-100 units), rectangular
- Buildings: Attached or close setback

**Suburban pattern** (medium density):
- Main roads: 60-80 unit spacing
- Side streets: 30-50 unit spacing, some cul-de-sacs
- Lots: Medium (100-200 units), varied shapes
- Buildings: Moderate setback, detached

**Rural pattern** (sparse):
- Main roads: 80-120 unit spacing
- Dirt roads: Winding, irregular
- Lots: Large (200-800 units), irregular
- Buildings: Large setback, scattered

**Industrial pattern** (functional):
- Main roads: Wide (5-7 tiles)
- Service roads: Grid or loop
- Lots: Very large (200-400 units), rectangular
- Buildings: Minimal setback, maximize footprint

**Military pattern** (secure):
- Perimeter road: Loops compound
- Internal roads: Grid, wide
- Lots: Functional zones (barracks, admin, storage)
- Buildings: Organized, aligned

#### 8.5 Lot Feature Generation

**Residential lot features**:
- **Front yard**: Grass, 1-2 trees, mailbox
- **Driveway**: Paved path from road to building/garage
- **Backyard**: Fenced (70% chance), scattered objects
- **Side yard**: Narrow, minimal features
- **Parking**: Driveway or carport (car object 50% chance)

**Industrial lot features**:
- **Perimeter fence**: Chain-link (90% chance)
- **Gate**: Aligned with main entrance
- **Parking lot**: Paved area with truck/car objects
- **Loading area**: Concrete pad, shipping containers
- **Storage**: Outdoor crates, pallets, fuel tanks

**Commercial lot features**:
- **Front parking**: Paved lot with multiple cars
- **Signage**: Decorative objects near road
- **Sidewalk**: Wide pavement along road frontage
- **Rear access**: Alley or service road
- **Dumpster**: Rear of building

**Military compound features**:
- **Perimeter wall**: Concrete, 100% coverage
- **Guard post**: At gate entrance
- **Internal roads**: Paved grid
- **Parade ground**: Open paved area
- **Defensive positions**: Sandbags, trenches at perimeter

**Agricultural lot features**:
- **Fence**: Wooden, partial coverage (40%)
- **Dirt tracks**: Unpaved paths to fields
- **Equipment**: Tractor, plow, hay bales
- **Outbuildings**: Barn, silo, shed
- **Fields**: Open grass areas

#### 8.6 Implementation Algorithm

**High-level generation flow**:
```
1. Generate road network:
   a. Main roads (grid or organic)
   b. Side streets (branches)
   c. Alleys (rear access)
   d. Cul-de-sacs (dead ends)

2. Identify road segments and intersections

3. Generate lots:
   a. Voronoi subdivision of spaces between roads
   b. Classify lot type (residential, industrial, etc.)
   c. Compute lot boundaries and setbacks

4. Generate buildings within lots:
   a. Determine building size (proportional to lot)
   b. Position building (respect setbacks)
   c. Orient toward road (front door faces road)
   d. Generate building (type-specific architecture)

5. Connect buildings to roads:
   a. Trace path from door to road
   b. Stamp driveway/footpath

6. Add lot features:
   a. Fencing (type-specific)
   b. Parking (cars, trucks)
   c. Landscaping (trees, grass)
   d. Scatter objects (context-aware)

7. Stamp everything to TileMap
```

**Voronoi lot subdivision** (detailed):
```go
func generateLots(roads []Road, mapBounds rect, rng *rand.Rand) []Lot {
    // 1. Generate seed points in spaces between roads
    seeds := generateSeedPoints(roads, mapBounds, rng)

    // 2. Compute Voronoi diagram
    voronoi := computeVoronoi(seeds, mapBounds)

    // 3. Clip cells to road boundaries (lots can't overlap roads)
    lots := clipVoronoiToRoads(voronoi, roads)

    // 4. Merge small lots (< minLotSize)
    lots = mergeSmallLots(lots, minLotSize)

    // 5. Classify lot types (based on location, neighbors)
    for i := range lots {
        lots[i].Type = classifyLot(lots[i], roads, neighbors)
    }

    return lots
}

func generateBuildingInLot(lot Lot, rng *rand.Rand) Building {
    // 1. Compute setbacks
    frontSetback := lot.Type.FrontSetback()
    sideSetback := lot.Type.SideSetback()
    rearSetback := lot.Type.RearSetback()

    // 2. Compute buildable area (lot minus setbacks)
    buildable := lot.Bounds.Shrink(frontSetback, sideSetback, rearSetback)

    // 3. Determine building size (60-90% of buildable area)
    buildingSize := computeBuildingSize(buildable, lot.Type, rng)

    // 4. Position building within buildable area
    buildingPos := positionBuilding(buildable, buildingSize, lot.RoadSide)

    // 5. Generate building (type-specific)
    building := generateBuilding(buildingPos, buildingSize, lot.Type, rng)

    // 6. Add path from building to road
    path := tracePath(building.FrontDoor, lot.RoadEdge, rng)
    building.AccessPath = path

    return building
}
```

**Road-to-lot edge detection**:
```go
func identifyRoadEdge(lot Lot, roads []Road) (edgePos Vec2, roadSide Side) {
    // Find which lot edge is adjacent to a road
    for _, edge := range lot.Edges {
        for _, road := range roads {
            if edge.IntersectsRoad(road) {
                return edge.Center(), edge.Side
            }
        }
    }
    return lot.Bounds.Center(), SideFront // fallback
}
```

#### 8.7 Factory Compound Example

**Industrial compound with perimeter fence**:
```
Road ═══════════════════════════════════
     ║                                 ║
     ║  [Gate]                         ║
     ║    ↓                            ║
     ║  ┌─────────────────────────┐   ║ ← Chain-link fence
     ║  │                         │   ║
     ║  │   🏭 Factory Building   │   ║
     ║  │   (12×10 units)         │   ║
     ║  │                         │   ║
     ║  │  📦📦 Containers         │   ║
     ║  │  🚛 Truck               │   ║
     ║  │                         │   ║
     ║  │  [Loading Dock]         │   ║
     ║  │                         │   ║
     ║  └─────────────────────────┘   ║
     ║                                 ║
═════════════════════════════════════════
```

**Generation steps**:
1. Lot identified: 300 unit area, industrial type
2. Setbacks: Front 2 units, sides 1 unit, rear 2 units
3. Building: 12×10 factory, positioned center-rear of lot
4. Perimeter fence: Chain-link, 1 unit inside lot boundary
5. Gate: Aligned with road, 2 units wide
6. Access road: Paved from gate to factory front door
7. Loading dock: Rear of factory, concrete pad
8. Scatter objects: 4-6 shipping containers, 1-2 trucks, fuel tank
9. Parking: Small paved area near gate (employee parking)

---

### 9. Building Damage & Battle Scars

#### 9.1 Current Damage System

**Existing**: Random rubble circles (GroundRubbleLight, GroundRubbleHeavy)
- Scattered during biome generation
- Purely cosmetic (ground type)
- No structural damage to buildings
- No tactical impact beyond movement penalty

**Limitations**:
- No building damage (walls always intact)
- No partial destruction (building is whole or absent)
- No damage progression (no fresh vs old damage)
- No tactical opportunities (no sniper nests in ruins)

#### 9.2 Building Damage Taxonomy

**Damage levels** (progressive):

**Level 0: Pristine** (default)
- No damage
- All walls, windows, doors intact
- Full structural integrity

**Level 1: Light damage** (cosmetic)
- **Wall pockmarks**: Bullet holes, small cracks
- **Broken windows**: 20-40% of windows shattered
- **Damaged doors**: Some doors broken/ajar
- **Roof damage**: Missing tiles, small holes
- **Tactical**: Broken windows passable, minor cover reduction

**Level 2: Moderate damage** (functional impact)
- **Wall breaches**: 1-3 small holes (1×1 cell)
  - Provides vision through wall
  - Sniper nest potential (cover + sightline)
  - Not passable (rubble blocks)
- **Collapsed sections**: 10-20% of walls damaged
- **Broken windows**: 60-80% shattered
- **Interior debris**: Furniture destroyed, rubble piles
- **Tactical**: New firing positions, reduced cover quality

**Level 3: Heavy damage** (major structural)
- **Large breaches**: 2-5 large holes (2×2 to 3×2 cells)
  - Passable (alternative entry points)
  - Reduced cover (wall sections missing)
- **Partial collapse**: One corner/section collapsed
  - Roof caved in (open to sky)
  - Floor damage (rubble piles)
- **Broken windows**: 90-100% destroyed
- **Interior destruction**: Most furniture destroyed
- **Tactical**: Dangerous (collapse risk), but new routes

**Level 4: Severe damage** (near-ruin)
- **Massive breaches**: Multiple wall sections gone
- **Roof collapse**: 50-80% of roof missing
- **Floor collapse**: Some rooms inaccessible (rubble-filled)
- **Structural instability**: Leaning walls, exposed beams
- **Tactical**: Limited utility, mostly rubble cover

**Level 5: Ruined** (destroyed)
- **Walls**: Only fragments standing (20-40% of perimeter)
- **Roof**: Completely gone
- **Interior**: Filled with rubble (GroundRubbleHeavy)
- **Footprint**: Recognizable but gutted
- **Tactical**: Rubble pile, no interior movement

**Level 6: Razed** (obliterated)
- **Walls**: Scattered debris only
- **Foundation**: Crater or rubble field
- **Footprint**: Barely visible
- **Tactical**: Open ground with scattered cover

#### 9.3 Damage Feature Types

**Wall breaches**:
- **Small hole** (1×1 cell):
  - Vision through wall (LOS opacity 0.3)
  - Not passable (rubble blocks)
  - Good sniper position (cover + peek)
  - Placement: Random wall section, not corners

- **Medium breach** (2×1 or 1×2 cells):
  - Vision through wall (LOS opacity 0.1)
  - Passable with penalty (movement ×0.6)
  - Rubble pile at base (ObjectRubblePile)
  - Placement: Mid-wall, creates new entry

- **Large breach** (2×2 or 3×2 cells):
  - Fully open (LOS opacity 0.0)
  - Passable (movement ×0.8, rubble)
  - Major tactical feature (flanking route)
  - Placement: Wall section, may expose interior

**Collapsed sections**:
- **Corner collapse**:
  - One building corner destroyed
  - Walls on two adjacent sides missing (3-5 cells)
  - Roof section caved in
  - Rubble pile in corner (2×2 to 3×3)
  - Tactical: Open corner, reduced cover

- **Wall collapse**:
  - One wall section fallen (4-8 cells)
  - Roof sags or collapses above
  - Rubble pile along wall base
  - Tactical: Entire side exposed

- **Roof collapse**:
  - Roof caved into interior
  - Walls still standing
  - Interior filled with rubble (impassable)
  - Tactical: Building unusable, exterior cover only

**Window damage**:
- **Broken window**: ObjectWindowBroken (already exists)
  - Passable with penalty
  - Glass shards (minor damage)
  - 20-100% of windows depending on damage level

- **Blown-out window**:
  - Frame destroyed, fully open
  - No movement penalty
  - Tactical: Easy entry point

**Door damage**:
- **Damaged door**: ObjectDoorBroken (already exists)
  - Frame intact, door destroyed
  - Fully passable
  - 30-70% of doors depending on damage level

- **Blown doorway**:
  - Frame destroyed, wider opening
  - Rubble at threshold
  - Tactical: Easier breach, less chokepoint

**Interior damage**:
- **Furniture destruction**:
  - 30-90% of furniture destroyed (removed)
  - Replaced with ObjectRubblePile
  - Reduces cover, opens sightlines

- **Floor damage**:
  - Holes in floor (impassable cells)
  - Rubble piles (movement penalty)
  - Craters (from explosions)

- **Structural debris**:
  - Fallen beams (ObjectRubblePile, linear)
  - Collapsed walls (interior partitions)
  - Ceiling fragments

#### 9.4 Damage Patterns & Placement

**Blast damage** (explosion):
- **Epicenter**: Crater or large breach
- **Radial pattern**: Damage decreases with distance
- **Shockwave**: Windows blown out in radius
- **Debris scatter**: Rubble piles radiate outward
- **Example**: Artillery hit, bomb, grenade

**Fire damage** (burned):
- **Charred walls**: Walls intact but blackened
- **Roof collapse**: Burned through, caved in
- **Interior gutted**: All furniture destroyed
- **Windows**: Blown out from heat
- **Example**: Incendiary, prolonged fire

**Structural collapse** (age/neglect):
- **Sagging roof**: Partial collapse, uneven
- **Crumbling walls**: Irregular holes, not blast pattern
- **Vegetation**: Overgrown (bushes, vines)
- **Weathering**: Gradual deterioration
- **Example**: Abandoned building, old damage

**Combat damage** (small arms):
- **Bullet holes**: Wall pockmarks (cosmetic)
- **Broken windows**: Shattered glass
- **Chipped walls**: Small craters from impacts
- **Minimal structural**: Walls mostly intact
- **Example**: Firefight, sustained combat

**Heavy weapons damage** (tank/AT):
- **Large breaches**: 2×2 to 3×3 holes
- **Penetration**: Through-and-through holes
- **Blast secondary**: Surrounding damage
- **Structural**: May trigger collapse
- **Example**: Tank shell, anti-tank round

#### 9.5 Procedural Damage Generation

**Damage placement algorithm**:
```go
func applyBuildingDamage(building Building, damageLevel int, damageType DamageType, rng *rand.Rand) {
    switch damageLevel {
    case 1: // Light
        damageWindows(building, 0.2, 0.4, rng)
        damageDoors(building, 0.1, 0.2, rng)
        addWallPockmarks(building, 10, 30, rng)

    case 2: // Moderate
        damageWindows(building, 0.6, 0.8, rng)
        damageDoors(building, 0.3, 0.5, rng)
        addSmallBreaches(building, 1, 3, rng)
        damageInteriorFurniture(building, 0.3, 0.5, rng)

    case 3: // Heavy
        damageWindows(building, 0.9, 1.0, rng)
        damageDoors(building, 0.6, 0.8, rng)
        addLargeBreaches(building, 2, 5, rng)
        addPartialCollapse(building, 1, rng)
        damageInteriorFurniture(building, 0.6, 0.8, rng)

    case 4: // Severe
        damageWindows(building, 1.0, 1.0, rng)
        damageDoors(building, 0.9, 1.0, rng)
        addMassiveBreaches(building, 3, 6, rng)
        addRoofCollapse(building, 0.5, 0.8, rng)
        damageInteriorFurniture(building, 0.9, 1.0, rng)
        fillInteriorWithRubble(building, 0.3, 0.5, rng)

    case 5: // Ruined
        removeWalls(building, 0.6, 0.8, rng)
        addRoofCollapse(building, 1.0, 1.0, rng)
        fillInteriorWithRubble(building, 0.7, 0.9, rng)

    case 6: // Razed
        removeWalls(building, 0.9, 1.0, rng)
        replaceWithRubbleField(building, rng)
    }

    // Apply damage type modifiers
    if damageType == DamageTypeFire {
        charWalls(building)
        removeAllFurniture(building)
    } else if damageType == DamageTypeBlast {
        addCrater(building, rng)
        scatterDebris(building, rng)
    }
}
```

**Breach placement** (tactical considerations):
```go
func addSmallBreach(building Building, rng *rand.Rand) {
    // 1. Select wall section (not corner, not door/window)
    wall := selectSuitableWall(building, rng)

    // 2. Choose position along wall (avoid edges)
    pos := wall.RandomInteriorPosition(rng)

    // 3. Create 1×1 hole
    removeWallCell(building, pos)

    // 4. Add rubble pile at base (interior side)
    interiorPos := pos.ShiftInward(building)
    addObject(building, interiorPos, ObjectRubblePile)

    // 5. Reduce LOS opacity (can see through)
    setLOSOpacity(building, pos, 0.3)

    // 6. Mark as breach (tactical map trait)
    addTrait(building, pos, CellTraitBreach)
}
```

**Collapse simulation**:
```go
func addPartialCollapse(building Building, rng *rand.Rand) {
    // 1. Choose collapse type (corner, wall, roof)
    collapseType := rng.Intn(3)

    switch collapseType {
    case 0: // Corner collapse
        corner := selectCorner(building, rng)
        removeWalls(building, corner, 3, 5) // 3-5 cells
        removeRoof(building, corner, 2, 3)  // 2-3 cells
        addRubblePile(building, corner, 2, 3) // 2×2 or 3×3

    case 1: // Wall collapse
        wall := selectWall(building, rng)
        section := wall.RandomSection(4, 8) // 4-8 cells
        removeWalls(building, section)
        removeRoof(building, section.Above())
        addRubbleLine(building, section.Base())

    case 2: // Roof collapse
        room := selectRoom(building, rng)
        removeRoof(building, room.Bounds)
        fillWithRubble(building, room.Bounds, 0.6) // 60% filled
    }
}
```

#### 9.6 Tactical Impact of Damage

**Sniper nests** (small breaches):
- Soldier can position behind breach
- Good cover (wall on 3 sides)
- Clear sightline through hole
- Difficult to spot from outside
- **AI behavior**: Seek breaches for overwatch

**Alternative entry** (large breaches):
- Bypass defended doors
- Flank defenders
- Surprise attacks
- **AI behavior**: Recognize breaches as entry points

**Reduced cover** (collapsed sections):
- Building less defensible
- More exposure angles
- Easier to suppress defenders
- **AI behavior**: Avoid heavily damaged buildings

**Rubble cover** (ruins):
- Scattered ObjectRubblePile provides cover
- Open ground combat with obstacles
- No interior movement (filled with rubble)
- **AI behavior**: Use rubble as field cover

**Unstable structures** (severe damage):
- Risk of further collapse (future mechanic)
- Avoid prolonged occupation
- Quick raids only
- **AI behavior**: Prefer intact buildings

#### 9.7 Damage Visualization

**Wall breach rendering**:
- Remove wall tiles at breach position
- Add ObjectRubblePile at base (interior)
- Jagged edges (irregular hole shape)
- Exposed interior visible from outside

**Collapsed section rendering**:
- Remove wall and roof tiles
- Fill area with GroundRubbleHeavy
- Add ObjectRubblePile objects (3-6)
- Leaning wall fragments (angled tiles)

**Burned building rendering**:
- Darken wall color (charred)
- Remove roof (or darken)
- Interior: GroundRubbleLight (ash)
- No furniture (all destroyed)

**Ruined building rendering**:
- 20-40% of walls remain (fragments)
- No roof
- Interior: GroundRubbleHeavy + ObjectRubblePile
- Recognizable footprint outline

#### 9.8 Pre-Damaged Building Generation

**Map generation integration**:
```
1. Generate buildings (normal process)
2. Select subset for pre-damage (10-30% of buildings)
3. Assign damage levels (weighted toward light/moderate)
4. Apply damage (procedural)
5. Classify damaged buildings (BuildingQuality reduced)
```

**Damage distribution** (realistic battlefield):
- **10-15%**: Light damage (bullet holes, broken windows)
- **5-10%**: Moderate damage (small breaches, partial destruction)
- **3-5%**: Heavy damage (large breaches, major collapse)
- **1-2%**: Ruined (gutted, barely standing)
- **0-1%**: Razed (rubble field)

**Clustering** (battle lines):
- Damage concentrated in combat zones
- Gradient from heavy (front line) to light (rear)
- Intact buildings in safe areas
- Creates visual narrative of battle progression

**Contextual damage** (building type):
- **Bunkers**: Resistant, mostly light damage
- **Residential**: Moderate damage common
- **Industrial**: Heavy damage (large targets)
- **Wooden structures**: Fire damage, complete destruction

#### 9.9 Implementation Priority

**Phase 1: Basic breach system** (6-8 hours)
- Add small/medium/large breach generation
- Remove wall cells, add rubble
- Update LOS/movement mechanics
- Tactical map traits for breaches

**Phase 2: Damage levels** (8-10 hours)
- Implement 6-level damage taxonomy
- Procedural damage application
- Window/door damage
- Interior furniture destruction

**Phase 3: Collapsed sections** (6-8 hours)
- Corner/wall/roof collapse
- Rubble pile generation
- Structural instability visuals

**Phase 4: Pre-damaged buildings** (4-6 hours)
- Map generation integration
- Damage distribution algorithm
- Clustering and gradients
- Building quality adjustment

**Phase 5: Visual polish** (6-8 hours)
- Charred walls (fire damage)
- Jagged breach edges
- Leaning wall fragments
- Debris scatter

---

### 10. Implementation Strategy

#### 8.1 Phased Approach

**Phase 1: Interior Richness** (High Impact, Low Complexity)
1. Increase furniture density (30% → 60-80%)
2. Add new furniture objects (bed, sofa, desk, shelving, etc.)
3. Implement room-type-aware furniture placement
4. Add hallway generation (central corridor variant)
5. Increase room count (reduce BSP early-stop)

**Phase 2: Architectural Variety** (Medium Impact, Medium Complexity)
6. Implement L-shaped building footprints
7. Add larger building sizes (10×10 to 16×12)
8. Implement building type system (residential, industrial, etc.)
9. Type-specific interior layouts
10. Intelligent window/door placement (room-type-aware)

**Phase 3: Building Archetypes** (High Impact, High Complexity)
11. Warehouse archetype (large, open floor, sparse furniture)
12. Factory archetype (very large, multiple wings, machinery)
13. Bunker archetype (small, thick walls, firing slits)
14. Office archetype (hallway + many small rooms)
15. Compound structures (multiple buildings + perimeter)

**Phase 4: Advanced Features** (Polish, High Complexity)
16. T-shaped and U-shaped footprints
17. Exterior features (porches, loading docks, annexes)
18. Multi-story buildings (stairs, vertical dimension)
19. Destructible walls (breaching, structural damage)
20. Building damage states (pre-damaged, ruined variants)

#### 8.2 Technical Considerations

**Performance**:
- **Navmesh**: Larger buildings = more nav cells (optimize with spatial hashing)
- **LOS**: More walls = more raycasts (cache common queries)
- **Memory**: More furniture objects (use object pooling)
- **Generation time**: Complex layouts (acceptable if <100ms per building)

**Compatibility**:
- Building quality metrics already exist (leverage for type selection)
- Tactical map system handles interior traits (extend for hallways)
- Sector assignment works with any layout (no changes needed)
- Entry coordination adapts to door count (already flexible)

**Data structures**:
```go
// Extend existing rect-based footprint
type BuildingFootprint struct {
    Footprint     []rect          // Multiple rects for L/T/U shapes
    Type          BuildingType
    Rooms         []RoomInfo
    Hallways      []rect
    ExteriorDoors []DoorInfo
    InteriorDoors []DoorInfo
    Windows       []WindowInfo
}

type RoomInfo struct {
    Bounds     rect
    Type       RoomType  // living, kitchen, warehouse, etc.
    Furniture  []FurnitureInfo
}

type DoorInfo struct {
    X, Y       int
    Width      int  // 1 or 2 units
    IsExterior bool
    Facing     int  // 0=N, 1=E, 2=S, 3=W
}

type WindowInfo struct {
    X, Y       int
    Width      int
    Type       WindowType  // standard, large, slit
}
```

#### 8.3 Generation Algorithm Outline

**High-level flow**:
```
1. Select building type (weighted by biome/location)
2. Select size range (based on type)
3. Generate footprint shape (rectangle, L, T, U based on type)
4. Place exterior walls + doors + windows (type-aware)
5. Generate interior layout:
   a. If warehouse/barn: minimal subdivision (1-2 rooms)
   b. If residential/office: hallway + rooms
   c. If factory: multiple zones (production, office, storage)
6. Assign room types (based on building type + layout)
7. Furnish each room (type-specific furniture)
8. Stamp to TileMap (walls, doors, windows, furniture)
9. Compute building quality metrics
```

**Hallway generation** (residential/office):
```
1. Identify building orientation (longer axis)
2. Place central hallway (1 unit wide, runs length)
3. Partition remaining space into rooms (BSP)
4. Connect rooms to hallway (doors)
5. Some rooms may connect to each other (bedroom→bathroom)
```

**L-shape generation**:
```
1. Generate primary wing (larger rectangle)
2. Generate secondary wing (smaller, perpendicular)
3. Overlap at corner (1-2 unit overlap)
4. Merge footprints, resolve wall conflicts
5. Interior: treat as two zones, optional connecting door
```

---

### 9. Tactical Impact Analysis

#### 9.1 Gameplay Benefits

**Increased tactical depth**:
- **Room variety**: Different room types require different tactics
- **Hallway combat**: Fatal funnels, slice-the-pie, room clearing
- **Furniture cover**: More dynamic firefights, not just corner camping
- **Building types**: Players learn to recognize and adapt (warehouse = open, office = tight)
- **Entry options**: Multiple doors = more tactical choices

**Enhanced realism**:
- **Believable spaces**: Kitchens have kitchens, offices have desks
- **Architectural variety**: Not all buildings look the same
- **Clutter**: Real buildings are messy, not empty boxes
- **Functional design**: Windows where you'd expect them

**Strategic layer**:
- **Building prioritization**: Warehouse > house (size, position)
- **Type-specific tactics**: Clear office differently than warehouse
- **Compound assaults**: Multi-building objectives
- **Defensive planning**: Bunker = strongpoint, house = temporary cover

#### 9.2 AI Challenges

**Pathfinding**:
- **Hallways**: AI must navigate corridors, not just open rooms
- **Furniture density**: More obstacles, more complex paths
- **Multi-wing buildings**: AI must understand building connectivity

**Combat**:
- **Room clearing**: AI must check corners, not rush in
- **Hallway suppression**: AI should avoid standing in hallways
- **Furniture cover**: AI must use tables/crates, not just walls
- **Type awareness**: AI should recognize bunker = dangerous

**Coordination**:
- **Entry teams**: Already implemented, works with any layout
- **Sector assignment**: May need hallway-aware sectors
- **Position selection**: Already uses tactical map, should adapt

**Solutions**:
- Tactical map already handles interior traits (extend for hallways)
- Navmesh handles furniture automatically (obstacles)
- Building quality metrics guide AI selection (already implemented)
- Entry coordination is layout-agnostic (already flexible)

---

### 8. Visual & Rendering Considerations

**Current rendering** (`game.go:drawWorld`):
- Buildings drawn as filled rects (walls)
- Windows drawn as different color
- Furniture drawn as objects
- **Works for any layout** (no changes needed)

**Enhancements** (future):
- **Building textures**: Different wall materials (brick, concrete, wood)
- **Roof rendering**: Overhead view shows roof type
- **Damage states**: Cracked walls, broken windows
- **Interior lighting**: Darker interiors, windows provide light
- **Furniture sprites**: Distinct visuals for bed vs table vs desk

**No immediate changes required** — current system handles complexity.

---

## Prioritized Recommendations

### Immediate (High Impact, Low Effort)

1. **Increase furniture density** (30% → 60-80%)
   - Modify `furnishRoom()` probabilities
   - Add more table/crate clusters per room
   - **Effort**: 1-2 hours
   - **Impact**: Rooms feel lived-in, more cover

2. **Add new furniture objects** (bed, sofa, desk, shelving, barrel, pallet)
   - Extend `ObjectType` enum
   - Add to `furnishRoom()` placement logic
   - **Effort**: 2-3 hours
   - **Impact**: Visual variety, tactical variety

3. **Increase room count** (reduce BSP early-stop: 15% → 5%)
   - One-line change in `addBuildingWalls()`
   - **Effort**: 5 minutes
   - **Impact**: More complex interiors

4. **Larger buildings** (add 10×10 to 16×12 sizes)
   - Extend size pool in `generateBuildings()`
   - Limit count (1-2 per map)
   - **Effort**: 30 minutes
   - **Impact**: Dominant tactical positions

### Short-term (High Impact, Medium Effort)

5. **Building labels UI** (toggle-able text overlay)
   - Add `showBuildingLabels` flag
   - Render building type names above buildings
   - Keyboard shortcut ('L' key)
   - **Effort**: 2-3 hours
   - **Impact**: Immediate clarity, player orientation

6. **Scatter objects** (vehicles, containers, urban clutter)
   - Add new ObjectTypes: car, truck, dumpster, barrel, shipping container
   - Placement algorithm (context-aware: cars near houses, containers near warehouses)
   - **Effort**: 4-6 hours
   - **Impact**: Visual richness, tactical cover variety

7. **Hallway generation** (central corridor variant)
   - New function: `generateHallwayLayout()`
   - Alternative to pure BSP
   - **Effort**: 4-6 hours
   - **Impact**: Realistic layouts, new tactics

8. **L-shaped buildings**
   - New footprint generation function
   - Handle overlapping rectangles
   - **Effort**: 3-4 hours
   - **Impact**: Architectural variety

9. **Building type system** (residential, industrial, military)
   - Add `BuildingType` enum
   - Type-specific generation parameters
   - **Effort**: 6-8 hours
   - **Impact**: Meaningful building differences

10. **Room-type-aware furniture** (kitchen gets counters, bedroom gets bed)
    - Room type classification
    - Type-specific furniture placement
    - **Effort**: 4-6 hours
    - **Impact**: Realism, tactical variety

### Medium-term (Very High Impact, High Effort)

11. **Perimeter fencing** (wire, wooden, concrete walls)
    - Add fence ObjectTypes (chain-link, wooden, concrete, brick)
    - Fence generation algorithm (perimeter around buildings)
    - Gates aligned with doors
    - **Effort**: 5-7 hours
    - **Impact**: Compound realism, movement channelization

12. **Click-to-inspect building UI** (detailed info panel)
    - Mouse click detection on buildings
    - Info panel showing type, size, rooms, doors, windows, quality
    - Occupation status (friendly/enemy/unoccupied)
    - **Effort**: 6-8 hours
    - **Impact**: Player awareness, tactical planning

13. **Warehouse archetype** (large, open floor, minimal partitions)
    - Dedicated generation function
    - 12×10 to 16×12 size
    - Sparse furniture, crate clusters
    - **Effort**: 8-10 hours
    - **Impact**: New building class, dominant positions

14. **Factory archetype** (very large, multiple zones)
    - Compound-style generation
    - Production floor + offices + storage
    - **Effort**: 10-12 hours
    - **Impact**: Complex multi-zone combat

15. **Bunker archetype** (small, thick walls, firing slits)
    - Thick walls (2-cell width)
    - Narrow windows (firing slits)
    - Reinforced doors
    - **Effort**: 6-8 hours
    - **Impact**: Strongpoint gameplay

16. **Defensive fortifications** (trenches, barbed wire, sandbag lines)
    - Trench line generation (zigzag patterns)
    - Barbed wire entanglements (area denial)
    - Sandbag wall formations (U-shaped, L-shaped)
    - **Effort**: 8-10 hours
    - **Impact**: Military scenarios, defensive gameplay

17. **Intelligent window/door placement** (room-type-aware)
    - Windows based on room function
    - Multiple door types (front, back, side)
    - **Effort**: 6-8 hours
    - **Impact**: Realism, tactical options

### Long-term (Polish, Very High Effort)

18. **Compound generation** (residential, industrial, military complexes)
    - Multi-building clusters with perimeter walls
    - Context-aware scatter objects (industrial: containers, military: sandbags)
    - Guard posts at gates
    - **Effort**: 12-16 hours
    - **Impact**: Large-scale tactical objectives

19. **Building naming system** (procedural names)
    - Generate contextual names ("Warehouse A", "Bunker 3", "Small House")
    - Display in labels and info panel
    - **Effort**: 3-4 hours
    - **Impact**: Immersion, communication clarity

20. **T-shaped and U-shaped buildings**
    - Complex footprint generation
    - Courtyard mechanics
    - **Effort**: 8-10 hours
    - **Impact**: Architectural variety, tactical complexity





WARNING: DO NOT DO THIS YET, MULTI-STORY BUILDINGS WILL HAPPEN LATER IN DEVELOPMENT.
21. **Multi-story buildings** (stairs, vertical dimension)
    - Vertical pathfinding
    - Height advantage mechanics
    - **Effort**: 20-30 hours
    - **Impact**: Major gameplay expansion
WARNING: DO NOT DO THIS YET, MULTI-STORY BUILDINGS WILL HAPPEN LATER IN DEVELOPMENT.






22. **Exterior features** (porches, loading docks, balconies)
    - Attached structures to buildings
    - Type-specific (loading docks for warehouses)
    - **Effort**: 6-8 hours
    - **Impact**: Realism, tactical variety

23. **Destructible buildings** (breaching, structural damage)
    - Wall health system
    - Breach mechanics
    - Damage states
    - **Effort**: 15-20 hours
    - **Impact**: Dynamic combat, new tactics

---

## Conclusion

The current building system is **functional but simplistic**. Buildings serve as tactical cover but lack the **architectural richness**, **interior complexity**, **exterior context**, and **player feedback** that would make urban combat truly engaging.

**Key improvements**:

**Interior enhancements**:
- **More furniture** (60-80% density vs 30%)
- **More rooms** (8-15 vs 2-6 in large buildings)
- **Hallways** (realistic layouts, fatal funnel tactics)
- **Room types** (kitchens, bedrooms, offices with appropriate furniture)

**Architectural variety**:
- **Larger buildings** (warehouses, factories up to 20×14 units)
- **Building types** (residential, industrial, military, commercial archetypes)
- **L-shaped footprints** (architectural variety, courtyard formations)
- **Intelligent placement** (windows/doors based on room function)

**Exterior context**:
- **Perimeter fencing** (chain-link, wooden, concrete walls with gates)
- **Scatter objects** (vehicles, containers, street furniture, hay bales)
- **Defensive fortifications** (trenches, barbed wire, sandbag positions)
- **Compounds** (multi-building complexes with perimeter security)

**Player feedback & UI**:
- **Building labels** (toggle-able text showing building type)
- **Click-to-inspect** (detailed info panel with tactical metrics)
- **Building names** (procedural naming: "Warehouse A", "Bunker 3")
- **Visual indicators** (occupation status, building state overlays)

**Implementation path**:
1. **Quick wins**: Furniture density, new objects, building labels UI (1-3 hours each)
2. **Scatter & context**: Vehicles, containers, urban clutter (4-6 hours)
3. **Architectural depth**: Hallways, L-shapes, larger buildings (3-6 hours each)
4. **Building types**: Residential, industrial, military archetypes (6-8 hours)
5. **Perimeter & fortifications**: Fencing, trenches, barbed wire (5-10 hours)
6. **Advanced UI**: Click-to-inspect, building names (6-8 hours)
7. **Compounds**: Multi-building complexes with context (12-16 hours)
8. **Long-term polish**: Multi-story, destructible, T/U-shapes (15-30 hours each)

This transforms the battlefield from "rectangular boxes with some rooms scattered on grass" into a **rich, believable urban/rural environment** with:
- Buildings that feel lived-in and purposeful
- Exterior context that tells a story (fenced yards, parked cars, defensive positions)
- Clear player feedback about what they're looking at
- Tactical depth from furniture, hallways, fortifications, and building variety

The result is **meaningful urban combat** where buildings are objectives worth fighting for, not just abstract cover.
