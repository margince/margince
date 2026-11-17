// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents/runner"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

type sendingBrain struct{ call string }

func (sendingBrain) PromptWindow() int { return runner.MinimumPromptWindow }
func (b sendingBrain) Complete(context.Context, model.Request) (model.Response, runner.Meta, error) {
	return model.Response{Text: b.call, OutputTokens: 10}, runner.Meta{}, nil
}

func TestUnattendedSendPausesAndOneApprovalDoesNotReleaseTheNextSend(t *testing.T) {
	approvals := &recordingApprovals{}
	comms := &recordingComms{}
	registry := NewRegistry(approvals, auth.NewGate(fullSeatAuthority{}))
	RegisterCommsTools(registry, comms, &multiLinkProvider{})
	args := accountSendArgs(companyLink(ids.NewV7()))
	brain := sendingBrain{call: fmt.Sprintf(`{"tool":"send_company_email","args":%s}`, args)}
	job := runner.Job{Goal: "Prepare follow-ups", Tools: []string{"send_company_email"}}
	r := runner.New(registry, brain)
	ctx := sendCtx()
	actor, _ := principal.Actor(ctx)
	actor.SeatType = principal.SeatFull
	ctx = principal.WithActor(ctx, actor)
	result, err := r.Run(ctx, job)
	if err != nil || result.Outcome != runner.OutcomeAwaitingApproval || result.Pending == nil {
		t.Fatalf("run = %+v, %v; want a pending send", result, err)
	}
	if comms.accountSent != nil || len(approvals.staged) != 1 {
		t.Fatalf("send reached provider before approval: links=%v, staged=%d", comms.accountSent, len(approvals.staged))
	}
	firstPending := *result.Pending
	wantRedemption := fmt.Sprintf("%s %s %s", firstPending.ApprovalID, firstPending.Tool, approvals.staged[0].DiffHash)
	result, err = r.Resume(ctx, job, runner.Decision{Pending: firstPending, Approved: true})
	if err != nil || result.Outcome != runner.OutcomeAwaitingApproval {
		t.Fatalf("resume = %+v, %v; want next send to wait", result, err)
	}
	if len(comms.accountSent) != 1 || len(approvals.redeemed) != 1 || len(approvals.staged) != 2 {
		t.Fatalf("want one authorised send and another staged: links=%v, redeemed=%d, staged=%d", comms.accountSent, len(approvals.redeemed), len(approvals.staged))
	}
	if approvals.redeemed[0] != wantRedemption {
		t.Fatalf("redeemed %q, want the first staged call %q", approvals.redeemed[0], wantRedemption)
	}
}

func TestDirectSendDoesNotRequireASecondApproval(t *testing.T) {
	approvals := &recordingApprovals{}
	comms := &recordingComms{}
	registry := NewRegistry(approvals, auth.NewGate(fullSeatAuthority{}))
	RegisterCommsTools(registry, comms, &multiLinkProvider{})
	if _, err := registry.Invoke(sendCtx(), "send_company_email", accountSendArgs(companyLink(ids.NewV7()))); err != nil {
		t.Fatal(err)
	}
	if len(comms.accountSent) != 1 || len(approvals.staged) != 0 {
		t.Fatalf("direct send: links=%v, staged=%d", comms.accountSent, len(approvals.staged))
	}
}
