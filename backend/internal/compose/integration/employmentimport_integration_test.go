// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
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

func employmentPurchase(t *testing.T, e *Env, subject ids.UUID, claims []provider.Claim) {
	t.Helper()
	run := seedRun(t, e, subject, "completed", ids.NewV7().String(), false)
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return contacts.WriteProviderClaims(e.Admin(), tx, run, subject.String(), "surfe", claims, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
	}); err != nil {
		t.Fatal(err)
	}
}

func TestEmploymentImportPreservesDistinctRolesAndReplays(t *testing.T) {
	e := Setup(t)
	store := contacts.NewStore(e.DB())
	subject := seedMergeSubject(t, e, "Employment history")
	contact := ids.From[ids.ContactKind](subject)
	_, err := store.CreateCompany(e.Admin(), contacts.CreateCompanyInput{DisplayName: "History Co", Source: "manual", Domains: []contacts.CompanyDomainInput{{Domain: "employment-history.example", IsPrimary: true}}})
	if err != nil {
		t.Fatal(err)
	}
	employmentPurchase(t, e, subject, []provider.Claim{{Key: provider.ClaimJobHistory, Value: []byte(`[
 {"company_name":"employment-history.example","job_title":"Engineer","started_at":"2020-02","ended_at":"2021-07"},
 {"company_name":"employment-history.example","job_title":"Director","started_at":"2021-08-17","ended_at":"2024-01-23"},
 {"company_name":"employment-history.example","job_title":"Advisor"},
 {"company_name":"Unresolved Name","job_title":"Consultant"}]`)}})
	preview, err := store.PreviewEmploymentImport(e.Admin(), contact)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Items) != 4 {
		t.Fatalf("preview items=%d", len(preview.Items))
	}
	for _, item := range preview.Items {
		if item.State != "pending" {
			t.Fatalf("preview wrote a resolution: %+v", item)
		}
	}
	report, err := store.ApplyEmploymentImport(e.Admin(), contact, crmcontracts.EmploymentImportRequest{})
	if err != nil {
		t.Fatal(err)
	}
	linked := 0
	for _, item := range report.Items {
		if item.State == "linked" {
			linked++
		}
		if item.Role == "Advisor" && item.EmploymentStatus != "unknown" {
			t.Fatal("undated history became current")
		}
	}
	if linked != 3 {
		t.Fatalf("linked=%d, report=%+v", linked, report)
	}
	again, err := store.ApplyEmploymentImport(e.Admin(), contact, crmcontracts.EmploymentImportRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for i, item := range again.Items {
		if item.RelationshipId != nil && *item.RelationshipId != *report.Items[i].RelationshipId {
			t.Fatal("replay created another relationship")
		}
	}
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		var count, current int
		if err := tx.QueryRow(context.Background(), `SELECT count(*), count(*) FILTER (WHERE `+employment.CurrentPrimarySlotSQL("")+`) FROM relationship WHERE contact_id=$1 AND archived_at IS NULL`, subject).Scan(&count, &current); err != nil {
			return err
		}
		if count != 3 || current != 0 {
			t.Fatalf("roles=%d, primary=%d", count, current)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	first := ids.UUID(*report.Items[0].RelationshipId)
	if _, err := store.ArchiveRelationship(e.Admin(), first, nil); err != nil {
		t.Fatal(err)
	}
	after, err := store.ApplyEmploymentImport(e.Admin(), contact, crmcontracts.EmploymentImportRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if after.Items[0].State != "dismissed" {
		t.Fatal("replay resurrected a removed role")
	}
}

func TestEmploymentImportCreatesTrustedCompanyAndKeepsAmbiguity(t *testing.T) {
	e := Setup(t)
	store := contacts.NewStore(e.DB())
	subject := seedMergeSubject(t, e, "New employer")
	contact := ids.From[ids.ContactKind](subject)
	employmentPurchase(t, e, subject, []provider.Claim{{Key: provider.ClaimCurrentEmployment, Value: []byte(`{"company_name":"Verified Co","company_domain":"verified-employer.example","job_title":"Lead"}`)}})
	report, err := store.ApplyEmploymentImport(e.Admin(), contact, crmcontracts.EmploymentImportRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Items) != 1 || report.Items[0].State != "linked" || report.Items[0].CompanyId == nil {
		t.Fatalf("new employer not linked: %+v", report)
	}
	if err := store.SweepEmploymentImports(e.Admin()); err != nil {
		t.Fatal(err)
	}
}

func TestContactPhoneCanBeAddedChangedAndCleared(t *testing.T) {
	e := Setup(t)
	store := contacts.NewStore(e.DB())
	subject := seedMergeSubject(t, e, "Phone editor")
	contact := ids.From[ids.ContactKind](subject)
	phones := []contacts.ContactPhoneInput{{Phone: "+442079460001", PhoneType: "work", IsPrimary: true}, {Phone: "+442079460002", PhoneType: "mobile", Position: 1}}
	added, err := store.UpdateContact(e.Admin(), contact, contacts.UpdateContactInput{Phones: phones})
	if err != nil {
		t.Fatal(err)
	}
	if added.Phones == nil || len(*added.Phones) != 2 {
		t.Fatal("phone add did not persist")
	}
	phones[0].IsPrimary = false
	phones[1].IsPrimary = true
	changed, err := store.UpdateContact(e.Admin(), contact, contacts.UpdateContactInput{Phones: phones, IfVersion: added.Version})
	if err != nil {
		t.Fatal(err)
	}
	for _, phone := range *changed.Phones {
		if phone.Phone == phones[1].Phone && !phone.IsPrimary {
			t.Fatal("primary swap did not persist")
		}
	}
	empty := []contacts.ContactPhoneInput{}
	cleared, err := store.UpdateContact(e.Admin(), contact, contacts.UpdateContactInput{Phones: empty, IfVersion: changed.Version})
	if err != nil {
		t.Fatal(err)
	}
	if cleared.Phones != nil && len(*cleared.Phones) != 0 {
		t.Fatal("phone removal did not persist")
	}
	if cleared.Emails == nil || len(*cleared.Emails) != 1 {
		t.Fatal("phone editor changed email list")
	}
}

func TestEmploymentImportHumanCorrectionSurvivesReplayAndRevert(t *testing.T) {
	e := Setup(t)
	store := contacts.NewStore(e.DB())
	subject := seedMergeSubject(t, e, "Human correction")
	contact := ids.From[ids.ContactKind](subject)
	employmentPurchase(t, e, subject, []provider.Claim{{Key: provider.ClaimCurrentEmployment, Value: []byte(`{"company_name":"Correction Co","company_domain":"correction-employer.example","job_title":"Lead"}`)}})
	report, err := store.ApplyEmploymentImport(e.Admin(), contact, crmcontracts.EmploymentImportRequest{})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.UUID(*report.Items[0].RelationshipId)
	role, status := "Advisor", "former"
	row, err := store.UpdateRelationship(e.Admin(), id, contacts.UpdateRelationshipInput{Role: &role, EmploymentStatus: &status, ClearEndedAt: true})
	if err != nil {
		t.Fatal(err)
	}
	if row.IsCurrentPrimary || row.EmploymentStatus == nil || *row.EmploymentStatus != "former" {
		t.Fatal("correction stayed current")
	}
	if _, err := store.ApplyEmploymentImport(e.Admin(), contact, crmcontracts.EmploymentImportRequest{}); err != nil {
		t.Fatal(err)
	}
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		reverted, err := contacts.RevertProviderFills(e.Admin(), tx, "surfe", subject)
		if err != nil {
			return err
		}
		if len(reverted.Fields) != 0 {
			t.Fatal("revert removed a human correction")
		}
		var got string
		return tx.QueryRow(e.Admin(), `SELECT role FROM relationship WHERE id=$1 AND archived_at IS NULL`, id).Scan(&got)
	}); err != nil {
		t.Fatal(err)
	}
}

