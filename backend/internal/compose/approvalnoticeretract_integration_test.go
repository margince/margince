// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Taking back the lines a settled card left in other seats' queues.
//
// Asserted through the reader's own unread lane rather than off the column,
// because the column is only half the fix: a retraction the lane predicate does
// not read leaves the badge exactly as it was, which is the symptom.

import (
	"context"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// unreadFor answers what one seat's badge counts, THROUGH THE STORE'S OWN
// lane read. A hand-written count over the column would assert the migration
// and nothing else: the half that makes this a fix is the lane predicate
// learning to skip a retracted line, and that predicate lives in the store.
func (a approvalNotifyEnv) unreadFor(t *testing.T, seat ids.UUID) int {
	t.Helper()
	unread, err := notices.NewStore(a.db).UnreadFor(
		a.e.As(seat, nil, principal.Permissions{RowScope: principal.RowScopeOwn}), 50)
	if err != nil {
		t.Fatalf("reading seat %s's unread notices: %v", seat, err)
	}
	return len(unread)
}

// Only the member a request is for is told, and a decision takes the line back.
//
// Two seats may answer the card, but it names the deal's owner, so only the
// owner is told. Once it is decided, the owner's line no longer counts: the card
// refuses when opened, and a badge that kept counting it would never clear.
func TestADecidedApprovalTakesBackEveryDecidersNotice(t *testing.T) {
	a := newApprovalNotifyEnv(t)
	a.grantRole(t, a.e.AdminUser)
	a.grantRole(t, a.e.Rep2)

	approvalID, staged := a.stageCorrection(t)
	a.deliver(t, staged)
	if got := len(a.delivered(t)); got != 1 {
		t.Fatalf("one member is asked to decide this; %d notice(s) were written", got)
	}
	if got := a.unreadFor(t, a.e.AdminUser); got != 1 {
		t.Fatalf("the deal's owner has %d unread notice(s) before the decision, want 1", got)
	}
	if got := a.unreadFor(t, a.e.Rep2); got != 0 {
		t.Fatalf("a colleague who may decide but was not asked has %d notice(s), want 0", got)
	}

	retract := NewApprovalNoticeRetract(
		a.e.Pool, notices.NewStore(a.db), identity.NewService(a.e.Pool), slog.New(slog.DiscardHandler))
	if err := retract.HandleEvent(context.Background(), decidedEnvelope(t, approvalID, "approved", deals.CloseDateCorrectionKind)); err != nil {
		t.Fatalf("the retraction refused the verdict: %v", err)
	}

	for _, seat := range []ids.UUID{a.e.AdminUser, a.e.Rep2} {
		if got := a.unreadFor(t, seat); got != 0 {
			t.Errorf("seat %s still counts %d unread notice(s) for a card somebody already decided", seat, got)
		}
	}
	// The ROW stays: the unique index on (recipient, dedupe_key) is what makes a
	// redelivered announcement free, and a deleted line would let the next
	// replay of the staging envelope put the request back.
	if got := len(a.delivered(t)); got != 1 {
		t.Errorf("%d notice row(s) survive the retraction, want the one — deleting it reopens the replay", got)
	}
	a.deliver(t, staged)
	for _, seat := range []ids.UUID{a.e.AdminUser, a.e.Rep2} {
		if got := a.unreadFor(t, seat); got != 0 {
			t.Errorf("replaying the staging envelope put seat %s's line back (%d unread)", seat, got)
		}
	}
}

// A redelivered verdict changes nothing, which is what lets the bus deliver it
// twice: the retraction matches only lines not already taken back, so the
// instant the first one wrote is the one that stands.
func TestARedeliveredVerdictRetractsNothingTwice(t *testing.T) {
	a := newApprovalNotifyEnv(t)
	a.grantRole(t, a.e.AdminUser)

	approvalID, staged := a.stageCorrection(t)
	a.deliver(t, staged)

	retract := NewApprovalNoticeRetract(
		a.e.Pool, notices.NewStore(a.db), identity.NewService(a.e.Pool), slog.New(slog.DiscardHandler))
	verdict := decidedEnvelope(t, approvalID, "approved", deals.CloseDateCorrectionKind)
	if err := retract.HandleEvent(context.Background(), verdict); err != nil {
		t.Fatalf("the retraction refused the verdict: %v", err)
	}
	var first string
	a.read(t, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT retracted_at::text FROM notice LIMIT 1`).Scan(&first)
	})

	if err := retract.HandleEvent(context.Background(), verdict); err != nil {
		t.Fatalf("the retraction refused a redelivered verdict: %v", err)
	}
	var second string
	a.read(t, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT retracted_at::text FROM notice LIMIT 1`).Scan(&second)
	})
	if first != second {
		t.Errorf("a redelivered verdict moved the retraction from %s to %s", first, second)
	}
}

// The consumer group carries every approval event, not only decisions, so a
// staging envelope has to pass through untouched. Answering an error on one
// would stall the group; retracting on one would take back the line the SAME
// envelope just caused.
func TestAStagingEnvelopeRetractsNothing(t *testing.T) {
	a := newApprovalNotifyEnv(t)
	a.grantRole(t, a.e.AdminUser)

	_, staged := a.stageCorrection(t)
	a.deliver(t, staged)

	retract := NewApprovalNoticeRetract(
		a.e.Pool, notices.NewStore(a.db), identity.NewService(a.e.Pool), slog.New(slog.DiscardHandler))
	if err := retract.HandleEvent(context.Background(), staged); err != nil {
		t.Fatalf("the retraction refused an envelope that is not a decision: %v", err)
	}
	if got := a.unreadFor(t, a.e.AdminUser); got != 1 {
		t.Errorf("the seat has %d unread notice(s) after a staging envelope reached the retraction, want the one it was just told about", got)
	}
}
