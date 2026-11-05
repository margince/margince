// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// Scope filters and cards resolve the same owner, including folded rows.

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAFoldedRowNamesTheSameOwnerForScopeAndDisplay(t *testing.T) {
	t.Parallel()
	holder := ids.NewV7()
	members := []ranked{{ownerRef: ownedBy(holder)}, {ownerRef: ownedBy(holder)}}
	group := ownerOfTheGroup(members)
	if group.kind != ownerNamed || group.user != holder {
		t.Fatalf("a pile agreeing on one owner says %v, so this test no longer reaches "+
			"the shape it was written for", group)
	}
	folded := ranked{ownerRef: group}
	if got, readable := answersTo(folded); !readable || got != holder {
		t.Fatalf("folded owner = %v, readable %v; want %v", got, readable, holder)
	}
}
