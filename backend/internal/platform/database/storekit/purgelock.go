// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// PurgeSubjectLockKey names the advisory lock one purge subject holds for the
// whole of a purge run: a capture exclusion rule, or a seat's personal-mail
// pass. Session holders (the purge itself) and transaction holders (a rule's
// removal) must spell it the same way, or they would not exclude each other.
func PurgeSubjectLockKey(subject ids.UUID) string {
	return "capture-purge:" + subject.String()
}

// LockPurgeSubjectTx waits for any purge run of the subject to finish and then
// holds the subject until the transaction ends.
func LockPurgeSubjectTx(ctx context.Context, tx pgx.Tx, subject ids.UUID) error {
	if _, err := tx.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, PurgeSubjectLockKey(subject)); err != nil {
		return fmt.Errorf("storekit: waiting for a purge of this subject to finish: %w", err)
	}
	return nil
}
