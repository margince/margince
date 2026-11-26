// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package runner

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// mutating wraps a bare schema as a MUTATING tool — the only kind the surface
// splices its retry key into, and so the only kind whose rendered schema can
// carry one at the root at all (assertNoDeclaredRetryKey refuses any tool that
// declares the member itself).
func mutating(schema json.RawMessage) mcp.ToolSpec {
	return mcp.ToolSpec{Name: "probe", RequiredScope: principal.ScopeWrite, InputSchema: schema}
}

// mutatingSpec is a tool shaped the way the surface splices one: a closed
// object, one domain argument, and the retry key the surface owns. The key
// carries no description, as agents.retryKeyProperty serves it.
func mutatingSpec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name:          "log_activity",
		Description:   "Record something that happened on a record.",
		RequiredScope: principal.ScopeWrite,
		InputSchema: json.RawMessage(`{"type":"object","required":["record_id"],"properties":{` +
			`"record_id":{"type":"string","description":"The record this happened on."},` +
			`"idempotency_key":{"type":"string","maxLength":255}},` +
			`"additionalProperties":false}`),
	}
}

// nestedSpec closes an object INSIDE an array's items, which is where the
// whole-catalog count of `"additionalProperties":false` exceeds the tool count.
func nestedSpec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name:          "run_report",
		Description:   "Run one of this workspace's prebuilt reports.",
		RequiredScope: principal.ScopeRead,
		InputSchema: json.RawMessage(`{"type":"object","properties":{` +
			`"aggregates":{"type":"array","items":{"type":"object","required":["fn"],"properties":{` +
			`"fn":{"type":"string"},"as":{"type":"string","description":"Output column name."}},` +
			`"additionalProperties":false}}},"additionalProperties":false}`),
	}
}

// The retry key and every argument stay. A model still has to know the
// argument exists, its type and its bound to send one.
func TestTheCompactionKeepsEveryArgument(t *testing.T) {
	compacted := CompactSchema(mutatingSpec())
	// Maps rather than nested structs: JSON Schema's own keywords are camelCase
	// (`maxLength` here) and no snake_case tag can spell them.
	var parsed struct {
		Properties map[string]map[string]json.RawMessage `json:"properties"`
		Required   []string                              `json:"required"`
	}
	if err := json.Unmarshal([]byte(compacted), &parsed); err != nil {
		t.Fatalf("the compacted schema is not valid JSON: %v\n%s", err, compacted)
	}
	retryKey := parsed.Properties[mcp.ReservedIdempotencyKeyArg]
	if string(retryKey["type"]) != `"string"` || string(retryKey["maxLength"]) != "255" {
		t.Errorf("the retry key lost its type or its bound, so a caller cannot tell what to send:\n%s", compacted)
	}
	if _, described := parsed.Properties["record_id"]["description"]; !described {
		t.Error("a tool's own argument lost its description, which is per-tool meaning and not a surface fact")
	}
	if len(parsed.Required) != 1 || parsed.Required[0] != "record_id" {
		t.Errorf("required = %v, want [record_id] — the compaction moved what a call must carry", parsed.Required)
	}
}

// The closed form goes at every depth. run_report's `aggregates` items close
// themselves, which is why the whole-catalog count of the keyword exceeds the
// tool count: a top-level-only compaction would leave most of it behind.
func TestTheClosedFormIsOmittedAtEveryDepth(t *testing.T) {
	compacted := CompactSchema(nestedSpec())
	if strings.Contains(compacted, `"additionalProperties":false`) {
		t.Errorf("a nested closed object still renders the keyword:\n%s", compacted)
	}
	// And nothing else moved: the nested member's own description and its
	// required list are per-tool meaning.
	for _, kept := range []string{`"Output column name."`, `"required":["fn"]`, `"items"`} {
		if !strings.Contains(compacted, kept) {
			t.Errorf("the compaction dropped %s, which is not a surface fact:\n%s", kept, compacted)
		}
	}
}

