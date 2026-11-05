// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTheWriteAPIRefusesAnUnknownEmploymentStatus(t *testing.T) {
	e := setupRelationships(t)
	var edge struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/relationships", AnyMap{"kind": "employment", "contact_id": e.contactID, "company_id": e.companyID, "source": "manual"}, nil, &edge); status != http.StatusCreated {
		t.Fatal(status)
	}
	var minted struct {
		Token string `json:"token"`
	}
	if status := e.Call(t, "POST", "/v1/passports", AnyMap{"label": "relationship writer", "scopes": []string{"read", "write"}}, nil, &minted); status != http.StatusCreated {
		t.Fatal(status)
	}
	for _, headers := range []map[string]string{nil, {"Authorization": "Bearer " + minted.Token}} {
		for _, request := range []struct {
			method, path string
			body         AnyMap
		}{
			{"POST", "/v1/relationships", AnyMap{"kind": "employment", "contact_id": e.contactID, "company_id": e.companyID, "source": "manual", "employment_status": "unknown"}},
			{"PATCH", "/v1/relationships/" + edge.ID, AnyMap{"employment_status": "unknown"}},
			{"POST", "/v1/contacts/" + e.contactID + "/employment-import", AnyMap{"employment_status": "unknown"}},
		} {
			var problem json.RawMessage
			status := e.Call(t, request.method, request.path, request.body, headers, &problem)
			if status != http.StatusUnprocessableEntity || !strings.Contains(string(problem), "employment_status") {
				t.Fatalf("%s: %d %s", request.method, status, problem)
			}
		}
	}
	var company struct {
		ContactCount *int `json:"contact_count"`
	}
	if status := e.Call(t, "GET", "/v1/companies/"+e.companyID, nil, nil, &company); status != http.StatusOK || company.ContactCount == nil || *company.ContactCount != 1 {
		t.Fatalf("omitted status count: %d %+v", status, company)
	}
	if got := employmentReaderIDs(t, e, "/v1/companies/"+e.companyID+"/contacts?"); !slices.Equal(got, []string{e.contactID}) {
		t.Fatalf("omitted status roster: %v", got)
	}
}

func employmentReaderIDs(t *testing.T, e *relEnv, path string) []string {
	t.Helper()
	var ids []string
	cursor := ""
	for {
		var page struct {
			Data []struct {
				ID        string `json:"id"`
				ContactID string `json:"contact_id"`
			} `json:"data"`
			Page struct {
				HasMore    bool    `json:"has_more"`
				NextCursor *string `json:"next_cursor"`
			} `json:"page"`
		}
		query := path + "&limit=1"
		if cursor != "" {
			query += "&cursor=" + url.QueryEscape(cursor)
		}
		if status := e.Call(t, "GET", query, nil, nil, &page); status != http.StatusOK {
			t.Fatal(status)
		}
		for _, row := range page.Data {
			if row.ContactID != "" {
				ids = append(ids, row.ContactID)
			} else {
				ids = append(ids, row.ID)
			}
		}
		if !page.Page.HasMore {
			break
		}
		if page.Page.NextCursor == nil || *page.Page.NextCursor == cursor {
			t.Fatal("roster cursor did not advance")
		}
		cursor = *page.Page.NextCursor
	}
	slices.Sort(ids)
	return ids
}

