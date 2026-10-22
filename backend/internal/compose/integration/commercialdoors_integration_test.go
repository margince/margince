// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// sharedOffer is what both doors answer about an offer's lines.
type sharedOffer struct {
	Currency  string `json:"currency"`
	LineItems []struct {
		Quantity       float64 `json:"quantity"`
		UnitPriceMinor int64   `json:"unit_price_minor"`
	} `json:"line_items"`
}

// A passport makes, reads, lists and prices an offer through the record tools
// and change_record_parts as it does through the offer routes. The two answers agree
// on the deal's offers and on each offer's lines.
func TestOfferRoutesAnswerAsTheirTools(t *testing.T) {
	d := newTwoDoors(t, "offer-doors", "read", "write")
	deal := openDeal(t, d.e, apptest.DiscoverSeededPipeline(t, d.e), "Offer renewal")
	product := createdRecord(t, d.e, "/v1/products", AnyMap{
		"source": "manual", "name": "Onboarding day", "unit_price_minor": 120000, "currency": "EUR",
	})
	lines := []AnyMap{{"product_id": product, "quantity": 2}}

	var viaREST struct {
		ID string `json:"id"`
	}
	if status := d.e.Call(t, "POST", "/v1/deals/"+deal+"/offers", AnyMap{"currency": "EUR", "source": "manual", "line_items": lines},
		d.bearer, &viaREST); status != http.StatusCreated {
		t.Fatalf("creating an offer as a passport → %d", status)
	}
	viaTool := recordIDOf(t, d.tool(t, "create_record", map[string]any{"record_type": "offer", "fields": map[string]any{
		"deal_id": deal, "currency": "EUR", "line_items": lines,
	}}))

	sameDoorRecords(t, "the deal's offers",
		d.rest(t, "GET", "/v1/deals/"+deal+"/offers?limit=10", nil),
		d.tool(t, "list_records", map[string]any{"record_type": "offer", "limit": 10, "filters": map[string]any{"deal_id": deal}}),
		viaREST.ID, viaTool)

	added := AnyMap{"product_id": product, "quantity": 1}
	var restAnswer json.RawMessage
	if status := d.e.Call(t, "POST", "/v1/offers/"+viaREST.ID+"/line-items", added, d.bearer, &restAnswer); status != http.StatusCreated {
		t.Fatalf("adding a line as a passport → %d", status)
	}
	toolAnswer := d.tool(t, "change_record_parts", map[string]any{"action": "add_offer_line", "offer_id": viaTool, "line": added})
	priced := sameRecords[sharedOffer](t, "after a line is added", restAnswer, toolAnswer)
	if len(priced.LineItems) != 2 || priced.LineItems[1].UnitPriceMinor != 120000 {
		t.Errorf("the offer carries %+v, want two lines priced from the product", priced.LineItems)
	}
	sameRecords[sharedOffer](t, "read back",
		d.rest(t, "GET", "/v1/offers/"+viaREST.ID, nil),
		recordFieldsOf(t, d.tool(t, "read_record", map[string]any{"record_type": "offer", "id": viaTool})))
}

// sharedFact is what both doors answer about a stated company fact.
type sharedFact struct {
	Category string `json:"category"`
	Field    string `json:"field"`
	Value    string `json:"value"`
	Source   string `json:"source"`
}

// A fact stated or confirmed through change_record_parts is the row the facts
// routes write.
func TestCompanyFactRoutesAnswerAsTheirTool(t *testing.T) {
	d := newTwoDoors(t, "fact-doors", "read", "write")
	viaREST := createdRecord(t, d.e, "/v1/companies", AnyMap{"display_name": "Fact Door REST GmbH", "source": "manual"})
	viaTool := createdRecord(t, d.e, "/v1/companies", AnyMap{"display_name": "Fact Door Tool GmbH", "source": "manual"})
	fact := AnyMap{"category": "company", "field": "employee_range", "value": "11-50"}

	var restAnswer struct {
		sharedFact
		ValueKey string `json:"value_key"`
	}
	if status := d.e.Call(t, "POST", "/v1/companies/"+viaREST+"/facts", fact, d.bearer, &restAnswer); status != http.StatusCreated {
		t.Fatalf("stating a fact as a passport → %d", status)
	}
	restRaw, err := json.Marshal(restAnswer.sharedFact)
	if err != nil {
		t.Fatalf("encoding the REST fact: %v", err)
	}
	toolAnswer := d.tool(t, "change_record_parts", map[string]any{
		"action": "add_company_fact", "company_id": viaTool, "category": "company", "field": "employee_range", "value": "11-50",
	})
	sameRecords[sharedFact](t, "a stated fact", restRaw, toolAnswer)

	key := "employee_range:" + restAnswer.ValueKey
	sameRecords[confirmedFact](t, "a confirmed fact",
		d.rest(t, "POST", "/v1/companies/"+viaTool+"/facts/"+key+"/confirm", nil),
		d.tool(t, "change_record_parts", map[string]any{"action": "confirm_company_fact", "company_id": viaREST, "fact_key": key}))
}

// confirmedFact is what a confirm changes on a fact.
type confirmedFact struct {
	Source     string  `json:"source"`
	Confidence float64 `json:"confidence"`
	Version    int64   `json:"version"`
}

// A room and a thread in it are made, listed and read alike on either door.
func TestDealRoomRoutesAnswerAsTheirTools(t *testing.T) {
	d := newTwoDoors(t, "room-doors", "read", "write")
	deal := openDeal(t, d.e, apptest.DiscoverSeededPipeline(t, d.e), "Room renewal")

	room := recordIDOf(t, d.tool(t, "create_record", map[string]any{"record_type": "deal_room", "fields": map[string]any{
		"deal_id": deal, "title": "Renewal room",
	}}))
	sameDoorRecords(t, "the deal's rooms",
		d.rest(t, "GET", "/v1/deal-rooms?deal_id="+deal, nil),
		d.tool(t, "list_records", map[string]any{"record_type": "deal_room", "filters": map[string]any{"deal_id": deal}}),
		room)

	thread := recordIDOf(t, d.tool(t, "create_record", map[string]any{"record_type": "deal_room_thread", "fields": map[string]any{
		"deal_room_id": room, "body": "Any questions on the terms?",
	}}))
	sameDoorRecords(t, "the room's threads",
		d.rest(t, "GET", "/v1/deal-rooms/"+room+"/threads", nil),
		d.tool(t, "list_records", map[string]any{"record_type": "deal_room_thread", "filters": map[string]any{"deal_room_id": room}}),
		thread)
}

// recordFieldsOf is a record tool's answer reduced to the record's own fields.
func recordFieldsOf(t *testing.T, answer json.RawMessage) json.RawMessage {
	t.Helper()
	return memberOf(t, answer, "fields")
}
