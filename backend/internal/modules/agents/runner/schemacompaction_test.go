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

// mutatingSpec is a tool shaped the way the surface splices one: a closed
// object, one domain argument, and the retry key the surface owns — including
// its description, which is what the compaction takes out.
//
// The retry key's rendering is the SURFACE's, byte for byte
// (agents.retryKeyProperty), so a change to the member there shows up here as a
// changed expectation rather than as a silently weaker case.
func mutatingSpec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name:          "log_activity",
		Description:   "Record something that happened on a record.",
		RequiredScope: principal.ScopeWrite,
		InputSchema: json.RawMessage(`{"type":"object","required":["record_id"],"properties":{` +
			`"record_id":{"type":"string","description":"The record this happened on."},` +
			`"idempotency_key":{"type":"string","maxLength":255,` +
			`"description":"Optional. Same key, same result; a key reused with other arguments is refused."}},` +
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
	compacted := CompactSchema(mutatingSpec().InputSchema)
	if strings.Contains(compacted, "Same key, same result") {
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
	compacted := CompactSchema(nestedSpec().InputSchema)
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
	compacted := CompactSchema(open)
	if !strings.Contains(compacted, `"additionalProperties":{"type":"string"}`) {
		t.Errorf("a constrained additionalProperties was dropped as though it were the closed form:\n%s", compacted)
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
	compacted := CompactSchema(spec)
	if !strings.Contains(compacted, "Say whether an idempotency_key was sent.") {
		t.Errorf("a tool's own prose was stripped because it mentioned the reserved name:\n%s", compacted)
	}
}

// `approval_id` is NOT compacted, and that is a decision rather than an
// omission: it carries three descriptions across nineteen members, and one of
// them is a per-tool replay instruction that no frame sentence replaces. Keying
// the compaction on text would have taken all three; keying it on ownership
// takes none, which is the right answer for a member whose meaning varies.
func TestTheApprovalKeyIsNotCompacted(t *testing.T) {
	const instruction = "Set on retry after a human approved overwriting their edit; " +
		"send it with exactly the staged replay arguments"
	spec := json.RawMessage(`{"type":"object","properties":{` +
		`"approval_id":{"type":"string","description":"` + instruction + `"}},` +
		`"additionalProperties":false}`)
	compacted := CompactSchema(spec)
	if !strings.Contains(compacted, instruction) {
		t.Errorf("approval_id's per-tool replay instruction was compacted away:\n%s", compacted)
	}
}

// Byte-stable across processes. The equivalence gate in the composition compares
// this function's output against the listing's, so a map iteration leaking into
// the rendered order would make that gate flap rather than fail.
func TestTheCompactionIsByteStable(t *testing.T) {
	first := CompactSchema(mutatingSpec().InputSchema)
	for range 16 {
		if again := CompactSchema(mutatingSpec().InputSchema); again != first {
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
	if got := CompactSchema(broken); got != string(broken) {
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
	if !strings.Contains(frame, "Same key, same result") {
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
	if strings.Contains(listing, "Same key, same result") {
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
