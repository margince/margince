// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The contract does not describe the agent tool surface — it IS the agent tool
// surface. An operation carrying `x-mcp-tool` promises a client that a governed
// tool of that verb exists, and `tools/list` and `GET /v1/agent-tools` publish
// that promise. So a declared verb with no registered tool is not a gap to
// document; it is the contract saying something untrue.
//
// This is the MCP twin of the REST guarantee: `var _ crmcontracts.ServerInterface
// = Server{}` (server.go) makes a declared operation with no handler a compile
// error. Here it is a failing gate rather than a compile error only because a
// registry is populated at runtime, not by an interface.
//
// There is deliberately NO waiver map. A verb that cannot honestly have a tool
// has the wrong annotation, and the fix is `x-agent-access: human-only` in
// api/crm.yaml — which is a statement about authority, not an exemption from
// one. That is why this gate can be absolute where the old pinned backlog could
// not: the escape hatch lives in the contract, where a reviewer sees it.

import (
	"context"
	"encoding/json"
	"sort"
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
)

func TestEveryDeclaredToolVerbIsRegistered(t *testing.T) {
	registry := NewRegistry(nil, SendPath{})

	// route by verb, so the failure names where the reader has to go.
	routes := map[string]string{}
	for route, pol := range agentPolicies {
		if pol.Access != accessTool {
			continue
		}
		if _, registered := registry.Spec(pol.Tool); registered {
			continue
		}
		routes[pol.Tool] = route
	}

	verbs := make([]string, 0, len(routes))
	for verb := range routes {
		verbs = append(verbs, verb)
	}
	sort.Strings(verbs)
	for _, verb := range verbs {
		t.Errorf("%s (%s) declares x-mcp-tool but no tool is registered for it, so the contract "+
			"advertises a verb tools/list cannot offer. Register it, or — if no tool can honestly "+
			"exist for it — give the operation x-agent-access: human-only in api/crm.yaml.",
			verb, routes[verb])
	}
}

// The gate above proves a tool EXISTS for every declared verb. This one proves
// the surface has no tool the contract never declared: an agent could call it,
// and no operation says an agent may.
//
// Both directions, because either alone is satisfied by a surface that is wrong
// in the other. Registry-only tools are legitimate for the §2.2 intents, which
// compose over contract operations rather than backing one, so those are named
// by the verbs the policy table cannot see.
//
// A composed EXTENSION tool is the third legitimate case, and it is declared —
// just not here. Its authority comes from its unit's manifest (ADR-0120), which
// is why an installation can add a verb without editing the contract, and the
// composed set is what a reviewer reads instead of the policy table. Skipping it
// by name rather than by "not in the table" keeps the sweep absolute for
// everything else.
func TestEveryRegisteredToolIsDeclaredOrAnIntent(t *testing.T) {
	declared := map[string]bool{}
	for _, pol := range agentPolicies {
		if pol.Access == accessTool {
			declared[pol.Tool] = true
		}
	}

	specs := NewRegistry(nil, SendPath{}).Specs()
	if len(specs) == 0 {
		t.Fatal("the registry has no tools — this sweep checked nothing")
	}
	composed := composedToolNames()
	for _, spec := range specs {
		if declared[spec.Name] || composedIntents[spec.Name] || composed[spec.Name] {
			continue
		}
		t.Errorf("%s is registered but no operation declares it, so an agent may call a verb the "+
			"contract never granted. Declare the backing operation's x-mcp-tool, or add it to "+
			"composedIntents with the operations it composes over.", spec.Name)
	}
}

