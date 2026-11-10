// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAHoldingAnswerRetractsPreviouslyOpenedSiblings(t *testing.T) {
	for _, old := range []string{capture.VerdictCleared, capture.VerdictSharedByOwner} {
		t.Run(old, func(t *testing.T) {
			e := integration.Setup(t)
			older := seedHeldThreadMail(t, e, "reopened-thread", "customer@example.test", "Earlier exchange")
			judged := seedHeldThreadMail(t, e, "reopened-thread", "counsel@example.test", "Later exchange")
			thread := seedThreadQuestion(t, e, "reopened-thread", judged)
			e.WsExec(t, `UPDATE capture_import SET verdict_status=@status WHERE activity_id=@id`, pgx.StrictNamedArgs{"id": older, "status": old})
			e.WsExec(t, `UPDATE activity SET audience='workspace', audience_reason=NULL WHERE id=@id`, pgx.StrictNamedArgs{"id": older})
			runConfidentiality(t, e, thread, confidentialityPersonnel, 0.95)
			if got := activityAudience(t, e, older); got != "participants" {
				t.Fatalf("earlier mail audience=%s", got)
			}
			if got := importVerdictFor(t, e, older, e.Rep1); got != capture.VerdictHeld {
				t.Fatalf("earlier verdict=%s", got)
			}
			if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id=@id AND after->>'audience'='participants'`, pgx.StrictNamedArgs{"id": older}); n != 1 {
				t.Fatalf("audience audit count=%d", n)
			}
		})
	}
}

func TestHistoricalHoldRepairDrainsForEveryClosingVerdict(t *testing.T) {
	for _, status := range []string{capture.VerdictHeld, capture.VerdictUnsure, capture.VerdictHeldByOwner} {
		t.Run(status, func(t *testing.T) {
			e := integration.Setup(t)
			mail := seedHeldThreadMail(t, e, "historical-hold", "customer@example.test", "Earlier exchange")
			thread := seedThreadQuestion(t, e, "historical-hold", mail)
			e.WsExec(t, `UPDATE capture_thread_verdict SET status=@status,kind='personnel' WHERE id=@id`, pgx.StrictNamedArgs{"id": thread, "status": status})
			e.WsExec(t, `UPDATE capture_import SET verdict_status='cleared' WHERE activity_id=@id`, pgx.StrictNamedArgs{"id": mail})
			e.WsExec(t, `UPDATE activity SET audience='workspace', audience_reason=NULL WHERE id=@id`, pgx.StrictNamedArgs{"id": mail})
			ctx := principal.SystemActing(e.Admin(), "system:hold-repair-test")
			task := threadHoldClosesDataTask(e.Pool)
			if got := estimateThreadHolds(ctx, t, e, task); got != 1 {
				t.Fatalf("repair estimate=%d", got)
			}
			runMachineAddressTask(ctx, t, e, task)
			if got := activityAudience(t, e, mail); got != "participants" {
				t.Fatalf("repaired audience=%s", got)
			}
			if got := estimateThreadHolds(ctx, t, e, task); got != 0 {
				t.Fatalf("remaining repair rows=%d", got)
			}
		})
	}
}

func TestASiblingHoldRollsBackWithItsAudienceChange(t *testing.T) {
	e := integration.Setup(t)
	mail := seedHeldThreadMail(t, e, "rolled-back-hold", "customer@example.test", "Earlier exchange")
	e.WsExec(t, `UPDATE capture_import SET verdict_status='cleared' WHERE activity_id=@id`, pgx.StrictNamedArgs{"id": mail})
	store := capture.NewThreadVerdictStore(InstallationDB(e.Pool))
	ctx := e.Admin()
	err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		out, err := store.RecordOutcomeOnThreadTx(ctx, tx, "rolled-back-hold", e.Rep1, capture.VerdictHeld, "personnel", nil)
		if err != nil {
			return err
		}
		if len(out.Stamped) != 1 || out.Stamped[0] != ids.UUID(mail) {
			t.Fatalf("stamped=%v", out.Stamped)
		}
		return context.Canceled
	})
	if err == nil {
		t.Fatal("transaction did not roll back")
	}
	if got := importVerdictFor(t, e, mail, e.Rep1); got != capture.VerdictCleared {
		t.Fatalf("rolled back verdict=%s", got)
	}
}

func TestASettledHoldProtectsHistoryBeforeTheBoundedSweepFinishes(t *testing.T) {
	e := integration.Setup(t)
	var last ids.UUID
	for range 502 {
		last = seedHeldThreadMail(t, e, "long-held-history", "customer@example.test", "Earlier exchange")
	}
	thread := seedThreadQuestion(t, e, "long-held-history", last)
	e.WsExec(t, `UPDATE capture_import i SET verdict_status='cleared' FROM activity a WHERE a.id=i.activity_id AND a.thread_key=@thread`, pgx.StrictNamedArgs{"thread": "long-held-history"})
	e.WsExec(t, `UPDATE activity SET audience='workspace',audience_reason=NULL WHERE thread_key=@thread`, pgx.StrictNamedArgs{"thread": "long-held-history"})
	e.WsExec(t, `UPDATE capture_thread_verdict SET status='held',kind='personnel' WHERE id=@id`, pgx.StrictNamedArgs{"id": thread})
	reader := e.As(e.Rep3, []ids.UUID{e.Team2}, integration.AdminPerms)
	if _, err := e.Activities.GetActivityContent(reader, ids.From[ids.ActivityKind](last), storekit.LiveOnly); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("outside reader before reconciliation: %v", err)
	}
	if _, err := e.Activities.GetActivityContent(e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms), ids.From[ids.ActivityKind](last), storekit.LiveOnly); err != nil {
		t.Fatalf("importing owner: %v", err)
	}
	repairCtx := principal.SystemActing(e.Admin(), "system:hold-repair-test")
	task := threadHoldClosesDataTask(e.Pool)
	if err := database.WithWorkspaceTx(repairCtx, e.Pool, func(tx pgx.Tx) error {
		_, err := task.Steps[0].Batch(repairCtx, tx, datatask.BatchIn{Limit: 200, RunID: ids.NewV7()})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got := importVerdictFor(t, e, last, e.Rep1); got != capture.VerdictCleared {
		t.Fatalf("the later batch was not left pending: %s", got)
	}
	ctx := e.Admin()
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		return activities.RecomputeAudienceTx(ctx, tx, ids.From[ids.ActivityKind](last))
	}); err != nil {
		t.Fatal(err)
	}
	if got := activityAudience(t, e, last); got != "participants" {
		t.Fatalf("recompute ignored authoritative hold: %s", got)
	}
}

func TestAClassifiersHoldDoesNotReplaceAnExistingOwnerHold(t *testing.T) {
	e := integration.Setup(t)
	mail := seedHeldThreadMail(t, e, "owner-held-history", "customer@example.test", "Earlier exchange")
	e.WsExec(t, `UPDATE capture_import SET verdict_status='held_by_owner',verdict_reason='owner' WHERE activity_id=@id`, pgx.StrictNamedArgs{"id": mail})
	store := capture.NewThreadVerdictStore(InstallationDB(e.Pool))
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		out, err := store.RecordOutcomeOnThreadTx(e.Admin(), tx, "owner-held-history", e.Rep1, capture.VerdictHeld, "personnel", nil)
		if err != nil {
			return err
		}
		if len(out.Stamped) != 0 {
			t.Fatalf("owner hold was overwritten: %v", out.Stamped)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := importVerdictFor(t, e, mail, e.Rep1); got != capture.VerdictHeldByOwner {
		t.Fatalf("owner hold lost its provenance: %s", got)
	}
}
