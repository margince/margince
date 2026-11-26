// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind shape H1

package gates

// A record's source channel is spelled three times: provenance.RecordSources(),
// the table's CHECK and the contract's enum. A value in Go and not in the CHECK
// fails the write at run time; one in the contract and not in Go is refused 422.

import (
	"bufio"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// recordSourceHeld maps a table whose source is held to the list onto the
// create request that carries it. A partner has no source on the wire.
//
// gatekit:fixture the held tables and the request each one's source arrives on
var recordSourceHeld = map[string]string{
	"company": "CreateCompanyRequest",
	"contact": "CreateContactRequest",
	"deal":    "CreateDealRequest",
	"lead":    "CreateLeadRequest",
	"offer":   "CreateOfferRequest",
	"partner": "",
	"project": "CreateProjectRequest",
	"signal":  "CreateSignalRequest",
}

// recordSourceFree names each create request whose source is still free text,
// with the reason. A request that gains the enum moves to recordSourceHeld.
var recordSourceFree = gatekit.Waive(map[string]string{
	"CreateActivityRequest":     "an activity's source also names the code that wrote the row, and readers select on those words",
	"CreateTaskRequest":         "a task is an activity and shares its source",
	"CreateRelationshipRequest": "a relationship's source cites the record or mail the edge was read from",
	"CreateCustomFieldRequest":  "a custom field is configuration, not a record that arrives by a channel",
	"CreateProductRequest":      "a product is catalog data, not a record that arrives by a channel",
	"CreateDealRoomRequest":     "a deal room's source names the surface or agent that opened it",
})

// wantMinimumSourceRequests is a floor under the create requests the contract
// scan finds, so a scan that stops reading the contract cannot pass.
const wantMinimumSourceRequests = 10

var (
	recordSourceCheckRE = regexp.MustCompile(
		`^public\.([a-z_]+)\.([a-z_]+)_source_check CHECK \(\(source = ANY \(ARRAY\[([^\]]*)\]\)\)\)`)
	recordSourceTriggerRE = regexp.MustCompile(
		`^public\.([a-z_]+)\.([a-z_]+)_source_channel CREATE TRIGGER .*new\.source <> ALL \(ARRAY\[([^\]]*)\]`)
	recordSourceFunctionRE = regexp.MustCompile(`(?m)^ +WHEN source IN \(([^)]*)\) THEN source$`)
	contractSchemaRE       = regexp.MustCompile(`^    ([A-Za-z0-9]+):$`)
	contractSourceRE       = regexp.MustCompile(`^        source:(?: \{ type: string, enum: \[([^\]]*)\])?`)
)

// recordSourceLists reads every spelling of the list the head catalog holds:
// each table's check, each table's update trigger and the mapping function.
func recordSourceLists(catalog string) (checks, triggers map[string][]string, mapping []string) {
	checks, triggers = map[string][]string{}, map[string][]string{}
	for line := range strings.SplitSeq(catalog, "\n") {
		if match := recordSourceCheckRE.FindStringSubmatch(line); match != nil && match[1] == match[2] {
			checks[match[1]] = splitCatalogTokens(match[3])
		}
		if match := recordSourceTriggerRE.FindStringSubmatch(line); match != nil && match[1] == match[2] {
			triggers[match[1]] = splitCatalogTokens(match[3])
		}
	}
	if match := recordSourceFunctionRE.FindStringSubmatch(catalog); match != nil {
		mapping = splitCatalogTokens(match[1])
	}
	return checks, triggers, mapping
}

// contractSourceRequests reads each create request that has a source property,
// with its enum; a free-text source has none.
func contractSourceRequests(contract string) map[string][]string {
	requests := map[string][]string{}
	schema := ""
	scanner := bufio.NewScanner(strings.NewReader(contract))
	scanner.Buffer(make([]byte, 0, 1<<20), 1<<24)
	for scanner.Scan() {
		line := scanner.Text()
		if match := contractSchemaRE.FindStringSubmatch(line); match != nil {
			schema = match[1]
			continue
		}
		match := contractSourceRE.FindStringSubmatch(line)
		if match == nil || !strings.HasPrefix(schema, "Create") || !strings.HasSuffix(schema, "Request") {
			continue
		}
		var values []string
		for value := range strings.SplitSeq(match[1], ",") {
			if value = strings.TrimSpace(value); value != "" {
				values = append(values, value)
			}
		}
		slices.Sort(values)
		requests[schema] = values
	}
	return requests
}

// recordSourceFindings names each place the list is spelled differently. It
// also names each table that joined the list unheld, and each create request
// the gate does not rule on.
func recordSourceFindings(vocabulary []string, checks, triggers, requests map[string][]string) []string {
	free := recordSourceFree.Reasons()
	var findings []string
	held := map[string]bool{}
	for table, request := range recordSourceHeld {
		check, ok := checks[table]
		switch {
		case !ok:
			findings = append(findings, table+": no "+table+"_source_check in the head catalog")
		case !equalSets(vocabulary, check):
			findings = append(findings, table+": the CHECK allows "+strings.Join(check, ", "))
		}
		if trigger, ok := triggers[table]; ok && !equalSets(vocabulary, trigger) {
			findings = append(findings, table+": the update trigger lists "+strings.Join(trigger, ", "))
		}
		if request == "" {
			continue
		}
		held[request] = true
		if !equalSets(vocabulary, requests[request]) {
			findings = append(findings, request+": the contract's source enum is ["+strings.Join(requests[request], ", ")+"]")
		}
	}
	// Other tables check a source against words of their own; one that
	// spells this list has joined it.
	for table, check := range checks {
		if _, ok := recordSourceHeld[table]; !ok && equalSets(vocabulary, check) {
			findings = append(findings, table+": checks its source against this list, so add it to recordSourceHeld with its create request")
		}
	}
	for request := range requests {
		if _, isFree := free[request]; !held[request] && !isFree {
			findings = append(findings, request+": carries a source this gate does not rule on; hold it to the list or name it free")
		}
	}
	for request := range free {
		if enum, ok := requests[request]; !ok || len(enum) > 0 || held[request] {
			findings = append(findings, request+": is named free, and its source is gone, held or an enum")
		}
	}
	slices.Sort(findings)
	return findings
}

func TestTheRecordSourceVocabularyIsTheCatalogsCheck(t *testing.T) {
	t.Parallel()
	catalog, err := gatekit.ReadFile(replyVerdictCatalog)
	if err != nil {
		t.Fatalf("reading the head catalog: %v", err)
	}
	contract, err := gatekit.ReadFile("api/crm.yaml")
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	requests := contractSourceRequests(string(contract))
	if len(requests) < wantMinimumSourceRequests {
		t.Fatalf("the contract scan found %d create requests with a source, under the floor of %d", len(requests), wantMinimumSourceRequests)
	}
	defer recordSourceFree.AssertAllMatched(t)
	for request, enum := range requests {
		if _, isFree := recordSourceFree.Reasons()[request]; isFree && len(enum) == 0 {
			recordSourceFree.Waived(t, request)
		}
	}
	vocabulary := provenance.RecordSources()
	checks, triggers, mapping := recordSourceLists(string(catalog))
	if !equalSets(vocabulary, mapping) {
		t.Errorf("record_source_channel passes %v through unchanged; provenance.RecordSources() spells %v", mapping, vocabulary)
	}
	for _, finding := range recordSourceFindings(vocabulary, checks, triggers, requests) {
		t.Errorf("%s; provenance.RecordSources() spells %v", finding, vocabulary)
	}
}

func TestTheRecordSourceGateSeesEachWayTheListCanDrift(t *testing.T) {
	t.Parallel()
	vocabulary := []string{"api", "manual"}
	agreeing := func() (checks, triggers, requests map[string][]string) {
		checks, triggers, requests = map[string][]string{}, map[string][]string{}, map[string][]string{}
		for table, request := range recordSourceHeld {
			checks[table], triggers[table] = vocabulary, vocabulary
			if request != "" {
				requests[request] = vocabulary
			}
		}
		for request := range recordSourceFree.Reasons() {
			requests[request] = nil
		}
		return checks, triggers, requests
	}
	checks, triggers, requests := agreeing()
	if findings := recordSourceFindings(vocabulary, checks, triggers, requests); len(findings) != 0 {
		t.Fatalf("an agreeing tree was reported: %v", findings)
	}
	for name, drift := range map[string]func(checks, triggers, requests map[string][]string){
		"a check with one more value":     func(checks, _, _ map[string][]string) { checks["deal"] = []string{"api", "manual", "seed"} },
		"a held table with no check":      func(checks, _, _ map[string][]string) { delete(checks, "partner") },
		"a trigger that is shorter":       func(_, triggers, _ map[string][]string) { triggers["offer"] = []string{"manual"} },
		"a contract enum that is shorter": func(_, _, requests map[string][]string) { requests["CreateOfferRequest"] = []string{"manual"} },
		"a held request with no enum":     func(_, _, requests map[string][]string) { requests["CreateProjectRequest"] = nil },
		"a check on a table not held":     func(checks, _, _ map[string][]string) { checks["activity"] = vocabulary },
		"a new request with a source":     func(_, _, requests map[string][]string) { requests["CreateTicketRequest"] = nil },
		"a free request that gained an enum": func(_, _, requests map[string][]string) {
			requests["CreateActivityRequest"] = vocabulary
		},
		"a free request that lost its source": func(_, _, requests map[string][]string) { delete(requests, "CreateProductRequest") },
	} {
		checks, triggers, requests := agreeing()
		drift(checks, triggers, requests)
		if findings := recordSourceFindings(vocabulary, checks, triggers, requests); len(findings) != 1 {
			t.Errorf("%s: got findings %v, want one", name, findings)
		}
	}
}

func TestTheRecordSourceReadersParseTheCommittedSpellings(t *testing.T) {
	t.Parallel()
	checks, triggers, mapping := recordSourceLists(strings.Join([]string{
		`public.deal.deal_source_check CHECK ((source = ANY (ARRAY['api'::text, 'manual'::text]))) NOT VALID`,
		`public.partner.partner_source_check CHECK ((source = ANY (ARRAY['manual'::text])))`,
		`public.deal.offer_source_check CHECK ((source = ANY (ARRAY['manual'::text])))`,
		`public.signal.signal_source_channel_check CHECK ((source_channel = ANY (ARRAY['derived'::text])))`,
		`public.deal.deal_source_channel CREATE TRIGGER deal_source_channel BEFORE UPDATE ON public.deal FOR EACH ROW WHEN ((new.source <> ALL (ARRAY['api'::text, 'manual'::text]))) EXECUTE FUNCTION move_source_onto_its_channel()`,
		`public.record_source_channel(source text) secdef=false cfg=- acl=- `,
		` SELECT CASE`,
		`  WHEN source IN ('api', 'manual') THEN source`,
		`  WHEN source IN ('mcp', 'ui') THEN 'manual'`,
	}, "\n"))
	if len(checks) != 2 || !equalSets(checks["deal"], []string{"api", "manual"}) || !equalSets(checks["partner"], []string{"manual"}) {
		t.Errorf("checks = %v, want deal and partner only", checks)
	}
	if len(triggers) != 1 || !equalSets(triggers["deal"], []string{"api", "manual"}) {
		t.Errorf("triggers = %v, want deal only", triggers)
	}
	if !equalSets(mapping, []string{"api", "manual"}) {
		t.Errorf("mapping = %v, want the pass-through list", mapping)
	}
	requests := contractSourceRequests(strings.Join([]string{
		"    CreateDealRequest:",
		"        source: { type: string, enum: [manual, api], x-go-type: string }",
		"    CreateLeadRequest:",
		"        source: { type: string }",
		"    Deal:",
		"        source: { type: string }",
		"    CreateNoteRequest:",
		"        source_system: { type: string }",
	}, "\n"))
	if len(requests) != 2 || !equalSets(requests["CreateDealRequest"], []string{"api", "manual"}) || len(requests["CreateLeadRequest"]) != 0 {
		t.Errorf("requests = %v, want the deal enum and a free lead source", requests)
	}
}
