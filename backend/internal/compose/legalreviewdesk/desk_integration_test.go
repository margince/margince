// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package legalreviewdesk_test

// The legal review desk against a real roster and the real notice lane. Each
// reviewer gets one inbox entry and one in-app notice. A seat without the grants
// gets neither. Settling the review clears both.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/approvalseat"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/legalreviewdesk"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/database"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	legalReviewerGrants = `{"objects":{"legal_review":{"read":true,"update":true}},"row_scope":"all"}`
	dealOnlyGrants      = `{"objects":{"deal":{"read":true,"update":true}},"row_scope":"all"}`
)

// noMail fails the test when the notice lane stages a mail job: reviewers are
// told in the app only.
type noMail struct{ t *testing.T }

func (m noMail) EnqueueTx(_ context.Context, _ pgx.Tx, args river.JobArgs, _ *river.InsertOpts) error {
	m.t.Errorf("a %s mail job was staged for a legal review", args.Kind())
	return nil
}

type deskEnv struct {
	e    *integration.Env
	db   *database.DB
	desk legalreviewdesk.Desk
	lane *compose.ApprovalNotify
}

func newDeskEnv(t *testing.T) deskEnv {
	t.Helper()
	e := integration.Setup(t)
	db := compose.InstallationDB(e.Pool)
	return deskEnv{
		e: e, db: db, lane: compose.NewApprovalNotify(e.Pool, db, noMail{t: t}),
		desk: legalreviewdesk.Desk{Approvals: compose.NewApprovalsService(db), Notices: notices.NewStore(db)},
	}
}

func (d deskEnv) grant(t *testing.T, user ids.UUID, permissions string) {
	t.Helper()
	key := "deskrole-" + user.String()
	d.e.WsExec(t, `INSERT INTO role (key, name, permissions) VALUES ($1, 'Desk role', $2::jsonb)`, key, permissions)
	d.e.WsExec(t, `INSERT INTO role_assignment (role_id, user_id) SELECT r.id, $1 FROM role r WHERE r.key = $2`, user, key)
}

func (d deskEnv) ctx() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), d.e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalSystem, ID: "system:test"})
}

func (d deskEnv) inTx(t *testing.T, fn func(context.Context, pgx.Tx) error) {
	t.Helper()
	ctx := d.ctx()
	if err := d.db.Tx(ctx, func(tx pgx.Tx) error { return fn(ctx, tx) }); err != nil {
		t.Fatal(err)
	}
}

func (d deskEnv) query(t *testing.T, sql string, scan func(pgx.Rows) error) {
	t.Helper()
	ctx := d.ctx()
	if err := database.WithWorkspaceTx(ctx, d.e.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, sql)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			if err := scan(rows); err != nil {
				return err
			}
		}
		return rows.Err()
	}); err != nil {
		t.Fatal(err)
	}
}

func (d deskEnv) deliverAll(t *testing.T) {
	t.Helper()
	var envelopes []kevents.Envelope
	d.query(t, `SELECT envelope FROM event_outbox WHERE envelope->>'type' = 'approval.requested' ORDER BY id`,
		func(r pgx.Rows) error {
			var env kevents.Envelope
			envelopes = append(envelopes, env)
			return r.Scan(&envelopes[len(envelopes)-1])
		})
	for _, env := range envelopes {
		if err := d.lane.HandleEvent(context.Background(), env); err != nil {
			t.Fatal(err)
		}
	}
}

func (d deskEnv) count(t *testing.T, sql string) int {
	t.Helper()
	n := 0
	d.query(t, sql, func(pgx.Rows) error { n++; return nil })
	return n
}

func TestEveryReviewerGetsOneEntryAndOneNoticeAndSettlingClearsBoth(t *testing.T) {
	t.Parallel()
	d := newDeskEnv(t)
	d.grant(t, d.e.AdminUser, legalReviewerGrants)
	d.grant(t, d.e.Rep3, legalReviewerGrants)
	d.grant(t, d.e.Rep2, dealOnlyGrants)
	for _, seat := range []ids.UUID{d.e.AdminUser, d.e.Rep3} {
		d.e.WsExec(t, `INSERT INTO notification_preference (user_id, class, delivery) VALUES ($1, $2, 'email')`,
			seat, notices.ClassApprovalPending)
	}

	var reviewers []ids.UUID
	d.inTx(t, func(ctx context.Context, tx pgx.Tx) (err error) {
		reviewers, err = d.desk.Reviewers(ctx, tx)
		return err
	})
	if len(reviewers) != 2 {
		t.Fatalf("reviewers = %v, want the two seats holding the review grants and not the third", reviewers)
	}

	review := ids.NewV7()
	request := deals.ReviewRequest{ReviewID: review, DealID: ids.NewV7(), DealName: "Fleet renewal", Seats: reviewers}
	for range 2 {
		d.inTx(t, func(ctx context.Context, tx pgx.Tx) error { return d.desk.Raise(ctx, tx, request) })
	}
	pending := `SELECT 1 FROM approval WHERE kind = 'legal_review_request' AND status = 'pending'`
	if n := d.count(t, pending); n != 2 {
		t.Fatalf("inbox entries = %d, want one per reviewer after a repeated request", n)
	}
	d.deliverAll(t)
	live := `SELECT 1 FROM notice WHERE retracted_at IS NULL`
	if n := d.count(t, live); n != 2 {
		t.Fatalf("live notices = %d, want one per reviewer", n)
	}
	if n := d.count(t, live+` AND recipient_user_id = '`+d.e.Rep2.String()+`'`); n != 0 {
		t.Errorf("a seat without the review grants got %d notices", n)
	}

	d.inTx(t, func(ctx context.Context, tx pgx.Tx) error { return d.desk.Settle(ctx, tx, review) })
	if n := d.count(t, pending); n != 0 {
		t.Errorf("inbox entries after settling = %d, want none", n)
	}
	if n := d.count(t, live); n != 0 {
		t.Errorf("live notices after settling = %d, want none", n)
	}
}

func TestAReviewerWhoLosesTheGrantsIsNoLongerAskedAboutTheReview(t *testing.T) {
	t.Parallel()
	d := newDeskEnv(t)
	d.grant(t, d.e.AdminUser, legalReviewerGrants)
	request := deals.ReviewRequest{ReviewID: ids.NewV7(), DealID: ids.NewV7(), DealName: "Fleet renewal", Seats: []ids.UUID{d.e.AdminUser}}
	d.inTx(t, func(ctx context.Context, tx pgx.Tx) error { return d.desk.Raise(ctx, tx, request) })
	var entry ids.ApprovalID
	d.query(t, `SELECT id FROM approval WHERE kind = 'legal_review_request' AND status = 'pending'`,
		func(r pgx.Rows) error { return r.Scan(&entry) })
	ident := identity.NewService(d.e.Pool)
	asked := func() bool {
		var ok bool
		d.inTx(t, func(ctx context.Context, tx pgx.Tx) (err error) {
			ok, err = approvalseat.StillDecidable(ctx, tx, ident, d.desk.Approvals, entry, d.e.AdminUser)
			return err
		})
		return ok
	}
	if !asked() {
		t.Fatal("a reviewer who holds the grants is not asked about the review")
	}
	d.e.WsExec(t, `DELETE FROM role_assignment WHERE user_id = $1`, d.e.AdminUser)
	if asked() {
		t.Error("a reviewer who lost the grants is still asked about the review")
	}
}
