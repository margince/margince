// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A site read's industry lands as a code, or not at all: free words reach
// neither the industry column nor the evidence row.
func TestASiteReadStoresAnIndustryCodeOrNothing(t *testing.T) {
	e := setupDedupe(t)
	for raw, want := range map[string]string{"hospitality_tourism": "hospitality_tourism", "casual dining restaurant": ""} {
		company, err := e.store.CreateCompany(e.as(), CreateCompanyInput{DisplayName: "Site " + raw, Source: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		companyID := ids.From[ids.CompanyKind](ids.UUID(company.Id))
		if err := e.store.ApplyDeepRead(e.as(), DeepReadProposal{
			CompanyID: companyID, SourceURL: "https://example.test/", SiteReadID: ids.NewV7(),
			Fields: []DeepReadField{{Field: columnIndustry, Value: raw, EvidenceSnippet: raw, SourceURL: "https://example.test/", Confidence: 0.9}},
		}); err != nil {
			t.Fatalf("ApplyDeepRead: %v", err)
		}
		reread, err := e.store.GetCompany(e.as(), companyID, storekit.LiveOnly)
		if err != nil {
			t.Fatal(err)
		}
		got := ""
		if reread.Industry != nil {
			got = string(*reread.Industry)
		}
		if got != want {
			t.Errorf("industry %q stored %q, want %q", raw, got, want)
		}
		fields, err := e.store.ListCompanyProfileFields(e.as(), companyID)
		if err != nil {
			t.Fatal(err)
		}
		for _, field := range fields {
			if string(field.Field) == columnIndustry && field.Value == raw && want == "" {
				t.Errorf("the free words %q were kept as profile evidence", raw)
			}
		}
	}
}
