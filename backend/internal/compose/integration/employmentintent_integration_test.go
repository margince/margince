// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

func TestAnImportedFormerRoleDoesNotBlockTheNextCurrentRole(t *testing.T) {
	e := Setup(t)
	seedEmployerOnDomain(t, e, "History Employer", "intent-history.test")
	subject := seedMergeSubject(t, e, "History Reader")
	contact := ids.From[ids.ContactKind](subject)
	employmentPurchase(t, e, subject, []provider.Claim{{Key: provider.ClaimJobHistory, Value: []byte(`[{"company_domain":"intent-history.test","company_name":"History Employer","job_title":"Engineer","ended_at":"2020-01"}]`)}})
	if _, err := e.Contacts.ApplyEmploymentImport(e.Admin(), contact, crmcontracts.EmploymentImportRequest{}); err != nil {
		t.Fatal(err)
	}
	employmentPurchase(t, e, subject, []provider.Claim{{Key: provider.ClaimCurrentEmployment, Value: []byte(`{"company_domain":"intent-history.test","company_name":"History Employer","job_title":"Director","started_at":"2024-01"}`)}})
	report, err := e.Contacts.ApplyEmploymentImport(e.Admin(), contact, crmcontracts.EmploymentImportRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range report.Items {
		if row.State != "linked" {
			t.Fatalf("imported history blocked a role: %+v", report.Items)
		}
	}
	before := e.WsCount(t, `SELECT count(*) FROM audit_log`)
	if _, err := e.Contacts.ApplyEmploymentImport(e.Admin(), contact, crmcontracts.EmploymentImportRequest{}); err != nil {
		t.Fatal(err)
	}
	if after := e.WsCount(t, `SELECT count(*) FROM audit_log`); before != after {
		t.Fatalf("replay wrote %d audit rows", after-before)
	}
}

func TestAUserEndedEmploymentBlocksBothImportAndCapture(t *testing.T) {
	for _, ending := range []string{"archive", "end date", "former status", "agent archive"} {
		t.Run(ending, func(t *testing.T) {
			e := Setup(t)
			company := seedEmployerOnDomain(t, e, "Ended Employer", "intent-ended.test")
			first := seedMailFrom(t, "reader@intent-ended.test", mailAtTheNewJob)
			subject := captureSender(t, e, first, "reader@intent-ended.test", "intent-ended.test")
			edge := ids.MustParse(e.WsScalar(t, `SELECT id::text FROM relationship WHERE contact_id=@contact AND company_id=@company AND kind='employment'`, pgx.NamedArgs{"contact": subject, "company": company}))
			var err error
			switch ending {
			case "agent archive":
				_, err = e.Contacts.ArchiveRelationship(e.AgentFor(t, e.AdminUser, nil, AdminPerms), edge, nil)
			case "archive":
				_, err = e.Contacts.ArchiveRelationship(e.Admin(), edge, nil)
			case "end date":
				at := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
				_, err = e.Contacts.UpdateRelationship(e.Admin(), edge, contacts.UpdateRelationshipInput{EndedAt: &at})
			case "former status":
				former := "former"
				_, err = e.Contacts.UpdateRelationship(e.Admin(), edge, contacts.UpdateRelationshipInput{EmploymentStatus: &former})
			}
			if err != nil {
				t.Fatal(err)
			}
			employmentPurchase(t, e, subject, []provider.Claim{{Key: provider.ClaimCurrentEmployment, Value: []byte(`{"company_domain":"intent-ended.test","company_name":"Ended Employer","job_title":"Director"}`)}})
			report, err := e.Contacts.ApplyEmploymentImport(e.Admin(), ids.From[ids.ContactKind](subject), crmcontracts.EmploymentImportRequest{})
			if err != nil {
				t.Fatal(err)
			}
			if len(report.Items) != 1 || report.Items[0].State != "needs_review" {
				t.Fatalf("import revived a user-ended link: %+v", report.Items)
			}
			next := seedMailFrom(t, "reader@intent-ended.test", mailAtTheNewJob)
			captureSender(t, e, next, "reader@intent-ended.test", "intent-ended.test")
			if live := e.WsCount(t, `SELECT count(*) FROM relationship WHERE contact_id=@contact AND company_id=@company AND `+employment.LiveSlotSQL("")+``, pgx.NamedArgs{"contact": subject, "company": company}); live != 0 {
				t.Fatalf("capture revived %d links", live)
			}
		})
	}
}

func TestEmploymentUserDecisionsUseTheSameAuthorityForMembersAndMCP(t *testing.T) {
	e := Setup(t)
	for _, c := range []struct {
		actor    string
		behalf   *ids.UUID
		evidence string
		want     string
	}{
		{"human", nil, "{}", "true"},
		{"agent", &e.Rep1, "{}", "true"},
		{"agent", nil, "{}", "false"},
		{"human", nil, `{"run_id":"provider-import"}`, "false"},
		{"system", nil, "{}", "false"},
	} {
		got := e.WsScalar(t, `SELECT (`+employment.UserDecisionSQL("a")+`)::text FROM (VALUES(@actor::text,@behalf::uuid,@evidence::jsonb)) a(actor_type,on_behalf_of,evidence)`, pgx.StrictNamedArgs{"actor": c.actor, "behalf": c.behalf, "evidence": c.evidence})
		if got != c.want {
			t.Errorf("actor %s behalf %v evidence %s: %s", c.actor, c.behalf, c.evidence, got)
		}
	}
}
