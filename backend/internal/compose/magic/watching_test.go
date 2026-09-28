// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

// The watching lane's cases.
//
// They are about the two ways this lane can lie: drawing a condition it cannot
// name, and reporting silence where a refusal belongs. Either one tells an
// administrator their capture is healthy when nobody has checked.

import (
	"context"
	"errors"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// watchInstant is the read's own moment, injected so a line's timing is a fact
// of the case rather than of the clock the test ran on.
var watchInstant = time.Date(2026, 3, 4, 9, 30, 0, 0, time.UTC)

type stubSourceHealth struct {
	concerns []CaptureConcern
	err      error
}

func (s stubSourceHealth) CaptureConcerns(context.Context) ([]CaptureConcern, error) {
	return s.concerns, s.err
}

func TestAMailboxNeedingItsOwnerIsWatched(t *testing.T) {
	connection := ids.UUID{0x11}
	for _, tc := range []struct {
		kind        string
		sentence    string
		consequence string
	}{
		{
			kind:        "reauth_required",
			sentence:    "magic.action.capture_reauth_required",
			consequence: "magic.consequence.capture_not_collecting",
		},
		{
			kind:        "connection_error",
			sentence:    "magic.action.capture_connection_error",
			consequence: "magic.consequence.capture_not_collecting",
		},
		{
			kind:        "sync_failing",
			sentence:    "magic.action.capture_sync_failing",
			consequence: "magic.consequence.capture_may_be_incomplete",
		},
		{
			kind:        "backfill_failed",
			sentence:    "magic.action.capture_backfill_failed",
			consequence: "magic.consequence.capture_history_incomplete",
		},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			line, ok := concernLine(CaptureConcern{
				ConnectionID: connection,
				Kind:         tc.kind,
				Provider:     "google",
				AccountLabel: "sales@acme.test",
			}, watchInstant)
			if !ok {
				t.Fatalf("%q draws no line, so the lane cannot report it at all", tc.kind)
			}
			if line.Summary.Key != tc.sentence {
				t.Errorf("sentence = %q, want %q", line.Summary.Key, tc.sentence)
			}
			if line.Consequence == nil || *line.Consequence != tc.consequence {
				t.Errorf("consequence = %v, want %q", line.Consequence, tc.consequence)
			}
			if line.Lane != crmcontracts.MagicLineLaneMagicLaneWatching {
				t.Errorf("lane = %q, want watching", line.Lane)
			}
			if line.Entity == nil || line.Entity.Type != "capture_connection" {
				t.Fatalf("entity = %v, want the capture connection to open", line.Entity)
			}
			if line.Undo == nil || line.Undo.Undoable {
				t.Error("the line offers an undo — a standing condition is not a change to reverse")
			}
			values := *line.Summary.Values
			if values["provider"] != "google" || values["account"] != "sales@acme.test" {
				t.Errorf("values = %v, want the provider and the mailbox named", values)
			}
		})
	}
}

// An unknown condition is dropped rather than drawn: this lane only ever asks
// for a hand, so a line with no sentence would ask for one and say nothing.
func TestAConditionThisBuildCannotNameIsDroppedRatherThanDrawnBlank(t *testing.T) {
	service := &Service{sources: stubSourceHealth{concerns: []CaptureConcern{
		{ConnectionID: ids.UUID{0x21}, Kind: "quota_exceeded", Provider: "microsoft"},
		{ConnectionID: ids.UUID{0x22}, Kind: "sync_failing", Provider: "google"},
	}}}
	lines, refused, err := service.watching(context.Background(), watchInstant)
	if err != nil {
		t.Fatalf("watching: %v", err)
	}
	if refused != nil {
		t.Fatalf("a nameless condition was reported as a refused source: %v", refused)
	}
	if len(lines) != 1 || lines[0].Summary.Key != "magic.action.capture_sync_failing" {
		t.Fatalf("lines = %v, want the one condition this build can name", lines)
	}
}

