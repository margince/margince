// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Whose meeting it was, which is not the same question as who typed it up.
//
// A meeting names the one place an activity says which of US was there, and a
// rep's week is counted from it. Left to captured_by, a colleague who minutes
// somebody else's meeting takes it into their own week — the case the
// attribution audit named, and the one a calendar import already answers
// because it knows whose appointment it read.

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A rep logging a meeting they were at held it, without saying so.
func TestAMeetingLoggedWithNoHostBelongsToWhoLoggedIt(t *testing.T) {
	rep := ids.NewV7()

	got := meetingHost(humanWriting(rep), LogActivityInput{Kind: KindMeeting})

	if got == nil {
		t.Fatal("a meeting a rep logged came out hosted by nobody, so it falls to the recorder fallback")
	}
	if got.UUID != rep {
		t.Fatalf("the meeting is hosted by %v, wanted the rep who logged it %v", got.UUID, rep)
	}
}

// The case this exists for: a colleague minutes somebody else's meeting, and it
// counts into the host's week rather than the minute-taker's.
func TestAMinutedMeetingKeepsTheHostItNames(t *testing.T) {
	minuteTaker, host := ids.NewV7(), ids.From[ids.UserKind](ids.NewV7())

	got := meetingHost(humanWriting(minuteTaker), LogActivityInput{
		Kind:       KindMeeting,
		HostUserID: &host,
	})

	if got == nil || got.UUID != host.UUID {
		t.Fatalf("a minuted meeting is hosted by %v, wanted the colleague it names %v", got, host)
	}
}

// Only a meeting has a host. A task or a mail logged by the same rep must not
// pick one up, or every activity they touch starts claiming to be theirs in a
// sense the column does not mean.
func TestOnlyAMeetingTakesAHost(t *testing.T) {
	for _, kind := range []string{"task", "email", "call", "note"} {
		t.Run(kind, func(t *testing.T) {
			if got := meetingHost(humanWriting(ids.NewV7()), LogActivityInput{Kind: kind}); got != nil {
				t.Errorf("a %s came out hosted by %v", kind, got)
			}
		})
	}
}

// A pass that runs as the system has no week to count a meeting into, and
// naming the machine as the host would put a row in a rep's own view that
// nobody attended. Null is the honest answer, and the fallback still reads.
func TestAMeetingLoggedByNoHumanNamesNoHost(t *testing.T) {
	system := principal.WithActor(t.Context(), principal.Principal{
		Type: principal.PrincipalSystem, ID: "system:import",
	})

	if got := meetingHost(system, LogActivityInput{Kind: KindMeeting}); got != nil {
		t.Errorf("a system-logged meeting claims host %v", got)
	}
}
