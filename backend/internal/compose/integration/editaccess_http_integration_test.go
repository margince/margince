// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A rep asks the owner of a lead for edit access over the real wire. The
// owner's approval in the inbox lets the rep change the lead.

import (
	"context"
	"net/http"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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

// editAccessSeats is an administrator, a rep who asks, and a colleague who
// owns the records asked about.
type editAccessSeats struct {
	admin, asker *apptest.AppEnv
	ownerID      string
}

func newEditAccessSeats(t *testing.T) editAccessSeats {
	t.Helper()
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	e.DescribeCompany(t)
	seat := func(email string) (string, *apptest.AppEnv) {
		const password = "edit-access-test-password"
		id := createdID(t, e, "/v1/users", AnyMap{"email": email, "display_name": email, "role": "rep"})
		apptest.SetPasswordByLink(t, e, id, password)
		return id, e.AsSecondSeat(t, email, password)
	}
	owner, _ := seat("owner@edit-access.test")
	_, asker := seat("asker@edit-access.test")
	return editAccessSeats{admin: e, asker: asker, ownerID: owner}
}

func (s editAccessSeats) ask(t *testing.T, kind, id string) crmcontracts.EditAccessRequest {
	t.Helper()
	var asked crmcontracts.EditAccessRequest
	key := map[string]string{"Idempotency-Key": ids.NewV7().String()}
	if status := s.asker.Call(t, http.MethodPost, "/v1/records/"+kind+"/"+id+"/edit-access-request", nil, key, &asked); status != http.StatusCreated {
		t.Fatalf("asking about the %s = %d, want 201", kind, status)
	}
	return asked
}

func (s editAccessSeats) pending(t *testing.T, approval string) bool {
	t.Helper()
	var pending bool
	if err := apptest.InWorkspace(s.admin, t, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT status = 'pending' FROM approval WHERE id = $1`, ids.MustParse(approval)).Scan(&pending)
	}); err != nil {
		t.Fatalf("reading the request: %v", err)
	}
	return pending
}

// A company that changes hands through its own route closes the request put to
// its former owner.
func TestACompanyThatChangedHandsClosesItsEditAccessRequest(t *testing.T) {
	s := newEditAccessSeats(t)
	company := createdID(t, s.admin, "/v1/companies", AnyMap{"display_name": "Brandt Logistik", "source": "manual", "owner_id": s.ownerID})
	asked := s.ask(t, "company", company)
	var current struct {
		Version int64 `json:"version"`
	}
	s.admin.Call(t, http.MethodGet, "/v1/companies/"+company, nil, nil, &current)
	if status := s.admin.Call(t, http.MethodPatch, "/v1/companies/"+company, AnyMap{"owner_id": apptest.SessionUserID(t, s.admin)},
		map[string]string{"If-Match": strconv.FormatInt(current.Version, 10)}, nil); status != http.StatusOK {
		t.Fatalf("moving the company = %d, want 200", status)
	}
	if s.pending(t, asked.ApprovalId.String()) {
		t.Error("the request put to the company's former owner is still pending")
	}
}

// A member who leaves takes the requests put to them along. Their work is
// handed on, and nobody answers a card on their behalf.
func TestAnOwnerWhoLeftTakesTheirEditAccessRequestsAlong(t *testing.T) {
	s := newEditAccessSeats(t)
	lead := createdID(t, s.admin, "/v1/leads", AnyMap{"full_name": "Jonas Keller", "owner_id": s.ownerID})
	asked := s.ask(t, "lead", lead)
	if status := s.admin.Call(t, http.MethodPost, "/v1/users/"+s.ownerID+"/deactivate", AnyMap{}, nil, nil); status != http.StatusOK {
		t.Fatalf("deactivating the owner = %d, want 200", status)
	}
	if status := s.admin.Call(t, http.MethodPost, "/v1/users/"+s.ownerID+"/handover", AnyMap{"to_user_id": apptest.SessionUserID(t, s.admin)},
		map[string]string{"Idempotency-Key": ids.NewV7().String()}, nil); status != http.StatusOK {
		t.Fatalf("handing over the owner's work = %d, want 200", status)
	}
	if s.pending(t, asked.ApprovalId.String()) {
		t.Error("the request put to the member who left is still pending")
	}
}

// Once the owner makes the company private, the asker cannot read back the
// requests they made before. That holds for the handover and edit access.
func TestARequestOnACompanyTheAskerLostSightOfReadsAsNotFound(t *testing.T) {
	s := newEditAccessSeats(t)
	company := createdID(t, s.admin, "/v1/companies", AnyMap{"display_name": "Brandt Logistik", "source": "manual", "owner_id": s.ownerID})
	s.ask(t, "company", company)
	if status := s.asker.Call(t, http.MethodPost, "/v1/records/company/"+company+"/ownership-request", AnyMap{},
		map[string]string{"Idempotency-Key": ids.NewV7().String()}, nil); status != http.StatusCreated {
		t.Fatalf("asking for the company = %d, want 201", status)
	}
	var current struct {
		Version int64 `json:"version"`
	}
	s.admin.Call(t, http.MethodGet, "/v1/companies/"+company, nil, nil, &current)
	if status := s.admin.Call(t, http.MethodPatch, "/v1/companies/"+company, AnyMap{"visibility": "owner"},
		map[string]string{"If-Match": strconv.FormatInt(current.Version, 10)}, nil); status != http.StatusOK {
		t.Fatalf("making the company private = %d, want 200", status)
	}
	for _, route := range []string{"edit-access-request", "ownership-request"} {
		if status := s.asker.Call(t, http.MethodGet, "/v1/records/company/"+company+"/"+route, nil, nil, nil); status != http.StatusNotFound {
			t.Errorf("reading the %s on a company out of sight = %d, want 404", route, status)
		}
	}
}
