// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// What the editor accepts, checked against what the parser accepts.
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
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"testing"
	"time"

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
	// cloud_frontier, because this compares the per-binding shape.
	tiered := func(binding string) string {
		return "profile: cloud_frontier\ntiers:\n  premium: {" + binding + "}\nembeddings: {provider: gemini, model: e}\n"
	}
	embedded := func(routing string) string {
		return "profile: cloud_frontier\ntiers:\n  premium: {" + broker + "}\n" +
			"embeddings: {provider: openai_compatible, model: e, base_url: 'https://openrouter.ai/api', " + routing + "}\n"
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
		// Shapes the schema declares (minItems, uniqueItems) that the parser has
		// to refuse too — an editor and a runtime that authorize different
		// configs is exactly the drift this test exists to catch.
		"a written-but-empty allowlist": {tiered(broker + ", routing: {only: []}"), false},
		"a repeated quantization":       {tiered(broker + ", routing: {quantizations: [bf16, bf16]}"), false},
		// The host is case-insensitive in URLs and IsOpenRouterHost lowercases
		// it, so the schema's pattern must not be the stricter half.
		"an uppercase broker host": {tiered(
			"provider: openai_compatible, model: m, base_url: 'https://OPENROUTER.AI/api', routing: {sort: throughput}"), true},
		// A threshold json cannot encode: it would boot and then fail every call.
		"a non-finite latency ceiling": {tiered(broker + ", routing: {preferred_max_latency_p90: .nan}"), false},
		// An explicit false is a real choice, not an empty block.
		"require_parameters written false": {tiered(broker + ", routing: {require_parameters: false}"), true},
		// 0 is LEGAL and means unset. Once this binding has round-tripped through
		// the settings store as JSON a written 0 and an absent key are the same
		// value, so neither half can refuse one without refusing the other — the
		// same limit `input:` runs into one field over.
		"a zero latency ceiling": {tiered(broker + ", routing: {preferred_max_latency_p90: 0}"), true},

		// OpenRouter's own nested request shape, every field the parser reads.
		"a nested sort":                    {tiered(broker + ", routing: {provider: {sort: latency}}"), true},
		"a nested sort with a partition":   {tiered(broker + ", routing: {provider: {sort: {by: price, partition: none}}}"), true},
		"a host order":                     {tiered(broker + ", routing: {provider: {order: [groq, cerebras]}}"), true},
		"a price ceiling":                  {tiered(broker + ", routing: {provider: {max_price: {prompt: 0.5, completion: 2}}}"), true},
		"a throughput floor":               {tiered(broker + ", routing: {provider: {preferred_min_throughput: {p50: 40}}}"), true},
		"one latency for every percentile": {tiered(broker + ", routing: {provider: {preferred_max_latency: 3}}"), true},
		"a reasoning budget":               {tiered(broker + ", routing: {reasoning: {max_tokens: 512, exclude: true}}"), true},
		"an empty nested declaration":      {tiered(broker + ", routing: {provider: {}}"), true},
		// A seed writing the connection's keys on a tier has them lifted, as the
		// flat spelling always was; the settings API is where they are refused.
		"a nested privacy key on a tier": {tiered(broker + ", routing: {provider: {zdr: true}}"), true},

		"an unknown nested key":               {tiered(broker + ", routing: {provider: {sort_by: price}}"), false},
		"an unknown reasoning key":            {tiered(broker + ", routing: {reasoning: {budget: 10}}"), false},
		"an unknown sort partition":           {tiered(broker + ", routing: {provider: {sort: {by: price, partition: all}}}"), false},
		"a sort object with no order":         {tiered(broker + ", routing: {provider: {sort: {partition: none}}}"), false},
		"an unknown data collection":          {tiered(broker + ", routing: {provider: {data_collection: maybe}}"), false},
		"a negative price ceiling":            {tiered(broker + ", routing: {provider: {max_price: {prompt: -1}}}"), false},
		"an unknown percentile":               {tiered(broker + ", routing: {provider: {preferred_max_latency: {p95: 3}}}"), false},
		"a percentile object with none":       {tiered(broker + ", routing: {provider: {preferred_max_latency: {}}}"), false},
		"effort and a token budget":           {tiered(broker + ", routing: {reasoning: {effort: low, max_tokens: 10}}"), false},
		"a zero token budget":                 {tiered(broker + ", routing: {reasoning: {max_tokens: 0}}"), false},
		"the flat and nested spellings mixed": {tiered(broker + ", routing: {sort: price, provider: {order: [groq]}}"), false},
		"a nested order written empty":        {tiered(broker + ", routing: {provider: {order: []}}"), false},

		// The connection takes its own keys in either spelling, and nothing else.
		"privacy on the connection": {
			"profile: cloud_frontier\nproviders:\n  openai_compatible: {base_url: 'https://openrouter.ai/api', upstream: {provider: {zdr: true, data_collection: deny, only: [mistral]}}}\n" +
				"tiers:\n  premium: {provider: openai_compatible, model: m}\nembeddings: {provider: gemini, model: e}\n", true,
		},
		"a serving key on the connection": {
			"profile: cloud_frontier\nproviders:\n  openai_compatible: {base_url: 'https://openrouter.ai/api', upstream: {provider: {sort: price}}}\n" +
				"tiers:\n  premium: {provider: openai_compatible, model: m}\nembeddings: {provider: gemini, model: e}\n", false,
		},
		"a nested host pin on the embeddings lane":    {embedded("routing: {provider: {only: [mistral/eu]}}"), true},
		"a nested privacy key on the embeddings lane": {embedded("routing: {provider: {zdr: true}}"), true},
		"a nested serving key on the embeddings lane": {embedded("routing: {provider: {order: [groq]}}"), false},

		// The block cannot be honoured here: a native vendor fronts one host.
		"a block on a native vendor": {
			tiered("provider: gemini, model: m, routing: {sort: throughput}"), false,
		},
		// Nor here: these are OpenRouter's own fields.
		"a block on the wire pointed elsewhere": {
			tiered("provider: openai_compatible, model: m, base_url: 'https://api.mistral.ai', routing: {sort: throughput}"), false,
		},
		// The embeddings lane has no tail to bound, so a tail preference is
		// refused there — but WHICH hosts may read the text is a question it
		// shares with every chat tier, and a residency pin must reach it.
		"a tail preference on the embeddings lane": {embedded("routing: {sort: throughput}"), false},
		"a host pin on the embeddings lane":        {embedded("routing: {only: [mistral/eu]}"), true},
		"a host blocklist on the embeddings lane":  {embedded("routing: {ignore: [deepinfra], allow_fallbacks: false}"), true},
		"an effort cap on the embeddings lane":     {embedded("routing: {reasoning_effort: low}"), false},
		// The embeddings lane shares the chat tiers' host rule: a direct vendor
		// on the OpenAI wire fronts one host, so a pin there has nothing to pick.
		"a host pin on an embeddings lane pointed elsewhere": {
			"profile: cloud_frontier\ntiers:\n  premium: {" + broker + "}\n" +
				"embeddings: {provider: openai_compatible, model: e, base_url: 'https://api.mistral.ai', routing: {only: [x]}}\n", false,
		},
		"a host pin on a native embeddings lane": {
			"profile: cloud_frontier\ntiers:\n  premium: {" + broker + "}\nembeddings: {provider: gemini, model: e, routing: {only: [x]}}\n", false,
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

// The `location:` matrix. A Vertex host is built from the location, so the
// editor must refuse every spelling the parser refuses, and on the same lanes.
func TestTheSchemaAndTheParserAgreeOnEveryVertexPlacement(t *testing.T) {
	t.Parallel()
	sch := compiledRoutingSchema(t)

	const embedder = "{provider: gemini, model: e}"
	routing := func(tier, embeddings string) string {
		return "profile: cloud_frontier\ntiers:\n  premium: " + tier + "\nembeddings: " + embeddings + "\n"
	}
	// A lane may take its location from providers.gemini_vertex, which the
	// schema cannot follow from the lane; for a lane naming none, only the
	// parser, reading the resolved lane, can say whether it has one.
	onProvider := func(location, tier string) string {
		return "profile: cloud_frontier\nproviders:\n  gemini_vertex: {location: " + location + "}\ntiers:\n  premium: " + tier + "\nembeddings: " + embedder + "\n"
	}
	for name, tc := range map[string]struct {
		yaml          string
		legal         bool
		editorLenient bool
	}{
		"the EU multi-region":        {routing("{provider: gemini_vertex, model: m, location: eu}", embedder), true, false},
		"a region":                   {routing("{provider: gemini_vertex, model: m, location: europe-west4}", embedder), true, false},
		"global":                     {routing("{provider: gemini_vertex, model: m, location: global}", embedder), true, false},
		"the US multi-region":        {routing("{provider: gemini_vertex, model: m, location: us}", embedder), true, false},
		"a region with no number":    {routing("{provider: gemini_vertex, model: m, location: europe-west}", embedder), false, false},
		"a region with no area":      {routing("{provider: gemini_vertex, model: m, location: west4}", embedder), false, false},
		"no location":                {routing("{provider: gemini_vertex, model: m}", embedder), false, true},
		"a location on the provider": {onProvider("europe-west4", "{provider: gemini_vertex, model: m}"), true, false},
		"a provider location the parser refuses": {
			onProvider("'eu.attacker.example'", "{provider: gemini_vertex, model: m}"), false, false,
		},
		"a base_url beside it":       {routing("{provider: gemini_vertex, model: m, location: eu, base_url: 'https://x.example'}", embedder), false, false},
		"a host smuggled in":         {routing("{provider: gemini_vertex, model: m, location: 'eu.attacker.example'}", embedder), false, false},
		"an uppercase location":      {routing("{provider: gemini_vertex, model: m, location: EU}", embedder), false, false},
		"a location on another wire": {routing("{provider: gemini, model: m, location: eu}", embedder), false, false},
		"the embeddings lane":        {routing(embedder, "{provider: gemini_vertex, model: e, location: eu}"), true, false},
		"the embeddings lane with no location": {
			routing(embedder, "{provider: gemini_vertex, model: e}"), false, true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var doc any
			if err := yaml.Unmarshal([]byte(tc.yaml), &doc); err != nil {
				t.Fatalf("test yaml is not yaml: %v", err)
			}
			schemaAccepts := sch.Validate(doc) == nil
			_, parseErr := ai.ParseRouting([]byte(tc.yaml))
			if want := tc.legal || tc.editorLenient; schemaAccepts != want {
				t.Errorf("the EDITOR accepts=%v, want %v", schemaAccepts, want)
			}
			if parserAccepts := parseErr == nil; parserAccepts != tc.legal {
				t.Errorf("the PARSER accepts=%v, want %v (err: %v)", parserAccepts, tc.legal, parseErr)
			}
		})
	}
}

// The `thinking_level` acceptance matrix, editor and runtime together. Its
// legality depends on the provider, so like `routing:` it is a conditional that
// can be inverted while its enum still matches.
//
// The parser also refuses it on a model that predates the field, which a schema
// cannot see from a model id; that refusal is the ai package's own test.
//
// Held by: TestTheSchemaAndTheParserAgreeOnEveryThinkingLevel (backend/gates/airoutingschema_test.go) — this test.
func TestTheSchemaAndTheParserAgreeOnEveryThinkingLevel(t *testing.T) {
	t.Parallel()
	sch := compiledRoutingSchema(t)

	tiered := func(binding string) string {
		return "profile: cloud_frontier\ntiers:\n  cheap_cloud: {" + binding + "}\nembeddings: {provider: gemini, model: e}\n"
	}
	for name, tc := range map[string]struct {
		yaml  string
		legal bool
	}{
		"omitted":                       {tiered("provider: gemini, model: gemini-3.1-flash-lite"), true},
		"low on a flash-lite":           {tiered("provider: gemini, model: gemini-3.1-flash-lite, thinking_level: low"), true},
		"minimal":                       {tiered("provider: gemini, model: gemini-3.5-flash, thinking_level: minimal"), true},
		"high":                          {tiered("provider: gemini, model: gemini-3.5-flash, thinking_level: high"), true},
		"an unknown level":              {tiered("provider: gemini, model: gemini-3.5-flash, thinking_level: lots"), false},
		"an effort word Gemini lacks":   {tiered("provider: gemini, model: gemini-3.5-flash, thinking_level: none"), false},
		"on a provider without it":      {tiered("provider: anthropic, model: m, thinking_level: low"), false},
		"on the OpenAI-compatible wire": {tiered("provider: openai_compatible, base_url: https://x, model: m, thinking_level: low"), false},
		"on the Gemini wire on Vertex":  {tiered("provider: gemini_vertex, location: eu, model: gemini-3.5-flash, thinking_level: low"), true},
		"on the embeddings lane": {
			"profile: cloud_frontier\ntiers:\n  cheap_cloud: {provider: gemini, model: m}\n" +
				"embeddings: {provider: gemini, model: e, thinking_level: low}\n", false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			var doc any
			if err := yaml.Unmarshal([]byte(tc.yaml), &doc); err != nil {
				t.Fatalf("test yaml is not yaml: %v", err)
			}
			schemaAccepts := sch.Validate(doc) == nil
			_, parseErr := ai.ParseRouting([]byte(tc.yaml))
			if schemaAccepts != tc.legal {
				t.Errorf("the EDITOR accepts=%v, want %v", schemaAccepts, tc.legal)
			}
			if parserAccepts := parseErr == nil; parserAccepts != tc.legal {
				t.Errorf("the PARSER accepts=%v, want %v (err: %v)", parserAccepts, tc.legal, parseErr)
			}
		})
	}
}

