// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Who the installation's own profile belongs to.
//
// It used to ride the `company` object — the one governing CUSTOMER records —
// so every role that may edit an account could edit the installation's
// identity through the API. By the seeded matrix that is admin, ops, manager
// AND rep. Nobody granted that; it is what a surface inherits when it borrows
// an object broader than itself.
//
// Both halves are asserted, because leaving the read on the object would have
// one surface answering two different questions about who the profile belongs
// to — and because the read is the half a reviewer is most likely to forget.

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

func TestTheInstallationProfileIsAnAdministratorsToReadAndWrite(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	// The admin first, so the refusals below are about the ROLE rather than
	// about a surface that refuses everybody.
	body := wellFormedCompany()
	if status := e.Call(t, http.MethodPut, "/v1/company", body, nil, nil); status != http.StatusOK {
		t.Fatalf("PUT /v1/company as an admin = %d, want 200", status)
	}
	if status := e.Call(t, http.MethodGet, "/v1/company", nil, nil, nil); status != http.StatusOK {
		t.Fatalf("GET /v1/company as an admin = %d, want 200", status)
	}

	demoteToRep(t, e)

	// A rep holds `company` create and update by the seeded matrix, so this is
	// exactly the grant that used to admit them.
	if status := e.Call(t, http.MethodPut, "/v1/company", wellFormedCompany(), nil, nil); status != http.StatusForbidden {
		t.Errorf("PUT /v1/company as a rep = %d, want 403 — a seat that may edit a customer account "+
			"must not be able to rename the installation", status)
	}
	if status := e.Call(t, http.MethodGet, "/v1/company", nil, nil, nil); status != http.StatusForbidden {
		t.Errorf("GET /v1/company as a rep = %d, want 403", status)
	}
}

// And the customer-record routes are untouched: the change narrows one surface,
// not the object it used to borrow.
func TestARepStillReadsAndWritesCustomerAccounts(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	demoteToRep(t, e)

	var created struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, http.MethodPost, "/v1/companies",
		AnyMap{"display_name": "A Customer Of Ours"}, nil, &created); status != http.StatusCreated {
		t.Fatalf("POST /v1/companies as a rep = %d, want 201 — the object still governs customer records", status)
	}
	if status := e.Call(t, http.MethodGet, "/v1/companies/"+created.ID, nil, nil, nil); status != http.StatusOK {
		t.Errorf("GET /v1/companies/{id} as a rep = %d, want 200", status)
	}
}

// The growth fit is the reason the standing read is not the administered one.
//
// It is a reading aid ANY human seat opens on any account, and it caps the band
// it reports when this installation has not described what it offers. A seat
// that could not resolve that would be refused a page they may read — so the
// gate change had to leave this answerable, and this is the case that says so.
func TestARepStillReadsAGrowthFitAfterTheProfileBecameAdministered(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	if status := e.Call(t, http.MethodPut, "/v1/company",
		wellFormedCompany(), nil, nil); status != http.StatusOK {
		t.Fatalf("seeding the installation profile: %d", status)
	}
	var customer struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, http.MethodPost, "/v1/companies",
		AnyMap{"display_name": "A Customer Of Ours"}, nil, &customer); status != http.StatusCreated {
		t.Fatalf("create the customer: %d", status)
	}

	demoteToRep(t, e)

	status := e.Call(t, http.MethodGet, "/v1/companies/"+customer.ID+"/growth-fit", nil, nil, nil)
	if status == http.StatusForbidden {
		t.Fatal("a rep was refused a growth fit — the profile gate reached a surface that is not administered")
	}
}
