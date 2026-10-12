// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// dryRun validates every part of a staged run inside the test-run budget. A
// failure is recorded on the run before it is answered, so the run never stays
// in `validating`.
func (h importHandlers) dryRun(
	ctx context.Context, runs *migration.RunStore, id migration.RunID, source migration.Source,
	skipped func(object string) []migration.SkippedLine, mapping *migration.RunMapping,
) (migration.Report, error) {
	budget := cmp.Or(h.testRunBudget, importTestRunBudget)
	testCtx, release := context.WithTimeout(ctx, budget)
	defer release()
	report, err := validateParts(testCtx, h.db, id, source, skipped, mapping)
	if err == nil {
		return report, nil
	}
	failValidation(ctx, runs, id, err)
	if testCtx.Err() != nil {
		return migration.Report{}, httperr.Validation("file", "test_run_too_long", fmt.Sprintf(
			"the test run did not finish within %d seconds; split the file into smaller files and import each one",
			int(budget.Seconds())))
	}
	return migration.Report{}, importProblem(err)
}

// validateParts is the dry run proper. The writers read every row once for
// owners and tags, so a file that breaks past its header fails here first.
func validateParts(
	ctx context.Context, db *database.DB, id migration.RunID, source migration.Source,
	skipped func(object string) []migration.SkippedLine, mapping *migration.RunMapping,
) (migration.Report, error) {
	writers, err := newBundleWriters(ctx, db, id, mapping, source)
	if err != nil {
		return migration.Report{}, err
	}
	report, err := migration.NewEngine(migration.NewRunStore(db), writers).DryRun(ctx, source)
	if err != nil {
		return migration.Report{}, err
	}
	report, err = predictEveryPart(ctx, source, skipped, writers, *mapping, report)
	if err != nil {
		return migration.Report{}, err
	}
	return withGrantProblems(ctx, report, mapping.PartList(), writers.grantProblems), nil
}

// failValidation records a dry run that could not finish, so the run it was
// opened for does not sit in `validating`. The dry run may have stopped because
// the request's deadline passed, so the record cannot borrow that context.
func failValidation(ctx context.Context, runs *migration.RunStore, id migration.RunID, cause error) {
	slog.ErrorContext(ctx, "import validation stopped", "run", id, "err", cause)
	ctx, release := context.WithTimeout(context.WithoutCancel(ctx), importRecordStopTimeout)
	defer release()
	if err := runs.FailValidation(ctx, id); err != nil {
		slog.ErrorContext(ctx, "recording a failed import validation", "run", id, "err", err)
	}
}
