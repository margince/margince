// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extcapability

// What each unit keeps in the object store. The store has no list or size
// call, so the core counts a unit's objects and bytes itself. The count
// changes in the transaction that writes the call's audit row.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The object calls an audit row of a unit's usage names in its evidence.
const (
	ObjectPut    = "put"
	ObjectDelete = "delete"
)

// usageEntity is the audit entity of a unit's usage row.
const usageEntity = "extension_object_usage"

// Usage is what one unit holds in the store of this workspace.
type Usage struct {
	Objects int64
	Bytes   int64
}

// ObjectChange is one object call as the ledger records it: the call, the
// object's name and size, and how it moves the unit's usage. The zero value is
// a call that changes nothing, such as deleting an object that is not there.
type ObjectChange struct {
	Action string
	Name   string
	Size   int64
	Usage  Usage
}

// ErrWrittenNotRecorded is a store write that succeeded under a ledger whose
// count and audit row then did not commit. The caller takes the write back, so
// a stored object is never left uncounted.
var ErrWrittenNotRecorded = errors.New("extcapability: the store call succeeded and its count did not commit")

// Ledger runs one object call while it holds the unit's usage. plan answers
// the change from what the unit holds, or refuses it. write runs next, and the
// count and the audit row commit only after it succeeded. An error from plan or
// write records nothing; a zero change runs no write and records nothing.
type Ledger func(ctx context.Context, unit string, plan func(Usage) (ObjectChange, error), write func() error) error

// PoolLedger is the Ledger over the installation's database. The unit's row
// stays locked until the store write returns, so two calls of one unit cannot
// both count against the same room.
//
// The store and the database cannot commit together, so every failure leaves
// the count high rather than low. A Put whose count does not commit answers
// ErrWrittenNotRecorded, and Objects deletes the object at that name, so an
// object it replaced stays counted with nothing behind it. A Delete
// whose count does not commit leaves the object counted, and a later
// decrement stops at zero. A process that stops between a Put's store call and
// its commit still leaves that object uncounted until a removal sweeps it.
func PoolLedger(pool *pgxpool.Pool) Ledger {
	return func(ctx context.Context, unit string, plan func(Usage) (ObjectChange, error), write func() error) error {
		var refused error
		wrote := false
		err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
			held, err := lockUsage(ctx, tx, unit)
			if err != nil {
				return err
			}
			change, err := plan(held.Usage)
			if err == nil && change == (ObjectChange{}) {
				return nil
			}
			if err == nil {
				err = write()
			}
			if err != nil {
				refused = err
				return err
			}
			wrote = true
			return recordChange(ctx, tx, unit, held, change)
		})
		if refused != nil {
			return refused
		}
		if err != nil {
			slog.ErrorContext(ctx, "extcapability: an extension object call could not be recorded", "err", err, "unit", unit)
			if wrote {
				return ErrWrittenNotRecorded
			}
			return ErrObjectsFailed
		}
		return nil
	}
}

// heldUsage is a unit's usage row, locked for the call.
type heldUsage struct {
	id ids.UUID
	Usage
}

func lockUsage(ctx context.Context, tx pgx.Tx, unit string) (heldUsage, error) {
	if _, err := tx.Exec(ctx, `INSERT INTO extension_object_usage (id, unit) VALUES ($1, $2) ON CONFLICT (unit) DO NOTHING`,
		ids.NewV7(), unit); err != nil {
		return heldUsage{}, err
	}
	var held heldUsage
	err := tx.QueryRow(ctx, `SELECT id, objects, bytes FROM extension_object_usage WHERE unit = $1 FOR UPDATE`, unit).
		Scan(&held.id, &held.Objects, &held.Bytes)
	return held, err
}

func recordChange(ctx context.Context, tx pgx.Tx, unit string, held heldUsage, change ObjectChange) error {
	after := Usage{Objects: max(held.Objects+change.Usage.Objects, 0), Bytes: max(held.Bytes+change.Usage.Bytes, 0)}
	evidence := map[string]any{"extension": unit, "object_call": change.Action, "object": change.Name, "object_bytes": change.Size}
	if cid, ok := principal.CorrelationID(ctx); ok {
		evidence["correlation_id"] = cid.String()
	}
	if _, err := storekit.AuditWithEvidence(ctx, tx, "update", usageEntity, held.id,
		usageImage(held.Usage), usageImage(after), evidence); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE extension_object_usage SET objects = $2, bytes = $3, updated_at = now() WHERE id = $1`,
		held.id, after.Objects, after.Bytes)
	return err
}

// usageImage is the audit image of a usage row, named as its columns are.
func usageImage(u Usage) map[string]int64 {
	return map[string]int64{"objects": u.Objects, "bytes": u.Bytes}
}

// ForgetUnits drops the usage of units whose objects were swept.
func ForgetUnits(ctx context.Context, pool *pgxpool.Pool, units []string) error {
	if len(units) == 0 {
		return nil
	}
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM extension_object_usage WHERE unit = ANY($1)`, units)
		return err
	})
	if err != nil {
		return fmt.Errorf("extcapability: forgetting the object usage of removed extensions: %w", err)
	}
	return nil
}