// A schema that CONSTRAINS its extra members is saying something the frame does
// not, so it survives. Only the closed form is a surface fact; `{"type":"string"}`
// there is this tool's own rule about what an unlisted key must look like.
func TestOnlyTheClosedFormIsOmitted(t *testing.T) {
	open := json.RawMessage(`{"type":"object","additionalProperties":{"type":"string"}}`)
	compacted := CompactSchema(mutating(open))
	if !strings.Contains(compacted, `"additionalProperties":{"type":"string"}`) {
		t.Errorf("a constrained additionalProperties was dropped as though it were the closed form:\n%s", compacted)
	}
}

// `additionalProperties` is RECURSED INTO when it is a schema.
//
// It is the third place this surface nests an object schema, and an earlier pass
// walked only `properties` and `items` — so a closed object under one survived
// the compaction while the comment claimed there was nowhere left to hide.
// qualify_lead's own output schema nests a `properties` tree under one, which is
// what makes this reachable rather than hypothetical.
func TestAClosedObjectUnderAdditionalPropertiesIsCompacted(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{` +
		`"filled":{"type":"object","additionalProperties":{"type":"object","properties":{` +
		`"value":{"type":"string","description":"What was filled in."}},` +
		`"additionalProperties":false}}},` +
		`"additionalProperties":false}`)
	compacted := CompactSchema(mutating(schema))
	if strings.Contains(compacted, `"additionalProperties":false`) {
		t.Errorf("a closed object nested under `additionalProperties` kept the closed form, so the "+
			"third nesting place is still unvisited:\n%s", compacted)
	}
	// The per-tool meaning inside it survives, so this is not passing by the
	// whole branch having been dropped.
	if !strings.Contains(compacted, `"What was filled in."`) {
		t.Errorf("the compaction dropped a description nested under `additionalProperties`:\n%s", compacted)
	}
}

// No description is compacted, at any depth. A batch tool's per-item key named
// like the surface's own is the tool's own meaning, and so is prose naming it.
func TestTheCompactionRemovesNoDescription(t *testing.T) {
	const perItem = "The key for THIS item, distinct from the call's own."
	const prose = "Say whether an idempotency_key was sent."
	spec := json.RawMessage(`{"type":"object","properties":{` +
		`"items":{"type":"array","items":{"type":"object","properties":{` +
		`"idempotency_key":{"type":"string","description":"` + perItem + `"}},` +
		`"additionalProperties":false}},` +
		`"note":{"type":"string","description":"` + prose + `"}},` +
		`"additionalProperties":false}`)
	compacted := CompactSchema(mutating(spec))
	for _, kept := range []string{perItem, prose} {
		if !strings.Contains(compacted, kept) {
			t.Errorf("the compaction dropped the description %q:\n%s", kept, compacted)
		}
	}
	if strings.Contains(compacted, `"additionalProperties":false`) {
		t.Errorf("the closed form survived, so nothing was compacted here at all:\n%s", compacted)
	}
}

// `approval_id` is NOT compacted, and that is a decision rather than an
// omission: its description VARIES by tool, and one spelling of it is a per-tool
// replay instruction that no frame sentence replaces. Keying the compaction on
// text would have taken every spelling; keying it on ownership takes none, which
// is the right answer for a member whose meaning is not the surface's.
func TestTheApprovalKeyIsNotCompacted(t *testing.T) {
	const instruction = "Set on retry after a human approved overwriting their edit; " +
		"send it with exactly the staged replay arguments"
	spec := json.RawMessage(`{"type":"object","properties":{` +
		`"approval_id":{"type":"string","description":"` + instruction + `"}},` +
		`"additionalProperties":false}`)
	compacted := CompactSchema(mutating(spec))
	if !strings.Contains(compacted, instruction) {
		t.Errorf("approval_id's per-tool replay instruction was compacted away:\n%s", compacted)
	}
}

// A property NAMED `items` or `properties` is a property, not a keyword.
//
// The distinction is real on this surface: an import tool's input schema
// declares `properties.items` as an array. It is safe because property names
// are walked in compaction.properties and keywords in compaction.shape, so
// the two never see each other's namespace — but "safe by construction" is the
// kind of claim that wants a case, because the fix for something else could
// merge those two walks without anybody noticing this.
func TestAPropertyNamedLikeAKeywordIsStillAProperty(t *testing.T) {
	spec := json.RawMessage(`{"type":"object","properties":{` +
		`"items":{"type":"array","description":"The rows to import.","items":{"type":"object",` +
		`"properties":{"ref":{"type":"string","description":"This row's ref."}},` +
		`"additionalProperties":false}},` +
		`"properties":{"type":"object","description":"Extra columns, by name."}},` +
		`"additionalProperties":false}`)
	compacted := CompactSchema(mutating(spec))
	var parsed struct {
		Properties map[string]map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal([]byte(compacted), &parsed); err != nil {
		t.Fatalf("the compacted schema is not valid JSON: %v\n%s", err, compacted)
	}
	for _, name := range []string{"items", "properties"} {
		if _, present := parsed.Properties[name]; !present {
			t.Errorf("the property named %q was consumed as a JSON Schema keyword:\n%s", name, compacted)
		}
	}
	for _, kept := range []string{`"The rows to import."`, `"This row's ref."`, `"Extra columns, by name."`} {
		if !strings.Contains(compacted, kept) {
			t.Errorf("the compaction dropped %s, which is per-tool meaning:\n%s", kept, compacted)
		}
	}
	// The nested closed form still goes, so this is not passing by the
	// compaction having skipped the whole schema.
	if strings.Contains(compacted, `"additionalProperties":false`) {
		t.Errorf("the closed form survived, so nothing was compacted here at all:\n%s", compacted)
	}
}

// `default`, `const` and `examples` hold INSTANCE data, not schemas, so the
// compaction leaves them alone even when their contents look like schema
// keywords.
//
// A tool whose `default` contains `{"additionalProperties": false}` is declaring
// a default VALUE with a member of that name — not closing an object. Stripping
// it would corrupt the default the tool publishes, and the composition's
// equivalence gate narrows its own walk to the same positions for the same
// reason: being independent in the wrong dimension turns a gate into a false
// alarm.
func TestInstanceDataThatLooksLikeSchemaKeywordsIsUntouched(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{` +
		`"cfg":{"type":"object","default":{"additionalProperties":false,"description":"kept"},` +
		`"examples":[{"additionalProperties":false}]}},` +
		`"additionalProperties":false}`)
	compacted := CompactSchema(mutating(schema))
	for _, kept := range []string{
		`"default":{"additionalProperties":false,"description":"kept"}`,
		`"examples":[{"additionalProperties":false}]`,
	} {
		if !strings.Contains(compacted, kept) {
			t.Errorf("the compaction reached into instance data and removed %s:\n%s", kept, compacted)
		}
	}
	// The real closed form — a sibling of `properties` — still goes, so this is
	// not passing by the compaction having stopped working altogether.
	if got := strings.Count(compacted, `"additionalProperties":false`); got != 2 {
		t.Errorf("want exactly the 2 instance-data occurrences surviving, got %d:\n%s", got, compacted)
	}
}

