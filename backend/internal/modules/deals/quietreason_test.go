// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"strings"
	"testing"
	"time"

	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// The reason exists to tell a reader which way the silence runs, because the
// two directions call for opposite actions. These tests pin that distinction
// and the facts that make it checkable.

func quietAt(day int) time.Time {
	return time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC).AddDate(0, 0, day)
}

var quietToday = quietAt(0)

func TestQuietReasonNamesWhoWeOweAReplyTo(t *testing.T) {
	anna := ids.NewV7()
	facts := QuietFacts{
		LastInbound:  &QuietSide{At: quietAt(-21), PersonID: anna},
		LastOutbound: &QuietSide{At: quietAt(-30)},
	}

	got := quietReason(facts, QuietNames{anna: "Anna Weber"}, quietToday, time.UTC)

	for _, want := range []string{"Anna Weber", "nobody has answered", "3 weeks ago", "5 August"} {
		if !strings.Contains(got, want) {
			t.Errorf("reason = %q, want it to contain %q", got, want)
		}
	}
}

func TestQuietReasonSaysWeGotNoReplyWhenWeSpokeLast(t *testing.T) {
	anna := ids.NewV7()
	facts := QuietFacts{
		LastInbound:  &QuietSide{At: quietAt(-40)},
		LastOutbound: &QuietSide{At: quietAt(-10), PersonID: anna},
	}

	got := quietReason(facts, QuietNames{anna: "Anna Weber"}, quietToday, time.UTC)

	if !strings.Contains(got, "We wrote to Anna Weber") {
		t.Errorf("reason = %q, want it to say we wrote to Anna Weber", got)
	}
	if !strings.Contains(got, "no reply") {
		t.Errorf("reason = %q, want it to say there was no reply", got)
	}
	if strings.Contains(got, "nobody has answered") {
		t.Errorf("reason = %q — that phrasing blames us for a prospect who went cold", got)
	}
}

// The direction is decided by which message is NEWER, not by which exists. A
// deal with correspondence both ways is the normal case, and reading it wrong
// tells a rep to chase somebody who is in fact waiting on them.
func TestQuietReasonPicksTheDirectionOfTheNewerMessage(t *testing.T) {
	them, us := ids.NewV7(), ids.NewV7()
	names := QuietNames{them: "Anna Weber", us: "Boris Klein"}

	theyRepliedLast := quietReason(QuietFacts{
		LastInbound:  &QuietSide{At: quietAt(-5), PersonID: them},
		LastOutbound: &QuietSide{At: quietAt(-9), PersonID: us},
	}, names, quietToday, time.UTC)
	if !strings.Contains(theyRepliedLast, "Anna Weber wrote") {
		t.Errorf("reason = %q, want the inbound reading — they spoke most recently", theyRepliedLast)
	}

	weWroteLast := quietReason(QuietFacts{
		LastInbound:  &QuietSide{At: quietAt(-9), PersonID: them},
		LastOutbound: &QuietSide{At: quietAt(-5), PersonID: us},
	}, names, quietToday, time.UTC)
	if !strings.Contains(weWroteLast, "We wrote to Boris Klein") {
		t.Errorf("reason = %q, want the outbound reading — we spoke most recently", weWroteLast)
	}
}

// The facts query accepts any directional activity, so the verb has to match
// what actually happened. Reporting a phone call as something somebody "wrote"
// is a small falsehood, and a reader who was on that call stops trusting
// everything else the card says.
func TestQuietReasonSaysWhatActuallyHappened(t *testing.T) {
	anna := ids.NewV7()
	names := QuietNames{anna: "Anna Weber"}

	for _, tc := range []struct {
		kind     string
		inbound  string
		outbound string
	}{
		// Both halves of the sentence follow the verb: you do not "answer" a
		// call, and a meeting never gets a "reply".
		{
			"call", "Anna Weber called on 6 August and nobody has followed up since",
			"We called Anna Weber on 6 August and nothing has happened since",
		},
		{
			"meeting", "Anna Weber met us on 6 August and nobody has followed up since",
			"We met Anna Weber on 6 August and nothing has happened since",
		},
		{
			"email", "Anna Weber wrote on 6 August and nobody has answered since",
			"We wrote to Anna Weber on 6 August and there has been no reply",
		},
	} {
		theyLast := quietReason(QuietFacts{
			LastInbound: &QuietSide{At: quietAt(-20), PersonID: anna, Kind: tc.kind},
		}, names, quietToday, time.UTC)
		if !strings.Contains(theyLast, tc.inbound) {
			t.Errorf("inbound %s reason = %q, want %q in it", tc.kind, theyLast, tc.inbound)
		}

		weLast := quietReason(QuietFacts{
			LastOutbound: &QuietSide{At: quietAt(-20), PersonID: anna, Kind: tc.kind},
		}, names, quietToday, time.UTC)
		if !strings.Contains(weLast, tc.outbound) {
			t.Errorf("outbound %s reason = %q, want %q in it", tc.kind, weLast, tc.outbound)
		}
	}
}

