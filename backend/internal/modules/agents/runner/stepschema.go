// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package runner

// The step protocol as a schema the PROVIDER enforces, kept beside its own
// tests rather than inside window.go: the window is about what the model is
// shown, and this is about what it may answer.

import "github.com/margince/margince/backend/internal/shared/schema"

// stepSchema is the step protocol as a JSON Schema, so a provider with
// schema-constrained decoding enforces the shape at GENERATION rather than
// leaving parseStep to refuse it afterwards.
//
// This matters where a reduction of the reply cannot help. A model answering in
// its own tool-call channel emits that channel's syntax as literal text, and it
// is not JSON to recover — `{q: "Anna Weber"}` has unquoted keys — so the only
// place the wrong shape can be prevented is before it is generated.
//
// Built with shared/schema rather than hand-written, which that package asks
// for outright: "callers compose Object/Array/String/… and render with Must
// instead of hand-writing a JSON string, so every structured-output schema is
// compile-checked, always valid JSON, and built one way across the codebase."
// This was a hand-written string first, which is one more spelling of a thing
// the tree already spells 30 times.
//
// It constrains the KEY SET and the types, and deliberately not the
// exactly-one-of rule. Expressing that needs oneOf/not, which the strict
// structured-output modes handle unevenly and may refuse the whole request
// over; a rejected request is a worse failure than the one this prevents. The
// XOR is a domain rule parseStep owns, and states with a better error than a
// schema could — so the descriptions below say it in words instead, where a
// model reads them.
//
// Object() closes the STEP, which mirrors parseStep's DisallowUnknownFields. A
// schema open where the parser is closed would let constrained decoding produce
// a step that then gets refused, which is this bug wearing the opposite face.
//
// args and final are FreeObject, and must be: their keys belong to whichever
// tool the model picked, which is a runtime registry rather than a shape known
// here. Closing them renders an object no key may go in, so a provider
// enforcing the schema refuses the arguments the loop has to send — which is
// what Object(nil) did here before TestTheStepSchemaLetsArgsAndFinalCarryKeys
// existed to say so.
//
// Held by: TestTheStepSchemaAdmitsExactlyWhatTheStepParserAccepts (internal/modules/agents/runner/stepschema_test.go)
var stepSchema = schema.Must(schema.Object(map[string]schema.Node{
	"tool": schema.String().Describe(
		"the name of ONE tool to call, from the tools listed to you. Set this and args, or set final, never both."),
	"args": schema.FreeObject().Describe(
		"that tool's arguments. Required whenever tool is set; write {} for a tool that takes none."),
	"final": schema.FreeObject().Describe(
		"your answer, when no tool call is left to make. Set this alone, with neither tool nor args."),
}))
