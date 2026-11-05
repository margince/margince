// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertest"
	"github.com/riverqueue/river/rivertype"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestNoticeReconciliationKeepsPendingAndRetractsWithdrawnApprovals(t *testing.T) {
	a := newApprovalNotifyEnv(t)
	a.grantRole(t, a.e.AdminUser)
	id, staged := a.stageCorrection(t)
	a.deliver(t, staged)
	worker := newApprovalNoticeReconcileWorker(a.e.Pool)
	ctx := principal.SystemActing(a.e.Admin(), approvalNoticeRetractActor)
	after, changed, err := worker.reconcilePage(ctx, ids.Nil)
	if err != nil || changed != 0 || after.IsZero() {
		t.Fatalf("pending page: after=%s changed=%d err=%v", after, changed, err)
	}
	if got := a.unreadFor(t, a.e.AdminUser); got != 1 {
		t.Fatalf("pending decision has %d notices, want 1", got)
	}
	assertReconciledHomeNotices(t, a, 0)
	err = a.db.Tx(ctx, func(tx pgx.Tx) error {
		withdrawn, err := a.svc.WithdrawInTx(ctx, tx, id, "The proposed correction is no longer needed")
		if err == nil && !withdrawn {
			t.Error("pending approval was not withdrawn")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	_, changed, err = worker.reconcilePage(ctx, ids.Nil)
	if err != nil || changed != 1 {
		t.Fatalf("withdrawn page: changed=%d err=%v", changed, err)
	}
	if got := a.unreadFor(t, a.e.AdminUser); got != 0 {
		t.Fatalf("withdrawn decision still has %d notices", got)
	}
	assertReconciledHomeNotices(t, a, 0)
	after, changed, err = worker.reconcilePage(ctx, ids.Nil)
	if err != nil || changed != 0 || !after.IsZero() {
		t.Fatalf("replayed page: after=%s changed=%d err=%v", after, changed, err)
	}
}

func assertReconciledHomeNotices(t *testing.T, a approvalNotifyEnv, want int) {
	t.Helper()
	feed := newAttentionService(a.e.Pool, a.svc, func() time.Time { return homeReadTime })
	day, err := feed.Assemble(a.e.Admin())
	if err != nil {
		t.Fatal(err)
	}
	if day.Counts.Notices == nil || *day.Counts.Notices != want {
		t.Fatalf("Home notice counts=%+v, want %d", day.Counts, want)
	}
}

func TestNoticeReconciliationResumesPastAPageOfPendingDecisions(t *testing.T) {
	a := newApprovalNotifyEnv(t)
	integration.ApplyRiverSchema(t)
	ctx := principal.SystemActing(a.e.Admin(), approvalNoticeRetractActor)
	store := notices.NewStore(a.db)
	var firstApproval ids.ApprovalID
	for range approvalNoticePageSize + 1 {
		id, _ := a.stageCorrection(t)
		if firstApproval.IsZero() {
			firstApproval = id
		}
		if _, err := store.Create(ctx, notices.NewNotice{
			Recipient: ids.From[ids.UserKind](a.e.AdminUser), Kind: notices.KindApprovalPending,
			Subject: "Review a proposal", DedupeKey: approvalNoticeDedupeKey(id.UUID),
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.Create(ctx, notices.NewNotice{Recipient: ids.From[ids.UserKind](a.e.Rep1), Kind: notices.KindApprovalPending, Subject: "Review a shared proposal", DedupeKey: approvalNoticeDedupeKey(firstApproval.UUID)}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(ctx, notices.NewNotice{Recipient: ids.From[ids.UserKind](a.e.AdminUser), Kind: notices.KindApprovalPending, Subject: "Legacy key", DedupeKey: "unreadable-legacy-key"}); err != nil {
		t.Fatal(err)
	}
	missing := ids.NewV7()
	missingNotice, err := store.Create(ctx, notices.NewNotice{
		Recipient: ids.From[ids.UserKind](a.e.AdminUser), Kind: notices.KindApprovalPending,
		Subject: "Review a removed proposal", DedupeKey: approvalNoticeDedupeKey(missing),
	})
	if err != nil {
		t.Fatal(err)
	}
	first := runNoticeReconcileJob(t, a, nil)
	if first.EventKind != river.EventKindJobSnoozed {
		t.Fatalf("first page ended as %s", first.EventKind)
	}
	var cursor struct {
		After ids.UUID `json:"approval_notice_after"`
	}
	if err := json.Unmarshal(first.Job.Metadata, &cursor); err != nil || cursor.After.IsZero() {
		t.Fatalf("cursor did not persist: %s, %v", first.Job.Metadata, err)
	}
	second := runNoticeReconcileJob(t, a, first.Job)
	if second.EventKind != river.EventKindJobSnoozed {
		t.Fatalf("second page ended as %s", second.EventKind)
	}
	if remaining := runNoticeReconcileJob(t, a, second.Job); remaining.EventKind != river.EventKindJobCompleted {
		t.Fatalf("finished scan ended as %s", remaining.EventKind)
	}
	page, err := store.UnreadFor(a.e.Admin(), approvalNoticePageSize+10)
	if err != nil || len(page) != approvalNoticePageSize+2 {
		t.Fatalf("remaining pending notices=%d err=%v", len(page), err)
	}
	for _, notice := range page {
		if notice.ID == missingNotice {
			t.Fatal("missing approval still has an actionable notice")
		}
	}
	// The approval names the admin as its decider, so the copy that reached
	// Rep1 is taken back while it is still pending.
	if got := a.unreadFor(t, a.e.Rep1); got != 0 {
		t.Fatalf("a member the pending approval is not asked of still has %d notices", got)
	}
}

func runNoticeReconcileJob(t *testing.T, a approvalNotifyEnv, row *rivertype.JobRow) *rivertest.WorkResult {
	t.Helper()
	spec, ok := jobs.SpecFor(ApprovalNoticeReconcileArgs{}.Kind())
	if !ok {
		t.Fatal("reconciliation job is not declared")
	}
	worker := rivertest.NewWorker(t, riverpgxv5.New(a.e.Pool), &river.Config{},
		jobs.Govern(newApprovalNoticeReconcileWorker(a.e.Pool), spec))
	tx, err := a.e.Pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(context.Background()); err != nil && err != pgx.ErrTxClosed {
			t.Error(err)
		}
	}()
	var result *rivertest.WorkResult
	if row == nil {
		result, err = worker.Work(t.Context(), t, tx, ApprovalNoticeReconcileArgs{}, nil)
	} else {
		result, err = worker.WorkJob(t.Context(), t, tx, row)
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestNotificationDeliveryWaitsForWithdrawalBeforeCopyingItsSummary(t *testing.T) {
	a := newApprovalNotifyEnv(t)
	a.grantRole(t, a.e.AdminUser)
	id, staged := a.stageCorrection(t)
	done := make(chan error, 1)
	ctx := principal.SystemActing(a.e.Admin(), approvalNoticeRetractActor)
	err := a.db.Tx(ctx, func(tx pgx.Tx) error {
		withdrawn, err := a.svc.WithdrawInTx(ctx, tx, id, "The proposal is no longer needed")
		if err != nil {
			return err
		}
		if !withdrawn {
			t.Fatal("pending proposal was not withdrawn")
		}
		var blocker int
		if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&blocker); err != nil {
			return err
		}
		go func() { done <- a.notify.HandleEvent(context.Background(), staged) }()
		waitForBackendBlockedBy(t, tx, blocker, done)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got := a.delivered(t); len(got) != 0 {
		t.Fatalf("withdrawn summary was copied: %+v", got)
	}
}

func TestReconciliationPreservesPrivatePendingNoticesWithoutInventingADecider(t *testing.T) {
	a := newApprovalNotifyEnv(t)
	_, staged := a.stage(a.proposerCtx(a.e.Rep1), t, approvals.StageInput{
		Kind: approvals.KindVolumeRelease, ProposedChange: json.RawMessage(`{"passport_label":"Research"}`),
		DiffHash: "private-stepup", Summary: "Research is asking to keep reading",
	})
	a.deliver(t, staged)
	before := a.delivered(t)
	if len(before) != 1 || before[0].recipient != a.e.Rep1 {
		t.Fatalf("private notice: %+v", before)
	}
	ctx := principal.SystemActing(a.e.Admin(), approvalNoticeRetractActor)
	_, changed, err := newApprovalNoticeReconcileWorker(a.e.Pool).reconcilePage(ctx, ids.Nil)
	if err != nil || changed != 0 {
		t.Fatalf("private pending reconciliation: changed=%d err=%v", changed, err)
	}
	after := a.delivered(t)
	if len(after) != 1 || after[0].subject != before[0].subject || after[0].id != before[0].id {
		t.Fatalf("pending notice text changed: %+v", after)
	}
	if got := a.unreadFor(t, a.e.Rep1); got != 1 {
		t.Fatalf("private pending notice count=%d", got)
	}
}

// A sender question only its mailbox owner may open is taken back from anybody
// else it was announced to. That is the shape a question staged for a fallback
// colleague left behind: the notice went to one member, the payload names another.
func TestNoticeReconciliationRetractsASenderQuestionAnnouncedToAnotherMember(t *testing.T) {
	a := newApprovalNotifyEnv(t)
	key := "senderdecider-" + a.e.Rep1.String()
	a.e.WsExec(t, `INSERT INTO role (key, name, permissions) VALUES ($1, 'Sender decider', $2::jsonb)`, key,
		`{"objects":{"activity":{"read":true},"contact":{"read":true,"create":true},"company":{"read":true,"create":true}},"row_scope":"all"}`)
	a.e.WsExec(t, `INSERT INTO role_assignment (role_id, user_id) SELECT r.id, $1 FROM role r WHERE r.key = $2`, a.e.Rep1, key)
	mail := seedCapturedMail(t, a.e, "gail@sender.example", "hello")
	body, err := json.Marshal(approvalsubject.Counterparty{
		DispositionID: ids.NewV7(), Email: "gail@sender.example", Domain: "sender.example", OwnerID: a.e.Rep1, ActivityID: mail,
	})
	if err != nil {
		t.Fatal(err)
	}
	id, staged := a.stage(a.proposerCtx(ids.Nil), t, approvals.StageInput{
		Kind: approvalsubject.KindCounterparty, ProposedChange: body, DiffHash: "gail",
		TargetType: counterpartyTargetType, TargetID: mail, Summary: "Add gail@sender.example",
	})
	a.deliver(t, staged)
	if got := a.unreadFor(t, a.e.Rep1); got != 1 {
		t.Fatalf("the mailbox owner has %d notices, want 1", got)
	}
	a.e.WsExec(t, `UPDATE approval SET proposed_change = jsonb_set(proposed_change, '{owner_id}', to_jsonb($2::text))
		 WHERE id = $1`, id, a.e.Rep2.String())

	sweep := principal.SystemActing(a.e.Admin(), approvalNoticeReconcileActor)
	if _, changed, err := newApprovalNoticeReconcileWorker(a.e.Pool).reconcilePage(sweep, ids.Nil); err != nil || changed != 1 {
		t.Fatalf("the reconcile sweep took back %d notices (err %v), want the one announced to a member who cannot open it", changed, err)
	}
	if got := a.unreadFor(t, a.e.Rep1); got != 0 {
		t.Fatalf("the member the question is not for still has %d notices", got)
	}
}

// A pending approval announced to everybody who could decide it, as it was
// before each request named one decider, keeps only its decider's copy.
func TestNoticeReconciliationTakesBackAPendingApprovalsCopyFromAMemberItIsNotFor(t *testing.T) {
	a := newApprovalNotifyEnv(t)
	a.grantRole(t, a.e.AdminUser)
	id, staged := a.stageCorrection(t)
	a.deliver(t, staged)
	ctx := principal.SystemActing(a.e.Admin(), approvalNoticeRetractActor)
	if _, err := notices.NewStore(a.db).Create(ctx, notices.NewNotice{
		Recipient: ids.From[ids.UserKind](a.e.Rep1), Kind: notices.KindApprovalPending,
		Subject: stagedSummary, DedupeKey: approvalNoticeDedupeKey(id.UUID),
	}); err != nil {
		t.Fatal(err)
	}

	sweep := principal.SystemActing(a.e.Admin(), approvalNoticeReconcileActor)
	if _, changed, err := newApprovalNoticeReconcileWorker(a.e.Pool).reconcilePage(sweep, ids.Nil); err != nil || changed != 1 {
		t.Fatalf("the sweep took back %d notices (err %v), want the copy for the member it is not for", changed, err)
	}
	if got := a.unreadFor(t, a.e.Rep1); got != 0 {
		t.Errorf("the member the approval is not for still has %d notices", got)
	}
	if got := a.unreadFor(t, a.e.AdminUser); got != 1 {
		t.Errorf("the decider has %d notices, want their own one", got)
	}
}
