// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
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

func TestDisqualificationApprovalDescribesTheActualRetirement(t *testing.T) {
	for _, rest := range []bool{false, true} {
		t.Run(fmt.Sprintf("rest=%t", rest), func(t *testing.T) {
			e := integration.Setup(t)
			perms := integration.AdminPerms
			perms.Objects = maps.Clone(perms.Objects)
			perms.Objects["custom_field"] = principal.ObjectGrant{Read: true}
			ctx := e.As(e.AdminUser, nil, perms)
			lead, _, err := e.Contacts.CreateLead(ctx, contacts.CreateLeadInput{FullName: new("Dana Buyer"), Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			tags := collections.NewStore(e.DB())
			tag, err := tags.CreateTag(ctx, "Qualification review", nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := tags.ApplyTag(ctx, tag.ID, "lead", ids.UUID(lead.Id)); err != nil {
				t.Fatal(err)
			}
			if count := e.WsCount(t, `SELECT count(*) FROM taggable WHERE entity_type = 'lead' AND entity_id = $1`, lead.Id); count != 1 {
				t.Fatalf("tag setup: %d", count)
			}
			payload := fmt.Sprintf(`{"lead_id":%q}`, lead.Id)
			in := contacts.DisqualifyLeadInput{}
			var reasonLabel string
			if rest {
				reasons, err := e.Contacts.ListLeadDisqualifyReasons(ctx)
				if err != nil || len(reasons) == 0 {
					t.Fatalf("reasons: %v, %v", reasons, err)
				}
				in.ReasonID, in.Note = new(ids.UUID(reasons[0].Id)), new("Outside the agreed scope")
				reasonLabel = reasons[0].Label
				payload = fmt.Sprintf(`{"operation":"DisqualifyLead","body":{"reason_id":%q,"note":%q}}`, in.ReasonID, *in.Note)
			}
			raw, hash, err := diffhash.Canonical([]byte(payload))
			if err != nil {
				t.Fatal(err)
			}
			svc := NewApprovalsService(e.DB())
			id, err := svc.Stage(ctx, approvals.StageInput{Kind: "disqualify_lead", TargetType: "lead", TargetID: ids.UUID(lead.Id), ProposedChange: raw, DiffHash: hash})
			if err != nil {
				t.Fatal(err)
			}
			proposal, err := svc.GetWire(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			preview := proposal.Preview
			if preview == nil || preview.State != "ready" {
				t.Fatalf("preview: %+v", preview)
			}
			after := map[string]string{}
			for _, change := range preview.Changes {
				after[change.Field] = change.After.Text
			}
			if after["lead_status"] != "disqualified" {
				t.Fatalf("status: %+v", after)
			}
			if rest && (after["disqualify_reason"] != reasonLabel || after["disqualify_note"] != *in.Note) {
				t.Fatalf("reason: %+v", after)
			}
			if preview.Effects == nil || len(*preview.Effects) != 1 || (*preview.Effects)[0] != "archive_lead_remove_tags" {
				t.Fatalf("effects: %+v", preview.Effects)
			}
			if _, err := svc.Decide(ctx, id, true, nil); err != nil {
				t.Fatal(err)
			}
			retired, err := e.Contacts.DisqualifyLead(ctx, ids.From[ids.LeadKind](ids.UUID(lead.Id)), in, contacts.OnlyAtVersion(new(int64(*lead.Version))))
			if err != nil {
				t.Fatal(err)
			}
			if count := e.WsCount(t, `SELECT count(*) FROM taggable WHERE entity_type = 'lead' AND entity_id = $1`, lead.Id); count != 0 {
				t.Fatalf("retirement kept %d tags", count)
			}
			if string(retired.Status) != after["lead_status"] || retired.ArchivedAt == nil {
				t.Fatalf("retirement: %+v", retired)
			}
		})
	}
}
