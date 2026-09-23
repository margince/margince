// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Naming the original behind an activity that predates raw_capture_id.
//
// This module owns `activity`, so the write lives here; the read that finds
// which raw_capture row an activity belongs to lives in compose instead,
// because that is the one place entitled to know both writers' key spellings
// (rawcapturelinkbackfill.go explains why). Splitting the write out is the
// same shape CancelCapturedMeetingTx uses for the sibling backfill beside
// this one: the caller orchestrates and reads, the owning module performs
// the write on the caller's own transaction.
//
// No audit row and no event: raw_capture_id is provenance a reader follows,
// never a fact a user typed or a surface renders, so it carries the same
// silence BackfillParticipantsBatch's structural backfill does and neither
// of theirs.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// StoredOriginalLink is one candidate the caller's read named: an activity and
// the raw_capture row it was read from.
type StoredOriginalLink struct {
	ActivityID   ids.ActivityID
	RawCaptureID ids.UUID
}

// LinkStoredOriginalTx sets raw_capture_id on every activity links names, and
// reports how many it actually set.
//
// The predicate keeps `raw_capture_id IS NULL`, restated here rather than
// trusted to the caller's read: this is the statement that writes, and it must
// stay additive even if a link named a row live capture — or a prior call —
// had already settled between the read and this write. One statement over the
// whole batch, not a loop of single updates, so there is no window in which a
// concurrent write could land between two of them.
func LinkStoredOriginalTx(ctx context.Context, tx pgx.Tx, links []StoredOriginalLink) (int, error) {
	if len(links) == 0 {
		return 0, nil
	}
	activityIDs := make([]ids.UUID, len(links))
	rawCaptureIDs := make([]ids.UUID, len(links))
	for i, link := range links {
		activityIDs[i] = link.ActivityID.UUID
		rawCaptureIDs[i] = link.RawCaptureID
	}
	tag, err := tx.Exec(ctx, `
		UPDATE activity a SET raw_capture_id = v.raw_capture_id
		  FROM unnest($1::uuid[], $2::uuid[]) AS v(activity_id, raw_capture_id)
		 WHERE a.id = v.activity_id AND a.raw_capture_id IS NULL`,
		activityIDs, rawCaptureIDs)
	if err != nil {
		return 0, fmt.Errorf("activities: naming the original behind a captured record: %w", err)
	}
	return int(tag.RowsAffected()), nil
}