// TestTheSweepSkipsExactlyWhatTheComposedSetRegisters: an extension tool reaches
// the registry through the same NewRegistry the sweep above builds, so without
// the composed-set skip an installation with one unit would fail a gate about
// the CONTRACT — and be told to declare an `x-mcp-tool` that would be wrong for
// it. The skip has to cover exactly what registration adds, no more.
func TestTheSweepSkipsExactlyWhatTheComposedSetRegisters(t *testing.T) {
	before := composedToolNames()
	if before["yogi_quote"] {
		t.Fatal("the composed set leaked in from another test — this one would prove nothing")
	}
	tools, err := buildExtensionTools([]extension.Extension{{
		Name: "demo", Version: "1.0.0",
		Tools: []extension.Tool{{
			Name:   "yogi_quote",
			Handle: func(context.Context, extension.Runtime, json.RawMessage) (json.RawMessage, error) { return nil, nil },
		}},
	}}, []extension.Verb{unitVerb("demo", "yogi_quote", extension.TierAutoExecute, extension.ScopeRead)})
	if err != nil {
		t.Fatal(err)
	}
	setComposedTools(tools)
	t.Cleanup(func() { setComposedTools(nil) })

	if !composedToolNames()["yogi_quote"] {
		t.Fatal("a served extension tool is missing from the skip set the sweep consults")
	}
	registered := false
	for _, spec := range NewRegistry(nil, SendPath{}).Specs() {
		if spec.Name == "yogi_quote" {
			registered = true
		}
	}
	if !registered {
		t.Fatal("the composed tool never reached the registry — the skip would be covering nothing")
	}
}

// composedIntents are the §2.2 tools that answer a question by composing several
// contract operations rather than backing one, so no single `x-mcp-tool`
// declares them. Some of them write — `qualify_lead` fills gap-only fields,
// `progress_deal` moves a deal and notes it — which §2.2 sanctions because the
// writes go through the same provider seam the declared CRUD verbs use.
// TestComposedIntentsNeverEgress holds the line that actually matters.
var composedIntents = map[string]bool{
	"catch_me_up_on":           true,
	"prep_for_meeting":         true,
	"who_knows":                true,
	"account_coverage":         true,
	"intro_path_to":            true,
	"at_risk_relationships":    true,
	"whats_slipping_this_week": true,
	"draft_follow_ups_for":     true,
	// whoami names the human this passport acts for. /v1/me is human-only —
	// correctly, since it is a session's own view — so there is no REST
	// operation to twin, and this reads a principal rather than a record.
	"whoami": true,
	// review_commitments reads the timeline for a set `GET /activities` cannot
	// select: open tasks ordered by when they came DUE, which that operation
	// neither filters on nor sorts by. Read-only.
	"review_commitments": true,
	// prepare_handoff composes the project read with the deals rolled up to it,
	// the people attached to it and the promises outstanding on it — four
	// operations, so no single one declares it. Read-only, and it writes
	// nothing: moving work into delivery is advance_project_phase's act.
	"prepare_handoff": true,
	"list_pipelines":  true,
	"qualify_lead":    true,
	"progress_deal":   true,
	"run_report":      true,
	// query_workspace composes over the same list operations search_records
	// does, but no single one of them can declare it: a plan chooses its target
	// at call time, and the records it selects are read back through the
	// datasource seam. It is read-only and reaches nothing outside the
	// workspace, which is what TestComposedIntentsNeverEgress holds it to.
	"query_workspace": true,
	// describe_query_vocabulary answers the document margince://schema/query
	// publishes, for a client that reads TOOLS and not resources. It backs no
	// REST operation at all: the vocabulary is composed at call time from the
	// field catalog and the live column catalog, narrowed to what this
	// principal may already read. Read-only, it returns no records, and it
	// names nothing a caller could not reach by asking.
	"describe_query_vocabulary": true,
	// describe_report_vocabulary answers the document margince://schema/reports
	// publishes, for a caller that reads no resources — and the Surface-B runner
	// is one, since it is offered no resource step at all. It backs no REST
	// operation: `runReport` runs a report, and there is no operation that
	// answers what a report's plan may SAY. Read-only, it returns no records,
	// and the vocabulary it names is the engine's own compile-time table, so it
	// names nothing about a workspace at all.
	"describe_report_vocabulary": true,
	// describe_report_blocks answers the document
	// margince://schema/report-blocks publishes, for the same caller and the
	// same reason: the Surface-B runner is offered no resource step. It backs
	// no REST operation either — `renderAnalyticsReport` renders a document,
	// and no operation answers what a document may CONTAIN. Read-only, it
	// returns no records, and the grammar it names is the engine's own
	// compile-time list, so it names nothing about a workspace at all.
	"describe_report_blocks": true,
	// describe_analytics_vocabulary answers the document
	// margince://schema/analytics publishes, for the same caller and the same
	// reason as the two doors above. It backs no REST operation:
	// `runAnalyticsQuery` runs a query, and no operation answers what a query
	// may SAY. Read-only, and the document is derived per caller, narrowed to
	// what this principal may already read — so it names nothing a caller
	// could not reach by asking.
	"describe_analytics_vocabulary": true,
	// describe_record_fields answers the document
	// margince://schema/record-fields publishes, for the same caller and the
	// same reason as the three doors above. It backs no REST operation:
	// `createRecord` and `updateRecord` WRITE, and no operation answers what a
	// write may NAME. It writes nothing itself, returns no records, and the
	// document is composed from the contract shapes, so it names nothing about
	// a workspace at all.
	"describe_record_fields": true,
	// search_context ranks across record types through the retrieval index,
	// which no single list operation is: `GET /search` is the lexical half
	// alone and answers no vector lane, and the records the sweep names are
	// read back through the datasource seam. Read-only.
	//
	// It does not EGRESS in the sense this file's rule is about — no record
	// leaves the workspace — but the caller's query string does reach the
	// configured embed provider, exactly as query_workspace's similarity clause
	// does and as every indexed record already did. That is the AI runtime's own
	// lane, governed by the routing config rather than by a passport scope, and
	// the rule below is about outbound authority no operation declared.
	"search_context": true,
	// resolve_entities asks the dedupe ladder a question, which is not an
	// operation at all: `/dedupe/candidates` serves the STORED review queue,
	// a different question from "who does this payload name". Read-only, and
	// every record it names is read back through the datasource seam.
	"resolve_entities": true,
	// check_location_support composes over NOTHING, which makes it the odd entry
	// in this map and worth saying rather than filing quietly. It reads no
	// record and no principal: it answers what this build ASKED its host for,
	// and the finding itself is produced in the browser by the card beside it.
	// There is no operation to declare it because there is no operation — a
	// second door onto it would be a door onto nothing.
	//
	// TEMPORARY. It answers one question per chat host, and it and its view
	// should be deleted once the matrix is filled in (see apps.GeoProbeURI).
	"check_location_support": true,
}

