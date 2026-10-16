// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// removedPart is what change_record_parts answers for a part it took away,
// where the route answers 204 with no body.
const removedPart = `{"removed":true}`

// stated is a fact or profile field as both doors answer it, with the version
// a later write pins.
type stated struct {
	Field   string `json:"field"`
	Value   string `json:"value"`
	Source  string `json:"source"`
	Version int64  `json:"version"`
}

// A fact is corrected and then removed alike on the facts routes and through
// change_record_parts, each write pinned to the version the caller read.
func TestCompanyFactCorrectionAndRemovalAnswerAsTheirRoutes(t *testing.T) {
	d := newTwoDoors(t, "fact-fix-doors", "read", "write")
	viaREST := createdRecord(t, d.e, "/v1/companies", AnyMap{"display_name": "Fact Fix REST GmbH", "source": "manual"})
	viaTool := createdRecord(t, d.e, "/v1/companies", AnyMap{"display_name": "Fact Fix Tool GmbH", "source": "manual"})
	fact := AnyMap{"category": "company", "field": "employee_range", "value": "11-50"}
	restFact := decodedAs[struct {
		stated
		ValueKey string `json:"value_key"`
	}](t, d.restStatus(t, "POST", "/v1/companies/"+viaREST+"/facts", fact, http.StatusCreated))
	toolFact := decodedAs[stated](t, d.toolSeed(t, "change_record_parts", map[string]any{
		"action": "add_company_fact", "company_id": viaTool, "category": "company", "field": "employee_range", "value": "11-50",
	}))
	key := "employee_range:" + restFact.ValueKey

	corrected := sameRecords[stated](t, "a corrected fact",
		d.restPinned(t, "PATCH", "/v1/companies/"+viaREST+"/facts/"+key, AnyMap{"value": "51-200"}, int(restFact.Version)),
		d.tool(t, "change_record_parts", map[string]any{
			"action": "update_company_fact", "company_id": viaTool, "fact_key": key, "value": "51-200", "if_version": toolFact.Version,
		}))
	if corrected.Value != "51-200" || corrected.Source != "agent" {
		t.Errorf("the corrected fact is %+v, want 51-200 stated by the passport's agent", corrected)
	}

	stale := d.toolRefused(t, "change_record_parts", map[string]any{
		"action": "remove_company_fact", "company_id": viaTool, "fact_key": key, "if_version": toolFact.Version,
	})
	if stale == "" {
		t.Error("removing a fact at a version since overtaken was refused without a word")
	}

	headers := map[string]string{"If-Match": strconv.FormatInt(corrected.Version, 10)}
	maps.Copy(headers, d.bearer)
	d.restStatusWith(t, "DELETE", "/v1/companies/"+viaREST+"/facts/"+key, nil, headers, http.StatusNoContent)
	if removed := d.tool(t, "change_record_parts", map[string]any{
		"action": "remove_company_fact", "company_id": viaTool, "fact_key": key, "if_version": corrected.Version,
	}); string(removed) != removedPart {
		t.Errorf("removing a fact through the tool answered %s, want %s", removed, removedPart)
	}
	sameDocument(t, "the facts left after removal",
		d.rest(t, "GET", "/v1/companies/"+viaREST+"/facts", nil),
		d.rest(t, "GET", "/v1/companies/"+viaTool+"/facts", nil))
}

// A profile field a human stated is corrected and confirmed alike on the
// profile-field routes and through change_record_parts.
func TestCompanyProfileFieldWritesAnswerAsTheirRoutes(t *testing.T) {
	d := newTwoDoors(t, "profile-doors", "read", "write")
	viaREST := createdRecord(t, d.e, "/v1/companies", AnyMap{"display_name": "Profile REST GmbH", "source": "manual"})
	viaTool := createdRecord(t, d.e, "/v1/companies", AnyMap{"display_name": "Profile Tool GmbH", "source": "manual"})
	// Originating a profile claim is a human act, so the signed-in admin states
	// it and the passport corrects it.
	for _, company := range []string{viaREST, viaTool} {
		if status := d.e.Call(t, "PATCH", "/v1/companies/"+company+"/profile-fields/legal_form",
			AnyMap{"value": "GmbH"}, nil, nil); status != http.StatusOK {
			t.Fatalf("a human stating the legal form → %d", status)
		}
	}

	corrected := sameRecords[stated](t, "a corrected profile field",
		d.rest(t, "PATCH", "/v1/companies/"+viaREST+"/profile-fields/legal_form", AnyMap{"value": "AG"}),
		d.tool(t, "change_record_parts", map[string]any{
			"action": "update_company_profile_field", "company_id": viaTool, "field": "legal_form", "value": "AG",
		}))
	if corrected.Field != "legal_form" || corrected.Value != "AG" {
		t.Errorf("the corrected field is %+v, want legal_form AG", corrected)
	}
	sameRecords[stated](t, "a confirmed profile field",
		d.rest(t, "POST", "/v1/companies/"+viaREST+"/profile-fields/legal_form/confirm", nil),
		d.tool(t, "change_record_parts", map[string]any{
			"action": "confirm_company_profile_field", "company_id": viaTool, "field": "legal_form", "if_version": corrected.Version,
		}))
}

