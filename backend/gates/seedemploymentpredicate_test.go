// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// The dev seeder and the boot proof ask "is this person currently employed?" the
// way the PRODUCT asks it, and they ask it in the same words.
//
// Neither is a Go client, so neither can call people.CurrentPrimaryEmploymentSQL:
// both are shell over the public API, and both therefore hand-spell the rule as
// a jq predicate. That makes it one invariant with three writers, and the two
// shell copies are the pair that can drift silently — the seeder deciding a
// record is already good while the boot proof calls the same record broken, or
// worse, both agreeing on a weaker rule than the product reads.
//
// The weaker rule is the one this repository already shipped: an existence probe
// (`.data | length > 0`) accepts an ended or secondary employment, and a company
// page shows neither. So the gate holds two things — that the two scripts spell
// the predicate identically, and that what they spell still names every column
// the server's own predicate reads.
//
// WHAT THIS CANNOT SEE: whether the jq means what the SQL means. Proving that
// would need a second implementation of the predicate to compare against, which
// is the duplicate this gate exists to prevent. It holds the shape — same words
// in both scripts, over the same columns as the server — and the live proof is
// scripts/verify-boot.sh failing against a planted ended edge.

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/people"
)

// The scripts that spell the rule, and the assignment they spell it in.
var currentPrimaryPredicateScripts = []string{
	"../scripts/seed-dev.sh",
	"../scripts/verify-boot.sh",
}

var currentPrimaryPredicate = regexp.MustCompile(`(?m)^CURRENT_PRIMARY_JQ='([^']+)'$`)

// The columns the server's predicate reads, which is what the shell copies must
// still be asking about. Derived from the helper rather than listed, so a rule
// that grows a third column fails here instead of leaving the seeder behind.
var employmentCurrencyColumns = []string{"is_current_primary", "ended_at"}

func predicateIn(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	match := currentPrimaryPredicate.FindStringSubmatch(string(body))
	if match == nil {
		t.Fatalf("%s spells no CURRENT_PRIMARY_JQ='…' assignment.\n"+
			"Both scripts ask whether a seeded person is currently employed, and they must ask it in the same words — the copy that drifts is the one that accepts an employment the product does not draw.", path)
	}
	return match[1]
}

func TestTheSeederAndTheBootProofSpellOneEmploymentRule(t *testing.T) {
	t.Parallel()

	first := predicateIn(t, currentPrimaryPredicateScripts[0])
	for _, path := range currentPrimaryPredicateScripts[1:] {
		if other := predicateIn(t, path); other != first {
			t.Errorf("%s and %s ask whether a person is currently employed in different words:\n\t%s\n\t%s\n"+
				"One writes the records the other refuses. Land both sides in one change.",
				currentPrimaryPredicateScripts[0], path, first, other)
		}
	}
}

func TestTheShellPredicateReadsTheColumnsTheServerReads(t *testing.T) {
	t.Parallel()

	// The server's spelling, and the reason this is derived: an existence probe
	// over /v1/relationships names NEITHER column and reads as employed anybody
	// who ever was.
	server := people.CurrentPrimaryEmploymentSQL("")
	predicate := predicateIn(t, currentPrimaryPredicateScripts[0])
	for _, column := range employmentCurrencyColumns {
		if !strings.Contains(server, column) {
			t.Fatalf("people.CurrentPrimaryEmploymentSQL no longer reads %q (%s) — the rule moved, and this gate was about to hold the shell copies to a shape the product has stopped using",
				column, server)
		}
		if !strings.Contains(predicate, column) {
			t.Errorf("the seeder's employment predicate does not read %q, and the server's does (%s):\n\t%s\n"+
				"A predicate short of a column the product reads accepts a record the product will not draw.",
				column, server, predicate)
		}
	}
}
