// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// What the EDITOR accepts, checked against what the parser accepts.
//
// config/ai-routing.schema.json is what a YAML language server reads while an
// operator types, so it is the first answer they get about whether a binding is
// legal — hours before a process ever boots. Its enums are drift-tested in
// schema_test.go, but an enum is not the interesting half: the `input:` rules
// are conditionals, and a conditional can be subtly inverted while every enum
// still matches.
//
// So this runs a real JSON Schema validator over the generated schema and
// asserts the same acceptances the parser makes. The two are allowed to differ
// in the MESSAGE they give; they are not allowed to differ in the ANSWER. A
// schema that green-lights a binding the parser refuses at boot is worse than
// no schema, because the operator was told it was fine.

import (
	"os"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/modules/ai"
)

func compiledRoutingSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	const path = "../config/margince.schema.json"
	raw, err := os.Open(path)
	if err != nil {
		t.Fatalf("open schema: %v", err)
	}
	defer func() {
		if cerr := raw.Close(); cerr != nil {
			t.Errorf("close schema: %v", cerr)
		}
	}()
	doc, err := jsonschema.UnmarshalJSON(raw)
	if err != nil {
		t.Fatalf("parse schema: %v", err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("margince.json", doc); err != nil {
		t.Fatalf("add schema: %v", err)
	}
	// The routing shape is a subtree now, so this compiles the pointer to it
	// rather than the whole document: the cases below write a BINDING, not a
	// whole margince.yaml.
	sch, err := c.Compile("margince.json#/$defs/aiRouting")
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}
	return sch
}

// The `input:` acceptance matrix, asserted against the editor's authority and
// the runtime's in one table so the two cannot drift apart silently.
//
// Held by: TestTheSchemaAndTheParserAgreeOnEveryInputDeclaration (backend/gates/airoutingschema_test.go) — this test.
func TestTheSchemaAndTheParserAgreeOnEveryInputDeclaration(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "k")
	t.Setenv("OPENAI_COMPATIBLE_API_KEY", "k")
	sch := compiledRoutingSchema(t)

	tiered := func(binding string) string {
		return "profile: eu_hosted\ntiers:\n  premium: {" + binding + "}\nembeddings: {provider: gemini, model: e}\n"
	}
	for name, tc := range map[string]struct {
		yaml  string
		legal bool
	}{
		// Every provider takes the field: on the OpenAI-compatible pair it IS the
		// carriage, on the rest it narrows the carriage their wire already has.
		"declared on openai_compatible": {tiered(`provider: openai_compatible, base_url: https://x, model: m, input: [text, image]`), true},
		"declared on vllm":              {tiered(`provider: vllm, model: m, input: [text, image]`), true},
		"declared on gemini":            {tiered(`provider: gemini, model: m, input: [text, image]`), true},
		"declared on anthropic":         {tiered(`provider: anthropic, model: m, input: [text, image]`), true},
		"declared on openai":            {tiered(`provider: openai, model: m, input: [text, image]`), true},
		"declared on ollama":            {tiered(`provider: ollama, model: m, input: [text, image]`), true},
		// The narrowing spelling: a native tier told to send no attachment.
		"narrowed to text on gemini": {tiered(`provider: gemini, model: m, input: [text]`), true},
		// Omitting it is the text-only default every existing config relies on.
		"omitted on gemini":            {tiered(`provider: gemini, model: m`), true},
		"omitted on openai_compatible": {tiered(`provider: openai_compatible, base_url: https://x, model: m`), true},
		// The value rules.
		"unknown modality":  {tiered(`provider: vllm, model: m, input: [text, pdf]`), false},
		"missing text":      {tiered(`provider: vllm, model: m, input: [image]`), false},
		"empty list":        {tiered(`provider: vllm, model: m, input: []`), false},
		"repeated modality": {tiered(`provider: vllm, model: m, input: [text, image, image]`), false},
		// The schema rejects a null because `input` is typed as an array; the
		// parser has to look at the document to see the difference between a
		// blank key and an absent one. Both forms belong here precisely because
		// that is where the two authorities could most easily part company.
		"explicit null": {tiered(`provider: vllm, model: m, input: null`), false},
		"bare key":      {"profile: eu_hosted\ntiers:\n  premium:\n    provider: vllm\n    model: m\n    input:\nembeddings: {provider: gemini, model: e}\n", false},
		"null on embeddings": {
			"profile: eu_hosted\ntiers:\n  premium: {provider: gemini, model: m}\n" +
				"embeddings: {provider: gemini, model: e, input: null}\n", false,
		},
		// The embeddings lane sends no attachments.
		"declared on the embeddings lane": {
			"profile: eu_hosted\ntiers:\n  premium: {provider: gemini, model: m}\n" +
				"embeddings: {provider: openai_compatible, base_url: https://x, model: e, input: [text, image]}\n", false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			var doc any
			if err := yaml.Unmarshal([]byte(tc.yaml), &doc); err != nil {
				t.Fatalf("fixture is not yaml: %v", err)
			}
			// The validator works on JSON types; yaml.v3 already decodes into
			// map[string]any for string keys, which is what it expects.
			schemaOK := sch.Validate(doc) == nil
			_, parseErr := ai.ParseRouting([]byte(tc.yaml))
			parserOK := parseErr == nil

			if schemaOK != tc.legal {
				t.Errorf("schema accepted=%v, want %v", schemaOK, tc.legal)
			}
			if parserOK != tc.legal {
				t.Errorf("parser accepted=%v, want %v (err: %v)", parserOK, tc.legal, parseErr)
			}
			if schemaOK != parserOK {
				t.Errorf("the editor and the runtime disagree: schema accepted=%v, parser accepted=%v (err: %v)",
					schemaOK, parserOK, parseErr)
			}
		})
	}
}

