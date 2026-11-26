// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"maps"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents"
)

// toolChecksRerun is what to do about a row that is missing or stale.
const toolChecksRerun = "add the seed or fixture its call needs in backend/internal/compose/integration/toolrole*, then run " +
	"UPDATE_MCP_TOOL_ROLES=1 make test-roles"

// Every verb the registry serves to an agent holds a row in the checks table
// the binary embeds. The table holds no row for a verb no longer served.
// The sweep writes the table; this holds it to the surface without a database.
func TestEveryServedVerbHasItsToolChecksRow(t *testing.T) {
	table, err := agents.EmbeddedToolChecks()
	if err != nil {
		t.Fatalf("the embedded toolchecks folder does not read: %v", err)
	}
	served := map[string]bool{}
	for _, spec := range NewRegistry(nil, SendPath{}).Specs() {
		switch {
		case spec.Unlisted:
		case len(spec.Actions) == 0:
			served[spec.Name] = true
		default:
			for _, action := range spec.Actions {
				served[action.Verb] = true
			}
		}
	}
	if len(served) == 0 {
		t.Fatal("the registry serves no verb, so this checked nothing")
	}
	for _, verb := range slices.Sorted(maps.Keys(served)) {
		if _, has := table[verb]; !has {
			t.Errorf("%s is served and toolchecks/ has no file for it: %s", verb, toolChecksRerun)
		}
	}
	for _, verb := range slices.Sorted(maps.Keys(table)) {
		if !served[verb] {
			t.Errorf("toolchecks/ has a file for %s, which the registry no longer serves: %s", verb, toolChecksRerun)
		}
	}
}

// The sweep writes each verb's checks and its row in one pass. A verb with one
// file and not the other is half an answer.
func TestEveryVerbHasBothItsChecksAndItsRow(t *testing.T) {
	_, rows := readToolRoles(t)
	checks := readToolChecks(t)
	if len(rows) == 0 || len(checks) == 0 {
		t.Fatalf("read %d rows and %d checks, so this checked nothing", len(rows), len(checks))
	}
	for _, verb := range slices.Sorted(maps.Keys(rows)) {
		if _, has := checks[verb]; !has {
			t.Errorf("%s has a row and no checks file: %s", verb, toolChecksRerun)
		}
	}
	for _, verb := range slices.Sorted(maps.Keys(checks)) {
		if _, has := rows[verb]; !has {
			t.Errorf("%s has a checks file and no row: %s", verb, toolChecksRerun)
		}
	}
}
