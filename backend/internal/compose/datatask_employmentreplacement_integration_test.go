// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/employmentrepair"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

func unresolvedNewerEmployment(t *testing.T, e *integration.Env, name string) ids.ContactID {
	t.Helper()
	contact := mergeTestContact(t, e, name)
	company, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{DisplayName: name, Source: "manual", Domains: []contacts.CompanyDomainInput{{Domain: name + ".example", IsPrimary: true}}})
	if err != nil {
		t.Fatal(err)
	}
	companyID := ids.From[ids.CompanyKind](ids.UUID(company.Id))
	if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{Kind: "employment", ContactID: &contact, CompanyID: &companyID, Role: new("Older role"), Source: "manual"}); err != nil {
		t.Fatal(err)
	}
	if err := database.WithTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		var run string
		if err := tx.QueryRow(e.Admin(), `INSERT INTO provider_run(subject_kind,contact_id,provider,trigger,state,input_fingerprint,external_correlation_id,connection_version,connection_epoch,configuration_snapshot,requested_categories,completed_at) VALUES('contact',@contact,'surfe','manual','completed','repair-fixture',gen_random_uuid(),1,1,'{}',ARRAY['job_history'],now()) RETURNING id::text`, pgx.NamedArgs{"contact": contact}).Scan(&run); err != nil {
			return err
		}
		return contacts.WriteProviderClaims(e.Admin(), tx, run, contact.String(), "surfe", []provider.Claim{{Key: provider.ClaimCurrentEmployment, Value: []byte(`{"company_name":"` + name + `","company_domain":"` + name + `.example","job_title":"Newer role","started_at":"2026-01"}`)}}, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
	}); err != nil {
		t.Fatal(err)
	}
	report, err := e.Contacts.PreviewEmploymentImport(e.Admin(), contact)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Items) != 1 {
		t.Fatalf("episodes=%+v", report.Items)
	}
	// The old importer saved the unresolved decision without changing either role.
	e.WsExec(t, `INSERT INTO provider_employment_resolution(contact_id,claim_id,episode_key,state,company_id) SELECT @contact,id,@key,'needs_review',@company FROM contact_provider_claim WHERE contact_id=@contact AND claim_key='current_employment'`, pgx.NamedArgs{"contact": contact, "key": report.Items[0].Key, "company": companyID})
	return contact
}

func TestUndatedEmploymentRepairIsBoundedAtomicAndReplaySafe(t *testing.T) {
	e := integration.Setup(t)
	first := unresolvedNewerEmployment(t, e, "first-role")
	second := unresolvedNewerEmployment(t, e, "second-role")
	task := employmentrepair.DataTask(e.Contacts)
	ctx := datatask.Acting(e.Admin(), task.ID)
	rollback := errors.New("roll back the completed batch")
	before := e.WsCount(t, `SELECT count(*) FROM audit_log`)
	err := database.WithTx(ctx, e.Pool, func(tx pgx.Tx) error {
		out, err := task.Steps[0].Batch(ctx, tx, datatask.BatchIn{Limit: 1, RunID: ids.NewV7()})
		if err != nil {
			return err
		}
		if out.Changed != 1 {
			t.Fatalf("batch changed %d contacts", out.Changed)
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM provider_employment_resolution WHERE state='needs_review'`); n != 2 {
		t.Fatalf("rollback left %d unresolved", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log`); n != before {
		t.Fatal("audit escaped rollback")
	}
	task.Steps[0].Limit = 1
	runMachineAddressTask(ctx, t, e, task)
	for _, contact := range []ids.ContactID{first, second} {
		if n := e.WsCount(t, `SELECT count(*) FROM relationship WHERE contact_id=@contact AND archived_at IS NULL AND ended_at IS NULL AND employment_status='current'`, pgx.NamedArgs{"contact": contact}); n != 1 {
			t.Fatalf("current roles=%d", n)
		}
	}
	before = e.WsCount(t, `SELECT count(*) FROM audit_log`)
	runMachineAddressTask(ctx, t, e, task)
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log`); n != before {
		t.Fatal("replay wrote new audit rows")
	}
	err = database.WithTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, err := task.Steps[0].Batch(ctx, tx, datatask.BatchIn{Limit: 1, Cursor: []byte(`"invalid"`)})
		return err
	})
	if err == nil {
		t.Fatal("repair accepted a malformed cursor")
	}
}
