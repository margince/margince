// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ParticipantEvidenceMissingSQL matches the partial correction index.
// alias is a compile-time table alias.
func ParticipantEvidenceMissingSQL(alias string) string {
	return fmt.Sprintf(`(%[1]s.domain_evidence_parent_version IS NULL OR %[1]s.domain_evidence_at IS NULL
 OR %[1]s.domain_evidence_precision IS NULL OR %[1]s.domain_evidence_visible IS NULL)`, alias)
}

// ParticipantEvidenceReadySQL lets readiness and its answer share one snapshot.
func ParticipantEvidenceReadySQL() string {
	return `SELECT NOT EXISTS(SELECT 1 FROM (SELECT id FROM activity_participant ap WHERE ` +
		ParticipantEvidenceMissingSQL("ap") + ` ORDER BY id LIMIT @limit) missing) AS ready`
}

// ParticipantEvidenceReady reports whether exact fallback is still required.
func ParticipantEvidenceReady(ctx context.Context, tx pgx.Tx) (bool, error) {
	var ready bool
	err := tx.QueryRow(ctx, ParticipantEvidenceReadySQL(), ParticipantEvidenceArgs(1)).Scan(&ready)
	return ready, err
}

// pageLimitParameter binds the size of a page a sweep reads.
const pageLimitParameter = "limit"

// ParticipantEvidenceArgs keeps readiness and repair limits checked by pgx.
func ParticipantEvidenceArgs(limit int64) pgx.StrictNamedArgs {
	return pgx.StrictNamedArgs{pageLimitParameter: limit}
}
