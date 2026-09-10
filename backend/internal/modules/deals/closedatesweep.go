// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The nightly close-date corrector (formulas-and-rules §11, DECISIONS
// A6, B-E09.20): the enforcement half of INV-CLOSE-PAST. Every open
// deal the §11 assessment flags is corrected the same night on the A6
// risk tier — 🟢 a low-stakes clear-overdue date is rolled forward
// finally (reversible: the audit row carries before/after), 🟡 a
// forecast-bearing / missing / unrealistic date is replaced with a
// PROVISIONAL guess (the invariant holds instantly, the deal stays out
// of Commit) and a close_date_correction approval asks a human for the
// real date, 🔻 a deal that has gone quiet is downgraded one forecast
// notch instead of being optimistically re-dated. Follows the retention
// evaluator's shape: one pass over every live workspace, one audited
// transaction per corrected deal.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// closeDateBatch bounds how many deals one workspace pass corrects — a
// first run against a migrated backlog drains over successive nights.
const closeDateBatch = 200

// CloseDateCorrector drives the sweep; the worker ticks it nightly.
type CloseDateCorrector struct {
	// db binds the workspace this store runs for (ADR-0091 §9 step 3).
	db       *database.DB
	policy   CorrectionPolicy
	reviewer QuietReviewReader
	log      *slog.Logger
	// now is the corrector's clock. WithClock overrides it, and a test that
	// asks what TOMORROW's pass does has no other way to get there: the
	// proposed date is computed from "today", so two passes in one process
	// otherwise always run on the same day.
	now func() time.Time
	// installation answers which zone this sweep computes its dates in. A
	// close date is a DATE, so the zone decides which day a deal is late on.
	installation Installation
}

// NewCloseDateCorrector assembles the sweep over the pool it reads through, the
// policy that says whose deals it may correct unasked, and the seam that
// answers which zone its dates are computed in.
func NewCloseDateCorrector(db *database.DB, policy CorrectionPolicy, reviewer QuietReviewReader,
	log *slog.Logger, inst Installation,
) *CloseDateCorrector {
	return &CloseDateCorrector{
		db: db, policy: policy, reviewer: reviewer, log: log,
		now: time.Now, installation: inst.orRefusing(),
	}
}

// WithClock overrides the "today" this sweep computes its dates against
// (tests only). Returns the corrector for chaining, like every other clock
// seam in this module.
//
// Without it no test can state what the sweep does on a SECOND night, because
// every pass in one process reads the same wall clock — and "the same question
// tomorrow" is the whole of what the rejection memory promises.
//
// It moves the ASSESSMENT's today and not the candidate query's, which reads the
// database clock. The two only have to agree to within the pre-filter's slack:
// that query is a deliberate superset — anything dated within the stalled window
// (60 days ahead), missing, or still provisional — so a clock ahead of the
// database's by less than that window changes which deals are FLAGGED and never
// which are looked at. A test crossing a day or two is well inside it.
func (c *CloseDateCorrector) WithClock(clock func() time.Time) *CloseDateCorrector {
	c.now = clock
	return c
}

// SweepWorkspace is one close-date hygiene pass over the workspace already
// bound in ctx. The fleet fan-out lives in the job layer: each workspace gets
// its own job row, so a failed pass is a failed row rather than a log line
// inside a run River recorded as completed.
func (c *CloseDateCorrector) SweepWorkspace(ctx context.Context) error {
	return c.sweepWorkspace(ctx)
}

// closeDateCandidate is one open deal the SQL pre-filter surfaced; the
// pure §11 assessment decides the truth in Go.
type closeDateCandidate struct {
	id             ids.DealID
	name           string
	createdAt      time.Time
	lastActivityAt *time.Time
	waitUntil      *time.Time
	expectedClose  *time.Time
	provisional    bool
	forecastCat    *string
	pipelineID     ids.PipelineID
	// ownerID is whose deal it is, for the policy that says whether the sweep
	// may correct it unasked. Nil where nobody owns it.
	ownerID        *ids.UUID
	winProbability int
	remainingOpen  int
}

