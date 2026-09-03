// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What the human sees before they decide.
//
// The approval row's summary is what an inbox renders and what a triaging
// approver acts on. For every action staged through the REST admission gate
// it used to be the method and the path — "Agent REST POST
// /v1/activities/9f2c…/send-email" — with no recipient, no amount, no field
// value anywhere in it. The content existed only in proposed_change, an
// untyped map the surface hands back raw, so the decision a human was asked
// to make was legible only to someone willing to read a JSON envelope.
//
// The summary is now built from the SAME bytes the diff_hash covers and the
// redemption re-checks, so the text a human reads and the call that executes
// cannot disagree. It stays structural rather than free prose: the operation
// in plain words, then the body's own fields and values. The values are
// agent-authored, so approvals.StageInTx sanitizes whatever lands here — this
// file decides WHAT to say, that one decides what may be rendered.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
)

// summaryFieldLimit bounds how many body fields a summary enumerates. A
// patch wider than this is summarized by its field names, which is still
// more than the method and path said.
const summaryFieldLimit = 8

// summaryValueLimit bounds one rendered value, so a single long string
// cannot crowd out the fields after it.
const summaryValueLimit = 48

// restSummary describes the staged call: what it does to what, plus the
// request body's own top-level fields. A body-less action route (send this
// offer, archive this record) names the act alone — which is the whole change,
// and says so.
//
// The head used to be the operationId and the concrete path —
// "updateDeal (PATCH /v1/deals/018f2a10-0000-7000-8000-00000000000a)" — which
// put a camelCase identifier and a uuid at the top of a card somebody had to
// decide. Neither told them anything: the uuid names a record they cannot read
// from it, and the operationId is the contract's word, not theirs. The tool
// verb and the record type are already on the policy, and together they are the
// sentence.
func restSummary(pol agentPolicy, r *http.Request, body []byte) string {
	head := actPhrase(pol, r.Method, r.URL.Path)
	fields := summaryFields(body)
	// A CREATE is routed by its parent — createOffer posts under the deal the
	// offer would belong to — and the parent appears nowhere in the body. The
	// old head carried it by accident, inside the path; naming it is what keeps
	// an approver able to tell which deal they are pricing.
	if pol.Tool == toolCreateRecord {
		if parent := createdUnder(r); parent != "" {
			fields = append([]string{"under=" + parent}, fields...)
		}
	}
	if len(fields) == 0 {
		return head
	}
	return head + ": " + strings.Join(fields, ", ")
}

// createdUnder is the record a nested create hangs off.
//
// The INNERMOST routed id, not `{id}`: a deal-room comment is posted to
// /deal-rooms/{id}/threads/{threadId}/comments and belongs to the THREAD, so
// naming the room would tell a reader which conversation it was in and not
// which exchange. A flat create names no parent and returns empty.
func createdUnder(r *http.Request) string {
	rctx := chi.RouteContext(r.Context())
	if rctx == nil {
		return ""
	}
	// chi appends params in the order the pattern declares them, so the last
	// one is the deepest.
	keys := rctx.URLParams.Keys
	for i := len(keys) - 1; i >= 0; i-- {
		if value := rctx.URLParams.Values[i]; value != "" {
			return value
		}
	}
	return ""
}

// actPhrase names the act in words: "Update a deal", "Send an email".
//
// Falls back to the operation and its path when the policy declares no tool —
// a route with no verb has nothing better to say, and the old shape is at least
// unambiguous to whoever has to debug it.
func actPhrase(pol agentPolicy, method, path string) string {
	if pol.Tool == "" {
		return fmt.Sprintf("%s (%s %s)", pol.Op, method, path)
	}
	// Some verbs cover two acts a reader must tell apart. `enrich` is both a
	// one-page read and a whole-site crawl; `update_record` on a custom field
	// is both retiring the field and changing what it offers. The verb cannot
	// distinguish them — only the operation can, so where one is named here it
	// wins.
	if phrase, named := opPhrases[pol.Op]; named {
		return phrase
	}
	verb := strings.ReplaceAll(pol.Tool, "_", " ")
	// Only the GENERIC verbs need the record type to mean anything: "update
	// record" says nothing until it says which. Every other verb already names
	// its own object — "send email" told against `activity` would read as
	// "Send email: activity", which is worse than the verb alone.
	if pol.RecordType == "" || !genericVerbs[pol.Tool] {
		return upperFirst(verb)
	}
	return fmt.Sprintf("%s %s", upperFirst(verb), recordNoun(pol.RecordType))
}

// recordNoun is the record type as a reader says it: the wire spells
// `deal_room`, a person says "deal room".
func recordNoun(record agentRecordType) string {
	return strings.ReplaceAll(string(record), "_", " ")
}

