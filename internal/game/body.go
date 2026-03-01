package game

import "math/rand"

// ---------------------------------------------------------------------------
// Body Regions
// ---------------------------------------------------------------------------

// BodyRegion identifies an anatomical zone on a soldier.
type BodyRegion int

const (
	RegionHead     BodyRegion = iota // 0
	RegionNeck                       // 1
	RegionTorso                      // 2
	RegionArmLeft                    // 3
	RegionArmRight                   // 4
	RegionAbdomen                    // 5
	RegionLegLeft                    // 6
	RegionLegRight                   // 7
	regionCount                      // sentinel — always last
)

func (r BodyRegion) String() string {
	switch r {
	case RegionHead:
		return "head"
	case RegionNeck:
		return "neck"
	case RegionTorso:
		return "torso"
	case RegionArmLeft:
		return "arm(L)"
	case RegionArmRight:
		return "arm(R)"
	case RegionAbdomen:
		return "abdomen"
	case RegionLegLeft:
		return "leg(L)"
	case RegionLegRight:
		return "leg(R)"
	default:
		return "unknown"
	}
}

// Per-region maximum HP.
var regionMaxHP = [regionCount]float64{
	RegionHead:     20,
	RegionNeck:     15,
	RegionTorso:    50,
	RegionArmLeft:  25,
	RegionArmRight: 25,
	RegionAbdomen:  35,
	RegionLegLeft:  30,
	RegionLegRight: 30,
}

// regionLethalityBias shifts severity determination upward for lethal zones.
var regionLethalityBias = [regionCount]float64{
	RegionHead:     0.35,
	RegionNeck:     0.30,
	RegionTorso:    0.10,
	RegionArmLeft:  -0.10,
	RegionArmRight: -0.10,
	RegionAbdomen:  0.05,
	RegionLegLeft:  -0.05,
	RegionLegRight: -0.05,
}

// ---------------------------------------------------------------------------
// Stance-adjusted hit-region weight tables
// ---------------------------------------------------------------------------

// stanceHitWeights[stance] gives the probability distribution of a confirmed
// hit landing on each body region. Sums to 1.0 per stance.
var stanceHitWeights = [3][regionCount]float64{
	// StanceStanding
	{0.08, 0.04, 0.38, 0.10, 0.10, 0.15, 0.075, 0.075},
	// StanceCrouching
	{0.10, 0.05, 0.35, 0.11, 0.11, 0.13, 0.075, 0.075},
	// StanceProne
	{0.18, 0.06, 0.10, 0.15, 0.15, 0.06, 0.15, 0.15},
}

// ---------------------------------------------------------------------------
// Wound Severity
// ---------------------------------------------------------------------------

// WoundSeverity classifies how bad a single wound is.
type WoundSeverity int

const (
	WoundMinor    WoundSeverity = iota // graze / fragment
	WoundModerate                      // through-and-through or lodged
	WoundSevere                        // major vessel / bone damage
	WoundCritical                      // catastrophic — immediate life threat
)

func (ws WoundSeverity) String() string {
	switch ws {
	case WoundMinor:
		return "minor"
	case WoundModerate:
		return "moderate"
	case WoundSevere:
		return "severe"
	case WoundCritical:
		return "critical"
	default:
		return "unknown"
	}
}

// Base bleed rates per severity (region HP / tick).
// Reduced 10x from original to give time for casualty response.
// At 60 ticks/sec: critical ~5-10min to bleed out, severe ~15-20min.
var severityBleedRate = [4]float64{
	WoundMinor:    0.005,
	WoundModerate: 0.020,
	WoundSevere:   0.060,
	WoundCritical: 0.150,
}

// Base pain per severity (0–1 contribution).
var severityPain = [4]float64{
	WoundMinor:    0.08,
	WoundModerate: 0.20,
	WoundSevere:   0.40,
	WoundCritical: 0.65,
}

// ---------------------------------------------------------------------------
// Wound
// ---------------------------------------------------------------------------

// Wound represents a single ballistic injury on a body region.
type Wound struct {
	Region        BodyRegion
	Severity      WoundSeverity
	BleedRate     float64 // HP/tick draining from the region
	Pain          float64 // 0–1 pain contribution
	Treated       bool    // true once bleeding is controlled
	TreatedTick   int
	TickInflicted int
}

// ---------------------------------------------------------------------------
// BodyMap
// ---------------------------------------------------------------------------

// BodyMap holds per-region health and the wound list for one soldier.
type BodyMap struct {
	HP    [regionCount]float64
	MaxHP [regionCount]float64

	Wounds []Wound

	// BloodVolume represents circulating blood as a fraction (1.0 = full).
	BloodVolume float64
}

// NewBodyMap returns a fully healthy body with no wounds.
func NewBodyMap() BodyMap {
	bm := BodyMap{BloodVolume: 1.0}
	for i := 0; i < int(regionCount); i++ {
		bm.MaxHP[i] = regionMaxHP[i]
		bm.HP[i] = regionMaxHP[i]
	}
	return bm
}

