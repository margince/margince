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
	stager   CorrectionStager
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

// NewCloseDateCorrector assembles the sweep over the pool it reads through,
// the stager it raises corrections into, and the seam that answers which zone
// its dates are computed in.
func NewCloseDateCorrector(db *database.DB, stager CorrectionStager, reviewer QuietReviewReader,
	log *slog.Logger, inst Installation,
) *CloseDateCorrector {
	return &CloseDateCorrector{
		db: db, stager: stager, reviewer: reviewer, log: log,
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
	winProbability int
	remainingOpen  int
}

func (c *CloseDateCorrector) sweepWorkspace(ctx context.Context) error {
	var tzName string
	var candidates []closeDateCandidate
	now := c.now().UTC()
	err := c.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		if tzName, err = c.installation.Timezone(ctx, tx); err != nil {
			return fmt.Errorf("read the installation's timezone: %w", err)
		}
		// The pre-filter is a deliberate superset of the §11 flags — a
		// date inside the widest (stalled) window, missing, or still
		// provisional; anything beyond it cannot be flagged today.
		rows, err := tx.Query(ctx, `
			SELECT d.id, d.name, d.created_at, d.last_activity_at, d.wait_until,
			       d.expected_close_date, d.close_date_provisional, d.forecast_category,
			       d.pipeline_id, s.win_probability,
			       (SELECT count(*) FROM stage s2
			         WHERE s2.pipeline_id = d.pipeline_id AND s2.archived_at IS NULL
			           AND s2.semantic = 'open' AND s2.position >= s.position)
			FROM deal d
			JOIN stage s ON s.id = d.stage_id
			WHERE d.status = 'open' AND d.archived_at IS NULL
			  AND (d.expected_close_date IS NULL
			       OR d.expected_close_date <= (timezone($1, now()))::date + $2::int
			       OR d.close_date_provisional)
			ORDER BY d.created_at, d.id
			LIMIT $3`, tzName, StalledThresholdDays, closeDateBatch)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var cand closeDateCandidate
			if err := rows.Scan(&cand.id, &cand.name, &cand.createdAt, &cand.lastActivityAt,
				&cand.waitUntil, &cand.expectedClose, &cand.provisional, &cand.forecastCat,
				&cand.pipelineID, &cand.winProbability, &cand.remainingOpen); err != nil {
				return err
			}
			candidates = append(candidates, cand)
		}
		return rows.Err()
	})
	if err != nil {
		return err
	}
	loc, err := installationZone(tzName)
	if err != nil {
		return err
	}

	velocities := map[ids.PipelineID]float64{}
	for _, cand := range candidates {
		velocity, known := velocities[cand.pipelineID]
		if !known {
			if velocity, err = c.stageVelocityDays(ctx, cand.pipelineID); err != nil {
				return fmt.Errorf("stage velocity for pipeline %s: %w", cand.pipelineID, err)
			}
			velocities[cand.pipelineID] = velocity
		}
		category := effectiveForecastCategory(cand.forecastCat, cand.winProbability)
		hygiene := CloseDateAssessment(CloseDateInput{
			Status:              "open",
			ExpectedClose:       cand.expectedClose,
			CreatedAt:           cand.createdAt,
			LastActivityAt:      cand.lastActivityAt,
			WaitUntil:           cand.waitUntil,
			StageWinProbability: cand.winProbability,
			RemainingOpenStages: cand.remainingOpen,
			InForecastCommit:    category == forecastCommit || category == forecastBestCase,
			StageVelocityDays:   velocity,
		}, now, loc)
		if err := c.correct(ctx, cand, hygiene, category, now, loc); err != nil {
			return fmt.Errorf("close-date correction on %s: %w", cand.id, err)
		}
	}
	return nil
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
func (c *CloseDateCorrector) apply(ctx context.Context, cand closeDateCandidate, correction string, build func(*storekit.Patch), extra map[string]any) (int64, error) {
	var version int64
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
		auditID, err := storekit.Audit(ctx, tx, "update", "deal", cand.id.UUID, patch.Before(), patch.After())
		if err != nil {
			return fmt.Errorf("audit %s: %w", correction, err)
		}
		changedFields := map[string]any{"close_date_correction": correction}
		for field, v := range patch.After() {
			changedFields[field] = v
		}
		for k, v := range extra {
			changedFields[k] = v
		}
		if err := storekit.EmitEvent(ctx, tx, auditID, cand.id.UUID, crmcontracts.PublicEventDealUpdated{ChangedFields: changedFields}); err != nil {
			return fmt.Errorf("emit %s: %w", correction, err)
		}
		return nil
	})
	return version, err
}

// ensureStaged stages the 🟡 confirm-the-real-date proposal unless one is
// already pending, or the rep has already refused this very date.
//
// TWO checks, because they answer different questions and each has a gap the
// other fills. Pending stops the same live card multiplying. Refused stops a
// decided one returning — and it is needed HERE, above the staging's own
// memory, because the sweep writes its new date onto the deal before staging:
// the standing date the identity is drawn from has already moved by the time
// the proposal is built, so a refusal recorded last night matches nothing
// tonight. Comparing the date being PROPOSED is what recognises it.
//
// Per date rather than per deal, so one "no" silences the date it was about
// rather than ending close-date hygiene on that deal for good.
func (c *CloseDateCorrector) ensureStaged(ctx context.Context, cand closeDateCandidate, targetVersion int64, proposal CloseDateCorrection) error {
	dealID, name := cand.id, cand.name
	pending, err := c.stager.HasPendingCorrection(ctx, dealID.UUID)
	if err != nil {
		return err
	}
	if pending {
		return nil
	}
	refused, err := c.stager.RefusedCloseDate(ctx, dealID.UUID, ProbeFor(proposal, cand.expectedClose))
	if err != nil {
		return err
	}
	if refused {
		return nil
	}
	if targetVersion == 0 {
		// The keep-alive path wrote nothing this pass; bind the staging
		// to the row's current version so redemption still detects skew.
		err := c.db.Tx(ctx, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT version FROM deal WHERE id = $1`, dealID).Scan(&targetVersion)
		})
		if err != nil {
			return err
		}
	}
	summary := fmt.Sprintf("Confirm the real close date for %q (proposed %s)", name, proposal.ExpectedCloseDate)
	return c.stager.StageCorrection(ctx, dealID.UUID, targetVersion, summary, proposal)
}

func dateString(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format(time.DateOnly)
	return &s
}
