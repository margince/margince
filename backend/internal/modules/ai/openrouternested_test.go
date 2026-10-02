// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func TestARoutingRefusalNamesEveryPathInTheProblemBody(t *testing.T) {
	err := joinFaults(
		invalidAt("tiers.cheap_cloud.routing.provider.sort.by", `must be one of price, throughput, latency. You wrote "fastest".`),
		faultAt("tiers.cheap_cloud.routing.provider.zdr", CodeMovedToProvider, connectionKeyOnTier),
	)
	var faults apperrors.FieldFaults
	if !errors.As(err, &faults) {
		t.Fatalf("refusal is not FieldFaults: %T", err)
	}
	got := faults.FieldFaults()
	if len(got) != 2 || got[0].Field != "tiers.cheap_cloud.routing.provider.sort.by" || got[1].Code != CodeMovedToProvider {
		t.Fatalf("paths = %+v", got)
	}
	rec := httptest.NewRecorder()
	httperr.Write(rec, httptest.NewRequest(http.MethodPut, "/", nil), err)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "provider.zdr") {
		t.Fatalf("status %d body %s", rec.Code, rec.Body)
	}
}

func TestAFaultWithNoPathNamesTheRoutingSetting(t *testing.T) {
	err := joinFaults(nil, errors.New("ai: routing config: no tiers bound"))
	var faults routingFaults
	if !errors.As(err, &faults) || faults.FieldFaults()[0].Field != RoutingKey || faults[0].Code != settings.CodeInvalidValue {
		t.Fatalf("a pathless refusal = %+v", err)
	}
	if joinFaults(nil, nil) != nil {
		t.Fatal("joining no refusals is a refusal")
	}
}

func TestALegacyFlatValueSendsTheSameWireBytes(t *testing.T) {
	flat := `{"only":["cerebras"],"quantizations":["fp16","bf16"],"sort":"throughput","require_parameters":true,"allow_fallbacks":false,"preferred_max_latency_p90":4,"reasoning_effort":"low"}`
	var r OpenRouterRouting
	if err := json.Unmarshal([]byte(flat), &r); err != nil {
		t.Fatal(err)
	}
	gotP, _ := json.Marshal(r.providerWire())
	gotR, _ := json.Marshal(r.reasoningWire())
	// The bytes the flat wire struct produced for the same value before the
	// routing value took the nested shape.
	const wantP = `{"only":["cerebras"],"quantizations":["fp16","bf16"],"sort":"throughput","require_parameters":true,"allow_fallbacks":false,"preferred_max_latency":{"p90":4}}`
	const wantR = `{"effort":"low"}`
	if string(gotP) != wantP || string(gotR) != wantR {
		t.Fatalf("provider %s\nreasoning %s", gotP, gotR)
	}
}

