// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestRelationshipArchiveApprovalDescribesTheActualEmployerSuccession(t *testing.T) {
	for _, remaining := range []int{0, 1, 2} {
		for _, rest := range []bool{false, true} {
			t.Run(fmt.Sprintf("remaining=%d/rest=%t", remaining, rest), func(t *testing.T) {
				e := integration.Setup(t)
				perms := integration.AdminPerms
				perms.Objects = maps.Clone(perms.Objects)
				perms.Objects["relationship"] = principal.ObjectGrant{Read: true, Create: true, Update: true, Delete: true}
				ctx := e.As(e.AdminUser, nil, perms)
				contact, err := e.Contacts.CreateContact(ctx, contacts.CreateContactInput{FullName: "Employee", Source: "manual"})
				if err != nil {
					t.Fatal(err)
				}
				contactID := ids.From[ids.ContactKind](ids.UUID(contact.Id))
				var edges []ids.UUID
				for i := 0; i <= remaining; i++ {
					company, err := e.Contacts.CreateCompany(ctx, contacts.CreateCompanyInput{DisplayName: fmt.Sprintf("Employer %d", i), Source: "manual"})
					if err != nil {
						t.Fatal(err)
					}
					companyID := ids.From[ids.CompanyKind](ids.UUID(company.Id))
					edge, err := e.Contacts.CreateRelationship(ctx, contacts.CreateRelationshipInput{Kind: "employment", ContactID: &contactID, CompanyID: &companyID, Role: new("Engineer"), StartedAt: new(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)), StartedPrecision: new("month"), IsCurrentPrimary: new(i == 0), Source: "manual"})
					if err != nil {
						t.Fatal(err)
					}
					edges = append(edges, edge.ID)
				}
				payload := map[string]any{"record_type": "relationship", "id": edges[0]}
				if rest {
					payload = map[string]any{"operation": "archiveRelationship", "path": "/v1/relationships/" + edges[0].String(), "body": nil}
				}
				encoded, err := json.Marshal(payload)
				if err != nil {
					t.Fatal(err)
				}
				raw, hash, err := diffhash.Canonical(encoded)
				if err != nil {
					t.Fatal(err)
				}
				svc := NewApprovalsService(e.DB())
				id, err := svc.Stage(ctx, approvals.StageInput{Kind: "archive_record", TargetType: "relationship", TargetID: edges[0], ProposedChange: raw, DiffHash: hash})
				if err != nil {
					t.Fatal(err)
				}
				proposal, err := svc.GetWire(ctx, id)
				if err != nil {
					t.Fatal(err)
				}
				p := proposal.Preview
				if p == nil || p.State != "ready" || p.Details == nil || p.Effects == nil || proposal.TargetVersion == nil {
					t.Fatalf("preview: %+v", p)
				}
				fields := map[string]string{}
				for _, detail := range *p.Details {
					fields[detail.Field] = detail.Value.Text
				}
				if fields["contact_id"] != "Employee" || fields["company_id"] != "Employer 0" || fields["archive_role"] != "Engineer" || fields["relationship_started"] != "2024-01-01" || fields["relationship_start_precision"] != "month" {
					t.Fatalf("unnamed link: %+v", fields)
				}
				if remaining == 1 {
					if fields["archive_primary_employer"] != "Employer 1" || !slices.Contains(*p.Effects, "archive_relationship_promote_employer") {
						t.Fatalf("successor absent: %+v", p)
					}
				} else if fields["archive_primary_employer"] != "" || !slices.Contains(*p.Effects, "archive_relationship_no_primary") {
					t.Fatalf("invented successor: %+v", p)
				}
				if _, err := e.Contacts.ArchiveRelationship(ctx, edges[0], new(int64(*proposal.TargetVersion))); err != nil {
					t.Fatal(err)
				}
				for _, edgeID := range edges[1:] {
					edge, err := e.Contacts.GetRelationship(ctx, edgeID)
					if err != nil {
						t.Fatal(err)
					}
					if edge.IsCurrentPrimary != (remaining == 1) {
						t.Fatalf("writer disagrees with succession: %+v", edge)
					}
				}
			})
		}
	}
}
