// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"fmt"
	"maps"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// StampRecordAffordances stamps the edit and archive flags of contacts,
// companies and deals, the records with an owner archive rule. Both flags read
// one row authority answer and one fresh archive state. Its return keeps
// archived update authority for field masks; it authorizes no mutation.
func StampRecordAffordances[T any](ctx context.Context, tx pgx.Tx, rule ArchiveRule,
	rows []T, id func(T) ids.UUID, setWritable, setArchivable func(*T, bool),
) (map[ids.UUID]bool, error) {
	switch rule.table {
	case tableDeal, tableContact, tableCompany:
	default:
		return nil, fmt.Errorf("auth: no owner archive affordances for %q", rule.table)
	}
	return stampAffordances(ctx, tx, rule.table, &archiveStamp[T]{rule: rule, set: setArchivable}, rows, id, setWritable)
}

// StampWritable stamps only the edit flag, through the same code as
// StampRecordAffordances. Leads and projects carry no archive flag on the
// wire and have no owner archive rule: their archive asks the delete grant
// and the row write gate alone. The deal-room preview reads only the deal's
// edit flag. Its return is the update authority field masks read.
//
// What it stamps is what a CLIENT is told. EnsureWritable stays the only gate
// a mutation passes.
func StampWritable[T any](ctx context.Context, tx pgx.Tx, table string,
	rows []T, id func(T) ids.UUID, set func(*T, bool),
) (map[ids.UUID]bool, error) {
	return stampAffordances[T](ctx, tx, table, nil, rows, id, set)
}

// archiveStamp is the archive half of a stamp: the record's rule and where
// its flag goes. Nil for a record that carries no archive flag.
type archiveStamp[T any] struct {
	rule ArchiveRule
	set  func(*T, bool)
}

// stampAffordances is the one walk both stamps take. Row authority and the
// archive state are read once for the page and shared by both flags. The
// returned map is the raw update authority: an archived row stays in it, so an
// owner's masked figures stay readable after an archive.
func stampAffordances[T any](ctx context.Context, tx pgx.Tx, table string, archive *archiveStamp[T],
	rows []T, id func(T) ids.UUID, setWritable func(*T, bool),
) (map[ids.UUID]bool, error) {
	if !shareableTables[table] {
		return nil, fmt.Errorf("auth: %q is not a shareable table", table)
	}
	if len(rows) == 0 {
		return map[ids.UUID]bool{}, nil
	}
	p, err := rbacActor(ctx)
	if err != nil {
		return nil, err
	}
	canUpdate := hasUpdateGrant(p, table)
	canArchive, canDelete := false, false
	if archive != nil {
		canArchive, canDelete = archiveAdmission(ctx, p, archive.rule)
	}
	named := make([]ids.UUID, len(rows))
	for i := range rows {
		named[i] = id(rows[i])
	}
	authority, archived, err := pageAuthority(ctx, tx, p, table, named, canUpdate || canArchive)
	if err != nil {
		return nil, err
	}
	update := map[ids.UUID]bool{}
	if canUpdate {
		update = authority
	}
	// The seat ceiling limits actions only; masks read the role's row authority.
	seat := seatAdmitsWrites(p)
	for i, rowID := range named {
		setWritable(&rows[i], liveEdit(seat, update, archived, rowID))
	}
	if archive == nil {
		return update, nil
	}
	archivable := map[ids.UUID]bool{}
	if canArchive {
		live := maps.Clone(authority)
		excludeArchived(live, archived)
		if archivable, err = archivableFromLive(ctx, tx, archive.rule, live, canDelete); err != nil {
			return nil, err
		}
	}
	for i, rowID := range named {
		archive.set(&rows[i], archivable[rowID])
	}
	return update, nil
}

// pageAuthority reads the rows the caller could write and which of those are
// archived, unless no grant could use the answer. The archive state is read
// fresh: a previously selected ArchivedAt may predate a concurrent archive.
func pageAuthority(ctx context.Context, tx pgx.Tx, p principal.Principal, table string,
	named []ids.UUID, wanted bool,
) (authority, archived map[ids.UUID]bool, err error) {
	if !wanted {
		return map[ids.UUID]bool{}, map[ids.UUID]bool{}, nil
	}
	authority, err = rowWriteSubset(ctx, tx, p, table, named)
	if err != nil {
		return nil, nil, err
	}
	archived, err = archivedAmong(ctx, tx, table, authority)
	if err != nil {
		return nil, nil, err
	}
	return authority, archived, nil
}

// liveEdit is the edit flag every record carries: a writing seat, row
// authority, and a row nobody has archived.
func liveEdit(seat bool, authority, archived map[ids.UUID]bool, row ids.UUID) bool {
	return seat && authority[row] && !archived[row]
}
