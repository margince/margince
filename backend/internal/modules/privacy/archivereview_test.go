// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The archive review refuses before it opens a transaction. The eraser holds a
// nil database, so a refusal that came after a query would panic instead.

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func archiveActor(kind principal.PrincipalType) context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type: kind, ID: "x:" + ids.NewV7().String(), UserID: ids.NewV7(), SeatType: principal.SeatFull,
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll, Objects: map[string]principal.ObjectGrant{
			retentionPolicyObject: {Read: true, Delete: true},
		}},
	})
}

func TestTheArchiveReviewRefusesAnAgentBeforeAnyQuery(t *testing.T) {
	t.Parallel()
	ctx := archiveActor(principal.PrincipalAgent)
	eraser := NewEraser(nil)
	if _, err := eraser.ListArchivedActivities(ctx, ArchiveFilter{}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("an agent listing the archive got %v, want ErrPermissionDenied", err)
	}
	if _, err := eraser.DeleteArchivedActivities(ctx, []ids.UUID{ids.NewV7()}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("an agent deleting from the archive got %v, want ErrPermissionDenied", err)
	}
}

func TestAnArchiveDeletionNamesOneToAHundredRecords(t *testing.T) {
	t.Parallel()
	for name, count := range map[string]int{"no records": 0, "one past the limit": ArchiveDeletionLimit + 1} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			named := make([]ids.UUID, count)
			for i := range named {
				named[i] = ids.NewV7()
			}
			_, err := NewEraser(nil).DeleteArchivedActivities(archiveActor(principal.PrincipalHuman), named)
			var refusal *httperr.DetailedError
			if !errors.As(err, &refusal) || refusal.Status != http.StatusUnprocessableEntity ||
				len(refusal.Fields) != 1 || refusal.Fields[0].Field != "activity_ids" {
				t.Errorf("deleting %d records got %v, want a validation error on activity_ids", count, err)
			}
		})
	}
}

// One record's failed transaction leaves it out of the deleted list and names
// it as failed. The records after it still run, and a repeated id runs once.
func TestAFailedRecordDoesNotStopTheRestOfADeletion(t *testing.T) {
	t.Parallel()
	first, broken, held, last := ids.NewV7(), ids.NewV7(), ids.NewV7(), ids.NewV7()
	runs := map[ids.UUID]int{}
	out := deleteEach(context.Background(), []ids.UUID{first, broken, held, first, last},
		func(_ context.Context, id ids.UUID) (string, error) {
			runs[id]++
			switch id {
			case broken:
				return "", errors.New("the transaction failed")
			case held:
				return keptLegalHold, nil
			}
			return "", nil
		})
	if !slices.Equal(out.Deleted, []ids.UUID{first, last}) {
		t.Errorf("deleted %v, want %v", out.Deleted, []ids.UUID{first, last})
	}
	if !slices.Equal(out.Failed, []ids.UUID{broken}) {
		t.Errorf("failed %v, want %v", out.Failed, []ids.UUID{broken})
	}
	if len(out.Kept) != 1 || out.Kept[0] != (KeptActivity{ActivityID: held, Reason: keptLegalHold}) {
		t.Errorf("kept %v, want %s for a legal hold", out.Kept, held)
	}
	if runs[first] != 1 {
		t.Errorf("a repeated id ran %d times, want once", runs[first])
	}
}

// The nightly activity selector and the archive review read one keeper list.
// The clock never erases what the review reports as kept, nor the reverse.
func TestTheNightlySelectorLeavesOutEveryArchiveKeeper(t *testing.T) {
	t.Parallel()
	selector := retentionSelectors["activity/"]
	reasons := activityKeptReasonSQL(3, 4)
	for _, keeper := range activityKeepers(3, 4) {
		if !strings.Contains(selector, excluding(keeper.holds)) {
			t.Errorf("the activity selector does not leave out what keeps a record as %s:\n%s", keeper.reason, keeper.holds)
		}
		if !strings.Contains(reasons, "WHEN "+keeper.holds+" THEN '"+keeper.reason+"'") {
			t.Errorf("the archive review does not report %s for:\n%s", keeper.reason, keeper.holds)
		}
	}
}
