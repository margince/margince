// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// The outbound category vocabulary is spelled twice (once in Go, once as a
// CHECK constraint on communication_decision), and the two must agree.
// enumsync_test.go holds the constants and Categories() to that CHECK. This
// file holds the properties of the Go list that comparison depends on.
//
// What this gate deliberately does NOT assert: that every category has a
// validator arm. Nine of the fourteen resolve to `default:` in
// consent/authorizevalidators.go and stay unsupported on purpose — they have no
// record evidence to read, so they fall through to the legacy verdict. That is
// a documented design choice, and a gate demanding otherwise would fail against
// correct code.

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// TestEveryCategoryCarriesADistinctWireValue holds the property the constraint
// comparison in enumsync_test.go silently depends on.
//
// Categories() is built from a map keyed by the Category values themselves, so
// two constants sharing a string would collapse into ONE member — and the
// comparison against the constraint would still pass, having compared a
// vocabulary that had already lost a member. A census that can fail short has
// already failed, so the count is asserted against the declared constants
// rather than against itself.
func TestEveryCategoryCarriesADistinctWireValue(t *testing.T) {
	t.Parallel()

	seen := map[commsauthz.Category]bool{}
	for _, c := range commsauthz.Categories() {
		if seen[c] {
			t.Errorf("category %q appears twice in the vocabulary", c)
		}
		seen[c] = true
		if strings.TrimSpace(string(c)) == "" {
			t.Error("a category carries an empty wire value")
		}
		if !c.Valid() {
			t.Errorf("category %q is listed by Categories() but Valid() denies it", c)
		}
	}
	// The five that serve the subject are the ones a hard suppression may not
	// stop. Asserting the COUNT here means a sixth cannot be added silently:
	// ServesTheSubject widens what reaches somebody who asked us to stop, so it
	// is the one predicate in this vocabulary that must never grow by accident.
	serving := 0
	for _, c := range commsauthz.Categories() {
		if c.ServesTheSubject() {
			serving++
		}
	}
	if want := 5; serving != want {
		t.Errorf("%d categories serve the subject, want %d: this predicate decides what passes a "+
			"hard suppression, so a change here reaches somebody who asked us to stop", serving, want)
	}
}
