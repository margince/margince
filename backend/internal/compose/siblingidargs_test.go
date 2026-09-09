// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package compose

// Sibling verbs over one record type name that record's id the same way.
//
// The defect: three lead lifecycle verbs advertised two spellings of one
// argument — `qualify_lead` took `record_id` while `promote_lead` and
// `disqualify_lead` took `lead_id` — so a caller who learned the name from two
// of them was refused by the third, and paid a round trip to find out.
//
// WHY CONTRACT-FIRST DID NOT CATCH IT, which is the interesting part. This is
// not a drift the drift gate missed: `gen-agentpolicy` lifts seven fields out of
// crm.yaml — route, operationId, access, verb, record type, tier, scope — and
// argument names are not among them, and structurally cannot be. On REST the id
// is a PATH parameter (`$ref: parameters/Id`); the request bodies carry only
// reason, trigger and evidence. Turning `{id}` in a URL into a named JSON member
// is an act with no source in the contract, so `make drift` is vacuous here by
// construction and every tool's InputSchema is a hand-written literal.
//
// So the obligation is derived from the one fact the contract DOES carry: which
// record type a verb targets. Two verbs that target the same record must ask for
// its id by the same name. This asserts agreement and deliberately does not
// assert WHICH name is right — that is a naming decision, and a gate that picked
// one would be legislating rather than holding a line.

import (
	"encoding/json"
	"sort"
	"testing"
)

// TestSiblingVerbsAgreeOnTheIDTheyName groups the tool surface by the record
// type the contract declares for each verb, and refuses two spellings of one
// record's id.
func TestSiblingVerbsAgreeOnTheIDTheyName(t *testing.T) {
	t.Parallel()

	schemas := map[string]json.RawMessage{}
	for _, spec := range NewRegistry(nil, SendPath{}).Specs() {
		schemas[spec.Name] = spec.InputSchema
	}

	// verb -> spelling, grouped by the record type the contract declares.
	byRecordType := map[string]map[string]string{}
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
		named := idArgumentsNaming(t, schema, string(pol.RecordType))
		if len(named) != 1 {
			// Nothing to compare: a verb that names no id for this record (a
			// collection read), or several (merge_records' source and target,
			// whose pair is its own shape rather than a spelling of one id).
			continue
		}
		if byRecordType[string(pol.RecordType)] == nil {
			byRecordType[string(pol.RecordType)] = map[string]string{}
		}
		byRecordType[string(pol.RecordType)][pol.Tool] = named[0]
	}

	if len(byRecordType) == 0 {
		t.Fatal("no declared tool named an id for its record type, so this census measures nothing")
	}

	for recordType, verbs := range byRecordType {
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

// idArgumentsNaming reads the REQUIRED uuid arguments whose name plausibly names
// this record — the record's own `<type>_id`, or the generic `record_id`/`id`.
//
// Required only, and uuid only: an optional id is not what a caller must learn,
// and a non-uuid required argument (a trigger, a reason) is not an id at all.
// Reading the schema rather than a Go struct tag is deliberate — the schema is
// what a caller is served on tools/list, so it is what a caller can be wrong
// about.
func idArgumentsNaming(t *testing.T, inputSchema json.RawMessage, recordType string) []string {
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
	// A verb that takes `record_type` is the GENERIC shape, and `record_id` is
	// the right spelling there — it is the companion of the type beside it.
	// Grouping those with the single-type verbs would report every generic verb
	// as disagreeing with every dedicated one, which is the convention rather
	// than a defect.
	if _, generic := schema.Properties["record_type"]; generic {
		return nil
	}
	candidates := map[string]bool{recordType + "_id": true, "record_id": true, "id": true}
	var named []string
	for _, req := range schema.Required {
		if !candidates[req] {
			continue
		}
		if schema.Properties[req].Format == "uuid" {
			named = append(named, req)
		}
	}
	sort.Strings(named)
	return named
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

// WHAT THIS CENSUS CANNOT SEE, said out loud because under-recognition is the
// one way a gate fails without a failing assertion (AGENTS.md, "a census that
// can fail short has already failed"):
//
//   - A COMPOSED INTENT. `qualify_lead` composes getLead + updateLead, so no
//     `x-mcp-tool` declares it and it carries no policy row — it is outside this
//     corpus entirely. It is also the verb whose divergence prompted this gate.
//     Reaching it needs every record-scoped tool to declare its record type in
//     Go, which is a real change and the honest next step rather than something
//     this file can assert around.
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

	schemas := map[string]json.RawMessage{}
	for _, spec := range NewRegistry(nil, SendPath{}).Specs() {
		schemas[spec.Name] = spec.InputSchema
	}

	perType := map[string]map[string]string{}
	for _, pol := range agentPolicies {
		if pol.Access != accessTool || pol.RecordType == "" || pol.Tool == "" {
			continue
		}
		schema, registered := schemas[pol.Tool]
		if !registered {
			continue
		}
		named := idArgumentsNaming(t, schema, string(pol.RecordType))
		if len(named) != 1 {
			continue
		}
		if perType[string(pol.RecordType)] == nil {
			perType[string(pol.RecordType)] = map[string]string{}
		}
		perType[string(pol.RecordType)][pol.Tool] = named[0]
	}

	checked := 0
	for recordType, verbs := range perType {
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
