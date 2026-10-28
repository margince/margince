// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The queue's case read binds the named recipient in the store, and its count
// agrees with its page; contact pairs leave the pair lane, counted or listed.
func TestTheQueueReadsEachOwnersCasesAndNoContactPair(t *testing.T) {
	c := setupCases(t)
	shared := c.seedCase(t, c.e.Rep1, c.e.Rep2)
	theirs := c.seedCase(t, c.e.Rep3, c.e.Rep3)
	lane := attentionContactCases{store: c.store}
	for recipient, want := range map[ids.UUID][]ids.UUID{c.e.Rep2: shared, c.e.Rep3: theirs} {
		q := attention.CaseQuery{Scope: attention.CaseScopeMine, Recipient: recipient, Limit: 10}
		cases, err := lane.OpenCases(c.e.Admin(), q)
		if err != nil {
			t.Fatal(err)
		}
		count, err := lane.CountCases(c.e.Admin(), q)
		if err != nil {
			t.Fatal(err)
		}
		if len(cases) != 1 || count != 1 || len(cases[0].Face.Members) != len(want) {
			t.Errorf("recipient %s: %d cases, count %d, want the one case of %d", recipient, len(cases), count, len(want))
		}
		if len(cases) == 1 && !slices.Contains(cases[0].Owners, recipient) {
			t.Errorf("recipient %s is not among the case's owners %v", recipient, cases[0].Owners)
		}
	}
	all, err := lane.CountCases(c.e.Admin(), attention.CaseQuery{Scope: attention.CaseScopeAll})
	if err != nil || all != 2 {
		t.Errorf("all cases = %d (%v), want 2", all, err)
	}

	pairs := attentionDuplicates{store: c.store}
	listed, err := pairs.OpenCandidates(c.e.Admin(), 50)
	if err != nil {
		t.Fatal(err)
	}
	counted, err := pairs.CountOpen(c.e.Admin())
	if err != nil {
		t.Fatal(err)
	}
	every, err := c.store.CountOpenDedupeCandidates(c.e.Admin())
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 || counted != 0 || every != 2 {
		t.Errorf("the pair lane lists %d and counts %d of %d open pairs, want none of them: contact pairs are cases",
			len(listed), counted, every)
	}
}
