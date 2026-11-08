// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Releasing an at-scale reassignment, on both kinds of system of record. The
// built-in provider writes on the redemption's transaction. An adapter over
// another system cannot, so its failures are recorded and never re-sent.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// remoteRecords stands in for an adapter over another system: it lacks
// recordUpdaterTx, and its Update answers with the error it is given. When
// writes is true the change lands first, as when only the answer is lost.
type remoteRecords struct {
	datasource.SystemOfRecordProvider
	writes bool
	answer error
}

func (r remoteRecords) Update(ctx context.Context, in datasource.UpdateInput) (datasource.EntityRef, error) {
	if r.writes {
		if _, err := r.SystemOfRecordProvider.Update(ctx, in); err != nil {
			return datasource.EntityRef{}, err
		}
	}
	return datasource.EntityRef{}, r.answer
}

// releaseThrough wires the reassignment effect over the given provider, the
// way coldstartaccept.go wires it over the built-in one.
func releaseThrough(e *integration.Env, provider datasource.SystemOfRecordProvider) *approvals.Service {
	svc := NewApprovalsServiceWithEffects(e.Pool)
	return svc.WithEffect(string(workflow.ActionAssignOwner),
		assignOwnerReleaseEffect(svc, provider, InstallationDB(e.Pool)))
}

func unconsumed(t *testing.T, e *integration.Env, id ids.ApprovalID) bool {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM approval WHERE id = $1 AND consumed_at IS NULL`, id) == 1
}

func ownerAudits(t *testing.T, e *integration.Env, contact, owner ids.UUID) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM audit_log
		WHERE entity_type = 'contact' AND entity_id = $1 AND after ->> 'owner_id' = $2`, contact, owner.String())
}

// A released reassignment on the built-in provider commits its redemption, the
// owner, the owner's audit row and the run in one transaction.
func TestALocalReassignmentCommitsTheRedemptionTheOwnerAndItsAuditTogether(t *testing.T) {
	e := integration.Setup(t)
	svc := NewApprovalsServiceWithEffects(e.Pool)
	contact := e.SeedContact(t, "Released Together", nil)
	id, handler := parkedRun(t, e, svc, string(workflow.ActionAssignOwner), contact,
		`{"owner_id":"`+e.Rep1.String()+`"}`)

	if _, err := svc.Decide(decider(e), id, true, nil); err != nil {
		t.Fatalf("Decide(approve) → %v", err)
	}
	if unconsumed(t, e, id) {
		t.Error("the reassignment landed and the approval is still unconsumed, so deciding again would apply it twice")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM contact WHERE id = $1 AND owner_id = $2`, contact, e.Rep1); n != 1 {
		t.Error("the contact kept its old owner")
	}
	if n := ownerAudits(t, e, contact, e.Rep1); n != 1 {
		t.Errorf("found %d audit rows for the owner change, want 1", n)
	}
	if got := runStatus(t, e, handler); got != "applied" {
		t.Errorf("run is %q, want applied", got)
	}
}

// A write that fails on the built-in provider rolls the redemption back with
// it, so the approval stays unconsumed and deciding again runs the write.
//
// The failure comes from the write, after the redemption passed: the new owner
// holds a read seat, which the owner hand-on check refuses.
func TestAFailedLocalReassignmentLeavesTheApprovalUnconsumedAndRetryable(t *testing.T) {
	e := integration.Setup(t)
	svc := NewApprovalsServiceWithEffects(e.Pool)
	contact := e.SeedContact(t, "Fails Then Lands", nil)
	id, handler := parkedRun(t, e, svc, string(workflow.ActionAssignOwner), contact,
		`{"owner_id":"`+e.Rep2.String()+`"}`)
	e.WsExec(t, `UPDATE app_user SET seat_type = 'read' WHERE id = $1`, e.Rep2)

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
	if n := ownerAudits(t, e, contact, e.Rep2); n != 0 {
		t.Errorf("found %d owner changes after a refused write, want none", n)
	}

	e.WsExec(t, `UPDATE app_user SET seat_type = 'full' WHERE id = $1`, e.Rep2)
	if _, err := svc.Decide(decider(e), id, true, nil); err != nil {
		t.Fatalf("deciding again did not re-drive the reassignment: %v", err)
	}
	if n := ownerAudits(t, e, contact, e.Rep2); n != 1 {
		t.Errorf("found %d audit rows for the owner change after the retry, want 1", n)
	}
	if got := runStatus(t, e, handler); got != "applied" {
		t.Errorf("run is %q after the retry landed, want applied", got)
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
	if n := ownerAudits(t, e, contact, e.Rep1); n != 1 {
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
