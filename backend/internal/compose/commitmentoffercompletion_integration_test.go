// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestSettledClaimWithdrawsItsUnacceptedTaskAndStaysSettledOnRereading(t *testing.T) {
	for _, outcome := range []string{claimStatusDone, claimStatusDismissed} {
		t.Run(outcome, func(t *testing.T) {
			e := setupCompletionTranscript(t, transcriptBody)
			linkInes(t, e)
			reading := e.readAt(t, cannedBrain{reply: attributedReply(t, "Priya Raman", "seat:"+e.Rep2.String(), 0.75)}, func() time.Time { return completionReadTime })
			if len(reading.ProposalIDs) != 1 {
				t.Fatalf("got %d proposals, want one", len(reading.ProposalIDs))
			}
			assertCommitmentNotice(t, e, reading.ProposalIDs[0])
			assertPromiseProjectionHTTP(t, e, reading.ProposalIDs[0], 1)
			claim, err := ids.Parse(e.wsString(t, `SELECT id::text FROM conversation_claim`))
			if err != nil {
				t.Fatal(err)
			}
			settleClaimHTTP(t, e, claim, outcome)
			settled := keptCommitment{transcriptEnv: e, claim: claim, settle: NewCommitmentSettleTrigger(e.Pool, slog.Default())}
			event := settled.newest(t, eventClaimChanged, `envelope->'payload'->>'claim_id' = $2`, claim.String())
			settled.deliver(t, event)
			settled.deliver(t, event)
			assertSettledCommitmentNoticeGone(t, e)
			assertPromiseProjectionHTTP(t, e, reading.ProposalIDs[0], 0)
			reader := e.As(e.Rep2, []ids.UUID{e.Team1}, transcriptPerms)
			proposal, err := e.svc.GetWire(reader, ids.From[ids.ApprovalKind](reading.ProposalIDs[0]))
			if err != nil {
				t.Fatal(err)
			}
			if proposal.Status != "expired" {
				t.Fatalf("settled promise still has a %s task proposal", proposal.Status)
			}
			again := e.readAt(t, cannedBrain{reply: attributedReply(t, "Priya Raman", "seat:"+e.Rep2.String(), 0.95)}, func() time.Time { return completionReadTime })
			if e.taskCount(t) != 0 || len(again.ProposalIDs) != 0 {
				t.Fatal("rereading a kept promise created fresh work")
			}
			if got := settled.claimStatus(t); got != outcome {
				t.Fatalf("claim became %s after rereading", got)
			}
		})
	}
}

var completionReadTime = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)

func setupCompletionTranscript(t *testing.T, body string) *transcriptEnv {
	t.Helper()
	e := setupTranscript(t)
	e.GrantRole(t, e.AdminUser, "admin")
	e.WsExec(t, `UPDATE app_user SET display_name = 'Priya Raman' WHERE id = $1`, e.Rep2)
	occurred := completionReadTime.Add(-time.Hour)
	if _, err := e.Activities.UpdateActivity(e.Admin(), e.activity, activities.UpdateActivityInput{Body: &body, OccurredAt: &occurred}); err != nil {
		t.Fatal(err)
	}
	return e
}

func TestCompletingOnePromiseKeepsAnotherAskFromTheSameMeetingPending(t *testing.T) {
	e := setupCompletionTranscript(t, transcriptBody+"\nPriya Raman: I'll send the security checklist.")
	linkInes(t, e)
	steps := []proposedStep{
		{Summary: "Send revised pricing", Owner: "Priya Raman", PromiserRef: "seat:" + e.Rep2.String(), SourceLines: []int{3}, Confidence: 0.75, Meaning: &commitmentMeaning{State: "open", DeadlinePrecision: "none"}},
		{Summary: "Send security checklist", Owner: "Priya Raman", PromiserRef: "seat:" + e.Rep2.String(), SourceLines: []int{5}, Confidence: 0.75, Meaning: &commitmentMeaning{State: "open", DeadlinePrecision: "none"}},
	}
	raw, err := json.Marshal(map[string][]proposedStep{"proposals": steps})
	if err != nil {
		t.Fatal(err)
	}
	reading := e.readAt(t, cannedBrain{reply: string(raw)}, func() time.Time { return completionReadTime })
	if len(reading.ProposalIDs) != 2 {
		t.Fatalf("got %d proposals, want two distinct asks", len(reading.ProposalIDs))
	}
	claim, err := ids.Parse(e.wsString(t, `SELECT id::text FROM conversation_claim WHERE body = $1`, "Send revised pricing"))
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Contacts.SettleConversationClaim(e.Admin(), claim, claimStatusDone); err != nil {
		t.Fatal(err)
	}
	settled := keptCommitment{transcriptEnv: e, claim: claim, settle: NewCommitmentSettleTrigger(e.Pool, slog.Default())}
	settled.deliver(t, settled.newest(t, eventClaimChanged, `envelope->'payload'->>'claim_id' = $2`, claim.String()))
	want := map[string]string{"Send revised pricing": "expired", "Send security checklist": "pending"}
	for _, id := range reading.ProposalIDs {
		proposal, err := e.svc.GetWire(e.As(e.Rep2, []ids.UUID{e.Team1}, transcriptPerms), ids.From[ids.ApprovalKind](id))
		if err != nil {
			t.Fatal(err)
		}
		if proposal.Summary == nil {
			t.Fatal("proposal lost its summary")
		}
		if string(proposal.Status) != want[*proposal.Summary] {
			t.Fatalf("%s is %s, want %s", *proposal.Summary, proposal.Status, want[*proposal.Summary])
		}
	}
}

