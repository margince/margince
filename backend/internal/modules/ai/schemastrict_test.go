// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// A schema that already satisfies OpenAI's strict rules is sent strict, and one
// that does not is not.
//
// The case behind this: stage_evidence_extract on ministral-14b answered with
// no `claims` key on 15 of 27 certification runs, and on 3 once the schema it
// had been sent all along was actually enforced.
//
// The eligibility rule is what carries that, so it is what is tested here —
// not the wire's effect on any one model, which is a measurement and belongs
// in a certification record rather than in a unit test.

import (
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

func TestStrictIsDerivedFromTheSchemaNotAssumed(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		schema string
		strict bool
	}{
		// The reply draft's own schema, which is what the incident was about.
		"closed object listing every property": {
			schema: `{"type":"object","additionalProperties":false,"required":["subject","body"],
				"properties":{"subject":{"type":"string","maxLength":998},"body":{"type":"string"}}}`,
			strict: true,
		},
		"open object": {
			schema: `{"type":"object","required":["a"],"properties":{"a":{"type":"string"}}}`,
			strict: false,
		},
		"closed object omitting a property from required": {
			schema: `{"type":"object","additionalProperties":false,"required":["a"],
				"properties":{"a":{"type":"string"},"b":{"type":"string"}}}`,
			strict: false,
		},
		// Recursion is the half a shallow check misses: the outer object can be
		// impeccable while a nested one is open, and OpenAI rejects the whole
		// schema for the nested miss.
		"nested open object": {
			schema: `{"type":"object","additionalProperties":false,"required":["inner"],
				"properties":{"inner":{"type":"object","required":["a"],"properties":{"a":{"type":"string"}}}}}`,
			strict: false,
		},
		"nested closed object": {
			schema: `{"type":"object","additionalProperties":false,"required":["inner"],
				"properties":{"inner":{"type":"object","additionalProperties":false,"required":["a"],
					"properties":{"a":{"type":"string"}}}}}`,
			strict: true,
		},
		"array of open objects": {
			schema: `{"type":"object","additionalProperties":false,"required":["rows"],
				"properties":{"rows":{"type":"array","items":{"type":"object","required":["a"],
					"properties":{"a":{"type":"string"}}}}}}`,
			strict: false,
		},
		// Not a schema this adapter can reason about, so it must not claim the
		// stricter contract on the model's behalf.
		"unparseable": {schema: `{"type":`, strict: false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := schemaAllowsStrict(json.RawMessage(tc.schema)); got != tc.strict {
				t.Errorf("schemaAllowsStrict = %v, want %v", got, tc.strict)
			}
		})
	}
}

// The wire carries what the schema earned, which is the whole point: a caller
// that never wrote a strict-shaped schema keeps today's behaviour exactly, and
// one that did stops having enforcement withheld from it.
func TestTheWireCarriesTheDerivedStrictness(t *testing.T) {
	t.Parallel()
	closed := json.RawMessage(`{"type":"object","additionalProperties":false,
		"required":["subject"],"properties":{"subject":{"type":"string"}}}`)
	open := json.RawMessage(`{"type":"object","properties":{"subject":{"type":"string"}}}`)

	for name, tc := range map[string]struct {
		schema json.RawMessage
		strict bool
	}{"closed": {closed, true}, "open": {open, false}} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := &openAICompatClient{defaultModel: "m"}
			wire := c.chatWire(model.Request{ResponseSchema: tc.schema}, false)
			if wire.ResponseFormat == nil {
				t.Fatal("a request carrying a schema must carry a response_format")
			}
			if got := wire.ResponseFormat.JSONSchema.Strict; got != tc.strict {
				t.Errorf("wire strict = %v, want %v", got, tc.strict)
			}
		})
	}
}
