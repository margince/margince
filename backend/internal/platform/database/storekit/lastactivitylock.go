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

// LockLastActivityTargets locks the enumerated reach before probes or writes.
// Non-key locks admit foreign-key shares; relationships added concurrently
// still require retrying the transaction with RetryLockCycles.
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

// clockTableIdent renders one of the constants as its literal, so no caller's
// string is ever formatted into a statement.
func clockTableIdent(table ClockTable) string {
	if table == ClockCompanies || table == ClockDeals || table == ClockProjects {
		return string(table)
	}
	return string(ClockContacts)
}
