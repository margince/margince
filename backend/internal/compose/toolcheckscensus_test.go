// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"maps"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents"
)

// toolChecksIncompleteCeiling is how many served verbs may hold a row the
// sweep could not finish, its admin call stopped before the last check. It
// only goes down; at zero every row is complete.
const toolChecksIncompleteCeiling = 7

// toolChecksRerun is what to do about a row that is missing, partial or stale.
const toolChecksRerun = "add the seed or fixture its call needs in backend/internal/compose/integration/toolrole*, then run " +
	"UPDATE_MCP_TOOL_ROLES=1 make test-it DIR=backend/internal/compose/integration RUN=TestTheToolRoleMatrixIsPublished"

// Every verb the registry serves to an agent holds a row in the checks table
// the binary embeds. The table holds no row for a verb no longer served.
// The sweep writes the table; this holds it to the surface without a database.
func TestEveryServedVerbHasItsToolChecksRow(t *testing.T) {
	table, err := agents.EmbeddedToolChecks()
	if err != nil {
		t.Fatalf("the embedded toolchecks.json does not read: %v", err)
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
	var incomplete []string
	for _, verb := range slices.Sorted(maps.Keys(served)) {
		row, has := table.Verbs[verb]
		switch {
		case !has:
			t.Errorf("%s is served and toolchecks.json has no row for it: %s", verb, toolChecksRerun)
		case !row.Complete:
			incomplete = append(incomplete, verb)
		}
	}
	for _, verb := range slices.Sorted(maps.Keys(table.Verbs)) {
		if !served[verb] {
			t.Errorf("toolchecks.json has a row for %s, which the registry no longer serves: %s", verb, toolChecksRerun)
		}
	}
	switch {
	case len(incomplete) > toolChecksIncompleteCeiling:
		t.Errorf("%d served verbs hold a partial row, above the ceiling of %d: %v. For each, %s",
			len(incomplete), toolChecksIncompleteCeiling, incomplete, toolChecksRerun)
	case len(incomplete) < toolChecksIncompleteCeiling:
		t.Errorf("%d served verbs hold a partial row, below the ceiling of %d; lower toolChecksIncompleteCeiling to %d",
			len(incomplete), toolChecksIncompleteCeiling, len(incomplete))
	}
}
