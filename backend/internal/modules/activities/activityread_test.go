// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The cursor a resumed read carries is keyed to whichever order actually
// ran, so the two may never disagree. The ORDER BY choice itself
// (newest-first vs. due-soonest) is proven behaviourally by
// TestThePlannedLaneCapKeepsTheMostOverdueNotTheNewestLogged, which fails
// against the pre-fix ordering — a unit test asserting the literal SQL
// string here would only restate orderClause's own body.

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A cursor from the recency read cannot resume an open-and-due read: it is
// keyed to occurred_at, the query runs ordered by due_at, and applying one
// axis's cursor to the other axis's order returns silently wrong rows rather
// than the next page.
//
// The sentinel that used to say so is gone, and this asserts the thing that
// replaced it: the keyset REFUSES the token itself, because a ListSort mints a
// cursor carrying the field it was ordered by and checks it on the way back in.
// One machine now answers for every order this read takes rather than a guard
// per pairing somebody has to remember to add.
func TestACursorFromTheRecencyOrderCannotResumeAnOpenAndDueRead(t *testing.T) {
	until := time.Now()
	minted, err := storekit.EncodeOpaque(storekit.Cursor{
		Scope: "activities", CreatedAt: time.Now(), ID: ids.NewV7(), SortField: "occurred_at", SortDesc: true,
	})
	if err != nil {
		t.Fatalf("minting a recency cursor: %v", err)
	}
	_, _, _, _, err = listActivitiesFilter(unscopedCtx(), ListActivitiesInput{
		OpenAndDueBy: &until,
		Cursor:       &minted,
	})
	if _, ok := errors.AsType[*storekit.CursorSortMismatchError](err); !ok {
		t.Fatalf("resuming the due queue with a timeline cursor → %v, want a sort mismatch", err)
	}
}

func TestCalendarMailFoldOnlyRunsWhenItsMeetingCanBeListed(t *testing.T) {
	id := ids.NewV7()
	for _, tc := range []struct {
		name  string
		kind  *string
		kinds []string
		fold  bool
	}{
		{name: "all", fold: true},
		{name: "mail and meetings", kinds: []string{"email", "meeting"}, fold: true},
		{name: "meetings only", kinds: []string{"meeting"}},
		{name: "mail only", kind: new("email")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page, err := timelinePageSQL(unscopedCtx(), ListActivitiesInput{
				EntityType: new("company"), EntityID: &id, CollapseSeatCopies: true,
				Kind: tc.kind, Kinds: tc.kinds,
			}, 25)
			if err != nil {
				t.Fatal(err)
			}
			if folded := strings.Contains(page.sql, foldedCalendarMailSQL); folded != tc.fold {
				t.Errorf("folding calendar mail = %t, want %t", folded, tc.fold)
			}
		})
	}
}
