// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A lead worked from a contact the CRM already knows takes that contact's
// name, address, title and employer for whatever the request left out, and
// keeps whatever it did send.
func TestALeadFromAContactTakesWhatTheContactKnows(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	var captured struct {
		Contact struct {
			ID string `json:"id"`
		} `json:"contact"`
	}
	if status := e.Call(t, "POST", "/v1/contacts/quick-capture", AnyMap{
		"full_name": "Anna Example", "title": "Head of Operations",
		"email": "anna@northwind.example", "company_name": "Northwind Traders",
	}, nil, &captured); status != http.StatusCreated {
		t.Fatalf("capture contact = %d", status)
	}

	type leadWire struct {
		FullName    string `json:"full_name"`
		Email       string `json:"email"`
		Title       string `json:"title"`
		CompanyName string `json:"company_name"`
	}
	var lead leadWire
	var created struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/leads", AnyMap{
		"contact_id": captured.Contact.ID, "source": "manual",
	}, nil, &lead); status != http.StatusCreated {
		t.Fatalf("create lead from contact = %d", status)
	}
	// The same person asked for twice is the lead already held, named in the
	// refusal so the screen that asked can open it.
	var duplicate struct {
		Details struct {
			ExistingID string `json:"existing_id"`
		} `json:"details"`
	}
	if status := e.Call(t, "POST", "/v1/leads", AnyMap{
		"contact_id": captured.Contact.ID, "source": "manual",
	}, nil, &duplicate); status != http.StatusConflict || duplicate.Details.ExistingID == "" {
		t.Fatalf("a second lead from the same contact = %d %+v, want 409 naming the first", status, duplicate)
	}
	if status := e.Call(t, "GET", "/v1/leads/"+duplicate.Details.ExistingID, nil, nil, &created); status != http.StatusOK {
		t.Fatalf("GET the lead the refusal named = %d", status)
	}
	want := leadWire{"Anna Example", "anna@northwind.example", "Head of Operations", "Northwind Traders"}
	if lead != want {
		t.Fatalf("lead from contact = %+v, want %+v", lead, want)
	}

	var stated leadWire
	if status := e.Call(t, "POST", "/v1/leads", AnyMap{
		"contact_id": captured.Contact.ID, "source": "manual",
		"email": "anna@apac.northwind.example", "company_name": "Northwind APAC",
	}, nil, &stated); status != http.StatusCreated {
		t.Fatalf("create lead from contact with its own address = %d", status)
	}
	stateWins := leadWire{"Anna Example", "anna@apac.northwind.example", "Head of Operations", "Northwind APAC"}
	if stated != stateWins {
		t.Fatalf("what the request states should win and the rest fill: %+v, want %+v", stated, stateWins)
	}

	// A blank name is the contact's to fill, not a refusal.
	var blank leadWire
	if status := e.Call(t, "POST", "/v1/leads", AnyMap{
		"contact_id": captured.Contact.ID, "full_name": "", "source": "manual",
		"email": "anna@third.northwind.example",
	}, nil, &blank); status != http.StatusCreated || blank.FullName != "Anna Example" {
		t.Fatalf("a blank name beside a contact = %d %+v, want the contact's name", status, blank)
	}
	if status := e.Call(t, "POST", "/v1/leads", AnyMap{
		"contact_id": ids.NewV7().String(), "source": "manual",
	}, nil, nil); status != http.StatusUnprocessableEntity {
		t.Fatalf("a lead from a contact nobody holds = %d, want 422", status)
	}
}
