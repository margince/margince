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

// importTestRunRowCap is the most rows one test run reads. A mixed company,
// contact and lead file was read at about 1,000 rows a second on a developer
// machine, so the budget reaches near 22,000 rows. The cap keeps a third of the
// budget for an estate each row is matched against.
const importTestRunRowCap = 15_000

// refuseOversizedTestRun refuses a file the test run cannot finish inside its
// budget, before the test run reads any row against the estate.
func refuseOversizedTestRun(ctx context.Context, source migration.Source) error {
	counts, err := source.Counts(ctx)
	if err != nil {
		return importProblem(err)
	}
	rows := 0
	for _, n := range counts {
		rows += n
	}
	if rows <= importTestRunRowCap {
		return nil
	}
	return httperr.Validation("file", "too_many_rows_for_test_run", fmt.Sprintf(
		"this upload has %d rows; a test run takes at most %d, so split it into smaller files and import each one",
		rows, importTestRunRowCap))
}

// dryRun validates every part of a staged run inside the test-run budget. A
// failure is recorded on the run before it is answered, so the run never stays
// in `validating`.
func (h importHandlers) dryRun(
	ctx context.Context, runs *migration.RunStore, id migration.RunID, source migration.Source,
	skipped func(object string) []migration.SkippedLine, mapping *migration.RunMapping,
) (migration.Report, error) {
	if err := refuseOversizedTestRun(ctx, source); err != nil {
		failValidation(ctx, runs, id, err)
		return migration.Report{}, err
	}
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
		report, err = migration.NewEngine(migration.NewRunStore(db), writers).DryRun(ctx, source)
		if err != nil {
			return err
		}
		report, err = predictEveryPart(ctx, source, skipped, writers, *mapping, report)
		if err != nil {
			return err
		}
		report = withGrantProblems(ctx, report, mapping.PartList(), writers.grantProblems)
		return nil
	})
	return report, err
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
