// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// An arm no search branch declares has no table to read and no expression to
// render itself with, so the hop cannot be composed at all.
//
// The arm gate refuses that before it ships, which is exactly why the runtime
// branch needs asserting rather than assuming: reaching it means the two came
// apart anyway, and the choice made there is to report a fault instead of
// dropping a neighbour type quietly. A dropped type is the failure this whole
// walk exists to avoid — the caller is told that what cannot be evidenced is
// absent, and reads silence as "nothing happened".
//
// No database is touched: the lookup fails before the query is composed, which
// is what lets this be asserted at all.
func TestAHopWhoseRecordTypeNoBranchDeclaresIsReportedRatherThanSkipped(t *testing.T) {
	t.Parallel()
	_, err := hopNeighbors(t.Context(), nil, activityLinkArm{entity: "unicorn", column: "unicorn_id"}, ids.UUID{}, nil)
	if err == nil {
		t.Fatal("a hop over a record type no branch declares composed a query; it must report the fault")
	}
	if !strings.Contains(err.Error(), "unicorn") {
		t.Errorf("the error is %q and does not name the record type it could not read", err)
	}
}

// The section name a record type gets. It is the wire name of a section a
// client renders, so an irregular plural is not cosmetic: the context screen
// looks up `related_companies`, and the naive rule produced `related_companys`,
// which that screen found under no name at all. This reads the module's one
// pluralizer rather than a second copy of the rule — the second copy is what
// got it wrong.
func TestASectionIsNamedForItsRecordTypesPlural(t *testing.T) {
	t.Parallel()
	for entity, want := range map[string]string{
		"contact": "contacts",
		"company": "companies",
		"deal":    "deals",
		"project": "projects",
		"lead":    "leads",
	} {
		if got := pluralRelationName(entity); got != want {
			t.Errorf("pluralRelationName(%q) = %q, want %q", entity, got, want)
		}
	}
}