// sweepWorkspace walks one pass's FROZEN membership, page by page, from
// wherever the last attempt durably reached.
//
// The old shape re-read the live deal table each night — the oldest 200 rows the
// pre-filter admitted — and because a corrected deal stays eligible, that was
// the same 200 rows every time. A workspace with more eligible deals never
// reached the rest of them, and nothing recorded that it had not.
//
// Now membership is decided once (startRun) and the pass drains it: each page
// asks for the members that are still unsettled, so settling one is what moves
// the walk forward. Two consequences the old shape could not offer — a pass
// interrupted by the worker's 5-minute deadline RESUMES rather than restarting
// at the first row, and a run can say how much of its own set it covered.
func (c *CloseDateCorrector) sweepWorkspace(ctx context.Context) error {
	now := c.now().UTC()
	var tzName string
	var run *CloseDateRun

	if err := c.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		if tzName, err = c.installation.Timezone(ctx, tx); err != nil {
			return fmt.Errorf("read the installation's timezone: %w", err)
		}
		loc, err := installationZone(tzName)
		if err != nil {
			return err
		}
		asOf := now.In(loc)
		asOf = time.Date(asOf.Year(), asOf.Month(), asOf.Day(), 0, 0, 0, 0, time.UTC)

		// A pass already open for this local day is resumed, not duplicated:
		// the retry after a worker deadline is the same pass continuing. No
		// such pass is the ordinary first run of the day, not a fault.
		run, err = c.openRun(ctx, tx)
		if errors.Is(err, apperrors.ErrNotFound) {
			run, err = c.startRun(ctx, tx, asOf, tzName)
		}
		return err
	}); err != nil {
		return err
	}

	loc, err := installationZone(tzName)
	if err != nil {
		return err
	}

	velocities := map[ids.PipelineID]float64{}
	for {
		var page []closeDateCandidate
		var members []closeDateMember
		if err := c.db.Tx(ctx, func(tx pgx.Tx) error {
			var err error
			page, members, err = c.nextMembers(ctx, tx, run.ID, closeDateBatch)
			return err
		}); err != nil {
			return err
		}
		if len(members) == 0 {
			break
		}
		live := make(map[ids.DealID]closeDateCandidate, len(page))
		for _, cand := range page {
			live[cand.id] = cand
		}
		for _, m := range members {
			outcome, settleErr := c.settleMember(ctx, live, m, velocities, now, loc, run.ID)
			if settleErr != nil {
				// One deal's failure is that deal's outcome, not the end of the
				// pass. Recording it and walking on is what keeps a single bad
				// row from starving every deal behind it — the same starvation
				// in a different disguise. The run ends incomplete, and the
				// receipt can name what it could not finish.
				c.log.WarnContext(ctx, "close-date sweep could not settle a deal",
					"deal_id", m.dealID, "run_id", run.ID, "error", settleErr)
				outcome = closeDateMemberFailed
			}
			if err := c.db.Tx(ctx, func(tx pgx.Tx) error {
				return c.settle(ctx, tx, run.ID, m, outcome)
			}); err != nil {
				return err
			}
		}
	}

	return c.db.Tx(ctx, func(tx pgx.Tx) error {
		return c.finishRun(ctx, tx, run.ID)
	})
}

// settleMember assesses one frozen member and reports the outcome its row
// should carry. A member whose deal has closed or been archived since the freeze
// is skipped — it stays in the ledger with a reason rather than vanishing from
// the denominator.
func (c *CloseDateCorrector) settleMember(
	ctx context.Context, live map[ids.DealID]closeDateCandidate, m closeDateMember,
	velocities map[ids.PipelineID]float64, now time.Time, loc *time.Location, runID ids.UUID,
) (string, error) {
	if m.gone {
		return closeDateMemberSkipped, nil
	}
	cand, ok := live[m.dealID]
	if !ok {
		return closeDateMemberSkipped, nil
	}
	velocity, known := velocities[cand.pipelineID]
	if !known {
		var err error
		if velocity, err = c.stageVelocityDays(ctx, cand.pipelineID); err != nil {
			return "", fmt.Errorf("stage velocity for pipeline %s: %w", cand.pipelineID, err)
		}
		velocities[cand.pipelineID] = velocity
	}
	category := effectiveForecastCategory(cand.forecastCat, cand.winProbability)
	hygiene := CloseDateAssessment(CloseDateInput{
		Status:              string(DealOpen),
		ExpectedClose:       cand.expectedClose,
		CreatedAt:           cand.createdAt,
		LastActivityAt:      cand.lastActivityAt,
		WaitUntil:           cand.waitUntil,
		StageWinProbability: cand.winProbability,
		RemainingOpenStages: cand.remainingOpen,
		InForecastCommit:    category == forecastCommit || category == forecastBestCase,
		StageVelocityDays:   velocity,
	}, now, loc)
	// The outcome is whatever correct actually DID — see closeDateOutcome. A
	// tier that decided to act and then wrote nothing (the switch off, or the
	// deal already holding what it proposed) settles as checked, so the run's
	// counters never claim a correction that did not reach a deal.
	outcome, err := c.correct(ctx, cand, hygiene, category, now, loc, runID)
	if err != nil {
		return "", fmt.Errorf("close-date correction on %s: %w", cand.id, err)
	}
	return outcome, nil
}

