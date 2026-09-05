// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package automation

// The match-time AUDIENCE gate over a real migrated Postgres.
//
// Its sibling next door (gate_integration_test.go) proves the OBJECT gate: the
// owner's live RBAC still permits the planned action. This proves the other
// question on the same firing — may the owner READ the record it fired on —
// and the two fail independently. An owner holding every permission in the
// tree still may not read a message limited to its participants, and the object
// gate cannot see that, because no grant carries a row's audience.
//
// Every case seeds a real activity and drives HandleEvent. The audience is a
// column, so a fixture that stubbed it would be asserting its own answer.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/database"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// fullyPermittedOwner grants everything the scripted handler's action needs, so
// the object gate always passes and only the audience can block. A fixture that
// withheld a permission would prove the wrong gate.
func fullyPermittedOwner() fixtureResolver {
	return fixtureResolver{rbac: authz.RBAC{Permissions: principal.Permissions{
		RowScope: principal.RowScopeAll,
		Objects: map[string]principal.ObjectGrant{
			"activity": {Create: true, Read: true, Update: true, Delete: true},
			"deal":     {Create: true, Read: true, Update: true, Delete: true},
			"person":   {Create: true, Read: true, Update: true, Delete: true},
			"lead":     {Create: true, Read: true, Update: true, Delete: true},
		},
	}}}
}

// activityEventFor is a firing naming one activity as its subject, the shape
// engagement.reply carries (its payload's EntityType() is "activity", and the
// entity id is the newly captured inbound message).
func activityEventFor(activity ids.UUID) kevents.Envelope {
	return kevents.Envelope{
		EventID:    ids.NewV7(),
		Type:       scriptedTrigger,
		OccurredAt: time.Date(2026, 7, 16, 9, 0, 0, 0, time.UTC),
		Entity:     kevents.EntityRef{Type: "activity", ID: activity},
	}
}

// seedActivity writes one message with the given audience, captured by SOMEBODY
// ELSE. Both halves matter: the audience arm admits a row's own capturer
// (`captured_by LIKE '%:<uuid>'`), so an activity the automation's owner
// captured is one they may read whatever its audience, and a fixture built that
// way would pass against a gate that did nothing.
func (fx *autoFixture) seedActivity(t *testing.T, audience string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	fx.exec(t, `
		INSERT INTO activity (id, kind, audience, source, captured_by, occurred_at)
		VALUES ($1, 'email', $2, 'gmail', 'connector:gmail:'||$3::text, now())`,
		id, audience, fx.rep2)
	return id
}

func TestAFiringOnAMessageTheOwnerCannotReadIsBlocked(t *testing.T) {
	fx := setupAutomationDB(t)
	applyCalls := 0
	handler := scriptedWorkflow{
		name: "reply_on_held_mail",
		apply: func(workflow.Event) (workflow.RunResult, error) {
			applyCalls++
			return workflow.RunResult{}, nil
		},
	}
	fx.seedAutomationWithOwner(t, handler.name, fx.rep1)
	held := fx.seedActivity(t, "participants")

	engine := NewWorkflowEngine(database.BindTo(fx.pool, ids.From[ids.WorkspaceKind](fx.ws)),
		fullyPermittedOwner())
	engine.RegisterWorkflow(handler)

	if err := engine.HandleEvent(context.Background(), activityEventFor(held)); err != nil {
		t.Fatalf("HandleEvent err = %v, want nil — a gate block is a recorded outcome, "+
			"not a dispatch failure", err)
	}

	run, ok := fx.runsByHandler(t)[handler.name]
	if !ok {
		t.Fatal("no run row recorded for the blocked firing")
	}
	if run.status != "blocked" {
		t.Fatalf("run.status = %q, want %q", run.status, "blocked")
	}
	detail, err := parseRunDetail(run.detail)
	if err != nil {
		t.Fatalf("parsing run detail: %v", err)
	}
	if !strings.Contains(detail.Reason, "read") {
		t.Errorf("run detail reason = %q, want it to say the owner cannot read the message — "+
			"a reason naming a permission would send the reader to the wrong gate", detail.Reason)
	}
	if applyCalls != 0 {
		t.Errorf("Apply called %d times, want 0: the firing would have created a task naming "+
			"a contact and a moment its owner may not read", applyCalls)
	}
}

