// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// RecoverPages commits successful recoveries even when another row cannot resume.
// The reader locks at most 100 rows ordered by ID after the supplied cursor.
// Savepoints contain row failures; advancing the cursor prevents a broken row
// from starving later work. The joined error keeps failures visible to job health.
func RecoverPages[T any](ctx context.Context, transaction func(context.Context, func(pgx.Tx) error) error,
	read func(pgx.Tx, ids.UUID) ([]T, error), id func(T) ids.UUID, resume func(pgx.Tx, T) error,
) error {
	var after ids.UUID
	var failures []error
	for {
		count := 0
		err := transaction(ctx, func(tx pgx.Tx) error {
			rows, err := read(tx, after)
			if err != nil {
				return err
			}
			count = len(rows)
			for _, row := range rows {
				next := id(row)
				if bytes.Compare(next[:], after[:]) <= 0 {
					return fmt.Errorf("recovery page did not advance its cursor")
				}
				if err := pgx.BeginFunc(ctx, tx, func(savepoint pgx.Tx) error { return resume(savepoint, row) }); err != nil {
					if errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrNotFound) {
						// Eligibility can change again: keep this carrier parked, without
						// turning ordinary access revocation into a failed sweep every minute.
						slog.DebugContext(ctx, "recovery left an ineligible request parked", "record", next, "reason", err)
					} else {
						failures = append(failures, err)
					}
				}
				after = next
			}
			return nil
		})
		if err != nil {
			return errors.Join(append(failures, err)...)
		}
		if count < 100 {
			return errors.Join(failures...)
		}
	}
}
