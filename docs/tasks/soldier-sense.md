# Soldier Sense — Game Design Document

**Genre:** Emergent Behaviour Toy / Tactical Battlefield Simulation
**Engine:** Go + Ebiten v2
**Perspective:** Top-down 2D

---

## 1. Vision

A top-down tactical battlefield where red and blue soldiers fight autonomously. This is **not** an RTS — the player never clicks on a soldier or issues direct commands. Every soldier is a self-preserving agent whose behaviour emerges from simulated stats, senses, emotions, and chain-of-command dynamics.

The goal is a **realistic portrayal of frontline combat** where:

- Soldiers are motivated primarily by **self-preservation**, not game-logic heroism.
- Orders flow through a **chain of command** with realistic lag and delay.
- Every bullet, sound wave, and radio transmission is simulated as a discrete entity.
- Missions are aborted when casualties mount; CASEVAC is a first-class system.

---

## 2. Core Pillars

1. **Autonomy** — Every soldier is an agent. They think, feel, hesitate, and act on their own.
2. **Emergent Behaviour** — No scripted outcomes. Realistic results arise from the interaction of simple, honest subsystems.
3. **Indirect Control** — The player paints heatmaps (objectives, danger zones, preferred routes) that _guide_ troops. No direct unit selection or micro.
4. **Physical Simulation** — Bullets, sound, radio signals, line of sight — all modelled as concrete phenomena, not abstracted.

---

## 3. The Soldier Agent

Each soldier is a fully autonomous entity with the following attribute groups.

### 3.1 Physical Stats

| Stat | Description |
|---|---|
| **Fitness** | Stamina, sprint speed, carry capacity. Degrades with exertion and wounds. |
| **Wounds** | Per-body-region damage model (head, torso, limbs). Affects mobility, accuracy, consciousness. |
| **Fatigue** | Accumulates with movement, stress, lack of rest. Degrades all performance. |
| **Stance** | Standing / crouching / prone. Affects speed, profile, accuracy. |

### 3.2 Skill & Training

| Stat | Description |
|---|---|
| **Marksmanship** | Base accuracy, affected by stress, fatigue, stance, weapon. |
| **Fieldcraft** | Ability to find cover, move tactically, read terrain. |
| **Discipline** | Likelihood of following orders under pressure vs. breaking. |
| **First Aid** | Ability to stabilise wounded comrades. |

### 3.3 Experience & Psychology

| Stat | Description |
|---|---|
| **Experience** | Combat exposure. Veterans read situations faster, panic less. |
| **Morale** | Confidence in mission, leadership, survival. Affected by casualties, success, rest. |
| **Fear** | Acute stress response. Spikes under fire, suppression, seeing casualties. Drives fight-or-flight. |
| **Composure** | How well a soldier manages fear. High composure = acts despite fear. |
| **Bond** | Attachment to nearby squadmates. Casualties to bonded soldiers hit morale harder; also motivates rescue. |

### 3.4 Agent Decision Loop

Each tick, a soldier runs a prioritised behaviour tree (or utility AI scorer):

```
1. AM I DYING?          → Apply self-aid / call for medic / go limp
2. AM I IN MORTAL DANGER? → Seek immediate cover / flee
3. AM I SUPPRESSED?      → Stay in cover, return blind fire if brave enough
4. DO I HAVE ORDERS?     → Evaluate order vs. personal risk assessment
   a. Accept → Execute (move, fire, hold, etc.)
   b. Refuse → Freeze / retreat / cower
5. DO I SEE A THREAT?    → Engage / alert squad / take cover
6. DEFAULT               → Hold position / scan / rest
```

**Order compliance** is a function of: `discipline + morale + trust_in_leader - fear - fatigue`

If the result is below a threshold, the soldier **passively refuses** (freezes, delays) or **actively disobeys** (flees, retreats to rear).

---

## 4. Chain of Command & Orders

### 4.1 Hierarchy

```
Player (Battalion/Company HQ — off-map)
  └── Platoon Leader (on-map, per platoon)
        └── Section/Squad Leader (on-map, per squad)
              └── Soldiers (4-8 per squad)
```

### 4.2 Order Flow

1. **Player paints** a heatmap overlay (objective, avoid zone, route preference).
2. Heatmap is interpreted by the **Platoon Leader AI** into discrete orders (e.g. "Squad A advance to grid ref X along route Y").
3. Order is transmitted via **radio** (subject to range, noise, jamming, stress-induced miscommunication).
4. **Squad Leader** receives and relays to soldiers via **voice** (short range, affected by ambient noise).
5. Each soldier individually **decides** whether to comply (see §3.4).

### 4.3 Order Types

- **Advance** — Move toward objective grid area.
- **Hold** — Maintain current position, engage targets of opportunity.
- **Withdraw** — Pull back to specified area.
- **Suppress** — Lay down fire on a zone (no specific target required).
- **CASEVAC** — Retrieve and evacuate a wounded soldier.
- **Regroup** — Rally at squad leader's position.
- **Abort Mission** — Full withdrawal triggered by casualty threshold.

