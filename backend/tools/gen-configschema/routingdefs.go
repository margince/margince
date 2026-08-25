// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gradionhq/margince/backend/internal/modules/ai"
)

// The model-binding shape, carried under $defs.
//
// It used to be config/ai-routing.schema.json, a file of its own, which was
// right while a routing FILE existed. The binding now lives in this document
// under `seeds.ai_routing` — and, once an installation is running, in the
// database — so its shape belongs where the thing it describes is written.
//
// Held as literals rather than assembled through the builder the rest of this
// generator uses: these descriptions are hand-tuned and the key order is
// deliberate, and round-tripping them would churn the file on every
// regeneration for no reader's benefit. What is NOT literal is the tier enum,
// which comes from the task contract through ai.AllTiers.
const routingDefsTemplate = `{
  "aiRouting": {
    "description": "The tier-to-model binding a fresh installation is bootstrapped with. Consumed ONCE, at bootstrap: a running installation is rebound through Settings -> AI, and editing this afterwards changes nothing until the database is rebuilt.",
"type": "object",
"additionalProperties": false,
"required": ["profile", "tiers", "embeddings"],
"properties": {
  "profile": {
    "description": "Location ladder: eu_hosted (partner EU inference), sovereign (zero egress — cloud providers refused), cloud_frontier (BYOK cloud).",
    "enum": ["eu_hosted", "sovereign", "cloud_frontier"]
  },
  "tiers": {
    "description": "Capability tiers; bind each to one provider. An unbound tier is legal — the router degrades honestly.",
    "type": "object",
    "minProperties": 1,
    "additionalProperties": false,
    "propertyNames": { "enum": [__TIERS__] },
    "patternProperties": { ".*": { "$ref": "#/$defs/binding" } }
  },
  "embeddings": {
    "description": "The embedding lane, bound separately from chat (retrieval must survive a chat-budget exhaustion). Required.",
    "$ref": "#/$defs/embeddingsBinding"
  }
}
  },

  "binding": {
    "type": "object",
    "additionalProperties": false,
    "required": ["provider"],
    "properties": {
      "provider": {
        "description": "fake | anthropic | ollama | vllm | openai_compatible | openai | gemini. The only place vendor names appear.",
        "enum": ["fake", "anthropic", "ollama", "vllm", "openai_compatible", "openai", "gemini"]
      },
      "model":    { "type": "string", "description": "Provider-native model id. ollama/vllm default to a Gemma-class model when omitted (A23)." },
      "base_url": { "type": "string", "description": "Endpoint override. REQUIRED for openai_compatible (the vendor host root, NO /v1). Empty ⇒ provider default." },
      "input": {
        "description": "What the bound model can be GIVEN. On openai_compatible/vllm it is the whole answer (the carriage depends on which model was bound). On every other provider it NARROWS the carriage fixed in that adapter's wire — at most what the wire carries, at most what is declared — so it can take pdf away from a gemini tier and can never add a lane a wire lacks. Omit to take whatever the provider carries; write [text] to send it no attachments. Must include text.",
        "type": "array",
        "minItems": 1,
        "uniqueItems": true,
        "contains": { "const": "text" },
        "items": { "enum": ["text", "image"] }
      }
    },
    "allOf": [
      {
        "if":   { "properties": { "provider": { "const": "openai_compatible" } } },
        "then": { "required": ["base_url"] }
      }
    ]
  },
  "embeddingsBinding": {
    "description": "The embeddings-lane binding: $defs/binding plus dimensions (a width only this lane has) and minus input (this lane sends no attachments). Two lanes with different fields, so each gets its own $def rather than one widened additionalProperties:false that would accept either field on either lane.",
    "type": "object",
    "additionalProperties": false,
    "required": ["provider"],
    "properties": {
      "provider": {
        "description": "fake | anthropic | ollama | vllm | openai_compatible | openai | gemini. The only place vendor names appear.",
        "enum": ["fake", "anthropic", "ollama", "vllm", "openai_compatible", "openai", "gemini"]
      },
      "model":    { "type": "string", "description": "Provider-native model id. ollama/vllm default to a Gemma-class model when omitted (A23)." },
      "base_url": { "type": "string", "description": "Endpoint override. REQUIRED for openai_compatible (the vendor host root, NO /v1). Empty ⇒ provider default." },
      "dimensions": { "type": "integer", "minimum": 0, "maximum": 2000, "description": "Vector width the provider is asked to emit. Optional; 0 or omitted defaults to 1536." }
    },
    "allOf": [
      {
        "if":   { "properties": { "provider": { "const": "openai_compatible" } } },
        "then": { "required": ["base_url"] }
      }
    ]
  }
}`

// routingDefs renders the $defs block with the tier names the contract declares.
func routingDefs() json.RawMessage {
	tiers := ai.AllTiers()
	quoted := make([]string, len(tiers))
	for i, t := range tiers {
		quoted[i] = fmt.Sprintf("%q", string(t))
	}
	raw := strings.Replace(routingDefsTemplate, "__TIERS__", strings.Join(quoted, ", "), 1)
	// Validated here so a substitution bug fails generation rather than shipping
	// a schema no editor can load.
	var probe any
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		fail(fmt.Errorf("gen-configschema: the routing $defs are not valid JSON: %w", err))
	}
	return json.RawMessage(raw)
}