func TestEmploymentImportMergeKeepsRolesAndMovesSupport(t *testing.T) {
	e := Setup(t)
	store := contacts.NewStore(e.DB())
	source := seedMergeSubject(t, e, "Merge source")
	target := seedMergeSubject(t, e, "Merge target")
	claims := []provider.Claim{{Key: provider.ClaimCurrentEmployment, Value: []byte(`{"company_name":"Merged Employer","company_domain":"merged-employer.example","job_title":"Lead"}`)}}
	employmentPurchase(t, e, source, claims)
	employmentPurchase(t, e, target, claims)
	first, err := store.ApplyEmploymentImport(e.Admin(), ids.From[ids.ContactKind](source), crmcontracts.EmploymentImportRequest{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.ApplyEmploymentImport(e.Admin(), ids.From[ids.ContactKind](target), crmcontracts.EmploymentImportRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if *first.Items[0].RelationshipId == *second.Items[0].RelationshipId {
		t.Fatal("different contacts shared an edge before merge")
	}
	if _, err := store.MergeContact(e.Admin(), ids.From[ids.ContactKind](source), ids.From[ids.ContactKind](target)); err != nil {
		t.Fatal(err)
	}
	replay, err := store.ApplyEmploymentImport(e.Admin(), ids.From[ids.ContactKind](target), crmcontracts.EmploymentImportRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(replay.Items) != 1 || replay.Items[0].State != "linked" || *replay.Items[0].RelationshipId != *second.Items[0].RelationshipId {
		t.Fatalf("merge lost surviving support: %+v", replay)
	}
}

func TestEmploymentImportConcurrentContactsShareOneCompany(t *testing.T) {
	e := Setup(t)
	store := contacts.NewStore(e.DB())
	subjects := []ids.UUID{seedMergeSubject(t, e, "Concurrent A"), seedMergeSubject(t, e, "Concurrent B")}
	for _, subject := range subjects {
		employmentPurchase(t, e, subject, []provider.Claim{{Key: provider.ClaimCurrentEmployment, Value: []byte(`{"company_name":"Shared Employer","company_domain":"concurrent-employer.example","job_title":"Lead"}`)}})
	}
	reports := make(chan crmcontracts.EmploymentImportReport, 2)
	failures := make(chan error, 2)
	start := make(chan struct{})
	for _, subject := range subjects {
		go func() {
			<-start
			report, err := store.ApplyEmploymentImport(e.Admin(), ids.From[ids.ContactKind](subject), crmcontracts.EmploymentImportRequest{})
			reports <- report
			failures <- err
		}()
	}
	close(start)
	first, second := <-reports, <-reports
	for range 2 {
		if err := <-failures; err != nil {
			t.Fatal(err)
		}
	}
	if len(first.Items) != 1 || len(second.Items) != 1 || first.Items[0].CompanyId == nil || second.Items[0].CompanyId == nil {
		t.Fatal("a concurrent import lost its company")
	}
	if *first.Items[0].CompanyId != *second.Items[0].CompanyId {
		t.Fatal("concurrent imports created duplicate companies")
	}
}

func TestEmploymentImportMonthEndAcceptsExactStartInTheSameMonth(t *testing.T) {
	e := Setup(t)
	store := contacts.NewStore(e.DB())
	subject := seedMergeSubject(t, e, "Month bounds")
	company := e.SeedCompany(t, "Month employer", nil)
	contact, employer := ids.From[ids.ContactKind](subject), ids.From[ids.CompanyKind](company)
	start := time.Date(2024, 9, 17, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 9, 1, 0, 0, 0, 0, time.UTC)
	month, status := "month", "former"
	row, err := store.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{Kind: "employment", ContactID: &contact, CompanyID: &employer, StartedAt: &start, EndedAt: &end, EndedPrecision: &month, EmploymentStatus: &status, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if row.EndedAt == nil || row.EndedAt.Day() != 1 || *row.EndedPrecision != "month" {
		t.Fatal("month precision was replaced with an invented day")
	}
	updated, err := store.UpdateRelationship(e.Admin(), row.ID, contacts.UpdateRelationshipInput{ClearEndedAt: true, IfVersion: &row.Version})
	if err != nil {
		t.Fatal(err)
	}
	if updated.EndedAt != nil || updated.EndedPrecision != nil || updated.IsCurrentPrimary {
		t.Fatal("clearing a date changed status or kept false precision")
	}
}

func TestEmploymentImportRetractionWaitsForTheFinalProvider(t *testing.T) {
	e := Setup(t)
	store := contacts.NewStore(e.DB())
	subject := seedMergeSubject(t, e, "Shared evidence")
	contact := ids.From[ids.ContactKind](subject)
	claims := []provider.Claim{{Key: provider.ClaimCurrentEmployment, Value: []byte(`{"company_name":"Supported Employer","company_domain":"supported-employer.example","job_title":"Lead"}`)}}
	employmentPurchase(t, e, subject, claims)
	first, err := store.ApplyEmploymentImport(e.Admin(), contact, crmcontracts.EmploymentImportRequest{})
	if err != nil {
		t.Fatal(err)
	}
	run := seedRun(t, e, subject, "completed", ids.NewV7().String(), false)
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(e.Admin(), `UPDATE provider_run SET provider='second_provider' WHERE id=$1`, run); err != nil {
			return err
		}
		return contacts.WriteProviderClaims(e.Admin(), tx, run, subject.String(), "second_provider", claims, time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC))
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyEmploymentImport(e.Admin(), contact, crmcontracts.EmploymentImportRequest{}); err != nil {
		t.Fatal(err)
	}
	for index, vendor := range []string{"surfe", "second_provider"} {
		if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
			if _, err := contacts.RevertProviderFills(e.Admin(), tx, vendor, subject); err != nil {
				return err
			}
			if _, err := tx.Exec(e.Admin(), `DELETE FROM contact_provider_claim WHERE contact_id=$1 AND provider=$2`, subject, vendor); err != nil {
				return err
			}
			var archived bool
			if err := tx.QueryRow(e.Admin(), `SELECT archived_at IS NOT NULL FROM relationship WHERE id=$1`, ids.UUID(*first.Items[0].RelationshipId)).Scan(&archived); err != nil {
				return err
			}
			if archived != (index == 1) {
				t.Fatalf("provider %s removal archived=%v", vendor, archived)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
}
