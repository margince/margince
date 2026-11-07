// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// A route-backed action has no hand-written copy of its route. So a change to the
// operation reaches the generated table on the next build. These run the real
// generator on the real contract with one operation changed.
func TestARouteBackedActionFollowsItsOperationInTheContract(t *testing.T) {
	declareBuildInputs(t)
	contract, err := os.ReadFile("../../api/crm.yaml")
	if err != nil {
		t.Fatal(err)
	}
	generator := filepath.Join(t.TempDir(), "gen-agentpolicy")
	if out, err := exec.Command("go", "build", "-o", generator, ".").CombinedOutput(); err != nil {
		t.Fatalf("building the generator: %v\n%s", err, out)
	}
	baseline := generate(t, generator, string(contract))
	const (
		mark        = "x-mcp-tool: { verb: create_team, router: change_access, action: create_team, tier: auto_execute, scope: write, served_by: route }"
		createRow   = `"POST /v1/teams"`
		requestName = "        name: { type: string, minLength: 1, maxLength: 120 }\n    UpdateTeamRequest:"
		answerName  = "      required: [id, name]\n      properties:\n        id: { type: string, format: uuid }\n        name: { type: string }\n        parent_team_id:"
	)
	if !strings.Contains(baseline, createRow) || !strings.Contains(baseline, `Action: "create_team"`) {
		t.Fatalf("the probe's subject left the contract: no create_team row at %s", createRow)
	}

	for _, change := range []struct {
		name, from, to string
		appears        string
		disappears     string
	}{
		{
			name: "a new body field", from: requestName,
			to:      "        name: { type: string, minLength: 1, maxLength: 120 }\n        colour: { type: string }\n    UpdateTeamRequest:",
			appears: `\"colour\":{\"type\":\"string\"}`,
		},
		{
			name: "a renamed body field", from: "      required: [name]\n      properties:\n" + requestName,
			to:         "      required: [title]\n      properties:\n        title: { type: string, minLength: 1, maxLength: 120 }\n    UpdateTeamRequest:",
			appears:    `\"title\":{\"maxLength\":120`,
			disappears: `BodySchema: "{\"properties\":{\"name\":{\"maxLength\":120,\"minLength\":1,\"type\":\"string\"}},\"required\":[\"name\"]`,
		},
		{
			name: "a new answer field", from: answerName,
			to:      "      required: [id, name]\n      properties:\n        id: { type: string, format: uuid }\n        name: { type: string }\n        colour_of_the_answer: { type: string }\n        parent_team_id:",
			appears: `\"colour_of_the_answer\":{\"type\":\"string\"}`,
		},
		{
			name: "a moved route", from: "  /teams:\n", to: "  /crews:\n",
			appears:    `"POST /v1/crews"`,
			disappears: createRow,
		},
		{
			name: "a renamed action", from: mark,
			to:         strings.Replace(mark, "action: create_team", "action: form_team", 1),
			appears:    `Action: "form_team"`,
			disappears: `Action: "create_team"`,
		},
		{
			name: "an action closed to agents", from: mark,
			to:         "x-agent-access: human-only\n      x-human-only-reason: security",
			disappears: `Action: "create_team"`,
		},
	} {
		t.Run(change.name, func(t *testing.T) {
			if strings.Count(string(contract), change.from) != 1 {
				t.Fatalf("the contract no longer holds this text once:\n%s", change.from)
			}
			got := generate(t, generator, strings.Replace(string(contract), change.from, change.to, 1))
			if change.appears != "" && !strings.Contains(got, change.appears) {
				t.Errorf("the table lacks %s", change.appears)
			}
			if change.disappears != "" && strings.Contains(got, change.disappears) {
				t.Errorf("the table still holds %s", change.disappears)
			}
			if got == baseline {
				t.Error("the table did not change at all")
			}
		})
	}
}

// The door runs a route at once, so a tier it cannot honour stops the build. An
// agent would otherwise read a promise of human approval that nobody keeps.
func TestARouteBackedActionCannotAskForATierItsDoorCannotHonour(t *testing.T) {
	declareBuildInputs(t)
	contract, err := os.ReadFile("../../api/crm.yaml")
	if err != nil {
		t.Fatal(err)
	}
	const mark = "action: create_team, tier: auto_execute"
	if strings.Count(string(contract), mark) != 1 {
		t.Fatalf("the contract no longer holds %q once", mark)
	}
	dir := t.TempDir()
	in := filepath.Join(dir, "crm.yaml")
	changed := strings.Replace(string(contract), mark, "action: create_team, tier: confirmation_required", 1)
	if err := os.WriteFile(in, []byte(changed), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("go", "run", ".", "-in", in, "-out", filepath.Join(dir, "table.go"),
		"-agents", "../../internal/modules/agents").CombinedOutput()
	if err == nil || !strings.Contains(string(out), "its tier must be auto_execute") {
		t.Fatalf("generating answered %v, want the tier refused:\n%s", err, out)
	}
}

// declareBuildInputs keys the test cache on what `go` reads, which it cannot see.
func declareBuildInputs(t *testing.T) {
	t.Helper()
	if err := gatekit.DeclareInputs(os.Getenv, ".", "../go.mod", "../../../backend"); err != nil {
		t.Fatal(err)
	}
}

// generate runs the real generator on one version of the contract and answers the table.
func generate(t *testing.T, generator, contract string) string {
	t.Helper()
	dir := t.TempDir()
	in, out := filepath.Join(dir, "crm.yaml"), filepath.Join(dir, "table.go")
	if err := os.WriteFile(in, []byte(contract), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(generator, "-in", in, "-out", out, "-agents", "../../internal/modules/agents")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generating: %v\n%s", err, output)
	}
	table, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return string(table)
}