func TestAClaimCompletedBeforeExtractionCreatesNoTaskOrProposal(t *testing.T) {
	e := setupCompletionTranscript(t, transcriptBody)
	linkInes(t, e)
	customer, err := ids.Parse(e.wsString(t, `SELECT id::text FROM contact WHERE full_name = $1`, "Ines Huber"))
	if err != nil {
		t.Fatal(err)
	}
	quote := "Priya Raman: I'll send the revised pricing over by Friday."
	locator := commitmentLocator(e.activity.UUID, claimKindOurs, "seat:"+e.Rep2.String(), "3:"+quote)
	claim, err := e.Contacts.RecordConversationClaim(e.Admin(), contacts.ClaimInput{
		ContactID: ids.From[ids.ContactKind](customer), Kind: claimKindOurs, Body: "Send revised pricing",
		ActivityID: e.activity.UUID, Quote: quote, Source: "manual", Locator: locator,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Contacts.SettleConversationClaim(e.Admin(), ids.UUID(claim.Id), claimStatusDone); err != nil {
		t.Fatal(err)
	}
	for _, confidence := range []float64{0.75, 0.95, 0.75} {
		reading := e.readAt(t, cannedBrain{reply: attributedReply(t, "Priya Raman", "seat:"+e.Rep2.String(), confidence)}, func() time.Time { return completionReadTime })
		if len(reading.ProposalIDs) != 0 || e.taskCount(t) != 0 {
			t.Fatalf("confidence %v recreated work for a completed claim", confidence)
		}
	}
}

func TestCompletedTaskWithoutAContactWithdrawsItsMatchingProposal(t *testing.T) {
	settled, proposal := seedTaskWithProposal(t)
	e := settled.transcriptEnv
	reader := e.As(e.Rep2, []ids.UUID{e.Team1}, transcriptPerms)
	taskID := settled.task
	settled.setDone(t, true)
	event := settled.newest(t, eventActivityUpdated, `envelope->'entity'->>'id' = $2`, taskID.String())
	settled.deliver(t, event)
	settled.deliver(t, event)
	decision, err := e.svc.GetWire(reader, proposal)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Status != "expired" {
		t.Fatalf("completed task still has a %s proposal", decision.Status)
	}
	again := e.readAt(t, cannedBrain{reply: attributedReply(t, "Priya Raman", "seat:"+e.Rep2.String(), 0.95)}, func() time.Time { return completionReadTime })
	if len(again.ProposalIDs) != 0 || e.taskCount(t) != 1 || settled.taskDone(t) != "true" {
		t.Fatal("rereading recreated completed work")
	}
}

func assertCommitmentNotice(t *testing.T, e *transcriptEnv, approval ids.UUID) {
	t.Helper()
	e.GrantRole(t, e.Rep2, "rep")
	source := keptCommitment{transcriptEnv: e}
	event := source.newest(t, "approval.requested", `envelope->'entity'->>'id' = $2`, approval.String())
	notify := NewApprovalNotify(e.Pool, e.DB(), &recordingNoticeMailQueue{})
	if err := notify.HandleEvent(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	store := notices.NewStore(e.DB())
	rows, err := store.UnreadFor(e.As(e.Rep2, []ids.UUID{e.Team1}, transcriptPerms), 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.Kind == notices.KindApprovalPending && row.Target.ID == e.activity.UUID {
			return
		}
	}
	t.Fatal("staged commitment did not notify its responsible owner")
}

func assertSettledCommitmentNoticeGone(t *testing.T, e *transcriptEnv) {
	t.Helper()
	worker := newApprovalNoticeReconcileWorker(e.Pool)
	ctx := principal.SystemActing(e.Admin(), approvalNoticeRetractActor)
	_, changed, err := worker.reconcilePage(ctx, ids.Nil)
	if err != nil || changed == 0 {
		t.Fatalf("settled notice cleanup changed=%d err=%v", changed, err)
	}
	_, changed, err = worker.reconcilePage(ctx, ids.Nil)
	if err != nil || changed != 0 {
		t.Fatalf("replayed cleanup changed=%d err=%v", changed, err)
	}
	rows, err := notices.NewStore(e.DB()).UnreadFor(e.As(e.Rep2, []ids.UUID{e.Team1}, transcriptPerms), 20)
	if err != nil || len(rows) != 0 {
		t.Fatalf("settled commitment notices=%d err=%v", len(rows), err)
	}
}

func TestAnOldTaskCompletionDoesNotSettleAReopenedPromise(t *testing.T) {
	k := seedKeptCommitment(t)
	k.setDone(t, true)
	event := k.newest(t, eventActivityUpdated, `envelope->'entity'->>'id' = $2`, k.task.String())
	k.setDone(t, false)
	k.deliver(t, event)
	if k.claimStatus(t) != "open" || k.taskDone(t) != "false" {
		t.Fatal("old completion settled work the owner reopened")
	}
}

func TestDelayedClaimSettlementCannotWithdrawAnUnreadableProposal(t *testing.T) {
	e := setupCompletionTranscript(t, transcriptBody)
	customer := linkInes(t, e)
	e.WsExec(t, `UPDATE contact SET owner_id = $2 WHERE id = $1`, customer, e.Rep3)
	e.GrantRole(t, e.Rep3, "rep")
	reading := e.readAt(t, cannedBrain{reply: attributedReply(t, "Priya Raman", "seat:"+e.Rep2.String(), 0.75)}, func() time.Time { return completionReadTime })
	if len(reading.ProposalIDs) != 1 {
		t.Fatal("expected one commitment proposal")
	}
	claim, err := ids.Parse(e.wsString(t, `SELECT id::text FROM conversation_claim`))
	if err != nil {
		t.Fatal(err)
	}
	reader := e.As(e.Rep3, []ids.UUID{e.Team2}, transcriptPerms)
	if err := e.Contacts.SettleConversationClaim(reader, claim, claimStatusDone); err != nil {
		t.Fatal(err)
	}
	settled := keptCommitment{transcriptEnv: e, claim: claim, settle: NewCommitmentSettleTrigger(e.Pool, slog.Default())}
	event := settled.newest(t, eventClaimChanged, `envelope->'payload'->>'claim_id' = $2`, claim.String())
	if _, err := e.Activities.SetAudience(e.Admin(), e.activity, activities.SetAudienceInput{Audience: "selected", Members: []activities.AudienceMember{{SubjectType: "user", SubjectID: e.Rep2}}}); err != nil {
		t.Fatal(err)
	}
	if status := e.wsString(t, `SELECT status FROM approval WHERE id = $1`, reading.ProposalIDs[0]); status != "pending" {
		t.Fatalf("fixture proposal already %s before delayed delivery", status)
	}
	settled.deliver(t, event)
	if status := e.wsString(t, `SELECT status FROM approval WHERE id = $1`, reading.ProposalIDs[0]); status != "pending" {
		t.Fatalf("unreadable proposal changed to %s", status)
	}
}

func seedTaskWithProposal(t *testing.T) (keptCommitment, ids.ApprovalID) {
	t.Helper()
	e := setupCompletionTranscript(t, transcriptBody)
	e.GrantRole(t, e.Rep2, "rep")
	e.readAt(t, cannedBrain{reply: attributedReply(t, "Priya Raman", "seat:"+e.Rep2.String(), 0.95)}, func() time.Time { return completionReadTime })
	taskID, err := ids.Parse(e.wsString(t, `SELECT id::text FROM activity WHERE kind = 'task'`))
	if err != nil {
		t.Fatal(err)
	}
	reader := e.As(e.Rep2, []ids.UUID{e.Team1}, transcriptPerms)
	task, err := e.Activities.GetActivity(reader, ids.From[ids.ActivityKind](taskID), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if task.SourceId == nil {
		t.Fatal("commitment task lost its identity")
	}
	raw, err := json.Marshal(CommitmentTaskProposal{SourceActivityID: e.activity.UUID, Summary: "Send revised pricing", Locator: *task.SourceId, SeatID: &e.Rep2, PromiserSeatID: &e.Rep2})
	if err != nil {
		t.Fatal(err)
	}
	canonical, hash, err := diffhash.Canonical(raw)
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := e.svc.Stage(reader, approvals.StageInput{Kind: CommitmentTaskKind, TargetType: "activity", TargetID: e.activity.UUID, ProposedChange: canonical, DiffHash: hash})
	if err != nil {
		t.Fatal(err)
	}
	return keptCommitment{transcriptEnv: e, task: taskID, settle: NewCommitmentSettleTrigger(e.Pool, slog.Default())}, proposal
}

func TestWithdrawalRechecksATaskChangedAfterItsCompletionWasRead(t *testing.T) {
	for _, reopen := range []bool{false, true} {
		t.Run(map[bool]string{false: "edited", true: "reopened"}[reopen], func(t *testing.T) {
			k, proposal := seedTaskWithProposal(t)
			reader := k.As(k.Rep2, []ids.UUID{k.Team1}, transcriptPerms)
			k.setDone(t, true)
			prior, err := k.Activities.GetActivity(reader, ids.From[ids.ActivityKind](k.task), storekit.LiveOnly)
			if err != nil {
				t.Fatal(err)
			}
			patch := activities.UpdateActivityInput{Subject: new("My revised follow-up")}
			if reopen {
				patch.IsDone = new(false)
			}
			if _, err := k.Activities.UpdateActivity(reader, ids.From[ids.ActivityKind](k.task), patch); err != nil {
				t.Fatal(err)
			}
			if err := k.settle.withdrawCompletedTaskOffers(reader, prior); err != nil {
				t.Fatal(err)
			}
			decision, err := k.svc.GetWire(reader, proposal)
			if err != nil {
				t.Fatal(err)
			}
			want := "expired"
			if reopen {
				want = "pending"
			}
			if string(decision.Status) != want {
				t.Fatalf("proposal is %s after task edit, want %s", decision.Status, want)
			}
		})
	}
}

func TestWithdrawalRechecksAClaimReopenedAfterSettlementWasRead(t *testing.T) {
	e := setupCompletionTranscript(t, transcriptBody)
	linkInes(t, e)
	reading := e.readAt(t, cannedBrain{reply: attributedReply(t, "Priya Raman", "seat:"+e.Rep2.String(), 0.75)}, func() time.Time { return completionReadTime })
	if len(reading.ProposalIDs) != 1 {
		t.Fatal("expected one proposed task")
	}
	claim, err := ids.Parse(e.wsString(t, `SELECT id::text FROM conversation_claim`))
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Contacts.SettleConversationClaim(e.Admin(), claim, claimStatusDone); err != nil {
		t.Fatal(err)
	}
	k := keptCommitment{transcriptEnv: e, claim: claim, settle: NewCommitmentSettleTrigger(e.Pool, slog.Default())}
	event := k.newest(t, eventClaimChanged, `envelope->'payload'->>'claim_id' = $2`, claim.String())
	prior, err := e.Contacts.ReadClaimSettlement(e.Admin(), claim, event.Trace.AuditLogID)
	if err != nil || prior == nil {
		t.Fatalf("settlement: %v, %v", prior, err)
	}
	if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error { _, err := e.Contacts.SetClaimDoneTx(e.Admin(), tx, claim, false); return err }); err != nil {
		t.Fatal(err)
	}
	if err := k.settle.withdrawSettledOffers(e.Admin(), settledCommitmentIdentity{sourceID: prior.SourceID, locator: prior.Locator, claimID: &claim, status: prior.Status, settlementID: event.Trace.AuditLogID}); err != nil {
		t.Fatal(err)
	}
	proposal, err := e.svc.GetWire(e.As(e.Rep2, []ids.UUID{e.Team1}, transcriptPerms), ids.From[ids.ApprovalKind](reading.ProposalIDs[0]))
	if err != nil {
		t.Fatal(err)
	}
	if proposal.Status != "pending" || k.claimStatus(t) != "open" {
		t.Fatal("stale settlement withdrew reopened work")
	}
}