// An intent may write inside the workspace; it may NOT reach outside it.
//
// An internal write is bounded by the granting human's own RBAC and row scope,
// which the provider seam applies whatever composed the call. Egress is not: a
// `send` or an `enrich` leaves the workspace, and the operation that would have
// declared it is the only place a reviewer would ever see that. An intent that
// egresses is therefore outbound authority nothing declared — invisible to the
// declaration gate above precisely because no operation declares an intent.
func TestComposedIntentsNeverEgress(t *testing.T) {
	registry := NewRegistry(nil, SendPath{})

	checked := 0
	for name := range composedIntents {
		spec, registered := registry.Spec(name)
		if !registered {
			t.Errorf("%s is listed as a composed intent but is not registered; delete the entry", name)
			continue
		}
		checked++
		if spec.RequiredScope.Egresses() {
			t.Errorf("intent %s spends the outbound %q cap. It backs no contract operation, so "+
				"nothing declares that this surface may leave the workspace — give it a backing "+
				"operation with x-mcp-tool, or keep it inside.", name, spec.RequiredScope)
		}
	}
	if checked == 0 {
		t.Fatal("no composed intents resolved — this sweep asserted nothing")
	}
}

// --- the same agreement, one level down: what a caller TYPES ---------------
//
// The gates above prove the contract and the surface agree about which verbs
// exist. These prove they agree about the argument a caller must name — and
// where the contract cannot say, that siblings agree among themselves.
//
// WHY THE DRIFT GATE CANNOT REACH THIS, which is what makes it a gate here
// rather than a regenerated file. `gen-agentpolicy` lifts seven fields out of
// crm.yaml — route, operationId, access, verb, record type, tier, scope — and
// argument names are not among them, and structurally cannot be: on REST the id
// is a PATH parameter (`$ref: parameters/Id`) and the request bodies carry only
// reason, trigger and evidence. Turning `{id}` in a URL into a named JSON member
// is an act with no source in the contract, so every tool's InputSchema is a
// hand-written literal and `make drift` is vacuous about it by construction.
//
// So the obligation is derived from the one fact the contract DOES carry — which
// record type a verb targets — and the schema is read as the thing a caller is
// actually served. The defect that prompted it: three lead lifecycle verbs
// advertised two spellings of one argument, and a caller who learned the name
// from two of them was refused by the third.

