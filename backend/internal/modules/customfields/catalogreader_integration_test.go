// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package customfields

import "testing"

// A catalog read on a dead transaction fails rather than answering an empty catalog.
func TestTheCatalogReaderReportsAFailedQuery(t *testing.T) {
	f := setupCandidates(t)
	tx, err := f.owner.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(f.ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := ActiveColumnsIn(f.ctx, tx, "lead"); err == nil {
		t.Error("ActiveColumnsIn on a closed transaction succeeded")
	}
}