// Byte-stable across processes. The equivalence gate in the composition compares
// this function's output against the listing's, so a map iteration leaking into
// the rendered order would make that gate flap rather than fail.
func TestTheCompactionIsByteStable(t *testing.T) {
	first := CompactSchema(mutatingSpec())
	for range 16 {
		if again := CompactSchema(mutatingSpec()); again != first {
			t.Fatalf("two compactions of one schema differ:\n%s\n%s", first, again)
		}
	}
}

// A schema that will not parse is rendered VERBATIM rather than dropped. The
// registry refuses one at boot, so this is unreachable from the real surface —
// and an unhelpfully large schema is still better than a silently absent one,
// which would leave a model unable to call a tool it can see.
func TestAnUnparseableSchemaIsRenderedRatherThanDropped(t *testing.T) {
	broken := json.RawMessage(`{"type":"object",`)
	if got := CompactSchema(mutating(broken)); got != string(broken) {
		t.Errorf("CompactSchema(%s) = %s, want it verbatim", broken, got)
	}
}

// EVERY OMISSION HAS A FRAME SENTENCE, and the frame states nothing about a
// member the compaction leaves in place. Both directions, because the second is
// what catches the frame growing rules for things the listing still prints — the
// duplicate that costs tokens on every run and contradicts nothing loudly.
func TestTheFrameStatesExactlyWhatTheCompactionOmits(t *testing.T) {
	frame := systemPrompt([]mcp.ToolSpec{mutatingSpec(), nestedSpec()}, promptfence.New(), "")
	// The retry key: no schema describes it, so the frame owes the rule.
	if !strings.Contains(frame, mcp.ReservedIdempotencyKeyRule) {
		t.Error("the frame does not say what the retry key does, and no listed schema says it either")
	}
	// The closed form: omitted, so the frame owes the refusal.
	if !strings.Contains(frame, "refused by name") {
		t.Error("the listing omits `additionalProperties: false` and the frame never says an unknown " +
			"argument is refused, so a model reads an open schema and invents members")
	}
	// The other direction. `approval_id` is still rendered per tool, so a frame
	// sentence about it would be a second copy paid for on every step.
	if strings.Contains(frame, mcp.ReservedApprovalIDArg) {
		t.Errorf("the frame states a rule about %s, which the listing still renders per tool — one "+
			"invariant in two places, and the tool's own wording is the one that varies",
			mcp.ReservedApprovalIDArg)
	}
}

