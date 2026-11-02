// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func legacyEmployerContact(t *testing.T, e *integration.Env) ids.UUID {
	t.Helper()
	lead, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{FullName: new("Dana Research"), CompanyName: new("Northwind Research"), Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	promoted, err := e.Contacts.QualifyLead(e.Admin(), ids.From[ids.LeadKind](ids.UUID(lead.Id)), contacts.PromoteLeadInput{Trigger: "human_qualify", CreateCompany: true})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.UUID(promoted.Contact.Id)
	if promoted.Contact.Employer == nil {
		t.Fatal("qualification lost employer")
	}
	e.WsExec(t, `DELETE FROM relationship WHERE contact_id = $1`, id)
	e.WsExec(t, `DELETE FROM company WHERE id = $1`, promoted.Contact.Employer.CompanyId)
	return id
}

func TestContactEmployerTaskRepairsArchivedLeadEvidenceAndReplays(t *testing.T) {
	e := integration.Setup(t)
	id := legacyEmployerContact(t, e)
	task := contactEmployersDataTask(e.Pool)
	task.Steps[0].Limit = 1
	ctx := datatask.Acting(e.Admin(), task.ID)
	runMachineAddressTask(ctx, t, e, task)
	if got := e.WsCount(t, `SELECT count(*) FROM relationship WHERE contact_id = $1 AND kind = 'employment'`, id); got != 1 {
		t.Fatalf("employment count = %d", got)
	}
	companies := e.WsCount(t, `SELECT count(*) FROM company WHERE display_name = 'Northwind Research'`)
	if companies != 1 {
		t.Fatalf("company count = %d", companies)
	}
	audits := e.WsCount(t, `SELECT count(*) FROM audit_log`)
	err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		out, err := task.Steps[0].Batch(ctx, tx, datatask.BatchIn{Limit: 100, RunID: ids.NewV7()})
		if err == nil && out.Changed != 0 {
			t.Errorf("replay changed %d records", out.Changed)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if again := e.WsCount(t, `SELECT count(*) FROM audit_log`); again != audits {
		t.Fatal("replay wrote more audit rows")
	}
}

func TestContactEmployerTaskRollsBackWithItsProgress(t *testing.T) {
	e := integration.Setup(t)
	id := legacyEmployerContact(t, e)
	task := contactEmployersDataTask(e.Pool)
	original := task.Steps[0].Batch
	task.Steps[0].Batch = func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
		out, err := original(ctx, tx, in)
		if err != nil {
			return out, err
		}
		return out, errors.New("interrupted after employer writes")
	}
	runMachineAddressTask(datatask.Acting(e.Admin(), task.ID), t, e, task)
	if got := e.WsCount(t, `SELECT count(*) FROM relationship WHERE contact_id = $1 AND kind = 'employment'`, id); got != 0 {
		t.Fatal("employment escaped rollback")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM company WHERE display_name = 'Northwind Research'`); got != 0 {
		t.Fatal("company escaped rollback")
	}
}

func TestContactEmployerTaskBoundsPagingAndRejectsMalformedCursor(t *testing.T) {
	e := integration.Setup(t)
	for _, name := range []string{"No Evidence One", "No Evidence Two"} {
		if _, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: name, Source: "manual"}); err != nil {
			t.Fatal(err)
		}
	}
	task := contactEmployersDataTask(e.Pool)
	ctx := datatask.Acting(e.Admin(), task.ID)
	count, more, err := task.Steps[0].Estimate(ctx, e.Pool, 1)
	if err != nil || count != 1 || !more {
		t.Fatalf("estimate %d, %v, %v", count, more, err)
	}
	err = database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		first, err := task.Steps[0].Batch(ctx, tx, datatask.BatchIn{Limit: 1, RunID: ids.NewV7()})
		if err != nil {
			return err
		}
		if first.Done || len(first.Cursor) == 0 || first.Changed != 0 {
			t.Fatalf("first page: %+v", first)
		}
		_, err = task.Steps[0].Batch(ctx, tx, datatask.BatchIn{Cursor: []byte(`"bad cursor"`), Limit: 1})
		return err
	})
	if err == nil {
		t.Fatal("malformed cursor accepted")
	}
}

func TestContactEmployerTaskSkipsConflictingEvidenceAndKeepsScanning(t *testing.T) {
	e := integration.Setup(t)
	id := legacyEmployerContact(t, e)
	for range 2 {
		if _, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{DisplayName: "Northwind Research", Source: "manual"}); err != nil {
			t.Fatal(err)
		}
	}
	task := contactEmployersDataTask(e.Pool)
	ctx := datatask.Acting(e.Admin(), task.ID)
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		out, err := task.Steps[0].Batch(ctx, tx, datatask.BatchIn{Limit: 100, RunID: ids.NewV7()})
		if err == nil && (out.Changed != 0 || !out.Done || len(out.Cursor) == 0) {
			t.Errorf("refused page: %+v", out)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM relationship WHERE contact_id=$1`, id); n != 0 {
		t.Fatal("ambiguous employer attached")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM system_log WHERE action='contact_employer_repair.skipped'`); n != 1 {
		t.Fatal("refusal was not recorded")
	}
}

func TestContactEmployerTaskPreservesArchivedEmployment(t *testing.T) {
	e := integration.Setup(t)
	contact, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: "Former Employee", Source: "manual", Employer: contacts.EmployerChoice{CompanyName: new("Former Works")}})
	if err != nil {
		t.Fatal(err)
	}
	var edge ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(e.Admin(), `SELECT id FROM relationship WHERE contact_id=$1`, contact.Id).Scan(&edge)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Contacts.ArchiveRelationship(e.Admin(), edge, nil); err != nil {
		t.Fatal(err)
	}
	task := contactEmployersDataTask(e.Pool)
	ctx := datatask.Acting(e.Admin(), task.ID)
	runMachineAddressTask(ctx, t, e, task)
	if n := e.WsCount(t, `SELECT count(*) FROM relationship WHERE contact_id=$1 AND archived_at IS NULL`, contact.Id); n != 0 {
		t.Fatal("repair restored removed employment")
	}
}