---

## 5. Senses & Perception

### 5.1 Vision

- **Field of View cone** per soldier (direction + arc width).
- Blocked by buildings and terrain (existing LOS system extended).
- Range degrades with fatigue, wounds, smoke/dust.
- Identified contacts vs. detected movement (two-tier awareness).

### 5.2 Sound Propagation

- Every gunshot, explosion, footstep, and voice command emits a **sound event** at a world position with a **volume**.
- Sound propagates outward, attenuated by distance (inverse-square) and occluded/reflected by buildings.
- Soldiers hear sounds within their perceptual threshold; louder = more precise direction estimate.
- Suppressive fire and explosions raise **ambient noise**, masking quiet sounds (footsteps, voice commands, radio).

### 5.3 Radio

- Radio messages are discrete packets with a **sender**, **content**, and **signal strength**.
- Can be garbled by: low signal, high ambient noise, receiver stress/distraction.
- Radio is the only way the player's intent (heatmaps → orders) reaches front-line troops.
- If radio operator is killed or radio destroyed, squad loses contact with command.

---

## 6. Ballistics & Combat

### 6.1 Bullets

- Each round fired is a **discrete projectile** with position, velocity vector, and energy.
- Bullets travel in straight lines (no drop at these ranges, but could add later).
- Hit detection: ray-cast per tick against soldier colliders and terrain.
- Penetration / ricochet possible in later phases.

### 6.2 Firing

- Accuracy is a cone of deviation influenced by: `marksmanship + stance + fatigue + fear + weapon_accuracy`.
- Rate of fire governed by weapon stats and soldier's fire discipline.
- Ammunition is finite and tracked per magazine.

### 6.3 Suppression

- Near-miss bullets within a radius of a soldier generate **suppression stress**.
- Suppression accumulates and decays over time.
- High suppression forces soldiers into cover, degrades accuracy, and can trigger panic/flight.
- This is the **key emergent mechanic** — firefights should look like real suppression exchanges, not FPS aim-duels.

### 6.4 Wounds & CASEVAC

- Hits apply damage to a body region based on projectile angle.
- Wound severity: scratch → moderate → severe → critical → fatal.
- Wounded soldiers cry out (sound event), attracting medics and demoralising nearby troops.
- CASEVAC: a buddy pair must drag/carry the casualty to a collection point. They are slow and vulnerable.
- **Mission abort** triggers when casualties exceed a configurable threshold (e.g. 30% of force).

---

## 7. Player Interaction — Heatmap Painting

The player's **only** interface with the battlefield is painting overlays onto the grid.

### 7.1 Overlay Types

| Layer | Colour | Effect on AI |
|---|---|---|
| **Objective** | Green | Attracts advance orders toward painted cells. |
| **Danger Zone** | Red | AI avoids or approaches with extreme caution. |
| **Preferred Route** | Blue | Pathfinding cost reduced along painted cells. |
| **Suppression Target** | Orange | Soldiers directed to lay fire into this zone. |
| **Rally Point** | White | Regroup / CASEVAC collection point. |

### 7.2 Interaction Delay

- Heatmap changes are **not instant**. They propagate through the command chain (§4.2).
- Delay = radio transmission time + leader decision time + relay to soldiers.
- Under stress / broken comms, orders may arrive **late, garbled, or not at all**.

---

## 8. Map & Terrain

- Grid-based world (current 16px cell size).
- **Buildings** — hard cover, block LOS and sound, impassable.
- **Walls / Fences** — partial cover, block LOS, traversable (slow).
- **Open ground** — fast movement, no cover, high exposure.
- **Craters / Ditches** — provide cover when prone, slightly slow movement.
- **Roads** — faster movement, no cover.
- Maps are defined as data files in `design/levels/`.

---

## 9. Technical Architecture

### 9.1 Current State (what exists)

| Component | File | Status |
|---|---|---|
| Game loop & rendering | `internal/game/game.go` | ✅ Basic |
| Soldier entity & movement | `internal/game/soldier.go` | ✅ Pathfinding patrol |
| A* nav grid | `internal/game/navmesh.go` | ✅ Working |
| Line of sight | `internal/game/los.go` | ✅ Ray-vs-AABB |
| Entry point | `cmd/game/main.go` | ✅ Ebiten bootstrap |

### 9.2 Planned Package Structure

```
internal/
  game/           — Top-level game loop, rendering, input
  agent/          — Soldier AI: decision tree, order evaluation, personality
  sense/          — Vision, hearing, radio perception
  ballistics/     — Projectile simulation, hit detection, suppression
  command/        — Chain of command, order types, radio transmission
  medical/        — Wound model, CASEVAC, mission abort logic
  intel/           — Intelligence heatmap store, per-team layers, decay, leader queries
  heatmap/        — Player overlay system, painting input, propagation
  terrain/        — Map loading, terrain types, cover values
  physics/        — Sound propagation, signal attenuation
```