// opPhrases names the acts whose VERB is not enough to tell them apart.
//
// Deliberately keyed by operation rather than by route: the operation is what
// the contract declares and what the policy carries, and a route pattern would
// be a second spelling of it. Held by TestNoTwoStageableActsShareAHeadline,
// which fails when a new pair collides.
var opPhrases = map[string]string{
	opScrapeCompany:            "Read this company's website",
	opDeepReadCompany:          "Read this company's whole site",
	opTechnicalEnrichCompany:   "Look up what this company publicly runs",
	opRetireCustomField:        "Retire a custom field",
	opUpdateCustomFieldOptions: "Change a custom field's options",
	// "Merge tags" alone reads as reversible, which this is not, and the head is
	// the half a triaging approver reads first. The routed tag — the word being
	// RETIRED — appears nowhere in the body, so unlike the tool door's sentence
	// this summary cannot name it: the head says what the act costs instead.
	//
	// It is not replaced by the resolver's prose, deliberately. This summary is
	// built from the same bytes diff_hash covers and redemption re-checks, so
	// the text a human reads and the call that runs cannot disagree; prose
	// resolved from a separate read would break that.
	opMergeTags: "Fold one tag into another and release its name",
}

// The operations whose headline this file names.
//
// A typo here leaves an act sharing a headline with its sibling, which is the
// exact defect opPhrases exists to prevent — so the names are checked against
// the generated policy table rather than trusted.
// Held by: TestNoTwoStageableActsShareAHeadline (this package).
const (
	opScrapeCompany            = "scrapeCompany"
	opDeepReadCompany          = "deepReadCompany"
	opTechnicalEnrichCompany   = "technicalEnrichCompany"
	opRetireCustomField        = "retireCustomField"
	opUpdateCustomFieldOptions = "updateCustomFieldOptions"
	opMergeTags                = "mergeTags"
)

// genericVerbs are the tools whose name carries no record: they act on
// whatever the route points at, so the record type is the other half of what
// they do rather than a repetition of it.
var genericVerbs = map[string]bool{
	toolUpdateRecord:  true,
	toolCreateRecord:  true,
	toolArchiveRecord: true,
	toolMergeRecords:  true,
}

// upperFirst capitalises the phrase, which is a sentence rather than a label.
func upperFirst(phrase string) string {
	if phrase == "" {
		return phrase
	}
	return strings.ToUpper(phrase[:1]) + phrase[1:]
}

// summaryFields renders the body's top-level entries as key=value, sorted so
// two renderings of the same call read the same way. A nested object or
// array is named and counted rather than expanded: the inbox is a summary,
// and proposed_change carries the whole envelope for anyone who wants it.
func summaryFields(body []byte) []string {
	var payload map[string]json.RawMessage
	if len(strings.TrimSpace(string(body))) == 0 || json.Unmarshal(body, &payload) != nil {
		return nil
	}
	keys := make([]string, 0, len(payload))
	for key := range payload {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	rendered := make([]string, 0, len(keys))
	for _, key := range keys {
		if len(rendered) == summaryFieldLimit {
			rendered = append(rendered, fmt.Sprintf("+%d more", len(keys)-summaryFieldLimit))
			break
		}
		rendered = append(rendered, key+"="+summaryValue(payload[key]))
	}
	return rendered
}

// summaryValue renders one JSON value for a human. Strings lose their quotes
// (the reader wants the name, not its encoding) and everything is bounded.
func summaryValue(raw json.RawMessage) string {
	// null is recognized BEFORE the string probe, because unmarshaling null
	// into a plain string SUCCEEDS and leaves it empty (encoding/json: null
	// into a non-nullable type "has no effect and produces no error"). Left to
	// the string branch, clearing a field would render exactly like setting it
	// to "" — and clearing an owner or a close date is precisely the change the
	// approving human most needs to see.
	if strings.TrimSpace(string(raw)) == "null" {
		return "null"
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return truncateValue(s)
	}
	var arr []json.RawMessage
	if json.Unmarshal(raw, &arr) == nil {
		return fmt.Sprintf("[%d]", len(arr))
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) == nil {
		return fmt.Sprintf("{%d fields}", len(obj))
	}
	return truncateValue(string(raw)) // numbers, booleans, null
}

func truncateValue(s string) string {
	if len(s) <= summaryValueLimit {
		return s
	}
	cut := summaryValueLimit
	for cut > 0 && !isRuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}

// isRuneStart reports whether b begins a UTF-8 rune (a continuation byte is
// 10xxxxxx). Cutting mid-rune would put an invalid sequence in front of a
// human, which the summary sanitizer would then drop as noise.
func isRuneStart(b byte) bool { return b&0xC0 != 0x80 }
