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
	anna := captureContact(t, e, AnyMap{
		"full_name": "Anna Example", "title": "Head of Operations",
		"email": "anna@northwind.example", "company_name": "Northwind Traders",
	})

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
		"contact_id": anna, "source": "manual",
	}, nil, &lead); status != http.StatusCreated {
		t.Fatalf("create lead from contact = %d", status)
	}
	// The same contact asked for twice is the lead already held, named in the
	// refusal so the screen that asked can open it.
	var duplicate struct {
		Details struct {
			ExistingID string `json:"existing_id"`
		} `json:"details"`
	}
	if status := e.Call(t, "POST", "/v1/leads", AnyMap{
		"contact_id": anna, "source": "manual",
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

	ben := captureContact(t, e, AnyMap{
		"full_name": "Ben Example", "title": "Head of Operations",
		"email": "ben@northwind.example", "company_name": "Northwind Traders",
	})
	var stated leadWire
	if status := e.Call(t, "POST", "/v1/leads", AnyMap{
		"contact_id": ben, "source": "manual",
		"email": "anna@apac.northwind.example", "company_name": "Northwind APAC",
	}, nil, &stated); status != http.StatusCreated {
		t.Fatalf("create lead from contact with its own address = %d", status)
	}
	stateWins := leadWire{"Ben Example", "anna@apac.northwind.example", "Head of Operations", "Northwind APAC"}
	if stated != stateWins {
		t.Fatalf("what the request states should win and the rest fill: %+v, want %+v", stated, stateWins)
	}

	// A blank name is the contact's to fill, not a refusal.
	carla := captureContact(t, e, AnyMap{"full_name": "Carla Example", "email": "carla@northwind.example"})
	var blank leadWire
	if status := e.Call(t, "POST", "/v1/leads", AnyMap{
		"contact_id": carla, "full_name": "", "source": "manual",
		"email": "carla@third.northwind.example",
	}, nil, &blank); status != http.StatusCreated || blank.FullName != "Carla Example" {
		t.Fatalf("a blank name beside a contact = %d %+v, want the contact's name", status, blank)
	}
	if status := e.Call(t, "POST", "/v1/leads", AnyMap{
		"contact_id": ids.NewV7().String(), "source": "manual",
	}, nil, nil); status != http.StatusUnprocessableEntity {
		t.Fatalf("a lead from a contact nobody holds = %d, want 422", status)
	}
}

// A contact with no address and no profile has no key but itself, so the lead
// worked from it is found by the contact: asked twice it is one lead, the list
// filter finds it, and it frees the contact only while it is closed.
func TestAContactWithoutAnAddressIsWorkedThroughOneLead(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	dana := captureContact(t, e, AnyMap{"full_name": "Dana Example", "company_name": "Contoso Ltd"})

	var first struct {
		ID            string `json:"id"`
		FromContactID string `json:"from_contact_id"`
	}
	if status := e.Call(t, "POST", "/v1/leads", AnyMap{"contact_id": dana, "source": "manual"}, nil, &first); status != http.StatusCreated {
		t.Fatalf("create lead from contact = %d", status)
	}
	if first.FromContactID != dana {
		t.Fatalf("the lead records from_contact_id %q, want the contact %q", first.FromContactID, dana)
	}
	assertContactLeadRefused(t, "a retried create", func() int {
		var refusal problemWithExisting
		status := e.Call(t, "POST", "/v1/leads", AnyMap{"contact_id": dana, "source": "manual"}, nil, &refusal)
		return refusal.check(t, status, first.ID)
	})

	var listed struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/leads?from_contact_id="+dana, nil, nil, &listed); status != http.StatusOK ||
		len(listed.Data) != 1 || listed.Data[0].ID != first.ID {
		t.Fatalf("leads worked from the contact = %d %+v, want only %s", status, listed.Data, first.ID)
	}

	if status := e.Call(t, "DELETE", "/v1/leads/"+first.ID, AnyMap{}, nil, nil); status != http.StatusOK {
		t.Fatalf("disqualify the first lead = %d", status)
	}
	var second struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/leads", AnyMap{"contact_id": dana, "source": "manual"}, nil, &second); status != http.StatusCreated {
		t.Fatalf("a lead from the contact after the first was closed = %d, want 201", status)
	}
	assertContactLeadRefused(t, "reopening the closed lead", func() int {
		var refusal problemWithExisting
		status := e.Call(t, "POST", "/v1/leads/"+first.ID+"/reopen", AnyMap{}, nil, &refusal)
		return refusal.check(t, status, second.ID)
	})

	if status := e.Call(t, "POST", "/v1/leads/"+second.ID+"/promote", AnyMap{"trigger": "human_qualify"}, nil, nil); status != http.StatusOK {
		t.Fatalf("promote the second lead = %d", status)
	}
	var third struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/leads", AnyMap{"contact_id": dana, "source": "manual"}, nil, &third); status != http.StatusCreated {
		t.Fatalf("a lead from the contact after the second was promoted = %d, want 201", status)
	}
	assertContactLeadRefused(t, "demoting the promoted lead", func() int {
		var refusal problemWithExisting
		status := e.Call(t, "POST", "/v1/leads/"+second.ID+"/demote", AnyMap{"reason": "qualified too early"}, nil, &refusal)
		return refusal.check(t, status, third.ID)
	})
}

type problemWithExisting struct {
	Code    string `json:"code"`
	Details struct {
		ExistingID string `json:"existing_id"`
	} `json:"details"`
}

func (p problemWithExisting) check(t *testing.T, status int, want string) int {
	t.Helper()
	if status == http.StatusConflict && (p.Code != "duplicate_contact_lead" || p.Details.ExistingID != want) {
		t.Errorf("the refusal = %+v, want duplicate_contact_lead naming %s", p, want)
	}
	return status
}

func assertContactLeadRefused(t *testing.T, what string, call func() int) {
	t.Helper()
	if status := call(); status != http.StatusConflict {
		t.Fatalf("%s = %d, want 409 naming the live lead", what, status)
	}
}

func captureContact(t *testing.T, e *apptest.AppEnv, body AnyMap) string {
	t.Helper()
	var captured struct {
		Contact struct {
			ID string `json:"id"`
		} `json:"contact"`
	}
	if status := e.Call(t, "POST", "/v1/contacts/quick-capture", body, nil, &captured); status != http.StatusCreated {
		t.Fatalf("capture contact = %d", status)
	}
	return captured.Contact.ID
}
