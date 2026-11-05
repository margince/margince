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

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

func TestDealArchiveApprovalNamesTheWritersEntireCascade(t *testing.T) {
	for _, rest := range []bool{false, true} {
		t.Run(fmt.Sprintf("rest=%t", rest), func(t *testing.T) {
			e := integration.Setup(t)
			perms := integration.AdminPerms
			perms.Objects = maps.Clone(perms.Objects)
			for _, object := range []string{"tag", "list", "relationship", "custom_field"} {
				perms.Objects[object] = principal.ObjectGrant{Read: true, Create: true, Update: true, Delete: true}
			}
			ctx := e.As(e.AdminUser, nil, perms)
			store := deals.NewStore(e.DB(), DealsInstallation())
			pipeline, stage, _ := integration.DealFixture(t, e)
			deal, err := store.CreateDeal(ctx, deals.CreateDealInput{Name: "Archive candidate", PipelineID: pipeline, StageID: stage, Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			dealID := ids.From[ids.DealKind](ids.UUID(deal.Id))
			contact, err := e.Contacts.CreateContact(ctx, contacts.CreateContactInput{FullName: "Review sponsor", Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			edge, err := e.Contacts.CreateRelationship(ctx, contacts.CreateRelationshipInput{Kind: "deal_stakeholder", ContactID: new(ids.From[ids.ContactKind](ids.UUID(contact.Id))), DealID: &dealID, Role: new("Decision maker"), Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			collection := collections.NewStore(e.DB())
			list, err := collection.CreateList(ctx, collections.CreateListInput{Name: "Review shortlist", EntityType: "deal"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := collection.AddMember(ctx, list.ID, collections.MemberChange{EntityType: "deal", EntityID: dealID.UUID, Reason: collections.ReasonChosen}); err != nil {
				t.Fatal(err)
			}
			tag, err := collection.CreateTag(ctx, "Review category", nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := collection.ApplyTag(ctx, tag.ID, "deal", dealID.UUID); err != nil {
				t.Fatal(err)
			}
			other, err := store.CreateDeal(ctx, deals.CreateDealInput{Name: "Keep this deal", PipelineID: pipeline, StageID: stage, Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			otherID := ids.From[ids.DealKind](ids.UUID(other.Id))
			otherEdge, err := e.Contacts.CreateRelationship(ctx, contacts.CreateRelationshipInput{Kind: "deal_stakeholder", ContactID: new(ids.From[ids.ContactKind](ids.UUID(contact.Id))), DealID: &otherID, Role: new("Sponsor"), Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := collection.AddMember(ctx, list.ID, collections.MemberChange{EntityType: "deal", EntityID: otherID.UUID, Reason: collections.ReasonChosen}); err != nil {
				t.Fatal(err)
			}
			if _, err := collection.ApplyTag(ctx, tag.ID, "deal", otherID.UUID); err != nil {
				t.Fatal(err)
			}
			payload := map[string]any{"record_type": "deal", "id": deal.Id}
			if rest {
				payload = map[string]any{"operation": "archiveDeal", "path": "/v1/deals/" + deal.Id.String(), "body": nil}
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
			id, err := svc.Stage(ctx, approvals.StageInput{Kind: "archive_record", TargetType: "deal", TargetID: dealID.UUID, ProposedChange: raw, DiffHash: hash})
			if err != nil {
				t.Fatal(err)
			}
			proposal, err := svc.GetWire(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			preview := proposal.Preview
			if preview == nil || preview.State != "ready" || preview.Details == nil || proposal.TargetVersion == nil {
				t.Fatalf("preview: %+v", preview)
			}
			reader := newApprovalPreviewReader(e.DB())
			record, err := reader.records.Read(ctx, datasource.EntityRef{Type: datasource.EntityDeal, ID: dealID.UUID})
			if err != nil {
				t.Fatal(err)
			}
			stale := proposal
			stale.TargetVersion = new(*proposal.TargetVersion - 1)
			stalePreview, err := reader.archivePreview(ctx, stale, record.Fields, &crmcontracts.ApprovalPreview{})
			if err != nil || stalePreview == nil || stalePreview.State != "stale" {
				t.Fatalf("changed archive context: %+v / %v", stalePreview, err)
			}
			values := map[string]string{}
			for _, detail := range *preview.Details {
				values[detail.Field] = detail.Value.Text
			}
			for field, want := range map[string]string{"archive_stakeholder": "Review sponsor", "archive_role": "Decision maker", "archive_list": "Review shortlist", "archive_tag": "Review category"} {
				if values[field] != want {
					t.Fatalf("%s = %q, want %q", field, values[field], want)
				}
			}
			for _, hidden := range []string{"tag", "list", "contact", "relationship"} {
				limited := perms
				limited.Objects = maps.Clone(perms.Objects)
				delete(limited.Objects, hidden)
				wire, err := svc.GetWire(e.As(e.AdminUser, nil, limited), id)
				if err != nil {
					t.Fatal(err)
				}
				if wire.Preview == nil || wire.Preview.State == "ready" || wire.Preview.Details != nil {
					t.Fatalf("%s scope leaked: %+v", hidden, wire.Preview)
				}
			}
			if _, err := store.ArchiveDeal(ctx, dealID, new(int64(*proposal.TargetVersion))); err != nil {
				t.Fatal(err)
			}
			remaining, err := store.PreviewDealArchive(ctx, otherID, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(remaining.Relationships, []ids.UUID{otherEdge.ID}) || !slices.Equal(remaining.Lists, []ids.UUID{list.ID.UUID}) || !slices.Equal(remaining.Tags, []ids.UUID{tag.ID.UUID}) {
				t.Fatalf("unrelated deal scope changed: %+v", remaining)
			}
			if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
				archived, err := storekit.LatestArchive(ctx, tx, "deal", dealID.UUID)
				if err != nil {
					return err
				}
				cascade := archived.Cascade
				if !slices.Equal(cascade.Retired["relationship"], []ids.UUID{edge.ID}) || len(cascade.Memberships) != 1 || cascade.Memberships[0].ListID != list.ID.UUID || len(cascade.Tags) != 1 || cascade.Tags[0].TagID != tag.ID.UUID {
					t.Fatalf("writer cascade: %+v", cascade)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
