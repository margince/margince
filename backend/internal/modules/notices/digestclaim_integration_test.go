// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package notices

// The morning digest's own two halves against a real database: the claim that
// makes one message a day, and the content read that refuses to run as anybody
// but its recipient.
//
// The lane that uses them is tested where it lives (compose). What is here is
// what only the store can answer: that the claim is conditional and unreleasable,
// that a cause cannot be recorded against a morning nobody claimed, and that the
// principal guard on the content read is real rather than a comment.

import (
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// digestDay is the local day a morning is filed under, in LocalDayAt's own
// convention: the installation's date carried at UTC midnight.
var digestDay = time.Date(2026, 6, 4, 0, 0, 0, 0, time.UTC)

// raiseAutomation records one notice the way the automation engine's notify
// action does.
func (e *noticeEnv) raiseAutomation(t *testing.T, recipient ids.UserID, subject string) ids.UUID {
	t.Helper()
	id, err := e.store.Create(e.engineCtx(), NewNotice{
		Recipient: recipient,
		Kind:      "automation",
		Subject:   subject,
		DedupeKey: "digest_claim_suite:" + recipient.String() + ":" + subject,
	})
	if err != nil {
		t.Fatalf("raising %q: %v", subject, err)
	}
	return id
}

func TestTheDigestClaimIsTakenOnceAndNeverReleased(t *testing.T) {
	e := setupNotices(t)

	claimed, err := e.store.ClaimDigestRun(e.engineCtx(), e.recipient, digestDay)
	if err != nil {
		t.Fatalf("claiming the morning: %v", err)
	}
	if !claimed {
		t.Fatal("the first tick of the morning did not take the claim")
	}

	// A second tick of the same local day finds it spent. This is the whole of
	// what stands between an hourly pass and twenty-four messages.
	again, err := e.store.ClaimDigestRun(e.engineCtx(), e.recipient, digestDay)
	if err != nil {
		t.Fatalf("the second tick failed rather than declining: %v", err)
	}
	if again {
		t.Fatal("the morning was claimed twice")
	}

	// Recording a failure does NOT hand the morning back: the attempt is spent
	// either way, and a releasable claim is the retry loop this design refuses.
	if err := e.store.DigestFailed(e.engineCtx(), e.recipient, digestDay, "relay refused"); err != nil {
		t.Fatalf("recording the cause: %v", err)
	}
	afterFailure, err := e.store.ClaimDigestRun(e.engineCtx(), e.recipient, digestDay)
	if err != nil {
		t.Fatalf("claiming after a recorded failure: %v", err)
	}
	if afterFailure {
		t.Fatal("recording why a morning failed released its claim, so the next tick would send again")
	}

	// The next local day is a new morning and gets its own.
	tomorrow, err := e.store.ClaimDigestRun(e.engineCtx(), e.recipient, digestDay.AddDate(0, 0, 1))
	if err != nil {
		t.Fatalf("claiming the next morning: %v", err)
	}
	if !tomorrow {
		t.Fatal("the next local day could not be claimed; the claim is per day, not per seat")
	}
}

// A cause beside no claim describes a send that never happened, and the row an
// operator reads to find out why a message did not arrive must not fill up with
// them.
func TestADigestFailureWithoutAClaimIsRefused(t *testing.T) {
	e := setupNotices(t)

	err := e.store.DigestFailed(e.engineCtx(), e.recipient, digestDay, "relay refused")
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("recording a cause against an unclaimed morning answered %v, want the absent sentinel", err)
	}
}

// THE CONTENT READ IS THE RECIPIENT'S OWN ACT. A digest read under anybody
// else's principal would resolve every record reference in it against somebody
// else's scope, which is the one mistake this lane exists to make impossible —
// so the store refuses rather than trusting its caller to have bound the right
// seat.
func TestAMorningDigestIsOnlyReadableByItsOwnRecipient(t *testing.T) {
	e := setupNotices(t)
	if _, err := e.store.SaveNotificationPreference(
		e.asUser(e.recipient), classAutomation, DeliveryDigest); err != nil {
		t.Fatalf("routing the class into the batch: %v", err)
	}
	e.raiseAutomation(t, e.recipient, "A proposal you own was accepted")

	if _, err := e.store.DigestBody(e.asUser(e.other), e.recipient, digestDay); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a colleague read somebody else's digest and got %v", err)
	}
	// The engine's own system principal is refused too: there is no human behind
	// it, so there is no scope for the re-scoping to run against.
	if _, err := e.store.DigestBody(e.engineCtx(), e.recipient, digestDay); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("the system principal read a colleague's digest and got %v", err)
	}

	held, err := e.store.DigestBody(e.asUser(e.recipient), e.recipient, digestDay)
	if err != nil {
		t.Fatalf("the recipient reading their own digest: %v", err)
	}
	if len(held) != 1 || held[0].Subject != "A proposal you own was accepted" {
		t.Fatalf("the recipient's own digest holds %+v", held)
	}
}

// The enumeration answers WHO is owed a morning, and it answers it from the
// routing choice rather than from the presence of a notice: a colleague who
// reads their queue on screen has notices waiting and is owed no message.
func TestOnlySeatsWhoBatchedAClassAreDigestCandidates(t *testing.T) {
	e := setupNotices(t)
	if _, err := e.store.SaveNotificationPreference(
		e.asUser(e.recipient), classAutomation, DeliveryDigest); err != nil {
		t.Fatalf("routing the class into the batch: %v", err)
	}
	e.raiseAutomation(t, e.recipient, "A proposal you own was accepted")
	e.raiseAutomation(t, e.other, "A renewal you own is due next week")

	due, err := e.store.DigestCandidates(e.engineCtx(), digestDay)
	if err != nil {
		t.Fatalf("listing the seats due a digest: %v", err)
	}
	if len(due) != 1 || due[0] != e.recipient {
		t.Fatalf("the pass is due %v, want only the seat who asked for a batch (%s)", due, e.recipient)
	}

	// Once the day is claimed, the seat leaves the list: the anti-join is what
	// keeps the hourly tick from re-reading a morning already sent.
	if _, err := e.store.ClaimDigestRun(e.engineCtx(), e.recipient, digestDay); err != nil {
		t.Fatalf("claiming the morning: %v", err)
	}
	after, err := e.store.DigestCandidates(e.engineCtx(), digestDay)
	if err != nil {
		t.Fatalf("listing the seats due a digest after the claim: %v", err)
	}
	if len(after) != 0 {
		t.Fatalf("a seat already served is still due a morning: %v", after)
	}
}
