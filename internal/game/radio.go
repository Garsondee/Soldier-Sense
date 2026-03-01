package game

import (
	"fmt"
	"math"
)

const (
	radioContactReportCooldown = 75
	radioStatusReportCooldown  = 180
	radioFearReportCooldown    = 150
	radioFearReportThreshold   = 0.78

	radioStatusRequestInterval = 300
	radioStatusReplyWindow     = 120

	radioMaxReliableRange = 900.0
	radioDropThreshold    = 0.22
	radioGarbleThreshold  = 0.50

	radioBaseTransmitTicks = 5
	radioMinTransmitTicks  = 4
	radioMaxTransmitTicks  = 140
	radioCharsPerTick      = 4

	radioChannelTurnaroundTicks  = 5
	radioResponsePauseTicks      = 4
	radioRequestTransitAllowance = 180
)

// RadioMessageType categorises squad radio traffic.
type RadioMessageType uint8

const (
	RadioMsgContactReport RadioMessageType = iota
	RadioMsgStatusReport
	RadioMsgFearReport
	RadioMsgStatusRequest
)

func (t RadioMessageType) String() string {
	switch t {
	case RadioMsgContactReport:
		return "contact"
	case RadioMsgStatusReport:
		return "status"
	case RadioMsgFearReport:
		return "fear"
	case RadioMsgStatusRequest:
		return "status_request"
	default:
		return "unknown"
	}
}

func (sq *Squad) radioTransmitDurationTicks(msg RadioMessage, sender *Soldier) int {
	chars := len(msg.Summary)
	if chars < 1 {
		chars = 1
	}
	ticks := radioBaseTransmitTicks + (chars+radioCharsPerTick-1)/radioCharsPerTick

	if sender != nil {
		ef := clamp01(sender.profile.Psych.EffectiveFear())
		discipline := clamp01(sender.profile.Skills.Discipline)
		if ef > 0.64 {
			roll := math.Abs(math.Sin(float64((sender.tickVal()+7)*(sender.id+23)) * 0.061))
			if roll < 0.45 {
				ticks = int(float64(ticks) * (0.70 - discipline*0.15))
			} else {
				ticks = int(float64(ticks) * (1.10 + ef*0.95))
			}
		} else {
			ticks = int(float64(ticks) * (0.90 + (1.0-discipline)*0.30 + ef*0.18))
		}
	}

	if ticks < radioMinTransmitTicks {
		return radioMinTransmitTicks
	}
	if ticks > radioMaxTransmitTicks {
		return radioMaxTransmitTicks
	}
	return ticks
}

// RadioPriority controls who gets channel time first.
type RadioPriority uint8

const (
	RadioPriRoutine RadioPriority = iota
	RadioPriUrgent
	RadioPriCritical
)

// RadioMessage is the phase-A structured packet used on a squad net.
type RadioMessage struct {
	ID          uint64
	TickCreated int
	NetID       int

	SenderID      int
	SenderLabel   string
	ReceiverID    int
	ReceiverLabel string

	Type     RadioMessageType
	Priority RadioPriority
	Summary  string

	ContactX     float64
	ContactY     float64
	ContactCount int
	Distance     float64
	Fear         float64
	Injured      bool
}

type radioNet struct {
	netID   int
	nextID  uint64
	pending []RadioMessage
}

type radioVisualEvent struct {
	MessageID  uint64
	StartTick  int
	Duration   int
	SenderX    float64
	SenderY    float64
	ReceiverX  float64
	ReceiverY  float64
	MsgType    RadioMessageType
	Delivery   radioDeliveryOutcome
	SenderTeam Team
}

type radioChatLine struct {
	Tick     int
	Sender   string
	Message  string
	Receiver string
	Quality  string
	Duration int
}

type radioTransmission struct {
	msg          RadioMessage
	dispatchTick int
	arrivalTick  int
	resolvedMsg  RadioMessage
	outcome      radioDeliveryOutcome
}

func (rn *radioNet) enqueue(msg RadioMessage) {
	rn.nextID++
	msg.ID = rn.nextID
	msg.NetID = rn.netID
	rn.pending = append(rn.pending, msg)
}

func (rn *radioNet) dequeue() (RadioMessage, bool) {
	if len(rn.pending) == 0 {
		return RadioMessage{}, false
	}

	best := 0
	for i := 1; i < len(rn.pending); i++ {
		cur := rn.pending[i]
		inc := rn.pending[best]
		if cur.Priority > inc.Priority ||
			(cur.Priority == inc.Priority && cur.TickCreated < inc.TickCreated) ||
			(cur.Priority == inc.Priority && cur.TickCreated == inc.TickCreated && cur.ID < inc.ID) {
			best = i
		}
	}

	msg := rn.pending[best]
	rn.pending = append(rn.pending[:best], rn.pending[best+1:]...)
	return msg, true
}

