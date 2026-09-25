// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/modules/ai"
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
// which comes from the task contract through ai.AllTiers, and the decisions
// provider enum, which comes from the provider registry through
// ai.DecisionProviders.
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
  },
  "decisions": {
    "description": "The decision-model lane. Optional: absent, every call is the task's own ladder. Present, a task that declares a decision form is asked it first, on a certified site, and falls back to its ladder whenever the answer does not stand.",
    "$ref": "#/$defs/decisionsBinding"
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
        "description": "What the bound model can be GIVEN. On openai_compatible/vllm it is the whole answer (the carriage depends on which model was bound). On every other provider it NARROWS the carriage fixed in that adapter's wire — at most what the wire carries, at most what is declared — so it can take image away from a gemini tier and can never add a lane a wire lacks. Omit to take whatever the provider carries; write [text] to send it no attachments. Must include text.",
        "type": "array",
        "minItems": 1,
        "uniqueItems": true,
        "contains": { "const": "text" },
        "items": { "enum": ["text", "image"] }
      },
      "routing": { "$ref": "#/$defs/upstreamRouting" },
      "thinking_level": {
        "description": "How deeply a gemini tier thinks when the request names no level of its own. Omit it for the adapter's default: a structured request thinks at low, and a Flash-Lite keeps its own shallower default (minimal), which low RAISES. Gemini charges thinking to the same output ceiling as the answer. Gemini 3 or later only — a Gemini 2.5 answers the field with a 400, and gemini-3.1-pro-preview refuses minimal.",
        "enum": ["minimal", "low", "medium", "high"]
      }
    },
    "allOf": [
      {
        "if":   { "properties": { "provider": { "const": "openai_compatible" } } },
        "then": { "required": ["base_url"] }
      },
      { "$ref": "#/$defs/routingNeedsOpenRouter" },
      {
        "if":   { "required": ["thinking_level"] },
        "then": { "properties": { "provider": { "const": "gemini" } } }
      }
    ]
  },
  "routingNeedsOpenRouter": {
    "description": "A routing block is OpenRouter's own fields, so a lane may declare one only when it is an openai_compatible binding whose base_url is an OpenRouter host. One clause for both lanes, so the chat tiers and the embeddings lane cannot come to disagree about which host that is. The pattern is case-insensitive because URL hosts are, and the parser lowercases the host before it compares.",
    "if": { "required": ["routing"] },
    "then": {
      "properties": {
        "provider": { "const": "openai_compatible" },
        "base_url": { "pattern": "^[Hh][Tt][Tt][Pp][Ss]?://([^/]*\\.)?[Oo][Pp][Ee][Nn][Rr][Oo][Uu][Tt][Ee][Rr]\\.[Aa][Ii](:[0-9]+)?(/|$)" }
      },
      "required": ["provider", "base_url"]
    }
  },
  "upstreamRouting": {
    "description": "Which of a broker's upstream hosts may serve this tier. OpenRouter fronts many inference hosts per model, and its own default picks among them weighted by the inverse square of price — so one model id is served at fp4 on one call and at bf16 on the next, with latency to match. Valid ONLY on an openai_compatible binding whose base_url is an OpenRouter host; the parser refuses it anywhere else rather than send a vendor fields it never asked for. OMIT the block to inherit the product default (sort: throughput, quantizations: [fp16, bf16], require_parameters: true — reliability over price); write an empty object to opt out and take the broker's own price-weighted routing. Measured 2026-09-02 — see docs/reference/openrouter.md.",
    "type": "object",
    "additionalProperties": false,
    "properties": {
      "sort": {
        "description": "Order candidate hosts by throughput, price or latency. This is the lever that collapses the latency tail, and it disables the broker's load balancing, which is the trade. throughput measured best; latency reached slower hosts; price is roughly what the unpinned default already does.",
        "enum": ["throughput", "price", "latency"]
      },
      "quantizations": {
        "description": "Serving precisions a host may use. A HARD filter, and what makes repeated calls comparable: two answers from one model id at fp4 and at bf16 are two different models for every purpose except billing.",
        "type": "array",
        "minItems": 1,
        "uniqueItems": true,
        "items": { "enum": ["int4", "int8", "fp4", "mxfp4", "nvfp4", "fp6", "fp8", "mxfp8", "fp16", "bf16", "fp32", "unknown"] }
      },
      "require_parameters": {
        "description": "Route only to hosts supporting every parameter the request carries. Belt-and-braces for a structured-output call: a soft preference already covers response_format most of the time, and this makes it a rule.",
        "type": "boolean"
      },
      "only": {
        "description": "Allowlist of upstream host slugs; a base slug matches every region and variant, a full slug such as deepinfra/turbo pins one endpoint. A HARD filter. Prefer sort — pinning by slug reaches the same host while throwing away the failover breadth a sort leaves intact.",
        "type": "array",
        "minItems": 1,
        "uniqueItems": true,
        "items": { "type": "string" }
      },
      "ignore": {
        "description": "Blocklist of upstream host slugs. A HARD filter, for excluding one endpoint measured bad rather than for choosing among the rest.",
        "type": "array",
        "minItems": 1,
        "uniqueItems": true,
        "items": { "type": "string" }
      },
      "allow_fallbacks": {
        "description": "Whether the broker may switch hosts when the chosen one fails. Omit to keep its own default of true; set false only when a compliance rule or a measurement run demands a single host, because it trades availability for certainty.",
        "type": "boolean"
      },
      "preferred_max_latency_p90": {
        "description": "Deprioritize hosts whose p90 latency over a rolling five-minute window exceeds this many seconds. SOFT: it reorders and never excludes, so it cannot bound a tail. Measured on its own it was worse than setting nothing — p90 43.7s, including one 231s call. Omit it, or 0, to leave it unset: once this binding has round-tripped through the settings store as JSON a written 0 and an absent key are the same value, so the runtime reads both as unset and this schema may not pretend otherwise.",
        "type": "number",
        "minimum": 0
      },
      "reasoning_effort": {
        "description": "Cap a reasoning model's thinking budget. Unset means each host applies its own default. Halves latency and cost, and cost a fifth of the certification score on a drafting task, so set it where throughput dominates rather than by default.",
        "enum": ["max", "xhigh", "high", "medium", "low", "minimal", "none"]
      }
    }
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
      "dimensions": { "type": "integer", "minimum": 0, "maximum": 2000, "description": "Vector width the provider is asked to emit. Optional; 0 or omitted defaults to 1536." },
      "routing": { "$ref": "#/$defs/embeddingsRouting" }
    },
    "allOf": [
      {
        "if":   { "properties": { "provider": { "const": "openai_compatible" } } },
        "then": { "required": ["base_url"] }
      },
      { "$ref": "#/$defs/routingNeedsOpenRouter" }
    ]
  },
  "decisionsBinding": {
    "description": "The decisions-lane binding: a provider that answers the decision wire, its model and its endpoint. No input and no routing: the lane sends no attachments and the decisions endpoint takes no broker preferences.",
    "type": "object",
    "additionalProperties": false,
    "required": ["provider", "model"],
    "properties": {
      "provider": {
        "description": "openrouter_decision (OpenRouter's decisions endpoint, reached with the openai_compatible key; base_url must be an OpenRouter host) | laya (a same-host decision encoder; keyless, sovereign-eligible, defaults to http://127.0.0.1:8765).",
        "enum": [__DECISION_PROVIDERS__]
      },
      "model":    { "type": "string", "minLength": 1, "description": "The decision model id, e.g. typesafe/jev-1.13, or a Laya checkpoint name." },
      "base_url": { "type": "string", "description": "Endpoint root. REQUIRED for openrouter_decision (e.g. https://openrouter.ai/api). Empty ⇒ provider default." }
    },
    "allOf": [
      {
        "if":   { "properties": { "provider": { "const": "openrouter_decision" } } },
        "then": { "required": ["base_url"] }
      }
    ]
  },
  "embeddingsRouting": {
    "description": "Which of a broker's upstream hosts may read the text this lane embeds. Only the host-selection fields: the lane embeds the same text the chat tiers send, so a residency pin must reach it too, and the other upstreamRouting fields bound a completion's tail, which a single forward pass does not have. Valid only on an openai_compatible binding whose base_url is an OpenRouter host. Omit it to leave the broker's own choice of host.",
    "type": "object",
    "additionalProperties": false,
    "properties": {
      "only":            { "$ref": "#/$defs/upstreamRouting/properties/only" },
      "ignore":          { "$ref": "#/$defs/upstreamRouting/properties/ignore" },
      "allow_fallbacks": { "$ref": "#/$defs/upstreamRouting/properties/allow_fallbacks" }
    }
  }
}`

// routingDefs renders the $defs block with the tier names the contract declares
// and the decision providers the registry holds.
func routingDefs() json.RawMessage {
	tiers := ai.AllTiers()
	names := make([]string, len(tiers))
	for i, t := range tiers {
		names[i] = string(t)
	}
	raw := strings.Replace(routingDefsTemplate, "__TIERS__", quotedList(names), 1)
	raw = strings.Replace(raw, "__DECISION_PROVIDERS__", quotedList(ai.DecisionProviders()), 1)
	// Validated here so a substitution bug fails generation rather than shipping
	// a schema no editor can load.
	var probe any
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		fail(fmt.Errorf("gen-configschema: the routing $defs are not valid JSON: %w", err))
	}
	return json.RawMessage(raw)
}

// quotedList is names as JSON string literals, comma-separated, for an enum
// the template leaves a placeholder for.
func quotedList(names []string) string {
	quoted := make([]string, len(names))
	for i, name := range names {
		quoted[i] = fmt.Sprintf("%q", name)
	}
	return strings.Join(quoted, ", ")
}
