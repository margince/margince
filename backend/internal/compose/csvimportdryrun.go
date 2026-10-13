// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// importTestRunRowCap is the most rows one upload may hold, because one test
// run reads them all. A mixed company, contact and lead file was read at about
// 1,000 rows a second on a developer machine, so the budget reaches near
// 22,000 rows. The cap keeps a third of the budget for an estate each row is
// matched against.
const importTestRunRowCap = 15_000

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
	if testCtx.Err() != nil {
		err = httperr.Validation("file", "test_run_too_long", fmt.Sprintf(
			"the test run did not finish within %d seconds; split the file into smaller files and import each one",
			int(budget.Seconds())))
	} else {
		err = importProblem(err)
	}
	returnToDraft(ctx, runs, id, err)
	return migration.Report{}, err
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

// returnToDraft ends a test run that stopped, keeping why on the draft so the
// importer reads it beside the choices they can change. The test may have
// stopped on the request's deadline, so the record cannot borrow that context.
func returnToDraft(ctx context.Context, runs *migration.RunStore, id migration.RunID, cause error) {
	slog.ErrorContext(ctx, "import test run stopped", "run", id, "err", cause)
	ctx, release := context.WithTimeout(context.WithoutCancel(ctx), importRecordStopTimeout)
	defer release()
	if err := runs.ReturnToDraft(ctx, id, testStopCause(cause)); err != nil {
		slog.ErrorContext(ctx, "returning a stopped import test run to its draft", "run", id, "err", err)
	}
}

// testStopCause is what a draft says about its last test. A refusal the
// importer can act on keeps its answer. Anything else may hold a driver
// message, so it becomes a fixed sentence.
func testStopCause(err error) string {
	if fault, ok := httperr.Classify(err); ok && fault.Status < http.StatusInternalServerError {
		return fault.Detail
	}
	return migration.CauseTestRunStopped
}
