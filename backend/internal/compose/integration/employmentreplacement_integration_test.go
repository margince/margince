// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

func TestANewerDatedRoleRetiresTheUndatedRoleAndUndoRestoresIt(t *testing.T) {
	for _, edit := range []string{"unchanged", "signature", "human", "human then signature"} {
		t.Run(edit, func(t *testing.T) {
			e := Setup(t)
			store := contacts.NewStore(e.DB())
			contact := ids.From[ids.ContactKind](e.SeedContact(t, "Lena Roles", nil))
			company, err := store.CreateCompany(e.Admin(), contacts.CreateCompanyInput{DisplayName: "Role Company", Source: "manual", Domains: []contacts.CompanyDomainInput{{Domain: "roles.example", IsPrimary: true}}})
			if err != nil {
				t.Fatal(err)
			}
			companyID := ids.From[ids.CompanyKind](ids.UUID(company.Id))
			old, err := store.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{Kind: "employment", ContactID: &contact, CompanyID: &companyID, Role: new("Role A"), Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			employmentPurchase(t, e, contact.UUID, []provider.Claim{{Key: provider.ClaimCurrentEmployment, Value: []byte(`{"company_name":"Role Company","company_domain":"roles.example","job_title":"Role B","started_at":"2026-01"}`)}})
			report, err := store.ApplyEmploymentImport(e.Admin(), contact, crmcontracts.EmploymentImportRequest{})
			if err != nil {
				t.Fatal(err)
			}
			if len(report.Items) != 1 || report.Items[0].State != "linked" || report.Items[0].RelationshipId == nil {
				t.Fatalf("new role=%+v", report.Items)
			}
			if n := e.WsCount(t, `SELECT count(*) FROM relationship WHERE contact_id=@contact AND company_id=@company AND kind='employment' AND `+employment.LiveCurrentPrimarySQL("relationship")+``, pgx.NamedArgs{"contact": contact, "company": companyID}); n != 1 {
				t.Fatalf("current roles=%d", n)
			}
			replacement := ids.UUID(*report.Items[0].RelationshipId)
			human := edit == "human" || edit == "human then signature"
			if human {
				if _, err := store.UpdateRelationship(e.Admin(), replacement, contacts.UpdateRelationshipInput{Role: new("Founder")}); err != nil {
					t.Fatal(err)
				}
			}
			if edit == "signature" || edit == "human then signature" {
				mail, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "email", Direction: new("inbound"), Subject: new("Updated title"), Body: new("Lena | VP Finance"), Source: "manual", Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact.UUID}}})
				if err != nil {
					t.Fatal(err)
				}
				applied, err := store.ApplySignatureFields(principal.SystemActing(e.Admin(), "system:signature"), contact, ids.UUID(mail.Id), []contacts.SignatureField{{Name: "title", Value: "VP Finance", Evidence: "Lena | VP Finance", Confidence: 0.95}}, true)
				if err != nil || applied.Applied != 1 || applied.SourceLimited {
					t.Fatalf("signature did not land: %+v %v", applied, err)
				}
				want := "VP Finance"
				if human {
					want = "Founder"
				}
				if role := e.WsScalar(t, `SELECT role FROM relationship WHERE id=@id`, pgx.NamedArgs{"id": replacement}); role != want {
					t.Fatalf("signature role=%q", role)
				}
			}
			var audit ids.UUID
			var version int64
			err = database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
				if err := tx.QueryRow(e.Admin(), `SELECT id FROM audit_log WHERE entity_type='relationship' AND entity_id=@id AND action='update' ORDER BY occurred_at DESC,id DESC LIMIT 1`, pgx.NamedArgs{"id": old.ID}).Scan(&audit); err != nil {
					return err
				}
				return tx.QueryRow(e.Admin(), `SELECT version FROM contact WHERE id=@id`, pgx.NamedArgs{"id": contact}).Scan(&version)
			})
			if err != nil {
				t.Fatal(err)
			}
			seam := compose.NewRestoreSeam(e.Pool, compose.NewProvider(e.Pool), nil)
			_, err = seam.Restore(e.Admin(), "contact", contact.UUID, audit, version)
			if human {
				if err == nil {
					t.Fatal("undo discarded a human edit to the replacement")
				}
				if n := e.WsCount(t, `SELECT count(*) FROM relationship WHERE id=@id AND archived_at IS NULL AND role='Founder'`, pgx.NamedArgs{"id": replacement}); n != 1 {
					t.Fatal("refused undo changed the human role")
				}
				if n := e.WsCount(t, `SELECT count(*) FROM relationship WHERE id=@id AND employment_status='former'`, pgx.NamedArgs{"id": old.ID}); n != 1 {
					t.Fatal("refused undo reopened the older role")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if n := e.WsCount(t, `SELECT count(*) FROM relationship WHERE id=@id AND archived_at IS NULL AND ended_at IS NULL AND coalesce(employment_status,'current')='current'`, pgx.NamedArgs{"id": old.ID}); n != 1 {
				t.Fatal("undo did not restore the old current role")
			}
			replay, err := store.ApplyEmploymentImport(e.Admin(), contact, crmcontracts.EmploymentImportRequest{})
			if err != nil {
				t.Fatal(err)
			}
			if replay.Items[0].State != "dismissed" {
				t.Fatalf("undo was reapplied: %+v", replay.Items[0])
			}
		})
	}
}
