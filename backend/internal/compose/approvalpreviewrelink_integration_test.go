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
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestRelinkApprovalNamesTheExactActivitiesAndRemovedLinks(t *testing.T) {
	for _, batch := range []bool{false, true} {
		for _, rest := range []bool{false, true} {
			t.Run(fmt.Sprintf("batch=%t/rest=%t", batch, rest), func(t *testing.T) {
				e := integration.Setup(t)
				perms := integration.AdminPerms
				perms.Objects = maps.Clone(perms.Objects)
				perms.Objects["custom_field"] = principal.ObjectGrant{Read: true}
				ctx := e.As(e.AdminUser, nil, perms)
				var targets []ids.UUID
				for _, name := range []string{"Original buyer", "Correct buyer"} {
					contact, err := e.Contacts.CreateContact(ctx, contacts.CreateContactInput{FullName: name, Source: "manual"})
					if err != nil {
						t.Fatal(err)
					}
					targets = append(targets, ids.UUID(contact.Id))
				}
				activity, _, err := e.Activities.LogActivity(ctx, activities.LogActivityInput{Kind: "note", Subject: new("Buyer discussion"), Source: "manual", Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: targets[0]}}})
				if err != nil {
					t.Fatal(err)
				}
				kind, targetType, targetID, operation := "relink_activity", "activity", ids.UUID(activity.Id), "RelinkActivity"
				body := map[string]any{"entity_type": "contact", "entity_id": targets[1], "replace_existing_of_type": true}
				if batch {
					kind, targetType, targetID, operation = "relink_activities", "contact", targets[1], "RelinkActivities"
					body["activity_ids"] = []ids.UUID{ids.UUID(activity.Id)}
				} else if !rest {
					body["activity_id"] = activity.Id
				}
				payload := body
				if rest {
					payload = map[string]any{"operation": operation, "body": body}
				}
				encoded, err := json.Marshal(payload)
				if err != nil {
					t.Fatal(err)
				}
				raw, hash, err := diffhash.Canonical(encoded)
				if err != nil {
					t.Fatal(err)
				}
				service := NewApprovalsService(e.DB())
				approvalID, err := service.Stage(ctx, approvals.StageInput{Kind: kind, TargetType: targetType, TargetID: targetID, ProposedChange: raw, DiffHash: hash})
				if err != nil {
					t.Fatal(err)
				}
				proposal, err := service.GetWire(ctx, approvalID)
				if err != nil {
					t.Fatal(err)
				}
				preview := proposal.Preview
				if preview == nil || preview.State != "ready" || preview.Version != 2 || preview.Details == nil {
					t.Fatalf("relink preview: %+v", preview)
				}
				details := map[string]string{}
				for _, detail := range *preview.Details {
					details[detail.Field] = detail.Value.Text
				}
				if details["relink_destination"] != "Correct buyer" || details["relink_activity"] != "Buyer discussion" || details["relink_removed_link"] != "Original buyer" {
					t.Fatalf("wrong scope: %+v", details)
				}
				if activity.Version == nil {
					t.Fatal("missing version")
				}
				result, err := e.Activities.RelinkActivity(ctx, ids.From[ids.ActivityKind](ids.UUID(activity.Id)), activities.RelinkActivityInput{EntityType: "contact", EntityID: targets[1], ReplaceExistingOfType: true, IfVersion: new(int64(*activity.Version))})
				if err != nil {
					t.Fatal(err)
				}
				if result.Links == nil || len(*result.Links) != 1 || ids.UUID((*result.Links)[0].EntityId) != targets[1] {
					t.Fatalf("writer disagrees with preview: %+v", result.Links)
				}
			})
		}
	}
}
