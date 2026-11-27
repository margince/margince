// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"regexp"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// refusalCase is one request a caller got wrong, and the answer that names
// the fault. That is the status and the code, plus the field and its code.
type refusalCase struct {
	name, method, path string
	body               any
	status             int
	code               string
	field, fieldCode   string
}

// sentinelTail matches the words an error sentinel leaves at the end of a
// wrapped sentence, which a caller must never read.
var sentinelTail = regexp.MustCompile(`: (not found|conflict|invalid argument|permission denied|version skew)$`)

// Each refusal names the input that is wrong, with the code the same mistake
// gets on other routes. No detail ends in a sentinel's words.
func TestEveryRefusalNamesTheFaultTheCallerMade(t *testing.T) {
	e, _ := listsApp(t, true)
	stages := apptest.DiscoverSeededPipeline(t, e)
	contact := createdRecord(t, e, "/v1/contacts", AnyMap{"linkedin_url": apptest.ContactProfile(), "source": "manual", "full_name": "Named Fault"})
	deal := openDeal(t, e, stages, "Named fault deal")
	product := createdRecord(t, e, "/v1/products", AnyMap{"source": "manual", "name": "Day", "unit_price_minor": 1000, "currency": "EUR"})
	offer := createdRecord(t, e, "/v1/deals/"+deal+"/offers", AnyMap{"currency": "EUR", "source": "manual", "line_items": []AnyMap{{"product_id": product, "quantity": 1}}})
	tag := createdRecord(t, e, "/v1/tags", AnyMap{"name": "Partner"})
	list := createdRecord(t, e, "/v1/lists", AnyMap{"name": "Chosen", "entity_type": "contact"})

	contactPath, primary := "/v1/contacts/"+contact, func(address string) []AnyMap { return []AnyMap{{"email": address, "is_primary": true}} }
	const unprocessable, validation = http.StatusUnprocessableEntity, "validation_error"
	for _, c := range []refusalCase{
		{"an address with a trailing dot", "PATCH", contactPath, AnyMap{"emails": primary("x@example.com.")}, unprocessable, validation, "emails[0].email", "email_malformed"},
		{"an address with a display name", "PATCH", contactPath, AnyMap{"emails": primary("Name <y@example.com>")}, unprocessable, validation, "emails[0].email", "email_malformed"},
		{"one address twice", "PATCH", contactPath, AnyMap{"emails": []AnyMap{{"email": "z@example.com", "is_primary": true}, {"email": "Z@example.com"}}}, unprocessable, validation, "emails[1].email", "duplicate"},
		{"a number past its range", "PATCH", "/v1/products/" + product, json.RawMessage(`{"default_tax_rate":1e400}`), unprocessable, validation, "default_tax_rate", "out_of_range"},
		{"a convert naming no holder", "PATCH", contactPath, AnyMap{"emails": primary("w@example.com"), "convert_lead_ids": []string{deal}}, unprocessable, validation, "convert_lead_ids", "reference_not_found"},
		{"an unknown key on an offer line", "POST", "/v1/offers/" + offer + "/line-items", AnyMap{"product_id": product, "quantity": 1, "bogus": 1}, unprocessable, validation, "fields", "unknown_field"},
		{"a tag without a name", "POST", "/v1/tags", AnyMap{}, unprocessable, validation, "name", "required"},
		{"a list without a name", "POST", "/v1/lists", AnyMap{"entity_type": "contact"}, unprocessable, validation, "name", "required"},
		{"a view without a resource", "POST", "/v1/views", AnyMap{"name": "Mine"}, unprocessable, validation, "resource", "required"},
		{"a visibility outside its list", "PATCH", contactPath, AnyMap{"visibility": "bogus"}, unprocessable, validation, "visibility", "invalid_enum"},
		{"a tag colour outside its list", "PATCH", "/v1/tags/" + tag, AnyMap{"color": "bogus"}, unprocessable, validation, "color", "invalid_enum"},
		{"a tag name taken", "POST", "/v1/tags", AnyMap{"name": "partner"}, http.StatusConflict, "conflict", "", ""},
		{"a removal of no member", "POST", "/v1/lists/" + list + "/members/remove", AnyMap{"entity_type": "contact", "entity_id": contact}, http.StatusNotFound, "not_found", "", ""},
		{"a reporting window without its start", "GET", "/v1/analytics/evaluate?end_at=2026-01-01T00:00:00Z", nil, unprocessable, validation, "start_at", "required"},
		{"a pin naming no lane", "PUT", "/v1/worklist/pins", AnyMap{}, unprocessable, validation, "source", "required"},
		{"Done naming no lane", "PUT", "/v1/worklist/done", AnyMap{"rows": []AnyMap{{"source": "made_up", "row_id": "x"}}}, unprocessable, validation, "rows[0].source", "invalid_enum"},
		{"Done naming no row", "PUT", "/v1/worklist/done", AnyMap{"rows": []AnyMap{{"source": "deal_at_risk", "row_id": ""}}}, unprocessable, validation, "rows[0].row_id", "required"},
		{"Done naming a malformed activity", "PUT", "/v1/worklist/done", AnyMap{"rows": []AnyMap{{"source": "awaiting_reply", "row_id": "not-an-id"}}}, unprocessable, validation, "rows[0].row_id", "invalid_uuid"},
		{"a report scope of no kind", "POST", "/v1/reports/deals-by-stage", AnyMap{"scope": AnyMap{"kind": "galaxy"}}, unprocessable, validation, "scope.kind", "invalid"},
	} {
		t.Run(c.name, func(t *testing.T) {
			var problem problemBody
			status := e.Call(t, c.method, c.path, c.body, nil, &problem)
			if status != c.status || problem.Code != c.code {
				t.Fatalf("answered %d %s %q, want %d %s", status, problem.Code, problem.Detail, c.status, c.code)
			}
			if sentinelTail.MatchString(problem.Detail) {
				t.Errorf("detail %q ends in a sentinel's words", problem.Detail)
			}
			if c.field == "" {
				return
			}
			if len(problem.Details.Errors) != 1 || problem.Details.Errors[0].Field != c.field || problem.Details.Errors[0].Code != c.fieldCode {
				t.Errorf("named %+v, want %s/%s", problem.Details.Errors, c.field, c.fieldCode)
			}
		})
	}
}

// A duplicate tag name answers with the tag that holds it, so a caller can
// apply that one.
func TestATakenTagNameNamesTheTagThatHoldsIt(t *testing.T) {
	e, _ := listsApp(t, true)
	held := createdRecord(t, e, "/v1/tags", AnyMap{"name": "Partner"})
	var problem struct {
		Detail  string `json:"detail"`
		Details struct {
			ExistingID string `json:"existing_id"`
		} `json:"details"`
	}
	if status := e.Call(t, "POST", "/v1/tags", AnyMap{"name": "PARTNER"}, nil, &problem); status != http.StatusConflict || problem.Details.ExistingID != held {
		t.Fatalf("answered %d %+v, want 409 naming %s", status, problem, held)
	}
}