func (sq *Squad) ensureRadioState() {
	if sq.radioNet.netID == 0 {
		sq.radioNet.netID = sq.ID + 1
	}
	if sq.radioPendingStatus == nil {
		sq.radioPendingStatus = make(map[int]int)
	}
	if sq.radioStatusReplyQueued == nil {
		sq.radioStatusReplyQueued = make(map[int]bool)
	}
	if sq.radioUnresponsive == nil {
		sq.radioUnresponsive = make(map[int]bool)
	}
}

func (sq *Squad) queueRadio(msg RadioMessage) {
	sq.ensureRadioState()
	sq.radioNet.enqueue(msg)
	sq.RadioQueued++
}

// PlanComms creates candidate member/leader transmissions for this tick.
func (sq *Squad) PlanComms(tick int) {
	if sq.Leader == nil || sq.Leader.state == SoldierStateDead {
		return
	}
	sq.ensureRadioState()

	if tick-sq.radioLastStatusRequestTick >= radioStatusRequestInterval {
		sq.planLeaderStatusRequest(tick)
	}

	for _, m := range sq.Members {
		if m == sq.Leader || m.state == SoldierStateDead {
			continue
		}

		if deadline, ok := sq.radioPendingStatus[m.id]; ok {
			if tick <= deadline && !sq.radioStatusReplyQueued[m.id] {
				reply := m.buildStatusReportMessage(sq.Leader, tick, RadioPriUrgent, "STATUS REPLY")
				sq.queueRadio(reply)
				sq.radioStatusReplyQueued[m.id] = true
				continue
			}
		}

		if msg, ok := m.buildContactReportMessage(sq.Leader, tick); ok {
			sq.queueRadio(msg)
			continue
		}
		if msg, ok := m.buildInjuryStatusMessage(sq.Leader, tick); ok {
			sq.queueRadio(msg)
			continue
		}
		if msg, ok := m.buildFearReportMessage(sq.Leader, tick); ok {
			sq.queueRadio(msg)
		}
	}
}

func (sq *Squad) planLeaderStatusRequest(tick int) {
	if len(sq.Members) <= 1 {
		return
	}

	start := sq.radioStatusRequestCursor
	for i := 0; i < len(sq.Members); i++ {
		idx := (start + i) % len(sq.Members)
		m := sq.Members[idx]
		if m == sq.Leader || m.state == SoldierStateDead {
			continue
		}
		if _, pending := sq.radioPendingStatus[m.id]; pending {
			continue
		}

		msg := RadioMessage{
			TickCreated:   tick,
			SenderID:      sq.Leader.id,
			SenderLabel:   sq.Leader.label,
			ReceiverID:    m.id,
			ReceiverLabel: m.label,
			Type:          RadioMsgStatusRequest,
			Priority:      RadioPriRoutine,
			Summary:       "STATUS?",
		}
		sq.queueRadio(msg)
		sq.radioPendingStatus[m.id] = tick + radioStatusReplyWindow + radioRequestTransitAllowance
		sq.radioStatusReplyQueued[m.id] = false
		sq.radioStatusRequestCursor = (idx + 1) % len(sq.Members)
		sq.radioLastStatusRequestTick = tick
		return
	}
}

// ResolveComms resolves one transmission slot for this tick and applies effects.
func (sq *Squad) ResolveComms(tick int, tl *ThoughtLog) {
	if sq.Leader == nil || sq.Leader.state == SoldierStateDead {
		return
	}
	sq.ensureRadioState()
	sq.pruneRadioChatLines(tick)

	sq.resolveStatusTimeouts(tick, tl)
	sq.resolveInFlightTransmission(tick, tl)
	if sq.radioInFlight != nil {
		return
	}
	if tick < sq.radioChannelBusyUntil {
		return
	}

	msg, ok := sq.radioNet.dequeue()
	if !ok {
		return
	}

	sender := sq.memberByID(msg.SenderID)
	transmitTicks := sq.radioTransmitDurationTicks(msg, sender)
	arrivalTick := tick + transmitTicks
	resolvedMsg, outcome := sq.resolveDelivery(msg, tick)

	sq.radioInFlight = &radioTransmission{
		msg:          msg,
		dispatchTick: tick,
		arrivalTick:  arrivalTick,
		resolvedMsg:  resolvedMsg,
		outcome:      outcome,
	}
	sq.radioChannelBusyUntil = arrivalTick + radioChannelTurnaroundTicks
	sq.RadioSent++
}