// The `routing:` block's acceptance matrix, asserted against the editor's
// authority and the runtime's together.
//
// Same rule as `input:` above — the two may differ in the MESSAGE and never in
// the ANSWER. It matters more here than for most fields because the block's
// legality depends on TWO other keys (provider and base_url), and a schema that
// green-lit it on a direct vendor would send an operator to write something the
// parser refuses at boot.
//
// Held by: TestTheSchemaAndTheParserAgreeOnEveryUpstreamRoutingDeclaration (backend/gates/airoutingschema_test.go) — this test.
func TestTheSchemaAndTheParserAgreeOnEveryUpstreamRoutingDeclaration(t *testing.T) {
	t.Parallel()
	sch := compiledRoutingSchema(t)

	const broker = "provider: openai_compatible, model: m, base_url: 'https://openrouter.ai/api'"
	tiered := func(binding string) string {
		return "profile: eu_hosted\ntiers:\n  premium: {" + binding + "}\nembeddings: {provider: gemini, model: e}\n"
	}
	for name, tc := range map[string]struct {
		yaml  string
		legal bool
	}{
		// Omitted entirely: the common case, and the one that inherits the default.
		"no declaration at all": {tiered(broker), true},
		// The explicit opt-out. Legal, and distinct from the line above.
		"an empty declaration": {tiered(broker + ", routing: {}"), true},
		"the product default": {tiered(broker +
			", routing: {sort: throughput, quantizations: [fp16, bf16], require_parameters: true}"), true},
		"a slug allowlist":  {tiered(broker + ", routing: {only: [cerebras]}"), true},
		"an effort cap":     {tiered(broker + ", routing: {reasoning_effort: low}"), true},
		"a latency ceiling": {tiered(broker + ", routing: {preferred_max_latency_p90: 8}"), true},
		"fallbacks off":     {tiered(broker + ", routing: {allow_fallbacks: false}"), true},

		// Values the broker would silently drop, so both halves must refuse them.
		"an unknown sort":         {tiered(broker + ", routing: {sort: cheapest}"), false},
		"an unknown quantization": {tiered(broker + ", routing: {quantizations: [fp5]}"), false},
		"an unknown effort":       {tiered(broker + ", routing: {reasoning_effort: lots}"), false},
		"an unknown preference":   {tiered(broker + ", routing: {sort_by: price}"), false},
		// 0 is LEGAL and means unset. Once this binding has round-tripped through
		// the settings store as JSON a written 0 and an absent key are the same
		// value, so neither half can refuse one without refusing the other — the
		// same limit `input:` runs into one field over.
		"a zero latency ceiling": {tiered(broker + ", routing: {preferred_max_latency_p90: 0}"), true},

		// The block cannot be honoured here: a native vendor fronts one host.
		"a block on a native vendor": {
			tiered("provider: gemini, model: m, routing: {sort: throughput}"), false,
		},
		// Nor here: these are OpenRouter's own fields.
		"a block on the wire pointed elsewhere": {
			tiered("provider: openai_compatible, model: m, base_url: 'https://api.mistral.ai', routing: {sort: throughput}"), false,
		},
		// The embeddings lane has no tail to bound.
		"a block on the embeddings lane": {
			"profile: eu_hosted\ntiers:\n  premium: {" + broker + "}\n" +
				"embeddings: {provider: openai_compatible, model: e, base_url: 'https://openrouter.ai/api', routing: {sort: throughput}}\n", false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			var doc any
			if err := yaml.Unmarshal([]byte(tc.yaml), &doc); err != nil {
				t.Fatalf("test yaml is not yaml: %v", err)
			}
			schemaAccepts := sch.Validate(doc) == nil
			_, parseErr := ai.ParseRouting([]byte(tc.yaml))
			parserAccepts := parseErr == nil

			if schemaAccepts != tc.legal {
				t.Errorf("the EDITOR accepts=%v, want %v — an operator is told the wrong thing hours before boot", schemaAccepts, tc.legal)
			}
			if parserAccepts != tc.legal {
				t.Errorf("the PARSER accepts=%v, want %v (err: %v)", parserAccepts, tc.legal, parseErr)
			}
		})
	}
}