### 9.3 ECS-ish or Struct-of-Agents?

Given that every soldier is a rich autonomous agent (not a swarm of identical particles), a **struct-of-agents** approach is more natural than a pure ECS. Each `Soldier` struct owns its stats, perception state, and decision context. Systems (ballistics, sound, command) operate over slices of agents each tick.

---

## 10. Development Phases

### Phase 0 — Foundation (CURRENT)
- [x] Ebiten game loop
- [x] Grid rendering
- [x] Buildings & collision
- [x] A* pathfinding
- [x] Basic soldier movement (patrol)
- [x] Line of sight (ray-vs-AABB)

### Phase 1 — Soldier Agent Core
- [x] Soldier stat model (fitness, skill, experience, fear, morale)
- [x] Stance system (standing, crouching, prone)
- [x] Vision cone (directional FOV, not omniscient LOS)
- [x] Basic decision loop (idle → move → take cover)
- [x] Squad grouping (soldiers belong to a squad with a leader)

### Phase 2 — Ballistics & Combat
- [ ] Discrete bullet simulation
- [ ] Firing mechanics (accuracy cone, rate of fire, ammo)
- [ ] Hit detection (bullet vs. soldier collider)
- [ ] Wound model (body regions, severity levels)
- [ ] Suppression system (near-miss stress accumulation)
- [ ] Death and incapacitation

### Phase 3 — Sound & Perception
- [ ] Sound event system (gunshots, footsteps, voices)
- [ ] Sound propagation with distance attenuation
- [ ] Building occlusion of sound
- [ ] Soldiers react to heard sounds (turn toward, alert, panic)
- [ ] Ambient noise level tracking

### Phase 4 — Chain of Command
- [ ] Squad / platoon hierarchy data model
- [ ] Order types (advance, hold, withdraw, suppress, CASEVAC, regroup)
- [ ] Leader AI: interprets situation → issues orders
- [ ] Order compliance check per soldier
- [ ] Radio system (signal, garbling, loss of comms)

### Phase 4.5 — Intelligence Heatmaps
- [ ] `HeatLayer` / `IntelMap` / `IntelStore` data structures (see `design/systems/intelligence-heatmaps.md`)
- [ ] Write `ContactHeat` from vision contacts each tick
- [ ] Write `RecentContactHeat` from blackboard threat facts
- [ ] Write `FriendlyPresenceHeat` and `DangerZoneHeat` from soldier state
- [ ] Initialise and clear `UnexploredHeat` as cells are seen
- [ ] Per-tick decay pass on all layers
- [ ] Leader query functions: `SumInRadius`, `Centroid`, `SampleAt`
- [ ] Replace crude threat-count logic in Squad Think with map queries
- [ ] `DangerZoneHeat` feeds A* path cost modifier
- [ ] Debug rendering: toggleable per-layer colour wash overlays

### Phase 5 — Player Heatmap Interface
- [ ] Mouse-paint heatmap overlays on grid
- [ ] Multiple overlay layers (objective, danger, route, suppress, rally)
- [ ] Player layers write into dedicated `IntelMap` layers (same infrastructure as Phase 4.5)
- [ ] Heatmap → platoon leader AI interpretation
- [ ] Propagation delay through command chain
- [ ] Visual feedback (translucent colour washes on grid)

### Phase 6 — CASEVAC & Mission Logic
- [ ] Wounded soldier extraction (buddy carry)
- [ ] Collection point / rally point mechanics
- [ ] Casualty threshold → mission abort trigger
- [ ] Withdrawal behaviour under abort conditions

### Phase 7 — Polish & Emergence
- [ ] Panic cascades (one soldier fleeing triggers neighbours)
- [ ] Veteran vs. green troop behaviour differences
- [ ] Morale recovery during lulls
- [ ] Battlefield sounds ambience (for the player's benefit)
- [ ] Replay / observation mode
- [ ] Map editor or level data format

---

## 11. Key Design Principles

1. **No magic knowledge.** A soldier can only act on what they personally perceive or are told.
2. **No perfect compliance.** Orders are requests filtered through a human (simulated) psyche.
3. **No health bars.** Wounds are anatomical and consequential. A leg wound slows you. A chest wound kills you.
4. **No teleporting information.** Every piece of intelligence travels through a physical medium (sight, sound, radio) with latency and error.
5. **Suppression > Accuracy.** Firefights are won by fire superiority and manoeuvre, not by who clicks heads faster (there is no clicking at all).

---

## 12. Open Questions

- **Scale:** How many soldiers per side? 12v12 (platoon)? 40v40 (company)? Performance will guide this.
- **Time scale:** Real-time 1:1? Or accelerated? Probably 1:1 with a fast-forward option.
- **Terrain variety:** Urban only (buildings) or also rural (trees, hedgerows, hills)?
- **Player faction:** Always commanding one side, or an observer of both?
- **Persistence:** Do soldiers carry stats across missions? Campaign mode vs. sandbox?
