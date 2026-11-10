// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Releasing an at-scale reassignment, on both kinds of system of record. The
// built-in provider writes on the redemption's transaction. An adapter over
// another system cannot, so its failures are recorded and never re-sent.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// remoteRecords stands in for an adapter over another system: it lacks
// recordUpdaterTx, and its Update answers with the error it is given. When
// writes is true the change lands first, as when only the answer is lost.
// afterWrite, when set, runs once the change has landed.
type remoteRecords struct {
	datasource.SystemOfRecordProvider
	writes     bool
	answer     error
	afterWrite func()
}

func (r remoteRecords) Update(ctx context.Context, in datasource.UpdateInput) (datasource.EntityRef, error) {
	if r.writes {
		if _, err := r.SystemOfRecordProvider.Update(ctx, in); err != nil {
			return datasource.EntityRef{}, err
		}
	}
	if r.afterWrite != nil {
		r.afterWrite()
	}
	return datasource.EntityRef{}, r.answer
}

// releaseThrough wires the reassignment effect over the given provider, the
// way coldstartaccept.go wires it over the built-in one.
func releaseThrough(e *integration.Env, provider datasource.SystemOfRecordProvider) *approvals.Service {
	return releaseThroughDB(e, provider, InstallationDB(e.Pool))
}

// releaseThroughDB is releaseThrough with the handle the effect settles its run
// through. The service keeps its own, so the redemption is unaffected.
func releaseThroughDB(e *integration.Env, provider datasource.SystemOfRecordProvider, db *database.DB) *approvals.Service {
	svc := NewApprovalsServiceWithEffects(e.Pool)
	return svc.WithEffect(string(workflow.ActionAssignOwner), assignOwnerReleaseEffect(svc, provider, db))
}

func unconsumed(t *testing.T, e *integration.Env, id ids.ApprovalID) bool {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM approval WHERE id = $1 AND consumed_at IS NULL`, id) == 1
}

func ownerAudits(t *testing.T, e *integration.Env, entity string, record, owner ids.UUID) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM audit_log
		WHERE entity_type = $1 AND entity_id = $2 AND after ->> 'owner_id' = $3`, entity, record, owner.String())
}

// ownerRecord is one record type a reassignment can target, seeded through its
// real writer. The entity name is also its table.
type ownerRecord struct {
	entity string
	seed   func(t *testing.T, e *integration.Env) ids.UUID
}

func ownerRecords() []ownerRecord {
	return []ownerRecord{
		{"contact", func(t *testing.T, e *integration.Env) ids.UUID { return e.SeedContact(t, "Owned Contact", nil) }},
		{"company", func(t *testing.T, e *integration.Env) ids.UUID { return e.SeedCompany(t, "Owned Company", nil) }},
		{"lead", seedOwnedLead},
		{"deal", func(t *testing.T, e *integration.Env) ids.UUID {
			pipeline, open, _ := integration.DealFixture(t, e)
			return e.SeedDeal(t, "Owned Deal", pipeline, open, nil)
		}},
		{"project", seedOwnedProject},
	}
}

func seedOwnedLead(t *testing.T, e *integration.Env) ids.UUID {
	t.Helper()
	lead, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{FullName: new("Owned Lead"), Source: "manual"})
	if err != nil {
		t.Fatalf("seeding the lead: %v", err)
	}
	return ids.UUID(lead.Id)
}

func seedOwnedProject(t *testing.T, e *integration.Env) ids.UUID {
	t.Helper()
	company := e.SeedCompany(t, "Project Anchor", nil)
	ref, err := NewProvider(e.Pool).Create(e.Admin(), datasource.CreateInput{
		EntityType: datasource.EntityProject,
		Fields:     json.RawMessage(`{"name":"Owned Project","company_id":"` + company.String() + `"}`),
		Source:     "test",
	})
	if err != nil {
		t.Fatalf("seeding the project: %v", err)
	}
	return ref.ID
}

