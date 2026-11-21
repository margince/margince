// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/tools/internal/oasnode"
)

const narrowFixture = `openapi: 3.1.0
info: { title: Narrow, version: '1' }
security:
  - bearerAuth: []
paths:
  /me/timezone:
    put:
      tags: [Identity]
      operationId: saveMyTimezone
      summary: Set the time zone.
      x-mcp-tool: { verb: save_my_timezone, tier: auto_execute, scope: write }
      requestBody:
        content:
          application/json:
            schema: { $ref: '#/components/schemas/Zone' }
      responses: { '200': { description: ok } }
    get:
      tags: [Identity]
      operationId: getMyTimezone
      summary: Read the time zone.
      x-mcp-tool: { verb: get_my_timezone, tier: auto_execute, scope: read }
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema: { $ref: '#/components/schemas/Zone' }
  /reports/{report}:
    parameters:
      - name: report
        in: path
        required: true
        schema: { type: string, enum: [deals-by-stage, meeting-conversion] }
    post:
      tags: [Identity]
      operationId: runReport
      summary: Run a report.
      x-mcp-tool: { verb: run_report, tier: auto_execute, scope: read }
      responses: { '200': { description: ok } }
components:
  securitySchemes:
    bearerAuth: { type: http, scheme: bearer }
  schemas:
    Zone:
      type: object
      required: [timezone, source]
      properties:
        timezone: { type: string }
        source: { type: string, enum: [browser, chosen, agent], x-agent-values: [agent] }
        stated_by: { type: string, x-human-only: true }
`

func TestTheSkillOffersAnAgentOnlyWhatItMaySend(t *testing.T) {
	root := narrowedContract(t, narrowFixture)
	sent, ok := at(root, "paths", "/me/timezone", "put", "requestBody", "content", "application/json", "schema", "properties")
	if !ok {
		t.Fatal("the skill carries no narrowed body for saveMyTimezone")
	}
	if _, kept := oasnode.Lookup(sent, "stated_by"); kept {
		t.Error("a human-only member is still offered to the agent")
	}
	if enum, _ := at(sent, "source", "enum"); enum == nil || len(enum.Content) != 1 || enum.Content[0].Value != "agent" {
		t.Errorf("source offers %v, want [agent] alone", enum)
	}
}

// A read answers every member, so the schema it shares with a body is not narrowed.
func TestAnAnswerKeepsWhatTheBodyNarrows(t *testing.T) {
	root := narrowedContract(t, narrowFixture)
	if ref, _ := at(root, "paths", "/me/timezone", "get", "responses", "200", "content", "application/json", "schema", "$ref"); ref == nil || ref.Value != "#/components/schemas/Zone" {
		t.Fatalf("getMyTimezone answers %v, want the Zone schema", ref)
	}
	answered, ok := at(root, "components", "schemas", "Zone", "properties")
	if !ok {
		t.Fatal("the skill carries no Zone schema for the answer")
	}
	if _, kept := oasnode.Lookup(answered, "stated_by"); !kept {
		t.Error("the answer lost a human-only member a read returns")
	}
	if enum, _ := at(answered, "source", "enum"); enum == nil || len(enum.Content) != 3 {
		t.Errorf("the answer's source offers %v, want all three values a read returns", enum)
	}
}

// A marker reached through allOf and a member's $ref is narrowed as the catalog narrows it.
func TestTheSkillNarrowsThroughRefAndAllOf(t *testing.T) {
	composed := strings.Replace(narrowFixture, "schema: { $ref: '#/components/schemas/Zone' }\n      responses: { '200': { description: ok } }",
		"schema: { allOf: [ { $ref: '#/components/schemas/Zone' } ] }\n      responses: { '200': { description: ok } }", 1)
	composed = strings.Replace(composed, "source: { type: string, enum: [browser, chosen, agent], x-agent-values: [agent] }",
		"source: { $ref: '#/components/schemas/Source', x-agent-values: [agent] }", 1)
	composed += "    Source: { type: string, enum: [browser, chosen, agent] }\n"
	sent, ok := at(narrowedContract(t, composed), "paths", "/me/timezone", "put", "requestBody", "content", "application/json", "schema", "properties")
	if !ok {
		t.Fatal("the skill carries no narrowed body for saveMyTimezone")
	}
	if _, kept := oasnode.Lookup(sent, "stated_by"); kept {
		t.Error("a human-only member reached through allOf is still offered")
	}
	if enum, _ := at(sent, "source", "enum"); enum == nil || len(enum.Content) != 1 || enum.Content[0].Value != "agent" {
		t.Errorf("source behind a $ref offers %v, want [agent] alone", enum)
	}
}

// The generator stops on a marker the agent gate could not hold.
func TestTheSkillRefusesAMarkerItCannotApply(t *testing.T) {
	const member = "source: { type: string, enum: [browser, chosen, agent], x-agent-values: [agent] }"
	for name, replacement := range map[string]string{
		"no enum":           "source: { type: string, x-agent-values: [agent] }",
		"value outside":     "source: { type: string, enum: [browser, chosen], x-agent-values: [agent] }",
		"deeper than a top": "source: { type: object, properties: { kind: { type: string, enum: [a, b], x-agent-values: [a] } } }",
	} {
		if _, _, _, err := agentContract([]byte(strings.Replace(narrowFixture, member, replacement, 1)), nil); err == nil {
			t.Errorf("%s: the generator wrote a skill, want it to refuse the marker", name)
		}
	}
}

func narrowedContract(t *testing.T, src string) *yaml.Node {
	t.Helper()
	out, _, _, err := agentContract([]byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "x-agent-values") || strings.Contains(string(out), "x-human-only") {
		t.Error("a marker itself reached the skill")
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	return oasnode.Root(&doc)
}

func TestTheIndexNamesTheValuesOfAPathParameter(t *testing.T) {
	_, index, _, err := agentContract([]byte(narrowFixture), nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := "`{report}` is one of deals-by-stage, meeting-conversion."; !strings.Contains(string(index), want) {
		t.Errorf("index:\n%s\nlacks %q", index, want)
	}
}