func TestAConditionThatIsAStateReportsNoBeginning(t *testing.T) {
	line, ok := concernLine(CaptureConcern{Kind: "reauth_required", Provider: "google"}, watchInstant)
	if !ok {
		t.Fatal("a re-authentication concern draws no line")
	}
	if !line.OccurredAt.Equal(watchInstant) {
		t.Errorf("occurred_at = %v, want the instant it was observed %v", line.OccurredAt, watchInstant)
	}
	if began, dated := (*line.Summary.Values)["failing_since"]; dated {
		t.Errorf("failing_since = %q over a condition that never started failing", began)
	}
}

func TestAFailureStreakCarriesItsOwnBeginning(t *testing.T) {
	began := watchInstant.Add(-72 * time.Hour)
	line, ok := concernLine(CaptureConcern{
		Kind: "sync_failing", Provider: "google", FailingSince: &began,
	}, watchInstant)
	if !ok {
		t.Fatal("a failing sync draws no line")
	}
	if !line.OccurredAt.Equal(watchInstant) {
		t.Errorf("occurred_at = %v, want the instant it was observed %v", line.OccurredAt, watchInstant)
	}
	if got := (*line.Summary.Values)["failing_since"]; got != began.UTC().Format(time.RFC3339) {
		t.Errorf("failing_since = %q, want the streak's start %v", got, began)
	}
}

// A withheld read and a healthy fleet are different answers, and only one of
// them means somebody checked.
func TestAWithheldSourceIsNamedRatherThanEmpty(t *testing.T) {
	service := &Service{sources: stubSourceHealth{err: apperrors.ErrPermissionDenied}}
	lines, refused, err := service.watching(context.Background(), watchInstant)
	if err != nil {
		t.Fatalf("a refusal reached the caller as a failure: %v", err)
	}
	if refused == nil {
		t.Fatal("the refusal is silent, so the page reads as every source healthy")
	}
	if refused.Source != sourceCaptureHealth {
		t.Errorf("source = %q, want %q", refused.Source, sourceCaptureHealth)
	}
	if refused.Reason != crmcontracts.WorklistSourceUnavailableReasonWithheld {
		t.Errorf("reason = %q, want withheld", refused.Reason)
	}
	if len(lines) != 0 {
		t.Errorf("lines = %v, want none behind a refusal", lines)
	}
}

// An installation that wired no capture has nothing to say here, which is not
// the same as a read that failed.
func TestAnUnwiredSourceLeavesTheLaneEmptyRatherThanRefusing(t *testing.T) {
	lines, refused, err := (&Service{}).watching(context.Background(), watchInstant)
	if err != nil {
		t.Fatalf("an unwired seam failed the page: %v", err)
	}
	if refused != nil {
		t.Errorf("an unwired seam was reported as withheld: %v", refused)
	}
	if lines == nil || len(lines) != 0 {
		t.Errorf("lines = %v, want an empty array the contract's clients can iterate", lines)
	}
}

// Anything that is not a refusal is a real fault, and a receipt that swallowed
// it would report a healthy fleet over a broken read.
func TestAReadThatBrokeReachesTheCaller(t *testing.T) {
	broken := errors.New("capture connections unreadable")
	service := &Service{sources: stubSourceHealth{err: broken}}
	lines, refused, err := service.watching(context.Background(), watchInstant)
	if !errors.Is(err, broken) {
		t.Fatalf("err = %v, want the read's own failure", err)
	}
	if lines != nil || refused != nil {
		t.Errorf("a failed read still produced %v / %v", lines, refused)
	}
}

func TestAConnectionWithNoRecordedAddressNamesNone(t *testing.T) {
	line, ok := concernLine(CaptureConcern{Kind: "connection_error", Provider: "microsoft"}, watchInstant)
	if !ok {
		t.Fatal("a connection in error draws no line")
	}
	values := *line.Summary.Values
	if _, named := values["account"]; named {
		t.Errorf("values = %v, want no account key where no address was recorded", values)
	}
	if values["provider"] != "microsoft" {
		t.Errorf("provider = %q, want microsoft", values["provider"])
	}
}
