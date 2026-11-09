// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package openenums

import (
	"fmt"
	"strings"
	"testing"
)

// fixture reaches one enum from each place the walk must look.
const fixture = `openapi: 3.1.0
paths:
  /things:
    parameters:
      - name: view
        in: query
        schema: { type: string, enum: [compact, full] }
    post:
      requestBody:
        content:
          application/json:
            schema: { $ref: '#/components/schemas/NewThing' }
      responses:
        '200':
          $ref: '#/components/responses/Thing'
components:
  responses:
    Thing:
      headers:
        Cache-Control:
          schema: { type: string, enum: [no-store] }
      content:
        application/json:
          schema: { $ref: '#/components/schemas/Thing' }
  schemas:
    NewThing:
      type: object
      properties:
        kind:
          type: string
          enum: [a, b]
        shared:
          $ref: '#/components/schemas/Shared'
    Thing:
      allOf:
        - $ref: '#/components/schemas/Shared'
        - type: object
          properties:
            state:
              type: string
              enum:
                - open
                - closed
    Shared:
      type: string
      enum: [x, y]
    Unreached:
      type: string
      enum: [z]
`

// lineOf is the line the first occurrence of needle sits on.
func lineOf(t *testing.T, raw, needle string) int {
	t.Helper()
	at := strings.Index(raw, needle)
	if at < 0 {
		t.Fatalf("the fixture has no %q", needle)
	}
	return strings.Count(raw[:at], "\n") + 1
}

func sides(t *testing.T, raw string) map[int]Enum {
	t.Helper()
	enums, err := Classify([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	out := map[int]Enum{}
	for _, e := range enums {
		out[e.Line] = e
	}
	return out
}

func TestClassifyFindsEachSideThroughRefsHeadersAndParameters(t *testing.T) {
	got := sides(t, fixture)
	want := map[int][2]bool{ // line → {response, request}
		lineOf(t, fixture, "enum: [compact, full]"): {false, true}, // path parameter
		lineOf(t, fixture, "enum: [no-store]"):      {true, false}, // response header, through components/responses
		lineOf(t, fixture, "enum: [a, b]"):          {false, true}, // request body property
		lineOf(t, fixture, "- open"):                {true, false}, // response property inside allOf, block list
		lineOf(t, fixture, "enum: [x, y]"):          {true, true},  // one schema both reach
	}
	if len(got) != len(want) {
		t.Fatalf("classified %d enums, want %d (the unreached schema is not one): %+v", len(got), len(want), got)
	}
	for line, side := range want {
		e, ok := got[line]
		if !ok {
			t.Errorf("the enum on line %d was not reached", line)
			continue
		}
		if e.Response != side[0] || e.Request != side[1] {
			t.Errorf("line %d: response=%v request=%v, want response=%v request=%v",
				line, e.Response, e.Request, side[0], side[1])
		}
	}
}

func TestViolationsNameAClosedResponseEnumAndAnOpenRequestEnum(t *testing.T) {
	raw := strings.Replace(fixture, "enum: [a, b]", "enum: [a, b]\n          x-extensible-enum: true", 1)
	enums, err := Classify([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	violations := strings.Join(Violations("crm.yaml", enums), "\n")
	closedResponse := fmt.Sprintf("crm.yaml:%d: a response can carry this enum and it is closed", lineOf(t, raw, "enum: [no-store]"))
	openRequest := fmt.Sprintf("crm.yaml:%d: only a request carries this enum, so it stays closed", lineOf(t, raw, "enum: [a, b]"))
	for _, want := range []string{closedResponse, openRequest} {
		if !strings.Contains(violations, want) {
			t.Errorf("missing %q in:\n%s", want, violations)
		}
	}
	if strings.Contains(violations, fmt.Sprintf("crm.yaml:%d:", lineOf(t, raw, "enum: [compact, full]"))) {
		t.Errorf("a closed request parameter was reported:\n%s", violations)
	}
}

func TestMarkOpenMarksBlockAndFlowResponseEnumsAndNothingElse(t *testing.T) {
	marked, count, err := MarkOpen([]byte(fixture))
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("marked %d enums, want the 3 response-side ones", count)
	}
	enums, err := Classify(marked)
	if err != nil {
		t.Fatalf("the marked contract does not parse: %v\n%s", err, marked)
	}
	if v := Violations("crm.yaml", enums); len(v) > 0 {
		t.Fatalf("the marked contract still breaks the rule:\n%s", strings.Join(v, "\n"))
	}
	if !strings.Contains(string(marked), "enum: [no-store], x-extensible-enum: true }") {
		t.Errorf("the flow-style header enum was not marked in place:\n%s", marked)
	}
	again, count, err := MarkOpen(marked)
	if err != nil || count != 0 || string(again) != string(marked) {
		t.Errorf("marking a marked contract changed it (count %d, err %v)", count, err)
	}
}
