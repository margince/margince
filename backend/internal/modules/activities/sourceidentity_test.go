// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import "testing"

// Whether two rows may be bound into one message turns on whether one person
// wrote both. A Message-ID is typed by whoever sent the message, so binding on
// a forged one across two people is a way to reach somebody else's mail.
func TestOnlyOnePersonsOwnRowsBindTogether(t *testing.T) {
	const (
		lars  = "01a0a203-b6ab-70b2-988b-b103e09fb822"
		other = "01a0a203-b6ab-70b2-988b-b103e09fb823"
	)
	for _, tc := range []struct {
		name        string
		left, right string
		want        bool
	}{
		{
			// The measured case: the same person imported the mail and
			// connected the mailbox, reaching the two doors as two stamps.
			name:  "one person, through two doors",
			left:  "human:" + lars,
			right: "connector:gmail:" + lars,
			want:  true,
		},
		{"the same door twice", "human:" + lars, "human:" + lars, true},
		{"two mailboxes of one person", "connector:gmail:" + lars, "connector:imap:" + lars, true},

		{"two different people", "human:" + lars, "human:" + other, false},
		{"another person's mailbox", "human:" + lars, "connector:gmail:" + other, false},

		// Everything below names nobody, and naming nobody must never read as
		// naming the same person — that is the direction this must not fail in.
		{"the system, twice", "system", "system", false},
		{"a system notice", "system:owed_verdict", "system:owed_verdict", false},
		{"nothing at all", "", "", false},
		{"nothing against somebody", "", "human:" + lars, false},
		{"a stamp with no uuid in it", "human:not-a-uuid", "human:not-a-uuid", false},
		{"a connector naming no one", "connector:gmail", "connector:gmail", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sameActingHuman(tc.left, tc.right); got != tc.want {
				t.Fatalf("sameActingHuman(%q, %q) = %v, want %v", tc.left, tc.right, got, tc.want)
			}
		})
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