func TestAFiringOnAMessageTheOwnerCanReadProceeds(t *testing.T) {
	// The admit case. Without it a gate that blocked every firing would pass
	// every other test in this file.
	fx := setupAutomationDB(t)
	applyCalls := 0
	handler := scriptedWorkflow{
		name: "reply_on_open_mail",
		apply: func(workflow.Event) (workflow.RunResult, error) {
			applyCalls++
			return workflow.RunResult{}, nil
		},
	}
	fx.seedAutomationWithOwner(t, handler.name, fx.rep1)
	open := fx.seedActivity(t, "workspace")

	engine := NewWorkflowEngine(database.BindTo(fx.pool, ids.From[ids.WorkspaceKind](fx.ws)),
		fullyPermittedOwner())
	engine.RegisterWorkflow(handler)

	if err := engine.HandleEvent(context.Background(), activityEventFor(open)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	run, ok := fx.runsByHandler(t)[handler.name]
	if !ok {
		t.Fatal("no run row recorded")
	}
	if run.status != "applied" {
		t.Errorf("run.status = %q, want applied: the message is workspace-visible, so the "+
			"firing must proceed exactly as before", run.status)
	}
	if applyCalls != 1 {
		t.Errorf("Apply called %d times, want 1", applyCalls)
	}
}

func TestAFiringWhoseOwnerCannotReadMessagesAtAllIsBlocked(t *testing.T) {
	// The object half, which the row half cannot answer.
	// EnsureActivityContentVisibleLive tests row and audience scope; it never
	// asks whether this principal may read activities at all. The two come
	// apart because every catalog action touching an activity requires
	// activity.CREATE, and a role's CRUD verbs are independently editable — so
	// an owner granted create but not read cannot open the message through the
	// API, while a gate checking only the row would fire on it. The activity
	// here is workspace-visible, so the row half passes and only the grant can
	// block.
	fx := setupAutomationDB(t)
	applyCalls := 0
	handler := scriptedWorkflow{
		name: "create_but_not_read",
		apply: func(workflow.Event) (workflow.RunResult, error) {
			applyCalls++
			return workflow.RunResult{}, nil
		},
	}
	fx.seedAutomationWithOwner(t, handler.name, fx.rep1)
	open := fx.seedActivity(t, "workspace")

	writeOnly := fixtureResolver{rbac: authz.RBAC{Permissions: principal.Permissions{
		RowScope: principal.RowScopeAll,
		Objects: map[string]principal.ObjectGrant{
			"activity": {Create: true, Update: true, Delete: true},
		},
	}}}
	engine := NewWorkflowEngine(database.BindTo(fx.pool, ids.From[ids.WorkspaceKind](fx.ws)), writeOnly)
	engine.RegisterWorkflow(handler)

	if err := engine.HandleEvent(context.Background(), activityEventFor(open)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	run, ok := fx.runsByHandler(t)[handler.name]
	if !ok {
		t.Fatal("no run row recorded")
	}
	if run.status != "blocked" {
		t.Errorf("run.status = %q, want blocked: the owner may create activities but not read "+
			"them, so they cannot see the message this fired on", run.status)
	}
	if applyCalls != 0 {
		t.Errorf("Apply called %d times, want 0", applyCalls)
	}
}

func TestAFiringOnAnArchivedMessageIsBlocked(t *testing.T) {
	// What "Live" buys. EnsureActivityContentVisible admits an archived row;
	// the Live variant does not. A firing acts NOW, on a record a person can no
	// longer open, so the strict variant is the right one — and swapping it for
	// the lenient one is a one-word edit nothing else would notice.
	fx := setupAutomationDB(t)
	applyCalls := 0
	handler := scriptedWorkflow{
		name: "archived_subject",
		apply: func(workflow.Event) (workflow.RunResult, error) {
			applyCalls++
			return workflow.RunResult{}, nil
		},
	}
	fx.seedAutomationWithOwner(t, handler.name, fx.rep1)
	archived := fx.seedActivity(t, "workspace")
	fx.exec(t, `UPDATE activity SET archived_at = now() WHERE id = $1`, archived)

	engine := NewWorkflowEngine(database.BindTo(fx.pool, ids.From[ids.WorkspaceKind](fx.ws)),
		fullyPermittedOwner())
	engine.RegisterWorkflow(handler)

	if err := engine.HandleEvent(context.Background(), activityEventFor(archived)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if applyCalls != 0 {
		t.Error("the firing applied on an archived message: the check admits rows a person " +
			"can no longer open")
	}
}

func TestAFiringOnAMessageTheOwnerCapturedProceeds(t *testing.T) {
	// The second admit case, and the one that proves the check reads the real
	// audience arm rather than testing `audience = 'workspace'`. The message is
	// limited to its participants, but this owner's own mailbox delivered it —
	// the arm's captured_by clause — so they may read it and the firing runs.
	fx := setupAutomationDB(t)
	applyCalls := 0
	handler := scriptedWorkflow{
		name: "own_mailbox",
		apply: func(workflow.Event) (workflow.RunResult, error) {
			applyCalls++
			return workflow.RunResult{}, nil
		},
	}
	fx.seedAutomationWithOwner(t, handler.name, fx.rep1)
	mine := ids.NewV7()
	fx.exec(t, `
		INSERT INTO activity (id, kind, audience, source, captured_by, occurred_at)
		VALUES ($1, 'email', 'participants', 'gmail', 'connector:gmail:'||$2::text, now())`,
		mine, fx.rep1)

	engine := NewWorkflowEngine(database.BindTo(fx.pool, ids.From[ids.WorkspaceKind](fx.ws)),
		fullyPermittedOwner())
	engine.RegisterWorkflow(handler)

	if err := engine.HandleEvent(context.Background(), activityEventFor(mine)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if applyCalls != 1 {
		t.Errorf("Apply called %d times, want 1: the owner's own mailbox delivered this "+
			"message, so a limited audience still admits them", applyCalls)
	}
}

func TestAFiringOnANonActivitySubjectIsUnaffected(t *testing.T) {
	// The gate is deliberately narrow: audience is an activity's property, and
	// every other subject's visibility follows row scope, which the object gate
	// already resolves. A deal-subject firing must not pay for a check that
	// cannot apply to it — nor be blocked by one.
	fx := setupAutomationDB(t)
	applyCalls := 0
	handler := scriptedWorkflow{
		name: "deal_subject",
		apply: func(workflow.Event) (workflow.RunResult, error) {
			applyCalls++
			return workflow.RunResult{}, nil
		},
	}
	fx.seedAutomationWithOwner(t, handler.name, fx.rep1)

	engine := NewWorkflowEngine(database.BindTo(fx.pool, ids.From[ids.WorkspaceKind](fx.ws)),
		fullyPermittedOwner())
	engine.RegisterWorkflow(handler)

	if err := engine.HandleEvent(context.Background(), gateTestEnvelope(fx.ws)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if applyCalls != 1 {
		t.Errorf("Apply called %d times, want 1 — a deal-subject firing is not this gate's "+
			"business", applyCalls)
	}
}

// A system-seeded automation stamps no owner_id, so there is no authority to
// resolve — and the shipped gate read that as "proceed, unchecked". It is the
// opposite: no authority to check is not a licence, it is the case with nothing
// standing behind it.
//
// post_meeting_recap is what that cost. It fired 2,232 times on a seeded stack
// and planned 468 drafts holding model-written summaries of mail held to its
// participants, with zero blocked runs — because the gate that would have
// blocked them never ran.
func TestAnOwnerlessFiringOnHeldMailIsBlocked(t *testing.T) {
	fx := setupAutomationDB(t)
	applyCalls := 0
	handler := scriptedWorkflow{
		name: "system_seeded",
		apply: func(workflow.Event) (workflow.RunResult, error) {
			applyCalls++
			return workflow.RunResult{}, nil
		},
	}
	fx.seedAutomation(t, handler.name)
	held := fx.seedActivity(t, "participants")

	engine := NewWorkflowEngine(database.BindTo(fx.pool, ids.From[ids.WorkspaceKind](fx.ws)), nil)
	engine.RegisterWorkflow(handler)

	if err := engine.HandleEvent(context.Background(), activityEventFor(held)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if applyCalls != 0 {
		t.Errorf("Apply called %d times, want 0 — an automation nobody owns derived content from a "+
			"message held to its participants", applyCalls)
	}
}

// The other half, and the one that keeps the narrowing from becoming a ban: an
// ownerless automation on mail the whole workspace can read still runs. Without
// this the fix above would be satisfied by disabling the starter automations
// outright, which is not what it is for.
func TestAnOwnerlessFiringOnWorkspaceMailProceeds(t *testing.T) {
	fx := setupAutomationDB(t)
	applyCalls := 0
	handler := scriptedWorkflow{
		name: "system_seeded",
		apply: func(workflow.Event) (workflow.RunResult, error) {
			applyCalls++
			return workflow.RunResult{}, nil
		},
	}
	fx.seedAutomation(t, handler.name)
	open := fx.seedActivity(t, "workspace")

	engine := NewWorkflowEngine(database.BindTo(fx.pool, ids.From[ids.WorkspaceKind](fx.ws)), nil)
	engine.RegisterWorkflow(handler)

	if err := engine.HandleEvent(context.Background(), activityEventFor(open)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if applyCalls != 1 {
		t.Errorf("Apply called %d times, want 1 — a message every seat may read is the most an "+
			"ownerless automation can be asked to respect, and it is readable", applyCalls)
	}
}

// `selected` is the third audience and it is NOT workspace, so it takes the
// same refusal. Asserted separately because a check written as "not
// participants" would pass every test above and admit the narrowest audience
// the product has.
func TestAnOwnerlessFiringOnSelectedMailIsBlocked(t *testing.T) {
	fx := setupAutomationDB(t)
	applyCalls := 0
	handler := scriptedWorkflow{
		name: "system_seeded",
		apply: func(workflow.Event) (workflow.RunResult, error) {
			applyCalls++
			return workflow.RunResult{}, nil
		},
	}
	fx.seedAutomation(t, handler.name)
	narrow := fx.seedActivity(t, "selected")

	engine := NewWorkflowEngine(database.BindTo(fx.pool, ids.From[ids.WorkspaceKind](fx.ws)), nil)
	engine.RegisterWorkflow(handler)

	if err := engine.HandleEvent(context.Background(), activityEventFor(narrow)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if applyCalls != 0 {
		t.Errorf("Apply called %d times, want 0 — `selected` is the narrowest audience there is", applyCalls)
	}
}

func TestTheAudienceGateDoesNotInheritTheEnginesSystemPrincipal(t *testing.T) {
	// The failure this gate is one line away from. The engine binds
	// PrincipalSystem for attribution, and auth.ActivityAudienceArm answers TRUE
	// for a system principal — so a check that used the caller's context would
	// admit every firing and look exactly like a working gate.
	//
	// Asserted by driving the whole engine, which binds that system principal
	// before runOne: if the gate inherited it, the held case below would apply.
	fx := setupAutomationDB(t)
	applyCalls := 0
	handler := scriptedWorkflow{
		name: "not_the_system",
		apply: func(workflow.Event) (workflow.RunResult, error) {
			applyCalls++
			return workflow.RunResult{}, nil
		},
	}
	fx.seedAutomationWithOwner(t, handler.name, fx.rep1)
	held := fx.seedActivity(t, "participants")

	engine := NewWorkflowEngine(database.BindTo(fx.pool, ids.From[ids.WorkspaceKind](fx.ws)),
		fullyPermittedOwner())
	engine.RegisterWorkflow(handler)

	// A system principal on the INBOUND context too, which is what the bus
	// consumer carries in production.
	sysCtx := principal.WithActor(context.Background(),
		principal.Principal{Type: principal.PrincipalSystem, ID: "system"})
	if err := engine.HandleEvent(sysCtx, activityEventFor(held)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if applyCalls != 0 {
		t.Error("the firing applied: the audience check ran as the system principal, which " +
			"reads every row, so it admits every firing while looking like a gate")
	}
}
