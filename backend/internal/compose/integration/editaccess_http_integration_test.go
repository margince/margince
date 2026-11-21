// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A rep asks the owner of a lead for edit access over the real wire. The
// owner's approval in the inbox lets the rep change the lead.

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestARepAsksTheOwnerOfALeadForEditAccessOverTheWire(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	e.DescribeCompany(t)
	owner := apptest.SessionUserID(t, e)
	lead := createdID(t, e, "/v1/leads", AnyMap{"full_name": "Jonas Keller", "owner_id": owner})
	const email, password = "rep@edit-access.test", "edit-access-test-password"
	rep := createdID(t, e, "/v1/users", AnyMap{"email": email, "display_name": "Rep", "role": "rep"})
	apptest.SetPasswordByLink(t, e, rep, password)
	seat := e.AsSecondSeat(t, email, password)
	var before struct {
		Writable bool `json:"writable"`
	}
	if status := seat.Call(t, http.MethodGet, "/v1/leads/"+lead, nil, nil, &before); status != http.StatusOK || before.Writable {
		t.Fatalf("the rep reads the lead = %d, writable %v; want it readable and not theirs to change", status, before.Writable)
	}

	path := "/v1/records/lead/" + lead + "/edit-access-request"
	key := map[string]string{"Idempotency-Key": "6f1d2c3b-4a5e-4f60-8a7b-9c0d1e2f3a4b"}
	var asked crmcontracts.EditAccessRequest
	if status := seat.Call(t, http.MethodPost, path, nil, key, &asked); status != http.StatusCreated {
		t.Fatalf("asking = %d, want 201", status)
	}
	if asked.OwnerId.String() != owner || asked.RecordType != "lead" {
		t.Errorf("the request = %+v, want it put to the lead's owner", asked)
	}
	var open crmcontracts.EditAccessRequest
	if status := seat.Call(t, http.MethodGet, path, nil, nil, &open); status != http.StatusOK || open.ApprovalId != asked.ApprovalId {
		t.Fatalf("reading the open request = %d (%+v), want the request just made", status, open)
	}
	if status := e.Call(t, http.MethodPost, "/v1/approvals/"+asked.ApprovalId.String()+"/approve", AnyMap{}, nil, nil); status != http.StatusOK {
		t.Fatalf("the owner approving = %d, want 200", status)
	}

	var after struct {
		Writable bool   `json:"writable"`
		OwnerID  string `json:"owner_id"`
	}
	if status := seat.Call(t, http.MethodGet, "/v1/leads/"+lead, nil, nil, &after); status != http.StatusOK || !after.Writable {
		t.Errorf("after the approval the rep reads the lead = %d, writable %v; want it theirs to change", status, after.Writable)
	}
	if after.OwnerID != owner {
		t.Errorf("the lead is owned by %s after the approval, want its owner unchanged", after.OwnerID)
	}
	if status := seat.Call(t, http.MethodGet, path, nil, nil, nil); status != http.StatusNotFound {
		t.Errorf("reading the decided request = %d, want 404", status)
	}
}
