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
