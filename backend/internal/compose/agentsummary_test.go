// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The inbox row is what a triaging approver acts on, and for the whole REST
// admission surface it used to say only which verb hit which path. These
// cases pin what a human can now see without opening the envelope: the
// operation, and the values the call would actually write.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// summaryRequest is the request shape restSummary reads: the method, the path,
// and the routed {id} a nested create is posted under.
func summaryRequest(method, path string) *http.Request {
	return httptest.NewRequest(method, path, nil)
}

func TestRestSummaryNamesTheValuesTheCallWouldWrite(t *testing.T) {
	pol := agentPolicy{Op: "updateDeal", Tool: "update_record", RecordType: recordTypeDeal}
	got := restSummary(pol, summaryRequest("PATCH", "/v1/deals/018f2a10-0000-7000-8000-00000000000a"),
		[]byte(`{"amount_minor":100,"currency":"EUR","expected_close_date":"2027-06-30"}`))

	for _, want := range []string{
		"amount_minor=100", "currency=EUR", "expected_close_date=2027-06-30",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("summary %q does not carry %q — an approver cannot see what they are approving", got, want)
		}
	}
}

// The head is a sentence, not the contract's own vocabulary.
//
// It used to read "updateDeal (PATCH /v1/deals/018f2a10-…)": a camelCase
// operationId and a uuid, at the top of a card somebody had to decide. Neither
// tells a reader anything — the uuid names a record they cannot read from it,
// and the operationId is the contract's word rather than theirs.
func TestRestSummaryLeadsWithTheActRatherThanTheRoute(t *testing.T) {
	pol := agentPolicy{Op: "updateDeal", Tool: "update_record", RecordType: recordTypeDeal}
	got := restSummary(pol, summaryRequest("PATCH", "/v1/deals/018f2a10-0000-7000-8000-00000000000a"),
		[]byte(`{"amount_minor":100}`))

	if !strings.HasPrefix(got, "Update record deal") {
		t.Errorf("summary %q does not open by naming the act", got)
	}
	for _, unwanted := range []string{"updateDeal", "PATCH", "018f2a10"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("summary %q still carries %q, which says nothing to the person deciding", got, unwanted)
		}
	}
}

// A verb that already names its own record does not say it twice.
func TestRestSummaryDoesNotRepeatTheRecordTheVerbNames(t *testing.T) {
	pol := agentPolicy{Op: "sendEmail", Tool: "send_email", RecordType: recordTypeActivity}
	got := restSummary(pol, summaryRequest("POST", "/v1/activities/018f2a10-0000-7000-8000-00000000beef/send-email"), nil)
	if got != "Send email" {
		t.Errorf("summary is %q, want a sentence that names the act once", got)
	}
}

// A route the contract gives no verb has nothing better to say than what it is.
func TestRestSummaryFallsBackToTheRouteWhenNoVerbIsDeclared(t *testing.T) {
	pol := agentPolicy{Op: "someInternalThing"}
	got := restSummary(pol, summaryRequest("POST", "/v1/internal/thing"), nil)
	if !strings.Contains(got, "someInternalThing") || !strings.Contains(got, "/v1/internal/thing") {
		t.Errorf("summary %q drops the only identification an undeclared route has", got)
	}
}

// A body-less action route (send this offer, archive this record) has no
// fields to name, and the operation IS the whole change.
func TestRestSummaryOfABodylessActionNamesTheOperation(t *testing.T) {
	pol := agentPolicy{Op: "sendOffer", Tool: "send_offer", RecordType: recordTypeOffer}
	got := restSummary(pol, summaryRequest("POST", "/v1/offers/018f2a10-0000-7000-8000-00000000beef/send"), nil)
	if !strings.Contains(got, "Send offer") {
		t.Errorf("summary %q does not name the act", got)
	}
	if strings.Contains(got, ":") && strings.HasSuffix(got, ":") {
		t.Errorf("summary %q trails an empty field list", got)
	}
}