// The admin screen is served the routing $defs from a copy embedded in the ai
// module at generation time. It must be this document, byte for byte in
// meaning, or the field reference would describe a schema nobody gates.
//
// Held by: TestTheServedRoutingSchemaIsTheGatedSchema (backend/gates/airoutingschema_test.go) — this test.
func TestTheServedRoutingSchemaIsTheGatedSchema(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../config/margince.schema.json")
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	var doc struct {
		Defs json.RawMessage `json:"$defs"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse schema: %v", err)
	}
	served := ai.RoutingSchemaDocument()
	var want, got any
	if err := json.Unmarshal(doc.Defs, &want); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(served, &got); err != nil {
		t.Fatalf("the served schema is not JSON: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("GET /ai/routing/schema serves a schema other than config/margince.schema.json's $defs; run make gen")
	}
}

// An override the contract accepts is one the server saves, and the reverse:
// the AiTaskOverride enum and bounds are a declared mirror of the Go owner.
func TestTheTaskOverrideContractIsTheServersBounds(t *testing.T) {
	t.Parallel()
	var doc struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]struct {
					Enum    []string `yaml:"enum"`
					Minimum int64    `yaml:"minimum"`
					Maximum int64    `yaml:"maximum"`
				} `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	raw, err := os.ReadFile("api/crm.yaml")
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing the contract: %v", err)
	}
	props := doc.Components.Schemas["AiTaskOverride"].Properties

	wire, levels := slices.Sorted(slices.Values(props["thinking"].Enum)), slices.Sorted(slices.Values(ai.TaskThinkingLevels))
	if len(wire) == 0 || !slices.Equal(wire, levels) {
		t.Errorf("AiTaskOverride.thinking is %v in the contract but the server accepts %v", wire, levels)
	}
	for field, want := range map[string][2]time.Duration{
		"decision_timeout_ms": {ai.MinDecisionTimeout, ai.MaxDecisionTimeout},
		"attempt_timeout_ms":  {ai.MinAttemptTimeout, ai.MaxAttemptTimeout},
	} {
		got := props[field]
		if got.Minimum != want[0].Milliseconds() || got.Maximum != want[1].Milliseconds() {
			t.Errorf("AiTaskOverride.%s is %d–%d ms in the contract but the server accepts %d–%d",
				field, got.Minimum, got.Maximum, want[0].Milliseconds(), want[1].Milliseconds())
		}
	}
}
