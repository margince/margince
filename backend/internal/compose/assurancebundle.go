// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Turning the night's findings into work somebody can pick up.
//
// The scan raises exceptions and three surfaces render them. What nobody had
// was ONE thing to act on: a deal with four problems produced four findings,
// and a rep clearing them cleared the same deal four times. This is the seam
// that groups them — one task per deal per pass — and it lives in compose
// because it spans two modules that may not import each other: assurance owns
// the cycle and the bundling, activities owns the task.
//
// THE CYCLE IS THE RUN. Its scope is the scan's own run id, which makes the
// pass idempotent for free: River retries this job, and a retry that re-scanned
// would produce a new run and therefore a new cycle, while a retry landing on
// the same run re-finds the cycle it already opened. Neither mints a second task
// for one deal. The alternative — a long-lived cycle somebody closes by hand —
// needs a surface to close it, and until that surface exists an unclosed cycle
// would silently stop the next night's bundling.
//
// A TASK ALREADY DONE IS LEFT DONE. OpenTaskFor answers with the cycle's task
// for a subject whether or not somebody has completed it, and a finding arriving
// afterwards files under that same task rather than minting a fresh one. If a
// rep has answered it and the condition is still there, the next cycle asks
// again — which is the honest re-ask, because the finding was re-observed after
// somebody acted on it.
//
// A TASK STILL OPEN IS NOT ASKED TWICE. The cycle is one night, so cycle scope
// alone left one indistinguishable task per night behind: same subject line, no
// body, nothing to choose between them. A subject whose earlier task is still
// open adopts it (adoptOpenTask), and the extras an installation already
// accumulated are archived as the sweep meets them.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// bundleDeps are the two doors this seam needs, named as functions so the
// worker can hand in the real stores and a test can hand in the same stores
// built over its own pool.
type bundleDeps struct {
	bundles    *assurance.Store
	activities *activities.Store
	// exceptions reads the findings THIS RUN observed and nobody has deferred.
	// It is bundleableFindings in production.
	exceptions func(ctx context.Context, tx pgx.Tx, runID ids.UUID) ([]assurance.Exception, error)
}

// bundleRunFindings groups one run's open findings into one task per subject.
//
// It reports how many tasks it minted, which is what the sweep logs: a pass that
// found work and a pass that found none are different facts, and a count of zero
// beside a non-zero finding count is the shape that says this seam stopped
// working.
//
// It never returns an error for a finding it could not bundle. The scan has
// already committed — the exceptions are recorded and the surfaces render them —
// so failing the job here would leave River retrying a pass whose real output
// already landed, and the retry would re-scan and re-mint. What a failure costs
// instead is one deal's task, which the next night's cycle mints again.
func bundleRunFindings(
	ctx context.Context, deps bundleDeps, runID ids.UUID,
) (minted int, err error) {
	cycle, err := deps.bundles.OpenCycle(ctx, cycleScopeForRun(runID))
	if err != nil {
		return 0, fmt.Errorf("compose: opening the cycle for run %s: %w", runID, err)
	}

	// The close is registered THE MOMENT the cycle exists, before the read that
	// could fail below it.
	//
	// An open cycle is what the partial unique index refuses a second of, and
	// the scope is a run id nobody revisits — so a cycle left open is left open
	// for ever, and one bad night turns into a feature that never runs again.
	// Two earlier versions of this function each left one path out: the first
	// returned on a subject's error and skipped the close, the second installed
	// the defer only after the exception read, so a read failure walked past it.
	// Registering here is what makes every return below it pass through the
	// close, which is the property, not the placement.
	defer func() {
		if closeErr := deps.bundles.CloseCycle(ctx, cycle); closeErr != nil {
			err = errors.Join(err, fmt.Errorf(
				"compose: closing the cycle for run %s: %w", runID, closeErr))
		}
	}()

	var open []assurance.Exception
	if readErr := deps.bundles.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var scanErr error
		open, scanErr = deps.exceptions(ctx, tx, runID)
		return scanErr
	}); readErr != nil {
		return 0, fmt.Errorf("compose: reading the findings run %s observed: %w", runID, readErr)
	}

	// Grouped before anything is minted, because the task's subject line names
	// how many findings it stands for — and that count is only known once the
	// whole list has been walked. Minting on first sight would title every task
	// "1 finding" and then bundle three more onto it.
	//
	// ONE SUBJECT'S FAILURE IS NOT THE NIGHT'S. The list is ordered by severity
	// and then by money at stake, so aborting on the first error would suppress
	// every task below it — a single deal whose owner has left the company would
	// silently cost the whole workspace its remaining tasks. Each failure is
	// carried and the walk continues.
	var failed []error
	for _, subject := range groupBySubject(open) {
		wasMinted, subjectErr := bundleSubject(ctx, deps, runID, cycle, subject)
		if subjectErr != nil {
			failed = append(failed, subjectErr)
			continue
		}
		if wasMinted {
			minted++
		}
	}
	if len(failed) > 0 {
		return minted, errors.Join(failed...)
	}
	return minted, nil
}

