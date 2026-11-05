// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package notices

// What the mail claim refuses, against a real database.
//
// The lane that drives it is proven end to end in compose, where the sender
// lives. What cannot be reached from there is the pair of refusals underneath:
// a cause offered for a notice nobody claimed, and one offered for a notice
// that does not exist. Both are the same statement declining to write, and the
// table's own CHECK says the same thing from the other side — a cause may not
// stand beside an unspent claim.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A cause is only ever written beside a claim that was actually taken.
//
// Not a defensive check: EmailFailed is what the sender calls when the relay
// refuses, and calling it on an unclaimed notice would mean the sender had
// recorded a failed send for a message it never attempted. The refusal is what
// turns that into an error somebody sees rather than a row that quietly
// contradicts the column beside it.
func TestACauseIsRefusedWithoutTheClaimItExplains(t *testing.T) {
	e := setupNotices(t)
	id, err := e.store.Create(e.engineCtx(), NewNotice{
		Recipient: e.recipient,
		Kind:      "automation",
		Subject:   "A deal you own moved stage",
	})
	if err != nil {
		t.Fatalf("recording the notice: %v", err)
	}

	if err := e.store.EmailFailed(e.engineCtx(), id, "relay refused the recipient"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("recording a cause on an unattempted notice → %v, want %v", err, apperrors.ErrNotFound)
	}

	var cause *string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT email_error FROM notice WHERE id = $1`, id).Scan(&cause); err != nil {
		t.Fatalf("reading the notice back: %v", err)
	}
	if cause != nil {
		t.Errorf("the refusal still wrote a cause beside no attempt: %q", *cause)
	}

	// The claim taken, the same call lands — so the refusal above is about the
	// missing claim and not about EmailFailed being unable to write at all.
	if _, claimed, err := e.store.ClaimEmailAttempt(e.engineCtx(), id); err != nil || !claimed {
		t.Fatalf("claiming the attempt → claimed=%v, err=%v", claimed, err)
	}
	if err := e.store.EmailFailed(e.engineCtx(), id, "relay refused the recipient"); err != nil {
		t.Fatalf("recording the cause on a claimed notice: %v", err)
	}
	if err := e.owner.QueryRow(context.Background(),
		`SELECT email_error FROM notice WHERE id = $1`, id).Scan(&cause); err != nil {
		t.Fatalf("reading the notice back: %v", err)
	}
	if cause == nil || *cause != "relay refused the recipient" {
		t.Errorf("the claimed notice does not carry the cause: %v", cause)
	}
}

// A notice that is not there answers absent rather than pretending to record.
//
// The same branch as the case above, reached the other way: an id that names no
// row. It matters because the sender is handed an id by a job, and a job can
// outlive the notice it names — an erasure, a purge — so "nothing to record" has
// to be an answer rather than a silent success.
func TestACauseForANoticeThatIsGoneReportsItAbsent(t *testing.T) {
	e := setupNotices(t)

	err := e.store.EmailFailed(e.engineCtx(), ids.NewV7(), "relay refused the recipient")
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("recording a cause on a notice that is gone → %v, want %v", err, apperrors.ErrNotFound)
	}
}
