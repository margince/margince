// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// A day entry dated 9 Aug is stored at noon UTC, which is already 02:00 on
// 10 Aug in Kiritimati. Read on the morning of 10 Aug there, it was yesterday.
func kiritimatiMorning(t *testing.T) (Occurred, time.Time, *time.Location) {
	t.Helper()
	loc, err := time.LoadLocation("Pacific/Kiritimati")
	if err != nil {
		t.Fatal(err)
	}
	logged := Occurred{At: time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC), DateOnly: true}
	return logged, time.Date(2026, time.August, 10, 9, 0, 0, 0, loc), loc
}

func TestADayTouchDatedYesterdayReadsYesterday(t *testing.T) {
	logged, now, loc := kiritimatiMorning(t)
	said := briefCopyFor("en")

	if got, want := lastTouchLine(Input{Language: "en", LastTouchAt: &logged, Now: now, Zone: loc}),
		said.Say(touchWords.Yesterday); got != want {
		t.Errorf("last touch = %q, want %q", got, want)
	}
	attendee := AttendeeIn{FullName: "Ana Roth", LastTouch: &logged}
	if got, want := said.attendeeLine(attendee, now, loc),
		fmt.Sprintf(said.Say(touchWords.SpokeYesterday), "Ana Roth"); got != want {
		t.Errorf("attendee = %q, want %q", got, want)
	}
	prior := PriorMeetingIn{Subject: "Kickoff", StartsAt: logged}
	if got, want := said.priorMeetingLine(prior, now, loc),
		fmt.Sprintf(said.Say(touchWords.MetYesterday), "Kickoff"); got != want {
		t.Errorf("prior meeting = %q, want %q", got, want)
	}
	if got, want := said.lastDealtLine(now, logged, loc), said.Say(changedWords.Yesterday); got != want {
		t.Errorf("last dealt = %q, want %q", got, want)
	}
}

// The prompt states a day entry as its date, so the model never reads the
// stored noon as a time somebody named.
func TestAPromptStatesADayTouchAsItsDate(t *testing.T) {
	logged, _, _ := kiritimatiMorning(t)
	encoded, err := json.Marshal(AttendeeIn{FullName: "Ana Roth", LastTouch: &logged})
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct{ LastTouch string }
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.LastTouch != "2026-08-09" {
		t.Errorf("prompt last touch = %q, want the date 2026-08-09", decoded.LastTouch)
	}
}

// An instant on 1 Jul and a day entry dated 22 Jul are 21 days apart, one
// moment. The day entry's noon is 23 Jul in Kiritimati, which would split them.
func TestArcGapsCountADayEntryByItsDate(t *testing.T) {
	_, _, loc := kiritimatiMorning(t)
	first := Occurred{At: time.Date(2026, time.July, 1, 9, 0, 0, 0, loc)}
	logged := Occurred{At: time.Date(2026, time.July, 22, 12, 0, 0, 0, time.UTC), DateOnly: true}
	threads := []thread{{Key: "a", First: first, Last: first}, {Key: "b", First: logged, Last: logged}}
	if got := len(clusterThreads(threads, loc)); got != 1 {
		t.Errorf("moments = %d, want 1: 1 Jul to 22 Jul is 21 days", got)
	}
}