// bleedToBloodFactor converts region-HP bleed into blood-volume loss.
// Total region HP pool is ~230; blood volume is 0–1. Scale so that
// draining 100 region-HP ≈ 0.45 blood volume.
const bleedToBloodFactor = 0.0045

// ---------------------------------------------------------------------------
// ApplyHit — the core hit-resolution entry point
// ---------------------------------------------------------------------------

// ApplyHit resolves a bullet impact. It rolls a body region from the stance-
// weighted table (with optional cover masking), creates a Wound, reduces
// region HP, and returns the wound plus whether the soldier died instantly.
func (bm *BodyMap) ApplyHit(damage float64, stance Stance, coverMask [regionCount]float64, tick int, rng *rand.Rand) (Wound, bool) {
	region := rollHitRegion(stance, coverMask, rng)
	severity := determineSeverity(damage, region, rng)

	w := Wound{
		Region:        region,
		Severity:      severity,
		BleedRate:     severityBleedRate[severity],
		Pain:          severityPain[severity],
		TickInflicted: tick,
	}
	bm.Wounds = append(bm.Wounds, w)

	bm.HP[region] -= damage
	if bm.HP[region] < 0 {
		bm.HP[region] = 0
	}

	// Instant death: head or neck reduced to zero.
	instantDeath := (region == RegionHead || region == RegionNeck) && bm.HP[region] <= 0
	return w, instantDeath
}

// rollHitRegion picks a body region from the stance-adjusted weight table,
// modified by cover masking. Cover mask values are 0–1 per region (0 = fully
// exposed, 1 = fully shielded).
func rollHitRegion(stance Stance, coverMask [regionCount]float64, rng *rand.Rand) BodyRegion {
	weights := stanceHitWeights[stance]
	var exposed [regionCount]float64
	total := 0.0
	for i := 0; i < int(regionCount); i++ {
		exposed[i] = weights[i] * (1.0 - coverMask[i])
		if exposed[i] < 0 {
			exposed[i] = 0
		}
		total += exposed[i]
	}
	if total <= 0 {
		// All regions masked — shouldn't happen (caller should miss), default to torso.
		return RegionTorso
	}

	roll := rng.Float64() * total
	cumulative := 0.0
	for i := 0; i < int(regionCount); i++ {
		cumulative += exposed[i]
		if roll < cumulative {
			return BodyRegion(i)
		}
	}
	return RegionTorso // fallback
}

// determineSeverity rolls wound severity from damage, region lethality, and randomness.
func determineSeverity(damage float64, region BodyRegion, rng *rand.Rand) WoundSeverity {
	maxHP := regionMaxHP[region]
	if maxHP <= 0 {
		maxHP = 1
	}
	score := (damage / maxHP) + regionLethalityBias[region] + (rng.Float64()*0.20 - 0.10)
	switch {
	case score < 0.25:
		return WoundMinor
	case score < 0.50:
		return WoundModerate
	case score < 0.75:
		return WoundSevere
	default:
		return WoundCritical
	}
}

// ---------------------------------------------------------------------------
// TickBleed — per-tick wound bleeding and status derivation
// ---------------------------------------------------------------------------

// TickBleed advances bleeding for all untreated wounds, drains blood volume,
// and returns updated status flags.
func (bm *BodyMap) TickBleed() (ambulatory, conscious, alive bool) {
	alive = true
	conscious = true
	ambulatory = true

	// Drain blood from untreated wounds.
	for i := range bm.Wounds {
		w := &bm.Wounds[i]
		if w.Treated {
			continue
		}
		// Bleed drains the region HP further and reduces blood volume.
		bm.HP[w.Region] -= w.BleedRate
		if bm.HP[w.Region] < 0 {
			bm.HP[w.Region] = 0
		}
		bm.BloodVolume -= w.BleedRate * bleedToBloodFactor
	}
	if bm.BloodVolume < 0 {
		bm.BloodVolume = 0
	}

	// Check region failures.
	if bm.HP[RegionHead] <= 0 || bm.HP[RegionNeck] <= 0 {
		alive = false
		conscious = false
		ambulatory = false
		return
	}
	if bm.HP[RegionTorso] <= 0 {
		conscious = false
		ambulatory = false
	}
	if bm.HP[RegionAbdomen] <= 0 {
		conscious = false
		ambulatory = false
	}

	// Blood volume effects.
	if bm.BloodVolume <= 0 {
		alive = false
		conscious = false
		ambulatory = false
		return
	}
	if bm.BloodVolume <= 0.20 {
		conscious = false
		ambulatory = false
	}
	if bm.BloodVolume <= 0.40 {
		ambulatory = false
	}

	// Leg check for ambulatory status.
	if bm.HP[RegionLegLeft] <= 0 || bm.HP[RegionLegRight] <= 0 {
		ambulatory = false
	}

	if !conscious {
		ambulatory = false
	}
	return
}

// ---------------------------------------------------------------------------
// Functional degradation helpers
// ---------------------------------------------------------------------------

