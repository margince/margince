// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// A lead takes a tag the way a deal does: the record shows it, the word
// counts it, and the lead list narrows to it — on the default work queue and
// under an explicit sort, which are two different reads of the same list.
func TestALeadCarriesATagAndTheListNarrowsToIt(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	tagged := createTaggableLead(t, e, "Anna Example", "Northwind Traders")
	untagged := createTaggableLead(t, e, "Ben Sample", "Contoso Ltd")

	var tag struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/tags", AnyMap{"name": "Product A"}, nil, &tag); status != http.StatusCreated {
		t.Fatalf("create tag = %d", status)
	}
	if status := e.Call(t, "POST", "/v1/tags/"+tag.ID+"/apply", AnyMap{
		"entity_type": "lead", "entity_id": tagged,
	}, nil, nil); status != http.StatusCreated {
		t.Fatalf("apply tag to lead = %d", status)
	}

	var onRecord struct {
		Data []struct {
			TagID string `json:"tag_id"`
		} `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/records/lead/"+tagged+"/tags", nil, nil, &onRecord); status != http.StatusOK {
		t.Fatalf("GET the lead's tags = %d", status)
	}
	if len(onRecord.Data) != 1 || onRecord.Data[0].TagID != tag.ID {
		t.Fatalf("the lead's tags = %+v, want the one applied", onRecord.Data)
	}

	var word struct {
		Usage struct {
			Leads int `json:"leads"`
		} `json:"usage"`
	}
	if status := e.Call(t, "GET", "/v1/tags/"+tag.ID, nil, nil, &word); status != http.StatusOK {
		t.Fatalf("GET the tag = %d", status)
	}
	if word.Usage.Leads != 1 {
		t.Fatalf("usage.leads = %d, want 1", word.Usage.Leads)
	}

	for _, sort := range []string{"", "&sort=-score"} {
		var page struct {
			Data []struct {
				ID   string `json:"id"`
				Tags []struct {
					TagID string `json:"tag_id"`
				} `json:"tags"`
			} `json:"data"`
		}
		if status := e.Call(t, "GET", "/v1/leads?tag_id="+tag.ID+sort, nil, nil, &page); status != http.StatusOK {
			t.Fatalf("GET /v1/leads?tag_id%s = %d", sort, status)
		}
		if len(page.Data) != 1 || page.Data[0].ID != tagged {
			t.Fatalf("tag_id%s returned %+v, want only lead %s (not %s)", sort, page.Data, tagged, untagged)
		}
		if len(page.Data[0].Tags) != 1 || page.Data[0].Tags[0].TagID != tag.ID {
			t.Fatalf("tag_id%s row tags = %+v, want the applied tag", sort, page.Data[0].Tags)
		}
	}
}

func createTaggableLead(t *testing.T, e *apptest.AppEnv, name, company string) string {
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