// idArgByRecordType groups the declared tool surface by the record type the
// contract names for each verb, mapping verb -> the single id argument it
// requires for that record.
//
// One helper because both censuses below ask the same question of the same
// corpus and differ only in what they conclude from it — agreement across
// siblings, and the convention where there is no sibling. Two copies drifted the
// moment one of them learned to skip a shape (AGENTS.md: two writers of one
// invariant share a helper or say why they do not).
//
// Verbs naming no id, or several, are left out: a collection read has none, and
// merge_records' source and target are a pair rather than two spellings of one
// id.
func idArgByRecordType(t *testing.T) map[string]map[string]string {
	t.Helper()

	schemas := map[string]json.RawMessage{}
	for _, spec := range NewRegistry(nil, SendPath{}).Specs() {
		schemas[spec.Name] = spec.InputSchema
	}

	grouped := map[string]map[string]string{}
	for _, pol := range agentPolicies {
		if pol.Access != accessTool || pol.RecordType == "" || pol.Tool == "" {
			continue
		}
		schema, registered := schemas[pol.Tool]
		if !registered {
			// A declared verb nobody registered is a different defect, and
			// TestEveryDeclaredToolVerbIsRegistered is the gate that names it.
			continue
		}
		arg, settled := requiredRecordID(t, schema)
		if !settled {
			continue
		}
		recordType := string(pol.RecordType)
		if grouped[recordType] == nil {
			grouped[recordType] = map[string]string{}
		}
		grouped[recordType][pol.Tool] = arg
	}
	if len(grouped) == 0 {
		t.Fatal("no declared tool named an id for its record type, so both censuses below measure nothing")
	}
	return grouped
}

// TestSiblingVerbsAgreeOnTheIDTheyName refuses two spellings of one record's id.
func TestSiblingVerbsAgreeOnTheIDTheyName(t *testing.T) {
	t.Parallel()
	for recordType, verbs := range idArgByRecordType(t) {
		spellings := map[string][]string{}
		for verb, arg := range verbs {
			spellings[arg] = append(spellings[arg], verb)
		}
		if len(spellings) < 2 {
			continue
		}
		var lines []string
		for arg, named := range spellings {
			sort.Strings(named)
			lines = append(lines, "  "+arg+": "+joinVerbs(named))
		}
		sort.Strings(lines)
		t.Errorf("the %s verbs ask for that record's id by %d different names, so a caller who "+
			"learns one from a sibling is refused by the next:\n%s\n\nPick the spelling the rest "+
			"of the surface uses for a single-type verb — the record's own `<type>_id` — and make "+
			"them agree. `record_id` belongs to a verb that also takes `record_type`.",
			recordType, len(spellings), joinLines(lines))
	}
}

