// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The rows a closing thread verdict left open before it closed the messages a
// thread had already opened. The thread-hold data task reads them here and
// closes them through RecordOutcomeOnThreadTx.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// heldThreadImportsSQL joins one seat's import rows to that seat's own closing
// verdict on the message's thread. It excludes the rows siblingsTx excludes.
const heldThreadImportsSQL = `
	  FROM capture_import i
	  JOIN activity a ON a.id = i.activity_id
	  JOIN capture_thread_verdict v ON v.thread_key = a.thread_key AND v.user_id = i.user_id
	 WHERE v.status = ANY(@closing)
	   AND a.archived_at IS NULL AND a.restricted_at IS NULL`

// EstimateOpenUnderHeldThreads counts the import rows still open under a thread
// their own seat holds. It is the data task's dry run, and reads 0 once done.
func EstimateOpenUnderHeldThreads(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT i.id `+heldThreadImportsSQL+`
	   AND i.verdict_status = ANY(@opened) LIMIT @cap) AS candidates`,
		pgx.NamedArgs{"closing": closingVerdicts, "opened": openedVerdicts, "cap": maximum + 1}).Scan(&count)
	if err != nil {
		return 0, false, fmt.Errorf("capture: counting messages open under held threads: %w", err)
	}
	return min(count, maximum), count > maximum, nil
}

// HeldThreadSeat names one seat's closing verdict on one thread.
type HeldThreadSeat struct {
	ThreadKey string
	UserID    ids.UUID
}

// OpenUnderHeldThreadsTx pages the messages after `after`, by activity id, that
// a closing verdict on their thread still has to take. It returns the page, and
// the seats whose verdict takes them.
//
// The page holds every status the closing verdict takes, undecided ones too.
// siblingsTx pages a closing verdict's rows by activity id as well. So one call
// for a seat takes every row of that seat's thread on this page.
func OpenUnderHeldThreadsTx(
	ctx context.Context, tx pgx.Tx, after ids.UUID, limit int,
) ([]ids.UUID, []HeldThreadSeat, error) {
	rows, err := tx.Query(ctx, `
		WITH page AS (
		  SELECT DISTINCT a.id `+heldThreadImportsSQL+`
		     AND (i.verdict_status IS NULL OR i.verdict_status = ANY(@taken))
		     AND a.id > @after
		   ORDER BY a.id
		   LIMIT @page_size)
		SELECT a.id, a.thread_key, i.user_id `+heldThreadImportsSQL+`
		   AND a.id IN (SELECT id FROM page)
		   AND (i.verdict_status IS NULL OR i.verdict_status = ANY(@taken))
		 ORDER BY a.id, i.user_id`,
		pgx.NamedArgs{
			"closing": closingVerdicts, "taken": siblingStatusesTaken(VerdictHeld),
			"after": after, "page_size": limit,
		})
	if err != nil {
		return nil, nil, fmt.Errorf("capture: paging messages open under held threads: %w", err)
	}
	defer rows.Close()
	var page []ids.UUID
	var seats []HeldThreadSeat
	seen := map[HeldThreadSeat]bool{}
	for rows.Next() {
		var activity ids.UUID
		var seat HeldThreadSeat
		if err := rows.Scan(&activity, &seat.ThreadKey, &seat.UserID); err != nil {
			return nil, nil, fmt.Errorf("capture: paging messages open under held threads: %w", err)
		}
		if len(page) == 0 || page[len(page)-1] != activity {
			page = append(page, activity)
		}
		if !seen[seat] {
			seen[seat] = true
			seats = append(seats, seat)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("capture: paging messages open under held threads: %w", err)
	}
	return page, seats, nil
}
