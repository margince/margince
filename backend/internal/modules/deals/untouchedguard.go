// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// An archive that may only land while nobody has touched the deal since a given
// instant, re-asked inside the transaction that writes. The import undo reads
// which deals a colleague edited, then archives the rest. A colleague could act
// between those two transactions, so the write re-asks under its own row lock.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ArchiveOption attaches a precondition to an archive.
type ArchiveOption func(*archiveOptions)

type archiveOptions struct {
	untouchedSince *time.Time
}

// NotTouchedByHumanSince refuses the archive when a colleague has acted on the
// deal since `at`.
func NotTouchedByHumanSince(at time.Time) ArchiveOption {
	return func(o *archiveOptions) { o.untouchedSince = &at }
}

func collectArchiveOptions(opts []ArchiveOption) archiveOptions {
	var out archiveOptions
	for _, apply := range opts {
		apply(&out)
	}
	return out
}

// HumanTouchedError refuses an archive whose caller asked for it only while
// the deal was untouched. It is a conflict, so an undo records the deal as one
// it left alone rather than stopping.
type HumanTouchedError struct{ DealID ids.UUID }

func (e *HumanTouchedError) Error() string {
	return fmt.Sprintf("a colleague has changed deal %s since it was imported, so it was left alone", e.DealID)
}

// Is makes the refusal a conflict for errors.Is.
func (e *HumanTouchedError) Is(target error) bool { return target == apperrors.ErrConflict }

// refuseIfTouched asks the question inside the caller's transaction, after the
// row's write check has taken the lock.
func refuseIfTouched(ctx context.Context, tx pgx.Tx, id ids.DealID, o archiveOptions) error {
	if o.untouchedSince == nil {
		return nil
	}
	touched, err := storekit.HumanActedSince(ctx, tx, "deal", id.UUID, *o.untouchedSince)
	if err != nil {
		return err
	}
	if touched {
		return &HumanTouchedError{DealID: id.UUID}
	}
	return nil
}
