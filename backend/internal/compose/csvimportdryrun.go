// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// validateParts is the dry run proper. The writers read every row once for
// owners and tags, so a file that breaks past its header fails here first.
// progress hears the two passes that match rows against the estate.
func validateParts(
	ctx context.Context, db *database.DB, id migration.RunID, source migration.Source,
	skipped func(object string) []migration.SkippedLine, mapping *migration.RunMapping, progress *testRunProgress,
) (report migration.Report, err error) {
	err = database.WithWorkspaceSnapshot(ctx, db.Pool(), func(ctx context.Context) error {
		writers, err := newBundleWriters(ctx, db, id, mapping, source)
		if err != nil {
			return err
		}
		for _, part := range writers.parts {
			if err := part.preloadBindings(ctx); err != nil {
				return err
			}
		}
		if err := progress.enter(ctx, migration.PhaseReading); err != nil {
			return err
		}
		report, err = migration.NewEngine(migration.NewRunStore(db), writers).DryRun(ctx, source)
		if err != nil {
			return err
		}
		if err := progress.enter(ctx, migration.PhaseChecking); err != nil {
			return err
		}
		report, err = predictEveryPart(ctx, source, skipped, writers, *mapping, report)
		if err != nil {
			return err
		}
		report = withGrantProblems(ctx, report, mapping.PartList(), writers.grantProblems)
		if report.ReviewPlan != nil && report.ReviewPlan.Complete {
			actor, ok := principal.Actor(ctx)
			if !ok {
				return apperrors.ErrPermissionDenied
			}
			report.ReviewPlan.ActorID = actor.ID
			settleReviewLinks(report.ReviewPlan)
			report.ReviewPlan.Associations, err = source.Associations(ctx)
			if err != nil {
				return err
			}
			if len(report.ReviewPlan.Associations) > migration.ReviewRowLimit {
				report.ReviewPlan.Complete = false
				report.ReviewPlan.Rows, report.ReviewPlan.Associations = nil, nil
			}
		}
		return nil
	})
	return report, err
}
