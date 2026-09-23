// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// A schema that already satisfies OpenAI's strict rules is sent strict, and one
// that does not is not.
//
// The case behind this: stage_evidence_extract on ministral-14b answered with
// no `claims` key on most of a 27-run certification, and on three of them once
// the schema it had been sent all along was actually enforced.
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
		"closed object listing every property": {
			schema: `{"type":"object","additionalProperties":false,"required":["subject","body"],
				"properties":{"subject":{"type":"string"},"body":{"type":"string"}}}`,
			strict: true,
		},
		// Closed, complete, and still refused: the strict profile has no
		// `uniqueItems`, and an endpoint answers one with a 400 rather than
		// ignoring it. This is the case that turned a working call into a
		// failing one, so it is the case this file exists to keep.
		"closed object carrying uniqueItems": {
			schema: `{"type":"object","additionalProperties":false,"required":["ids"],
				"properties":{"ids":{"type":"array","uniqueItems":true,"items":{"type":"string"}}}}`,
			strict: false,
		},
		"closed object carrying a length bound": {
			schema: `{"type":"object","additionalProperties":false,"required":["subject"],
				"properties":{"subject":{"type":"string","maxLength":998}}}`,
			strict: false,
		},
		// enum and const hold VALUES; walking them as subschemas would read a
		// string as a shape, and refusing them would rule out every closed
		// vocabulary the tree writes.
		"closed object with an enum": {
			schema: `{"type":"object","additionalProperties":false,"required":["kind"],
				"properties":{"kind":{"type":"string","enum":["a","b"]}}}`,
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

		// Every case below reported ELIGIBLE under this check's first shape,
		// which recognised an object only by a literal "type":"object" and let
		// a failed type assertion stand for "nothing to object to". Each one
		// is a request the endpoint answers with a 400.
		"nested object declaring properties but no type": {
			schema: `{"type":"object","additionalProperties":false,"required":["i"],
				"properties":{"i":{"properties":{"a":{"type":"string"}}}}}`,
			strict: false,
		},
		"nullable object spelled as a union type": {
			schema: `{"type":["object","null"],"properties":{"a":{"type":"string"}}}`,
			strict: false,
		},
		"root that is not an object": {schema: `{"type":"string"}`, strict: false},
		"root anyOf":                 {schema: `{"anyOf":[{"type":"object","additionalProperties":false,"required":["a"],"properties":{"a":{"type":"string"}}}]}`, strict: false},
		"empty schema":               {schema: `{}`, strict: false},
		"null properties": {
			schema: `{"type":"object","additionalProperties":false,"properties":null}`,
			strict: false,
		},
		"a property whose schema is null": {
			schema: `{"type":"object","additionalProperties":false,"required":["a"],"properties":{"a":null}}`,
			strict: false,
		},
		"required that is not a list": {
			schema: `{"type":"object","additionalProperties":false,"required":"a","properties":{"a":{"type":"string"}}}`,
			strict: false,
		},
		// Counts match and every name is declared, and "b" is still unrequired.
		"required naming one property twice": {
			schema: `{"type":"object","additionalProperties":false,"required":["a","a"],
				"properties":{"a":{"type":"string"},"b":{"type":"string"}}}`,
			strict: false,
		},
		"required naming a property that does not exist": {
			schema: `{"type":"object","additionalProperties":false,"required":["a","ghost"],
				"properties":{"a":{"type":"string"}}}`,
			strict: false,
		},
		"items in tuple form": {
			schema: `{"type":"object","additionalProperties":false,"required":["a"],
				"properties":{"a":{"type":"array","items":[{"type":"string"}]}}}`,
			strict: false,
		},
		"items that is not a schema": {
			schema: `{"type":"object","additionalProperties":false,"required":["a"],
				"properties":{"a":{"type":"array","items":true}}}`,
			strict: false,
		},
		"additionalProperties given as a schema": {
			schema: `{"type":"object","additionalProperties":{"type":"string"},"required":["a"],
				"properties":{"a":{"type":"string"}}}`,
			strict: false,
		},
		// A reference is only as eligible as its target, and this walk does not
		// resolve one — so the keyword itself is refused rather than trusted.
		"dangling $ref": {
			schema: `{"type":"object","additionalProperties":false,"required":["a"],
				"properties":{"a":{"$ref":"#/$defs/Missing"}}}`,
			strict: false,
		},
		"a closed object with no properties at all": {
			schema: `{"type":"object","additionalProperties":false}`,
			strict: true,
		},
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
