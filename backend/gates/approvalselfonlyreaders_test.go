// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every approvals reader that filters rows by the decision grants also applies
// the self-only narrowing.
//
// decidable() is the module's ONE visibility predicate and it has two authority
// halves: the grants approving a row would take (requireDecisionGrants), and the
// narrowing to the single seat a row was staged FOR (withheldFromOtherSeats) —
// a linkedin_match, a vcard_create, a held_draft. The two target-filtered reads
// settle target visibility once for the record instead of per row, so they
// cannot call decidable and must spell the per-row halves themselves. One of
// them spelled only the first half, and the omission is invisible in the code:
// a reader that never applied the narrowing looks exactly like one that never
// needed to, and its rows are a colleague's imported address book.
//
// So the obligation is derived rather than listed: whoever asks the grant
// question about a row is a reader deciding what a caller may SEE, and owes the
// narrowing in the same breath. A fourth reader added tomorrow inherits it
// without anybody remembering to add it here.
//
// The rule is deliberately about DIRECT calls. requireDecisionGrants is the
// half a reader reaches for when it is filtering per row, and a function that
// reaches it only through decidable already has both halves — widening to
// transitive reach would put every caller of decidable in the corpus and each
// would need a waiver saying "discharged by decidable", which is a list of
// sentences nobody reads standing in for the rule itself.

import (
	"sort"
	"testing"
)

const (
	// approvalsPackage is the module whose readers this census governs.
	approvalsPackage = "internal/modules/approvals"

	// grantHalf is the authority half every per-row reader already spells, and
	// so the marker that SELECTS the corpus: asking it is what makes a function
	// a per-row visibility filter.
	grantHalf = "requireDecisionGrants"

	// seatHalf is the half the target-filtered readers dropped. Naming both
	// symbols here is what makes the census fail loudly on a rename instead of
	// walking an empty corpus and reporting PASS.
	seatHalf = "withheldFromOtherSeats"
)

func TestEveryApprovalsGrantFilterAlsoAppliesTheSelfOnlyNarrowing(t *testing.T) {
	t.Parallel()
	graph := packageCallGraph(t, approvalsPackage)

	// Under-recognition is the one way this census must not break: a renamed or
	// deleted predicate would leave nothing to select on, and an empty corpus
	// reads exactly like a clean one. Both halves are asserted present as
	// declarations before the walk trusts anything the walk found.
	for _, half := range []string{grantHalf, seatHalf} {
		if _, declared := graph[half]; !declared {
			t.Fatalf("%s is not declared in %s — this census selects and certifies on those two names, "+
				"so a rename leaves it sweeping nothing while still reporting PASS",
				half, approvalsPackage)
		}
	}

	var corpus, missing []string
	for name, fn := range graph {
		if name == grantHalf || !fn.calls[grantHalf] {
			continue
		}
		corpus = append(corpus, name)
		if !fn.calls[seatHalf] {
			missing = append(missing, name)
		}
	}
	if len(corpus) == 0 {
		t.Fatalf("no function in %s filters rows with %s — the corpus is empty, "+
			"which is this census reporting PASS over a tree it cannot see",
			approvalsPackage, grantHalf)
	}
	sort.Strings(missing)
	for _, name := range missing {
		t.Errorf("%s filters approval rows with %s but never calls %s: it serves a caller "+
			"the stagings staged for OTHER seats — a colleague's linkedin_match, vcard_create "+
			"or held_draft, whose summary and proposed_change ARE that private row's content",
			name, grantHalf, seatHalf)
	}
}
