// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// captured_by on a contact an agent wrote names only the passport. The page
// names the colleague the agent worked for from the create audit row, and it
// must say what the record History says about that same row.

import (
	"encoding/json"
	"net/http"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTheContactPageNamesTheColleagueAnAgentCreatedItFor(t *testing.T) {
	e := Setup(t)
	agent := e.AgentFor(t, e.Rep1, []ids.UUID{e.Team1}, roomPerms)
	created, err := e.Contacts.CreateContact(agent, contacts.CreateContactInput{FullName: "Met At An Event", Source: "manual"})
	if err != nil {
		t.Fatalf("the agent's create: %v", err)
	}
	contact := ids.UUID(created.Id)

	page := readContact360(t, e, contact)
	if page.CreatedBy == nil {
		t.Fatal("created_by is absent for a contact whose create row exists")
	}
	var create *crmcontracts.PrincipalAttribution
	for _, entry := range readRecordHistory(t, e, contact).Entries {
		if entry.Action == "create" {
			create = &crmcontracts.PrincipalAttribution{ActorType: crmcontracts.PrincipalAttributionActorType(entry.ActorType), OnBehalfOfName: entry.OnBehalfOfName}
		}
	}
	if create == nil || create.OnBehalfOfName == nil || *create.OnBehalfOfName == "" {
		t.Fatalf("the record history names no colleague behind the create: %+v", create)
	}
	if page.CreatedBy.ActorType != crmcontracts.PrincipalAttributionActorType("agent") {
		t.Errorf("created_by.actor_type = %q, want agent", page.CreatedBy.ActorType)
	}
	if page.CreatedBy.OnBehalfOfName == nil || *page.CreatedBy.OnBehalfOfName != *create.OnBehalfOfName {
		t.Errorf("created_by names %v, the record history names %q", page.CreatedBy.OnBehalfOfName, *create.OnBehalfOfName)
	}
}

func readContact360(t *testing.T, e *Env, contact ids.UUID) crmcontracts.Contact360 {
	t.Helper()
	h := contactHandlers(e)
	code, body := call(e.As(e.Rep1, []ids.UUID{e.Team1}, roomPerms), http.MethodGet, "/v1/contacts/"+contact.String()+"/360",
		func(w http.ResponseWriter, r *http.Request) {
			h.GetContact360(w, r, crmcontracts.Id(contact), crmcontracts.GetContact360Params{})
		})
	if code != http.StatusOK {
		t.Fatalf("360 → %d, want 200: %s", code, body)
	}
	var page crmcontracts.Contact360
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatalf("the composite did not parse: %v", err)
	}
	return page
}