func (sq *Squad) resolveInFlightTransmission(tick int, tl *ThoughtLog) {
	if sq.radioInFlight == nil || tick < sq.radioInFlight.arrivalTick {
		return
	}

	tx := sq.radioInFlight
	sq.radioInFlight = nil

	sq.pushRadioVisualEvent(tx.resolvedMsg, tx.outcome, tx.dispatchTick, tx.arrivalTick-tx.dispatchTick)
	sq.pushRadioChatLine(tx.resolvedMsg, tx.outcome, tick)
	switch tx.outcome {
	case radioDeliveryDrop:
		sq.RadioDropped++
		if tl != nil {
			tl.Add(tick, tx.msg.SenderLabel, sq.Team, fmt.Sprintf("radio %s->%s DROP %s", tx.msg.SenderLabel, tx.msg.ReceiverLabel, tx.msg.Summary), LogCatRadio)
		}
		return
	case radioDeliveryGarbled:
		sq.RadioGarbled++
	}

	if tx.resolvedMsg.Type == RadioMsgStatusRequest {
		deadline := tick + radioStatusReplyWindow + radioResponsePauseTicks
		if cur, ok := sq.radioPendingStatus[tx.resolvedMsg.ReceiverID]; !ok || deadline > cur {
			sq.radioPendingStatus[tx.resolvedMsg.ReceiverID] = deadline
		}
		sq.radioStatusReplyQueued[tx.resolvedMsg.ReceiverID] = false
	}

	if tx.resolvedMsg.ReceiverID == sq.Leader.id {
		sq.RadioReceived++
		sq.applyRadioMessage(tx.resolvedMsg, tick)
	}
	if sq.radioChannelBusyUntil < tick+radioResponsePauseTicks {
		sq.radioChannelBusyUntil = tick + radioResponsePauseTicks
	}
	if tl != nil {
		quality := "CLEAR"
		if tx.outcome == radioDeliveryGarbled {
			quality = "GARBLED"
		}
		tl.Add(tick, tx.resolvedMsg.SenderLabel, sq.Team, fmt.Sprintf("radio %s->%s %s (%s)", tx.resolvedMsg.SenderLabel, tx.resolvedMsg.ReceiverLabel, tx.resolvedMsg.Summary, quality), LogCatRadio)
	}
}

func (sq *Squad) resolveStatusTimeouts(tick int, tl *ThoughtLog) {
	for id, deadline := range sq.radioPendingStatus {
		if tick <= deadline {
			continue
		}
		delete(sq.radioPendingStatus, id)
		delete(sq.radioStatusReplyQueued, id)
		sq.radioUnresponsive[id] = true
		sq.RadioTimeouts++
		if sq.Leader != nil {
			sq.Leader.profile.Psych.ApplyStress(0.02)
			sq.Leader.blackboard.ShatterEvent = true
			sq.Leader.blackboard.UnresponsiveMembers = len(sq.radioUnresponsive)
		}
		if tl != nil {
			tl.Add(tick, sq.Leader.label, sq.Team, fmt.Sprintf("radio timeout: %s no reply", sq.memberLabelByID(id)), LogCatRadio)
		}
	}
}

func (sq *Squad) applyRadioMessage(msg RadioMessage, tick int) {
	if sq.Leader == nil {
		return
	}
	bb := &sq.Leader.blackboard
	bb.RadioLastHeardTick = tick
	bb.RadioLastSenderID = msg.SenderID
	bb.RadioLastMessageType = msg.Type

	switch msg.Type {
	case RadioMsgContactReport:
		bb.RadioHasContact = true
		bb.RadioContactX = msg.ContactX
		bb.RadioContactY = msg.ContactY
		bb.RadioContactTick = msg.TickCreated
		bb.SquadHasContact = true
		bb.SquadContactX = msg.ContactX
		bb.SquadContactY = msg.ContactY
	case RadioMsgStatusReport:
		delete(sq.radioPendingStatus, msg.SenderID)
		delete(sq.radioStatusReplyQueued, msg.SenderID)
		delete(sq.radioUnresponsive, msg.SenderID)
		if msg.Injured {
			sq.Leader.profile.Psych.ApplyStress(0.01 + 0.02*msg.Fear)
		}
	case RadioMsgFearReport:
		sq.Leader.profile.Psych.ApplyStress(0.01 + 0.03*msg.Fear)
	}

	bb.UnresponsiveMembers = len(sq.radioUnresponsive)
}

