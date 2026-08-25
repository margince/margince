// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The inbox row is what a triaging approver acts on, and for the whole REST
// admission surface it used to say only which verb hit which path. These
// cases pin what a human can now see without opening the envelope: the
// operation, and the values the call would actually write.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// summaryRequest is the request shape restSummary reads. `params` are the
// routed values in the order the pattern declares them — chi keeps that order,
// and a nested create reads the LAST of them.
func summaryRequest(method, path string, params ...[2]string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	rctx := chi.NewRouteContext()
	for _, p := range params {
		rctx.URLParams.Add(p[0], p[1])
	}
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

// A nested create names the record it hangs off, and it is the INNERMOST one.
//
// A deal-room comment is posted to /deal-rooms/{id}/threads/{threadId}/comments
// and belongs to the thread. Naming `{id}` would tell a reader which
// conversation it was in and not which exchange — two comments with the same
// body on different threads in one room would read identically. The first
// version of this did exactly that, and no test caught it because none of them
// installed a route context at all.
func TestANestedCreateNamesTheRecordItHangsOff(t *testing.T) {
	pol := agentPolicy{Op: "replyDealRoomThread", Tool: "create_record", RecordType: recordTypeDealRoomComment}
	req := summaryRequest("POST", "/v1/deal-rooms/room-1/threads/thread-9/comments",
		[2]string{"id", "room-1"}, [2]string{"threadId", "thread-9"})

	got := restSummary(pol, req, []byte(`{"body":"Sounds good"}`))
	if !strings.Contains(got, "under=thread-9") {
		t.Errorf("summary %q does not name the thread the comment attaches to", got)
	}
	if strings.Contains(got, "under=room-1") {
		t.Errorf("summary %q names the room, which does not identify the exchange", got)
	}
}

// A flat create has no parent, and inventing one would be worse than silence.
func TestAFlatCreateNamesNoParent(t *testing.T) {
	pol := agentPolicy{Op: "createCustomField", Tool: "create_record", RecordType: recordTypeCustomField}
	got := restSummary(pol, summaryRequest("POST", "/v1/custom-fields"), []byte(`{"key":"industry"}`))
	if strings.Contains(got, "under=") {
		t.Errorf("summary %q claims a parent a top-level create does not have", got)
	}
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

// No two acts a human can be asked to decide share a headline.
//
// This is the property the whole change exists for. A verb often covers more
// than one operation — `enrich` is both a one-page read and a whole-site crawl,
// `update_record` on a custom field is both retiring it and changing what it
// offers — and where two of those are BOTH confirm-first, one headline for two
// acts asks somebody to approve a thing they cannot identify.
//
// Derived from the policy table rather than listed, so a contract change that
// creates a new collision fails here instead of reaching a card.
func TestNoTwoStageableActsShareAHeadline(t *testing.T) {
	byPhrase := map[string][]string{}
	// A verb covering several stageable operations must name each of them, or
	// the ones it does not name collapse together. Checking only for identical
	// phrases is not enough: naming ONE of a colliding pair separates them
	// while leaving the other reading "Enrich organization", which still does
	// not say which enrichment a reader is approving.
	byVerb := map[string]map[string]bool{}
	for _, pol := range agentPolicies {
		if pol.Access != accessTool || pol.Tier == tierAutoExecute {
			continue
		}
		phrase := actPhrase(pol, "POST", "/v1/x")
		byPhrase[phrase] = append(byPhrase[phrase], pol.Op)
		// Keyed by verb AND record, because the generic verbs already separate
		// two acts that differ only by what they act on: "Create record custom
		// field" and "Create record webhook subscription" are distinct without
		// naming either operation. What needs naming is two acts the headline
		// cannot separate at all.
		act := pol.Tool + "/" + string(pol.RecordType)
		if byVerb[act] == nil {
			byVerb[act] = map[string]bool{}
		}
		byVerb[act][pol.Op] = true
	}
	for phrase, ops := range byPhrase {
		if len(ops) > 1 {
			sort.Strings(ops)
			t.Errorf("%q is the headline for %v — a reader cannot tell which they are approving", phrase, ops)
		}
	}
	for act, ops := range byVerb {
		if len(ops) < 2 {
			continue
		}
		for op := range ops {
			if _, named := opPhrases[op]; !named {
				t.Errorf("%s covers %d stageable acts and %q is not named in opPhrases — its headline cannot identify it", act, len(ops), op)
			}
		}
	}
}

// Every headline a human can meet is words, never a wire identifier.
func TestEveryStageableActReadsAsWords(t *testing.T) {
	for key, pol := range agentPolicies {
		if pol.Access != accessTool || pol.Tier == tierAutoExecute {
			continue
		}
		phrase := actPhrase(pol, "POST", "/v1/x")
		if phrase == "" {
			t.Errorf("%s: staged calls render an empty headline", key)
			continue
		}
		if strings.Contains(phrase, "_") {
			t.Errorf("%s: %q still carries a wire identifier", key, phrase)
		}
	}
}
