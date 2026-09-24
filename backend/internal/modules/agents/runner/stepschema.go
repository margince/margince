// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package runner

// The step protocol as a schema the PROVIDER enforces, kept beside its own
// tests rather than inside window.go: the window is about what the model is
// shown, and this is about what it may answer.

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// stepSchema is the step protocol as a JSON Schema, so a provider with
// schema-constrained decoding enforces the shape at GENERATION rather than
// leaving parseStep to refuse it afterwards.
//
// A model answering in its own tool-call channel emits that channel's syntax as
// literal text, which is not JSON to recover, so the only place the wrong shape
// can be prevented is before it is generated.
//
// HAND-WRITTEN, against shared/schema's own instruction to compose instead —
// and the exception is measured, not preferred. That package builds Properties
// from a Go map, and encoding/json sorts a map's keys, so it can only ever emit
// args, final, tool. Property ORDER is load-bearing under grammar-constrained
// decoding: converted to the builder, agent_loop fell 0.78→0.18 on one binding
// and 0.53→0.31 on another, and no tool call in the failing run carried args at
// all. Do not "fix" it back without re-certifying agent_loop on two bindings.
//
// NO OBJECT HERE MAY BE DECLARED WITHOUT ITS KEYS. Gemini's decoder admits no
// key into an object whose schema lists no properties, open or not: a model
// that wanted to write arguments padded `"args": {` with whitespace to the
// output ceiling, and a closing step wrote `"final": { }`. So args is the named
// tool's own input schema and final declares its summary.
//
// ONE BRANCH PER OFFERED TOOL, pairing `tool` with that tool's own `args`. A
// single tool-call branch whose args was an anyOf over every tool let
// `{"tool":"read_record","args":{}}` through — a zero-argument tool's schema
// admits `{}` — so no tool's `required` was enforced, and `tool` was any string.
// `enum`, not `const`: Gemini's documented keyword subset has no `const`.
//
// Each branch REQUIRES its keys, because an optional args was simply skipped:
// the same decoder closed the object after "tool" in every measured call. The
// branches are also the exactly-one-of rule parseStep holds, and each is closed
// to mirror its DisallowUnknownFields.
//
// The schema is O(offered tools) and rides every step, so window.bounded counts
// it against the prompt window with the rest of the request.
//
// Held by: TestTheStepSchemaAdmitsExactlyWhatTheStepParserAccepts (internal/modules/agents/runner/stepschema_test.go)
func stepSchema(offered []mcp.ToolSpec) json.RawMessage {
	var b strings.Builder
	b.WriteString(`{"anyOf":[`)
	for _, branch := range toolCallBranches(offered) {
		b.WriteString(branch)
		b.WriteString(",")
	}
	b.WriteString(`{"type":"object","properties":{"final":{"type":"object",` +
		`"properties":{"summary":{"type":"string"}},"required":["summary"]}},` +
		`"required":["final"],"additionalProperties":false}]}`)
	return json.RawMessage(b.String())
}

// toolCallBranches renders one branch per offered tool, in the listing's name
// order so one catalog always yields one request. Each is written tool, then
// args: the order that was measured.
//
// There is NO fallback for a schema that will not parse. Registration panics on
// one at boot (assertObjectSchemas), and one that bypassed it is carried
// verbatim, so the adapter refuses to encode the request instead of the model
// being offered a bare `args` — the exact shape this schema keeps off the wire.
func toolCallBranches(offered []mcp.ToolSpec) []string {
	sorted := append([]mcp.ToolSpec(nil), offered...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	branches := make([]string, 0, len(sorted))
	for _, spec := range sorted {
		branches = append(branches, `{"type":"object","properties":{"tool":{"type":"string","enum":[`+
			strconv.Quote(spec.Name)+`]},"args":`+CompactSchema(spec)+
			`},"required":["tool","args"],"additionalProperties":false}`)
	}
	return branches
}
