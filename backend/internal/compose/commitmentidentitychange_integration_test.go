// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestResolvingAPromiserPreservesTheExistingHumanDecision(t *testing.T) {
	for _, state := range []string{"pending", "rejected", "approved"} {
		t.Run(state, func(t *testing.T) {
			e := setupTranscript(t)
			e.WsExec(t, `UPDATE app_user SET display_name='Priya Raman' WHERE id=$1`, e.Rep2)
			first := e.read(t, cannedBrain{reply: ownedReply(t, "Priya Raman", 0.95)})
			if len(first.ProposalIDs) != 1 {
				t.Fatalf("unresolved promise created %d proposals", len(first.ProposalIDs))
			}
			if state != "pending" {
				reason := "Reviewed this promise"
				if _, err := e.svc.Decide(e.ctx, ids.From[ids.ApprovalKind](first.ProposalIDs[0]), state == "approved", &reason); err != nil {
					t.Fatal(err)
				}
			}
			before := e.taskCount(t)
			again := e.read(t, cannedBrain{reply: attributedReply(t, "Priya Raman", "seat:"+e.Rep2.String(), 0.95)})
			if got := e.taskCount(t); got != before || len(again.ProposalIDs) != 0 {
				t.Fatalf("resolving attribution bypassed %s decision: tasks %d -> %d, new proposals %d", state, before, got, len(again.ProposalIDs))
			}
			if got := e.WsCount(t, `SELECT count(*) FROM approval WHERE kind='commitment_task'`); got != 1 {
				t.Fatalf("same source clause acquired %d decisions", got)
			}
		})
	}
}

func TestLosingPromiserAttributionDoesNotRepeatExistingWork(t *testing.T) {
	for _, state := range []string{"direct", "pending", "rejected", "approved"} {
		t.Run(state, func(t *testing.T) {
			e := setupTranscript(t)
			e.WsExec(t, `UPDATE app_user SET display_name='Priya Raman' WHERE id=$1`, e.Rep2)
			confidence := 0.75
			if state == "direct" {
				confidence = 0.95
			}
			first := e.read(t, cannedBrain{reply: attributedReply(t, "Priya Raman", "seat:"+e.Rep2.String(), confidence)})
			if state == "rejected" || state == "approved" {
				if len(first.ProposalIDs) != 1 {
					t.Fatalf("expected one initial proposal, got %d", len(first.ProposalIDs))
				}
				if _, err := e.svc.Decide(e.As(e.Rep2, []ids.UUID{e.Team1}, integration.AdminPerms), ids.From[ids.ApprovalKind](first.ProposalIDs[0]), state == "approved", new("Reviewed this promise")); err != nil {
					t.Fatal(err)
				}
			}
			before := e.taskCount(t)
			again := e.read(t, cannedBrain{reply: ownedReply(t, "Priya Raman", 0.95)})
			if got := e.taskCount(t); got != before || len(again.ProposalIDs) != 0 {
				t.Fatalf("lost attribution repeated %s work: tasks %d -> %d, new proposals %d", state, before, got, len(again.ProposalIDs))
			}
		})
	}
}

func TestRereadingAnApprovedPromiseDoesNotBypassItsFailedEffect(t *testing.T) {
	e := setupTranscript(t)
	e.WsExec(t, `UPDATE app_user SET display_name='Priya Raman' WHERE id=$1`, e.Rep2)
	first := e.read(t, cannedBrain{reply: attributedReply(t, "Priya Raman", "seat:"+e.Rep2.String(), 0.75)})
	if len(first.ProposalIDs) != 1 {
		t.Fatal("expected a proposal")
	}
	e.svc.WithEffect(CommitmentTaskKind, func(context.Context, ids.ApprovalID, json.RawMessage, string) error {
		return errors.New("task writer unavailable")
	})
	ctx := e.As(e.Rep2, []ids.UUID{e.Team1}, integration.AdminPerms)
	if _, err := e.svc.Decide(ctx, ids.From[ids.ApprovalKind](first.ProposalIDs[0]), true, nil); err == nil {
		t.Fatal("failed effect was reported as successful")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM approval WHERE status='approved' AND effect_failed_at IS NOT NULL`); got != 1 {
		t.Fatal("fixture did not retain the approved failed effect")
	}
	e.WsExec(t, `UPDATE approval SET decided_at='2001-01-01T00:00:00Z' WHERE id=$1`, first.ProposalIDs[0])
	again := e.read(t, cannedBrain{reply: attributedReply(t, "Priya Raman", "seat:"+e.Rep2.String(), 0.95)})
	if e.taskCount(t) != 0 || len(again.ProposalIDs) != 0 {
		t.Fatal("rereading bypassed the existing approved effect")
	}
	failed, _, err := e.svc.ListWire(ctx, approvals.ListInput{FailedForDecider: true, Limit: 8})
	if err != nil || len(failed) != 1 || ids.UUID(failed[0].Id) != first.ProposalIDs[0] {
		t.Fatalf("expired failed effect disappeared: rows=%d err=%v", len(failed), err)
	}
}
