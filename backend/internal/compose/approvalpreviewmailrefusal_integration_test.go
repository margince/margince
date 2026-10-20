// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"maps"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestMailApprovalDoesNotPresentAnIncompleteMessageAsReady(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change map[string]any
	}{
		{"no visible addressee", map[string]any{"cc": []string{"buyer@example.test"}}},
		{"blank body", map[string]any{"body": " \n "}},
		{"missing filing scope", map[string]any{"links": []any{}}},
		{"unknown communication context", map[string]any{"communication_context": "not-a-category"}},
		{"unavailable attachment", map[string]any{"attachment_ids": []string{ids.NewV7().String()}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := integration.Setup(t)
			company := e.SeedCompany(t, "Harbor account", &e.AdminUser)
			payload := map[string]any{
				"to": []string{"buyer@example.test"}, "subject": "Delivery terms", "body": "Review the proposed delivery terms.",
				"links": []map[string]any{{"entity_type": "company", "entity_id": company}},
			}
			maps.Copy(payload, tc.change)
			raw, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			canonical, hash, err := diffhash.Canonical(raw)
			if err != nil {
				t.Fatal(err)
			}
			svc := NewApprovalsService(e.DB())
			id, err := svc.Stage(e.Admin(), approvals.StageInput{
				Kind: "send_company_email", TargetType: "company", TargetID: company,
				ProposedChange: canonical, DiffHash: hash,
			})
			if err != nil {
				t.Fatal(err)
			}
			proposal, err := svc.GetWire(e.Admin(), id)
			if err != nil {
				t.Fatal(err)
			}
			preview := proposal.Preview
			if preview == nil || preview.State != previewIncompleteState || preview.Details != nil || preview.Files != nil {
				t.Fatalf("partial message was presented for review: %+v", preview)
			}
			if err := approvals.PreviewRefusal(preview); err == nil {
				t.Fatal("incomplete message permits approval")
			}
		})
	}
}
