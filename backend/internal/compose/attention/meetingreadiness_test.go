// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The reader's own imminent preparation keeps its place: crowding does not
// demote it, and a day fuller than the meetings lane's cap does not drop it.

import (
	"fmt"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func imminentMeetings(n int, ready bool) []ranked {
	rows := make([]ranked, 0, n)
	for i := range n {
		meeting := Meeting{
			ID: ids.NewV7(), Subject: fmt.Sprintf("Meeting %d", i),
			StartsAt: rankInstant.Add(time.Duration(10+i) * time.Minute), Preparation: ownPreparation(ready),
		}
		rows = append(rows, classifyMeeting(meetingItem(meeting), rankInstant))
	}
	return rows
}

func crowdedCount(rows []ranked) int {
	crowded := 0
	for _, row := range markCrowding(rows) {
		if row.crowded {
			crowded++
		}
	}
	return crowded
}

func TestCrowdingNeverDemotesTheReadersImminentPreparation(t *testing.T) {
	if got := crowdedCount(imminentMeetings(crowdLead+3, false)); got != 0 {
		t.Errorf("%d of the reader's imminent meetings still to prepare were crowded, want none", got)
	}
	if got := crowdedCount(imminentMeetings(crowdLead+3, true)); got != 3 {
		t.Errorf("%d imminent meetings the reader is ready for were crowded, want the 3 past the lead", got)
	}
}

func TestAMeetingFurtherOffThanTheHorizonIsCrowdedAsUsual(t *testing.T) {
	rows := imminentMeetings(crowdLead+1, false)
	for i := range rows {
		later := rankInstant.Add(meetingHorizon + time.Duration(i+1)*time.Minute)
		rows[i] = classifyMeeting(meetingItem(Meeting{
			ID: ids.NewV7(), Subject: "later", StartsAt: later, Preparation: ownPreparation(false),
		}), rankInstant)
	}
	if got := crowdedCount(rows); got != 1 {
		t.Errorf("%d meetings past the horizon were crowded, want the one past the lead", got)
	}
}

func timedMeeting(i int, prepared *crmcontracts.MeetingPreparation) Meeting {
	return Meeting{
		ID: ids.NewV7(), Subject: fmt.Sprintf("At %02d", i),
		StartsAt: readInstant.Add(time.Duration(i+1) * 15 * time.Minute), Preparation: prepared,
	}
}

func TestTheMeetingsLaneKeepsEveryMeetingTheReaderStillHasToPrepare(t *testing.T) {
	var read []Meeting
	for i := range plannedCap + 4 {
		prepared := ownPreparation(true)
		switch {
		case i >= plannedCap+2:
			prepared = ownPreparation(false)
		case i == plannedCap:
			prepared = nil
		}
		read = append(read, timedMeeting(i, prepared))
	}
	day := Meeting{ID: ids.NewV7(), Subject: "Trade fair", StartsAt: readInstant, Precision: crmcontracts.OccurredPrecisionOccurredPrecisionDay}
	read = append([]Meeting{day}, read...)

	kept := map[string]bool{}
	for _, meeting := range plannedAndUnprepared(read, plannedCap) {
		kept[meeting.Subject] = true
	}
	for _, want := range []string{"Trade fair", "At 00", fmt.Sprintf("At %02d", plannedCap-1), fmt.Sprintf("At %02d", plannedCap+2), fmt.Sprintf("At %02d", plannedCap+3)} {
		if !kept[want] {
			t.Errorf("the lane dropped %q", want)
		}
	}
	for _, cut := range []string{fmt.Sprintf("At %02d", plannedCap), fmt.Sprintf("At %02d", plannedCap+1)} {
		if kept[cut] {
			t.Errorf("the lane kept %q past its cap though the reader has nothing to prepare there", cut)
		}
	}
}

func TestTheMeetingsLaneReadsPastItsCap(t *testing.T) {
	meetings := &stubMeetings{}
	svc := meetingPrepService(nil)
	svc.meetings = meetings
	if _, err := svc.Worklist(meetingPrepReader(), "", "", ids.UUID{}, 25, ""); err != nil {
		t.Fatalf("worklist: %v", err)
	}
	if meetings.limit != meetingsPrepCeiling || meetingsPrepCeiling <= plannedCap {
		t.Errorf("the lane read %d meetings, want the ceiling %d above the cap %d", meetings.limit, meetingsPrepCeiling, plannedCap)
	}
}

func TestAnImminentMeetingTheReaderIsReadyForIsAReminderNotUrgentWork(t *testing.T) {
	soon := rankInstant.Add(30 * time.Minute)
	for _, tc := range []struct {
		name      string
		prepared  *crmcontracts.MeetingPreparation
		wantLevel int
	}{
		{"still to prepare", ownPreparation(false), levelWaiting},
		{"not attended", nil, levelWaiting},
		{"ready", ownPreparation(true), levelAgreed},
	} {
		row := classifyMeeting(meetingItem(Meeting{ID: ids.NewV7(), Subject: tc.name, StartsAt: soon, Preparation: tc.prepared}), rankInstant)
		if row.item.Level != tc.wantLevel {
			t.Errorf("an imminent meeting %s ranked at level %d, want %d", tc.name, row.item.Level, tc.wantLevel)
		}
		if !hasReason(row.item, "meeting_soon") {
			t.Errorf("an imminent meeting %s lost its starting-soon reminder", tc.name)
		}
	}
}