func TestASecondaryCurrentJobAgreesAcrossCompanyReaders(t *testing.T) {
	e := setupRelationships(t)
	var other struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/companies", AnyMap{"display_name": "Primary employer", "source": "manual"}, nil, &other); status != http.StatusCreated {
		t.Fatal(status)
	}
	for _, company := range []string{other.ID, e.companyID} {
		if status := e.Call(t, "POST", "/v1/relationships", AnyMap{"kind": "employment", "contact_id": e.contactID, "company_id": company, "source": "manual"}, nil, nil); status != http.StatusCreated {
			t.Fatal(status)
		}
	}
	var coworker struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/contacts", AnyMap{"full_name": "Second colleague", "source": "manual"}, nil, &coworker); status != http.StatusCreated {
		t.Fatal(status)
	}
	if status := e.Call(t, "POST", "/v1/relationships", AnyMap{"kind": "employment", "contact_id": coworker.ID, "company_id": e.companyID, "source": "manual"}, nil, nil); status != http.StatusCreated {
		t.Fatal(status)
	}
	if status := e.Call(t, "POST", "/v1/relationships", AnyMap{"kind": "employment", "contact_id": e.contactID, "company_id": e.companyID, "source": "manual", "employment_status": "current", "ended_at": "9998-01-01"}, nil, nil); status != http.StatusCreated {
		t.Fatal(status)
	}
	want := []string{e.contactID, coworker.ID}
	slices.Sort(want)
	tab := employmentReaderIDs(t, e, "/v1/companies/"+e.companyID+"/contacts?")
	filtered := employmentReaderIDs(t, e, "/v1/contacts?company_id="+e.companyID)
	if !slices.Equal(tab, want) || !slices.Equal(filtered, tab) {
		t.Fatalf("tab %v differs from filter %v", tab, filtered)
	}
	filteredTree := previewIDs(t, e.AppEnv, "contact", leaf("company_id", "eq", e.companyID))
	if !slices.Equal(filteredTree, tab) {
		t.Fatalf("filter builder %v differs from roster %v", filteredTree, tab)
	}
	var graph struct {
		Nodes []struct {
			ID   string `json:"id"`
			Kind string `json:"kind"`
		} `json:"nodes"`
	}
	if status := e.Call(t, "GET", "/v1/companies/"+e.companyID+"/graph", nil, nil, &graph); status != http.StatusOK {
		t.Fatal(status)
	}
	graphContacts := []string{}
	for _, node := range graph.Nodes {
		if node.Kind == "contact" {
			graphContacts = append(graphContacts, node.ID)
		}
	}
	slices.Sort(graphContacts)
	if !slices.Equal(graphContacts, tab) {
		t.Fatalf("graph %v differs from roster %v", graphContacts, tab)
	}
	var company struct {
		ContactCount *int `json:"contact_count"`
	}
	if status := e.Call(t, "GET", "/v1/companies/"+e.companyID, nil, nil, &company); status != http.StatusOK || company.ContactCount == nil || *company.ContactCount != len(tab) {
		t.Fatalf("count differs from roster: %d %+v", status, company)
	}
}

func TestTheToolSurfaceRefusesAnUnknownEmploymentStatus(t *testing.T) {
	e := Setup(t)
	e.GrantRole(t, e.Rep1, "admin")
	registry := compose.NewRegistry(e.Pool, compose.SendPath{})
	human := e.As(e.Rep1, []ids.UUID{e.Team1}, AdminPerms)
	contact, company := seedEndpointPair(human, t, e, "Colleague", "Employer")
	edge, _ := createEmployment(human, t, registry, contact, company, "")
	passport := e.SeedPassport(t, OwnerConn(t), "Employment writer")
	agent := e.AgentOnPassport(t, e.Rep1, []ids.UUID{e.Team1}, AdminPerms, ids.From[ids.PassportKind](passport))
	for _, request := range []struct {
		tool string
		args string
	}{
		{"create_record", fmt.Sprintf(`{"record_type":"relationship","fields":{"kind":"employment","contact_id":%q,"company_id":%q,"source":"manual","employment_status":"unknown"}}`, contact, company)},
		{"update_record", fmt.Sprintf(`{"record_type":"relationship","id":%q,"fields":{"employment_status":"unknown"}}`, edge)},
	} {
		_, err := registry.Invoke(agent, request.tool, json.RawMessage(request.args))
		if err == nil || !strings.Contains(err.Error(), "employment_status") {
			t.Fatalf("%s did not name the refused field: %v", request.tool, err)
		}
	}
}
