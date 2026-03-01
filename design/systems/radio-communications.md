# Radio Communications System

**Status:** Planning Draft
**Priority:** Critical — enables realistic chain-of-command uncertainty
**Scope:** Squad member ↔ Squad leader tactical communications

---

## 1. Design Intent

Create a fully simulated radio layer where information flow is **physical, delayed, lossy, and emotional**.

This system must ensure:

- Soldiers only know what they personally perceive unless informed by comms.
- Reporting and command flow can fail at both send and receive stages.
- Soldiers avoid talking over each other using channel arbitration.
- Leaders build their tactical picture from incoming reports, not omniscience.
- Missing replies are meaningful (injured, suppressed, dead, disconnected).
- Communication itself affects stress, morale, and decisions.

---

## 2. Core Behaviors Required

### 2.1 Upward Reports (Member → Leader)

When appropriate, soldiers report:

- Enemy contact spotted (position, count estimate, distance, direction)
- Own status (healthy, wounded, pinned, low confidence, panic)
- Casualty sightings (friendly down, officer down)
- Ammunition / combat readiness state (future-ready hook)
- Outnumbered / fear threshold crossed
- Unable to execute order + reason

### 2.2 Downward Requests/Orders (Leader → Member)

Leader can:

- Request individual status checks
- Request contact confirmation from specific members
- Issue behavior changes (hold, move, fallback, regroup, suppress)
- Appoint replacement command when leader/officer down is reported

### 2.3 Silence Has Meaning

If leader requests status and no reply arrives in expected window:

- Leader marks member as **unresponsive** (not immediately dead)
- Confidence of member being combat-effective decays over time
- Repeated misses increase leader stress and defensive bias
- If later visual/radio evidence confirms KIA, update confidence to certainty

---

## 3. System Architecture

## 3.1 New Tick Stage

Extend cognition flow with a communication stage:

1. Sense
2. Believe
3. Squad Think (leader)
4. Individual Think
5. **Comms Plan** (decide what to transmit this tick)
6. **Comms Resolve** (arbitration + send/receive outcomes)
7. Act

This keeps communication explicit and avoids hidden side effects.

### 3.2 Core Components