// notTheRecordsOwnID names the required uuid arguments that are some OTHER
// record, so the one left over is the record the verb is about.
//
// A DECLARED set of roles rather than an allowlist of accepted id names, and the
// difference is the whole correctness of this census. Matching names against
// `<type>_id`/`record_id`/`id` meant a verb that renamed its id to anything else
// produced no candidate at all and was skipped — so a lead verb changing
// `lead_id` to `case_id` would have left both censuses green while the lead
// verbs disagreed, which is the under-recognition a census must not have. Naming
// the roles that are NOT the subject inverts that: an unfamiliar id stays in and
// is compared.
//
// Each entry is a role, with why it is not the subject:
//
//   - to_stage_id, into_tag_id — the DESTINATION of a move or a merge.
//   - source_id, target_id — merge_records' pair. Neither is "the record"; the
//     call is about both, which is why it names neither generically.
//   - entity_id — what relink_* attaches activities TO, not the activity.
//   - from, to — forecast_movement's two periods.
//   - approval_id — the staged call being redeemed, not the record it touches.
//
// A DECLARED FIXTURE rather than a waiver, and the distinction is the one
// gatekit draws. A waiver is a ratified COST asked about an offender, so it
// decays when the offence goes; this map is asked BEFORE the subject is known —
// a pre-filter — which is precisely the shape gatekit.Waived's own doc refuses,
// because a pre-filter discards the findings behind it. What it holds is
// expected data about the surface: which argument names are structurally some
// other record.
//
// gatekit:fixture the required uuid argument names that are another record
// rather than the one a verb is about, each with why it is not the subject
var notTheRecordsOwnID = map[string]string{
	"to_stage_id":  "the destination of a stage move",
	"into_tag_id":  "the tag being merged into",
	"source_id":    "one half of a merge pair",
	"target_id":    "the other half of a merge pair",
	"entity_id":    "what activities are relinked onto",
	"from":         "the period a movement starts in",
	"to":           "the period a movement ends in",
	"approval_id":  "the staged call being redeemed",
	"host_user_id": "whose calendar a meeting is booked on",
	"assignee_id":  "who a task is for",
}