type radioDeliveryOutcome uint8

const (
	radioDeliveryDrop radioDeliveryOutcome = iota
	radioDeliveryGarbled
	radioDeliveryClear
)

func (sq *Squad) resolveDelivery(msg RadioMessage, tick int) (RadioMessage, radioDeliveryOutcome) {
	sender := sq.memberByID(msg.SenderID)
	receiver := sq.memberByID(msg.ReceiverID)
	if sender == nil || receiver == nil || sender.state == SoldierStateDead || receiver.state == SoldierStateDead {
		return msg, radioDeliveryDrop
	}

	dist := math.Hypot(sender.x-receiver.x, sender.y-receiver.y)
	distancePenalty := clamp01(dist / radioMaxReliableRange)
	senderFear := sender.profile.Psych.EffectiveFear()
	receiverFear := receiver.profile.Psych.EffectiveFear()
	noisePenalty := sq.radioDeterministicNoise(msg, tick) * 0.12

	quality := 1.0 - (0.55 * distancePenalty) - (0.20 * senderFear) - (0.13 * receiverFear) - noisePenalty
	quality = clamp01(quality)
	if quality < radioDropThreshold {
		return msg, radioDeliveryDrop
	}
	if quality < radioGarbleThreshold {
		return sq.garbledMessage(msg, tick), radioDeliveryGarbled
	}
	return msg, radioDeliveryClear
}

func (sq *Squad) radioDeterministicNoise(msg RadioMessage, tick int) float64 {
	phase := float64(msg.ID*17+uint64(msg.SenderID*31)+uint64(msg.ReceiverID*13)+uint64(tick*7)+uint64(sq.ID*19)) * 0.071 // #nosec G115 -- intentional bit-mixing for deterministic noise
	v := math.Sin(phase)
	return (v + 1.0) * 0.5
}

func (sq *Squad) garbledMessage(msg RadioMessage, tick int) RadioMessage {
	garbled := msg
	jitter := sq.radioDeterministicNoise(msg, tick)
	garbled.Summary = "GARBLED " + msg.Summary

	switch msg.Type {
	case RadioMsgContactReport:
		offsetX := (jitter - 0.5) * 120.0
		offsetY := (0.5 - jitter) * 120.0
		garbled.ContactX = msg.ContactX + offsetX
		garbled.ContactY = msg.ContactY + offsetY
		if msg.ContactCount > 0 {
			garbled.ContactCount = max(1, msg.ContactCount-1)
		}
	case RadioMsgStatusReport, RadioMsgFearReport:
		garbled.Fear = clamp01(msg.Fear*0.65 + jitter*0.2)
	}

	return garbled
}

func (sq *Squad) memberByID(id int) *Soldier {
	for _, m := range sq.Members {
		if m.id == id {
			return m
		}
	}
	return nil
}

func (sq *Squad) pushRadioVisualEvent(msg RadioMessage, outcome radioDeliveryOutcome, tick int, transmitTicks int) {
	sender := sq.memberByID(msg.SenderID)
	receiver := sq.memberByID(msg.ReceiverID)
	if sender == nil || receiver == nil {
		return
	}

	duration := 22
	if outcome == radioDeliveryDrop {
		duration = 16
	}
	if transmitTicks > duration {
		duration = transmitTicks
	}

	event := radioVisualEvent{
		MessageID:  msg.ID,
		StartTick:  tick,
		Duration:   duration,
		SenderX:    sender.x,
		SenderY:    sender.y,
		ReceiverX:  receiver.x,
		ReceiverY:  receiver.y,
		MsgType:    msg.Type,
		Delivery:   outcome,
		SenderTeam: sender.team,
	}
	sq.radioVisualEvents = append(sq.radioVisualEvents, event)
}

func (sq *Squad) pruneRadioVisualEvents(tick int) {
	if len(sq.radioVisualEvents) == 0 {
		return
	}
	kept := sq.radioVisualEvents[:0]
	for _, ev := range sq.radioVisualEvents {
		if tick-ev.StartTick < ev.Duration {
			kept = append(kept, ev)
		}
	}
	sq.radioVisualEvents = kept
}