// A released reassignment on the built-in provider commits its redemption, the
// owner, the owner's audit row and the run in one transaction. The two audit
// rows sharing one xmin is what proves the write used the caller's transaction.
func TestALocalReassignmentCommitsTheRedemptionTheOwnerAndItsAuditTogether(t *testing.T) {
	e := integration.Setup(t)
	svc := NewApprovalsServiceWithEffects(e.Pool)
	for _, record := range ownerRecords() {
		t.Run(record.entity, func(t *testing.T) {
			target := record.seed(t, e)
			id, handler := parkedRunOn(t, e, svc, string(workflow.ActionAssignOwner), record.entity, target,
				`{"owner_id":"`+e.Rep1.String()+`"}`)

			if _, err := svc.Decide(decider(e), id, true, nil); err != nil {
				t.Fatalf("Decide(approve) → %v", err)
			}
			if unconsumed(t, e, id) {
				t.Error("the reassignment landed and the approval is still unconsumed")
			}
			if n := e.WsCount(t, `SELECT count(*) FROM `+record.entity+` WHERE id = $1 AND owner_id = $2`, target, e.Rep1); n != 1 {
				t.Error("the record kept its old owner")
			}
			if n := ownerAudits(t, e, record.entity, target, e.Rep1); n != 1 {
				t.Errorf("found %d audit rows for the owner change, want 1", n)
			}
			if n := e.WsCount(t, `SELECT count(DISTINCT xmin::text) FROM audit_log
				WHERE (entity_type = 'approval' AND entity_id = $1 AND after ->> 'redeemed' = 'true')
				   OR (entity_type = $2 AND entity_id = $3 AND after ->> 'owner_id' = $4)`,
				id, record.entity, target, e.Rep1.String()); n != 1 {
				t.Errorf("the redemption and the owner change were written by %d transactions, want 1", n)
			}
			if got := runStatus(t, e, handler); got != "applied" {
				t.Errorf("run is %q, want applied", got)
			}
		})
	}
}

// A write that fails on the built-in provider rolls the redemption back with
// it, so the approval stays unconsumed and deciding again runs the write.
//
// The failure comes from the write, after the redemption passed: the new owner
// holds a read seat, which the owner check refuses.
func TestAFailedLocalReassignmentLeavesTheApprovalUnconsumedAndRetryable(t *testing.T) {
	e := integration.Setup(t)
	svc := NewApprovalsServiceWithEffects(e.Pool)
	for _, record := range ownerRecords() {
		t.Run(record.entity, func(t *testing.T) {
			target := record.seed(t, e)
			id, handler := parkedRunOn(t, e, svc, string(workflow.ActionAssignOwner), record.entity, target,
				`{"owner_id":"`+e.Rep2.String()+`"}`)
			e.WsExec(t, `UPDATE app_user SET seat_type = 'read' WHERE id = $1`, e.Rep2)
			t.Cleanup(func() { e.WsExec(t, `UPDATE app_user SET seat_type = 'full' WHERE id = $1`, e.Rep2) })

			_, err := svc.Decide(decider(e), id, true, nil)
			if err == nil || errors.Is(err, apperrors.ErrVersionSkew) {
				t.Fatalf("Decide → %v, want the write's refusal of a read-seat owner", err)
			}
			if !unconsumed(t, e, id) {
				t.Fatal("the write failed and the approval is consumed: the human's yes is spent on nothing")
			}
			if got := runStatus(t, e, handler); got != "requires_approval" {
				t.Errorf("run is %q after its write failed, want requires_approval", got)
			}
			if n := ownerAudits(t, e, record.entity, target, e.Rep2); n != 0 {
				t.Errorf("found %d owner changes after a refused write, want none", n)
			}

			e.WsExec(t, `UPDATE app_user SET seat_type = 'full' WHERE id = $1`, e.Rep2)
			if _, err := svc.Decide(decider(e), id, true, nil); err != nil {
				t.Fatalf("deciding again did not re-drive the reassignment: %v", err)
			}
			if n := ownerAudits(t, e, record.entity, target, e.Rep2); n != 1 {
				t.Errorf("found %d audit rows for the owner change after the retry, want 1", n)
			}
			if got := runStatus(t, e, handler); got != "applied" {
				t.Errorf("run is %q after the retry landed, want applied", got)
			}
		})
	}
}

