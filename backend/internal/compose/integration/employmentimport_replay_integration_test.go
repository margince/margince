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
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

func TestEmploymentImportResolvesRepeatedPurchasedEpisode(t *testing.T) {
	e := Setup(t)
	store := contacts.NewStore(e.DB())
	subject := seedMergeSubject(t, e, "Repeated employment purchase")
	contact := ids.From[ids.ContactKind](subject)
	company, err := store.CreateCompany(e.Admin(), contacts.CreateCompanyInput{DisplayName: "Chosen Employer", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	claims := []provider.Claim{{Key: provider.ClaimJobHistory, Value: []byte(`[{"company_name":"Ambiguous Employer","job_title":"Advisor"}]`)}}
	employmentPurchase(t, e, subject, claims)
	employmentPurchase(t, e, subject, claims)
	before, err := store.PreviewEmploymentImport(e.Admin(), contact)
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Items) != 1 {
		t.Fatalf("repeated evidence should show once: %+v", before)
	}
	action := crmcontracts.EmploymentImportRequestAction("resolve")
	after, err := store.ApplyEmploymentImport(e.Admin(), contact, crmcontracts.EmploymentImportRequest{Action: &action, Key: &before.Items[0].Key, CompanyId: &company.Id})
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Items) != 1 || after.Items[0].State != "linked" {
		t.Fatalf("resolution failed: %+v", after)
	}
	replay, err := store.ApplyEmploymentImport(e.Admin(), contact, crmcontracts.EmploymentImportRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if replay.Items[0].RelationshipId == nil || *replay.Items[0].RelationshipId != *after.Items[0].RelationshipId {
		t.Fatal("replay replaced the chosen relationship")
	}
}

func TestEmploymentImportTrustsExplicitHistoricalDomain(t *testing.T) {
	e := Setup(t)
	store := contacts.NewStore(e.DB())
	subject := seedMergeSubject(t, e, "Explicit history domain")
	contact := ids.From[ids.ContactKind](subject)
	employmentPurchase(t, e, subject, []provider.Claim{{Key: provider.ClaimJobHistory, Value: []byte(`[{"company_name":"Past Employer","company_domain":"explicit-history.example","job_title":"Engineer","ended_at":"2020-06"}]`)}})
	report, err := store.ApplyEmploymentImport(e.Admin(), contact, crmcontracts.EmploymentImportRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Items) != 1 || report.Items[0].State != "linked" || report.Items[0].CompanyId == nil {
		t.Fatalf("explicit provider domain did not create the employer: %+v", report)
	}
}

func TestEmploymentImportOneMatchResolvesAllRolesAtEmployer(t *testing.T) {
	e := Setup(t)
	store := contacts.NewStore(e.DB())
	subject := seedMergeSubject(t, e, "Group matching")
	contact := ids.From[ids.ContactKind](subject)
	company, err := store.CreateCompany(e.Admin(), contacts.CreateCompanyInput{DisplayName: "Chosen Employer", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	employmentPurchase(t, e, subject, []provider.Claim{{Key: provider.ClaimJobHistory, Value: []byte(`[{"company_name":"Employer Name","job_title":"Engineer","ended_at":"2020-01"},{"company_name":"Employer Name","job_title":"Advisor"}]`)}})
	before, err := store.PreviewEmploymentImport(e.Admin(), contact)
	if err != nil {
		t.Fatal(err)
	}
	action := crmcontracts.EmploymentImportRequestAction("resolve")
	group := true
	after, err := store.ApplyEmploymentImport(e.Admin(), contact, crmcontracts.EmploymentImportRequest{Action: &action, Key: &before.Items[0].Key, CompanyId: &company.Id, ResolveGroup: &group})
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Items) != 2 {
		t.Fatalf("lost roles: %+v", after)
	}
	for _, item := range after.Items {
		if item.State != "linked" || item.CompanyId == nil || *item.CompanyId != company.Id {
			t.Fatalf("group not resolved: %+v", item)
		}
	}
	if *after.Items[0].RelationshipId == *after.Items[1].RelationshipId {
		t.Fatal("different roles collapsed")
	}
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := contacts.RevertProviderFills(e.Admin(), tx, "surfe", subject); err != nil {
			return err
		}
		var count int
		if err := tx.QueryRow(e.Admin(), `SELECT count(*) FROM relationship WHERE contact_id=$1 AND archived_at IS NULL`, subject).Scan(&count); err != nil {
			return err
		}
		if count != 2 {
			t.Fatalf("retraction removed human-confirmed company links: %d", count)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestEmploymentImportNewSnapshotWithdrawsOldCurrentAssertion(t *testing.T) {
	e := Setup(t)
	store := contacts.NewStore(e.DB())
	subject := seedMergeSubject(t, e, "Changing employer")
	contact := ids.From[ids.ContactKind](subject)
	employmentPurchase(t, e, subject, []provider.Claim{{Key: provider.ClaimCurrentEmployment, Value: []byte(`{"company_name":"Earlier Employer","company_domain":"snapshot-old.example","job_title":"Lead"}`)}})
	before, err := store.ApplyEmploymentImport(e.Admin(), contact, crmcontracts.EmploymentImportRequest{})
	if err != nil {
		t.Fatal(err)
	}
	old := ids.UUID(*before.Items[0].RelationshipId)
	employmentPurchase(t, e, subject, []provider.Claim{{Key: provider.ClaimCurrentEmployment, Value: []byte(`{"company_name":"New Employer","company_domain":"snapshot-new.example","job_title":"Director"}`)}})
	after, err := store.ApplyEmploymentImport(e.Admin(), contact, crmcontracts.EmploymentImportRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Items) != 2 {
		t.Fatalf("lost history: %+v", after)
	}
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		var status string
		var primary bool
		var end *time.Time
		if err := tx.QueryRow(e.Admin(), `SELECT employment_status,is_current_primary,ended_at FROM relationship WHERE id=$1`, old).Scan(&status, &primary, &end); err != nil {
			return err
		}
		if status != "unknown" || primary || end != nil {
			t.Fatalf("old snapshot retained current assertion or invented departure: %s %v %v", status, primary, end)
		}
		var count int
		if err := tx.QueryRow(e.Admin(), `SELECT count(*) FROM relationship WHERE contact_id=$1 AND `+employment.CurrentPrimarySQL("")+` AND archived_at IS NULL`, subject).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			t.Fatalf("current primary count=%d", count)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestEmploymentImportMalformedHistoryDoesNotBlockValidRole(t *testing.T) {
	e := Setup(t)
	store := contacts.NewStore(e.DB())
	subject := seedMergeSubject(t, e, "Malformed history")
	contact := ids.From[ids.ContactKind](subject)
	employmentPurchase(t, e, subject, []provider.Claim{{Key: provider.ClaimJobHistory, Value: []byte(`{"unexpected":"not an array"}`)}, {Key: provider.ClaimCurrentEmployment, Value: []byte(`{"company_name":"Valid Employer","company_domain":"valid-history.example","job_title":"Lead"}`)}})
	if err := store.SweepEmploymentImports(e.Admin()); err != nil {
		t.Fatal(err)
	}
	report, err := store.PreviewEmploymentImport(e.Admin(), contact)
	if err != nil {
		t.Fatal(err)
	}
	states := map[string]int{}
	for _, item := range report.Items {
		states[string(item.State)]++
	}
	if states["linked"] != 1 || states["needs_review"] != 1 {
		t.Fatalf("bad evidence blocked valid role: %+v", report)
	}
}

func TestEmploymentImportMergePreservesDifferentHistoricalRoles(t *testing.T) {
	e := Setup(t)
	store := contacts.NewStore(e.DB())
	source := seedMergeSubject(t, e, "Earlier career")
	target := seedMergeSubject(t, e, "Later career")
	for i, subject := range []ids.UUID{source, target} {
		role := "Engineer"
		if i == 1 {
			role = "Director"
		}
		employmentPurchase(t, e, subject, []provider.Claim{{Key: provider.ClaimJobHistory, Value: []byte(`[{"company_name":"Shared Employer","company_domain":"distinct-merge.example","job_title":"` + role + `","ended_at":"2020-01"}]`)}})
		if _, err := store.ApplyEmploymentImport(e.Admin(), ids.From[ids.ContactKind](subject), crmcontracts.EmploymentImportRequest{}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.MergeContact(e.Admin(), ids.From[ids.ContactKind](source), ids.From[ids.ContactKind](target)); err != nil {
		t.Fatal(err)
	}
	report, err := store.ApplyEmploymentImport(e.Admin(), ids.From[ids.ContactKind](target), crmcontracts.EmploymentImportRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Items) != 2 || report.Items[0].State != "linked" || report.Items[1].State != "linked" || *report.Items[0].RelationshipId == *report.Items[1].RelationshipId {
		t.Fatalf("merge collapsed distinct historical roles: %+v", report)
	}
}