// projectSide is a project's company or stakeholder edge as both doors answer
// it.
type projectSide struct {
	CompanyID string `json:"company_id"`
	ContactID string `json:"contact_id"`
	Role      string `json:"role"`
}

// A project's companies and stakeholders are set and taken off alike on the
// project routes and through change_record_parts.
func TestProjectCompaniesAndStakeholdersAnswerAsTheirRoutes(t *testing.T) {
	d := newTwoDoors(t, "project-part-doors", "read", "write")
	customer := createdRecord(t, d.e, "/v1/companies", AnyMap{"source": "manual", "display_name": "Part Customer"})
	partner := createdRecord(t, d.e, "/v1/companies", AnyMap{"source": "manual", "display_name": "Part Partner"})
	contact := createdRecord(t, d.e, "/v1/contacts", AnyMap{"source": "manual", "full_name": "Petra Sponsor"})
	viaREST := createdRecord(t, d.e, "/v1/projects", AnyMap{"name": "Part rollout REST", "company_id": customer, "source": "manual"})
	viaTool := createdRecord(t, d.e, "/v1/projects", AnyMap{"name": "Part rollout tool", "company_id": customer, "source": "manual"})

	on := sameRecords[struct {
		Data []projectSide `json:"data"`
	}](t, "the companies on the project",
		d.rest(t, "PUT", "/v1/projects/"+viaREST+"/companies", AnyMap{"company_id": partner, "role": "partner"}),
		d.tool(t, "change_record_parts", map[string]any{
			"action": "set_project_company", "project_id": viaTool, "company_id": partner, "role": "partner",
		}))
	if len(on.Data) != 2 {
		t.Errorf("the project carries %+v, want its customer and the partner", on.Data)
	}
	if refusal := d.toolRefused(t, "change_record_parts", map[string]any{
		"action": "set_project_company", "project_id": viaTool, "company_id": partner, "role": " ",
	}); refusal == "" {
		t.Error("a blank role was refused without a word")
	}

	seat := sameRecords[projectSide](t, "a project stakeholder",
		d.rest(t, "PUT", "/v1/projects/"+viaREST+"/stakeholders", AnyMap{"contact_id": contact, "role": "sponsor"}),
		d.tool(t, "change_record_parts", map[string]any{
			"action": "set_project_stakeholder", "project_id": viaTool, "contact_id": contact, "role": "sponsor",
		}))
	if seat.ContactID != contact || seat.Role != "sponsor" {
		t.Errorf("the stakeholder edge is %+v, want %s as sponsor", seat, contact)
	}

	for _, c := range []struct{ route, action, arg, id string }{
		{"stakeholders", "remove_project_stakeholder", "contact_id", contact},
		{"companies", "remove_project_company", "company_id", partner},
	} {
		d.restStatus(t, "DELETE", "/v1/projects/"+viaREST+"/"+c.route+"/"+c.id, nil, http.StatusNoContent)
		if removed := d.tool(t, "change_record_parts", map[string]any{
			"action": c.action, "project_id": viaTool, c.arg: c.id,
		}); string(removed) != removedPart {
			t.Errorf("%s answered %s, want %s", c.action, removed, removedPart)
		}
	}
	back := sameRecords[struct {
		Data []projectSide `json:"data"`
	}](t, "a company put back with no role",
		d.rest(t, "PUT", "/v1/projects/"+viaREST+"/companies", AnyMap{"company_id": partner}),
		d.tool(t, "change_record_parts", map[string]any{"action": "set_project_company", "project_id": viaTool, "company_id": partner}))
	for _, side := range back.Data {
		if side.CompanyID == partner && side.Role != "customer" {
			t.Errorf("a company put back with no role is %q, want the default customer", side.Role)
		}
	}
	sameDocument(t, "the stakeholders left",
		d.rest(t, "GET", "/v1/projects/"+viaREST+"/stakeholders", nil),
		d.rest(t, "GET", "/v1/projects/"+viaTool+"/stakeholders", nil))
}

