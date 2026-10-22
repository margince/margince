// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
)

// An agent's proposed contact on a sending system's address cannot be created.
// The approver is told why before deciding, not shown a bare preview.
func TestAContactCreationOnASendingSystemsAddressShowsABlockingConflict(t *testing.T) {
	e := integration.Setup(t)
	e.GrantRole(t, e.AdminUser, "admin")
	ctx := e.As(e.AdminUser, nil, integration.AdminPerms)
	encoded, err := json.Marshal(map[string]any{"record_type": "contact", "fields": map[string]any{
		"full_name": "Signing-Noreply", "source": "manual",
		"emails": []map[string]any{{"email": "signing-noreply@preview-machine.test", "is_primary": true}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	raw, hash, err := diffhash.Canonical(encoded)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewApprovalsService(e.DB())
	approvalID, err := svc.Stage(ctx, approvals.StageInput{Kind: "create_record", TargetType: "contact", ProposedChange: raw, DiffHash: hash})
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := svc.GetWire(ctx, approvalID)
	if err != nil {
		t.Fatal(err)
	}
	preview := proposal.Preview
	if preview == nil || preview.Conflicts == nil || len(*preview.Conflicts) != 1 {
		t.Fatalf("preview = %+v, want one conflict", preview)
	}
	conflict := (*preview.Conflicts)[0]
	if conflict.Kind != contacts.MachineAddressCode || !conflict.BlocksApproval || conflict.Summary == "" {
		t.Errorf("conflict = %+v, want a blocking %s with a reason", conflict, contacts.MachineAddressCode)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM contact_email WHERE email = 'signing-noreply@preview-machine.test'`); n != 0 {
		t.Errorf("the preview stored the address %d times", n)
	}
}
