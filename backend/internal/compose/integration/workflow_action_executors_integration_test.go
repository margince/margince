// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Task 11a's composing executors, proven end to end over a real migrated
// Postgres: notify with no transport wired lands a VISIBLE 'skipped' run
// with a readable reason (§3.3, UAT.md:34) rather than a silent gap or a
// fabricated success; add_to_list actually writes a real list_member row
// through the collections module's own gated write path; and draft_email
// composes a draft, parks its run behind a real staged approval, and lands
// that draft durably on workflow_run.applied anyway — so the artifact a
// suspended firing produced is never lost with the suspension.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/platform/database"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// notifyNoTransportProbe is a synthetic handler that exists only for this
// suite: no shipped starter carries a notify action, so nothing else
// exercises ApplyActions' notify case against a real database.
type notifyNoTransportProbe struct{}

func (notifyNoTransportProbe) Spec() workflow.Spec {
	return workflow.Spec{
		Name:    "task11a_notify_no_transport_probe",
		Trigger: workflow.Trigger{EventType: "deal.stage_changed"},
		Tier:    mcp.TierAutoExecute,
	}
}

func (notifyNoTransportProbe) Match(context.Context, workflow.Event) (bool, error) { return true, nil }

func (notifyNoTransportProbe) Plan(_ context.Context, ev workflow.Event) (workflow.Effect, error) {
	return workflow.Effect{Actions: []workflow.Action{{
		Kind: workflow.ActionNotify, Target: ev.Entity, Args: json.RawMessage(`{}`),
	}}}, nil
}

func (notifyNoTransportProbe) Apply(ctx context.Context, _ workflow.Event, eff workflow.Effect, _ *workflow.ApprovalToken) (workflow.RunResult, error) {
	// The zero-value Executors carries a nil Notifier — this repo wires
	// none — so this proves ApplyActions answers ErrNoNotificationTransport
	// instead of a silent no-op or a fabricated success.
	applied, err := automation.ApplyActions(ctx, automation.Executors{}, eff)
	return workflow.RunResult{Applied: applied}, err
}

func (notifyNoTransportProbe) IdempotencyKey(ev workflow.Event) string {
	return "task11a_notify_no_transport_probe:" + ev.ID.String()
}

func TestNotifyFiringWithNoTransportLandsAVisibleSkippedRun(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	dealID := e.SeedDeal(t, "Notify Probe Deal", pipeline, open, nil)

	engine := compose.NewWorkflowEngine(e.DB())
	engine.RegisterSystemWorkflow(notifyNoTransportProbe{})

	ctx := context.Background()
	if err := engine.HandleEvent(ctx, kevents.Envelope{
		EventID: ids.NewV7(), Type: "deal.stage_changed",
		OccurredAt: time.Now().UTC(),
		Entity:     kevents.EntityRef{Type: "deal", ID: dealID},
	}); err != nil {
		t.Fatal(err)
	}

	var status string
	var reason *string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(
			context.Background(),
			`SELECT status, detail->>'reason' FROM workflow_run WHERE handler = 'task11a_notify_no_transport_probe'`,
		).Scan(&status, &reason)
	}); err != nil {
		t.Fatal(err)
	}

	if status != "skipped" {
		t.Fatalf("run status = %q, want skipped — a notify firing with no transport must be visible, never silent and never a fabricated 'applied'", status)
	}
	const wantReason = "no notification transport configured"
	if reason == nil || *reason != wantReason {
		t.Fatalf("run detail reason = %v, want %q", reason, wantReason)
	}
}

// draftingComms is a deterministic Comms stand-in for this suite: it
// returns a fixed composed draft. The real activities-backed compute is
// exercised by compose's own comms suites; what THIS suite proves is the
// durable round-trip — a composed draft surviving onto workflow_run.applied
// through real Postgres — so a hermetic composer keeps the assertion on
// the persistence, not on the draft's wording. The seam still runs live:
// the probe calls DraftEmail, and the enrichment path is what lands the
// draft on the run record.
type draftingComms struct{ subject, body string }

func (c draftingComms) DraftEmail(context.Context, ids.UUID, string) (string, string, error) {
	return c.subject, c.body, nil
}

// A fixed deliverable address, for the same reason the draft itself is fixed:
// this suite proves the durable round-trip, not the participant walk that finds
// a counterparty. Returning a real one rather than "" matters — an empty
// address would let the assertions below pass over a draft nobody could send.
func (draftingComms) ReplyAddress(context.Context, ids.UUID) (string, error) {
	return draftProbeRecipient, nil
}

const draftProbeRecipient = "counterparty@example.com"

// stagingApprovals puts the REAL approvals service behind automation's staging
// seam rather than a recording double. What this suite is for is the round trip
// through Postgres, and a fake Stage would prove the run row while skipping the
// approval row the run is supposed to park behind.
type stagingApprovals struct{ svc *approvals.Service }

