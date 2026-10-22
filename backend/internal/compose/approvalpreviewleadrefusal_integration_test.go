// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestLeadDisqualificationRefusesUnavailableContext(t *testing.T) {
	for _, changeLead := range []bool{false, true} {
		t.Run(fmt.Sprintf("changed-lead=%t", changeLead), func(t *testing.T) {
			e := integration.Setup(t)
			lead, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{FullName: new("Dana Buyer"), Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			payload := fmt.Sprintf(`{"reason_id":%q}`, ids.NewV7())
			expectedState := "incomplete"
			if changeLead {
				payload, expectedState = `{}`, "stale"
			}
			raw, hash, err := diffhash.Canonical([]byte(payload))
			if err != nil {
				t.Fatal(err)
			}
			svc := NewApprovalsService(e.DB())
			id, err := svc.Stage(e.Admin(), approvals.StageInput{Kind: "disqualify_lead", TargetType: "lead", TargetID: ids.UUID(lead.Id), ProposedChange: raw, DiffHash: hash})
			if err != nil {
				t.Fatal(err)
			}
			leadID := ids.From[ids.LeadKind](ids.UUID(lead.Id))
			if changeLead {
				if _, err := e.Contacts.UpdateLead(e.Admin(), leadID, contacts.UpdateLeadInput{FullName: new("Dana Qualified")}); err != nil {
					t.Fatal(err)
				}
			}
			proposal, err := svc.GetWire(e.Admin(), id)
			if err != nil {
				t.Fatal(err)
			}
			if proposal.Preview == nil || proposal.Preview.State != expectedState || len(proposal.Preview.Changes) != 0 || proposal.Preview.Effects != nil {
				t.Fatalf("refused preview: %+v", proposal.Preview)
			}
			_, err = svc.Decide(e.Admin(), id, true, nil)
			var unavailable *approvals.PreviewContextError
			if changeLead && !errors.Is(err, apperrors.ErrVersionSkew) {
				t.Fatalf("changed lead decision: %v", err)
			}
			if !changeLead && !errors.As(err, &unavailable) {
				t.Fatalf("missing reason decision: %v", err)
			}
			current, err := e.Contacts.GetLead(e.Admin(), leadID, storekit.LiveOnly)
			if err != nil || current.ArchivedAt != nil {
				t.Fatalf("refusal changed the lead: %+v, %v", current, err)
			}
			proposal, err = svc.GetWire(e.Admin(), id)
			if err != nil || proposal.Status != "pending" {
				t.Fatalf("refusal recorded a decision: %+v, %v", proposal, err)
			}
		})
	}
}
