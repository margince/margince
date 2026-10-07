// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// A seller who remembers the company and not the person types the start of
// the company's name, and the lead list finds the person who works there.
// A whole word already matched through search_tsv; a fragment needs the
// substring arm to read the company as well as the name.
func TestTheLeadListFindsALeadByAFragmentOfItsCompany(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	northwind := createQuickFindLead(t, e, "Anna Example", "Northwind Traders")
	contoso := createQuickFindLead(t, e, "Ben Sample", "Contoso Ltd")

	found := listLeadIDs(t, e, "northw")
	if !found[northwind] || found[contoso] {
		t.Fatalf("q=northw returned %v, want only the Northwind lead %s", found, northwind)
	}
	// The name arm still answers on its own.
	if found := listLeadIDs(t, e, "sampl"); !found[contoso] || found[northwind] {
		t.Fatalf("q=sampl returned %v, want only the Contoso lead %s", found, contoso)
	}
}

func createQuickFindLead(t *testing.T, e *apptest.AppEnv, name, company string) string {
	t.Helper()
	var lead struct {
		ID string `json:"id"`
	}
	status := e.Call(t, "POST", "/v1/leads", AnyMap{
		"full_name": name, "company_name": company, "source": "manual",
	}, nil, &lead)
	if status != http.StatusCreated {
		t.Fatalf("create lead %q = %d", name, status)
	}
	return lead.ID
}

func listLeadIDs(t *testing.T, e *apptest.AppEnv, q string) map[string]bool {
	t.Helper()
	var page struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/leads?q="+url.QueryEscape(q), nil, nil, &page); status != http.StatusOK {
		t.Fatalf("GET /v1/leads?q=%s = %d", q, status)
	}
	ids := make(map[string]bool, len(page.Data))
	for _, l := range page.Data {
		ids[l.ID] = true
	}
	return ids
}
