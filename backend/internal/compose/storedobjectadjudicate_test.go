// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/platform/storedobjects"
)

// A kind nobody owns is reported, and the pass still settles every kind that is owned.
//
// One unrecognised kind must not keep every other writer's bytes alive, and must not
// pass unnoticed either: a sweep that silently skipped those keys would report a clean
// pass over bytes it never looked at.
func TestAnUnownedKindIsReportedWithoutStoppingThePass(t *testing.T) {
	t.Parallel()
	settled, err := adjudicate(context.Background(), nil, []storedobjects.Provisional{
		{Key: "ws/strange/one", Kind: storedobjects.Kind("strange")},
	})
	if err == nil {
		t.Fatal("an unowned kind was adjudicated silently, so its keys pass as a clean sweep")
	}
	if len(settled.orphans) != 0 || len(settled.adopted) != 0 {
		t.Errorf("settled %+v for a kind nobody owns: nothing may be deleted or retired "+
			"on an answer no owner gave", settled)
	}
}

// Nothing provisional is no work and no fault.
//
// The sweep runs on a timer against every workspace, so the empty ledger is its
// ordinary case rather than an edge one.
func TestAnEmptyLedgerSettlesNothing(t *testing.T) {
	t.Parallel()
	settled, err := adjudicate(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("adjudicating an empty ledger: %v", err)
	}
	if len(settled.orphans) != 0 || len(settled.adopted) != 0 {
		t.Errorf("settled %+v from an empty ledger", settled)
	}
}

// What the owner did not call unreferenced is what the sweep retires.
//
// The complement, not a second query: asking twice would let the two answers disagree,
// and a key in neither list is a key the pass forgets — left provisional, occupying a
// slot in every later pass's limit.
func TestTheComplementIsEveryKeyTheOwnerKept(t *testing.T) {
	t.Parallel()
	keys := []string{"a", "b", "c", "d"}
	for _, c := range []struct {
		name         string
		unreferenced []string
		kept         []string
	}{
		{"none unreferenced", nil, keys},
		{"all unreferenced", keys, nil},
		{"some unreferenced", []string{"b", "d"}, []string{"a", "c"}},
		{"a key the owner never saw is ignored", []string{"z"}, keys},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := keysExcept(keys, c.unreferenced)
			if len(got) != len(c.kept) {
				t.Fatalf("keysExcept(%v, %v) = %v, want %v", keys, c.unreferenced, got, c.kept)
			}
			for i := range got {
				if got[i] != c.kept[i] {
					t.Fatalf("keysExcept(%v, %v) = %v, want %v", keys, c.unreferenced, got, c.kept)
				}
			}
		})
	}
}