// stageVelocityDays is §11's experience-informed pace: the workspace
// median duration of completed stage stints across won deals of the
// pipeline. Below the CLOSE_DATE_MIN_HISTORY floor the observation is
// noise, so zero is returned and the fold falls back to the default.
func (c *CloseDateCorrector) stageVelocityDays(ctx context.Context, pipelineID ids.PipelineID) (float64, error) {
	var wonDeals int
	var medianSeconds *float64
	err := c.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			WITH stints AS (
				SELECT d.id AS deal_id,
				       extract(epoch FROM lead(h.changed_at) OVER (PARTITION BY h.deal_id ORDER BY h.changed_at, h.id) - h.changed_at) AS secs
				FROM deal_stage_history h
				JOIN deal d ON d.id = h.deal_id
				WHERE d.pipeline_id = $1 AND d.status = 'won' AND d.archived_at IS NULL
			)
			SELECT count(DISTINCT deal_id),
			       percentile_cont(0.5) WITHIN GROUP (ORDER BY secs)
			FROM stints WHERE secs IS NOT NULL`, pipelineID).Scan(&wonDeals, &medianSeconds)
	})
	if err != nil {
		return 0, err
	}
	if wonDeals < CloseDateMinHistory || medianSeconds == nil || *medianSeconds <= 0 {
		return 0, nil
	}
	return *medianSeconds / 86400, nil
}

// apply runs one tier's write shape: re-verify the deal is still open
// and live under a row lock, patch it, audit with the exact before/after
// diff (the reversibility the 🟢 tier promises), and emit deal.updated —
// all in one transaction. Returns the row's post-write version so a 🟡
// staging can bind to exactly what the human will see.
// Returns the row's post-write version and whether a domain write actually
// happened — the caller records the latter in the run's ledger, because a tier
// that decided to correct and then wrote nothing must not be counted as one.
func (c *CloseDateCorrector) apply(ctx context.Context, cand closeDateCandidate, correction string, evidence CorrectionEvidence, runID ids.UUID, build func(*storekit.Patch), extra map[string]any) (int64, bool, error) {
	var version int64
	var wrote bool
	err := c.db.Tx(ctx, func(tx pgx.Tx) error {
		// The candidate scan and this write are separate transactions:
		// a deal closed or archived in between must not be re-dated.
		lock, err := storekit.LockRow(ctx, tx, dealTable, cand.id.UUID, storekit.LiveOnly)
		if errors.Is(err, apperrors.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		var status string
		if err := tx.QueryRow(ctx, `SELECT status FROM deal WHERE id = $1`, cand.id).Scan(&status); err != nil {
			return err
		}
		if DealStatus(status) != DealOpen {
			return nil
		}
		// The kill switch, read here rather than once per pass: an operator who
		// switches maintenance off mid-sweep stops the next deal's write, not
		// the one after it. A halted write is not a failure — the pass carries
		// on assessing, and the version is still read so a 🟡 staging binds to
		// the row a human would see.
		on, err := settings.ApplyTx(ctx, tx, MaintenanceWritesEnabled)
		if err != nil {
			return fmt.Errorf("read whether deal maintenance may write: %w", err)
		}
		if !on {
			return tx.QueryRow(ctx, `SELECT version FROM deal WHERE id = $1`, cand.id).Scan(&version)
		}
		patch := storekit.NewPatch()
		build(patch)
		if patch.Empty() {
			// The tier fired but the deal already holds everything it proposes —
			// a quiet deal re-flagged on a night its velocity date has not moved.
			// There is no write to make, and no audit row or event to raise about
			// one. The version is still read, because a 🟡 staging binds to it.
			return tx.QueryRow(ctx, `SELECT version FROM deal WHERE id = $1`, cand.id).Scan(&version)
		}
		if err := applyDealPatchLocked(ctx, tx, patch, lock); err != nil {
			return fmt.Errorf("apply %s patch: %w", correction, err)
		}
		if err := tx.QueryRow(ctx, `SELECT version FROM deal WHERE id = $1`, cand.id).Scan(&version); err != nil {
			return err
		}
		// The evidence goes on the AUDIT row, not only into the event's changed
		// fields. The morning receipt reads its reason back out of
		// audit_log.evidence long after the event has been consumed, so an
		// event-only basis renders a correction with no stated reason.
		auditEvidence := map[string]any{CloseDateCorrectionKind: correction}
		for k, v := range extra {
			auditEvidence[k] = v
		}
		auditID, err := storekit.AuditWithEvidence(
			ctx, tx, "update", "deal", cand.id.UUID, patch.Before(), patch.After(), auditEvidence)
		if err != nil {
			return fmt.Errorf("audit %s: %w", correction, err)
		}
		changedFields := map[string]any{CloseDateCorrectionKind: correction}
		for field, v := range patch.After() {
			changedFields[field] = v
		}
		for k, v := range extra {
			changedFields[k] = v
		}
		if err := storekit.EmitEvent(ctx, tx, auditID, cand.id.UUID, crmcontracts.PublicEventDealUpdated{ChangedFields: changedFields}); err != nil {
			return fmt.Errorf("emit %s: %w", correction, err)
		}
		// The lifecycle row commits WITH the change it describes. Written in a
		// second transaction it could be lost to a crash, and then the deal has
		// moved while nothing records that a correction moved it — which is the
		// state this row exists to make impossible.
		if _, err := recordCorrection(ctx, tx, DealCorrection{
			DealID:     cand.id,
			AuditLogID: auditID,
			Correction: correction,
			// Moved, not After: After holds every column the tier SET, and a
			// reversal that tried to restore a field which never changed would
			// be putting back a value that is already there.
			Fields:   movedFields(patch),
			Evidence: evidence,
		}, &runID); err != nil {
			return err
		}
		wrote = true
		return nil
	})
	return version, wrote, err
}

func dateString(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format(time.DateOnly)
	return &s
}