// bundleableFindings reads the open findings THIS RUN observed, minus the ones
// somebody has deferred.
//
// Two narrowings the surface read (AssuranceExceptions) does not make, and both
// are the difference between a task somebody should act on and one that will
// annoy them:
//
//   - JOINED TO assurance_run_finding. Scan returns early when a source cannot
//     be read, finishing the run INCOMPLETE with nothing observed — and the
//     bundler still runs. Reading every open exception would then mint a full
//     night of tasks for conditions nothing verified, including deals somebody
//     fixed that morning. A run's own membership is what says "still true as of
//     tonight", and RecordRunFindings writes it for exactly this.
//   - DEFERRED FINDINGS EXCLUDED. Resolve with remind_later deliberately leaves
//     the exception open, and CloseCleared carves the same rows out of its own
//     sweep. Without the matching clause a rep who deferred a finding for two
//     weeks is handed a fresh task about it every night for fourteen nights,
//     which makes the deferral a no-op against this feature. The outcome is
//     bound from assurance.OutcomeRemindLater rather than written in: the
//     sibling clause in CloseCleared still said 'deferred' — the vocabulary the
//     table shipped with, renamed by migration 1788416000 — and so had never
//     matched a row.
//
// It reads assurance_exception directly rather than going through the deal join
// AssuranceExceptions uses. The pass runs as PrincipalSystem over the whole
// pipeline — that is what the scan is unscoped for — so a scope clause here
// would narrow nothing while implying it narrows something. What keeps a finding
// from a reader who may not see its deal is the surface read, which is
// unchanged.
func bundleableFindings(
	ctx context.Context, tx pgx.Tx, runID ids.UUID,
) ([]assurance.Exception, error) {
	rows, err := tx.Query(ctx, `
		SELECT `+exceptionColumns+`
		  FROM assurance_exception e
		  JOIN assurance_run_finding f ON f.exception_id = e.id AND f.run_id = $1
		 WHERE e.status = 'open'
		   AND e.subject_kind = 'deal'
		   AND NOT EXISTS (SELECT 1 FROM assurance_resolution r
		                    WHERE r.exception_id = e.id
		                      AND r.outcome = $2
		                      AND r.remind_at > now())
		 ORDER BY
		   CASE e.severity WHEN 'high' THEN 0 WHEN 'medium' THEN 1 ELSE 2 END,
		   e.affected_minor DESC NULLS LAST,
		   e.last_seen_at DESC`, runID, assurance.OutcomeRemindLater)
	if err != nil {
		return nil, fmt.Errorf("compose: reading the findings to bundle: %w", err)
	}
	defer rows.Close()

	// scanException, shared with AssuranceExceptions: one select list, one
	// scanner. Two copies of a positional scan drift into reading a currency
	// into a status, and nothing fails loudly when they do.
	return pgx.CollectRows(rows, scanException)
}

// cycleScopeForRun names the cycle a run's findings bundle into.
//
// The run id, prefixed so the scope says what it is when an operator reads the
// row. OpenCycle enforces one open cycle per scope, so this is also what makes
// the pass safe to retry: the same run resolves to the same scope, and a second
// attempt finds the cycle already open rather than minting a parallel one.
func cycleScopeForRun(runID ids.UUID) string {
	return "assurance_run:" + runID.String()
}

