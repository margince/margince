// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacyreview

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const approvalHistoryReviews = `SELECT count(*) FROM notice WHERE kind='privacy_review_required' AND target_type='approval_history' AND target_id=$1`

const keptImage = `SELECT count(*) FROM audit_log WHERE id=$1 AND after->>'full_name'='Private Contact'`

// emptiedApproval writes a decided approval an erasure emptied, whose older
// audit image still holds a name.
func emptiedApproval(t *testing.T, e *integration.Env, targetType string, target ids.UUID) (approval, audit ids.UUID) {
	t.Helper()
	approval = ids.NewV7()
	e.WsExec(t, `INSERT INTO approval(id,kind,status,proposed_change,diff_hash,expires_at,proposed_by,summary,target_entity_type,target_entity_id,decided_at)
 VALUES($1,'contact_update','approved','{}','legacy',timestamptz '2030-01-01','system:legacy','',$2,$3,now())`, approval, targetType, target)
	writeAudit(t, e, func(tx pgx.Tx) (err error) {
		audit, err = storekit.AuditWithEvidence(e.Admin(), tx, "create", "approval", approval, nil, map[string]string{"full_name": "Private Contact"}, nil)
		return err
	})
	return approval, audit
}

func writeAudit(t *testing.T, e *integration.Env, write func(pgx.Tx) error) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, write); err != nil {
		t.Fatal(err)
	}
}

func grantAdmin(t *testing.T, e *integration.Env, users ...ids.UUID) {
	t.Helper()
	for _, user := range users {
		e.WsExec(t, `INSERT INTO role_assignment(user_id,role_id) SELECT $1,id FROM role WHERE key='admin' ON CONFLICT DO NOTHING`, user)
	}
}

func runBatch(ctx context.Context, e *integration.Env, step datatask.Step) (int, error) {
	var changed int
	err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		out, err := step.Batch(ctx, tx, datatask.BatchIn{Limit: 1000, RunID: ids.NewV7()})
		changed = out.Changed
		return err
	})
	return changed, err
}

// runThroughEngine executes the step as the worker does, one audit row per batch.
// The engine's own checks on a batch's counts then apply to that row.
func runThroughEngine(ctx context.Context, t *testing.T, e *integration.Env, step datatask.Step) {
	t.Helper()
	now := clocktest.Now(t)
	step.Limit = 1
	task := datatask.Task{ID: "core/1794076293_linked_erased_audit_payloads", Version: 1, Title: "reviews", Steps: []datatask.Step{step}}
	engine, err := datatask.New(e.Pool, []datatask.Task{task}, "test", func() time.Time { return now }, func(context.Context, pgx.Tx, datatask.Progress) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	for range e.WsCount(t, `SELECT count(*) FROM audit_log`) + 2 {
		if _, err := engine.Execute(ctx); err != nil {
			t.Fatal(err)
		}
	}
}

func TestEveryReviewerIsToldOnceAndTheBatchCommits(t *testing.T) {
	e := integration.Setup(t)
	grantAdmin(t, e, e.AdminUser, e.Rep1)
	unproven, audit := emptiedApproval(t, e, "company", e.SeedCompany(t, "Unproven target", nil))
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	step := UnlinkedApprovalReviewStep(notices.NewStore(e.DB()))
	runThroughEngine(ctx, t, e, step)
	for _, reviewer := range []ids.UUID{e.AdminUser, e.Rep1} {
		if n := e.WsCount(t, approvalHistoryReviews+` AND recipient_user_id=$2`, unproven, reviewer); n != 1 {
			t.Fatalf("reviewer %s got %d reviews, want 1", reviewer, n)
		}
	}
	if e.WsCount(t, keptImage, audit) != 1 {
		t.Fatal("the unproven history was changed")
	}
	if changed, err := runBatch(ctx, e, step); err != nil || changed != 0 {
		t.Fatalf("a replay changed %d rows: %v", changed, err)
	}
}

func TestALinkProvedAfterTheCorrectionStepIsCorrectedByTheReviewStep(t *testing.T) {
	e := integration.Setup(t)
	grantAdmin(t, e, e.AdminUser)
	subject := e.SeedContact(t, "Erased Subject", nil)
	approval, audit := emptiedApproval(t, e, "contact", subject)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	if _, err := runBatch(ctx, e, AuditSteps(privacy.RedactLinkedHistoricalAuditPageTx)[0]); err != nil {
		t.Fatal(err)
	}
	writeAudit(t, e, func(tx pgx.Tx) error {
		_, err := storekit.AuditWithEvidence(e.Admin(), tx, "erase", "contact", subject, nil, nil, map[string]any{"cause": "contact_erasure"})
		return err
	})
	if changed, err := runBatch(ctx, e, UnlinkedApprovalReviewStep(notices.NewStore(e.DB()))); err != nil || changed != 1 {
		t.Fatalf("the review step changed %d rows: %v", changed, err)
	}
	if e.WsCount(t, keptImage, audit) != 0 || e.WsCount(t, approvalHistoryReviews, approval) != 0 {
		t.Fatal("a proved link must be corrected, not reviewed")
	}
}

func TestNoReviewerRefusesTheBatchAndWritesNothing(t *testing.T) {
	e := integration.Setup(t)
	e.WsExec(t, `DELETE FROM role_assignment ra USING role r WHERE ra.role_id=r.id AND r.key IN ('admin','ops')`)
	unproven, _ := emptiedApproval(t, e, "company", e.SeedCompany(t, "Unproven target", nil))
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	_, err := runBatch(ctx, e, UnlinkedApprovalReviewStep(notices.NewStore(e.DB())))
	if _, ok := errors.AsType[*UnavailableError](err); !ok {
		t.Fatalf("a missing reviewer must refuse the batch: %v", err)
	}
	if n := e.WsCount(t, approvalHistoryReviews, unproven); n != 0 {
		t.Fatalf("wrote %d reviews with nobody to read them", n)
	}
}
