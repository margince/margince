// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestCalendarCompletionUsesTheOccurrenceRatherThanTheSeriesOrTitle(t *testing.T) {
	e := integration.Setup(t)
	sink := identifiedCalendarSink(e)
	ctx := calendarOwnerCtx(e, e.AdminUser)
	occurrences := []time.Time{meetingStart, meetingStart.AddDate(0, 0, 7)}
	meetings := make([]ids.UUID, 0, len(occurrences))
	for _, start := range occurrences {
		identity := connector.CrossDoorIdentity{Series: "weekly-customer-review", Occurrence: start}
		record := connector.NormalizedRecord{
			EntityType: "activity", NaturalKey: connector.NaturalKey{SourceSystem: calendarSystem, SourceID: start.Format(time.RFC3339)},
			Fields:            capture.ActivityFields{Kind: "meeting", Subject: "Customer review", Body: "Organizer: client@acme.test", OccurredAt: start},
			CrossDoorIdentity: identity, Source: calendarSystem, CapturedBy: "connector:" + calendarSystem, Raw: []byte(`{}`),
		}
		first, err := sink.Upsert(ctx, record)
		if err != nil {
			t.Fatal(err)
		}
		again, err := sink.Upsert(ctx, record)
		if err != nil {
			t.Fatal(err)
		}
		if first.ID != again.ID {
			t.Fatal("replayed calendar occurrence created another meeting")
		}
		meetings = append(meetings, first.ID)
	}
	if meetings[0] == meetings[1] {
		t.Fatal("two occurrences became one meeting")
	}
	unknownKey := connector.NaturalKey{SourceSystem: calendarSystem, SourceID: "provider-renamed-occurrence"}
	identity := connector.CrossDoorIdentity{Series: "weekly-customer-review", Occurrence: occurrences[0]}
	for range 2 {
		if err := sink.CancelIdentifiedMeeting(ctx, unknownKey, identity, occurrences[0]); err != nil {
			t.Fatal(err)
		}
	}
	first, _ := readMeetingStatus(t, e, meetings[0])
	second, _ := readMeetingStatus(t, e, meetings[1])
	if first != "canceled" || second == "canceled" {
		t.Fatalf("occurrence statuses %q, %q", first, second)
	}
	missing := connector.CrossDoorIdentity{}
	if err := sink.CancelIdentifiedMeeting(ctx, unknownKey, missing, occurrences[1]); err != nil {
		t.Fatal(err)
	}
	if status, _ := readMeetingStatus(t, e, meetings[1]); status == "canceled" {
		t.Fatal("missing identity canceled a same-title occurrence")
	}
}
