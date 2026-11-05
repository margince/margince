// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"errors"
	"testing"
)

// A merge made from a company page settles the open pair of the two companies
// the way the queue does. The pair stays, marked merged by the caller. The
// merged list shows it, and an undo is refused as not undoable.
func TestADirectCompanyMergeKeepsTheOpenPairAsMerged(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	holder := e.createNamedCompany(ctx, t, "Example Group", "example.com")
	created := e.createNamedCompany(ctx, t, "Cloudy", "cloudy.test", "cloud.example.com")
	pair := e.openPairID(ctx, t, entityCompany, holder.UUID, created.UUID)
	if pair.IsZero() {
		t.Fatal("no open pair was filed, so the merge has nothing to settle")
	}

	if _, err := e.store.MergeCompany(ctx, created, holder, nil); err != nil {
		t.Fatalf("merge from the company page: %v", err)
	}

	row, err := e.store.GetDedupeCandidate(ctx, pair)
	if err != nil {
		t.Fatalf("read the pair after the merge: %v", err)
	}
	if row.Disposition != dispositionMerged || row.DisposedBy == nil || *row.DisposedBy != e.rep || row.DisposedAt == nil {
		t.Fatalf("pair = %+v, want merged by the caller with a time", row)
	}
	listed, _, err := e.store.ListDedupeCandidates(ctx, DedupeQueueInput{Status: dispositionMerged, EntityType: entityCompany})
	if err != nil {
		t.Fatalf("list merged pairs: %v", err)
	}
	found := false
	for _, r := range listed {
		found = found || r.ID == pair
	}
	if !found {
		t.Errorf("the merged list does not show the pair a direct merge settled")
	}
	if _, err := e.store.UndoDedupeDisposition(ctx, pair); !errors.Is(err, ErrNotUndoable) {
		t.Errorf("undo = %v, want ErrNotUndoable", err)
	}
}
