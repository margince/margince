// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"slices"
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
        - {name: kinds, in: query, schema: {type: array, items: {type: string, enum: [a, b]}}}
        - {name: X-Trace, in: header, schema: {type: string, enum: [on]}}
      responses: {"200": {description: ok}}
    post:
      parameters:
        - {name: status, in: query, schema: {type: string, enum: [open]}}
      responses: {"200": {description: ok}}
  /shared:
    parameters:
      - {name: mode, in: query, schema: {type: string, enum: [x, y]}}
    get:
      responses: {"200": {description: ok}}
`

func TestOnlyStringEnumQueryParametersOfReadsAreListed(t *testing.T) {
	doc, err := openapi3.NewLoader().LoadFromData([]byte(spec))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	table := deriveTable(doc)

	things := table["GET /v1/things"]
	if len(things) != 2 || things[0].Name != "kinds" || things[1].Name != "status" {
		t.Fatalf("GET /v1/things = %+v, want kinds and status only (not a number enum, a free string, a header or a write)", things)
	}
	if !slices.Equal(things[0].Values, []string{"a", "b"}) {
		t.Errorf("an array parameter's values = %v, want its items' enum", things[0].Values)
	}
	if shared := table["GET /v1/shared"]; len(shared) != 1 || shared[0].Name != "mode" {
		t.Errorf("GET /v1/shared = %+v, want the path-level parameter", shared)
	}
	if _, ok := table["POST /v1/things"]; ok {
		t.Error("a write was listed")
	}
}