- `RadioDevice` (per soldier): range/quality/battery-like reliability knobs (battery optional)
- `RadioMessage` (structured payload)
- `RadioChannel` (per squad net): queue + anti-talk-over policy
- `CommsIntent` (soldier's desired outgoing transmission)
- `CommsInbox` (received, decoded messages awaiting belief updates)
- `LeaderCommsTracker` (pending requests, expected responders, timeout state)

---

## 4. Data Model Proposal

## 4.1 Message Envelope

```go
type RadioMessage struct {
    ID              uint64
    TickCreated     int
    NetID           int

    SenderID        int
    IntendedReceiverID int // -1 for squad-wide

    Type            RadioMessageType
    Priority        RadioPriority

    Payload         RadioPayload

    TxQuality       float32 // sender-side quality estimate
    RxQuality       float32 // receiver-side sampled quality
    DecodeConfidence float32

    Attempts        int
    AckRequired     bool
}
```

### 4.2 Message Types

- `ContactReport`
- `StatusReport`
- `CasualtyReport`
- `OfficerDownReport`
- `FearReport`
- `OrderMessage`
- `StatusRequest`
- `Ack`

### 4.3 Payload Examples

`ContactReportPayload`:

- Estimated enemy count (range + confidence)
- Approx distance band
- Bearing/direction
- Sender stress at time of report
- Last visual tick

`StatusReportPayload`:

- Alive/injured/suppressed/pinned
- Pain intensity (if wounded)
- Fear band
- Mobility impairment

---

## 5. Communication Reliability Model

Transmission succeeds only if **both** stages pass:

1. `canSend` (sender able to transmit)
2. `canReceive` (receiver able to decode)

### 5.1 Send Failure Factors

- Radio damaged / unavailable
- Fear/panic too high to coherently speak
- Suppression interrupts transmission
- Channel already occupied by higher priority traffic
- Sender currently performing blocking action (e.g., immediate self-preservation)

### 5.2 Receive Failure Factors

- Distance attenuation / terrain occlusion
- Ambient noise and local combat intensity
- Receiver cognitive overload (fear/stress/fatigue)
- Simultaneous collisions / partial overlap
- Message degradation and semantic corruption

### 5.3 Partial Decode / Garble

Messages may arrive as:

- `Clear` (full payload confidence)
- `Partial` (some fields unknown / low confidence)
- `Corrupted` (wrong count or wrong distance bucket)
- `Dropped` (no usable message)

Misheard info is allowed and should populate beliefs with lower confidence.

---

## 6. Anti-Talk-Over Protocol (Channel Arbitration)

Each squad net has a simple arbitration model:

- One active speaker slot per short transmission window
- Priority queue by message criticality:
  1. Officer down / distress
  2. Contact and casualty reports
  3. Explicit leader requests/replies
  4. Routine status chatter
- Randomized short backoff for same-priority collisions
- Max retries per message to avoid infinite channel lock

This creates natural delays while reducing chaotic overlap.

---

## 7. Leader Mental Map Integration

Leader should not directly trust all reports equally.

### 7.1 Report Ingestion

Incoming messages write/update leader blackboard facts:

- `KnownThreat` (source: radio report)
- `MemberStatusFact`
- `SquadCasualtyFact`
- `CommandContinuityFact`

Each fact stores:

- Source soldier ID
- Message confidence
- Time since report
- Corroboration count (visual + other radios)

### 7.2 Confidence Fusion

Leader confidence increases when:

- Multiple members report similar contact
- Leader also sees matching evidence

Leader confidence decreases when:

- Reports conflict
- Reporter is high stress / low composure
- Report age exceeds freshness threshold

### 7.3 Decision Effects

Low-confidence intel should bias leader toward safer intent:

- Prefer hold/regroup over aggressive advance
- Increase defensive stance if many unresponsive members
- Trigger re-check requests when information is stale

---

## 8. Status Requests, Non-Reply, and Emotional Consequences

### 8.1 Request-Reply Flow

1. Leader sends `StatusRequest(member)`
2. Tracker starts timeout window
3. Member may reply with:
   - `StatusReport(healthy)`
   - `StatusReport(injured + pain)`
   - No reply

### 8.2 No Reply Handling

On timeout:

- Mark member `Unresponsive`
- Raise leader stress incrementally
- Add uncertainty penalty to squad effectiveness estimate
- Optionally trigger nearby member query for confirmation

### 8.3 Injured Reply Effects

If injured member replies with high pain/fear:

- Leader receives morale/stress hit
- Decision policy shifts toward cover/defense/CASEVAC-like behavior
- Nearby bonded soldiers can receive second-order stress effects (future extension)

---

## 9. Officer Down and Command Succession

When `OfficerDownReport` is received (or leader visually confirms):

- Squad enters short command-latency state
- Candidate replacement selected by rank/discipline/composure rules
- New leader begins issuing requests/orders after handover delay
- During delay, members rely on personal survival priorities

If report is garbled, squad may temporarily split due to uncertain command authority.

---

## 10. Visual and UX Presentation

### 10.1 World Overlay Link Arc

Render a **semi-transparent green grainy arc line** between sender and intended receiver during radio activity.

Suggested visual style:

- Color: military console green (`#4CFF88` baseline)
- Alpha: ~0.30–0.55 depending on signal quality
- Grain/noise: animated dither along arc to imply analog/static transmission
- Arc jitter: subtle wobble under poor signal

### 10.2 Radio Console Feed

Add green terminal-style message panel for comms events:

- `TX` / `RX` prefixed lines
- Timestamp/tick, sender, receiver, message type
- Decode quality indicator (`CLEAR`, `PARTIAL`, `GARBLED`, `DROP`)
- Optional compact payload summary

Example entries:

- `[3124] TX ALPHA-2 -> ALPHA-LDR CONTACT x3 120m E (Q:0.82)`
- `[3125] RX ALPHA-LDR <- ALPHA-2 CONTACT x? ~far ?E (GARBLED Q:0.41)`
- `[3131] TX ALPHA-LDR -> ALPHA-4 STATUS?`
- `[3138] TIMEOUT ALPHA-4 no reply`

---

## 11. Telemetry, Debugging, and Headless Reporting

Track comms metrics for balancing and AAR:

- Messages attempted / sent / received / dropped
- Collision count and average channel wait
- Garble rate by stress and distance bands
- Status request timeout frequency
- False belief rate from corrupted reports
- Leadership stress deltas attributable to comms events

Log key comms events in simulation reports so behavior shifts are explainable.

---

## 12. Phased Implementation Plan

### Phase A — Message Skeleton + Console Visibility

- [ ] Add `RadioMessage`, message types, and basic payload structs
- [ ] Add `Comms Plan` and `Comms Resolve` tick stages
- [ ] Add simple reliable leader-member direct transmission (no failure yet)
- [ ] Add console log feed and world arc rendering stub

### Phase B — Failure and Arbitration

- [ ] Add channel queue and anti-talk-over arbitration
- [ ] Add send/receive failure checks and decode confidence
- [ ] Add garbled/partial payload handling
- [ ] Add retries/backoff and dropped message outcomes

### Phase C — Leader Knowledge Integration

- [ ] Update leader blackboard from comms inbox
- [ ] Implement confidence fusion and fact decay
- [ ] Drive Squad Think decisions from reported uncertainty

### Phase D — Requests, Silence, and Stress Coupling

- [ ] Add `StatusRequest` and timeout tracker
- [ ] Add no-reply inference (`Unresponsive` state)
- [ ] Add injured/pain reply effects on leader stress
- [ ] Add defensive policy bias from comms uncertainty

### Phase E — Officer Down and Succession

- [ ] Add `OfficerDownReport`
- [ ] Add command handover logic and delay
- [ ] Validate squad behavior during ambiguous command state

---

## 13. Testing Strategy

### 13.1 Unit Tests

- Message encode/decode confidence transitions
- Arbitration fairness and priority guarantees
- Timeout and no-reply state transitions
- Confidence decay and stale report handling

### 13.2 Scenario Tests

- Multiple soldiers contact-report simultaneously (collision pressure)
- Leader status request to dead/injured/healthy members
- Garbled officer-down report causing delayed succession
- High-fear squad producing degraded communication quality

### 13.3 Regression Goals

- No omniscient leader updates without valid message/sense source
- No infinite message retry loops
- Stress impacts remain bounded and recoverable

---

## 14. Open Design Questions

1. Should squads have one shared net or role-based subnets (leader/medic/fireteam)?
2. Should message semantics support abbreviated military brevity codes or stay plain structured payloads?
3. How quickly should leaders infer probable KIA from repeated non-replies?
4. How strongly should comms uncertainty influence intent switching vs. direct threat evidence?

---

## 15. Success Criteria

This system is successful when:

- Leaders make visibly different decisions based on what they actually heard.
- Missed/garbled comms create believable uncertainty and occasional mistakes.
- Soldiers naturally stagger transmissions under load instead of constant overlap.
- Injured/no-reply status checks produce realistic stress and defensive behavior changes.
- UI clearly communicates active radio links and message quality at a glance.

---

## 16. Transmission Doctrine (Who Talks, When, and Why)

To prevent constant radio spam and make behavior believable, each soldier follows doctrine gates before transmitting.

### 16.1 Event Classes

Every potential report is mapped to one event class:

- **Critical**: officer down, self-catastrophic injury, confirmed flank threat at close range
- **Urgent**: fresh enemy sighting, friendly casualty, no-ammo under contact
- **Routine**: periodic status check, formation drift, low-confidence sound-only contact
- **Administrative** (future): role acknowledgements, net test messages

### 16.2 Report Trigger Conditions

Soldier emits `CommsIntent` only if all are true:

1. Event class passes doctrine threshold
2. Cooldown elapsed for that class
3. Soldier can spare cognitive bandwidth this tick
4. Message novelty exceeds minimum delta (avoid duplicate chatter)

Novelty examples:

- Enemy count changed by >= 2
- Distance band changed (near -> mid -> far)
- Status changed (healthy -> injured -> pinned)
- Contact age reset by new visual confirmation

### 16.3 Cooldown Defaults

| Event Class | Base Cooldown | Discipline Modifier | Fear Modifier |
|---|---:|---:|---:|
| Critical | 20 ticks | -20% at high discipline | +10% at high fear |
| Urgent | 50 ticks | -15% | +20% |
| Routine | 120 ticks | -10% | +35% |

High fear increases hesitancy and verbal coherence penalties unless panic spillover forces distress bursts.

### 16.4 Interrupt Rules

Critical intents can interrupt lower-priority outgoing intents from the same sender. Interrupted intents are re-queued once with reduced priority to avoid starvation.

---

## 17. Signal Propagation Model

Communication quality should be physically grounded but lightweight.

### 17.1 Base Signal Quality

Suggested scalar in `[0,1]`:

```
base = devicePower * receiverSensitivity
dist = clamp01(1 - (d / effectiveRange)^2)
los  = terrainOcclusionFactor   // 1.0 clear, down to ~0.35 through dense occlusion
noise = 1 - ambientNoisePenalty // e.g. 0.0 to 0.6

rxQuality = clamp01(base * dist * los * noise)
```

### 17.2 Human Factors Layer

Apply sender + receiver cognitive penalties:

```
senderClarity = 1 - (0.35*fear + 0.25*fatigue + 0.25*pain + 0.15*suppression)
receiverFocus = 1 - (0.40*fear + 0.30*fatigue + 0.30*incomingThreatLoad)

decodeConfidence = clamp01(rxQuality * senderClarity * receiverFocus)
```

### 17.3 Decode Band Thresholds

| DecodeConfidence | Outcome |
|---:|---|
| `>= 0.80` | Clear decode |
| `0.55 - 0.79` | Partial decode (field drops) |
| `0.30 - 0.54` | Corrupted decode (field substitution risk) |
| `< 0.30` | Dropped |

### 17.4 Semantic Corruption Rules

Corrupted messages should be plausibly wrong, not random nonsense:

- Enemy count perturbs by +/-1 band (never jumps 1 -> 20 instantly)
- Distance band shifts one step (near<->mid<->far)
- Bearing may quantize to adjacent 45deg sector
- Injury severity may drift one level

---

## 18. Message Taxonomy (Expanded)

### 18.1 Enumerations

```go
type RadioMessageType uint8

const (
    MsgContactReport RadioMessageType = iota
    MsgStatusReport
    MsgCasualtyReport
    MsgOfficerDownReport
    MsgFearReport
    MsgOutnumberedReport
    MsgUnableComplyReport
    MsgStatusRequest
    MsgContactConfirmRequest
    MsgOrder
    MsgAck
)

type RadioPriority uint8

const (
    PriRoutine RadioPriority = iota
    PriUrgent
    PriCritical
)

type DecodeState uint8

const (
    DecodeClear DecodeState = iota
    DecodePartial
    DecodeCorrupted
    DecodeDropped
)
```

### 18.2 Payload Struct Sketches

```go
type ContactReportPayload struct {
    EnemyCountMin   int
    EnemyCountMax   int
    DistanceBand    uint8 // 0 near, 1 mid, 2 far
    BearingSector   uint8 // 0..7 (45deg sectors)
    ReporterFear    float32
    VisualConfidence float32
    LastSeenTick    int
}

type StatusReportPayload struct {
    Alive           bool
    InjurySeverity  uint8 // 0 none .. 3 critical
    PainLevel       float32
    Suppressed      bool
    Pinned          bool
    Mobility        float32 // 0..1
    Fear            float32 // 0..1
}

type CasualtyReportPayload struct {
    FriendlyID      int
    State           uint8 // down, incapacitated, KIA (confidence-based)
    LocationX       float64
    LocationY       float64
    Confidence      float32
}

type UnableComplyPayload struct {
    OrderID         uint64
    Reason          uint8 // suppressed, wounded, no-ammo, disoriented
    Fear            float32
}

type StatusRequestPayload struct {
    RequestID       uint64
    RequestedID     int
    DeadlineTick    int
}

type AckPayload struct {
    AckedMessageID  uint64
    AckCode         uint8 // received, partially-understood, cannot-comply
}
```

---

## 19. Channel Arbitration State Machine

### 19.1 Net State

Each squad net tracks:

- `ActiveTx` (current sender, end tick)
- `Pending` priority queues (critical/urgent/routine)
- `BackoffUntil[soldierID]`
- `RecentSpeakerHistory` (fairness weighting)

### 19.2 States

1. **Idle**: no active transmission
2. **Acquire**: choose next speaker by priority + fairness score
3. **Transmit**: occupy channel for message duration
4. **Resolve**: compute send + receive outcomes
5. **AckWait**: optional ack window
6. **Requeue/Done**: retry or complete

### 19.3 Selection Rule

Priority first, then score:

```
score = priorityWeight
      + ageWeight
      - recentSpeakerPenalty
      - retryPenalty
      + randomnessJitter
```

This avoids one highly active soldier monopolizing the net.

### 19.4 Talk-Over Prevention

- If channel is in `Transmit`, new intents are queued only.
- Equal-priority intents arriving same tick roll backoff lottery.
- Backoff jitter range should be small (e.g. 5-20 ticks) to keep latency believable.

---

## 20. Request/Reply and Non-Reply Inference FSM

### 20.1 Member Response States (Leader's View)

- `Responsive`
- `PendingReply`
- `Late`
- `Unresponsive`
- `ConfirmedDown`

### 20.2 Transition Rules

1. `Responsive -> PendingReply` on request send
2. `PendingReply -> Responsive` on valid reply before deadline
3. `PendingReply -> Late` when deadline missed once
4. `Late -> Unresponsive` after grace window
5. `Unresponsive -> Responsive` on any later coherent reply
6. `Unresponsive -> ConfirmedDown` on corroborated casualty evidence

### 20.3 Stress and Uncertainty Effects

Per unresolved member:

```
leaderStressDelta += 0.01 + 0.02*memberBond + 0.01*threatPressure
uncertaintyPenalty += 0.05
```

Cap total comms-driven stress delta per tick to avoid runaway panic loops.

---

## 21. Leader Knowledge Fusion Algorithm

### 21.1 Contact Hypothesis Keying

Cluster reports into contact hypotheses by:

- Team (enemy)
- Distance band
- Bearing sector
- Spatial centroid proximity
- Time window

### 21.2 Fusion Formula

For each hypothesis `h`:

```
confidence(h) = clamp01(
    visualSupport
  + radioSupport * sourceReliabilityAvg
  - conflictPenalty
  - stalenessPenalty
)
```

Where:

- `sourceReliabilityAvg` comes from each reporter's historical decode + composure profile
- `conflictPenalty` increases for contradictory count/distance claims
- `stalenessPenalty` increases after freshness window (e.g. 180 ticks)

### 21.3 Reliability Memory per Reporter

Leader tracks rolling reliability score per member:

- Starts at neutral `0.5`
- Increases when reports are corroborated
- Decreases on repeated contradictions or garbled source conditions
- Never reaches absolute 0/1 to preserve uncertainty

---

## 22. Behavioral Coupling (Cognition Integration)

### 22.1 Member-Level Effects

Communication tasks compete with survival tasks.

Priority gate:

1. Self-preservation
2. Immediate engagement
3. Critical comms
4. Movement/formation
5. Routine comms

If a soldier is pinned and panic > threshold, routine reporting is skipped.

### 22.2 Leader-Level Effects

Squad Think should read comms uncertainty features:

- `UnresponsiveCount`
- `AvgReportConfidence`
- `ConflictingContactRate`
- `RecentOfficerContinuityConfidence`

Policy nudges:

- Higher uncertainty -> bias toward `Hold` / `Regroup`
- High injured pain reports -> increase CASEVAC weighting
- Sudden silence after high contact -> caution + scan + fallback checks

---

## 23. UI/Rendering Spec (Detailed)

### 23.1 Arc Rendering Layers

For each active transmission, render in order:

1. **Base Arc**: thin bezier/segmented curve, soft green
2. **Noise Overlay**: scrolling grain mask modulated by quality
3. **Pulse Envelope**: moving brighter packet pulse along arc
4. **Error Flicker**: occasional segment dropout for low quality

### 23.2 Arc Geometry

- Arc peak height proportional to distance (with clamp)
- Curve direction seeded by sender ID for deterministic variety
- Very short links can render as slight bowed line

### 23.3 Console Feed Layout

Columns:

- Tick
- Net
- Direction (`TX`, `RX`, `REQ`, `ACK`, `TIMEOUT`)
- Sender -> Receiver
- Type
- Summary
- Quality + decode state

Color semantics (green palette only):

- Clear: bright green
- Partial: muted green
- Corrupted: yellow-green warning tint
- Dropped/timeout: dark green + blink

### 23.4 Noise/Grain Style

The grain should look analog/radio-like, not CRT gimmick.

- Keep noise subtle at good signal
- Increase spatial breakup with low confidence
- Avoid full-screen post-processing; localize effect to link arc + console row iconography

---

## 24. Integration Map (Probable Go File Targets)

Expected touch points (adjust to actual code layout during implementation):

- `internal/game/soldier.go`
  - add radio device state, cooldown memory, outgoing intents
- `internal/game/blackboard.go`
  - add comms-derived fact structs + reliability metadata
- `internal/game/cognition*.go`
  - add Comms Plan and Comms Resolve stages
- `internal/game/squad*.go`
  - leader request/timeout tracker and succession hooks
- `internal/game/simlog*.go`
  - comms event logging + counters
- `internal/game/render*.go`
  - arc rendering and console panel
- `cmd/headless-report/main.go`
  - aggregate comms metrics into run summaries

If the existing architecture differs, preserve current boundaries and move comms logic into whichever subsystem already owns perception/action pipelines.

---

## 25. Balancing Knobs (Config Surface)

Expose these in config/tuning tables (not hardcoded constants):

- Base radio range per class of radio
- Terrain attenuation multipliers
- Noise-to-decode penalty curve
- Fear/fatigue/pain communication penalties
- Message class cooldowns
- Arbitration backoff ranges
- Retry count and ack timeouts
- Non-reply stress increments
- Confidence decay half-life for stale reports

This is required to tune realism without repeated code edits.

---

## 26. Performance Budget and Constraints

### 26.1 Complexity Targets

- Arbitration: `O(m log m)` per net per tick (m = pending messages)
- Decode pass: `O(r)` per receiver for relevant active transmissions
- Belief fusion: near-linear in number of active hypotheses

### 26.2 Guardrails

- Hard cap max pending messages per net (drop lowest-value routine traffic first)
- Hard cap max rendered arcs per frame (prioritize on-screen and leader-relevant links)
- Keep console history ring-buffered (fixed memory)

---

## 27. Failure Modes and Mitigations

### 27.1 Potential Failure Modes

1. **Radio Storm**: too many repetitive reports flooding channel
2. **Leader Paralysis**: uncertainty pushes permanent defensive behavior
3. **Runaway Stress Loop**: no-reply -> stress -> worse receive -> more no-reply
4. **Arc Visual Spam**: too many links reduce readability

### 27.2 Mitigations

- Novelty/cooldown filters for routine traffic
- Clamp uncertainty impact on intent switching per window
- Stress feedback saturation + recovery floors
- Visual LOD and arc prioritization

---

## 28. Detailed Test Matrix

### 28.1 Unit Tests (Named)

- `TestRadioDecode_BandsAndThresholds`
- `TestRadioCorruption_PlausibleFieldPerturbation`
- `TestArbitration_PriorityThenFairness`
- `TestArbitration_BackoffPreventsStarvation`
- `TestLeaderTracker_RequestTimeoutStateMachine`
- `TestLeaderFusion_ConflictingReportsReduceConfidence`
- `TestReporterReliability_ConvergesWithCorroboration`

### 28.2 Scenario Tests (Named)

- `Scenario_MultiContactSameTick_ChannelContention`
- `Scenario_StatusRequest_DeadMember_NoReplyEscalation`
- `Scenario_StatusRequest_InjuredReply_IncreasesLeaderStress`
- `Scenario_OfficerDown_GarbledReport_DelayedSuccession`
- `Scenario_HighNoiseUrbanFight_DegradesCommsQuality`
- `Scenario_RadioRecovery_AfterSuppressionLull`

### 28.3 Property-Style Tests

- No message may retry indefinitely
- Leader confidence remains within `[0,1]`
- Unresponsive member can recover to responsive on valid later reply
- Net queue never exceeds configured hard cap

### 28.4 Headless Metrics Assertions

For seeded benchmark scenarios, assert ranges (not exact values):

- Drop rate under moderate combat within expected window
- Garble rate increases with fear/noise cohort
- Intent shift frequency rises with uncertainty but remains bounded

---

## 29. Rollout Plan with Acceptance Gates

### Gate 1 — Functional Skeleton

- Messages pass sender -> receiver via net model
- Console logs all TX/RX paths
- No omniscient blackboard write bypasses

### Gate 2 — Reliability Realism

- Send + receive failures active
- Partial/corrupt decode visible in logs
- Arbitration prevents talk-over chaos

### Gate 3 — Decision Impact

- Leader behavior measurably changes due to comms confidence
- No-reply path creates uncertainty and stress effects

### Gate 4 — Command Continuity

- Officer-down succession works under clear and garbled reports
- Squad remains coherent enough to continue simulation without deadlocks

### Gate 5 — Presentation & AAR

- Arc visualization readable in live view
- Console feed informative but not overwhelming
- Headless report includes comms summary tables

---

## 30. Example Radio Transcript (Target Feel)

```
[4021] TX BRAVO-2 -> BRAVO-LDR CONTACT x2 NEAR N (Q:0.91 CLEAR)
[4022] TX BRAVO-3 -> BRAVO-LDR CONTACT x3 MID NE (Q:0.63 PARTIAL)
[4023] RX BRAVO-LDR <- BRAVO-2 CONTACT x2 80m N
[4024] RX BRAVO-LDR <- BRAVO-3 CONTACT x? ~mid ?NE (PARTIAL)
[4025] TX BRAVO-LDR -> BRAVO-4 STATUS?
[4034] TIMEOUT BRAVO-4 no reply (state: LATE)
[4042] TIMEOUT BRAVO-4 no reply (state: UNRESPONSIVE)
[4048] TX BRAVO-1 -> BRAVO-LDR CASUALTY BRAVO-4 DOWN @ 311,552 (Q:0.77)
[4050] FUSION BRAVO-LDR member BRAVO-4 -> CONFIRMED_DOWN (confidence 0.86)
[4054] INTENT BRAVO-LDR HOLD -> REGROUP (uncertainty + casualty pressure)
```

This style of output should make tactical uncertainty legible and emotionally meaningful.
