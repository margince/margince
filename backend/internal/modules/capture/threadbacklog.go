// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
)

// threadOwedAnswerSQL selects the thread questions a pass can still answer,
// due or not. ClaimDue adds the clock and the lease; the gauge adds neither,
// because a thread backing off is still hidden mail. One text for both, so the
// gauge counts what the pass works. Both queries select from
// capture_thread_verdict without an alias, because the connection check names
// the table.
var threadOwedAnswerSQL = `status = 'pending'
	AND next_attempt_at IS NOT NULL
	AND attempts < ` + strconv.Itoa(ThreadVerdictMaxAttempts) + `
	AND EXISTS (SELECT 1 FROM activity a
	             WHERE a.id = first_activity_id
	               AND a.restricted_at IS NULL)
	AND EXISTS (SELECT 1 FROM capture_connection c
	             WHERE c.user_id = capture_thread_verdict.user_id
	               AND c.archived_at IS NULL
	               AND c.mail_posture = 'classified')`

// ThreadBacklog is the held threads a confidentiality pass still owes an
// answer, as the metrics surface reads them.
type ThreadBacklog struct {
	// Waiting is how many threads have no answer yet.
	Waiting int64
	// OldestWait is how long the longest-held one has been invisible to the
	// team, counted from the capture of the message that opened the thread. Zero
	// when nothing waits.
	OldestWait time.Duration
}

// ReadThreadBacklog answers the installation's backlog in one scan. Both
// clocks are the database's, so a replica with a skewed clock reports the same
// age as every other.
func ReadThreadBacklog(ctx context.Context, db *database.DB) (ThreadBacklog, error) {
	var out ThreadBacklog
	err := db.Tx(ctx, func(tx pgx.Tx) error {
		var seconds float64
		if err := tx.QueryRow(ctx, `
			SELECT count(*),
			       COALESCE(EXTRACT(EPOCH FROM now() - min(
			         (SELECT a.created_at FROM activity a WHERE a.id = first_activity_id))), 0)
			  FROM capture_thread_verdict
			 WHERE `+threadOwedAnswerSQL).Scan(&out.Waiting, &seconds); err != nil {
			return err
		}
		out.OldestWait = time.Duration(seconds * float64(time.Second))
		return nil
	})
	if err != nil {
		return ThreadBacklog{}, fmt.Errorf("capture: reading the confidentiality backlog: %w", err)
	}
	return out, nil
}
