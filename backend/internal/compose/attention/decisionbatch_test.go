// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// How many reads the decision lane costs to render.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// serviceForDuplicates wires the feed with only the dedupe queue standing in;
// every other lane answers empty, which keeps this about the decision lane.
func serviceForDuplicates(t *testing.T, dup Duplicates) *Service {
	t.Helper()
	return NewService(
		stubApprovals{}, dup, &stubTasks{}, stubReceipts{},
		stubBriefing{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, fixedClock)
}

// pairsOf builds n open candidate pairs of one entity type, which is 2n records
// for the lane to name.
func pairsOf(entityType string, n int) []DuplicatePair {
	pairs := make([]DuplicatePair, 0, n)
	for range n {
		pairs = append(pairs, DuplicatePair{
			ID: ids.NewV7(), EntityType: entityType, Confidence: 0.9,
			LeftID: ids.NewV7(), RightID: ids.NewV7(),
		})
	}
	return pairs
}

// THE LANE NAMES A PAGE IN ONE READ PER ENTITY TYPE.
//
// Naming a record is a read of that record, and the lane renders up to ten
// pairs — so a read per record is twenty scoped transactions on the surface a
// rep opens first every morning, each of them a full composite record read to
// produce a name, one line and a count.
func TestTheDecisionLaneNamesAPageInOneReadPerEntityType(t *testing.T) {
	var asked []string
	s := serviceForDuplicates(t, stubDuplicates{pairs: pairsOf("person", 10), open: 10, asked: &asked})

	if _, _, err := s.decisionsToDepth(context.Background(), 10); err != nil {
		t.Fatalf("decisionsToDepth: %v", err)
	}
	if len(asked) != 1 {
		t.Errorf("named the page in %d read(s) (%v), want one — ten pairs is twenty records, and "+
			"a read each is what makes the morning screen slow", len(asked), asked)
	}
}

// AND A MIXED PAGE COSTS ONE PER TYPE, not one per record and not one for all:
// the row scope is a different predicate per table, so the types cannot share a
// statement.
func TestAMixedPageCostsOneReadPerTypePresent(t *testing.T) {
	pairs := append(pairsOf("person", 3), pairsOf("organization", 2)...)
	pairs = append(pairs, pairsOf("lead", 1)...)
	var asked []string
	s := serviceForDuplicates(t, stubDuplicates{pairs: pairs, open: len(pairs), asked: &asked})

	if _, _, err := s.decisionsToDepth(context.Background(), 10); err != nil {
		t.Fatalf("decisionsToDepth: %v", err)
	}
	seen := map[string]int{}
	for _, entityType := range asked {
		seen[entityType]++
	}
	if len(asked) != 3 || seen["person"] != 1 || seen["organization"] != 1 || seen["lead"] != 1 {
		t.Errorf("reads = %v, want exactly one per type present", asked)
	}
}
