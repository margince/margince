// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"net/url"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// probedLeadFilters is each filter the lead list binds, with the operand the
// probe sends it. A seat reading leads alone shows the refusable ones.
func probedLeadFilters(t *testing.T) []storekit.FilterReach {
	t.Helper()
	reach, err := contacts.LeadListFilterReach(principal.WithActor(t.Context(), principal.Principal{
		Type: principal.PrincipalHuman,
		Permissions: principal.Permissions{
			RowScope: principal.RowScopeTeam,
			Objects:  map[string]principal.ObjectGrant{"lead": {Read: true}},
		},
	}), nil)
	if err != nil {
		t.Fatalf("probing the lead list filters: %v", err)
	}
	if !slices.ContainsFunc(reach, func(probe storekit.FilterReach) bool { return probe.Refused }) {
		t.Fatal("no lead list filter is refusable, so the snapshot would be checked against nothing")
	}
	return reach
}

// refusedLeadFilters is what /me tells the signed-in seat it may not narrow leads by.
func refusedLeadFilters(t *testing.T, e *apptest.AppEnv) []string {
	t.Helper()
	var me struct {
		Authorization struct {
			RefusedLeadFilters *[]string `json:"refused_lead_filters"`
		} `json:"authorization"`
	}
	if status := e.Call(t, "GET", "/v1/me", nil, nil, &me); status != http.StatusOK {
		t.Fatalf("GET /v1/me = %d", status)
	}
	if me.Authorization.RefusedLeadFilters == nil {
		t.Fatal("/me carried no refused_lead_filters, so a client cannot tell which dials the lead list refuses")
	}
	return *me.Authorization.RefusedLeadFilters
}

// The company's facts are the company's. A seat that may not read companies is
// refused every dial over them, and /me names what the list refuses.
func TestMeNamesTheCompanyDialsOfTheLeadListForASeatWithoutCompanyRead(t *testing.T) {
	e := reachApp(t)
	narrowSignedInRole(t, e, "lead_reach_no_company_read", `{"company": {"create": false, "read": false, "update": false, "delete": false}}`)

	refused := refusedLeadFilters(t, e)
	want := []string{"country", "industry", "size_band"}
	if !slices.Equal(refused, want) {
		t.Errorf("/me refused %v, want %v", refused, want)
	}
	assertListSnapshot(t, e, "/leads", probedLeadFilters(t), refused)
}

func TestMeNamesTheListFilterOfTheLeadListForASeatWithoutListRead(t *testing.T) {
	e := reachApp(t)
	narrowSignedInRole(t, e, "lead_reach_no_list_read", `{"list": {"create": false, "read": false, "update": false, "delete": false}}`)

	refused := refusedLeadFilters(t, e)
	assertRefuses(t, refused, "list_id")
	assertListSnapshot(t, e, "/leads", probedLeadFilters(t), refused)
}

func TestMeRefusesNoLeadFilterToTheAdmin(t *testing.T) {
	e := reachApp(t)

	refused := refusedLeadFilters(t, e)
	if len(refused) != 0 {
		t.Errorf("/me refused the admin %v, want none", refused)
	}
	assertListSnapshot(t, e, "/leads", probedLeadFilters(t), refused)
}

// A code no company could hold is refused by the field it names, never
// answered with an empty page that reads as "no such leads".
func TestTheLeadListRefusesACompanyFactNoCompanyHolds(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	for parameter, operand := range map[string]string{"country": "DEU", "industry": "logistics", "size_band": "2-9"} {
		var problem codeRefusal
		status := e.Call(t, "GET", "/v1/leads?"+url.Values{parameter: {operand}}.Encode(), nil, nil, &problem)
		fault := problem.Details.Errors
		if status != http.StatusUnprocessableEntity || len(fault) != 1 || fault[0].Field != parameter {
			t.Errorf("GET /v1/leads?%s=%s = %d %+v, want 422 naming %s", parameter, operand, status, fault, parameter)
		}
	}
}
