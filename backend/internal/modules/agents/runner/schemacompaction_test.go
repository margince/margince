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
// object, one domain argument, and the retry key the surface owns — including
// its description, which is what the compaction takes out.
//
// It shares mcp.ReservedIdempotencyKeyRule with the surface, so a reworded rule
// moves this fixture too. The bound and the `Optional. ` prefix are copied, and
// that much is a copy: nothing holds them equal to agents.retryKeyProperty, and
// what the compaction reads is the member's presence rather than its shape.
func mutatingSpec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name:          "log_activity",
		Description:   "Record something that happened on a record.",
		RequiredScope: principal.ScopeWrite,
		InputSchema: json.RawMessage(`{"type":"object","required":["record_id"],"properties":{` +
			`"record_id":{"type":"string","description":"The record this happened on."},` +
			`"idempotency_key":{"type":"string","maxLength":255,` +
			`"description":"Optional. ` + mcp.ReservedIdempotencyKeyRule + `"}},` +
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

// The retry key's description goes and the member STAYS. A model still has to
// know the argument exists, its type and its bound in order to send one — what
// it no longer needs, 32 times over, is the sentence the frame states once.
func TestTheRetryKeysDescriptionIsOmittedAndTheMemberKept(t *testing.T) {
	compacted := CompactSchema(mutatingSpec())
	if strings.Contains(compacted, mcp.ReservedIdempotencyKeyRule) {
		t.Errorf("the retry key's description is still rendered per tool:\n%s", compacted)
	}
	// Maps rather than nested structs: JSON Schema's own keywords are camelCase
	// (`maxLength` here, `additionalProperties` next door) and no snake_case tag
	// can spell them, which the repo's tag linter is right to insist on for a
	// wire type.
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
	if description, described := retryKey["description"]; described {
		t.Errorf("the retry key still carries a description: %s", description)
	}
	// A TOOL's own argument keeps its description. This is the whole difference
	// between compacting what the surface owns and compacting text.
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

// THE PLANTED FALSE POSITIVE. `idempotency_key` appears inside the retry key's
// OWN description text, so a compaction that matched the property NAME against
// the rendered bytes — rather than against the member the surface owns — has a
// way to be wrong that has to be watched.
//
// Here the string sits in a DIFFERENT tool's own argument description. A
// text-keyed implementation would strip that sentence too, silently taking away
// per-tool meaning; an ownership-keyed one leaves it exactly where it is.
func TestAToolThatMentionsTheReservedNameKeepsItsOwnProse(t *testing.T) {
	spec := json.RawMessage(`{"type":"object","properties":{` +
		`"note":{"type":"string","description":"Say whether an idempotency_key was sent."}},` +
		`"additionalProperties":false}`)
	compacted := CompactSchema(mutating(spec))
	if !strings.Contains(compacted, "Say whether an idempotency_key was sent.") {
		t.Errorf("a tool's own prose was stripped because it mentioned the reserved name:\n%s", compacted)
	}
}

// THE SECOND PLANTED CASE, and the one the first version of this compaction got
// wrong: a member NAMED `idempotency_key` nested inside an array's items.
//
// The surface owns that name at the ROOT of a mutating tool's schema and nowhere
// else — spliceRetryKey writes it into the top-level `properties` and refuses a
// tool that declares it there itself. A nested one is somebody's own per-item
// key, exactly the shape a batch tool invites, and the frame's sentence is not
// about it. Nothing in the catalog nests one today, which is what made this
// invisible rather than harmless.
func TestANestedMemberOfTheReservedNameKeepsItsDescription(t *testing.T) {
	const perItem = "The key for THIS item, distinct from the call's own."
	spec := json.RawMessage(`{"type":"object","properties":{` +
		`"items":{"type":"array","items":{"type":"object","properties":{` +
		`"idempotency_key":{"type":"string","description":"` + perItem + `"}},` +
		`"additionalProperties":false}},` +
		`"idempotency_key":{"type":"string","maxLength":255,` +
		`"description":"Optional. ` + mcp.ReservedIdempotencyKeyRule + `"}},` +
		`"additionalProperties":false}`)
	compacted := CompactSchema(mutating(spec))
	if !strings.Contains(compacted, perItem) {
		t.Errorf("a NESTED member named %q lost its own description, which the surface never wrote "+
			"and the frame does not state:\n%s", mcp.ReservedIdempotencyKeyArg, compacted)
	}
	// And the ROOT one is still compacted — otherwise this test would pass by
	// the compaction having stopped working altogether.
	if strings.Contains(compacted, "Optional. "+mcp.ReservedIdempotencyKeyRule) {
		t.Errorf("the root retry key kept its description, so the level rule went the wrong way:\n%s", compacted)
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
// are walked in compactSchemaProperties and keywords in compactSchemaShape, so
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

// The shared rule has to survive being pasted into a JSON string literal.
//
// registration.go builds the member's `description` by concatenating this
// constant into a raw Go string that IS JSON — there is no encoder in that path,
// because the property is a compile-time constant. A quote or a backslash in the
// rule would produce a schema that does not parse, and the whole tools/list
// response with it. The runtime enforcement is a boot-time panic in a code path
// no test exercises by name, so the character set is asserted here instead.
func TestTheSharedRetryKeyRuleIsSafeInAJSONLiteral(t *testing.T) {
	// Asserted by ENCODING it the way the surface does, not by listing forbidden
	// characters: JSON requires every one of U+0000-U+001F escaped, and a
	// hand-written set of five will always be an incomplete version of that.
	spliced := json.RawMessage(`{"type":"string","description":"Optional. ` +
		mcp.ReservedIdempotencyKeyRule + `"}`)
	if !json.Valid(spliced) {
		t.Errorf("mcp.ReservedIdempotencyKeyRule does not survive being pasted into a JSON string "+
			"literal, which is what the surface does to it with no encoder in the path: %q",
			mcp.ReservedIdempotencyKeyRule)
	}
	if mcp.ReservedIdempotencyKeyRule == "" {
		t.Error("the shared rule is empty, so the frame states nothing and every check on it is vacuous")
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
	// The retry key: omitted from every listing, so the frame owes the rule.
	if !strings.Contains(frame, mcp.ReservedIdempotencyKeyArg) {
		t.Error("the listing omits the retry key's description and the frame never states the rule, " +
			"so a run can no longer learn that the argument exists")
	}
	if !strings.Contains(frame, mcp.ReservedIdempotencyKeyRule) {
		t.Error("the frame does not say what a reused key does, which is the whole content of the " +
			"sentence the listing stopped printing")
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
	if strings.Contains(listing, mcp.ReservedIdempotencyKeyRule) {
		t.Errorf("the listing still prints the retry key's description:\n%s", listing)
	}
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
