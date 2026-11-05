// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"fmt"
	"maps"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestTagMergeApprovalDistinguishesMovedAndCollapsedAssignments(t *testing.T) {
	for _, rest := range []bool{false, true} {
		t.Run(fmt.Sprintf("rest=%t", rest), func(t *testing.T) {
			e := integration.Setup(t)
			perms := integration.AdminPerms
			perms.Objects = maps.Clone(perms.Objects)
			perms.Objects["tag"] = principal.ObjectGrant{Read: true, Create: true, Update: true, Delete: true}
			ctx := e.As(e.AdminUser, nil, perms)
			store := collections.NewStore(e.DB())
			source, err := store.CreateTag(ctx, "Source category", nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			target, err := store.CreateTag(ctx, "Surviving category", nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			for i, name := range []string{"Gains surviving category", "Already has both categories"} {
				company, err := e.Contacts.CreateCompany(ctx, contacts.CreateCompanyInput{DisplayName: name, Source: "manual"})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := store.ApplyTag(ctx, source.ID, "company", ids.UUID(company.Id)); err != nil {
					t.Fatal(err)
				}
				if i == 1 {
					if _, err := store.ApplyTag(ctx, target.ID, "company", ids.UUID(company.Id)); err != nil {
						t.Fatal(err)
					}
				}
			}
			payload := map[string]any{"tag_id": source.ID, "into_tag_id": target.ID}
			if rest {
				payload = map[string]any{"operation": "mergeTags", "path": "/v1/tags/" + source.ID.String() + "/merge", "body": map[string]any{"into_tag_id": target.ID, "into_tag_version": target.Version}}
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
			id, err := svc.Stage(ctx, approvals.StageInput{Kind: "merge_tags", TargetType: "tag", TargetID: source.ID.UUID, CoTargetType: "tag", CoTargetID: target.ID.UUID, ProposedChange: raw, DiffHash: hash})
			if err != nil {
				t.Fatal(err)
			}
			proposal, err := svc.GetWire(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			p := proposal.Preview
			if p == nil || p.State != "ready" || p.Details == nil || p.Effects == nil || len(*p.Effects) != 1 || (*p.Effects)[0] != "merge_tags_release_source" {
				t.Fatalf("merge preview: %+v", p)
			}
			fields := map[string]string{}
			for _, detail := range *p.Details {
				fields[detail.Field] = detail.Value.Text
			}
			for field, want := range map[string]string{"merge_tag_source": "Source category", "merge_tag_target": "Surviving category", "merge_tag_move": "Gains surviving category", "merge_tag_collapse": "Already has both categories"} {
				if fields[field] != want {
					t.Fatalf("%s=%q, want %q", field, fields[field], want)
				}
			}
			limited := perms
			limited.Objects = maps.Clone(perms.Objects)
			delete(limited.Objects, "company")
			withheld, err := svc.GetWire(e.As(e.AdminUser, nil, limited), id)
			if err != nil {
				t.Fatal(err)
			}
			if withheld.Preview == nil || withheld.Preview.State == "ready" || withheld.Preview.Details != nil {
				t.Fatalf("hidden merge scope: %+v", withheld.Preview)
			}
			result, err := store.MergeTags(ctx, source.ID, target.ID, target.Version)
			if err != nil {
				t.Fatal(err)
			}
			if result.Moved != 1 || result.Collapsed != 1 {
				t.Fatalf("writer disagrees: %+v", result)
			}
			_, usage, err := store.GetTag(ctx, target.ID)
			if err != nil || usage.Companies != 2 {
				t.Fatalf("survivor usage: %+v / %v", usage, err)
			}
			if _, err := store.CreateTag(ctx, "Source category", nil, nil); err != nil {
				t.Fatalf("source name not released: %v", err)
			}
		})
	}
}
