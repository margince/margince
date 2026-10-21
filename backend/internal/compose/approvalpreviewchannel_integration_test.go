// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestChannelApprovalUsesTheConversationRecipient(t *testing.T) {
	for _, rest := range []bool{false, true} {
		t.Run(fmt.Sprintf("rest=%t", rest), func(t *testing.T) {
			e := integration.Setup(t)
			contact := e.SeedContact(t, "Dana Buyer", &e.AdminUser)
			ctx := e.Admin()
			err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
				bound, err := contacts.ResolveOrCreateChannelIdentity(ctx, tx, ids.From[ids.ContactKind](contact), connector.ChannelIdentity{Provider: "telegram", ChannelUserID: "12345", Username: "dana_buyer"})
				if err == nil && bound.UUID != contact {
					t.Fatalf("identity bound to %v", bound)
				}
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			anchor, _, err := e.Activities.LogActivity(ctx, activities.LogActivityInput{Kind: activities.KindMessage, ChannelProvider: "telegram", Body: new("Please send the plan."), Direction: new("inbound"), Source: "manual", Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}}})
			if err != nil {
				t.Fatal(err)
			}
			payload := fmt.Sprintf(`{"activity_id":%q,"body":"Here is the complete delivery plan."}`, anchor.Id)
			if rest {
				payload = `{"operation":"sendMessage","body":{"body":"Here is the complete delivery plan."}}`
			}
			raw, hash, err := diffhash.Canonical([]byte(payload))
			if err != nil {
				t.Fatal(err)
			}
			svc := NewApprovalsService(e.DB())
			id, err := svc.Stage(ctx, approvals.StageInput{Kind: "send_message", TargetType: "activity", TargetID: ids.UUID(anchor.Id), ProposedChange: raw, DiffHash: hash})
			if err != nil {
				t.Fatal(err)
			}
			proposal, err := svc.GetWire(ctx, id)
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
			if details["channel_provider"] != "telegram" || details["channel_account"] != "12345" || details["channel_username"] != "dana_buyer" || details["filed_under"] != "Dana Buyer" || details["body"] != "Here is the complete delivery plan." {
				t.Fatalf("channel details: %+v", details)
			}
			if count := e.WsCount(t, `SELECT count(*) FROM activity WHERE direction = 'outbound'`); count != 0 {
				t.Fatalf("preview sent %d replies", count)
			}
		})
	}
}
