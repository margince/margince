// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A corrected activities file landing on activities an earlier import made.
//
// The dry run and the commit share one plan, so the report says what the commit
// does. A re-import rewrites only the subject, body, timestamp, duration and
// meeting status. The type, direction, links and author are fixed when the
// activity lands.

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// activityPlan is what one row asks of an activity an import already made.
type activityPlan struct {
	patch   activities.UpdateActivityInput
	changed int
}

// storedActivity is the part of the stored activity a plan decides on.
type storedActivity struct {
	Kind string `json:"kind"`
}

// planActivityUpdate reads the stored activity and decides what the row
// changes. A refusal comes back as its reason and the row is skipped whole.
func (w *csvWriters) planActivityUpdate(
	ctx context.Context, id ids.UUID, row migration.Row, typed map[string]any,
) (activityPlan, migration.Reason, error) {
	current, err := w.read(ctx, id)
	if err != nil {
		return activityPlan{}, migration.Reason{}, err
	}
	fields := textFields(row.Fields)
	changed, err := changedFields(current, fields, typed)
	if err != nil {
		return activityPlan{}, migration.Reason{}, err
	}
	for target := range changed {
		if isCreateOnly(w.object, target) {
			delete(changed, target)
		}
	}
	plan := activityPlan{patch: activityUpdateFrom(changed, typed), changed: len(changed)}
	var stored storedActivity
	if err := json.Unmarshal(current, &stored); err != nil {
		return plan, migration.Reason{}, fmt.Errorf("import: reading the stored activity: %w", err)
	}
	// The row's own type cell may be absent, so the kind the fields are held
	// against is the stored one: a patch cannot change it.
	err = activities.CheckKindFields(stored.Kind, plan.patch.DurationSeconds, plan.patch.MeetingStatus, "")
	if _, why, known := activityKindReason(err, fields); known {
		return plan, why, nil
	}
	return plan, migration.Reason{}, nil
}

// reconcileActivity brings an activity up to what the file now says.
func (w *csvWriters) reconcileActivity(
	ctx context.Context, id ids.UUID, row migration.Row, typed map[string]any,
) (migration.EnsureResult, error) {
	plan, refusal, err := w.planActivityUpdate(ctx, id, row, typed)
	if err != nil {
		return migration.EnsureResult{}, err
	}
	switch {
	case refusal.Code != "":
		return skippedFor(refusal), nil
	case plan.changed == 0:
		return migration.EnsureResult{Unchanged: true}, nil
	case !auth.Allows(ctx, w.object, principal.ActionUpdate):
		return skippedFor(cannotEditWhy(w.object)), nil
	}
	_, err = w.activities.UpdateActivity(ctx, ids.From[ids.ActivityKind](id), plan.patch)
	if why, refused := activityRefusalOf(err, textFields(row.Fields)); refused {
		return skippedFor(why), nil
	}
	if err != nil {
		return migration.EnsureResult{}, err
	}
	w.updated++
	return migration.EnsureResult{Disclosure: fmt.Sprintf("row %s: %d field(s) rewritten from the file", row.ExternalID, plan.changed)}, nil
}

// predictActivityUpdate is reconcilePrediction for an activity: the same plan,
// not written.
func (w *csvWriters) predictActivityUpdate(
	ctx context.Context, id ids.UUID, row migration.Row, typed map[string]any,
) (predictedOutcome, migration.Reason, error) {
	plan, refusal, err := w.planActivityUpdate(ctx, id, row, typed)
	switch {
	case err != nil:
		return predictCreate, migration.Reason{}, err
	case refusal.Code != "":
		return predictUnwritable, refusal, nil
	case plan.changed == 0:
		return predictUnchanged, migration.Reason{}, nil
	case !auth.Allows(ctx, w.object, principal.ActionUpdate):
		return predictUnwritable, cannotEditWhy(w.object), nil
	}
	return predictUpdate, migration.Reason{}, nil
}
