// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind shape H1

package gates

// Every scope the retention engine can act on ships a default, or says why not.
//
// The defect this exists for is not a wrong number. It is a scope that reaches
// production with NO row at all, which reads exactly like a scope whose number
// an admin chose: the settings page shows nothing either way, and the sweep
// simply never visits it. `raw_capture` shipped like that, and the table it
// governs was 92% of a measured database — 6.7 GB of 7.3 — because the only
// thing aging those rows was the activity sweep's natural-key join, on an
// installation whose activity policy is the seeded 1095 days.
//
// So this walks the seeded INSERT and the engine's own vocabulary and asserts
// they agree. It reads SeedDefaultRetentionTx's SQL rather than executing it,
// because the subject is what a fresh installation gets planted with, and that
// is a source fact available without a database.
//
// A scope that is deliberately unseeded stays possible — deal/won is one, and
// retentionselectors.go says why beside it — but it costs an entry here with a
// reason, which is the difference between a decision and an oversight.

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"

	"github.com/margince/margince/backend/internal/modules/privacy"
)

// deliberatelyUnseeded names a scope the engine can act on that a fresh
// installation must NOT be planted with, and why. An entry that stops matching
// a real scope fails below rather than lingering.
var deliberatelyUnseeded = gatekit.Waive(map[string]string{
	"deal/won": "a won deal is the record of revenue earned and the evidence behind it, so " +
		"archiving one on a timer is a decision an operator makes rather than one they " +
		"inherit — retentionselectors.go says the same beside the selector",
})

var seedRowRE = regexp.MustCompile(`\(\s*'([a-z_]+)'\s*,\s*(?:'([a-z_]+)'|NULL)\s*,\s*(\d+)\s*,\s*'([a-z]+)'\s*\)`)

// seededScopes reads the scopes SeedDefaultRetentionTx plants, as
// `object_type/category` with an empty category for a NULL.
func seededScopes(t *testing.T) map[string]bool {
	t.Helper()
	src, err := os.ReadFile("internal/modules/consent/retention.go")
	if err != nil {
		t.Fatalf("reading the seed: %v", err)
	}
	out := map[string]bool{}
	for _, m := range seedRowRE.FindAllStringSubmatch(string(src), -1) {
		out[m[1]+"/"+m[2]] = true
	}
	if len(out) == 0 {
		t.Fatal("no seeded rows read out of SeedDefaultRetentionTx — this gate would pass over anything")
	}
	return out
}

func TestEveryActionableRetentionScopeShipsADefaultOrSaysWhyNot(t *testing.T) {
	t.Parallel()
	seeded := seededScopes(t)

	// The waiver reports its own stale entries, so an exemption for a scope
	// that has since been seeded or removed fails here rather than standing.
	defer deliberatelyUnseeded.AssertAllMatched(t)

	// The engine's own vocabulary, so a new selector joins this gate by
	// existing rather than by somebody remembering to list it here.
	for _, scope := range privacy.AuthorableScopes() {
		key := strings.TrimSuffix(scope, "/")
		if seeded[scope] || seeded[key+"/"] || seeded[scope+"/"] {
			continue
		}
		if slices.Contains(deliberatelyUnseeded.Subjects(), scope) {
			deliberatelyUnseeded.Waived(t, scope)
			continue
		}
		t.Errorf("the retention engine can act on %q and a fresh installation is planted with no rule for it.\n"+
			"A scope with no row is indistinguishable from a scope an admin chose a number for, and nothing "+
			"ages it. Seed it in SeedDefaultRetentionTx, or record it in deliberatelyUnseeded with the reason.", scope)
	}
}
