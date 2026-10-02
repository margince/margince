// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"reflect"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// legacyRoutingKeys is the flat spelling the routing value had before it took
// OpenRouter's nested request shape. Stored rows, YAML seeds and
// MARGINCE_AICERT_UPSTREAM still carry it, so it is read at every door and
// written nested on the next save.
var legacyRoutingKeys = []string{
	"only", "ignore", "quantizations", "sort", "require_parameters",
	"allow_fallbacks", "preferred_max_latency_p90", "reasoning_effort",
}

// UnmarshalJSON reads either spelling. Unknown keys are refused at every depth
// (a misspelt key the broker would drop is the silence Validate exists to end),
// and each refusal names its path relative to the routing value.
func (r *OpenRouterRouting) UnmarshalJSON(data []byte) error {
	parsed, err := DecodeRouting("", data)
	if err != nil {
		return err
	}
	*r = *parsed
	return nil
}

// legacyRouting is the flat spelling, field for field as it was declared:
// every key present, none omitted. Its encoding is what the binding digest was
// computed over, so it must not change.
type legacyRouting struct {
	Only                   []string `json:"only"`
	Ignore                 []string `json:"ignore"`
	Quantizations          []string `json:"quantizations"`
	Sort                   string   `json:"sort"`
	RequireParameters      *bool    `json:"require_parameters"`
	AllowFallbacks         *bool    `json:"allow_fallbacks"`
	PreferredMaxLatencyP90 float64  `json:"preferred_max_latency_p90"`
	ReasoningEffort        string   `json:"reasoning_effort"`
}

// MarshalJSON writes the flat spelling whenever it can say the whole value,
// and the nested one otherwise.
//
// Every cached brief, dossier and growth-fit is keyed on the binding digest,
// which marshals this value: an unchanged binding that began encoding
// differently would regenerate all of them through paid models. Only a value
// using a field the flat spelling never had is written nested.
func (r OpenRouterRouting) MarshalJSON() ([]byte, error) {
	if flat, ok := r.legacy(); ok {
		return json.Marshal(flat)
	}
	type nested OpenRouterRouting
	return json.Marshal(nested(r))
}

// legacy is r in the flat spelling, false when r uses a field it cannot carry.
func (r OpenRouterRouting) legacy() (legacyRouting, bool) {
	p := r.Provider
	rest := p
	rest.Only, rest.Ignore, rest.Quantizations, rest.RequireParameters, rest.AllowFallbacks = nil, nil, nil, nil, nil
	rest.Sort, rest.PreferredMaxLatency = nil, nil
	if !rest.isEmpty() || (p.Sort != nil && p.Sort.Partition != "") {
		return legacyRouting{}, false
	}
	out := legacyRouting{
		Only: p.Only, Ignore: p.Ignore, Quantizations: p.Quantizations,
		RequireParameters: p.RequireParameters, AllowFallbacks: p.AllowFallbacks,
	}
	if p.Sort != nil {
		out.Sort = p.Sort.By
	}
	if l := p.PreferredMaxLatency; l != nil {
		if l.All != nil || l.P50 != nil || l.P75 != nil || l.P99 != nil || l.P90 == nil || *l.P90 == 0 {
			return legacyRouting{}, false
		}
		out.PreferredMaxLatencyP90 = *l.P90
	}
	// An empty reasoning block says nothing, so it does not keep a value nested.
	if !r.Reasoning.isEmpty() {
		if (*r.Reasoning != OpenRouterReasoning{Effort: r.Reasoning.Effort}) || r.Reasoning.Effort == "" {
			return legacyRouting{}, false
		}
		out.ReasoningEffort = r.Reasoning.Effort
	}
	return out, true
}

// UnmarshalYAML reads a YAML seed through the JSON reader, so the two doors
// accept exactly the same values.
func (r *OpenRouterRouting) UnmarshalYAML(node *yaml.Node) error {
	raw, err := yamlNodeJSON(node)
	if err != nil {
		return fmt.Errorf("ai: routing config: %w", err)
	}
	return r.UnmarshalJSON(raw)
}

