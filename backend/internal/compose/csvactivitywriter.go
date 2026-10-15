// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The activity object's own write path.
//
// An activity row brings history: it happened on a day in the past, with
// contacts the file names. The activities store owns how such a row is filed. This file
// decides what a row asks of it, and answers each refusal as a skipped row with
// its reason, never as a failed run. The dry run and the commit ask the store
// through the same functions, so the report says what the commit does.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// createActivity lands one row as a new activity, filed under the records its
// links name. The idempotency key lives in the identity map, which `land`
// writes in the same transaction.
func (w *csvWriters) createActivity(ctx context.Context, row migration.Row, typed map[string]any) (migration.EnsureResult, error) {
	in, why := w.activityBirthOf(row, typed)
	if why.Code != "" {
		return skippedFor(why), nil
	}
	err := w.land(ctx, row, func(tx pgx.Tx) (ids.UUID, error) {
		// The seat the owner names wins over the seat an author address resolves
		// to, and an author cell that names no seat stands as written.
		owner := in.Author.AuthorID
		if err := w.authors.resolve(ctx, tx, &in.Author); err != nil {
			return ids.UUID{}, err
		}
		if owner != nil {
			in.Author.AuthorID = owner
		}
		activity, _, err := w.activities.LogActivityTx(ctx, tx, in)
		if err != nil {
			return ids.UUID{}, fmt.Errorf("import: creating activity %s: %w", row.ExternalID, err)
		}
		return ids.UUID(activity.Id), nil
	})
	if why, refused := activityRefusalOf(err, textFields(row.Fields)); refused {
		return skippedFor(why), nil
	}
	if err != nil {
		return migration.EnsureResult{}, err
	}
	return migration.EnsureResult{Created: true}, nil
}

// activityBirthOf is what a row asks the activities store to file, and the
// reason it cannot be filed at all. The commit and the dry run share it.
func (w *csvWriters) activityBirthOf(row migration.Row, typed map[string]any) (activities.LogActivityInput, migration.Reason) {
	in := activityCreateFrom(textFields(row.Fields), typed, w.sourceSystem, w.provenanceOf(row.ExternalID))
	switch {
	case in.Kind == "":
		return in, migration.Why(migration.ReasonActivityTypeUnnamed)
	case in.OccurredAt == nil:
		return in, migration.Why(migration.ReasonOccurredAtUnnamed)
	}
	return in, migration.Reason{}
}

// predictActivityCreate asks the activities store, without writing, the
// refusals a filing meets outside the row's cells: a record the importer may
// read but not attach to. The commit's own function answers the refusal.
func (w *csvWriters) predictActivityCreate(ctx context.Context, row migration.Row, typed map[string]any) (predictedOutcome, migration.Reason, error) {
	in, why := w.activityBirthOf(row, typed)
	if why.Code != "" {
		return predictUnwritable, why, nil
	}
	err := w.activities.ProbeLinkWrites(ctx, in.Links)
	if why, refused := activityRefusalOf(err, textFields(row.Fields)); refused {
		return predictUnwritable, why, nil
	}
	return predictCreate, migration.Reason{}, err
}

// activityRefusalOf answers the store's refusals of one activity row as the
// reason the report shows. Anything it does not know is not a bad row and
// stops the run.
func activityRefusalOf(err error, fields map[string]string) (migration.Reason, bool) {
	if err == nil {
		return migration.Reason{}, false
	}
	if why := linkTypeRefusal(err); why != nil {
		return *why, true
	}
	if _, why, known := activityKindReason(err, fields); known {
		return why, true
	}
	switch {
	case errors.Is(err, apperrors.ErrRetentionHold):
		return migration.Why(migration.ReasonActivityHeld), true
	case errors.Is(err, apperrors.ErrPermissionDenied), errors.Is(err, apperrors.ErrNotFound):
		return migration.Why(migration.ReasonLinkNotAttachable), true
	}
	return migration.Reason{}, false
}
