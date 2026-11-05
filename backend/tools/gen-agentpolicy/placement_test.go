// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"strings"
	"testing"
)

// placementPaths places operations so that each signal, taken alone, gives a
// different router: a suggestion names the signal that decided it.
const placementPaths = `
/dedupe/settings:
  get:
    tags: [Contacts]
    operationId: getContactCaseSettings
    x-mcp-tool: { verb: get_contact_case_settings, router: read_workspace, action: contact_case_settings, tier: auto_execute, scope: read, served_by: route }
  put:
    tags: [Contacts]
    operationId: updateContactCaseSettings
/pipelines/settings:
  get:
    tags: [Pipelines]
    operationId: getPipelineSettings
    x-mcp-tool: { verb: get_pipeline_settings, router: read_workspace, action: pipeline_settings, tier: auto_execute, scope: read, served_by: route }
  put:
    tags: [Pipelines]
    operationId: updatePipelineSettings
    x-mcp-tool: { verb: update_pipeline_settings, router: change_settings, action: update_pipeline_settings, tier: auto_execute, scope: write, served_by: route }
/contacts/{id}/owner:
  put:
    tags: [Contacts]
    operationId: setContactOwner
    x-mcp-tool: { verb: set_contact_owner, router: change_access, action: set_contact_owner, tier: auto_execute, scope: write, served_by: route }
/contacts/{id}/team:
  put:
    tags: [Contacts]
    operationId: setContactTeam
    x-mcp-tool: { verb: set_contact_team, router: change_access, action: set_contact_team, tier: auto_execute, scope: write, served_by: route }
  post:
    tags: [Contacts]
    operationId: addContactTeam
    x-mcp-tool: { verb: add_contact_team, router: change_relationships, action: add_contact_team, tier: auto_execute, scope: write, served_by: route }
/contacts/{id}/nickname:
  put:
    tags: [Contacts]
    operationId: renameContactNickname
/companies:
  post:
    tags: [Companies]
    operationId: createCompany
    x-mcp-tool: { verb: create_record, record_type: company, tier: auto_execute, scope: write }
/widgets:
  get:
    tags: [Contacts]
    operationId: listWidgets
    x-mcp-tool: { verb: list_records, record_type: widget, tier: auto_execute, scope: read }
  post:
    tags: [Contacts]
    operationId: createWidget
/odd/report:
  get:
    tags: [Oddities]
    operationId: getOddReport
`

// An unannotated operation is refused with the x-mcp-tool line the contract's
// own placements suggest, each from the signal the placement table ranks first.
func TestAnUnannotatedOperationGetsTheSuggestedPlacement(t *testing.T) {
	_, defects := policiesOf(t, placementPaths)
	for _, tc := range []struct{ name, op, line string }{
		{
			name: "a routed sibling gives its paired router",
			op:   "updateContactCaseSettings",
			line: "{ verb: update_contact_case_settings, router: change_settings, action: update_contact_case_settings, " +
				"tier: auto_execute, scope: write, served_by: route }",
		},
		{
			name: "only a tag gives the router its method most uses",
			op:   "renameContactNickname",
			line: "{ verb: rename_contact_nickname, router: change_access, action: rename_contact_nickname, " +
				"tier: auto_execute, scope: write, served_by: route }",
		},
		{
			name: "a read with neither goes to read_view",
			op:   "getOddReport",
			line: "{ verb: read_view, tier: auto_execute, scope: read }",
		},
		{
			name: "a record create goes to create_record with its record type",
			op:   "createWidget",
			line: "{ verb: create_record, record_type: widget, tier: auto_execute, scope: write }",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defect := defectNaming(t, defects, tc.op)
			if got := suggestedLine(defect); got != "x-mcp-tool: "+tc.line {
				t.Errorf("suggested %q, want %q\nin: %s", got, "x-mcp-tool: "+tc.line, defect)
			}
		})
	}
}

// The suggestion is a hint: the operation is still refused, and the message
// says an agent is kept out only for security, and how.
func TestASuggestionStillRefusesAndNamesTheHumanOnlyMark(t *testing.T) {
	policies, defects := policiesOf(t, placementPaths)
	for _, p := range policies {
		if p.Op == "createWidget" {
			t.Fatalf("createWidget reached the table as %+v; a suggestion must not write the annotation", p)
		}
	}
	defect := defectNaming(t, defects, "createWidget")
	for _, want := range []string{
		"open to agents unless security is the reason to close it",
		"x-agent-access: human-only and give x-human-only-reason: security",
		"make it send if the act delivers",
	} {
		if !strings.Contains(defect, want) {
			t.Errorf("the refusal lacks %q:\n%s", want, defect)
		}
	}
}

func defectNaming(t *testing.T, defects []string, op string) string {
	t.Helper()
	for _, defect := range defects {
		if strings.Contains(defect, "("+op+")") {
			return defect
		}
	}
	t.Fatalf("no defect names %s in %v", op, defects)
	return ""
}

func suggestedLine(defect string) string {
	for line := range strings.SplitSeq(defect, "\n") {
		if rest, found := strings.CutPrefix(strings.TrimSpace(line), "Suggested: "); found {
			return rest
		}
	}
	return ""
}
