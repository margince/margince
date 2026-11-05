// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestCompanyReadPreviewMatchesAppliedFieldsAndPreservesHumanClaims(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	company, err := e.store.CreateCompany(ctx, CreateCompanyInput{DisplayName: "Preview Glass", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.From[ids.CompanyKind](ids.UUID(company.Id))
	proposal := DeepReadProposal{CompanyID: id, SourceURL: "https://preview.example.test/about", Fields: []DeepReadField{
		{Field: "industry", Value: "Construction", EvidenceSnippet: "We supply architectural glass.", SourceURL: "https://preview.example.test/about", Confidence: 0.9},
		{Field: "icp", Value: "Builders", EvidenceSnippet: "For builders.", SourceURL: "https://preview.example.test/about", Confidence: 0.9},
	}, Facts: []DeepReadFact{{Category: companyEntity, Field: FactEmployeeRange, Value: "25 employees", EvidenceSnippet: "Our 25 employees.", SourceURL: "https://preview.example.test/about", Confidence: 0.9}}}
	plan, err := e.store.PreviewCompanyRead(ctx, proposal)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Profiles) != 2 || len(plan.Facts) != 1 || len(plan.Columns) != 2 {
		t.Fatalf("incomplete preview: %+v", plan)
	}
	unchanged, err := e.store.GetCompany(ctx, id, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Version == nil || company.Version == nil || *unchanged.Version != *company.Version || unchanged.Industry != nil || unchanged.SizeBand != nil {
		t.Fatalf("preview changed the company: %+v", unchanged)
	}
	if err := e.store.ApplyDeepRead(ctx, proposal); err != nil {
		t.Fatal(err)
	}
	applied, err := e.store.GetCompany(ctx, id, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if applied.Industry == nil || *applied.Industry != "construction" || applied.SizeBand == nil || string(*applied.SizeBand) != "11-50" {
		t.Fatalf("apply differs from preview: %+v", applied)
	}
	human := "Public building contractors"
	if _, err := e.store.UpdateCompanyProfileField(ctx, id, "icp", ProfileFieldWriteInput{Value: &human}); err != nil {
		t.Fatal(err)
	}
	proposal.Fields[1].Value = "Unrelated buyers"
	plan, err = e.store.PreviewCompanyRead(ctx, proposal)
	if err != nil {
		t.Fatal(err)
	}
	protected := plan.Profiles[1]
	if !protected.Retained || protected.Before != human || protected.After != human || protected.Proposed != "Unrelated buyers" {
		t.Fatalf("human claim hidden: %+v", protected)
	}
	if err := e.store.ApplyDeepRead(ctx, proposal); err != nil {
		t.Fatal(err)
	}
	profiles, err := e.store.ListCompanyProfileFields(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range profiles {
		if field.Field == "icp" {
			if field.Value != protected.After {
				t.Fatalf("applied %q, preview %q", field.Value, protected.After)
			}
			return
		}
	}
	t.Fatal("human profile disappeared")
}

func TestColdStartPreviewMatchesCreationAndSubsequentFill(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	input := ApplyColdStartProfileInput{
		SourceURL: "https://glass-preview.example.test/about",
		Fields: []ColdStartFieldInput{
			{Field: "legal_name", Value: "Glass Preview Limited", EvidenceSnippet: "Glass Preview Limited", SourceURL: "https://glass-preview.example.test/about", Confidence: 0.9},
			{Field: "industry", Value: "Construction", EvidenceSnippet: "Architectural glass", SourceURL: "https://glass-preview.example.test/about", Confidence: 0.9},
		},
	}
	plan, err := e.store.PreviewColdStart(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Creates || plan.Name != "Glass Preview Limited" || plan.Domain != "glass-preview.example.test" || len(plan.Read.Profiles) != 2 {
		t.Fatalf("creation preview=%+v", plan)
	}
	id, err := e.store.ApplyColdStartProfile(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	company, err := e.store.GetCompany(ctx, id, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if company.DisplayName != plan.Name || company.Domains == nil || len(*company.Domains) != 1 || (*company.Domains)[0].Domain != plan.Domain {
		t.Fatalf("created company differs from preview: %+v", company)
	}
	input.Fields[1].Value = "Software"
	plan, err = e.store.PreviewColdStart(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Creates || plan.Read.Company.Id != company.Id {
		t.Fatalf("existing company not reused: %+v", plan)
	}
	appliedID, err := e.store.ApplyColdStartProfile(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	applied, err := e.store.GetCompany(ctx, appliedID, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	for _, column := range plan.Read.Columns {
		if column.Field == "industry" {
			if applied.Industry == nil || string(*applied.Industry) != column.After || !column.Retained {
				t.Fatalf("retained industry differs from preview: %+v / %+v", column, applied)
			}
			return
		}
	}
	t.Fatal("industry absent from existing-company preview")
}