// requiredRecordID is the one required uuid argument naming the record this verb
// is about, or false where the schema does not settle it.
//
// Read from the SCHEMA rather than a Go struct tag, because the schema is what a
// caller is served on tools/list and therefore what a caller can be wrong about.
func requiredRecordID(t *testing.T, inputSchema json.RawMessage) (string, bool) {
	t.Helper()
	var schema struct {
		Required   []string `json:"required"`
		Properties map[string]struct {
			Format string `json:"format"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(inputSchema, &schema); err != nil {
		t.Fatalf("a registered tool's input schema is not readable: %v", err)
	}
	// A verb that takes `record_type` is the GENERIC shape and is excluded, not
	// compared: it serves many record types, so `record_id`/`id` is the correct
	// spelling for it and grouping it under each type it can reach would report
	// the convention as a disagreement with every dedicated verb.
	// TestAGenericIDNamesAGenericVerb is what holds the generic shape instead.
	if _, generic := schema.Properties["record_type"]; generic {
		return "", false
	}
	var subject []string
	for _, req := range schema.Required {
		if schema.Properties[req].Format != "uuid" {
			continue
		}
		if _, other := notTheRecordsOwnID[req]; other {
			continue
		}
		subject = append(subject, req)
	}
	sort.Strings(subject)
	if len(subject) != 1 {
		// None: a collection read, or a verb whose every required id is another
		// record's (merge_records, the set relinks). Several: a shape this
		// census has no opinion on, and saying so beats guessing which is the
		// subject.
		return "", false
	}
	return subject[0], true
}

func joinVerbs(verbs []string) string {
	out := ""
	for i, v := range verbs {
		if i > 0 {
			out += ", "
		}
		out += v
	}
	return out
}

func joinLines(lines []string) string {
	out := ""
	for i, l := range lines {
		if i > 0 {
			out += "\n"
		}
		out += l
	}
	return out
}

// TestAGenericIDNamesAGenericVerb closes the gap the census above states it
// cannot see, and it closes it from the SCHEMA alone.
//
// `record_id` and a bare `id` are the spellings of the GENERIC shape: a verb
// that is handed the record's type as an argument and its id beside it. They
// carry no information about which record without that type, so a verb hard-wired
// to one record type wearing one of them tells the caller nothing and disagrees
// with every dedicated sibling. That is exactly what qualify_lead did — it
// required `record_id` with no `record_type` anywhere in its schema, and asked
// only leads.
//
// WHY THIS ONE REACHES WHAT THE OTHER CANNOT. The census above groups by the
// record type the CONTRACT declares, so a composed intent — which no
// `x-mcp-tool` declares and no policy row names — is outside it. This test asks
// a question the schema answers on its own: if you take a generic id, you must
// take the type that gives it meaning. No declaration needed, so every
// registered tool is in the corpus, composed intents included.
func TestAGenericIDNamesAGenericVerb(t *testing.T) {
	t.Parallel()

	genericIDs := map[string]bool{"record_id": true, "id": true}
	checked := 0
	for _, spec := range NewRegistry(nil, SendPath{}).Specs() {
		var schema struct {
			Required   []string `json:"required"`
			Properties map[string]struct {
				Format string `json:"format"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(spec.InputSchema, &schema); err != nil {
			t.Fatalf("%s: input schema is not readable: %v", spec.Name, err)
		}
		_, takesType := schema.Properties["record_type"]
		for _, req := range schema.Required {
			if !genericIDs[req] || schema.Properties[req].Format != "uuid" {
				continue
			}
			checked++
			if takesType {
				continue
			}
			t.Errorf("%s requires %q but takes no `record_type`, so the argument names no record: "+
				"a generic id is only meaningful beside the type that says what it points at. A verb "+
				"that answers ONE record type names that type's id — `lead_id`, `deal_id`, "+
				"`project_id` — which is also what its siblings ask for.", spec.Name, req)
		}
	}
	if checked == 0 {
		t.Fatal("no tool requires a generic id, so this census measures nothing — the spellings it " +
			"guards may have been renamed out from under it")
	}
}

// WHAT THIS CENSUS CANNOT SEE, said out loud because under-recognition is the
// one way a gate fails without a failing assertion (AGENTS.md, "a census that
// can fail short has already failed"):
//
//   - A COMPOSED INTENT. `qualify_lead` composes getLead + updateLead, so no
//     `x-mcp-tool` declares it and it carries no policy row — it is outside THIS
//     census. TestAGenericIDNamesAGenericVerb below covers the half that matters
//     by asking the schema instead of the contract, so the shape qualify_lead
//     actually had is now held. What remains out of reach is a composed intent
//     that names a plausible `<type>_id` for the WRONG type; catching that needs
//     every record-scoped tool to declare its record type in Go, which is a real
//     change and the honest next step.
//   - A RECORD TYPE WITH ONE VERB has no sibling to disagree with, so a lone
//     wrong spelling passes. TestOneVerbPerRecordTypeIsStillHeldToTheConvention
//     below plants that case rather than leaving it to chance.
//   - A NON-UUID id. Nothing on this surface names a record by anything else
//     today; if one appears, it is invisible here.

// TestOneVerbPerRecordTypeIsStillHeldToTheConvention is the planted case for the
// shape the census above cannot see: agreement is vacuous where there is only
// one verb, so the convention itself is asserted for those.
//
// It is the weaker claim of the two — it names a spelling — and it is confined
// to the verbs that have no sibling, which is exactly where agreement says
// nothing.
func TestOneVerbPerRecordTypeIsStillHeldToTheConvention(t *testing.T) {
	t.Parallel()

	checked := 0
	for recordType, verbs := range idArgByRecordType(t) {
		if len(verbs) != 1 {
			continue
		}
		for verb, arg := range verbs {
			checked++
			if want := recordType + "_id"; arg != want {
				t.Errorf("%s is the only verb over %s, so nothing disagrees with it — and it asks "+
					"for %q where the surface's convention for a single-type verb is %q",
					verb, recordType, arg, want)
			}
		}
	}
	if checked == 0 {
		t.Skip("every record type has more than one verb, so the census above covers them all")
	}
}