// An offer's line is changed and then removed alike on the line-item routes and
// through change_record_parts.
func TestOfferLineChangeAndRemovalAnswerAsTheirRoutes(t *testing.T) {
	d := newTwoDoors(t, "offer-line-doors", "read", "write")
	deal := openDeal(t, d.e, apptest.DiscoverSeededPipeline(t, d.e), "Line renewal")
	product := createdRecord(t, d.e, "/v1/products", AnyMap{
		"source": "manual", "name": "Training day", "unit_price_minor": 90000, "currency": "EUR",
	})
	offerWithLines := func() (string, string) {
		offer := recordIDOf(t, d.toolSeed(t, "create_record", map[string]any{"record_type": "offer", "fields": map[string]any{
			"deal_id": deal, "currency": "EUR", "line_items": []AnyMap{{"product_id": product, "quantity": 1}, {"product_id": product, "quantity": 4}},
		}}))
		lines := decodedAs[struct {
			LineItems []struct {
				ID string `json:"id"`
			} `json:"line_items"`
		}](t, d.rest(t, "GET", "/v1/offers/"+offer, nil))
		return offer, lines.LineItems[0].ID
	}
	restOffer, restLine := offerWithLines()
	toolOffer, toolLine := offerWithLines()

	changed := sameRecords[sharedOffer](t, "after a line is changed",
		d.rest(t, "PATCH", "/v1/offers/"+restOffer+"/line-items/"+restLine, AnyMap{"quantity": 3}),
		d.tool(t, "change_record_parts", map[string]any{
			"action": "update_offer_line", "offer_id": toolOffer, "line_item_id": toolLine, "line": map[string]any{"quantity": 3},
		}))
	if len(changed.LineItems) != 2 || changed.LineItems[0].Quantity != 3 {
		t.Errorf("the offer carries %+v, want its first line at quantity 3", changed.LineItems)
	}
	left := sameRecords[sharedOffer](t, "after a line is removed",
		d.rest(t, "DELETE", "/v1/offers/"+restOffer+"/line-items/"+restLine, nil),
		d.tool(t, "change_record_parts", map[string]any{"action": "remove_offer_line", "offer_id": toolOffer, "line_item_id": toolLine}))
	if len(left.LineItems) != 1 || left.LineItems[0].Quantity != 4 {
		t.Errorf("the offer carries %+v, want only the line of four", left.LineItems)
	}
}

// list_records narrows companies and leads to one list's members, as the
// routes' list_id does.
func TestListRecordsNarrowsCompaniesAndLeadsToAListsMembers(t *testing.T) {
	d := newTwoDoorsWith(t, "member-doors", []compose.Option{compose.WithListsEnabled(true)}, "read")
	for _, c := range []struct{ kind, route, nameField string }{
		{"company", "companies", "display_name"},
		{"lead", "leads", "full_name"},
	} {
		member := createdRecord(t, d.e, "/v1/"+c.route, AnyMap{"source": "manual", c.nameField: "Member " + c.kind})
		createdRecord(t, d.e, "/v1/"+c.route, AnyMap{"source": "manual", c.nameField: "Outsider " + c.kind})
		list := createdID(t, d.e, "/v1/lists", AnyMap{"name": "Shortlist " + c.kind, "entity_type": c.kind, "list_type": "static"})
		if status := d.e.Call(t, "POST", "/v1/lists/"+list+"/members", AnyMap{"entity_type": c.kind, "entity_id": member},
			nil, nil); status >= http.StatusBadRequest {
			t.Fatalf("adding a %s to its list → %d", c.kind, status)
		}
		sameDoorRecords(t, c.kind+" list members",
			d.rest(t, "GET", "/v1/"+c.route+"?list_id="+list+"&limit=10", nil),
			d.tool(t, "list_records", map[string]any{"record_type": c.kind, "limit": 10, "filters": map[string]any{"list_id": list}}),
			member)
	}
}

