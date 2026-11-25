// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agentmarks

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const zoneSchemas = `
Zone:
  type: object
  required: [timezone]
  properties:
    timezone: { type: string }
    source: { $ref: '#/components/schemas/Source', x-agent-values: [agent] }
    stated_by: { type: string, x-human-only: true }
Source: { type: string, enum: [browser, chosen, agent] }
`

//craft:ignore naked-any a JSON schema is walked as decoded YAML, whatever its shape
func decoded(t *testing.T, src string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := yaml.Unmarshal([]byte(src), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestABodyWithNoMarkerComesBackAsGiven(t *testing.T) {
	body := decoded(t, `{ $ref: '#/components/schemas/Plain' }`)
	got, marks, err := Narrow(body, decoded(t, `Plain: { type: object, properties: { name: { type: string } } }`))
	if err != nil || !reflect.DeepEqual(got, body) || marks.HumanOnly != nil || marks.AgentValues != nil {
		t.Errorf("Narrow = %v, %+v, %v; want the body untouched and no marks", got, marks, err)
	}
}

func TestNarrowResolvesAllOfAndRefsWithoutChangingTheSchemas(t *testing.T) {
	schemas := decoded(t, zoneSchemas)
	before := decoded(t, zoneSchemas)
	got, marks, err := Narrow(decoded(t, `{ allOf: [ { $ref: '#/components/schemas/Zone' } ] }`), schemas)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(marks.HumanOnly, []string{"stated_by"}) || !slices.Equal(marks.AgentValues["source"], []string{"agent"}) {
		t.Errorf("marks %+v, want stated_by human-only and source: [agent]", marks)
	}
	props, _ := got.(map[string]any)["properties"].(map[string]any)
	source, _ := props["source"].(map[string]any)
	if props["stated_by"] != nil || !reflect.DeepEqual(source["enum"], []any{"agent"}) || source[AgentValues] != nil {
		t.Errorf("narrowed body %v, want stated_by gone and source offering [agent] with no marker", got)
	}
	if !reflect.DeepEqual(schemas, before) {
		t.Error("Narrow changed components.schemas, which an answer still reads whole")
	}
}

func TestNarrowRefusesAMarkerTheGateCannotHold(t *testing.T) {
	for want, body := range map[string]string{
		"lists values":     `{ type: object, properties: { source: { type: string, x-agent-values: [agent] } } }`,
		"does not hold":    `{ type: object, properties: { source: { type: string, enum: [browser], x-agent-values: [agent] } } }`,
		"top-level":        `{ type: object, properties: { line: { type: object, properties: { price: { type: number, x-human-only: true } } } } }`,
		"required":         `{ type: object, required: [kind], properties: { kind: { type: string, x-human-only: true } } }`,
		"past the first":   `{ oneOf: [ { type: object }, { type: object, properties: { kind: { type: string, x-human-only: true } } } ] }`,
		"true or absent":   `{ type: object, properties: { kind: { type: string, x-human-only: yes please } } }`,
		"names schema":     `{ type: object, properties: { kind: { $ref: '#/components/schemas/Missing', x-human-only: true } } }`,
		"an object schema": `[ { x-human-only: true } ]`,
	} {
		var schema any
		if err := yaml.Unmarshal([]byte(body), &schema); err != nil {
			t.Fatal(err)
		}
		if _, _, err := Narrow(schema, map[string]any{}); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Narrow(%s) = %v, want an error saying %q", body, err, want)
		}
	}
}
