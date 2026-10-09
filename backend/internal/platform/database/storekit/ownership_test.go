// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// owner_id once sorted by the owner's id, so a cursor minted then carries a
// uuid as its key. Compared against names it would skip or repeat rows, so
// it is refused and the client re-issues the query.
func TestAnOwnerCursorMintedUnderTheIDSortIsRefused(t *testing.T) {
	vocab := map[string]SortField{"owner_id": OwnerNameSort("contact")}
	arg, _ := keysetArgs()
	sorted, err := ParseListSort(context.Background(), new("owner_id"), vocab, arg)
	if err != nil {
		t.Fatal(err)
	}
	key := ids.NewV7().String()
	legacy := mustEncodeOpaque(t, Cursor{CreatedAt: time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC), ID: ids.NewV7(), SortField: "owner_id", SortKey: &key})
	if _, err := sorted.KeysetClause(legacy, arg); !errors.As(err, new(*CursorSortMismatchError)) {
		t.Fatalf("a cursor from the id sort: err = %v, want CursorSortMismatchError", err)
	}

	name := "Anna Becker"
	current, err := sorted.EncodePageCursor(&name, time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC), ids.NewV7())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sorted.KeysetClause(current, arg); err != nil {
		t.Fatalf("a cursor this sort minted itself was refused: %v", err)
	}
}