// list_records narrows the timeline to one record's unanswered mail, as the
// activities route's entity pair and waiting_reply do.
func TestListRecordsNarrowsTheTimelineToARecordsUnansweredMail(t *testing.T) {
	d := newTwoDoors(t, "timeline-doors", "read")
	contact := createdID(t, d.e, "/v1/contacts", AnyMap{"full_name": "Wanda Waiting", "source": "manual"})
	waiting := createdID(t, d.e, "/v1/activities", AnyMap{
		"kind": "email", "direction": "inbound", "subject": "Contract?", "body": "Where is the contract", "source": "manual",
		"links": []AnyMap{{"entity_type": "contact", "entity_id": contact}},
	})
	note := createdID(t, d.e, "/v1/activities", AnyMap{
		"kind": "note", "subject": "Call notes", "source": "manual",
		"links": []AnyMap{{"entity_type": "contact", "entity_id": contact}},
	})

	filters := map[string]any{"entity_type": "contact", "entity_id": contact, "waiting_reply": "true"}
	rest := d.rest(t, "GET", "/v1/activities?entity_type=contact&entity_id="+contact+"&waiting_reply=true&limit=10", nil)
	sameDoorRecords(t, "the contact's unanswered mail", rest,
		d.tool(t, "list_records", map[string]any{"record_type": "activity", "limit": 10, "filters": filters}), waiting)
	if slices.Contains(documentRecords(rest), note) {
		t.Error("the waiting list carries a note, which no one replies to")
	}
}

// problemCode is the code a route's problem body names, which a tool refusal of
// the same kind carries too.
func problemCode(t *testing.T, answer json.RawMessage) string {
	t.Helper()
	code := decodedAs[struct {
		Code string `json:"code"`
	}](t, answer).Code
	if code == "" {
		t.Fatalf("the route's refusal names no code: %s", answer)
	}
	return code
}

// namesCode reports a tool refusal naming code, which a not-found refusal
// spells in words: "(not found)".
func namesCode(refusal, code string) bool {
	return strings.Contains(refusal, code) || strings.Contains(refusal, strings.ReplaceAll(code, "_", " "))
}

// A part write the route refuses, change_record_parts refuses too.
func TestChangeRecordPartsRefusesWhatItsRoutesRefuse(t *testing.T) {
	d := newTwoDoors(t, "part-refusal-doors", "read", "write")
	company := createdRecord(t, d.e, "/v1/companies", AnyMap{"source": "manual", "display_name": "Refusal Works"})
	contact := createdRecord(t, d.e, "/v1/contacts", AnyMap{"source": "manual", "full_name": "Rafa Refused"})
	project := createdRecord(t, d.e, "/v1/projects", AnyMap{"name": "Refusal rollout", "company_id": company, "source": "manual"})
	deal := openDeal(t, d.e, apptest.DiscoverSeededPipeline(t, d.e), "Refusal renewal")
	offer := recordIDOf(t, d.toolSeed(t, "create_record", map[string]any{"record_type": "offer", "fields": map[string]any{
		"deal_id": deal, "currency": "EUR",
	}}))
	nowhere := ids.NewV7().String()
	key := "employee_range:nowhere"

	for _, c := range []struct {
		what         string
		method, path string
		body         AnyMap
		args         map[string]any
		status       int
	}{
		{
			"a fact field outside the vocabulary", "POST", "/v1/companies/" + company + "/facts",
			AnyMap{"category": "company", "field": "shoe_size", "value": "42"},
			map[string]any{"action": "add_company_fact", "company_id": company, "category": "company", "field": "shoe_size", "value": "42"},
			422,
		},
		{
			"correcting a fact the company does not state", "PATCH", "/v1/companies/" + company + "/facts/" + key,
			AnyMap{"value": "11-50"},
			map[string]any{"action": "update_company_fact", "company_id": company, "fact_key": key, "value": "11-50"},
			404,
		},
		{
			"confirming a fact the company does not state", "POST", "/v1/companies/" + company + "/facts/" + key + "/confirm",
			nil,
			map[string]any{"action": "confirm_company_fact", "company_id": company, "fact_key": key},
			404,
		},
		{
			"an agent originating a profile claim", "PATCH", "/v1/companies/" + company + "/profile-fields/legal_form",
			AnyMap{"value": "GmbH"},
			map[string]any{"action": "update_company_profile_field", "company_id": company, "field": "legal_form", "value": "GmbH"},
			404,
		},
		{
			"confirming a profile field nobody stated", "POST", "/v1/companies/" + company + "/profile-fields/legal_form/confirm",
			nil,
			map[string]any{"action": "confirm_company_profile_field", "company_id": company, "field": "legal_form"},
			404,
		},
		{
			"putting a company that does not exist on the project", "PUT", "/v1/projects/" + project + "/companies",
			AnyMap{"company_id": nowhere},
			map[string]any{"action": "set_project_company", "project_id": project, "company_id": nowhere},
			404,
		},
		{
			"taking the last company off the project", "DELETE", "/v1/projects/" + project + "/companies/" + company,
			nil,
			map[string]any{"action": "remove_project_company", "project_id": project, "company_id": company},
			422,
		},
		{
			"a stakeholder role outside the vocabulary", "PUT", "/v1/projects/" + project + "/stakeholders",
			AnyMap{"contact_id": contact, "role": "astronaut"},
			map[string]any{"action": "set_project_stakeholder", "project_id": project, "contact_id": contact, "role": "astronaut"},
			422,
		},
		{
			"detaching a contact that is not on the project", "DELETE", "/v1/projects/" + project + "/stakeholders/" + contact,
			nil,
			map[string]any{"action": "remove_project_stakeholder", "project_id": project, "contact_id": contact},
			404,
		},
		{
			"a line on an offer that does not exist", "POST", "/v1/offers/" + nowhere + "/line-items",
			AnyMap{"quantity": 1},
			map[string]any{"action": "add_offer_line", "offer_id": nowhere, "line": map[string]any{"quantity": 1}},
			404,
		},
		{
			"changing a line the offer does not carry", "PATCH", "/v1/offers/" + offer + "/line-items/" + nowhere,
			AnyMap{"quantity": 2},
			map[string]any{"action": "update_offer_line", "offer_id": offer, "line_item_id": nowhere, "line": map[string]any{"quantity": 2}},
			404,
		},
		{
			"removing a line the offer does not carry", "DELETE", "/v1/offers/" + offer + "/line-items/" + nowhere,
			nil,
			map[string]any{"action": "remove_offer_line", "offer_id": offer, "line_item_id": nowhere},
			404,
		},
	} {
		var answer json.RawMessage
		if status := d.e.Call(t, c.method, c.path, c.body, d.bearer, &answer); status != c.status {
			t.Errorf("%s: the route answered %d %s, want %d", c.what, status, answer, c.status)
		}
		if refusal := d.toolRefused(t, "change_record_parts", c.args); !namesCode(refusal, problemCode(t, answer)) {
			t.Errorf("%s: the tool refused with %s, not the route's %s", c.what, refusal, problemCode(t, answer))
		}
	}
}