// TotalPain returns the summed pain across all active wounds (capped at 1.0).
func (bm *BodyMap) TotalPain() float64 {
	total := 0.0
	for i := range bm.Wounds {
		if !bm.Wounds[i].Treated {
			total += bm.Wounds[i].Pain
		} else {
			// Treated wounds still have residual pain.
			total += bm.Wounds[i].Pain * 0.3
		}
	}
	if total > 1.0 {
		total = 1.0
	}
	return total
}

// MobilityMul returns the combined speed multiplier from leg/abdomen wounds.
func (bm *BodyMap) MobilityMul() float64 {
	mul := 1.0

	// Leg damage.
	leftFrac := bm.HP[RegionLegLeft] / bm.MaxHP[RegionLegLeft]
	rightFrac := bm.HP[RegionLegRight] / bm.MaxHP[RegionLegRight]
	if bm.HP[RegionLegLeft] <= 0 && bm.HP[RegionLegRight] <= 0 {
		return 0.0 // immobile
	}
	if bm.HP[RegionLegLeft] <= 0 {
		mul *= 0.15 // crawl only
	} else if leftFrac < 0.6 {
		mul *= 0.50
	}
	if bm.HP[RegionLegRight] <= 0 {
		mul *= 0.15
	} else if rightFrac < 0.6 {
		mul *= 0.50
	}

	// Abdomen damage.
	abdFrac := bm.HP[RegionAbdomen] / bm.MaxHP[RegionAbdomen]
	if abdFrac < 0.4 {
		mul *= 0.70
	}

	// Blood volume penalty.
	if bm.BloodVolume < 0.80 {
		mul *= (0.50 + 0.50*clamp01((bm.BloodVolume-0.20)/0.60))
	}

	if mul < 0 {
		mul = 0
	}
	return mul
}

// AccuracyMul returns the accuracy multiplier from arm/head wounds.
func (bm *BodyMap) AccuracyMul() float64 {
	mul := 1.0

	// Arm damage — assume right-dominant for simplicity.
	rightFrac := bm.HP[RegionArmRight] / bm.MaxHP[RegionArmRight]
	leftFrac := bm.HP[RegionArmLeft] / bm.MaxHP[RegionArmLeft]
	if bm.HP[RegionArmRight] <= 0 {
		mul *= 0.30 // dominant arm disabled
	} else if rightFrac < 0.6 {
		mul *= 0.60
	}
	if bm.HP[RegionArmLeft] <= 0 {
		mul *= 0.70 // off-hand disabled
	} else if leftFrac < 0.6 {
		mul *= 0.85
	}

	// Head wound (non-fatal): disorientation.
	headFrac := bm.HP[RegionHead] / bm.MaxHP[RegionHead]
	if headFrac < 0.7 && headFrac > 0 {
		mul *= 0.75
	}

	// Blood volume penalty.
	if bm.BloodVolume < 0.80 {
		mul *= (0.60 + 0.40*clamp01((bm.BloodVolume-0.20)/0.60))
	}

	if mul < 0 {
		mul = 0
	}
	return mul
}

// CanSelfAid returns true if the soldier has at least one functional arm
// and sufficient blood volume to act.
func (bm *BodyMap) CanSelfAid(conscious bool) bool {
	if !conscious {
		return false
	}
	if bm.BloodVolume <= 0.40 {
		return false
	}
	return bm.HP[RegionArmLeft] > 0 || bm.HP[RegionArmRight] > 0
}

// HasUntreatedWounds returns true if any wound is still bleeding.
func (bm *BodyMap) HasUntreatedWounds() bool {
	for i := range bm.Wounds {
		if !bm.Wounds[i].Treated {
			return true
		}
	}
	return false
}

// WoundCount returns the total number of wounds.
func (bm *BodyMap) WoundCount() int {
	return len(bm.Wounds)
}

// WorstUntreatedWound returns the untreated wound with the highest bleed rate,
// or nil if all wounds are treated.
func (bm *BodyMap) WorstUntreatedWound() *Wound {
	var worst *Wound
	for i := range bm.Wounds {
		w := &bm.Wounds[i]
		if w.Treated {
			continue
		}
		if worst == nil || w.BleedRate > worst.BleedRate {
			worst = w
		}
	}
	return worst
}

// HealthFraction returns a 0–1 value representing overall body integrity,
// computed as the weighted average of all region HP fractions. This is the
// compatibility shim that replaces the old scalar health / soldierMaxHP.
func (bm *BodyMap) HealthFraction() float64 {
	sum := 0.0
	maxSum := 0.0
	for i := 0; i < int(regionCount); i++ {
		sum += bm.HP[i]
		maxSum += bm.MaxHP[i]
	}
	if maxSum <= 0 {
		return 0
	}
	return sum / maxSum
}

// IsInjured returns true if any region has taken damage.
func (bm *BodyMap) IsInjured() bool {
	for i := 0; i < int(regionCount); i++ {
		if bm.HP[i] < bm.MaxHP[i] {
			return true
		}
	}
	return false
}
