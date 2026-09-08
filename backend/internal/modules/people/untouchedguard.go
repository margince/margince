// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// A write that may only land while nobody has touched the record since a given
// instant, re-asked INSIDE the transaction that writes.
//
// The caller that needs it is the import undo: it reads which imported rows a
// human has edited, then archives the ones nobody touched. Those were two
// transactions, so a human could act in the window between them and have their
// edit reversed anyway — the check said untouched, and by the time the archive
// ran it was not. Narrowing the window is not the answer; a precondition the
// WRITE re-asks under its own row lock is, because then the two cannot
// disagree.
//
// It is an option rather than a parameter so no existing caller changes: a verb
// asked without one behaves exactly as it did.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// WriteOption is a precondition a caller attaches to a write.
type WriteOption func(*writeOptions)

type writeOptions struct {
	untouchedSince *time.Time
	atVersion      *int64
}

// NotTouchedByHumanSince refuses the write when a HUMAN has acted on the record
// since `at`. What counts as acting is any human-actor audit row on the entity,
// not narrowly an update: somebody who independently archived an imported row
// is exactly the case this protects, and a narrower filter would reverse their
// act too.
func NotTouchedByHumanSince(at time.Time) WriteOption {
	return func(o *writeOptions) { o.untouchedSince = &at }
}

// OnlyAtVersion refuses the write unless the record is still at this version —
// the option form of the contract's optional If-Match, asked inside the
// transaction that writes and under the row lock it already holds.
//
// A nil version attaches no precondition, so a caller that has one and a caller
// that does not spell the call the same way. That matters at the seam this
// exists for: an agent write carries the version its approval was released
// against, and an unapproved call at a static tier carries none.
func OnlyAtVersion(v *int64) WriteOption {
	return func(o *writeOptions) { o.atVersion = v }
}

func collectWriteOptions(opts []WriteOption) writeOptions {
	var out writeOptions
	for _, apply := range opts {
		apply(&out)
	}
	return out
}

// refuseIfVersionMoved answers skew when the locked row is no longer at the
// version the caller's authority was granted against.
//
// current comes from a read taken AFTER the row lock, so what it compares is
// the row this transaction is about to write — which is the whole point of the
// pin travelling this far. A check before the lock proves the row was right a
// moment ago and nothing about the write.
func refuseIfVersionMoved(entity string, current *int64, o writeOptions) error {
	if o.atVersion == nil {
		return nil
	}
	// A row that answers no version cannot satisfy a pin, so it refuses rather
	// than passing. The alternative reads as "checked" and is not: an unversioned
	// read would wave through exactly the write the pin was attached to stop.
	if current == nil {
		return fmt.Errorf(
			"the %s carries no version to compare against the %d this write was authorised at: %w",
			entity, *o.atVersion, apperrors.ErrVersionSkew)
	}
	if *current == *o.atVersion {
		return nil
	}
	return fmt.Errorf(
		"the %s is at version %d, not the version %d this write was authorised against: %w",
		entity, *current, *o.atVersion, apperrors.ErrVersionSkew)
}

// HumanTouchedError refuses a write whose caller asked for it only while the
// record was untouched, and it names the record so a report can say which one
// it left alone rather than only how many.
type HumanTouchedError struct {
	EntityType string
	EntityID   ids.UUID
}

func (e *HumanTouchedError) Error() string {
	return fmt.Sprintf("a person has changed this %s since it was imported, so it was left alone", e.EntityType)
}

// refuseIfHumanTouched enforces the precondition inside the caller's own
// transaction, which is the whole point: the archive that follows takes the
// row's lock, so a human write either lands entirely before this read or
// entirely after the commit, and never between them.
func refuseIfHumanTouched(
	ctx context.Context, tx pgx.Tx, entityType string, id ids.UUID, opts writeOptions,
) error {
	if opts.untouchedSince == nil {
		return nil
	}
	var touched bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM audit_log
			 WHERE entity_type = $1 AND entity_id = $2
			   AND actor_type = 'human' AND occurred_at > $3)`,
		entityType, id, *opts.untouchedSince).Scan(&touched); err != nil {
		return fmt.Errorf("checking whether a person has changed this %s: %w", entityType, err)
	}
	if touched {
		return &HumanTouchedError{EntityType: entityType, EntityID: id}
	}
	return nil
}
