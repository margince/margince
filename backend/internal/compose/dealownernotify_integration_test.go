// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// reassignAsTheProduct moves the deal's owner through the deal store every
// handler is built with, so the hand-over is the one production runs.
func (e *reconcileEnv) reassignAsTheProduct(t *testing.T, deal ids.UUID, to *ids.UUID) {
	t.Helper()
	change := deals.UpdateDealInput{Clear: []string{"owner_id"}}
	if to != nil {
		change = deals.UpdateDealInput{OwnerID: new(ids.From[ids.UserKind](*to))}
	}
	store := deals.NewStore(e.DB(), DealsInstallation())
	if _, err := store.UpdateDeal(e.Admin(), ids.From[ids.DealKind](deal), change); err != nil {
		t.Fatalf("reassigning the deal: %v", err)
	}
}

// tellPreviousOwner gives the deal's first owner the line the staging
// announcement gives them, through the same writer.
func (e *reconcileEnv) tellPreviousOwner(t *testing.T, seat ids.UUID, id ids.ApprovalID) {
	t.Helper()
	err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
		_, err := notices.NewStore(nil).CreateTx(e.Admin(), tx, approvalNoticeFor(seat, id, "Follow up", nil, notices.Target{}))
		return err
	})
	if err != nil {
		t.Fatalf("giving the previous owner their line: %v", err)
	}
}

func (e *reconcileEnv) liveLines(t *testing.T, seat ids.UUID, id ids.ApprovalID) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM notice WHERE recipient_user_id = $1 AND dedupe_key = $2
		AND kind = 'approval_pending' AND retracted_at IS NULL`, seat, approvalNoticeDedupeKey(id.UUID))
}

func (e *reconcileEnv) mailJobsFor(t *testing.T, seat ids.UUID) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM river_job j JOIN notice n ON n.id::text = j.args->>'notice_id'
		WHERE j.kind = 'notification_email' AND n.recipient_user_id = $1`, seat)
}

func (e *reconcileEnv) requestedEvents(t *testing.T) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'approval.requested'`)
}

// A request handed to a deal's new owner tells them by the line and the email
// they chose. The previous owner's line is taken back. It writes no
// second approval.requested, so a webhook subscriber sees the request once.
func TestARequestHandedToANewOwnerTellsThem(t *testing.T) {
	e := setupReconcile(t)
	e.GrantRole(t, e.Rep2, "rep")
	deal, id := e.stagedFollowUp(t, "Handed over and told")
	e.tellPreviousOwner(t, e.Rep1, id)
	events := e.requestedEvents(t)

	e.reassignAsTheProduct(t, deal, &e.Rep2)

	if n := e.liveLines(t, e.Rep2, id); n != 1 {
		t.Errorf("the new owner has %d line(s) about the request, want 1", n)
	}
	if n := e.mailJobsFor(t, e.Rep2); n != 1 {
		t.Errorf("%d email(s) staged for the new owner, want 1", n)
	}
	if n := e.liveLines(t, e.Rep1, id); n != 0 {
		t.Errorf("the previous owner still has %d line(s) about a request that is no longer theirs", n)
	}
	if n := e.requestedEvents(t); n != events {
		t.Errorf("the hand-over wrote %d approval.requested event(s), want none", n-events)
	}
}

// A new owner who reads notices only in the product gets the line and no email.
func TestANewOwnerWhoChoseInAppGetsNoEmail(t *testing.T) {
	e := setupReconcile(t)
	e.GrantRole(t, e.Rep2, "rep")
	deal, id := e.stagedFollowUp(t, "Handed to an in-app reader")
	e.WsExec(t, `INSERT INTO notification_preference (user_id, class, delivery) VALUES ($1, $2, $3)`,
		e.Rep2, notices.ClassApprovalPending, notices.DeliveryInApp)

	e.reassignAsTheProduct(t, deal, &e.Rep2)

	if n := e.liveLines(t, e.Rep2, id); n != 1 {
		t.Errorf("the new owner has %d line(s) about the request, want 1", n)
	}
	if n := e.mailJobsFor(t, e.Rep2); n != 0 {
		t.Errorf("%d email(s) staged for a member who chose in-app only, want none", n)
	}
}

// A deal left with no owner withdraws its request. Nobody is told, and the
// previous owner's line about it is taken back.
func TestARequestOnADealLeftWithNoOwnerTellsNobody(t *testing.T) {
	e := setupReconcile(t)
	deal, id := e.stagedFollowUp(t, "Left unowned")
	e.tellPreviousOwner(t, e.Rep1, id)

	e.reassignAsTheProduct(t, deal, nil)

	if status := e.statusOf(t, id); status != "expired" {
		t.Errorf("the request on an unowned deal is %s, want withdrawn", status)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM notice WHERE dedupe_key = $1 AND retracted_at IS NULL`,
		approvalNoticeDedupeKey(id.UUID)); n != 0 {
		t.Errorf("%d line(s) about a withdrawn request, want none", n)
	}
}

// A new owner who may not decide the request is not told about it, as the
// staging announcement would not tell them. The previous owner's line still
// goes, since the request is no longer theirs.
func TestANewOwnerWhoMayNotDecideIsNotTold(t *testing.T) {
	e := setupReconcile(t)
	deal, id := e.stagedFollowUp(t, "Handed to a member with no role")
	e.tellPreviousOwner(t, e.Rep1, id)

	e.reassignAsTheProduct(t, deal, &e.Rep2)

	if n := e.liveLines(t, e.Rep2, id); n != 0 {
		t.Errorf("a member who may not decide the request has %d line(s) about it, want none", n)
	}
	if n := e.mailJobsFor(t, e.Rep2); n != 0 {
		t.Errorf("%d email(s) staged for a member who may not decide, want none", n)
	}
	if n := e.liveLines(t, e.Rep1, id); n != 0 {
		t.Errorf("the previous owner still has %d line(s) about a request that is no longer theirs", n)
	}
}

// A request that comes back to an owner who held it before reaches them again.
// Their earlier line was taken back on the first hand-over. It is opened
// unread, and they are not emailed a second time about the same request.
func TestARequestHandedBackToAFormerOwnerTellsThemAgain(t *testing.T) {
	e := setupReconcile(t)
	e.GrantRole(t, e.Rep1, "rep")
	e.GrantRole(t, e.Rep2, "rep")
	deal, id := e.stagedFollowUp(t, "Handed back")
	e.tellPreviousOwner(t, e.Rep1, id)
	key := approvalNoticeDedupeKey(id.UUID)
	e.WsExec(t, `UPDATE notice SET email_attempted_at = now(), read_at = now()
		WHERE recipient_user_id = $1 AND dedupe_key = $2`, e.Rep1, key)

	e.reassignAsTheProduct(t, deal, &e.Rep2)
	e.reassignAsTheProduct(t, deal, &e.Rep1)

	if n := e.WsCount(t, `SELECT count(*) FROM notice WHERE recipient_user_id = $1 AND dedupe_key = $2
		AND retracted_at IS NULL AND read_at IS NULL`, e.Rep1, key); n != 1 {
		t.Errorf("the owner the request came back to has %d live unread line(s) about it, want 1", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM notice WHERE recipient_user_id = $1 AND dedupe_key = $2
		AND email_attempted_at IS NOT NULL`, e.Rep1, key); n != 1 {
		t.Error("the reopened line lost its mail attempt, so the same member can be emailed again")
	}
	if n := e.liveLines(t, e.Rep2, id); n != 0 {
		t.Errorf("the owner in between still has %d line(s) about it", n)
	}
}