// DecodeRouting reads one routing value written at path, in either spelling,
// refusing unknown keys by their path. It decodes only: parseRoutingAt and the
// routing store add Validate, so a write and a seed meet one bar.
func DecodeRouting(path string, data []byte) (*OpenRouterRouting, error) {
	fields, err := objectFields(path, data)
	if err != nil {
		return nil, err
	}
	var flat, nested []string
	for _, key := range slices.Sorted(maps.Keys(fields)) {
		switch {
		case slices.Contains(legacyRoutingKeys, key):
			flat = append(flat, key)
		case key == "provider" || key == "reasoning":
			nested = append(nested, key)
		default:
			return nil, unknownKey(joinPath(path, key), "provider, reasoning")
		}
	}
	if len(flat) > 0 && len(nested) > 0 {
		return nil, invalidAt(path, fmt.Sprintf("mixes the flat spelling (%s) with the nested one (%s); write every key under provider and reasoning",
			strings.Join(flat, ", "), strings.Join(nested, ", ")))
	}
	if len(flat) > 0 {
		return decodeFlatRouting(path, fields)
	}
	out := &OpenRouterRouting{}
	if raw, ok := fields["provider"]; ok {
		if out.Provider, err = decodeProvider(joinPath(path, "provider"), raw); err != nil {
			return nil, err
		}
	}
	if raw, ok := fields["reasoning"]; ok {
		if out.Reasoning, err = decodeReasoning(joinPath(path, "reasoning"), raw); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// RequestJSON is the value in OpenRouter's own nested shape — what a client is
// shown and edits — whatever spelling it is stored in.
func (r *OpenRouterRouting) RequestJSON() ([]byte, error) {
	type nested OpenRouterRouting
	return json.Marshal(nested(*r))
}

// parseRoutingAt decodes and validates one routing value, every refusal
// addressed under path.
func parseRoutingAt(path string, data []byte) (*OpenRouterRouting, error) {
	parsed, err := DecodeRouting(path, data)
	if err != nil {
		return nil, err
	}
	if err := parsed.Validate(path); err != nil {
		return nil, err
	}
	return parsed, nil
}

// decodeFlatRouting maps the legacy flat keys onto the nested shape.
func decodeFlatRouting(path string, fields map[string]json.RawMessage) (*OpenRouterRouting, error) {
	var flat legacyRouting
	raw, err := json.Marshal(fields)
	if err != nil {
		return nil, invalidAt(path, "could not be read")
	}
	if err := json.Unmarshal(raw, &flat); err != nil {
		return nil, typeFault(path, err)
	}
	out := &OpenRouterRouting{Provider: OpenRouterProvider{
		Only: flat.Only, Ignore: flat.Ignore, Quantizations: flat.Quantizations,
		RequireParameters: flat.RequireParameters, AllowFallbacks: flat.AllowFallbacks,
	}}
	if flat.Sort != "" {
		out.Provider.Sort = &OpenRouterSort{By: flat.Sort}
	}
	if flat.PreferredMaxLatencyP90 != 0 {
		p90 := flat.PreferredMaxLatencyP90
		out.Provider.PreferredMaxLatency = &OpenRouterPctile{P90: &p90}
	}
	if flat.ReasoningEffort != "" {
		out.Reasoning = &OpenRouterReasoning{Effort: flat.ReasoningEffort}
	}
	return out, nil
}

// providerKeys lists the keys the `provider` object takes, in the order the
// refusal for an unknown one names them.
var providerKeys = []string{
	"order", "only", "ignore", "allow_fallbacks", "require_parameters", "data_collection", "zdr",
	"enforce_distillable_text", "quantizations", "sort", "max_price", "preferred_min_throughput", "preferred_max_latency",
}

func decodeProvider(path string, data []byte) (OpenRouterProvider, error) {
	fields, err := objectFields(path, data)
	if err != nil {
		return OpenRouterProvider{}, err
	}
	var p OpenRouterProvider
	decoders := map[string]func(string, json.RawMessage) error{
		"order":                    func(at string, raw json.RawMessage) error { return decodeValue(at, raw, &p.Order) },
		"only":                     func(at string, raw json.RawMessage) error { return decodeValue(at, raw, &p.Only) },
		"ignore":                   func(at string, raw json.RawMessage) error { return decodeValue(at, raw, &p.Ignore) },
		"allow_fallbacks":          func(at string, raw json.RawMessage) error { return decodeValue(at, raw, &p.AllowFallbacks) },
		"require_parameters":       func(at string, raw json.RawMessage) error { return decodeValue(at, raw, &p.RequireParameters) },
		"data_collection":          func(at string, raw json.RawMessage) error { return decodeValue(at, raw, &p.DataCollection) },
		"zdr":                      func(at string, raw json.RawMessage) error { return decodeValue(at, raw, &p.ZDR) },
		"enforce_distillable_text": func(at string, raw json.RawMessage) error { return decodeValue(at, raw, &p.EnforceDistillableText) },
		"quantizations":            func(at string, raw json.RawMessage) error { return decodeValue(at, raw, &p.Quantizations) },
		"sort":                     func(at string, raw json.RawMessage) (err error) { p.Sort, err = decodeSort(at, raw); return err },
		"max_price":                func(at string, raw json.RawMessage) (err error) { p.MaxPrice, err = decodePrice(at, raw); return err },
		"preferred_min_throughput": func(at string, raw json.RawMessage) (err error) {
			p.PreferredMinThroughput, err = decodePctile(at, raw)
			return err
		},
		"preferred_max_latency": func(at string, raw json.RawMessage) (err error) {
			p.PreferredMaxLatency, err = decodePctile(at, raw)
			return err
		},
	}
	var errs []error
	for _, key := range slices.Sorted(maps.Keys(fields)) {
		decode, known := decoders[key]
		if !known {
			errs = append(errs, unknownKey(joinPath(path, key), strings.Join(providerKeys, ", ")))
			continue
		}
		// null states no preference, as an absent key does, for every field
		// alike: a threshold must not read it as zero.
		if string(fields[key]) == "null" {
			continue
		}
		errs = append(errs, decode(joinPath(path, key), fields[key]))
	}
	return p, joinFaults(errs...)
}

func decodeReasoning(path string, data []byte) (*OpenRouterReasoning, error) {
	var r OpenRouterReasoning
	if err := decodeStrictObject(path, data, &r, "effort", "max_tokens", "exclude", "enabled"); err != nil {
		return nil, err
	}
	return &r, nil
}

// decodeSort reads a bare order ("throughput") or {by, partition}.
func decodeSort(path string, data []byte) (*OpenRouterSort, error) {
	var by string
	if json.Unmarshal(data, &by) == nil {
		return &OpenRouterSort{By: by}, nil
	}
	var s OpenRouterSort
	if err := decodeStrictObject(path, data, &s, "by", "partition"); err != nil {
		return nil, err
	}
	// Only {by} alone needs the note: with a partition the path is sort.by anyway.
	s.asObject = s.Partition == ""
	return &s, nil
}

func decodePrice(path string, data []byte) (*OpenRouterPrice, error) {
	var p OpenRouterPrice
	if err := decodeStrictObject(path, data, &p, "prompt", "completion", "request", "image"); err != nil {
		return nil, err
	}
	// `max_price: {}` caps nothing; kept, it would send an empty object.
	if p == (OpenRouterPrice{}) {
		return nil, nil
	}
	return &p, nil
}

// decodePctile reads one number for every percentile, or {p50, p75, p90, p99}.
func decodePctile(path string, data []byte) (*OpenRouterPctile, error) {
	var all float64
	if json.Unmarshal(data, &all) == nil {
		return &OpenRouterPctile{All: &all}, nil
	}
	var p OpenRouterPctile
	if err := decodeStrictObject(path, data, &p, "p50", "p75", "p90", "p99"); err != nil {
		return nil, err
	}
	return &p, nil
}

// objectFields splits a JSON object into its keys, refusing anything else.
func objectFields(path string, data []byte) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil || fields == nil {
		return nil, invalidAt(path, "must be an object")
	}
	return fields, nil
}

// decodeStrictObject decodes a leaf object whose keys must be among known, the
// first unknown one named by its path.
func decodeStrictObject[T any](path string, data []byte, into *T, known ...string) error {
	fields, err := objectFields(path, data)
	if err != nil {
		return err
	}
	for _, key := range slices.Sorted(maps.Keys(fields)) {
		if !slices.Contains(known, key) {
			return unknownKey(joinPath(path, key), strings.Join(known, ", "))
		}
	}
	if err := json.Unmarshal(data, into); err != nil {
		return typeFault(path, err)
	}
	return nil
}

func decodeValue[T any](path string, data []byte, into *T) error {
	if err := json.Unmarshal(data, into); err != nil {
		return typeFault(path, err)
	}
	return nil
}

func unknownKey(path, known string) error {
	return invalidAt(path, "is not a key OpenRouter takes here; use one of "+known)
}

// typeFault names the expected shape without echoing decoder internals.
func typeFault(path string, err error) error {
	var typeErr *json.UnmarshalTypeError
	if !errors.As(err, &typeErr) {
		return invalidAt(path, "is not valid JSON for this field")
	}
	at := path
	if typeErr.Field != "" {
		at = joinPath(path, typeErr.Field)
	}
	return invalidAt(at, fmt.Sprintf("must be %s, not %s", jsonKindName(typeErr.Type.Kind()), typeErr.Value))
}

// jsonKindName says what JSON value a Go kind is decoded from.
func jsonKindName(kind reflect.Kind) string {
	switch kind {
	case reflect.Bool:
		return "true or false"
	case reflect.String:
		return "a string"
	case reflect.Slice, reflect.Array:
		return "a list"
	case reflect.Struct, reflect.Map:
		return "an object"
	default:
		return "a number"
	}
}

// yamlNodeJSON re-encodes a YAML value as JSON, scalars by their resolved tag,
// so a seed and a stored value meet one reader.
func yamlNodeJSON(node *yaml.Node) (json.RawMessage, error) {
	switch node.Kind {
	case yaml.DocumentNode:
		if len(node.Content) == 0 {
			return json.RawMessage("null"), nil
		}
		return yamlNodeJSON(node.Content[0])
	case yaml.AliasNode:
		return yamlNodeJSON(node.Alias)
	case yaml.MappingNode:
		fields := make(map[string]json.RawMessage, len(node.Content)/2)
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i].Value
			// Two answers to one key: keeping either would be a guess.
			if _, twice := fields[key]; twice {
				return nil, fmt.Errorf("line %d: %q is written twice", node.Content[i].Line, key)
			}
			value, err := yamlNodeJSON(node.Content[i+1])
			if err != nil {
				return nil, err
			}
			fields[key] = value
		}
		return json.Marshal(fields)
	case yaml.SequenceNode:
		items := make([]json.RawMessage, 0, len(node.Content))
		for _, item := range node.Content {
			value, err := yamlNodeJSON(item)
			if err != nil {
				return nil, err
			}
			items = append(items, value)
		}
		return json.Marshal(items)
	}
	return yamlScalarJSON(node)
}

func yamlScalarJSON(node *yaml.Node) (json.RawMessage, error) {
	switch node.ShortTag() {
	case "!!null":
		return json.RawMessage("null"), nil
	case "!!bool":
		var b bool
		if err := node.Decode(&b); err != nil {
			return nil, err
		}
		return json.Marshal(b)
	case "!!int", "!!float":
		var f float64
		if err := node.Decode(&f); err != nil {
			return nil, err
		}
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, fmt.Errorf("line %d: %s must be a finite number", node.Line, node.Value)
		}
		return json.Marshal(f)
	}
	return json.Marshal(node.Value)
}
