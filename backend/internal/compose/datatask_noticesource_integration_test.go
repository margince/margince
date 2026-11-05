// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestUnverifiedNoticeCorrectionPagesReplaysAndPreservesConfirmedSources(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	now := clocktest.Now(t)
	unknown := seedDirectSenderDuty(t, e, "unknown_legacy", "open", "connector:gmail")
	assigned := seedDirectSenderDuty(t, e, "unknown_legacy", "assigned", "connector:gmail")
	failed := seedDirectSenderDuty(t, e, "unknown_legacy", "delivery_failed", "connector:gmail")
	blocked := seedDirectSenderDuty(t, e, "unknown_legacy", "blocked", "connector:gmail")
	queued := seedDirectSenderDuty(t, e, "unknown_legacy", "queued", "connector:gmail")
	referral := seedDirectSenderDuty(t, e, "referral", "open", "human:seed")
	e.WsExec(t, `UPDATE contact_acquisition_evidence SET occurred_at = @at WHERE contact_id = @contact`, pgx.NamedArgs{"contact": referral, "at": now})
	task := noticeSourceDataTask(func() time.Time { return now })
	task.Steps[0].Limit, task.Steps[0].MinLimit = 1, 1
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		count, more, err := task.Steps[0].Estimate(ctx, tx, 1)
		if err == nil && (count != 1 || !more) {
			t.Fatalf("estimate %d/%t", count, more)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	runMachineAddressTask(ctx, t, e, task)
	runMachineAddressTask(ctx, t, e, task)
	for _, tc := range []struct {
		contact string
		state   string
	}{
		{unknown.String(), "source_review"},
		{blocked.String(), "source_review"},
		{assigned.String(), "source_review"},
		{failed.String(), "source_review"},
		{queued.String(), "queued"},
		{referral.String(), "open"},
	} {
		var state string
		if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT state FROM privacy_notice_case WHERE contact_id = @contact`, pgx.NamedArgs{"contact": tc.contact}).Scan(&state)
		}); err != nil {
			t.Fatal(err)
		}
		if state != tc.state {
			t.Fatalf("state=%s want=%s", state, tc.state)
		}
	}
	if got := e.WsCount(t, `SELECT count(*) FROM privacy_notice_case WHERE state = 'source_review' AND due_at IS NULL AND cardinality(allowed_routes) = 0 AND blocked_reason IS NULL`); got != 4 {
		t.Fatalf("review shape count=%d", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'privacy_notice_case'`); got != 4 {
		t.Fatalf("audit count=%d", got)
	}
}

func TestUnverifiedNoticeCorrectionRollsBackAndRejectsMalformedCursors(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	seedDirectSenderDuty(t, e, "unknown_legacy", "blocked", "connector:gmail")
	task := noticeSourceDataTask(func() time.Time { return clocktest.Now(t) })
	interrupted := errors.New("interrupt correction")
	err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		if _, err := task.Steps[0].Batch(ctx, tx, datatask.BatchIn{Limit: 1}); err != nil {
			return err
		}
		return interrupted
	})
	if !errors.Is(err, interrupted) {
		t.Fatal(err)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM privacy_notice_case WHERE state = 'blocked'`); got != 1 {
		t.Fatal("case escaped rollback")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'privacy_notice_case'`); got != 0 {
		t.Fatal("audit escaped rollback")
	}
	err = database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, err := task.Steps[0].Batch(ctx, tx, datatask.BatchIn{Limit: 1, Cursor: json.RawMessage(`"bad cursor"`)})
		return err
	})
	if err == nil {
		t.Fatal("malformed cursor accepted")
	}
}

func TestUnverifiedNoticeCorrectionUnassignsARetiredLegacyOwner(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	now := clocktest.Now(t)
	contact := seedDirectSenderDuty(t, e, "unknown_legacy", "open", "connector:gmail")
	e.WsExec(t, `UPDATE privacy_notice_case SET owner_user_id = @owner WHERE contact_id = @contact`, pgx.NamedArgs{"owner": e.Rep1, "contact": contact})
	// Legacy rows may predate the guard that refuses retiring a member with unresolved work.
	e.WsExec(t, `UPDATE app_user SET status = 'deactivated', removed_at = @now WHERE id = @owner`, pgx.NamedArgs{"owner": e.Rep1, "now": now})
	runMachineAddressTask(ctx, t, e, noticeSourceDataTask(func() time.Time { return now }))
	if got := e.WsCount(t, `SELECT count(*) FROM privacy_notice_case WHERE state = 'source_review' AND owner_user_id IS NULL`); got != 1 {
		t.Fatalf("unassigned source reviews=%d", got)
	}
}
