// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"fmt"
	"maps"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// StampRecordAffordances shares row authority between edit and archive flags.
// Its return keeps archived update authority for field masks; it authorizes no mutation.
func StampRecordAffordances[T any](ctx context.Context, tx pgx.Tx, rule ArchiveRule,
	rows []T, id func(T) ids.UUID, setWritable, setArchivable func(*T, bool),
) (map[ids.UUID]bool, error) {
	switch rule.table {
	case tableDeal, tableContact, tableCompany:
	default:
		return nil, fmt.Errorf("auth: no owner archive affordances for %q", rule.table)
	}
	p, err := rbacActor(ctx)
	if err != nil {
		return nil, err
	}
	update := make(map[ids.UUID]bool, len(rows))
	canUpdate := hasUpdateGrant(p, rule.table)
	canArchive, canDelete := archiveAdmission(ctx, p, rule)
	if !canUpdate && !canArchive {
		for i := range rows {
			setWritable(&rows[i], false)
			setArchivable(&rows[i], false)
		}
		return update, nil
	}
	named := make([]ids.UUID, len(rows))
	for i := range rows {
		named[i] = id(rows[i])
	}
	authority, err := rowWriteSubset(ctx, tx, p, rule.table, named)
	if err != nil {
		return nil, err
	}
	// A previously selected ArchivedAt may predate a concurrent archive.
	archived, err := archivedAmong(ctx, tx, rule.table, authority)
	if err != nil {
		return nil, err
	}
	live := maps.Clone(authority)
	excludeArchived(live, archived)
	if canUpdate {
		update = authority
	}
	archivable := map[ids.UUID]bool{}
	if canArchive {
		archivable, err = archivableFromLive(ctx, tx, rule, live, canDelete)
		if err != nil {
			return nil, err
		}
	}
	for i, rowID := range named {
		setWritable(&rows[i], update[rowID] && !archived[rowID])
		setArchivable(&rows[i], archivable[rowID])
	}
	return update, nil
}