// The binding digest keys every cached AI output, so a value the flat
// spelling can say is stored in it, byte for byte.
func TestAValueTheFlatSpellingCanSayIsStoredFlat(t *testing.T) {
	stored, err := json.Marshal(DefaultOpenRouterRouting())
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"only":null,"ignore":null,"quantizations":["fp16","bf16"],"sort":"throughput","require_parameters":true,"allow_fallbacks":null,"preferred_max_latency_p90":0,"reasoning_effort":""}`
	if string(stored) != want {
		t.Fatalf("stored %s", stored)
	}
	nested, err := json.Marshal(&OpenRouterRouting{Provider: OpenRouterProvider{Order: []string{"groq"}}})
	if err != nil || string(nested) != `{"provider":{"order":["groq"]}}` {
		t.Fatalf("a value only the nested spelling can say stored as %s (%v)", nested, err)
	}
}

func TestAnEmptyNestedValueIsStillTheOptOut(t *testing.T) {
	for _, in := range []string{`{}`, `{"provider":{}}`, `{"provider":{},"reasoning":{}}`} {
		var r OpenRouterRouting
		if err := json.Unmarshal([]byte(in), &r); err != nil {
			t.Fatal(in, err)
		}
		if !r.IsEmpty() || r.providerWire() != nil || r.reasoningWire() != nil {
			t.Fatalf("%s is not the opt-out", in)
		}
	}
}

func TestEveryOpenRouterServingFieldReachesTheWire(t *testing.T) {
	in := `{"provider":{"sort":{"by":"latency","partition":"none"},"preferred_max_latency":3,"order":["groq"],"max_price":{"prompt":0.5,"completion":1.5},"preferred_min_throughput":{"p50":50}},"reasoning":{"max_tokens":512,"exclude":true}}`
	var r OpenRouterRouting
	if err := json.Unmarshal([]byte(in), &r); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(struct {
		P *OpenRouterProvider  `json:"provider"`
		R *OpenRouterReasoning `json:"reasoning"`
	}{r.providerWire(), r.reasoningWire()})
	if string(b) != in {
		t.Fatalf("round trip\n got %s\nwant %s", b, in)
	}
	if err := r.Validate("x"); err != nil {
		t.Fatalf("a value of every serving field is refused: %v", err)
	}
}

func TestTheConnectionsPrivacyKeysReachTheWire(t *testing.T) {
	in := `{"provider":{"only":["mistral"],"ignore":["novita"],"allow_fallbacks":false,"data_collection":"deny","zdr":true,"enforce_distillable_text":true}}`
	r, err := parseRoutingAt("providers.openai_compatible.upstream", []byte(in))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(r.providerWire())
	if want := `{"only":["mistral"],"ignore":["novita"],"allow_fallbacks":false,"data_collection":"deny","zdr":true,"enforce_distillable_text":true}`; string(b) != want {
		t.Fatalf("wire %s", b)
	}
}

func TestEveryRefusalNamesItsPath(t *testing.T) {
	cases := map[string]string{
		`{"provider":{"sort":{"by":"fastest"}}}`:                 "x.provider.sort.by",
		`{"provider":{"sort":"fastest"}}`:                        "x.provider.sort",
		`{"provider":{"sort":{"by":"price","part":"x"}}}`:        "x.provider.sort.part",
		`{"provider":{"sort":{"by":"price","partition":"all"}}}`: "x.provider.sort.partition",
		`{"provider":{"quantizations":["fp9"]}}`:                 "x.provider.quantizations[0]",
		`{"provider":{"order":["a","a"]}}`:                       "x.provider.order[1]",
		`{"provider":{"only":[]}}`:                               "x.provider.only",
		`{"provider":{"bogus":1}}`:                               "x.provider.bogus",
		`{"provider":{"zdr":"yes"}}`:                             "x.provider.zdr",
		`{"provider":{"data_collection":"maybe"}}`:               "x.provider.data_collection",
		`{"provider":"fast"}`:                                    "x.provider",
		`{"model":"m"}`:                                          "x.model",
		`{"reasoning":{"effort":"low","max_tokens":10}}`:         "x.reasoning",
		`{"reasoning":{"effort":"lots"}}`:                        "x.reasoning.effort",
		`{"reasoning":{"max_tokens":0}}`:                         "x.reasoning.max_tokens",
		`{"reasoning":{"budget":10}}`:                            "x.reasoning.budget",
		`{"provider":{"max_price":{"prompt":-1}}}`:               "x.provider.max_price.prompt",
		`{"provider":{"max_price":{"tokens":1}}}`:                "x.provider.max_price.tokens",
		`{"provider":{"preferred_max_latency":{}}}`:              "x.provider.preferred_max_latency",
		`{"provider":{"preferred_max_latency":{"p95":1}}}`:       "x.provider.preferred_max_latency.p95",
		`{"provider":{"preferred_min_throughput":-5}}`:           "x.provider.preferred_min_throughput",
		`{"sort":"price","provider":{"sort":"latency"}}`:         "x",
		`{"sort":5}`: "x.sort",
		`[]`:         "x",
	}
	for in, path := range cases {
		_, err := parseRoutingAt("x", []byte(in))
		var f routingFaults
		if !errors.As(err, &f) || f[0].Path != path {
			t.Errorf("%s: got %v, want path %s", in, err, path)
		}
	}
}

// A YAML seed meets the same reader as the stored value, so the two doors
// cannot accept different configs.
func TestAYAMLSeedReadsBothSpellings(t *testing.T) {
	for doc, want := range map[string]string{
		"sort: latency\nreasoning_effort: low\n":                         `{"provider":{"sort":"latency"},"reasoning":{"effort":"low"}}`,
		"provider:\n  sort: {by: price, partition: none}\n  zdr: true\n": `{"provider":{"sort":{"by":"price","partition":"none"},"zdr":true}}`,
		"provider:\n  max_price: {prompt: 1}\n  only: [a, b]\n":          `{"provider":{"only":["a","b"],"max_price":{"prompt":1}}}`,
	} {
		var r OpenRouterRouting
		if err := yaml.Unmarshal([]byte(doc), &r); err != nil {
			t.Fatalf("%q: %v", doc, err)
		}
		got, _ := json.Marshal(struct {
			P *OpenRouterProvider  `json:"provider,omitempty"`
			R *OpenRouterReasoning `json:"reasoning,omitempty"`
		}{r.providerWire(), r.reasoningWire()})
		if string(got) != want {
			t.Errorf("%q read as %s, want %s", doc, got, want)
		}
	}
	var r OpenRouterRouting
	if err := yaml.Unmarshal([]byte("provider:\n  preferred_max_latency: .inf\n"), &r); err == nil {
		t.Error("a non-finite YAML number was accepted")
	}
}

func TestATierCannotCarryAConnectionKey(t *testing.T) {
	samples := map[string]string{
		"only": `["mistral"]`, "ignore": `["novita"]`, "allow_fallbacks": `false`,
		"zdr": `true`, "data_collection": `"deny"`, "enforce_distillable_text": `true`,
	}
	for _, key := range connectionKeys {
		next := storedOnBroker()
		lane := next.Tiers[TierCheapCloud]
		raw := fmt.Sprintf(`{"provider":{%q:%s}}`, key, samples[key])
		routing, err := parseRoutingAt(TierRoutingPath(TierCheapCloud), []byte(raw))
		if err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		lane.Routing = routing
		next.Tiers[TierCheapCloud] = lane
		_, _, err = next.replacing(storedOnBroker())
		wantConnectionKeyRefusal(t, err, "tiers.cheap_cloud.routing.provider."+key)
	}
}

func TestTheConnectionsPrivacyReachesEveryTiersWire(t *testing.T) {
	cfg := storedOnBroker()
	on := true
	conn := cfg.Providers[providerOpenAICompatible]
	conn.Upstream = &OpenRouterRouting{Provider: OpenRouterProvider{ZDR: &on, DataCollection: "deny", Only: []string{"mistral"}}}
	cfg.Providers[providerOpenAICompatible] = conn
	cheap := cfg.Tiers[TierCheapCloud]
	cheap.Routing = &OpenRouterRouting{Provider: OpenRouterProvider{Sort: &OpenRouterSort{By: SortLatency}}}
	cfg.Tiers[TierCheapCloud] = cheap
	premium := cfg.Tiers[TierPremium]
	premium.Routing = nil
	cfg.Tiers[TierPremium] = premium
	resolved := cfg.resolveProviders()
	for _, tier := range []Tier{TierCheapCloud, TierPremium} {
		wire := resolved.Tiers[tier].Routing.providerWire()
		if wire == nil || wire.ZDR == nil || !*wire.ZDR || wire.DataCollection != "deny" || !slices.Equal(wire.Only, []string{"mistral"}) {
			t.Errorf("%s wire = %+v", tier, wire)
		}
	}
	if s := resolved.Tiers[TierPremium].Routing.Provider.Sort; s == nil || s.By != SortThroughput {
		t.Errorf("a tier with no routing lost the shipped default under the connection's privacy: %+v", s)
	}
}

func TestTheConnectionTakesNoServingKey(t *testing.T) {
	cfg := storedOnBroker()
	conn := cfg.Providers[providerOpenAICompatible]
	conn.Upstream = &OpenRouterRouting{Provider: OpenRouterProvider{Order: []string{"groq"}}}
	cfg.Providers[providerOpenAICompatible] = conn
	err := cfg.validateProviderEntries()
	var f routingFaults
	if !errors.As(err, &f) || f[0].Path != "providers.openai_compatible.upstream" || !strings.Contains(f[0].Message, "set per tier") {
		t.Fatalf("a serving key on the connection = %v", err)
	}
}

func TestANullFieldStatesNoPreference(t *testing.T) {
	for _, in := range []string{`{"provider":{"preferred_max_latency":null}}`, `{"provider":{"max_price":null,"sort":null}}`} {
		r, err := parseRoutingAt("x", []byte(in))
		if err != nil || r.providerWire() != nil {
			t.Errorf("%s read as %+v (%v), want no preference", in, r, err)
		}
	}
}

// An empty list written on a tier is still a connection key there, and the
// refusal names it rather than arriving blank.
func TestAnEmptyPinListOnATierIsNamed(t *testing.T) {
	err := refuseConnectionKeysOnTier("tiers.premium.routing", &OpenRouterRouting{Provider: OpenRouterProvider{Only: []string{}}})
	var faults routingFaults
	if !errors.As(err, &faults) || len(faults) != 1 || faults[0].Path != "tiers.premium.routing.provider.only" {
		t.Fatalf("err = %v, want one fault on provider.only", err)
	}
	if err := refuseConnectionKeysOnTier("tiers.premium.routing", &OpenRouterRouting{}); err != nil {
		t.Errorf("no connection key = %#v, want nil", err)
	}
}

// A routing key written twice in a seed is refused, not settled by whichever
// line the decoder happened to keep.
func TestARoutingKeyWrittenTwiceInASeedIsRefused(t *testing.T) {
	yaml := "profile: cloud_frontier\ntiers:\n  premium:\n    provider: openai_compatible\n    model: m\n" +
		"    base_url: https://openrouter.ai/api\n    routing:\n      provider:\n        sort: price\n        sort: latency\n" +
		"embeddings: {provider: openai_compatible, model: e, base_url: 'https://openrouter.ai/api'}\n"
	if _, err := ParseRouting([]byte(yaml)); err == nil || !strings.Contains(err.Error(), "written twice") {
		t.Fatalf("err = %v, want the repeated key refused", err)
	}
}

// An empty reasoning block is the same opt-out as none, so it is stored in the
// same spelling and keys the same cached briefs.
func TestAnEmptyReasoningBlockKeepsTheFlatSpelling(t *testing.T) {
	bare, err := json.Marshal(&OpenRouterRouting{Provider: OpenRouterProvider{Sort: &OpenRouterSort{By: SortPrice}}})
	if err != nil {
		t.Fatal(err)
	}
	empty, err := json.Marshal(&OpenRouterRouting{Provider: OpenRouterProvider{Sort: &OpenRouterSort{By: SortPrice}}, Reasoning: &OpenRouterReasoning{}})
	if err != nil {
		t.Fatal(err)
	}
	if string(bare) != string(empty) {
		t.Errorf("an empty reasoning block stores %s, the same value without one %s", empty, bare)
	}
}

// A written-empty data_collection or quantization entry is a refusal on its
// path, not a value that decodes as unwritten.
func TestAWrittenEmptyValueIsRefusedByItsPath(t *testing.T) {
	for raw, path := range map[string]string{
		`{"provider":{"data_collection":""}}`: "routing.provider.data_collection",
		`{"provider":{"quantizations":[""]}}`: "routing.provider.quantizations[0]",
	} {
		r, err := DecodeRouting("routing", []byte(raw))
		if err == nil {
			err = r.Validate("routing")
		}
		var faults routingFaults
		if !errors.As(err, &faults) || faults[0].Path != path {
			t.Errorf("%s: err = %v, want a fault on %s", raw, err, path)
		}
	}
}
