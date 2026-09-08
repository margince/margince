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
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// linkActivityToProject writes the one link the ladder concluded.
//
// It REQUIRES activity.update, and that is not belt-and-braces over the
// activity.create the capture already checked. Filing an activity under a
// project changes who can reach it and bumps activity.version, which is the pin
// a staged approval re-checks before it redeems — so this is an update of the
// activity by every test that matters, and the audit row says so
// (auditProjectAttribution). A principal that may create captured mail but not
// change it attributes nothing, which is the honest outcome rather than a
// silent widening of what create means.
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
	// Touch the activity ROW, not just its link table, for the reason the
	// human relink path does it (activities/lifecycle.go): a staged approval
	// pins activity.version, and that pin is what stands between an approved
	// "send this on this conversation" and the conversation being repointed
	// before the approval redeems. Filing changes who the activity reaches, so
	// it must move the version the pin re-checks. The trigger
	// (set_updated_at_bump_version) does the bump; this only has to be a
	// genuine UPDATE of the row.
	if _, err := tx.Exec(ctx, `UPDATE activity SET updated_at = now() WHERE id = $1`, activityID); err != nil {
		return fmt.Errorf("capture: bumping the filed activity's version: %w", err)
	}
	// The link is what qualifies the correspondence, so the stamp commits with
	// it (D5).
	if err := stamp(ctx, tx, activityID, projectID); err != nil {
		return fmt.Errorf("capture: classifying the filed activity's correspondence: %w", err)
	}
	return auditProjectAttribution(ctx, tx, activityID, projectID)
}

// auditProjectAttribution records the link the ladder just wrote, under the
// same action the human-driven relink uses: a reader asking "how did this
// message end up on this project?" must find one answer whether a person or the
// ladder filed it, and the audit row's principal already says which.
//
// activity_relink maps to activity.update in auditActionGrant, and the caller
// really does require that grant (linkActivityToProject) — so the
// authorization_rule this row renders names the rule that actually admitted the
// write. audit_log is append-only, so a verb whose write path never checked the
// grant it claims would be an uncorrectable lie about who was allowed to do
// what.
//
// No public event rides with it, and that is deliberate rather than an
// omission. This link is part of landing ONE captured message, which
// activity.captured already announced in the transaction just before; a second
// event saying the same message changed would have subscribers reacting twice
// to one arrival. activity.updated is the activities module's type to mean
// what it means, and capture does not get to redefine it as "a message
// arrived, again".
func auditProjectAttribution(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, projectID ids.UUID) error {
	_, err := storekit.Audit(ctx, tx, "activity_relink", "activity", activityID.UUID, nil,
		map[string]any{"entity_type": string(datasource.EntityProject), "entity_id": projectID})
	return err
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
