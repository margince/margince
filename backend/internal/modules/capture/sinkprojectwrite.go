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
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// linkActivityToProject files the activity under the project the ladder chose.
// It uses the activities write of the automatic deal and lead filings.
// There the link, its audit row, the version bump and the relink event commit
// together. A replay that finds the link already present writes nothing.
//
// It requires activity.update, because filing changes who can reach the
// activity. Denial is not a fault: such a principal's mail lands unfiled.
//
// The target check stays here. A connector must not plant a link to a project
// its granting human could not see.
//
// A zero project means the activity is already filed. Only the stamp is
// repaired, because a link made before the stamp existed leaves a Handelsbrief
// an erasure destroys.
func linkActivityToProject(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, filing projectFiling, attribution ProjectAttribution) error {
	projectID := filing.project
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return nil
		}
		return err
	}
	if projectID.IsZero() {
		return stampTheProjectOnTheRow(ctx, tx, activityID, attribution.Stamp)
	}
	// The project row is locked in the shared order before the target check
	// reads it, so the link trigger never upgrades a weaker lock.
	if err := storekit.LockLastActivityTargets(ctx, tx, storekit.LastActivityTargets{Projects: []ids.UUID{projectID}}); err != nil {
		return fmt.Errorf("capture: %w", err)
	}
	if err := auth.EnsureLinkTarget(ctx, tx, string(datasource.EntityProject), projectID); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("capture: project link target: %w", err)
	}
	landed, err := attribution.File(ctx, tx, activityID, projectID, map[string]any{provenance.ProjectRungKey: filing.rung})
	if err != nil {
		return fmt.Errorf("capture: filing the activity under its project: %w", err)
	}
	if landed {
		return nil
	}
	return stampTheProjectOnTheRow(ctx, tx, activityID, attribution.Stamp)
}

// projectFiling is the project the ladder chose and the rung that chose it.
// A zero project proposes nothing.
type projectFiling struct {
	project ids.UUID
	rung    string
}

// stampTheProjectOnTheRow stamps whatever project the activity is actually
// filed under, for a call that wrote no link.
//
// Two callers reach it and both are repairs. One is a zero proposal for an
// activity already filed. The other is a filing write that landed nothing
// because a link was there. Nothing enforces that whoever filed a link also
// stamped it, which is why the stamp migration carries a backfill. A missed stamp leaves a
// Handelsbrief an erasure destroys, and a repeated one is a no-op.
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
