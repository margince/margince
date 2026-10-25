// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// capturedEmployee is a contact capture made at company's domain, with the
// employment edge the ensure planted, before the address was a seat's.
func capturedEmployee(t *testing.T, e *integration.Env, address string) (ids.ContactID, ids.UUID) {
	t.Helper()
	contact := captureCounterparty(t, e, "Somebody", address)
	edge := e.WsScalar(t, `SELECT id::text FROM relationship WHERE contact_id = $1 AND kind = 'employment' AND archived_at IS NULL`, contact)
	id, err := ids.Parse(edge)
	if err != nil {
		t.Fatalf("the ensure planted no employment for %s: %v", address, err)
	}
	return contact, id
}

func edgeLive(t *testing.T, e *integration.Env, edge ids.UUID) bool {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM relationship WHERE id = $1 AND archived_at IS NULL`, edge) == 1
}

func TestTheSeatEmploymentTaskRetiresOnlyInferredEdgesOfSeatsAndReplaysSafely(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	if _, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{
		DisplayName: "Outside GmbH", Source: "manual", Domains: []contacts.CompanyDomainInput{{Domain: "outside.test", IsPrimary: true}},
	}); err != nil {
		t.Fatalf("creating the company: %v", err)
	}
	other, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{DisplayName: "Elsewhere AG", Source: "manual"})
	if err != nil {
		t.Fatalf("creating the second company: %v", err)
	}
	seat, inferred := capturedEmployee(t, e, "colleague@outside.test")
	_, edited := capturedEmployee(t, e, "edited@outside.test")
	_, client := capturedEmployee(t, e, "client@outside.test")
	asserted, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
		Kind: "employment", ContactID: &seat, CompanyID: new(ids.From[ids.CompanyKind](ids.UUID(other.Id))), Source: "manual",
	})
	if err != nil {
		t.Fatalf("asserting an employment by hand: %v", err)
	}
	if _, err := e.Contacts.UpdateRelationship(e.Admin(), edited, contacts.UpdateRelationshipInput{Role: new("Advisor")}); err != nil {
		t.Fatalf("editing the inferred edge by hand: %v", err)
	}
	connectSeatMailbox(t, e, e.Rep2, "colleague@outside.test", "shared")
	connectSeatMailbox(t, e, e.Rep3, "edited@outside.test", "shared")

	task := seatEmploymentDataTask()
	step := task.Steps[0]
	rollback := errors.New("roll the batch back")
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		if _, err := step.Batch(ctx, tx, datatask.BatchIn{Limit: 10, RunID: ids.NewV7()}); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("the batch before the rollback: %v", err)
	}
	if !edgeLive(t, e, inferred) {
		t.Fatal("a rolled-back batch still retired the edge")
	}

	task.Steps[0].Limit = 1
	runMachineAddressTask(ctx, t, e, task)

	if edgeLive(t, e, inferred) {
		t.Error("the employment capture inferred for a colleague's address is still live")
	}
	for name, edge := range map[string]ids.UUID{"the hand-made edge": asserted.ID, "the hand-edited edge": edited, "a client's edge": client} {
		if !edgeLive(t, e, edge) {
			t.Errorf("%s was retired", name)
		}
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'relationship' AND action = 'archive' AND evidence ? 'data_task_run'`); got != 1 {
		t.Errorf("archive audit rows naming the run = %d, want 1", got)
	}

	in := datatask.BatchIn{Limit: 2, RunID: ids.NewV7()}
	for {
		var out datatask.BatchOut
		if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			var err error
			out, err = step.Batch(ctx, tx, in)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if out.Changed != 0 {
			t.Fatalf("a replay retired %d edges again", out.Changed)
		}
		if out.Done {
			break
		}
		in.Cursor = out.Cursor
	}
}
