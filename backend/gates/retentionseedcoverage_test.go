// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind shape H1

package gates

// Every scope the retention engine can act on ships a default, or says why not.
//
// A scope with no policy row is indistinguishable, on the settings page and in
// the sweep, from a scope whose number an admin chose: the page shows nothing
// either way and the pass simply never visits it. So the absence has to be
// caught here, where it is a source fact, rather than in a database where it
// looks like a decision.
//
// The seeded INSERT is read rather than executed: the subject is what a fresh
// installation is planted with, which is answerable without a database. The
// engine's vocabulary is read the same way, off its selector table, because
// gates may not depend on a module.
//
// A scope that must NOT be planted stays possible and costs an entry in
// deliberatelyUnseeded with a reason, which is the difference between a
// decision and an oversight.

import (
	"os"
	"regexp"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// deliberatelyUnseeded names a scope the engine can act on that a fresh
// installation must NOT be planted with, and why. An entry matching no
// remaining unseeded scope fails rather than standing.
var deliberatelyUnseeded = gatekit.Waive(map[string]string{
	"deal/won": "a won deal is the record of revenue earned and the evidence behind it, so " +
		"archiving one on a timer is a decision an operator makes rather than one they " +
		"inherit — retentionselectors.go says the same beside the selector",
})

var seedRowRE = regexp.MustCompile(`\(\s*'([a-z_]+)'\s*,\s*(?:'([a-z_]+)'|NULL)\s*,\s*(\d+)\s*,\s*'([a-z]+)'\s*\)`)

// selectorKeyRE matches a selector-table key. The character class admits digits
// so a scope named with one is not silently skipped by the reader that exists
// to notice missing scopes.
var selectorKeyRE = regexp.MustCompile("(?m)^\\t\"([a-z_]+/[a-z_]*)\": `")

// actionableScopes reads the scope keys off the engine's selector table. It
// claims no completeness beyond what the parse finds: an empty result fails
// loudly, because a reader that finds nothing passes over everything.
func actionableScopes(t *testing.T) []string {
	t.Helper()
	src, err := os.ReadFile("internal/modules/privacy/retentionselectors.go")
	if err != nil {
		t.Fatalf("reading the selector table: %v", err)
	}
	var out []string
	for _, m := range selectorKeyRE.FindAllStringSubmatch(string(src), -1) {
		out = append(out, m[1])
	}
	if len(out) == 0 {
		t.Fatal("no selector keys read out of retentionselectors.go — this gate would pass over anything")
	}
	return out
}

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
	for _, scope := range actionableScopes(t) {
		if seeded[scope] {
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
