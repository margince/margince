// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

import (
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTagIDsNamedReadsEveryLeafOnATagField(t *testing.T) {
	first, second := ids.NewV7(), ids.NewV7()
	tree := storekit.Predicate{And: []storekit.Predicate{
		{Field: "tag", Op: "eq", Value: first.String()},
		{Or: []storekit.Predicate{
			{Field: "tag", Op: "in", Value: []any{second.String(), first.String(), 7, "not-an-id"}},
			{Field: "city", Op: "eq", Value: ids.NewV7().String()},
		}},
	}}
	got := tagIDsNamed(tree, map[string]bool{"tag": true})
	if want := []ids.UUID{first, second}; !slices.Equal(got, want) {
		t.Errorf("tag ids named = %v, want %v — each once, nested branches and arrays read, other fields and non-ids skipped", got, want)
	}
}
