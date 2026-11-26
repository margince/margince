// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package recordsources

import (
	"errors"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// olderParty plants a contact or company as written before the source check.
// The writer's word is in source, and source_system is as given.
func olderParty(t *testing.T, owner *pgx.Conn, table, source string, system *string) ids.UUID {
	t.Helper()
	if table == "contact" {
		return integration.SeedBeforeSourceCheck(t, owner, table,
			`INSERT INTO contact (id, full_name, source, source_system, captured_by)
			 VALUES ($1, 'Older Contact', $2, $3, 'connector:gmail')`, source, system)
	}
	return integration.SeedBeforeSourceCheck(t, owner, table,
		`INSERT INTO company (id, display_name, source, source_system, captured_by)
		 VALUES ($1, 'Older Company', $2, $3, 'connector:gmail')`, source, system)
}

// partyOrigin reads a row's channel and its system, empty when it has none.
func partyOrigin(t *testing.T, e *integration.Env, table string, id ids.UUID) [2]string {
	t.Helper()
	return [2]string{
		e.WsScalar(t, `SELECT source FROM `+pgx.Identifier{table}.Sanitize()+` WHERE id = $1`, id),
		e.WsScalar(t, `SELECT COALESCE(source_system, '') FROM `+pgx.Identifier{table}.Sanitize()+` WHERE id = $1`, id),
	}
}

// Each older word lands on its channel, and the connector or writer it named
// moves to source_system. A system the writer already named is kept.
func TestTheTaskMovesOlderContactsAndCompaniesOntoAChannel(t *testing.T) {
	e := integration.Setup(t)
	owner := integration.OwnerConn(t)
	csv, crm := "mirror:csv:bundle", "ext:billing:crm"
	type planted struct {
		id            ids.UUID
		table, source string
		origin        [2]string
	}
	var rows []planted
	corrected := map[string]int{}
	plant := func(table, source string, system *string, origin [2]string) {
		rows = append(rows, planted{olderParty(t, owner, table, source, system), table, source, origin})
		corrected[table]++
	}
	plant("contact", "gmail:msg-1", nil, [2]string{"capture", "gmail"})
	plant("contact", "telegram:bot:chat:9", nil, [2]string{"capture", "telegram"})
	plant("contact", "capture_counterparty_verdict", nil, [2]string{"capture", "capture_counterparty_verdict"})
	plant("contact", "vcard_import", nil, [2]string{"import", "vcard_import"})
	plant("contact", "public_booking", nil, [2]string{"webform", "public_booking"})
	plant("contact", "mirror:csv:bundle:contact:7", &csv, [2]string{"import", csv})
	plant("contact", "website footer", nil, [2]string{"api", ""})
	crmImport := "crm_import"
	plant("contact", "gmail:msg-9", &crmImport, [2]string{"import", crmImport})
	plant("company", "domain_triage:acme.test", nil, [2]string{"capture", "domain_triage"})
	plant("company", "coldstart", nil, [2]string{"automation", "coldstart"})
	plant("company", "mirror:csv:bundle:contact:8", nil, [2]string{"import", csv})
	plant("company", crm, &crm, [2]string{"import", crm})
	plant("company", "ext:chat:dispact", nil, [2]string{"capture", "ext:chat:dispact"})

	ctx := systemCtx(e)
	task := PartyDataTask()
	if n, capped, err := task.Steps[0].Estimate(ctx, e.Pool, 3); err != nil || n != 3 || !capped {
		t.Fatalf("the contact estimate under a cap of 3 = %d, capped %v (%v), want 3 and capped", n, capped, err)
	}
	e.RunDataTask(ctx, t, task)

	for _, row := range rows {
		if got := partyOrigin(t, e, row.table, row.id); got != row.origin {
			t.Errorf("%s with source %q reads %v, want %v", row.table, row.source, got, row.origin)
		}
	}
	for _, table := range []string{"contact", "company"} {
		if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = $1 AND action = 'update'
			AND before ? 'source' AND evidence ? 'data_task_run'`, table); n != corrected[table] {
			t.Errorf("%d %s audit row(s) keep an older source, want %d", n, table, corrected[table])
		}
		if n := e.WsCount(t, `SELECT count(*) FROM event_outbox WHERE envelope->>'type' = $1`, table+".updated"); n != corrected[table] {
			t.Errorf("%d %s.updated event(s), want one per corrected row", n, table)
		}
	}
	for _, s := range task.Steps {
		if n, _, err := s.Estimate(ctx, e.Pool, 10); err != nil || n != 0 {
			t.Errorf("step %s still estimates %d row(s) (%v), want none", s.Name, n, err)
		}
	}
}

// A writer still passing a mailbox stamp gets a captured contact naming its
// connector, and the address the stamp was copied to keeps the stamp.
func TestACaptureStampIsStoredAsTheChannelAndTheConnector(t *testing.T) {
	e := integration.Setup(t)
	made, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
		FullName: "Ines Huber", Source: "gmail:msg-42",
		Emails: []contacts.ContactEmailInput{{Email: "ines@huber.test", EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("creating a contact with a capture stamp: %v", err)
	}
	if got, want := partyOrigin(t, e, "contact", ids.UUID(made.Id)), [2]string{"capture", "gmail"}; got != want {
		t.Errorf("the contact reads %v, want %v", got, want)
	}
	if made.Source != "capture" || made.SourceSystem == nil || *made.SourceSystem != "gmail" {
		t.Errorf("the create answers source %q and source_system %v, want capture and gmail", made.Source, made.SourceSystem)
	}
	if got := e.WsScalar(t, `SELECT source FROM contact_email WHERE contact_id = $1`, made.Id); got != "gmail:msg-42" {
		t.Errorf("the address reads source %q, want the stamp it was written with", got)
	}
}

// An older row is judged by the unvalidated check on its next write. An edit
// before the task goes through and moves the row onto its channel.
func TestAnEditBeforeTheTaskMovesAnOlderContactOntoItsChannel(t *testing.T) {
	e := integration.Setup(t)
	older := olderParty(t, integration.OwnerConn(t), "contact", "imap:msg-3", nil)
	name := "Renamed Contact"
	if _, err := e.Contacts.UpdateContact(e.Admin(), ids.From[ids.ContactKind](older), contacts.UpdateContactInput{FullName: &name}); err != nil {
		t.Fatalf("editing an older contact: %v", err)
	}
	if got, want := partyOrigin(t, e, "contact", older), [2]string{"capture", "imap"}; got != want {
		t.Errorf("the edited contact reads %v, want %v", got, want)
	}
}

// The mailbox purge selects what capture made. It finds a contact the task has
// not reached, and the task's own audit row does not count as somebody's work.
func TestTheMailboxPurgeFindsCapturedContactsBeforeAndAfterTheTask(t *testing.T) {
	e := integration.Setup(t)
	owner := integration.OwnerConn(t)
	captured := integration.SeedBeforeSourceCheck(t, owner, "contact",
		`INSERT INTO contact (id, full_name, source, captured_by, owner_id)
		 VALUES ($1, 'Captured', 'gmail:msg-5', 'connector:gmail', $2)`, e.Rep1)
	carded := integration.SeedBeforeSourceCheck(t, owner, "contact",
		`INSERT INTO contact (id, full_name, source, captured_by, owner_id)
		 VALUES ($1, 'Carded', 'vcard_import', 'connector:gmail', $2)`, e.Rep1)
	ctx := systemCtx(e)
	purgeable := func() []ids.UUID {
		t.Helper()
		var found []ids.UUID
		if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) (err error) {
			found, err = contacts.CaptureMintedForSeatTx(ctx, tx, e.Rep1, []ids.UUID{captured, carded})
			return err
		}); err != nil {
			t.Fatalf("asking the purge: %v", err)
		}
		return found
	}
	if got := purgeable(); !slices.Equal(got, []ids.UUID{captured}) {
		t.Errorf("before the task the purge selects %v, want only the captured contact %s", got, captured)
	}
	e.RunDataTask(ctx, t, PartyDataTask())
	if got := purgeable(); !slices.Equal(got, []ids.UUID{captured}) {
		t.Errorf("after the task the purge selects %v, want only the captured contact %s", got, captured)
	}
}

// A feed may adopt a company capture made, whose source_system names only the
// connector. One an import or an integration named stays refused.
func TestAFeedAdoptsACapturedCompanyAndNotAnImportedOne(t *testing.T) {
	e := integration.Setup(t)
	captured, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{DisplayName: "Captured Co", Source: "gmail:msg-6"})
	if err != nil {
		t.Fatalf("creating a captured company: %v", err)
	}
	csv := "mirror:csv:bundle"
	imported, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{DisplayName: "Imported Co", Source: "import", SourceSystem: &csv})
	if err != nil {
		t.Fatalf("creating an imported company: %v", err)
	}
	hubspot := "hubspot"
	named, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{DisplayName: "Named Co", Source: "api", SourceSystem: &hubspot})
	if err != nil {
		t.Fatalf("creating a company an integration named: %v", err)
	}
	adopt := func(id ids.UUID, key string) error {
		_, err := e.Contacts.UpdateCompany(e.Admin(), ids.From[ids.CompanyKind](id), contacts.UpdateCompanyInput{
			AdoptSource: &contacts.OutsideID{System: "erp", ID: key},
		})
		return err
	}
	if err := adopt(ids.UUID(captured.Id), "c-1"); err != nil {
		t.Errorf("adopting a captured company: %v, want it adopted", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'company' AND entity_id = $1
		AND before->>'source_system' = 'gmail' AND after->>'source_system' = 'erp'`, captured.Id); n != 1 {
		t.Errorf("%d audit row(s) keep the connector the adoption replaced, want 1", n)
	}
	if err := adopt(ids.UUID(imported.Id), "c-2"); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("adopting an imported company: %v, want a conflict", err)
	}
	if err := adopt(ids.UUID(named.Id), "c-3"); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("adopting a company an integration named: %v, want a conflict", err)
	}
}