// The listing renders the COMPACTED schema, not the served one. Asserted on the
// listing rather than on CompactSchema alone, because the two could agree in
// isolation while ToolListing kept printing the original.
func TestTheListingRendersTheCompactedSchema(t *testing.T) {
	listing := ToolListing([]mcp.ToolSpec{mutatingSpec(), nestedSpec()})
	if strings.Contains(listing, `"additionalProperties":false`) {
		t.Errorf("the listing still prints the closed form:\n%s", listing)
	}
	// Every tool is still listed, with its name and its description first.
	for _, spec := range []mcp.ToolSpec{mutatingSpec(), nestedSpec()} {
		if !strings.Contains(listing, "- "+spec.Name+" — "+spec.Description) {
			t.Errorf("%s lost its name or description line:\n%s", spec.Name, listing)
		}
	}
}

// The data boundary is the LAST thing in the system prompt, after the tool
// catalog — and the ordering is worth a test because it is worth ~97 KB.
//
// The marker is minted per run, so a provider reusing a byte-identical prefix
// can reuse everything BEFORE it and nothing after. The catalog is identical
// for every run of a given tool surface and is by far the largest part of this
// prompt, so putting the marker in front of it made the whole thing
// unreusable. Nothing about the boundary's meaning depends on where it sits:
// it names the markers that bound captured text, and captured text arrives in
// the user turns either way.
//
// Without this test the order is one editor's convenience away from silently
// reverting, and the cost would show up as a bill rather than a failure.
func TestTheDataBoundaryComesAfterTheToolCatalog(t *testing.T) {
	t.Parallel()
	specs := []mcp.ToolSpec{mutatingSpec(), nestedSpec()}
	system := systemPrompt(specs, promptfence.New(), "")

	boundary := strings.Index(system, "Data is delimited by")
	if boundary < 0 {
		t.Fatal("the system prompt declares no data boundary at all")
	}
	catalog := strings.Index(system, "Available tools:")
	if catalog < 0 {
		t.Fatal("the system prompt lists no tools, so this test proves nothing about their order")
	}
	if boundary < catalog {
		t.Errorf("the data boundary is declared BEFORE the tool catalog, which puts %d bytes of "+
			"identical catalog behind a marker that changes every run — nothing after the marker "+
			"can be reused by a provider", len(system)-boundary)
	}
	// And it must be last: anything written after it is in the same position,
	// unreusable, for the same reason.
	tail := strings.TrimSpace(system[boundary:])
	if !strings.HasSuffix(tail, "is part of the data.") {
		t.Errorf("something follows the data boundary, and whatever it is cannot be reused:\n%q",
			tail[strings.LastIndex(tail, "is part of the data.")+len("is part of the data."):])
	}
}