// An unrecognised kind takes the neutral verb rather than vanishing or
// guessing — a new activity kind must not silently produce a broken sentence.
func TestQuietReasonDegradesAnUnknownKindToTheNeutralVerb(t *testing.T) {
	got := quietReason(QuietFacts{
		LastInbound: &QuietSide{At: quietAt(-20), Kind: "carrier_pigeon"},
	}, nil, quietToday, time.UTC)

	if !strings.Contains(got, "The contact wrote") {
		t.Errorf("reason = %q, want the neutral verb for an unknown kind", got)
	}
}

// An unnamed counterparty is the common case, not an edge one: an unmatched
// address carries no person, and privacy erasure nulls the link outright. The
// reason must still read as a sentence.
func TestQuietReasonDegradesToAGenericNounWithoutAName(t *testing.T) {
	facts := QuietFacts{LastInbound: &QuietSide{At: quietAt(-8)}}

	got := quietReason(facts, nil, quietToday, time.UTC)

	if !strings.HasPrefix(got, "The contact wrote") {
		t.Errorf("reason = %q, want it to open with the generic noun", got)
	}
	if strings.Contains(got, "00000000") {
		t.Errorf("reason = %q — an identifier is never shown in place of a name", got)
	}
}

func TestQuietReasonSaysSoWhenThereIsNoCorrespondenceAtAll(t *testing.T) {
	got := quietReason(QuietFacts{}, nil, quietToday, time.UTC)

	if !strings.Contains(got, "no correspondence") {
		t.Errorf("reason = %q, want it to say there is nothing to judge the deal by", got)
	}
}

// The span is what a person would say out loud. Days while a reader still
// counts in days, weeks once they would not.
func TestQuietForSpeaksInTheUnitAPersonWouldUse(t *testing.T) {
	for _, tc := range []struct {
		daysAgo int
		want    string
	}{
		{0, "that was today"},
		{1, "that was yesterday"},
		{9, "9 days ago"},
		{14, "14 days ago"},
		{15, "2 weeks ago"},
		{45, "6 weeks ago"},
	} {
		if got := quietFor(quietAt(-tc.daysAgo), quietToday, time.UTC); got != tc.want {
			t.Errorf("quietFor(%d days ago) = %q, want %q", tc.daysAgo, got, tc.want)
		}
	}
}

// The date is the one the READER would write down, which is the date in the
// workspace's own zone. A message at 22:00 in Berlin is 20:00 UTC the same day,
// but one at 01:00 Berlin is the PREVIOUS day in UTC — and a card naming a day
// the reader can check against their own mailbox has to name the right one.
func TestQuietDayIsTheWorkspacesOwnCalendarDay(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatalf("loading the zone: %v", err)
	}
	// 00:30 on 6 August in Berlin is 22:30 on 5 August in UTC.
	justAfterMidnight := time.Date(2026, 8, 6, 0, 30, 0, 0, berlin)

	if got := quietDay(justAfterMidnight, berlin); got != "6 August" {
		t.Errorf("quietDay in Berlin = %q, want %q", got, "6 August")
	}
	if got := quietDay(justAfterMidnight, time.UTC); got != "5 August" {
		t.Errorf("quietDay in UTC = %q, want %q — the zones must genuinely differ here", got, "5 August")
	}
}

// A clock that has not caught up must not print a negative span.
func TestQuietForNeverReportsTheFuture(t *testing.T) {
	if got := quietFor(quietAt(3), quietToday, time.UTC); got != "that was today" {
		t.Errorf("quietFor(future) = %q, want it clamped to today", got)
	}
}

// The paced basis explains where a date came from. It must never show the
// arithmetic that produced it — a formula is not a reason a rep can weigh.
func TestPacedBasisExplainsWithoutShowingTheFormula(t *testing.T) {
	for _, stages := range []int{1, 4} {
		got := pacedBasis(stages)
		for _, forbidden := range []string{"×", "*", "velocity", "stage(s)"} {
			if strings.Contains(got, forbidden) {
				t.Errorf("pacedBasis(%d) = %q, want no %q in it", stages, got, forbidden)
			}
		}
	}
	if got := pacedBasis(1); strings.Contains(got, "1 stages") {
		t.Errorf("pacedBasis(1) = %q, want the singular reading", got)
	}
}
