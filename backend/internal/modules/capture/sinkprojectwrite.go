// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The WRITE half of project attribution: filing the activity under the project
// the ladder chose, and stamping it as correspondence that project owns.
//
// Split from sinkproject.go, which decides WHICH project. The two answer
// different questions and fail differently — a ladder that picks nothing leaves
// a message unfiled, while a write that skips its stamp leaves a business
// letter an erasure destroys — and the file had grown past the length cap
// holding both.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// linkActivityToProject writes the one link the ladder concluded.
//
// It requires activity.update, not only the activity.create the capture
// checked. Filing an activity under a project changes who can reach it. It
// also bumps activity.version, the pin a staged approval re-checks. So it is
// an update, and recordLinkChange audits it as one. A principal that may
// create captured mail but not change it attributes nothing.
//
// Denial is not a fault: a connector role without activity.update is an
// ordinary configuration, so its mail lands filed under nothing.
//
// Then the row-scope check on the target, exactly as every other link writer
// does: a connector must not plant a link to a row its granting human could not
// see. That check is narrower than the ladder's own, which already refused an
// unreadable project — it stays because this function is the write, and a write
// re-checks its own target rather than trusting the caller to have done it.
//
// ON CONFLICT DO NOTHING because uq_activity_link_project admits exactly one
// project link per activity: a concurrent pass that got there first is the
// system working, not a collision to report. Nothing is audited or bumped when
// nothing landed — a no-op writes no audit noise and moves no version.
func linkActivityToProject(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, projectID ids.UUID, stamp StampProjectCorrespondence) error {
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return nil
		}
		return err
	}
	// A zero project is the caller saying it has nothing to propose because the
	// activity is already filed. Everything below the insert still runs: the
	// STAMP may be owed even when the filing is not.
	inserted := false
	if !projectID.IsZero() {
		if err := auth.EnsureLinkTarget(ctx, tx, string(datasource.EntityProject), projectID); err != nil {
			if errors.Is(err, apperrors.ErrNotFound) {
				return nil
			}
			return fmt.Errorf("capture: project link target: %w", err)
		}
		tag, err := tx.Exec(ctx, `
			INSERT INTO activity_link (activity_id, entity_type, project_id)
			VALUES ($1, 'project', $2)
			ON CONFLICT DO NOTHING`, activityID, projectID)
		if err != nil {
			return fmt.Errorf("capture: filing the activity under its project: %w", err)
		}
		inserted = tag.RowsAffected() == 1
	}
	if !inserted {
		return stampTheProjectOnTheRow(ctx, tx, activityID, stamp)
	}
	// The link is what qualifies the correspondence, so the stamp commits with
	// it (D5).
	if err := stamp(ctx, tx, activityID, projectID); err != nil {
		return fmt.Errorf("capture: classifying the filed activity's correspondence: %w", err)
	}
	// Filing changes who the activity reaches, so it moves the version a
	// staged approval pinned, as the human relink path does
	// (activities/lifecycle.go).
	return recordLinkChange(ctx, tx, activityID, nil,
		map[string]any{"entity_type": string(datasource.EntityProject), "entity_id": projectID}, nil)
}

// stampTheProjectOnTheRow stamps whatever project the activity is ACTUALLY
// filed under, for a call that inserted no link.
//
// Two callers reach it and both are repairs. A conflicting insert means a link
// was already there, and the early exit used to skip the stamp on the reasoning
// that whoever filed the link also stamped it — exactly the guarantee nothing
// enforces, which is why the stamp migration carries a backfill. A missed stamp
// leaves a Handelsbrief an erasure destroys; a repeated one costs a no-op,
// because the stamp is idempotent. The echo path made the same call for the
// same reason (activities/messageidentity.go).
//
// It re-reads rather than trusting the project the caller proposed. The two can
// differ: a human relink landing between the ladder's read and the insert wins
// the row and the ON CONFLICT discards our choice — and stamping the discarded
// one would write retention evidence for a project that does not own the
// activity, which is worse than the missed stamp above. At most one project
// link exists per activity (uq_activity_link_project), so there is exactly one
// answer to read.
//
// The race is what produces that divergence. The sink's own guard reads the
// filing before it proposes anything, so on the sequential path an already-filed
// activity proposes a zero project and this branch simply repairs the stamp.
// The re-read is here for the concurrent path instead, and it sits in
// linkActivityToProject — where the insert that can lose the row already is —
// rather than at either call site.
//
// Nothing else runs: no version bump and no audit row, because nothing changed
// about where the activity is filed.
func stampTheProjectOnTheRow(
	ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, stamp StampProjectCorrespondence,
) error {
	var linked *ids.UUID
	err := tx.QueryRow(ctx, `
		SELECT project_id FROM activity_link
		 WHERE activity_id = $1 AND entity_type = 'project'`, activityID).Scan(&linked)
	if errors.Is(err, pgx.ErrNoRows) {
		// The conflict was not a project link, or nothing is filed — nothing to
		// stamp, and nothing to guess at.
		return nil
	}
	if err != nil {
		return fmt.Errorf("capture: reading the project the activity is filed under: %w", err)
	}
	if linked == nil {
		return nil
	}
	// Bounded like any other project this file touches. A project the capture
	// principal may not reach is one it does not file under either, which is the
	// rule the insert arm already applies.
	if err := auth.EnsureLinkTarget(ctx, tx, string(datasource.EntityProject), *linked); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("capture: project link target: %w", err)
	}
	return stamp(ctx, tx, activityID, *linked)
}