// An adapter whose answer is lost after the write landed. The approval stays
// spent and the run is failed. The decider is told the change may have
// happened, and deciding again sends nothing.
func TestAnUnconfirmedRemoteReassignmentIsRecordedAndNeverResent(t *testing.T) {
	e := integration.Setup(t)
	svc := releaseThrough(e, remoteRecords{
		SystemOfRecordProvider: NewProvider(e.Pool), writes: true, answer: errors.New("connection reset by peer"),
	})
	contact := e.SeedContact(t, "Answer Lost", nil)
	id, handler := parkedRun(t, e, svc, string(workflow.ActionAssignOwner), contact,
		`{"owner_id":"`+e.Rep1.String()+`"}`)

	_, err := svc.Decide(decider(e), id, true, nil)
	if !errors.Is(err, approvals.ErrEffectUnconfirmed) {
		t.Fatalf("Decide → %v, want ErrEffectUnconfirmed", err)
	}
	if unconsumed(t, e, id) {
		t.Error("an unconfirmed remote write left the approval unconsumed, so deciding again would re-send it")
	}
	if got := runStatus(t, e, handler); got != "failed" {
		t.Errorf("run is %q after an unconfirmed write, want failed", got)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM approval WHERE id = $1 AND effect_failure LIKE '%may have happened%'`, id); n != 1 {
		t.Error("the decider is not told the change may have happened")
	}
	assertNotRetryable(t, e, svc, id)

	if _, err := svc.Decide(decider(e), id, true, nil); err == nil {
		t.Error("deciding an unconfirmed release again reported success")
	}
	if n := ownerAudits(t, e, "contact", contact, e.Rep1); n != 1 {
		t.Errorf("found %d owner changes, want the one that landed and no re-send", n)
	}
}

// An adapter that refuses the write: nothing changed, so the run names the
// refusal and the decider is not told the change may have happened.
func TestARefusedRemoteReassignmentIsRecordedAsRefused(t *testing.T) {
	e := integration.Setup(t)
	svc := releaseThrough(e, remoteRecords{
		SystemOfRecordProvider: NewProvider(e.Pool), answer: apperrors.ErrVersionSkew,
	})
	contact := e.SeedContact(t, "Refused Remotely", nil)
	id, handler := parkedRun(t, e, svc, string(workflow.ActionAssignOwner), contact,
		`{"owner_id":"`+e.Rep1.String()+`"}`)

	_, err := svc.Decide(decider(e), id, true, nil)
	if err == nil || errors.Is(err, approvals.ErrEffectUnconfirmed) {
		t.Fatalf("Decide → %v, want the refusal and not ErrEffectUnconfirmed", err)
	}
	if got := runStatus(t, e, handler); got != "failed" {
		t.Errorf("run is %q after a refused write, want failed", got)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM workflow_run WHERE handler = $1
		AND detail ->> 'reason' = 'the record changed before this action could be applied'`, handler); n != 1 {
		t.Error("the failed run does not name the refusal")
	}
	assertNotRetryable(t, e, svc, id)
}

// assertNotRetryable reads the decider's failed-decision list, the surface
// that offers a retry, and requires this approval on it with no retry offered.
func assertNotRetryable(t *testing.T, e *integration.Env, svc *approvals.Service, id ids.ApprovalID) {
	t.Helper()
	failed, err := svc.OwnFailedDecisions(decider(e), 50)
	if err != nil {
		t.Fatalf("listing failed decisions: %v", err)
	}
	for _, decision := range failed {
		if ids.UUID(decision.Approval.Id) == id.UUID {
			if decision.Retryable {
				t.Error("a spent remote release is offered as retryable, which would re-send the write")
			}
			return
		}
	}
	t.Error("the failed release is missing from the decider's failed decisions")
}