// Both doors refuse a list narrowed by a list that does not exist, or sorted by
// a column the timeline lacks. Both refuse a commission nobody accrued.
func TestRecordListsRefuseWhatTheirRoutesRefuse(t *testing.T) {
	d := newTwoDoorsWith(t, "list-refusal-doors", []compose.Option{compose.WithListsEnabled(true)}, "read")
	nowhere := ids.NewV7().String()
	for _, c := range []struct {
		what, path string
		tool       string
		args       map[string]any
		status     int
	}{
		{
			"companies of a list that does not exist", "/v1/companies?list_id=" + nowhere, "list_records",
			map[string]any{"record_type": "company", "filters": map[string]any{"list_id": nowhere}},
			404,
		},
		{
			"leads of a list that does not exist", "/v1/leads?list_id=" + nowhere, "list_records",
			map[string]any{"record_type": "lead", "filters": map[string]any{"list_id": nowhere}},
			404,
		},
		{
			"contacts of a list that does not exist", "/v1/contacts?list_id=" + nowhere, "list_records",
			map[string]any{"record_type": "contact", "filters": map[string]any{"list_id": nowhere}},
			404,
		},
		{
			"the timeline in an order it has no column for", "/v1/activities?sort=shoe_size", "list_records",
			map[string]any{"record_type": "activity", "filters": map[string]any{"sort": "shoe_size"}},
			422,
		},
		{
			"a commission nobody accrued", "/v1/commissions/" + nowhere, "read_record",
			map[string]any{"record_type": "commission", "id": nowhere},
			404,
		},
	} {
		var answer json.RawMessage
		if status := d.e.Call(t, "GET", c.path, nil, d.bearer, &answer); status != c.status {
			t.Errorf("%s: the route answered %d %s, want %d", c.what, status, answer, c.status)
		}
		if refusal := d.toolRefused(t, c.tool, c.args); !namesCode(refusal, problemCode(t, answer)) {
			t.Errorf("%s: the tool refused with %s, not the route's %s", c.what, refusal, problemCode(t, answer))
		}
	}
}
