// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"fmt"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestMailApprovalSignsOnlyAsItsActualHumanSender(t *testing.T) {
	for _, mode := range []string{"own", "other_reviewer", "agent"} {
		t.Run(mode, func(t *testing.T) {
			e := integration.Setup(t)
			ctx := e.Admin()
			company := e.SeedCompany(t, "Harbor account", &e.AdminUser)
			const signature = "Dana Seller\nAccount Director"
			if _, err := e.Contacts.SaveMyEmailSignature(ctx, contacts.SaveSignatureInput{Body: signature}); err != nil {
				t.Fatal(err)
			}
			proposer := ctx
			if mode == "agent" {
				// A live passport granted by the reviewer, since the preview
				// is built as the agent that spends it.
				passport := ids.NewV7()
				e.WsExec(t, `INSERT INTO passport (id, on_behalf_of, granted_by, label, scopes, token_hash, expires_at)
				             VALUES ($1, $2, $2, 'signature', ARRAY['read','write']::text[], $3, now() + interval '1 day')`,
					passport, e.AdminUser, "hash-"+passport.String())
				e.GrantRole(t, e.AdminUser, "admin")
				agent, _ := principal.Actor(e.AgentCtxWithPassport(passport))
				agent.OnBehalfOf = e.AdminUser
				proposer = principal.WithActor(e.AgentCtx(), agent)
			}
			raw, hash, err := diffhash.Canonical([]byte(fmt.Sprintf(`{"to":["buyer@example.test"],"body":"Delivery Friday.","links":[{"entity_type":"company","entity_id":%q}]}`, company)))
			if err != nil {
				t.Fatal(err)
			}
			svc := NewApprovalsService(e.DB())
			id, err := svc.Stage(proposer, approvals.StageInput{Kind: "send_company_email", TargetType: "activity", ProposedChange: raw, DiffHash: hash})
			if err != nil {
				t.Fatal(err)
			}
			if mode == "other_reviewer" {
				ctx = e.As(e.Rep1, nil, integration.AdminPerms)
			}
			proposal, err := svc.GetWire(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			preview := proposal.Preview
			if mode == "other_reviewer" {
				if preview == nil || preview.State != "incomplete" || preview.Details != nil || preview.Conflicts == nil || len(*preview.Conflicts) != 1 || (*preview.Conflicts)[0].Kind != "sender_signature_private" || !(*preview.Conflicts)[0].BlocksApproval {
					t.Fatalf("private signature became reviewable: %+v", preview)
				}
				return
			}
			if preview == nil || preview.State != "ready" || preview.Details == nil || preview.Effects == nil {
				t.Fatalf("preview: %+v", preview)
			}
			signed := ""
			for _, detail := range *preview.Details {
				if detail.Field == "signature_text" {
					signed = detail.Value.Text
				}
			}
			if mode == "own" && signed != signature {
				t.Fatalf("own signature: %q", signed)
			}
			if mode == "agent" && (signed != "" || !slices.Contains(*preview.Effects, "send_without_human_signature")) {
				t.Fatalf("agent signed as reviewer: %q %+v", signed, *preview.Effects)
			}
		})
	}
}
