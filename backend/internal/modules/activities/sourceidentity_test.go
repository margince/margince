// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"testing"
	"time"
)

// Whether two rows may be bound into one message turns on whether one seat
// wrote both. A Message-ID is typed by whoever sent the message, so binding on
// a forged one across two seats is a way to reach somebody else's mail.
//
// The comparison is over the SEAT each stamp names, because the two doors spell
// one colleague differently: `human:<uuid>` when they log a message themselves,
// `connector:gmail:<uuid>` when their mailbox syncs it. Reading the seat out of
// the stamp is what lets those two meet.
func TestOnlyOneSeatsOwnRowsBindTogether(t *testing.T) {
	const (
		lars  = "01a0a203-b6ab-70b2-988b-b103e09fb822"
		other = "01a0a203-b6ab-70b2-988b-b103e09fb823"
	)
	// A stamp names a seat only when actingHumanOf can read one out of it, and
	// two stamps bind only when they name the SAME non-empty seat.
	bind := func(left, right string) bool {
		l, r := actingHumanOf(left), actingHumanOf(right)
		return l != "" && l == r
	}
	for _, tc := range []struct {
		name        string
		left, right string
		want        bool
	}{
		{
			// The measured case: the same colleague imported the mail and
			// connected the mailbox, reaching the two doors as two stamps.
			name:  "one colleague, through two doors",
			left:  "human:" + lars,
			right: "connector:gmail:" + lars,
			want:  true,
		},
		{"the same door twice", "human:" + lars, "human:" + lars, true},
		{"two mailboxes of one colleague", "connector:gmail:" + lars, "connector:imap:" + lars, true},

		{"two different colleagues", "human:" + lars, "human:" + other, false},
		{"another colleague's mailbox", "human:" + lars, "connector:gmail:" + other, false},

		// Everything below names nobody, and naming nobody must never read as
		// naming the same seat — that is the direction this must not fail in.
		{"the system, twice", "system", "system", false},
		{"a system notice", "system:owed_verdict", "system:owed_verdict", false},
		{"nothing at all", "", "", false},
		{"nothing against somebody", "", "human:" + lars, false},
		{"a stamp with no uuid in it", "human:not-a-uuid", "human:not-a-uuid", false},
		{"a connector naming no one", "connector:gmail", "connector:gmail", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := bind(tc.left, tc.right); got != tc.want {
				t.Fatalf("binding %q to %q = %v, want %v", tc.left, tc.right, got, tc.want)
			}
		})
	}
}

// The seat a connector stamp names is the MAILBOX OWNER, not the connector.
//
// This is the case the whole rule turned on and the one that was wrong: the
// stamp a capture writes is `connector:gmail:<uuid>`, while the principal's own
// ID is the bare `connector:gmail`. Comparing those two shapes never matches, so
// a seat never matched itself and one colleague's import-then-mailbox pair
// stayed two rows — silently, looking exactly like the cross-seat refusal
// working correctly.
func TestAConnectorStampNamesTheMailboxOwner(t *testing.T) {
	const seat = "01a0a203-b6ab-70b2-988b-b103e09fb822"
	if got := actingHumanOf("connector:gmail:" + seat); got != seat {
		t.Fatalf("actingHumanOf read %q out of a connector stamp, want the mailbox owner %q", got, seat)
	}
	// The bare principal ID carries no seat, and must not be mistaken for one.
	if got := actingHumanOf("connector:gmail"); got != "" {
		t.Fatalf("a connector naming no seat read as %q, want nobody", got)
	}
}

// A recurring series is one iCal UID and many meetings, so the occurrence has
// to be part of the identity or a weekly call collapses into a single row.
func TestAMeetingIdentityNamesTheOccurrence(t *testing.T) {
	first := MeetingIdentityKey("series-1@example.test", "2026-09-16T10:00:00Z")
	second := MeetingIdentityKey("series-1@example.test", "2026-09-23T10:00:00Z")
	if first == second {
		t.Fatalf("two occurrences of one series share the identity %q, so the series would be one meeting", first)
	}
	if MeetingIdentityKey(" series-1@example.test ", " 2026-09-16T10:00:00Z ") != first {
		t.Fatal("the same occurrence spelled with stray spacing is a different identity")
	}
}

// One meeting read from two calendars is ONE identity, however each provider
// spells the occurrence.
//
// This is the agreement the whole meeting dedupe rests on, and the failure it
// guards is silent: Google states the start as `+02:00`, Graph as `Z` with
// seven decimal places, and a client may state either. Three spellings of one
// instant that compared unequal would give one meeting three identities, and
// nothing would fail — a missed match looks exactly like a meeting nobody else
// has.
func TestOneOccurrenceIsOneIdentityHoweverItsStartIsSpelled(t *testing.T) {
	const series = "series-42@google.com"
	want := MeetingIdentityKey(series, "2026-09-23T08:00:00Z")
	for _, spelling := range []struct {
		name, instance string
	}{
		{"Google's offset form", "2026-09-23T10:00:00+02:00"},
		{"Graph's sub-second UTC", "2026-09-23T08:00:00.0000000Z"},
		{"plain UTC", "2026-09-23T08:00:00Z"},
		{"a different offset for the same instant", "2026-09-23T03:00:00-05:00"},
	} {
		t.Run(spelling.name, func(t *testing.T) {
			if got := MeetingIdentityKey(series, spelling.instance); got != want {
				t.Fatalf("%s keyed as %q, want %q — one instant must be one identity",
					spelling.name, got, want)
			}
		})
	}
	// And a connector holding the start as an INSTANT reaches the same key,
	// which is what lets the capture door meet the import door at all. It
	// formats through the same function, so there is one spelling rather than
	// two that have to be kept in step.
	start := time.Date(2026, 9, 23, 8, 0, 0, 0, time.UTC)
	if got := MeetingIdentityKey(series, start.UTC().Format(time.RFC3339)); got != want {
		t.Fatalf("a connector's instant keyed as %q, want %q", got, want)
	}
}

// Two occurrences of one series are two meetings.
//
// A weekly call is one iCal UID and fifty-two meetings, so keying on the series
// alone would resolve every one of them onto the first.
func TestTwoOccurrencesOfOneSeriesAreTwoIdentities(t *testing.T) {
	const series = "weekly@google.com"
	first := MeetingIdentityKey(series, "2026-09-23T08:00:00Z")
	second := MeetingIdentityKey(series, "2026-09-30T08:00:00Z")
	if first == second {
		t.Fatalf("two occurrences share the identity %q, so the series would be one meeting", first)
	}
}

// An unparseable occurrence is kept verbatim rather than refused, and two
// callers who spell it the same way still meet.
//
// The key is an opaque identity, not a validated field: refusing here would
// fail an import over a format question the identity does not care about.
func TestAnUnreadableOccurrenceStillKeysConsistently(t *testing.T) {
	const series = "odd@example.test"
	// The value survives into the key rather than being dropped or rewritten,
	// which is what lets two callers who spell it the same way still meet.
	// Asserting the function agrees with itself would prove only that it is a
	// function.
	if got := MeetingIdentityKey(series, "whenever"); got != series+"/whenever" {
		t.Fatalf("an unparseable occurrence keyed as %q, want it carried verbatim", got)
	}
	if MeetingIdentityKey(series, "whenever") == MeetingIdentityKey(series, "later") {
		t.Fatal("two different unparseable occurrences collapsed into one identity")
	}
}
