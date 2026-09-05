// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package orgscan

import (
	"errors"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The ensure rule, branch by branch. It is what keeps the scan on demand:
// nothing is read twice, nothing is read for an account that did not move,
// and a busy account is read at most once an hour per reader.

var ensureNow = time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)

func settledRow(fingerprint string, ago time.Duration) *row {
	at := ensureNow.Add(-ago)
	return &row{Status: StatusDone, Fingerprint: &fingerprint, GeneratedAt: &at}
}

func TestAReaderWhoNeverAskedGetsARead(t *testing.T) {
	if got := decide(nil, "fp", ensureNow, false); got != queueRead {
		t.Errorf("decide = %v, want a queued read", got)
	}
}

func TestAReadInFlightIsNeverStartedTwice(t *testing.T) {
	for _, status := range []string{StatusQueued, StatusRunning} {
		live := &row{Status: status}
		if got := decide(live, "fp", ensureNow, true); got != serveCurrent {
			t.Errorf("a %s read was started again (force or not): %v", status, got)
		}
	}
}

func TestAMatchingFingerprintIsServedWithoutAModelCall(t *testing.T) {
	if got := decide(settledRow("fp", 3*time.Hour), "fp", ensureNow, false); got != serveCurrent {
		t.Errorf("decide = %v, want the stored findings", got)
	}
}

func TestAChangedAccountUnderTheFloorIsServedStale(t *testing.T) {
	if got := decide(settledRow("old", 20*time.Minute), "new", ensureNow, false); got != serveStale {
		t.Errorf("decide = %v, want stale — a busy inbox must not re-read on every message", got)
	}
}

func TestAChangedAccountPastTheFloorIsReadAgain(t *testing.T) {
	if got := decide(settledRow("old", RescanFloor), "new", ensureNow, false); got != queueRead {
		t.Errorf("decide = %v, want a queued read once the floor has passed", got)
	}
}

func TestForceSkipsTheFloorAndTheFingerprintButNotTheInFlightCheck(t *testing.T) {
	if got := decide(settledRow("fp", time.Minute), "fp", ensureNow, true); got != queueRead {
		t.Errorf("force did not read a current account again: %v", got)
	}
}

func TestAFailedReadThatNeverSettledIsReadAgain(t *testing.T) {
	failed := &row{Status: StatusFailed}
	if got := decide(failed, "fp", ensureNow, false); got != queueRead {
		t.Errorf("decide = %v, want a queued read after a failure with nothing stored", got)
	}
}

// The merged list: the rules first, the model's after, one row per
// fingerprint, the cap reported.
func TestTheMergeKeepsOneRowPerSituationAndReportsTheCap(t *testing.T) {
	suggestion := func(fp string) crmcontracts.Organization360Suggestion {
		return crmcontracts.Organization360Suggestion{Fingerprint: fp}
	}
	rules := []crmcontracts.Organization360Suggestion{suggestion("a"), suggestion("b")}
	read := []crmcontracts.Organization360Suggestion{suggestion("b"), suggestion("c"), suggestion("d"), suggestion("e"), suggestion("f")}

	merged, dropped := merge(rules, read)
	if len(merged) != maxAdvice || dropped != 1 {
		t.Fatalf("merged %d, dropped %d; want %d and 1", len(merged), dropped, maxAdvice)
	}
	if merged[0].Fingerprint != "a" || merged[1].Fingerprint != "b" || merged[2].Fingerprint != "c" {
		t.Errorf("order = %v; want the rules first, then the model's, each once", merged)
	}
}

// A scan is a reading aid for a person. An agent holding a passport has the
// records themselves, and a call with no user has nobody to file the row
// under, so both are refused as permission rather than served as somebody's.
func TestTheScanBelongsToAHumanAndNobodyElse(t *testing.T) {
	svc := &Service{}
	agent := principal.WithActor(t.Context(), principal.Principal{Type: principal.PrincipalAgent, ID: "agent:x"})
	if _, err := svc.caller(agent); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("an agent: %v, want permission denied", err)
	}
	nobody := principal.WithActor(t.Context(), principal.Principal{Type: principal.PrincipalHuman, ID: "human:?"})
	if _, err := svc.caller(nobody); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a human with no user id: %v, want permission denied", err)
	}
	user := ids.NewV7()
	human := principal.WithActor(t.Context(), principal.Principal{Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user})
	if got, err := svc.caller(human); err != nil || got.UUID != user {
		t.Errorf("a human: %v, %v; want their own id", got, err)
	}
}
