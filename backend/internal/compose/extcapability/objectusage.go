// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extcapability

// What each unit keeps in the object store. The store has no list or size
// call, so the core counts a unit's objects and bytes itself, in the same
// transaction as the audit row of the call that changed them.

import (
	"context"
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
// object's name and size, and how it moves the unit's usage.
type ObjectChange struct {
	Action string
	Name   string
	Size   int64
	Usage  Usage
}

// Ledger runs one object call while it holds the unit's usage. plan answers
// the change from what the unit holds, or refuses it. The change is recorded
// before write runs, so bytes are never stored without their audit row, and an
// error from plan or write records nothing.
type Ledger func(ctx context.Context, unit string, plan func(Usage) (ObjectChange, error), write func() error) error

// PoolLedger is the Ledger over the installation's database. The unit's row
// stays locked until the store write returns, so two calls of one unit cannot
// both count against the same room.
func PoolLedger(pool *pgxpool.Pool) Ledger {
	return func(ctx context.Context, unit string, plan func(Usage) (ObjectChange, error), write func() error) error {
		var refused error
		err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
			held, err := lockUsage(ctx, tx, unit)
			if err != nil {
				return err
			}
			change, err := plan(held.Usage)
			if err == nil {
				if err = recordChange(ctx, tx, unit, held, change); err != nil {
					return err
				}
				err = write()
			}
			refused = err
			return err
		})
		if refused != nil {
			return refused
		}
		if err != nil {
			slog.ErrorContext(ctx, "extcapability: an extension object call could not be recorded", "err", err, "unit", unit)
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
	after := Usage{Objects: held.Objects + change.Usage.Objects, Bytes: held.Bytes + change.Usage.Bytes}
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
