// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// LastActivityTargets are the records one write files under: the rows whose
// last-activity clocks its triggers will move.
type LastActivityTargets struct {
	Contacts, Deals, Companies, Projects []ids.UUID
}

// LockLastActivityTargets takes FOR UPDATE on everything targets reach — the
// named records, the contacts' current employers and the deals' companies — in
// the one (kind, id) order every writer shares (migration 1790739626).
//
// It must run BEFORE the write's attach probes and inserts. A probe's FOR SHARE
// that a trigger later upgrades is a deadlock between any two writers naming
// the same record, and a lock reached through a contact or a deal is taken out
// of order unless the whole set is held first.
func LockLastActivityTargets(ctx context.Context, tx pgx.Tx, targets LastActivityTargets) error {
	if len(targets.Contacts)+len(targets.Deals)+len(targets.Companies)+len(targets.Projects) == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx,
		`SELECT lock_last_activity_targets($1::uuid[], $2::uuid[], $3::uuid[], $4::uuid[])`,
		nonNil(targets.Contacts), nonNil(targets.Deals), nonNil(targets.Companies), nonNil(targets.Projects)); err != nil {
		return fmt.Errorf("lock last-activity targets: %w", err)
	}
	return nil
}

// nonNil binds an empty array rather than NULL, so the function's unnest and
// ANY read "none" the way they read it for every other empty list.
func nonNil(list []ids.UUID) []ids.UUID {
	if list == nil {
		return []ids.UUID{}
	}
	return list
}

// ClockTable is a table that stores a last-activity clock.
type ClockTable string

// The three tables whose clock counts only the past.
const (
	ClockContacts  ClockTable = "contact"
	ClockCompanies ClockTable = "company"
	ClockDeals     ClockTable = "deal"
)

// ClockProjects is the project clock, which keeps its own meeting and time
// rule but drops tasks as the other three do.
const ClockProjects ClockTable = "project"

// ClockQuerier is what both a transaction and a data task's estimate read with.
type ClockQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// FutureClocks answers the next records of table after `after`, in id order,
// whose stored clock is later than now: values the clock rule no longer gives.
func FutureClocks(ctx context.Context, q ClockQuerier, table ClockTable, after ids.UUID, limit int) ([]ids.UUID, error) {
	rows, err := q.Query(ctx, `SELECT id FROM `+clockTableIdent(table)+`
		 WHERE id > $1 AND last_activity_at > now() ORDER BY id LIMIT $2`, after, limit)
	if err != nil {
		return nil, fmt.Errorf("reading %s clocks dated in the future: %w", table, err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
}

// CountFutureClocks counts up to maximum+1 records of table whose stored clock
// is later than now.
func CountFutureClocks(ctx context.Context, q ClockQuerier, table ClockTable, maximum int64) (int64, error) {
	var count int64
	if err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 FROM `+clockTableIdent(table)+`
		 WHERE last_activity_at > now() LIMIT $1) future`, maximum+1).Scan(&count); err != nil {
		return 0, fmt.Errorf("counting %s clocks dated in the future: %w", table, err)
	}
	return count, nil
}

// clockTableIdent renders one of the constants as its literal, so no caller's
// string is ever formatted into a statement.
func clockTableIdent(table ClockTable) string {
	if table == ClockCompanies || table == ClockDeals || table == ClockProjects {
		return string(table)
	}
	return string(ClockContacts)
}
