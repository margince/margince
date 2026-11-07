// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// An archive that may only land while nobody has touched the activity since a
// given instant, re-asked inside the transaction that writes. The import undo
// reads which activities a colleague edited, then archives the rest. A
// colleague could act between those two transactions, so the write re-asks
// under its own row lock.

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
	pristineTask   bool
	reason         string
	ruleVersion    string
}

// NotTouchedByHumanSince refuses the archive when a colleague has acted on the
// activity since `at`.
func NotTouchedByHumanSince(at time.Time) ArchiveOption {
	return func(o *archiveOptions) { o.untouchedSince = &at }
}

// OnlyPristineAutomaticTask protects work whose adoption history is incomplete.
func OnlyPristineAutomaticTask() ArchiveOption {
	return func(o *archiveOptions) { o.pristineTask = true }
}

// WithArchiveReason keeps a reviewed correction's reason beside the ordinary archive audit.
func WithArchiveReason(reason, ruleVersion string) ArchiveOption {
	return func(o *archiveOptions) { o.reason, o.ruleVersion = reason, ruleVersion }
}

func collectArchiveOptions(opts []ArchiveOption) archiveOptions {
	var out archiveOptions
	for _, apply := range opts {
		apply(&out)
	}
	return out
}

// HumanTouchedError refuses an archive whose caller asked for it only while
// the activity was untouched. It is a conflict, so an undo records the
// activity as one it left alone rather than stopping.
type HumanTouchedError struct{ ActivityID ids.UUID }

func (e *HumanTouchedError) Error() string {
	return fmt.Sprintf("a colleague has changed activity %s since it was imported, so it was left alone", e.ActivityID)
}

// Is makes the refusal a conflict for errors.Is.
func (e *HumanTouchedError) Is(target error) bool { return target == apperrors.ErrConflict }

// refuseIfTouched asks the question inside the caller's transaction, after the
// row's write check has taken the lock.
func refuseIfTouched(ctx context.Context, tx pgx.Tx, id ids.ActivityID, o archiveOptions) error {
	if o.pristineTask {
		if err := lockTaskAdoption(ctx, tx, id.String()); err != nil {
			return err
		}
		var pristine bool
		args := []any{id}
		query := storekit.SQLf("SELECT %s FROM activity a WHERE a.id=$%d", pristineRepairTaskSQL("a"), len(args))
		if err := tx.QueryRow(ctx, query, args...).Scan(&pristine); err != nil {
			return err
		}
		if !pristine {
			return fmt.Errorf("the task can no longer be proven untouched: %w", apperrors.ErrConflict)
		}
	}
	if o.untouchedSince == nil {
		return nil
	}
	touched, err := storekit.HumanActedSince(ctx, tx, "activity", id.UUID, *o.untouchedSince)
	if err != nil {
		return err
	}
	if touched {
		return &HumanTouchedError{ActivityID: id.UUID}
	}
	return nil
}

// A pin does not update the task row, so its writer shares the pristine decision's lock.
func lockTaskAdoption(ctx context.Context, tx pgx.Tx, rowID string) error {
	return storekit.LockWriteIdentity(ctx, tx, "task_adoption", rowID)
}
