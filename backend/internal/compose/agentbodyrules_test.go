// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"maps"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/margince/margince/backend/internal/modules/identity"
)

// A body field's words that state a rule for an agent. Such a rule is held by a
// marker the gate enforces and the catalog serves, never by the words alone.
var agentRuleInWords = regexp.MustCompile(`(?i)\bonly an agent\b|\b(?:an agent|agents?)(?:'s)?\b[^.]{0,80}\b(?:never|refused|may not|must not)\b`)

type routeBodyField struct {
	route, name string
	schema      *openapi3.Schema
}

// routeBodyFields lists the top-level JSON body fields of every route the agent
// door forwards, as the contract declares them.
func routeBodyFields(t *testing.T) []routeBodyField {
	t.Helper()
	doc, err := openapi3.NewLoader().LoadFromFile(contractFile)
	if err != nil {
		t.Fatal(err)
	}
	var fields []routeBodyField
	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			route := method + " /v1" + path
			if agentPolicies[route].ServedBy != servedByRoute || op.RequestBody == nil || op.RequestBody.Value == nil {
				continue
			}
			media := op.RequestBody.Value.Content.Get("application/json")
			if media == nil || media.Schema == nil || media.Schema.Value == nil {
				continue
			}
			for _, part := range append([]*openapi3.SchemaRef{media.Schema}, media.Schema.Value.AllOf...) {
				for name, field := range part.Value.Properties {
					fields = append(fields, routeBodyField{route: route, name: name, schema: field.Value})
				}
			}
		}
	}
	return fields
}

// Every word the contract says about a body field reaches the agent that reads
// the action catalog. A rule for an agent stated there is also a marker.
func TestEveryRouteBodyFieldIsServedWithItsWordsAndItsAgentRule(t *testing.T) {
	fields := routeBodyFields(t)
	described, rules := 0, 0
	for _, f := range fields {
		_, humanOnly := f.schema.Extensions["x-human-only"]
		_, agentValued := f.schema.Extensions["x-agent-values"]
		if agentRuleInWords.MatchString(f.schema.Description) {
			rules++
			if !humanOnly && !agentValued {
				t.Errorf("%s %s states a rule for an agent in words alone; mark it x-agent-values or x-human-only so the gate holds it",
					f.route, f.name)
			}
		}
		if f.schema.Description == "" || humanOnly {
			continue
		}
		described++
		served := servedBodyField(t, f.route, f.name)
		if got, want := strings.Fields(served["description"].(string)), strings.Fields(f.schema.Description); !slices.Equal(got, want) {
			t.Errorf("%s %s: the catalog serves %q, want the contract's words %q", f.route, f.name, strings.Join(got, " "), strings.Join(want, " "))
		}
	}
	if len(fields) < 900 || described < 400 || rules < 3 {
		t.Fatalf("read %d body fields, %d described and %d agent rules; the census no longer sees the contract", len(fields), described, rules)
	}
}

//craft:ignore naked-any a JSON schema is read as decoded JSON, whatever its shape
func servedBodyField(t *testing.T, route, name string) map[string]any {
	t.Helper()
	var body struct {
		Properties map[string]map[string]any `json:"properties"`
	}
	if err := json.Unmarshal([]byte(routeVerbShapes[route].BodySchema), &body); err != nil {
		t.Fatalf("%s: the served body schema does not decode: %v", route, err)
	}
	field, ok := body.Properties[name]
	if !ok {
		t.Fatalf("%s serves no field %s", route, name)
	}
	if _, has := field["description"].(string); !has {
		field["description"] = ""
	}
	return field
}

// The values an agent is offered for the time zone's source are the ones the
// service lets it send.
func TestAnAgentIsOfferedTheTimezoneSourcesTheServiceAccepts(t *testing.T) {
	const route = "PUT /v1/me/timezone"
	enforced := make([]string, 0, len(identity.AgentTimezoneSources))
	for _, source := range identity.AgentTimezoneSources {
		enforced = append(enforced, string(source))
	}
	if gated := routeVerbShapes[route].AgentValues["source"]; !slices.Equal(gated, enforced) {
		t.Errorf("the agent gate holds source to %v, and the service accepts %v", gated, enforced)
	}
	var offered []string
	for _, value := range servedBodyField(t, route, "source")["enum"].([]any) {
		word, _ := value.(string)
		offered = append(offered, word)
	}
	if !slices.Equal(offered, enforced) {
		t.Errorf("the catalog offers an agent source %v, and the service accepts %v", offered, enforced)
	}
	if got := slices.Sorted(maps.Keys(routeVerbShapes[route].AgentValues)); !slices.Equal(got, []string{"source"}) {
		t.Errorf("agent values are held on %v, want source alone", got)
	}
}
