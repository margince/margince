// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package runner

// The step protocol as a schema the PROVIDER enforces, kept beside its own
// tests rather than inside window.go: the window is about what the model is
// shown, and this is about what it may answer.

import "encoding/json"

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
// all. Removing the field descriptions did not recover it; the order is what
// the model follows.
//
// So this stays a string until the builder can express an order. Do not
// "fix" it back without re-certifying agent_loop on two bindings.
//
// It constrains the KEY SET and the types, and deliberately not the
// exactly-one-of rule. Expressing that needs oneOf/not, which the strict
// structured-output modes handle unevenly and may refuse the whole request
// over; a rejected request is a worse failure than the one this prevents. The
// XOR is a domain rule parseStep owns, and states with a better error than a
// schema could — so the descriptions below say it in words instead, where a
// model reads them.
//
// The STEP is closed, which mirrors parseStep's DisallowUnknownFields. A
// schema open where the parser is closed would let constrained decoding produce
// a step that then gets refused, which is this bug wearing the opposite face.
//
// args and final are OPEN objects, and must be: their keys belong to whichever
// tool the model picked, which is a runtime registry rather than a shape known
// here. A closed object with no properties admits no key at all, so a provider
// enforcing it would refuse the arguments the loop has to send —
// TestTheStepSchemaLetsArgsAndFinalCarryKeys exists to say so.
//
// Held by: TestTheStepSchemaAdmitsExactlyWhatTheStepParserAccepts (internal/modules/agents/runner/stepschema_test.go)
var stepSchema = json.RawMessage(`{` +
	`"type":"object",` +
	`"properties":{` +
	`"tool":{"type":"string"},` +
	`"args":{"type":"object"},` +
	`"final":{"type":"object"}},` +
	`"additionalProperties":false}`)
