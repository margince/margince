// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const hubspotBCC = "4711@bcc.eu1.hubspot.com"

func contactArchived(t *testing.T, e *integration.Env, contact ids.UUID) bool {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM contact WHERE id = $1 AND archived_at IS NOT NULL`, contact) == 1
}

// mixedContact holds a real address and, beside it, the technical one.
func mixedContact(t *testing.T, e *integration.Env, conn *pgx.Conn) ids.UUID {
	t.Helper()
	made, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
		FullName: "Mixed Contact", Source: "manual",
		Emails: []contacts.ContactEmailInput{
			{Email: "maria@technical-task.test", EmailType: "work", IsPrimary: true},
			{Email: "seed-" + ids.NewV7().String() + "@technical-task.test", EmailType: "other", Position: 1},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(context.Background(),
		`UPDATE contact_email SET email = $2 WHERE contact_id = $1 AND email LIKE 'seed-%'`, made.Id, "99@bcc.hubspot.com"); err != nil {
		t.Fatal(err)
	}
	return ids.UUID(made.Id)
}

func TestTheTechnicalAddressTaskArchivesRetiresAndTags(t *testing.T) {
	e := integration.Setup(t)
	conn := integration.OwnerConn(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	technical := machineAddressContact(t, e, conn, "HubSpot Logger", hubspotBCC)
	touched := machineAddressContact(t, e, conn, "Edited Logger", "12@bcc.hubspot.com")
	if _, err := e.Contacts.UpdateContact(e.Admin(), ids.From[ids.ContactKind](touched),
		contacts.UpdateContactInput{FullName: new("Renamed by a human")}); err != nil {
		t.Fatal(err)
	}
	held := machineAddressContact(t, e, conn, "Held Logger", "13@bcc.hubspot.com")
	if _, err := conn.Exec(ctx, `UPDATE contact SET legal_hold = true WHERE id = $1`, held); err != nil {
		t.Fatal(err)
	}
	mixed := mixedContact(t, e, conn)
	providerHuman := machineAddressContact(t, e, conn, "Anna at HubSpot", "anna@hubspot.com")
	bccHuman := machineAddressContact(t, e, conn, "Bcc Human", "bcc@acme-technical.test")

	task := technicalAddressDataTask(e.Pool)
	task.Steps[0].Limit = 1
	runMachineAddressTask(ctx, t, e, task)

	for name, id := range map[string]ids.UUID{"technical-only": technical, "human-edited technical-only": touched} {
		if !contactArchived(t, e, id) {
			t.Errorf("the %s contact is still live", name)
		}
		if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'contact' AND entity_id = $1
			AND action = 'archive' AND batch_id IS NOT NULL`, id); got != 1 {
			t.Errorf("the %s contact's archive left %d audit rows naming the run, want 1", name, got)
		}
	}
	if contactArchived(t, e, held) || machineTagged(t, e, held) != 1 {
		t.Error("a contact on legal hold must stay live and be tagged for review")
	}
	if contactArchived(t, e, mixed) {
		t.Error("a human with a real address was archived")
	}
	if got := e.WsScalar(t, `SELECT string_agg(email, ',' ORDER BY email) FROM contact_email
		WHERE contact_id = $1 AND archived_at IS NULL`, mixed); got != "maria@technical-task.test" {
		t.Errorf("the mixed contact holds %q, want only the real address", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'contact' AND entity_id = $1
		AND batch_id IS NOT NULL`, mixed); got != 1 {
		t.Errorf("retiring the mixed contact's address left %d audit rows naming the run, want 1", got)
	}
	for name, id := range map[string]ids.UUID{"anna@hubspot.com": providerHuman, "bcc@acme": bccHuman} {
		if contactArchived(t, e, id) || machineTagged(t, e, id) != 0 {
			t.Errorf("%s is a human and must be left alone", name)
		}
	}

	step := task.Steps[0]
	in := datatask.BatchIn{Limit: 2, RunID: ids.NewV7()}
	for {
		var out datatask.BatchOut
		err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			var err error
			out, err = step.Batch(ctx, tx, in)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		if out.Changed != 0 {
			t.Fatalf("a replay changed %d contacts again", out.Changed)
		}
		if out.Done {
			break
		}
		in.Cursor = out.Cursor
	}
}

func TestCaptureRefusesAnExistingTechnicalContact(t *testing.T) {
	e := integration.Setup(t)
	conn := integration.OwnerConn(t)
	technical := machineAddressContact(t, e, conn, "HubSpot Logger", hubspotBCC)
	_, err := e.Contacts.EnsureCounterparty(principal.SystemActing(e.Admin(), "connector:test"),
		contacts.EnsureCounterpartyInput{Email: hubspotBCC, Source: "gmail"})
	if err == nil {
		t.Fatal("a technical address was resolved as a counterparty")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM relationship WHERE contact_id = $1 AND archived_at IS NULL`, technical); got != 0 {
		t.Errorf("the technical contact gained %d employments", got)
	}
}
