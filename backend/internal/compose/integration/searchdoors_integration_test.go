// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// GET /v1/search and search_records run one ranked engine, so one query
// answers the same records in the same order on both doors.

import (
	"net/http"
	"slices"
	"strings"
	"testing"
)

type routeSearchPage struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
	TypesWithMore *[]string `json:"types_with_more"`
}

type toolSearchPage struct {
	Records []struct {
		ID    string `json:"id"`
		Match *struct {
			WorksAt *struct {
				CompanyID string `json:"company_id"`
			} `json:"works_at"`
		} `json:"match"`
	} `json:"records"`
	TypesWithMore *[]string `json:"types_with_more"`
}

func (p routeSearchPage) ids() []string {
	out := make([]string, 0, len(p.Data))
	for _, hit := range p.Data {
		out = append(out, hit.ID)
	}
	return out
}

func (p toolSearchPage) ids() []string {
	out := make([]string, 0, len(p.Records))
	for _, record := range p.Records {
		out = append(out, record.ID)
	}
	return out
}

// sameRanking fails unless both doors answer the same records in the same order.
func sameRanking(t *testing.T, what string, rest, tool []string) {
	t.Helper()
	if !slices.Equal(rest, tool) {
		t.Errorf("%s: the route ranks %v, the tool ranks %v", what, rest, tool)
	}
}

// searchDoorsSeed is a workspace where one word ranks several contacts and a
// company, and a contact is found only through the company it works at.
type searchDoorsSeed struct {
	strong, weak, company, employee, unrelated string
}

func seedSearchDoors(t *testing.T, d *twoDoors) searchDoorsSeed {
	t.Helper()
	seed := searchDoorsSeed{
		strong:    createdRecord(t, d.e, "/v1/contacts", AnyMap{"source": "manual", "full_name": "Zephyrwind Zephyrwind Mara"}),
		weak:      createdRecord(t, d.e, "/v1/contacts", AnyMap{"source": "manual", "full_name": "Quentin Zephyrwind"}),
		company:   createdRecord(t, d.e, "/v1/companies", AnyMap{"source": "manual", "display_name": "Zephyrwind Werke"}),
		employee:  createdRecord(t, d.e, "/v1/contacts", AnyMap{"source": "manual", "full_name": "Ida Fenn"}),
		unrelated: createdRecord(t, d.e, "/v1/contacts", AnyMap{"source": "manual", "full_name": "Bernd Unrelated"}),
	}
	linkEdge(t, d.e, AnyMap{"kind": "employment", "contact_id": seed.employee, "company_id": seed.company, "source": "manual"})
	return seed
}

func TestSearchRouteAndSearchRecordsRankAlike(t *testing.T) {
	d := newTwoDoors(t, "search-ranked-doors", "read")
	seed := seedSearchDoors(t, d)
	perType := 1

	for _, c := range []struct {
		what  string
		route string
		args  map[string]any
		want  []string
	}{
		{
			"no type named", "/v1/search?q=Zephyrwind&limit=10",
			map[string]any{"q": "Zephyrwind", "limit": 10},
			[]string{seed.strong, seed.weak, seed.company},
		},
		{
			"contacts alone", "/v1/search?q=Zephyrwind&types=contact&limit=10",
			map[string]any{"q": "Zephyrwind", "record_types": []string{"contact"}, "limit": 10},
			[]string{seed.strong, seed.weak},
		},
		{
			"with employees", "/v1/search?q=Werke&with_employees=true&limit=10",
			map[string]any{"q": "Werke", "with_employees": true, "limit": 10},
			[]string{seed.company, seed.employee},
		},
		{
			"one of each type", "/v1/search?q=Zephyrwind&per_type=1",
			map[string]any{"q": "Zephyrwind", "per_type": perType},
			[]string{seed.strong, seed.company},
		},
	} {
		route := decodedAs[routeSearchPage](t, d.rest(t, "GET", c.route, nil))
		tool := decodedAs[toolSearchPage](t, d.tool(t, "search_records", c.args))
		sameRanking(t, c.what, route.ids(), tool.ids())
		for _, id := range c.want {
			if !slices.Contains(route.ids(), id) {
				t.Errorf("%s: the seeded record %s is missing from %v", c.what, id, route.ids())
			}
		}
		if slices.Contains(tool.ids(), seed.unrelated) {
			t.Errorf("%s: the tool returns the contact the query does not match", c.what)
		}
		if (route.TypesWithMore == nil) != (tool.TypesWithMore == nil) ||
			(route.TypesWithMore != nil && !slices.Equal(*route.TypesWithMore, *tool.TypesWithMore)) {
			t.Errorf("%s: the route names types with more %v, the tool %v", c.what, route.TypesWithMore, tool.TypesWithMore)
		}
	}
}

// The employee found through its employer says where it works on the tool too.
func TestSearchRecordsSaysWhereAnEmployeeWorks(t *testing.T) {
	d := newTwoDoors(t, "search-works-at", "read")
	seed := seedSearchDoors(t, d)
	tool := decodedAs[toolSearchPage](t, d.tool(t, "search_records",
		map[string]any{"q": "Werke", "record_types": []string{"contact"}, "with_employees": true}))
	if len(tool.Records) != 1 || tool.Records[0].ID != seed.employee {
		t.Fatalf("the contacts found through Werke are %v, want the employee %s", tool.ids(), seed.employee)
	}
	if match := tool.Records[0].Match; match == nil || match.WorksAt == nil || match.WorksAt.CompanyID != seed.company {
		t.Errorf("the employee does not say it works at %s", seed.company)
	}
}

// A grouped page takes no cursor, and both doors refuse it naming per_type.
func TestBothDoorsRefuseAGroupedPageWithACursor(t *testing.T) {
	d := newTwoDoors(t, "search-refused-doors", "read")
	var refused struct {
		Details struct {
			Errors []struct {
				Field string `json:"field"`
			} `json:"errors"`
		} `json:"details"`
	}
	status := d.e.Call(t, "GET", "/v1/search?q=Zephyrwind&per_type=1&cursor=abc", nil, d.bearer, &refused)
	if status != http.StatusUnprocessableEntity || len(refused.Details.Errors) == 0 || refused.Details.Errors[0].Field != "per_type" {
		t.Errorf("the route answered %d %+v, want 422 naming per_type", status, refused)
	}
	text := d.toolRefused(t, "search_records", map[string]any{"q": "Zephyrwind", "per_type": 1, "cursor": "abc"})
	if !strings.Contains(text, "per_type") {
		t.Errorf("the tool refused with %q, want it to name per_type", text)
	}
}
