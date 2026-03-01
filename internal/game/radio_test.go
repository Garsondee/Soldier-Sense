package game

import (
	"strings"
	"testing"
)

func makeRadioSquadForTest(t *testing.T, leaderX, memberX float64) (*Squad, *Soldier, *Soldier, *int) {
	t.Helper()
	ng := NewNavGrid(2400, 800, nil, soldierRadius, nil, nil)
	tick := 0
	tl := NewThoughtLog()
	leader := NewSoldier(0, leaderX, 200, TeamRed, [2]float64{leaderX, 200}, [2]float64{2200, 200}, ng, nil, nil, tl, &tick)
	member := NewSoldier(1, memberX, 200, TeamRed, [2]float64{memberX, 200}, [2]float64{2200, 200}, ng, nil, nil, tl, &tick)
	sq := NewSquad(0, TeamRed, []*Soldier{leader, member})
	return sq, leader, member, &tick
}

func TestResolveComms_StatusReportClearsPending(t *testing.T) {
	sq, leader, member, tick := makeRadioSquadForTest(t, 100, 160)
	sq.radioPendingStatus[member.id] = 50
	sq.radioStatusReplyQueued[member.id] = true

	msg := member.buildStatusReportMessage(leader, *tick, RadioPriUrgent, "STATUS REPLY")
	sq.queueRadio(msg)
	sq.ResolveComms(*tick, nil)
	if sq.radioInFlight == nil {
		t.Fatalf("expected status report to enter in-flight state")
	}
	*tick = sq.radioInFlight.arrivalTick
	sq.ResolveComms(*tick, nil)

	if _, ok := sq.radioPendingStatus[member.id]; ok {
		t.Fatalf("pending status should be cleared after delivered status report")
	}
	if sq.RadioReceived != 1 {
		t.Fatalf("expected one leader receive, got %d", sq.RadioReceived)
	}
	if sq.RadioDropped != 0 {
		t.Fatalf("expected no drops, got %d", sq.RadioDropped)
	}
}

func TestResolveComms_GarbledContactReportLogged(t *testing.T) {
	sq, leader, member, tick := makeRadioSquadForTest(t, 100, 1000) // 900px separation -> always garble band.
	tickVal := *tick + 7
	*tick = tickVal
	tl := NewThoughtLog()

	sq.queueRadio(RadioMessage{
		TickCreated:   tickVal,
		SenderID:      member.id,
		SenderLabel:   member.label,
		ReceiverID:    leader.id,
		ReceiverLabel: leader.label,
		Type:          RadioMsgContactReport,
		Priority:      RadioPriUrgent,
		Summary:       "CONTACT x3 120m",
		ContactX:      900,
		ContactY:      250,
		ContactCount:  3,
	})

	sq.ResolveComms(tickVal, tl)
	if sq.radioInFlight == nil {
		t.Fatalf("expected contact report to enter in-flight state")
	}
	*tick = sq.radioInFlight.arrivalTick
	sq.ResolveComms(*tick, tl)

	if sq.RadioGarbled != 1 {
		t.Fatalf("expected one garbled message, got %d", sq.RadioGarbled)
	}
	if sq.RadioReceived != 1 {
		t.Fatalf("expected leader to receive garbled message, got %d", sq.RadioReceived)
	}
	if !leader.blackboard.RadioHasContact {
		t.Fatalf("leader should still receive contact signal from garbled report")
	}
	entries := tl.Recent()
	if len(entries) == 0 || !strings.Contains(entries[len(entries)-1].Message, "GARBLED") {
		t.Fatalf("expected thought log to record GARBLED quality, got %+v", entries)
	}
}

func TestResolveComms_DroppedReplyTimesOutMember(t *testing.T) {
	sq, leader, member, tick := makeRadioSquadForTest(t, 100, 1000) // 900px separation.
	leader.profile.Psych.Fear = 1
	member.profile.Psych.Fear = 1

	sq.radioPendingStatus[member.id] = 300
	sq.radioStatusReplyQueued[member.id] = true

	sq.queueRadio(member.buildStatusReportMessage(leader, *tick, RadioPriUrgent, "STATUS REPLY"))
	sq.ResolveComms(*tick, nil)
	if sq.radioInFlight == nil {
		t.Fatalf("expected status reply to enter in-flight state")
	}
	*tick = sq.radioInFlight.arrivalTick
	sq.ResolveComms(*tick, nil)
	if sq.RadioDropped != 1 {
		t.Fatalf("expected dropped status reply, got %d", sq.RadioDropped)
	}

	sq.radioPendingStatus[member.id] = *tick + 1
	sq.radioStatusReplyQueued[member.id] = true
	*tick += 2
	sq.ResolveComms(*tick, nil)

	if sq.RadioTimeouts != 1 {
		t.Fatalf("expected one status timeout, got %d", sq.RadioTimeouts)
	}
	if !sq.radioUnresponsive[member.id] {
		t.Fatalf("member should be tracked as unresponsive after timeout")
	}
	if leader.blackboard.UnresponsiveMembers != 1 {
		t.Fatalf("expected blackboard unresponsive count to be 1, got %d", leader.blackboard.UnresponsiveMembers)
	}
}