// A wide patch is summarized, not dumped: the row stays readable and
// proposed_change still carries the whole envelope.
func TestRestSummaryBoundsAWidePatch(t *testing.T) {
	var b strings.Builder
	b.WriteString("{")
	for i := range 30 {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`"field`)
		b.WriteString(string(rune('a' + i%26)))
		b.WriteString(string(rune('0' + i/26)))
		b.WriteString(`":1`)
	}
	b.WriteString("}")

	got := restSummary(agentPolicy{Op: "updatePerson", Tool: "update_record", RecordType: recordTypePerson}, summaryRequest("PATCH", "/v1/people/x"), []byte(b.String()))
	if !strings.Contains(got, "more") {
		t.Errorf("a 30-field patch was not bounded: %q", got)
	}
	if strings.Count(got, "=") > summaryFieldLimit {
		t.Errorf("summary enumerates %d fields, want at most %d", strings.Count(got, "="), summaryFieldLimit)
	}
}

// A single long value cannot crowd out the fields after it.
func TestRestSummaryBoundsOneLongValue(t *testing.T) {
	got := restSummary(agentPolicy{Op: "updatePerson", Tool: "update_record", RecordType: recordTypePerson}, summaryRequest("PATCH", "/v1/people/x"),
		[]byte(`{"notes":"`+strings.Repeat("a", 500)+`","title":"CEO"}`))
	if !strings.Contains(got, "title=CEO") {
		t.Errorf("a long value crowded out the field after it: %q", got)
	}
	if len(got) > 400 {
		t.Errorf("summary is %d bytes; one value should not dominate it", len(got))
	}
}

// Nested structure is named and counted rather than expanded — the summary
// answers "what shape", the envelope answers "what exactly".
func TestRestSummaryCountsNestedStructure(t *testing.T) {
	got := restSummary(agentPolicy{Op: "createOffer", Tool: "create_record", RecordType: recordTypeOffer}, summaryRequest("POST", "/v1/deals/x/offers"),
		[]byte(`{"currency":"EUR","line_items":[{"description":"Pilot"},{"description":"Support"}]}`))
	if !strings.Contains(got, "line_items=[2]") {
		t.Errorf("summary %q does not count the nested line items", got)
	}
}

// Clearing a field and setting it empty are different changes, and the
// approving human has to be able to tell them apart. Unmarshaling JSON null
// into a plain string succeeds and leaves it empty, so a naive string probe
// renders `owner_id=` for both — which reads like nothing is happening on the
// one that hands the record to nobody.
func TestRestSummaryDistinguishesNullFromEmpty(t *testing.T) {
	cleared := restSummary(agentPolicy{Op: "updateDeal", Tool: "update_record", RecordType: recordTypeDeal}, summaryRequest("PATCH", "/v1/deals/x"), []byte(`{"owner_id":null}`))
	if !strings.Contains(cleared, "owner_id=null") {
		t.Errorf("summary %q does not show that owner_id is being CLEARED", cleared)
	}
	emptied := restSummary(agentPolicy{Op: "updateDeal", Tool: "update_record", RecordType: recordTypeDeal}, summaryRequest("PATCH", "/v1/deals/x"), []byte(`{"owner_id":""}`))
	if strings.Contains(emptied, "owner_id=null") {
		t.Errorf("summary %q reports an empty string as a clear", emptied)
	}
	if cleared == emptied {
		t.Error("clearing a field and emptying it render identically")
	}
}

// Every verb that can actually reach a decision card reads as a sentence.
//
// The corpus is derived rather than listed: a tool is stageable only when the
// contract gives it a tier other than auto_execute, and it is exactly those
// nine pairs whose summary a human ever sees. A verb added to that set with a
// name that does not carry its own record — and no entry in genericVerbs —
// would put "Do thing" on a card and drop which record it was about.
func TestEveryStageableVerbNamesWhatItActsOn(t *testing.T) {
	for key, pol := range agentPolicies {
		if pol.Access != accessTool || pol.Tier == tierAutoExecute {
			continue
		}
		phrase := actPhrase(pol, "POST", "/v1/x")
		if phrase == "" {
			t.Errorf("%s: staged calls render an empty headline", key)
			continue
		}
		// Either the verb names its own object, or the record type is appended.
		namesRecord := strings.Contains(phrase, recordNoun(pol.RecordType))
		verbCarriesIt := pol.RecordType == "" ||
			strings.Contains(strings.ReplaceAll(pol.Tool, "_", " "), string(pol.RecordType))
		if !namesRecord && !verbCarriesIt && genericVerbs[pol.Tool] {
			t.Errorf("%s: %q names neither the verb's object nor its record type", key, phrase)
		}
		if strings.Contains(phrase, "_") {
			t.Errorf("%s: %q still carries a wire identifier", key, phrase)
		}
	}
}