// An adapter that refuses with a typed refusal, here the built-in provider
// turning down a read-seat owner. Nothing landed, so it is recorded as refused.
func TestATypedRemoteRefusalIsRecordedAsRefused(t *testing.T) {
	e := integration.Setup(t)
	svc := releaseThrough(e, remoteRecords{SystemOfRecordProvider: NewProvider(e.Pool), writes: true})
	contact := e.SeedContact(t, "Refused By Seat", nil)
	id, handler := parkedRun(t, e, svc, string(workflow.ActionAssignOwner), contact,
		`{"owner_id":"`+e.Rep2.String()+`"}`)
	e.WsExec(t, `UPDATE app_user SET seat_type = 'read' WHERE id = $1`, e.Rep2)

	_, err := svc.Decide(decider(e), id, true, nil)
	if err == nil || errors.Is(err, approvals.ErrEffectUnconfirmed) {
		t.Fatalf("Decide → %v, want the refusal and not ErrEffectUnconfirmed", err)
	}
	if got := runStatus(t, e, handler); got != "failed" {
		t.Errorf("run is %q after a refused write, want failed", got)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM approval WHERE id = $1 AND effect_failure LIKE '%may have happened%'`, id); n != 0 {
		t.Error("a refused write is reported to the decider as one that may have happened")
	}
}

// The adapter confirms the change and the decider's request is cancelled at
// that moment. The change landed, so the run must still finish.
func TestACancelledRequestStillFinishesTheRunOfAConfirmedRemoteReassignment(t *testing.T) {
	e := integration.Setup(t)
	ctx, cancel := context.WithCancel(decider(e))
	defer cancel()
	svc := releaseThrough(e, remoteRecords{SystemOfRecordProvider: NewProvider(e.Pool), writes: true, afterWrite: cancel})
	contact := e.SeedContact(t, "Cancelled Mid Release", nil)
	id, handler := parkedRun(t, e, svc, string(workflow.ActionAssignOwner), contact,
		`{"owner_id":"`+e.Rep1.String()+`"}`)

	if _, err := svc.Decide(ctx, id, true, nil); err != nil {
		t.Errorf("Decide → %v, want success for a change that landed", err)
	}
	if got := runStatus(t, e, handler); got != "applied" {
		t.Errorf("run is %q after the change landed, want applied", got)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM approval WHERE id = $1 AND effect_failed_at IS NOT NULL`, id); n != 0 {
		t.Error("a release whose change landed is marked as failed")
	}
}

// The adapter confirms the change and the run cannot be marked. The decider is
// told the change was made, never that the work did not run.
func TestAConfirmedRemoteReassignmentWhoseRunCannotFinishSaysTheChangeLanded(t *testing.T) {
	e := integration.Setup(t)
	unreachable, err := testdb.OwnPoolFromConfig(context.Background(), e.Pool.Config())
	if err != nil {
		t.Fatal(err)
	}
	unreachable.Close()
	svc := releaseThroughDB(e, remoteRecords{SystemOfRecordProvider: NewProvider(e.Pool), writes: true},
		InstallationDB(unreachable))
	contact := e.SeedContact(t, "Landed Unrecorded", nil)
	id, _ := parkedRun(t, e, svc, string(workflow.ActionAssignOwner), contact,
		`{"owner_id":"`+e.Rep1.String()+`"}`)

	_, err = svc.Decide(decider(e), id, true, nil)
	if !errors.Is(err, approvals.ErrEffectFollowUpFailed) {
		t.Fatalf("Decide → %v, want ErrEffectFollowUpFailed", err)
	}
	if n := ownerAudits(t, e, "contact", contact, e.Rep1); n != 1 {
		t.Errorf("found %d owner changes, want the one that landed", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM approval WHERE id = $1
		AND effect_failure LIKE '%change was made%' AND effect_failure NOT LIKE '%did not run%'`, id); n != 1 {
		t.Error("the decider is not told the change was made")
	}
}
