// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
)

func TestMCPContactCreationResolvesAndReusesEmployer(t *testing.T) {
	e := integration.Setup(t)
	registry := composedRegistry(e)
	for _, payload := range []string{
		`{"record_type":"contact","fields":{"full_name":"Dana Buyer","company_name":"Orion Research","emails":[{"email":"dana@orion-research.test","email_type":"work"}]}}`,
		`{"record_type":"contact","fields":{"full_name":"Robin Buyer","company_name":"ORION  RESEARCH","emails":[{"email":"robin@orion-research.test","email_type":"work"}]}}`,
	} {
		if _, err := registry.Invoke(tierFloorAgentOnPassport(t, e), "create_record", json.RawMessage(payload)); err != nil {
			t.Fatal(err)
		}
	}
	if n := e.WsCount(t, `SELECT count(*) FROM company WHERE display_name = 'Orion Research'`); n != 1 {
		t.Fatalf("companies: %d", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM relationship r JOIN company c ON c.id = r.company_id WHERE c.display_name = 'Orion Research' AND r.kind = 'employment'`); n != 2 {
		t.Fatalf("employments: %d", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'company.created'`); n != 1 {
		t.Fatalf("company creation events: %d", n)
	}
}

func TestContactCreationApprovalShowsEmployerEvidenceAndEffects(t *testing.T) {
	e := integration.Setup(t)
	e.GrantRole(t, e.Rep1, "rep")
	payload := map[string]any{"record_type": "contact", "fields": map[string]any{"full_name": "Dana Buyer", "company_name": "Orion Research"}}
	proposal := crmcontracts.Approval{Status: "pending", Kind: "create_record", ProposedBy: "agent:proposal", OnBehalfOf: new(crmcontracts.Id(e.Rep1)), TargetEntityType: new("contact"), ProposedChange: &payload}
	preview, err := newApprovalPreviewReader(e.DB()).read(e.Admin(), proposal)
	if err != nil {
		t.Fatal(err)
	}
	if preview == nil || preview.State != "ready" || preview.Effects == nil || !slices.Contains(*preview.Effects, "create_contact_employer") {
		t.Fatalf("preview: %+v", preview)
	}
	shown := false
	if preview.Details != nil {
		for _, detail := range *preview.Details {
			if detail.Field == "company_name" && detail.Value.Text == "Orion Research" {
				shown = true
			}
		}
	}
	if !shown {
		t.Fatal("approval omitted employer name")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM company WHERE display_name = 'Orion Research'`); n != 0 {
		t.Fatal("preview created company")
	}
}

func TestContactCreationApprovalBlocksAmbiguousEmployer(t *testing.T) {
	e := integration.Setup(t)
	e.GrantRole(t, e.Rep1, "rep")
	for range 2 {
		if _, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{DisplayName: "Ambiguous Works", Source: "manual"}); err != nil {
			t.Fatal(err)
		}
	}
	payload := map[string]any{"record_type": "contact", "fields": map[string]any{"full_name": "Dana Buyer", "company_name": "Ambiguous Works"}}
	proposal := crmcontracts.Approval{Status: "pending", Kind: "create_record", ProposedBy: "agent:proposal", OnBehalfOf: new(crmcontracts.Id(e.Rep1)), TargetEntityType: new("contact"), ProposedChange: &payload}
	preview, err := newApprovalPreviewReader(e.DB()).read(e.Admin(), proposal)
	if err != nil {
		t.Fatal(err)
	}
	if preview == nil || preview.State == "ready" || preview.Conflicts == nil || len(*preview.Conflicts) == 0 || !(*preview.Conflicts)[0].BlocksApproval {
		t.Fatalf("preview: %+v", preview)
	}
}
