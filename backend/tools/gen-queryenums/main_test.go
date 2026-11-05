// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

const spec = `
openapi: 3.0.3
info: {title: t, version: "1"}
paths:
  /things:
    get:
      parameters:
        - {name: status, in: query, schema: {type: string, enum: [open, closed]}}
        - {name: limit, in: query, schema: {type: integer, enum: [1, 2]}}
        - {name: q, in: query, schema: {type: string}}
        - {name: size, in: query, schema: {type: integer, minimum: 1, maximum: 200}}
        - {name: offset, in: query, schema: {type: integer, minimum: 0}}
        - {name: ratio, in: query, schema: {type: number, minimum: 0.5}}
        - {name: kinds, in: query, schema: {type: array, items: {type: string, enum: [a, b]}}}
        - {name: joined, in: query, explode: false, schema: {type: array, items: {type: string, enum: [c, d]}}}
        - {name: X-Trace, in: header, schema: {type: string, enum: [on]}}
      responses: {"200": {description: ok}}
    delete:
      parameters:
        - {name: scope, in: query, schema: {type: string, enum: [mine, all]}}
      responses: {"200": {description: ok}}
  /shared:
    parameters:
      - {name: mode, in: query, schema: {type: string, enum: [x, y]}}
    get:
      responses: {"200": {description: ok}}
`

func load(t *testing.T, data string) *openapi3.T {
	t.Helper()
	doc, err := openapi3.NewLoader().LoadFromData([]byte(data))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return doc
}

func TestOnlyStringEnumQueryParametersAreListed(t *testing.T) {
	table, err := deriveTable(load(t, spec))
	if err != nil {
		t.Fatalf("deriveTable: %v", err)
	}
	var names []string
	for _, e := range table["GET /v1/things"].enums {
		names = append(names, e.Name)
	}
	if !slices.Equal(names, []string{"joined", "kinds", "status"}) {
		t.Fatalf("GET /v1/things lists %v, want joined, kinds and status (not a number enum, a free string or a header)", names)
	}
	if shared := table["GET /v1/shared"].enums; len(shared) != 1 || shared[0].Name != "mode" {
		t.Errorf("GET /v1/shared = %+v, want the path-level parameter", shared)
	}
}

func TestADeleteIsListedLikeAnyOtherMethod(t *testing.T) {
	table, err := deriveTable(load(t, spec))
	if err != nil {
		t.Fatalf("deriveTable: %v", err)
	}
	if del := table["DELETE /v1/things"].enums; len(del) != 1 || del[0].Name != "scope" {
		t.Fatalf("DELETE /v1/things = %+v, want its scope enum", del)
	}
}

func TestArrayAndExplodeComeFromTheParameter(t *testing.T) {
	table, err := deriveTable(load(t, spec))
	if err != nil {
		t.Fatalf("deriveTable: %v", err)
	}
	got := map[string]queryEnum{}
	for _, e := range table["GET /v1/things"].enums {
		got[e.Name] = e
	}
	if e := got["status"]; e.Array || !e.Explode {
		t.Errorf("a single-value parameter = %+v, want Array false", e)
	}
	if e := got["kinds"]; !e.Array || !e.Explode || !slices.Equal(e.Values, []string{"a", "b"}) {
		t.Errorf("an array parameter = %+v, want an exploded array of its items' enum", e)
	}
	if e := got["joined"]; !e.Array || e.Explode {
		t.Errorf("an explode:false array = %+v, want Explode false", e)
	}
}

func TestAnEnumUnderACompositionIsRefusedNotSkipped(t *testing.T) {
	for _, composition := range []string{"allOf", "oneOf", "anyOf"} {
		t.Run(composition, func(t *testing.T) {
			doc := load(t, `
openapi: 3.0.3
info: {title: t, version: "1"}
paths:
  /things:
    get:
      parameters:
        - name: status
          in: query
          schema:
            `+composition+`:
              - {type: string, enum: [open, closed]}
      responses: {"200": {description: ok}}
`)
			_, err := deriveTable(doc)
			if err == nil || !strings.Contains(err.Error(), "status") {
				t.Fatalf("deriveTable = %v, want a refusal naming the parameter", err)
			}
		})
	}
}

func TestIntegerQueryParametersCarryTheirMinimum(t *testing.T) {
	table, err := deriveTable(load(t, spec))
	if err != nil {
		t.Fatalf("deriveTable: %v", err)
	}
	got := table["GET /v1/things"].minimums
	want := []queryMinimum{{Name: "offset", Minimum: 0}, {Name: "size", Minimum: 1}}
	if !slices.Equal(got, want) {
		t.Fatalf("GET /v1/things minimums = %+v, want %+v (not a number, a free string or an enum)", got, want)
	}
}

func TestAMinimumTheTableCannotHoldIsRefusedNotSkipped(t *testing.T) {
	for name, schema := range map[string]string{
		"exclusive":   "{type: integer, minimum: 0, exclusiveMinimum: true}",
		"array items": "{type: array, items: {type: integer, minimum: 1}}",
		"allOf":       "{allOf: [{type: integer, minimum: 1}]}",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := deriveTable(load(t, `
openapi: 3.0.3
info: {title: t, version: "1"}
paths:
  /things:
    get:
      parameters:
        - {name: size, in: query, schema: `+schema+`}
      responses: {"200": {description: ok}}
`))
			if err == nil || !strings.Contains(err.Error(), "size") {
				t.Fatalf("deriveTable = %v, want a refusal naming the parameter", err)
			}
		})
	}
}
