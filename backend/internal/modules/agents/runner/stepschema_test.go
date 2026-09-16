// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package runner

import (
	"encoding/json"
	"testing"
)

// The step request constrains its own shape at generation.
//
// This loop is the one place in the tree that asked a model to hand-write JSON
// with no schema on the request: 30 files under internal/compose pass a
// ResponseSchema and window.asRequest passed none, with nothing saying why.
// model.Request's own doc is what this is for — "providers with
// schema-constrained decoding enforce it at GENERATION so a weak model cannot
// emit the wrong shape".
//
// The cost of the omission is measurable rather than theoretical. Certifying an
// open-weight candidate, 4 of 72 turns never produced a step at all: three
// answered in the model's native tool-call channel, which arrives as literal
// text —
//
//	<|tool_call>call:search_records{q: "Anna Weber",record_type: "contact"}<tool_call|>
//
// — and one answered in prose. Neither is recoverable by any reduction of the
// reply, because neither contains the document: `{q: "Anna Weber"}` is not JSON.
// A parser cannot fix this after the fact, which is why the guardrail belongs on
// the request.
func TestTheStepRequestCarriesTheSchemaItsReplyMustMatch(t *testing.T) {
	win := newWindow(Job{Goal: "prep the meeting", TriggerRef: triggerRef}, nil, nil)

	schema := win.asRequest(1000, MinimumPromptWindow).ResponseSchema
	if len(schema) == 0 {
		t.Fatal("the step request carries no ResponseSchema, so a provider with " +
			"schema-constrained decoding has nothing to enforce and a weak model is free " +
			"to answer in its native tool-call channel or in prose")
	}
	if !json.Valid(schema) {
		t.Fatalf("the ResponseSchema is not valid JSON, so an adapter would embed a broken "+
			"schema in its request: %s", schema)
	}
}

// The schema describes the step protocol this package actually parses.
//
// Asserted against parseStep rather than against a copy of the schema: the two
// are one protocol spelled twice — once for the provider to enforce and once for
// this package to read — and the way that goes wrong is silently, with a schema
// admitting a shape the parser then refuses. So every shape named here is put
// through the real parser.
func TestTheStepSchemaAdmitsExactlyWhatTheStepParserAccepts(t *testing.T) {
	win := newWindow(Job{Goal: "prep the meeting", TriggerRef: triggerRef}, nil, nil)
	schema := win.asRequest(1000, MinimumPromptWindow).ResponseSchema

	var declared struct {
		Type                 string                     `json:"type"`
		Properties           map[string]json.RawMessage `json:"properties"`
		AdditionalProperties *bool                      `json:"additionalProperties"`
	}
	if err := json.Unmarshal(schema, &declared); err != nil {
		t.Fatalf("the ResponseSchema is not an object schema: %v", err)
	}
	if declared.Type != "object" {
		t.Errorf("the step schema declares type %q; a step is an object", declared.Type)
	}
	// The three keys parseStep reads, and no others. A schema naming a key the
	// parser does not read would invite a model to fill it.
	for _, key := range []string{"tool", "args", "final"} {
		if _, named := declared.Properties[key]; !named {
			t.Errorf("the step schema does not name %q, which parseStep reads", key)
		}
	}
	if len(declared.Properties) != 3 {
		t.Errorf("the step schema names %d properties; parseStep reads exactly 3", len(declared.Properties))
	}
	// additionalProperties:false mirrors the parser's DisallowUnknownFields. If
	// the schema were open where the parser is closed, constrained decoding
	// would happily produce a step the parser then rejects.
	if declared.AdditionalProperties == nil || *declared.AdditionalProperties {
		t.Error("the step schema allows extra properties while parseStep sets " +
			"DisallowUnknownFields — a model obeying the schema could still be refused")
	}

	// Both legal shapes survive the real parser, so the schema is not describing
	// a protocol this package cannot read.
	for name, step := range map[string]string{
		"tool call": `{"tool":"read_record","args":{"record_id":"x"}}`,
		"final":     `{"final":{"text":"done"}}`,
	} {
		if _, err := parseStep(step); err != nil {
			t.Errorf("%s is a shape the schema admits but parseStep refuses: %v", name, err)
		}
	}
}

// A step the model buried in a sentence is still a step.
//
// parseStep hand-rolled its own fence trim — a CutPrefix of "```json" and a
// TrimSuffix of "```" — which is exactly what kernel/modelreply says a caller
// must not do, and it was strictly weaker than the shared reduction: it reaches
// a fence at the very edges of the reply and nothing else. A model that writes a
// sentence before its JSON, or tags the block, kept losing the step over its
// manners.
//
// It was hand-rolled for a reason rather than out of haste: the reduction lived
// in the ai module and this is the agents module, which may not import a
// sibling. Moving it to shared/kernel is what makes one implementation reachable
// from both.
//
// RECOVERY, NEVER REPAIR still holds — the shapes below each contain a whole
// document, and nothing here guesses at one that does not.
func TestAStepSurvivesTheManners(t *testing.T) {
	for name, reply := range map[string]string{
		"a bare step":            `{"tool":"read_record","args":{"record_id":"x"}}`,
		"a fenced step":          "```json\n{\"tool\":\"read_record\",\"args\":{\"record_id\":\"x\"}}\n```",
		"an untagged fence":      "```\n{\"tool\":\"read_record\",\"args\":{\"record_id\":\"x\"}}\n```",
		"a sentence before it":   "Here is the step:\n{\"tool\":\"read_record\",\"args\":{\"record_id\":\"x\"}}",
		"a sentence after it":    "{\"tool\":\"read_record\",\"args\":{\"record_id\":\"x\"}}\nLet me know if that helps.",
		"a fence and a sentence": "Sure — \n```json\n{\"tool\":\"read_record\",\"args\":{\"record_id\":\"x\"}}\n```\nAnything else?",
	} {
		step, err := parseStep(reply)
		if err != nil {
			t.Errorf("%s: the step was lost to formatting: %v", name, err)
			continue
		}
		if step.Tool != "read_record" {
			t.Errorf("%s: read tool %q, want read_record", name, step.Tool)
		}
	}
}

// The reduction recovers a document; it never invents one. A reply carrying no
// step must still fail, or the loop would proceed on a fabrication.
func TestAReplyWithNoStepInItIsStillRefused(t *testing.T) {
	for name, reply := range map[string]string{
		"prose only":           "those are contacts. The `owner_id` field on a deal refers to a colleague.",
		"a native tool call":   `<|tool_call>call:search_records{q: "Anna Weber",record_type: "contact"}<tool_call|>`,
		"neither tool nor end": `{}`,
		"both at once":         `{"tool":"read_record","args":{},"final":{"text":"done"}}`,
	} {
		if _, err := parseStep(reply); err == nil {
			t.Errorf("%s: accepted a reply carrying no step", name)
		}
	}
}