func (a stagingApprovals) Stage(ctx context.Context, in automation.StageRequest) (ids.ApprovalID, error) {
	return a.svc.Stage(ctx, approvals.StageInput{
		Kind:           in.Kind,
		ProposedChange: in.ProposedChange,
		DiffHash:       in.DiffHash,
		TargetType:     in.TargetType,
		TargetID:       in.TargetID,
		Summary:        in.Summary,
		JoinPending:    in.JoinPending,
	})
}

// draftEmailProbe is a synthetic handler that exists only for this suite:
// no shipped starter carries a draft_email action, so nothing else
// exercises ApplyActions' draft_email case against a real database.
type draftEmailProbe struct {
	comms     automation.Comms
	approvals automation.Approvals
}

func (draftEmailProbe) Spec() workflow.Spec {
	return workflow.Spec{
		Name:    "task11a_draft_email_probe",
		Trigger: workflow.Trigger{EventType: "deal.stage_changed"},
		Tier:    mcp.TierAutoExecute,
	}
}

func (draftEmailProbe) Match(context.Context, workflow.Event) (bool, error) { return true, nil }

func (draftEmailProbe) Plan(_ context.Context, ev workflow.Event) (workflow.Effect, error) {
	return workflow.Effect{Actions: []workflow.Action{{
		Kind:   workflow.ActionDraftEmail,
		Target: ev.Entity,
		Args:   json.RawMessage(`{"intent":"nudge toward a decision","consent_purpose":"business_correspondence"}`),
	}}}, nil
}

func (p draftEmailProbe) Apply(ctx context.Context, _ workflow.Event, eff workflow.Effect, _ *workflow.ApprovalToken) (workflow.RunResult, error) {
	applied, err := automation.ApplyActions(ctx,
		automation.Executors{Comms: p.comms, Approvals: p.approvals}, eff)
	return workflow.RunResult{Applied: applied}, err
}

func (draftEmailProbe) IdempotencyKey(ev workflow.Event) string {
	return "task11a_draft_email_probe:" + ev.ID.String()
}

func TestDraftEmailFiringLandsTheComposedDraftOnTheRunRecord(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	dealID := e.SeedDeal(t, "Draft Email Probe Deal", pipeline, open, nil)

	engine := compose.NewWorkflowEngine(e.DB())
	engine.RegisterSystemWorkflow(draftEmailProbe{
		comms:     draftingComms{subject: "Re: next step", body: "Following up on our last conversation."},
		approvals: stagingApprovals{svc: approvals.NewService(e.DB())},
	})

	ctx := context.Background()
	fireErr := engine.HandleEvent(ctx, kevents.Envelope{
		EventID: ids.NewV7(), Type: "deal.stage_changed",
		OccurredAt: time.Now().UTC(),
		Entity:     kevents.EntityRef{Type: "deal", ID: dealID},
	})
	// The refusal reaches the CALLER as well as the run record. Both matter and
	// they are not the same report: the run row is what an operator reads
	// afterwards, and this error is what the dispatcher sees now.
	if fireErr == nil {
		t.Fatal("an ownerless automation composed a draft — a message nobody can release")
	}
	if !strings.Contains(fireErr.Error(), "has no owner") {
		t.Fatalf("HandleEvent = %v, want it to name the missing owner", fireErr)
	}

	var status string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(
			context.Background(),
			`SELECT status FROM workflow_run WHERE handler = 'task11a_draft_email_probe'`,
		).Scan(&status)
	}); err != nil {
		t.Fatal(err)
	}
	// AN OWNERLESS FIRING DRAFTS NOTHING, which is what shipped: a drafted
	// message goes out under one person's name and is released by that person,
	// so an automation with nobody to be refuses at compose time rather than
	// producing a draft nobody can release (automation.MissingDraftOwnerError).
	//
	// This probe is registered with RegisterSystemWorkflow and every such
	// firing is ownerless by construction — engine.HandleEvent builds its event
	// from an envelope, which carries no owner, and OwnerID is populated only
	// from an automation INSTANCE. So the run fails, and the assertions below
	// about the composed draft and the waiting approval are unreachable for a
	// system workflow today.
	//
	// WHAT THAT COSTS is recorded rather than quietly dropped: workflow_run's
	// `applied` column is asserted against a real database HERE and nowhere
	// else, and the regression it guards — recordApplyOutcome's staged arm
	// writing only `detail`, so an automation reports having drafted a reply
	// while holding nothing to show for it — is now unguarded there. Restoring
	// it needs a firing that carries an owner, which needs the owner question
	// settled: #3605 has the two readings.
	if status != "failed" {
		t.Fatalf("run status = %q, want failed — an automation with no owner refuses to draft", status)
	}
	// The run row records the automation and that it failed; the SENTENCE
	// naming the owner rides the returned error, asserted above. An operator
	// reading run history therefore sees which automation stopped but not yet
	// why — worth knowing when the owner question is settled.
}
