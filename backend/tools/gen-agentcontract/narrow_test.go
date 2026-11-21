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
	out, _, _, err := agentContract([]byte(narrowFixture), nil)
	if err != nil {
		t.Fatal(err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	zone, ok := at(oasnode.Root(&doc), "components", "schemas", "Zone", "properties")
	if !ok {
		t.Fatal("the skill carries no Zone schema")
	}
	if _, kept := oasnode.Lookup(zone, "stated_by"); kept {
		t.Error("a human-only member is still offered to the agent")
	}
	enum, _ := at(zone, "source", "enum")
	if enum == nil || len(enum.Content) != 1 || enum.Content[0].Value != "agent" {
		t.Errorf("source offers %v, want [agent] alone", enum)
	}
	if strings.Contains(string(out), "x-agent-values") {
		t.Error("the marker itself reached the skill")
	}
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
