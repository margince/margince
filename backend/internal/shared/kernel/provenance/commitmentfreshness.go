// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package provenance

import (
	"fmt"
	"time"
)

// CommitmentHorizonDays bounds automatic work without a recent deadline or renewed evidence.
const CommitmentHorizonDays = 30

// CurrentCommitmentClause checks the current source, including a recorded renewal.
// source, locator and due are compile-time SQL expressions supplied by readers.
func CurrentCommitmentClause(source, locator, due string, asOf time.Time, arg func(any) int) string {
	cutoff := arg(asOf.AddDate(0, 0, -CommitmentHorizonDays))
	return fmt.Sprintf(`EXISTS (SELECT 1 FROM activity_commitment_admission admitted
 JOIN activity evidence ON evidence.id=coalesce(admitted.renewed_by, admitted.source_activity_id)
 WHERE admitted.source_activity_id=%s AND admitted.locator=%s
 AND admitted.current_work AND admitted.meaning->>'state'='open'
 AND evidence.archived_at IS NULL AND evidence.occurred_at <= $%d
 AND (evidence.kind <> 'meeting' OR evidence.meeting_status IS DISTINCT FROM 'canceled')
 AND (evidence.occurred_at >= $%d OR %s >= $%d))`, source, locator, arg(asOf), cutoff, due, cutoff)
}
