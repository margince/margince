// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

func importedUnknownEmployment(t *testing.T, e *integration.Env, resolve bool) ids.UUID {
	t.Helper()
	subject := e.SeedContact(t, "Imported colleague", nil)
	contact := ids.From[ids.ContactKind](subject)
	company, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{DisplayName: "Imported employer", Source: "manual", Domains: []contacts.CompanyDomainInput{{Domain: "imported-employer.example", IsPrimary: true}}})
	if err != nil {
		t.Fatal(err)
	}
	err = database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		var run string
		err := tx.QueryRow(e.Admin(), `INSERT INTO provider_run (subject_kind,contact_id,provider,trigger,state,input_fingerprint,external_correlation_id,connection_version,connection_epoch,configuration_snapshot,requested_categories,completed_at)
 VALUES ('contact',@contact,'surfe','manual','completed',@fingerprint,gen_random_uuid(),1,1,'{}'::jsonb,ARRAY['professional_email'],now()) RETURNING id::text`, pgx.NamedArgs{"contact": subject, "fingerprint": ids.NewV7().String()}).Scan(&run)
		if err != nil {
			return err
		}
		return contacts.WriteProviderClaims(e.Admin(), tx, run, subject.String(), "surfe", []provider.Claim{{Key: provider.ClaimJobHistory, Value: []byte(`[{"company_name":"imported-employer.example","job_title":"Advisor"}]`)}}, time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC))
	})
	if err != nil {
		t.Fatal(err)
	}
	request := crmcontracts.EmploymentImportRequest{}
	if resolve {
		preview, err := e.Contacts.PreviewEmploymentImport(e.Admin(), contact)
		if err != nil || len(preview.Items) != 1 {
			t.Fatalf("preview: %+v %v", preview, err)
		}
		request.Action = new(crmcontracts.EmploymentImportRequestActionResolve)
		request.Key = &preview.Items[0].Key
		request.CompanyId = &company.Id
	}
	report, err := e.Contacts.ApplyEmploymentImport(e.Admin(), contact, request)
	if err != nil || len(report.Items) != 1 || report.Items[0].RelationshipId == nil {
		t.Fatalf("import: %+v %v", report, err)
	}
	return ids.UUID(*report.Items[0].RelationshipId)
}

func TestUnknownEmploymentRepairPreservesImportsAfterHumanEditsAndResolution(t *testing.T) {
	for _, resolve := range []bool{false, true} {
		t.Run(map[bool]string{false: "edited_import", true: "resolved_import"}[resolve], func(t *testing.T) {
			e := integration.Setup(t)
			id := importedUnknownEmployment(t, e, resolve)
			if !resolve {
				if _, err := e.Contacts.UpdateRelationship(e.Admin(), id, contacts.UpdateRelationshipInput{Role: new("Board advisor")}); err != nil {
					t.Fatal(err)
				}
			}
			before := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id=$1`, id)
			task := unknownEmploymentDataTask(e.Pool)
			runMachineAddressTask(datatask.Acting(e.Admin(), task.ID), t, e, task)
			row, err := e.Contacts.GetRelationship(e.Admin(), id)
			if err != nil {
				t.Fatal(err)
			}
			if row.EmploymentStatus == nil || *row.EmploymentStatus != "unknown" {
				t.Fatalf("import changed: %+v", row)
			}
			if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id=$1`, id); got != before {
				t.Fatal("protected import audited as a repair")
			}
		})
	}
}

func TestUnknownEmploymentRepairPreservesResolvedImportsAfterClaimsAreDeleted(t *testing.T) {
	e := integration.Setup(t)
	id := importedUnknownEmployment(t, e, true)
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := contacts.DeleteProviderClaims(e.Admin(), tx, "surfe"); err != nil {
			return err
		}
		_, err := tx.Exec(e.Admin(), `DELETE FROM provider_applied_field WHERE target_row_id=@id`, pgx.NamedArgs{"id": id})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	task := unknownEmploymentDataTask(e.Pool)
	runMachineAddressTask(datatask.Acting(e.Admin(), task.ID), t, e, task)
	row, err := e.Contacts.GetRelationship(e.Admin(), id)
	if err != nil {
		t.Fatal(err)
	}
	if row.EmploymentStatus == nil || *row.EmploymentStatus != "unknown" {
		t.Fatal("retired provider ledgers turned imported uncertainty into an assertion")
	}
}
