// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestEnrichmentApprovalShowsTheActualReadTarget(t *testing.T) {
	for _, tc := range []struct{ depth, operation, target string }{
		{"page", "scrapeCompany", "https://example.org/team"},
		{"site", "deepReadCompany", "https://example.org/team"},
		{"technical", "technicalEnrichCompany", "example.com"},
	} {
		for _, rest := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/rest=%t", tc.depth, rest), func(t *testing.T) {
				e := integration.Setup(t)
				company, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{DisplayName: "Harbor", Domains: []contacts.CompanyDomainInput{{Domain: "example.com", IsPrimary: true}}})
				if err != nil {
					t.Fatal(err)
				}
				payload := fmt.Sprintf(`{"company_id":%q,"depth":%q,"url":"https://example.org/team"}`, company.Id, tc.depth)
				if rest {
					payload = fmt.Sprintf(`{"operation":%q,"body":{"url":"https://example.org/team"}}`, tc.operation)
				}
				raw, hash, err := diffhash.Canonical([]byte(payload))
				if err != nil {
					t.Fatal(err)
				}
				svc := NewApprovalsService(e.DB())
				id, err := svc.Stage(e.Admin(), approvals.StageInput{Kind: "enrich", TargetType: "company", TargetID: ids.UUID(company.Id), ProposedChange: raw, DiffHash: hash})
				if err != nil {
					t.Fatal(err)
				}
				proposal, err := svc.GetWire(e.Admin(), id)
				if err != nil {
					t.Fatal(err)
				}
				preview := proposal.Preview
				if preview == nil || preview.State != "ready" || preview.Details == nil {
					t.Fatalf("preview: %+v", preview)
				}
				details := map[string]string{}
				for _, detail := range *preview.Details {
					details[detail.Field] = detail.Value.Text
				}
				if details["target_url"] != tc.target || details["enrichment_depth"] != tc.depth {
					t.Fatalf("read scope: %+v", details)
				}
				if preview.Effects == nil || len(*preview.Effects) != 1 || (*preview.Effects)[0] != "authorize_website_read" {
					t.Fatalf("effects: %+v", preview.Effects)
				}
			})
		}
	}
}