func (sq *Squad) pushRadioChatLine(msg RadioMessage, outcome radioDeliveryOutcome, tick int) {
	quality := "CLEAR"
	switch outcome {
	case radioDeliveryDrop:
		quality = "DROP"
	case radioDeliveryGarbled:
		quality = "GARBLED"
	}

	line := radioChatLine{
		Tick:     tick,
		Sender:   msg.SenderLabel,
		Message:  msg.Summary,
		Receiver: msg.ReceiverLabel,
		Quality:  quality,
		Duration: 360,
	}
	sq.radioChatLines = append(sq.radioChatLines, line)
	if len(sq.radioChatLines) > 24 {
		sq.radioChatLines = sq.radioChatLines[len(sq.radioChatLines)-24:]
	}
}

func (sq *Squad) pruneRadioChatLines(tick int) { // nolint:unused
	if len(sq.radioChatLines) == 0 {
		return
	}
	kept := sq.radioChatLines[:0]
	for _, line := range sq.radioChatLines {
		if tick-line.Tick < line.Duration {
			kept = append(kept, line)
		}
	}
	sq.radioChatLines = kept
}

func (sq *Squad) memberLabelByID(id int) string {
	for _, m := range sq.Members {
		if m.id == id {
			return m.label
		}
	}
	return fmt.Sprintf("ID-%d", id)
}

func (s *Soldier) buildContactReportMessage(leader *Soldier, tick int) (RadioMessage, bool) {
	if leader == nil {
		return RadioMessage{}, false
	}
	if tick-s.radioLastContactReportTick < radioContactReportCooldown {
		return RadioMessage{}, false
	}

	seen := s.blackboard.VisibleThreatCount()
	if seen == 0 {
		return RadioMessage{}, false
	}

	best := math.MaxFloat64
	cx, cy := s.x, s.y
	for _, t := range s.blackboard.Threats {
		if !t.IsVisible {
			continue
		}
		d := math.Hypot(t.X-s.x, t.Y-s.y)
		if d < best {
			best = d
			cx, cy = t.X, t.Y
		}
	}

	s.radioLastContactReportTick = tick
	return RadioMessage{
		TickCreated:   tick,
		SenderID:      s.id,
		SenderLabel:   s.label,
		ReceiverID:    leader.id,
		ReceiverLabel: leader.label,
		Type:          RadioMsgContactReport,
		Priority:      RadioPriUrgent,
		Summary:       fmt.Sprintf("CONTACT x%d %.0fm", seen, best),
		ContactX:      cx,
		ContactY:      cy,
		ContactCount:  seen,
		Distance:      best,
		Fear:          s.profile.Psych.EffectiveFear(),
	}, true
}

func (s *Soldier) buildStatusReportMessage(leader *Soldier, tick int, pri RadioPriority, prefix string) RadioMessage {
	fear := s.profile.Psych.EffectiveFear()
	injured := s.health < soldierMaxHP
	status := "OK"
	if injured {
		status = "INJURED"
	}
	if s.state == SoldierStateDead {
		status = "DOWN"
	}

	s.radioLastStatusReportTick = tick
	return RadioMessage{
		TickCreated:   tick,
		SenderID:      s.id,
		SenderLabel:   s.label,
		ReceiverID:    leader.id,
		ReceiverLabel: leader.label,
		Type:          RadioMsgStatusReport,
		Priority:      pri,
		Summary:       fmt.Sprintf("%s %s fear:%.2f hp:%.0f", prefix, status, fear, s.health),
		Fear:          fear,
		Injured:       injured,
	}
}

func (s *Soldier) buildInjuryStatusMessage(leader *Soldier, tick int) (RadioMessage, bool) {
	if leader == nil || s.health >= soldierMaxHP {
		return RadioMessage{}, false
	}
	if tick-s.radioLastStatusReportTick < radioStatusReportCooldown {
		return RadioMessage{}, false
	}
	msg := s.buildStatusReportMessage(leader, tick, RadioPriRoutine, "STATUS")
	return msg, true
}

func (s *Soldier) buildFearReportMessage(leader *Soldier, tick int) (RadioMessage, bool) {
	if leader == nil {
		return RadioMessage{}, false
	}
	fear := s.profile.Psych.EffectiveFear()
	if fear < radioFearReportThreshold {
		return RadioMessage{}, false
	}
	if tick-s.radioLastFearReportTick < radioFearReportCooldown {
		return RadioMessage{}, false
	}
	s.radioLastFearReportTick = tick
	return RadioMessage{
		TickCreated:   tick,
		SenderID:      s.id,
		SenderLabel:   s.label,
		ReceiverID:    leader.id,
		ReceiverLabel: leader.label,
		Type:          RadioMsgFearReport,
		Priority:      RadioPriUrgent,
		Summary:       fmt.Sprintf("FEAR high %.2f", fear),
		Fear:          fear,
	}, true
}
