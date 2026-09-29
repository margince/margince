// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The standard fields a seller filters on, over the wire: every record is
// written through its own POST, every filter is asked through /filters/preview
// or a Live List, and each field has to pick its record out from a neighbour
// that differs in that field alone.

import (
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// previewIDs is the set of record ids /filters/preview answers for one filter,
// failing unless its count agrees with its rows.
func previewIDs(t *testing.T, e *apptest.AppEnv, resource string, filter AnyMap) []string {
	t.Helper()
	var got struct {
		MatchCount int      `json:"match_count"`
		Rows       []AnyMap `json:"rows"`
	}
	mustCall(t, e, "POST", "/v1/filters/preview", AnyMap{
		"resource": resource, "filter": filter, "limit": 100,
	}, http.StatusOK, &got)
	out := make([]string, 0, len(got.Rows))
	for _, row := range got.Rows {
		id, _ := row["id"].(string)
		out = append(out, id)
	}
	if got.MatchCount != len(out) {
		t.Fatalf("%s %v: match_count %d over %d rows", resource, filter, got.MatchCount, len(out))
	}
	slices.Sort(out)
	return out
}

// expectSelects asserts each filter selects exactly the records named.
func expectSelects(t *testing.T, e *apptest.AppEnv, resource string, cases []filterCase) {
	t.Helper()
	for _, c := range cases {
		want := slices.Clone(c.want)
		slices.Sort(want)
		if got := previewIDs(t, e, resource, c.filter); !slices.Equal(got, want) {
			t.Errorf("%s %v selected %v, want %v", resource, c.filter, got, want)
		}
	}
}

type filterCase struct {
	filter AnyMap
	want   []string
}

func leaf(field, op string, value any) AnyMap {
	return AnyMap{"field": field, "op": op, "value": value}
}

func daysAgo(n int) AnyMap { return AnyMap{"days_ago": n} }

// touch logs a note on one record, dated n days back, through the activity
// writer — the path that moves the record's last activity.
func touch(t *testing.T, e *apptest.AppEnv, entityType, id string, n int) {
	t.Helper()
	mustCall(t, e, "POST", "/v1/activities", AnyMap{
		"kind": "note", "body": "Checked in", "source": "manual",
		"occurred_at": time.Now().UTC().AddDate(0, 0, -n).Format(time.RFC3339),
		"links":       []AnyMap{{"entity_type": entityType, "entity_id": id}},
	}, http.StatusCreated, nil)
}

func TestTheStandardContactFieldsSelectTheirRecords(t *testing.T) {
	e, _ := listsApp(t, true)
	acme := createdID(t, e, "/v1/companies", AnyMap{"display_name": "Acme Werke", "source": "manual"})
	anna := createdID(t, e, "/v1/contacts", AnyMap{
		"full_name": "Anna Quiet", "source": "manual", "title": "Head of Procurement",
		"emails":  []AnyMap{{"email": "anna@acme.example", "is_primary": true}},
		"address": AnyMap{"city": "Leipzig", "country": "DE"},
	})
	ben := createdID(t, e, "/v1/contacts", AnyMap{
		"full_name": "Ben Busy", "source": "manual", "title": "Engineer",
		"emails":  []AnyMap{{"email": "ben@other.example", "is_primary": true}},
		"address": AnyMap{"city": "Lyon", "country": "FR"},
	})
	mustCall(t, e, "POST", "/v1/relationships", AnyMap{
		"kind": "employment", "contact_id": anna, "company_id": acme, "is_current_primary": true, "source": "manual",
	}, http.StatusCreated, nil)
	touch(t, e, "contact", anna, 60)
	touch(t, e, "contact", ben, 3)

	expectSelects(t, e, "contact", []filterCase{
		{leaf("name", "eq", "Anna Quiet"), []string{anna}},
		{leaf("name", "contains", "busy"), []string{ben}},
		{leaf("email", "eq", "Anna@ACME.example"), []string{anna}},
		{leaf("email", "contains", "@other."), []string{ben}},
		{leaf("title", "contains", "procurement"), []string{anna}},
		{leaf("company_id", "eq", acme), []string{anna}},
		{leaf("company_id", "exists", false), []string{ben}},
		{leaf("country", "eq", "FR"), []string{ben}},
		{leaf("city", "in", []any{"Leipzig"}), []string{anna}},
		{leaf("created_at", "gte", daysAgo(0)), []string{anna, ben}},
		{leaf("created_at", "lt", daysAgo(0)), []string{}},
		{leaf("last_activity_at", "lt", daysAgo(45)), []string{anna}},
		{leaf("last_activity_at", "gte", daysAgo(45)), []string{ben}},
	})
}

func TestTheStandardCompanyFieldsSelectTheirRecords(t *testing.T) {
	e, _ := listsApp(t, true)
	quiet := createdID(t, e, "/v1/companies", AnyMap{
		"display_name": "Quiet Maschinenbau", "source": "manual",
		"address": AnyMap{"city": "Leipzig", "country": "DE"},
	})
	busy := createdID(t, e, "/v1/companies", AnyMap{
		"display_name": "Busy Logistique", "source": "manual",
		"address": AnyMap{"city": "Lyon", "country": "FR"},
	})
	touch(t, e, "company", quiet, 90)
	touch(t, e, "company", busy, 1)

	expectSelects(t, e, "company", []filterCase{
		{leaf("name", "contains", "maschinen"), []string{quiet}},
		{leaf("country", "eq", "FR"), []string{busy}},
		{leaf("city", "eq", "Leipzig"), []string{quiet}},
		{leaf("created_at", "gte", daysAgo(1)), []string{quiet, busy}},
		{leaf("last_activity_at", "lt", daysAgo(45)), []string{quiet}},
	})
}

func TestTheStandardDealFieldsSelectTheirRecordsAndNeverMixCurrencies(t *testing.T) {
	e, _ := listsApp(t, true)
	company := createdID(t, e, "/v1/companies", AnyMap{"display_name": "Deal Holder", "source": "manual"})
	pipeline, stage, _ := companyRollupOpenStage(t, e)
	deal := func(name string, amount int64, currency, closes string) string {
		return createdID(t, e, "/v1/deals", AnyMap{
			"name": name, "pipeline_id": pipeline, "stage_id": stage, "company_id": company,
			"source": "manual", "amount_minor": amount, "currency": currency, "expected_close_date": closes,
		})
	}
	big := deal("Big rollout", 5_000_000, "EUR", "2026-11-30")
	small := deal("Small pilot", 90_000, "EUR", "2027-02-15")
	// No USD rate is on the sheet, so this deal has no worth in the base
	// currency and must not be compared as though its dollars were euros.
	dollars := deal("Dollar deal", 9_000_000, "USD", "2026-12-01")

	expectSelects(t, e, "deal", []filterCase{
		{leaf("name", "contains", "rollout"), []string{big}},
		{leaf("amount", "gte", 1_000_000), []string{big}},
		{leaf("amount", "lt", 1_000_000), []string{small}},
		{leaf("amount", "exists", false), []string{dollars}},
		{leaf("expected_close_date", "lt", "2027-01-01"), []string{big, dollars}},
		{leaf("created_at", "gte", daysAgo(0)), []string{big, small, dollars}},
		{leaf("last_activity_at", "exists", true), []string{}},
	})
}

func TestTheStandardLeadFieldsSelectTheirRecords(t *testing.T) {
	e, _ := listsApp(t, true)
	webinar := createdID(t, e, "/v1/leads", AnyMap{
		"full_name": "Carla Webinar", "email": "carla@lead.example", "source": "webinar",
	})
	manual := createdID(t, e, "/v1/leads", AnyMap{"full_name": "Dan Manual", "source": "manual"})
	var judged struct {
		Score int `json:"score"`
	}
	mustCall(t, e, "PATCH", "/v1/leads/"+webinar, AnyMap{
		"score": 85, "score_override_reason": "Asked for a quote on the call",
	}, http.StatusOK, &judged)
	if judged.Score != 85 {
		t.Fatalf("the override left the score at %d", judged.Score)
	}

	expectSelects(t, e, "lead", []filterCase{
		{leaf("name", "eq", "Dan Manual"), []string{manual}},
		{leaf("email", "eq", "CARLA@lead.example"), []string{webinar}},
		{leaf("email", "exists", false), []string{manual}},
		{leaf("source", "eq", "webinar"), []string{webinar}},
		{leaf("score", "gte", 85), []string{webinar}},
		{leaf("score", "gt", 85), []string{}},
		{leaf("score", "lt", 84.5), []string{manual}},
		{leaf("created_at", "gte", daysAgo(0)), []string{webinar, manual}},
	})
}

// clauseWire is one explained clause of a Live List's filter.
type clauseWire struct {
	Field  string  `json:"field"`
	Result *bool   `json:"result"`
	Value  *string `json:"value"`
	Hidden bool    `json:"hidden"`
}

func TestAQuietForFortyFiveDaysListHoldsTheQuietContactAndSaysWhy(t *testing.T) {
	e, _ := listsApp(t, true)
	quiet := createdID(t, e, "/v1/contacts", AnyMap{"full_name": "Quiet Contact", "source": "manual"})
	busy := createdID(t, e, "/v1/contacts", AnyMap{"full_name": "Busy Contact", "source": "manual"})
	touch(t, e, "contact", quiet, 60)
	touch(t, e, "contact", busy, 2)

	var list listWire
	mustCall(t, e, "POST", "/v1/lists", AnyMap{
		"name": "No activity in 45 days", "entity_type": "contact", "list_type": "dynamic",
		"definition": leaf("last_activity_at", "lt", daysAgo(45)),
	}, http.StatusCreated, &list)
	var members pageWire
	mustCall(t, e, "GET", "/v1/lists/"+list.ID+"/members", nil, http.StatusOK, &members)
	if len(members.Data) != 1 || members.Data[0]["entity_id"] != quiet {
		t.Fatalf("members = %v, want only the contact quiet for 60 days", members.Data)
	}

	var why struct {
		Member  bool       `json:"member"`
		Clauses clauseWire `json:"clauses"`
	}
	mustCall(t, e, "GET", "/v1/lists/"+list.ID+"/members/"+quiet+"/why", nil, http.StatusOK, &why)
	wantDay := time.Now().UTC().AddDate(0, 0, -60).Format(time.DateOnly)
	if !why.Member || why.Clauses.Result == nil || !*why.Clauses.Result ||
		why.Clauses.Value == nil || *why.Clauses.Value != wantDay {
		t.Errorf("why for the quiet contact = %+v, want a member whose clause holds on %s", why, wantDay)
	}
	mustCall(t, e, "GET", "/v1/lists/"+list.ID+"/members/"+busy+"/why", nil, http.StatusOK, &why)
	if why.Member || why.Clauses.Result == nil || *why.Clauses.Result {
		t.Errorf("why for the busy contact = %+v, want a failed clause", why)
	}
}

func TestATextClauseExplainsWithTheRecordsOwnValue(t *testing.T) {
	e, _ := listsApp(t, true)
	named := createdID(t, e, "/v1/contacts", AnyMap{
		"full_name": "Explained Person", "source": "manual",
		"emails": []AnyMap{{"email": "explained@person.example", "is_primary": true}},
	})
	var list listWire
	mustCall(t, e, "POST", "/v1/lists", AnyMap{
		"name": "By address", "entity_type": "contact", "list_type": "dynamic",
		"definition": leaf("email", "eq", "Explained@Person.example"),
	}, http.StatusCreated, &list)
	var why struct {
		Member  bool       `json:"member"`
		Clauses clauseWire `json:"clauses"`
	}
	mustCall(t, e, "GET", "/v1/lists/"+list.ID+"/members/"+named+"/why", nil, http.StatusOK, &why)
	if !why.Member || why.Clauses.Hidden || why.Clauses.Value == nil || *why.Clauses.Value != "explained@person.example" {
		t.Errorf("why = %+v, want a member explained by the stored address", why)
	}
}