// subjectFindings is one subject and every open finding about it, in the order
// the scoped read returned them — severity first, then money at stake.
type subjectFindings struct {
	kind     string
	id       ids.UUID
	findings []assurance.Exception
}

// groupBySubject collects the findings per subject, KEEPING the read's order.
//
// A map alone would not: ranging one is randomised, so the task minted for a
// subject would take a different finding's owner from night to night, and the
// order findings bundle in would vary for no reason a reader could see. The
// order matters because the first finding is the one whose owner becomes the
// assignee, and the read already sorted by severity and amount — so the owner
// carried is the one on the most material finding.
func groupBySubject(open []assurance.Exception) []subjectFindings {
	var out []subjectFindings
	at := map[string]int{}
	for _, exception := range open {
		key := exception.SubjectKind + ":" + exception.SubjectID.String()
		index, seen := at[key]
		if !seen {
			at[key] = len(out)
			out = append(out, subjectFindings{kind: exception.SubjectKind, id: exception.SubjectID})
			index = len(out) - 1
		}
		out[index].findings = append(out[index].findings, exception)
	}
	return out
}

// bundleSubject files one subject's findings under one task, minting it if the
// cycle has none yet, and says whether it minted.
func bundleSubject(
	ctx context.Context, deps bundleDeps, runID, cycle ids.UUID, subject subjectFindings,
) (bool, error) {
	task, found, err := deps.bundles.OpenTaskFor(ctx, cycle, subject.kind, subject.id)
	if err != nil {
		return false, fmt.Errorf("compose: asking for the subject's task: %w", err)
	}
	if !found {
		// Nothing in THIS cycle, so ask across the earlier ones. A task still
		// open is the same question already on the rep's list, and asking it
		// again as a second row does not make it more likely to be answered.
		task, found, err = adoptOpenTask(ctx, deps, subject)
		if err != nil {
			return false, err
		}
	}

	minted := false
	if !found {
		task, err = mintBundleTask(ctx, deps, runID, subject)
		if err != nil {
			return false, err
		}
		minted = true
	}

	// The findings are filed on ONE transaction that holds the task, because
	// whether it is still open was read before this line and a rep can answer
	// it at any moment. Without the hold, a task completed in between takes
	// tonight's findings onto a row nobody will open again — they surface on
	// the next cycle, since the exception stays `open` and a done task is not
	// adopted, but a night is a night.
	//
	// Held the way the cycle already is one statement below: a share lock in
	// the read that admits the write, so a completion waits rather than
	// slipping between them. The lock is taken HERE and not in assurance,
	// which owns no `activity` row and reads none — its comment on
	// TaskIDsForSubject says so, and this keeps that true.
	// The check runs on the MINTED path too. mintBundleTask commits in a
	// transaction of its own, so by the time this one opens the task is on the
	// rep's list and they can answer it — "nobody has seen it yet" stops being
	// true the moment the mint commits.
	if err := deps.bundles.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		open, err := taskStillOpenTx(ctx, tx, task)
		if err != nil {
			return err
		}
		if !open {
			// The rep answered it between the read and this transaction. Their
			// answer stands and the findings wait for the next cycle, which
			// mints afresh — the same judgement adoptOpenTask makes about a
			// task that was already done when it looked.
			return nil
		}
		return bundleUnder(ctx, deps, tx, cycle, task, subject)
	}); err != nil {
		return minted, err
	}
	return minted, nil
}

// bundleUnder files every one of a subject's findings under one task.
func bundleUnder(
	ctx context.Context, deps bundleDeps, tx pgx.Tx,
	cycle, task ids.UUID, subject subjectFindings,
) error {
	for _, exception := range subject.findings {
		if _, err := deps.bundles.BundleExceptionTx(ctx, tx, assurance.BundleInput{
			CycleID:        cycle,
			ExceptionID:    exception.ID,
			TaskActivityID: task,
		}); err != nil {
			return fmt.Errorf("compose: bundling the finding: %w", err)
		}
	}
	return nil
}
