// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The record fields that took any size hold the bound the contract publishes,
// on create and on edit.

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

func TestRecordWritesHoldTheContractsSizeBounds(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	contactID := createdID(t, e, "/v1/contacts", AnyMap{"full_name": "Bounded", "source": "manual", "linkedin_url": apptest.ContactProfile()})
	companyID := createdID(t, e, "/v1/companies", AnyMap{"display_name": "Bounded Co", "source": "manual"})
	leadID := createdID(t, e, "/v1/leads", AnyMap{"full_name": "Lee Bounded", "email": "lee@bounds.test"})

	longName := strings.Repeat("n", 121)
	longAddress := strings.Repeat("a", 64) + "@" + strings.Repeat("b", 186) + ".com"
	listOf := func(n int, row func(int) AnyMap) []AnyMap {
		rows := make([]AnyMap, n)
		for i := range rows {
			rows[i] = row(i)
		}
		return rows
	}
	emails := listOf(26, func(i int) AnyMap { return AnyMap{"email": fmt.Sprintf("contact%d@bounds.test", i)} })
	phones := listOf(26, func(i int) AnyMap { return AnyMap{"phone": fmt.Sprintf("+4930123%04d", i)} })
	domains := listOf(101, func(i int) AnyMap { return AnyMap{"domain": fmt.Sprintf("bounds%d.test", i)} })

	for _, c := range []struct {
		name, method, path string
		body               AnyMap
		field, code        string
	}{
		{"a lead name on create", http.MethodPost, "/v1/leads", AnyMap{"full_name": longName}, "full_name", "too_long"},
		{"a lead name on edit", http.MethodPatch, "/v1/leads/" + leadID, AnyMap{"full_name": longName}, "full_name", "too_long"},
		{"a lead address", http.MethodPost, "/v1/leads", AnyMap{"email": longAddress}, "email", "too_long"},
		{"a contact source", http.MethodPost, "/v1/contacts", AnyMap{"full_name": "S", "source": strings.Repeat("s", 129), "linkedin_url": apptest.ContactProfile()}, "source", "too_long"},
		{"contact addresses on create", http.MethodPost, "/v1/contacts", AnyMap{"full_name": "E", "source": "manual", "emails": emails}, "emails", "too_many"},
		{"contact addresses on edit", http.MethodPatch, "/v1/contacts/" + contactID, AnyMap{"emails": emails}, "emails", "too_many"},
		{"contact numbers on edit", http.MethodPatch, "/v1/contacts/" + contactID, AnyMap{"phones": phones}, "phones", "too_many"},
		{"company domains on create", http.MethodPost, "/v1/companies", AnyMap{"display_name": "D", "source": "manual", "domains": domains}, "domains", "too_many"},
		{"company domains on edit", http.MethodPatch, "/v1/companies/" + companyID, AnyMap{"domains": domains}, "domains", "too_many"},
		{"a saved view query", http.MethodPost, "/v1/views", AnyMap{"resource": "contacts", "name": "Big", "query": AnyMap{"blob": strings.Repeat("x", 70000)}}, "query", "too_large"},
	} {
		var faults fieldFaults
		if status := e.Call(t, c.method, c.path, c.body, nil, &faults); status != http.StatusUnprocessableEntity {
			t.Errorf("%s → %d, want 422", c.name, status)
			continue
		}
		if field, code := faults.only(); field != c.field || code != c.code {
			t.Errorf("%s fault = %q %q, want %q %q", c.name, field, code, c.field, c.code)
		}
	}

	var lead AnyMap
	if status := e.Call(t, http.MethodPatch, "/v1/leads/"+leadID, AnyMap{"full_name": strings.Repeat("n", 120)}, nil, &lead); status != http.StatusOK {
		t.Errorf("a lead name at the limit → %d, want 200", status)
	}
}
