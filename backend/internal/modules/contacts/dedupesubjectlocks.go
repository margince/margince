// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Subject locks precede candidate locks so erasure cannot miss a new filing or
// finish before stale evidence is filed again. Archived subjects cannot reopen pairs.
func lockLiveDedupeSubjects(ctx context.Context, tx pgx.Tx, kind string, left, right ids.UUID) (bool, error) {
	var table string
	switch kind {
	case entityContact:
		table = entityContact
	case entityLead:
		table = entityLead
	case entityCompany:
		table = entityCompany
	default:
		return false, fmt.Errorf("contacts: unsupported duplicate subject kind %q", kind)
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`SELECT id FROM %s
  WHERE id = ANY(@subjects) AND archived_at IS NULL ORDER BY id FOR SHARE`, table),
		pgx.StrictNamedArgs{"subjects": []ids.UUID{left, right}})
	if err != nil {
		return false, fmt.Errorf("contacts: locking duplicate subjects: %w", err)
	}
	subjects, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return false, fmt.Errorf("contacts: reading duplicate subjects: %w", err)
	}
	return len(subjects) == 2, nil
}
