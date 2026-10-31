// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// Deal health (formulas-and-rules §10.5, B-E09.15/.16/.17): one
// deterministic weighted sum of four normalized factors, so a golden
// test can assert health == Σ wᵢ·factorᵢ and every factor carries its
// source-record evidence (P6 "no mystery number"). This is a *health*
// lens, not the Morning-Brief priority composite. Computing it is
// advisory by construction: the store path is read-only — it never
// writes the score back, flips a flag, or touches the deal row.

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// §10.5 tunables (spec parameter-registry names in comments).
const (
	healthWeightActivity    = 0.30 // W_HEALTH_ACT
	healthWeightVelocity    = 0.25 // W_HEALTH_VEL
	healthWeightEngagement  = 0.20 // W_HEALTH_ENG
	healthWeightCommitments = 0.25 // W_HEALTH_COM

	// engageNorm is the distinct engaged-stakeholder count at which the
	// engagement factor saturates.
	engageNorm = 3.0 // ENGAGE_NORM

	// stageVelocityFallbackDays stands in for the workspace median when
	// fewer than stageVelocityMinWonDeals won deals carry history for
	// the subject's current stage.
	stageVelocityFallbackDays = 14.0 // STAGE_VELOCITY_FALLBACK_DAYS
	stageVelocityMinWonDeals  = 10

	// dealHealthAtRisk is the at-risk threshold: health below it drives
	// the §11/§8 at-risk flag and signal.kind='risk'.
	dealHealthAtRisk = 0.35 // DEAL_HEALTH_AT_RISK

	// healthEngagementWindowDays is the §4 two-way-engagement window an
	// "engaged" stakeholder must have both directions inside.
	healthEngagementWindowDays = 90
)

// DealHealthFactors is the per-factor decomposition, each 0..1
// (1.0 = healthiest).
type DealHealthFactors struct {
	ActivityRecency float64
	StageVelocity   float64
	Engagement      float64
	Commitments     float64
}

// DealHealthWeights exposes the weights the score folded with, so a
// client can reconcile health == Σ wᵢ·factorᵢ without knowing the spec.
type DealHealthWeights struct {
	ActivityRecency float64
	StageVelocity   float64
	Engagement      float64
	Commitments     float64
}

// DealHealthEvidence links every factor to the records it was computed
// from — the clickable "why" behind each number.
type DealHealthEvidence struct {
	// MostRecentActivityID is the newest activity behind the past clock; nil
	// when the deal has none (recency 0.0 unless MeetingAhead).
	// note: the health evidence ids (activity/contact refs below) are
	// operational explainability pointers gathered through the generic
	// collectIDs helper, not typed entity handles — they stay ids.UUID.
	MostRecentActivityID *ids.UUID
	// LastActivityAt is the past clock the recency factor bands; nil with
	// MostRecentActivityID.
	LastActivityAt *time.Time
	// LastActivityDateOnly says the activity behind LastActivityAt names only a
	// date, so days count from that date.
	LastActivityDateOnly bool
	// MeetingAhead is a booked meeting the clock has not reached, which
	// scores recency 1.0 and keeps the deal off stalled.
	MeetingAhead bool

	// StageVelocity: where the deal sits and how its pace compares.
	CurrentStageID      ids.StageID
	DaysInStage         float64
	ExpectedDaysInStage float64

	// EngagedStakeholderIDs are the contact ids counted by the
	// engagement factor (two-way contact inside the window).
	EngagedStakeholderIDs []ids.UUID

	// Commitments: the open overdue task activity ids, and the §8
	// stalled flag that zeroes the factor outright.
	OverdueTaskIDs []ids.UUID
	Stalled        bool
}

// DealHealth is the explainable §10.5 output.
type DealHealth struct {
	DealID   ids.DealID
	Health   float64
	AtRisk   bool
	Factors  DealHealthFactors
	Weights  DealHealthWeights
	Evidence DealHealthEvidence
}

// dealHealthInputs are the raw facts the store gathers; healthFromInputs
// folds them deterministically so the formula is testable without a
// database.
type dealHealthInputs struct {
	dealID         ids.DealID
	status         string
	createdAt      time.Time
	lastActivityAt *time.Time
	// lastActivityDateOnly says the activity behind lastActivityAt names only
	// a date.
	lastActivityDateOnly bool
	waitUntil            *time.Time
	meetingAhead         bool
	stageID              ids.StageID

	// stageEnteredAt is the latest history entry into the current stage,
	// falling back to createdAt when the deal never moved.
	stageEnteredAt time.Time

	// medianWonStageDays is the workspace median days won deals of this
	// pipeline spent in the subject's current stage; nil below the
	// stageVelocityMinWonDeals history floor (→ fallback).
	medianWonStageDays *float64

	mostRecentActivityID  *ids.UUID
	engagedStakeholderIDs []ids.UUID
	overdueTaskIDs        []ids.UUID
}

// healthFromInputs is the pure §10.5 fold: same inputs + clock, same
// score, no I/O.
func healthFromInputs(in dealHealthInputs, now time.Time) DealHealth {
	expected := stageVelocityFallbackDays
	// A median under a day (instant stage hops — an import, a demo seed, a
	// deal closed the day it was opened) is not an expectation anyone holds:
	// measured against it every live deal reads as crawling and the pace
	// factor is zero for all of them. The fallback is the honest expectation
	// in that degenerate history, exactly as for a non-positive one.
	if in.medianWonStageDays != nil && *in.medianWonStageDays >= 1 {
		expected = *in.medianWonStageDays
	}
	age := daysBetween(in.stageEnteredAt, now)

	stalled := IsStalled(in.status, in.createdAt, in.lastActivityAt, in.waitUntil, in.meetingAhead, now)
	factors := DealHealthFactors{
		ActivityRecency: recencyScore(in.lastActivityAt, in.meetingAhead, now),
		StageVelocity:   velocityScore(age, expected),
		Engagement:      math.Min(1.0, float64(len(in.engagedStakeholderIDs))/engageNorm),
		Commitments:     commitmentScore(stalled, len(in.overdueTaskIDs)),
	}
	health := healthWeightActivity*factors.ActivityRecency +
		healthWeightVelocity*factors.StageVelocity +
		healthWeightEngagement*factors.Engagement +
		healthWeightCommitments*factors.Commitments

	return DealHealth{
		DealID:  in.dealID,
		Health:  health,
		AtRisk:  health < dealHealthAtRisk,
		Factors: factors,
		Weights: DealHealthWeights{
			ActivityRecency: healthWeightActivity,
			StageVelocity:   healthWeightVelocity,
			Engagement:      healthWeightEngagement,
			Commitments:     healthWeightCommitments,
		},
		Evidence: DealHealthEvidence{
			MostRecentActivityID:  in.mostRecentActivityID,
			CurrentStageID:        in.stageID,
			DaysInStage:           age,
			ExpectedDaysInStage:   expected,
			EngagedStakeholderIDs: in.engagedStakeholderIDs,
			OverdueTaskIDs:        in.overdueTaskIDs,
			Stalled:               stalled,
			LastActivityAt:        in.lastActivityAt,
			LastActivityDateOnly:  in.lastActivityDateOnly,
			MeetingAhead:          in.meetingAhead,
		},
	}
}

// recencyScore bands the past clock: an absolute-duration day count on UTC
// instants (like IsStalled), never a calendar-day count. A booked meeting
// ahead counts as today's contact.
func recencyScore(lastActivityAt *time.Time, meetingAhead bool, now time.Time) float64 {
	if meetingAhead {
		return 1.0
	}
	if lastActivityAt == nil {
		return 0.0
	}
	d := daysBetween(*lastActivityAt, now)
	switch {
	case d <= 3:
		return 1.0
	case d <= 7:
		return 0.8
	case d <= 14:
		return 0.6
	case d <= 30:
		return 0.4
	case d <= 60:
		return 0.2
	default:
		return 0.0
	}
}

// velocityScore bands the pace ratio r = age/expected: at or ahead of
// the won-deal median is healthy, ≥2× expected is stuck.
func velocityScore(ageDays, expectedDays float64) float64 {
	r := ageDays / expectedDays
	switch {
	case r <= 1.0:
		return 1.0
	case r <= 1.5:
		return 0.6
	case r <= 2.0:
		return 0.3
	default:
		return 0.0
	}
}

// commitmentScore: a §8-stalled deal has broken its commitments outright;
// otherwise the open-overdue-task count bands the factor.
func commitmentScore(stalled bool, openOverdue int) float64 {
	if stalled {
		return 0.0
	}
	switch openOverdue {
	case 0:
		return 1.0
	case 1:
		return 0.5
	default:
		return 0.2
	}
}

// daysBetween is the fractional UTC-instant day count, floored at zero
// so a fixed test clock behind a freshly seeded row cannot go negative.
func daysBetween(from, to time.Time) float64 {
	d := to.Sub(from).Hours() / 24
	if d < 0 {
		return 0
	}
	return d
}

// healthInteraction is the activity that qualifies as two-way engagement: an
// interaction kind (§4 inputs; tasks and notes are not contact), and no
// canceled or no-show meeting. It reads the shared definition rather than
// restating it: relationship scoring, deal health and contact strength all ask
// the same question, and a rule that drifts between them makes the network and
// coverage signals disagree about one activity.
var healthInteraction = relstrength.InteractionCountsSQL("a")

// DealHealth computes the §10.5 score for one deal. The read is
// row-scoped exactly like GetDeal — a deal the caller cannot see has no
// health to disclose — and the whole path is read-only (B-E09.17): the
// transaction issues SELECTs only.
func (s *Store) DealHealth(ctx context.Context, dealID ids.DealID, now time.Time) (DealHealth, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return DealHealth{}, err
	}
	in := dealHealthInputs{dealID: dealID}
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureVisible(ctx, tx, dealTable, dealID.UUID); err != nil {
			return err
		}
		return healthInputs(ctx, tx, now, &in)
	})
	if err != nil {
		return DealHealth{}, err
	}
	return healthFromInputs(in, now), nil
}

// healthInputs gathers the raw facts for one deal inside the workspace
// transaction: the deal row, its stage-entry instant, the won-deal
// stage-duration median, and the per-factor evidence rows.
func healthInputs(ctx context.Context, tx pgx.Tx, now time.Time, in *dealHealthInputs) error {
	// The deal's own fields AND the record behind last_activity_at, in ONE
	// statement.
	//
	// Read apart, they are two snapshots: this transaction is READ COMMITTED,
	// so an activity committed between the two statements is counted by the
	// later one and not by the earlier. The evidence would then name a row the
	// timestamp does not, and the card would cite a message as the reason for a
	// staleness the message disproves — a contradiction a reader can see and
	// nothing can explain.
	//
	// The subquery's meeting, origin, audience and time filters are the ones
	// last_activity_of_deal reads, so the evidence names a row the clock counts.
	// It also hides a follow-up task whose source is restricted, which the
	// clock does not. The meeting-ahead and day columns are read here for the
	// same reason.
	var pipelineID ids.PipelineID
	var recent *ids.UUID
	args := []any{in.dealID}
	arg := func(v any) int { args = append(args, v); return len(args) }
	nowPos := arg(now)
	err := tx.QueryRow(ctx, `
		SELECT d.status, d.created_at, d.last_activity_at, d.wait_until, d.stage_id, d.pipeline_id,
		       (SELECT a.id FROM activity a
		          JOIN activity_link l ON l.activity_id = a.id AND l.deal_id = d.id
		         WHERE a.archived_at IS NULL AND `+relstrength.LastActivityCountsSQL("a", storekit.SQLf("$%d::timestamptz", nowPos))+`
		           `+auth.OriginIsEngagement("a")+auth.AudienceWorkspaceOnly("a")+`
		         ORDER BY a.occurred_at DESC, a.id DESC
		         LIMIT 1),
		       `+LastActivityDaySQL("d")+`, `+MeetingAheadSQL("d")+`
		FROM deal d WHERE d.id = $1 AND d.archived_at IS NULL`, args...).
		Scan(&in.status, &in.createdAt, &in.lastActivityAt, &in.waitUntil, &in.stageID, &pipelineID, &recent,
			&in.lastActivityDateOnly, &in.meetingAhead)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.ErrNotFound
	}
	if err != nil {
		return err
	}
	// No activity ever: recency is 0.0 with no record to point at.
	in.mostRecentActivityID = recent

	// Days in current stage: the latest entry INTO this stage; a deal
	// that never moved has sat there since creation.
	var enteredAt *time.Time
	if err := tx.QueryRow(ctx, `
		SELECT max(changed_at) FROM deal_stage_history
		WHERE deal_id = $1 AND to_stage_id = $2`, in.dealID, in.stageID).Scan(&enteredAt); err != nil {
		return err
	}
	in.stageEnteredAt = in.createdAt
	if enteredAt != nil {
		in.stageEnteredAt = *enteredAt
	}

	if err := healthExpectedPace(ctx, tx, pipelineID, in); err != nil {
		return err
	}
	return healthActivityEvidence(ctx, tx, now, in)
}

// healthExpectedPace derives the won-deal stage-duration median the
// stage-velocity factor compares against.
func healthExpectedPace(ctx context.Context, tx pgx.Tx, pipelineID ids.PipelineID, in *dealHealthInputs) error {
	// Expected pace: for each won deal of this pipeline, the completed
	// stints in the subject's current stage (entry row → the next stage
	// change); the median of those durations. A stint still open at the
	// win has no exit row and contributes nothing. A stint only counts when
	// both its moves were made in Margince: an import lands a deal's whole
	// history in one instant, which would read as a stint of zero days. Below the
	// ten-won-deal history floor the median is statistically noise, so
	// the fold falls back to STAGE_VELOCITY_FALLBACK_DAYS.
	var wonDealsWithHistory int
	var medianSeconds *float64
	if err := tx.QueryRow(ctx, `
		WITH stints AS (
			SELECT h.deal_id, h.to_stage_id, h.changed_at AS entered, h.provenance,
			       lead(h.changed_at) OVER (PARTITION BY h.deal_id ORDER BY h.changed_at, h.id) AS left_at,
			       lead(h.provenance) OVER (PARTITION BY h.deal_id ORDER BY h.changed_at, h.id) AS left_provenance
			FROM deal_stage_history h
			JOIN deal d ON d.id = h.deal_id
			WHERE d.pipeline_id = $1 AND d.status = 'won' AND d.archived_at IS NULL
		)
		SELECT count(DISTINCT deal_id),
		       percentile_cont(0.5) WITHIN GROUP (ORDER BY extract(epoch FROM left_at - entered))
		FROM stints
		WHERE to_stage_id = $2 AND left_at IS NOT NULL
		  AND provenance = 'recorded' AND left_provenance = 'recorded'`,
		pipelineID, in.stageID).Scan(&wonDealsWithHistory, &medianSeconds); err != nil {
		return err
	}
	if wonDealsWithHistory >= stageVelocityMinWonDeals && medianSeconds != nil {
		days := *medianSeconds / 86400
		in.medianWonStageDays = &days
	}
	return nil
}

// healthActivityEvidence gathers the per-factor evidence rows: the
// freshest activity (recency), the two-way-engaged stakeholders, and
// the open overdue tasks (commitments).
func healthActivityEvidence(ctx context.Context, tx pgx.Tx, now time.Time, in *dealHealthInputs) error {
	// Engagement: the ONE definition, through the function that owns it, so the
	// composite's engagement factor and the coverage view's engaged seats
	// cannot disagree about the same deal.
	//
	// It carries the edge gate, and a caller refused it gets no score rather
	// than a lower one. That is the whole reason the gate belongs here: the
	// factor is a count of edges divided by a norm, so an edge the caller may
	// not read would silently subtract from the health of a deal that is
	// perfectly healthy — a WRONG number where the refusal is a missing one.
	//
	// It runs FIRST for that reason and no other. Order is free inside one
	// transaction — every fact here is read at the same instant either way —
	// so putting the gated read at the front costs nothing and buys the
	// property worth having: the refusal resolves before the composite issues
	// any statement at all, which is a claim a test can make with no database.
	engaged, err := EngagedStakeholders(ctx, tx, in.dealID, now)
	if err != nil {
		return err
	}
	in.engagedStakeholderIDs = engaged

	args := []any{in.dealID, now}
	arg := func(v any) int { args = append(args, v); return len(args) }
	identities, err := storekit.MailSeatIdentitiesSQL(ctx, tx, arg)
	if err != nil {
		return err
	}
	current := storekit.CurrentTaskClause("a", now, identities, arg)
	overdue, err := collectIDs(tx.Query(ctx, `
  SELECT a.id FROM activity a
  JOIN activity_link l ON l.activity_id = a.id AND l.deal_id = $1
  WHERE a.kind = 'task' AND a.is_done = false AND a.archived_at IS NULL
   AND `+storekit.TaskDueSQL("a")+` IS NOT NULL AND `+storekit.TaskDueSQL("a")+` < $2 AND `+current+`
  ORDER BY `+storekit.TaskDueSQL("a")+`, a.id`, args...))
	if err != nil {
		return err
	}
	in.overdueTaskIDs = overdue
	return nil
}

// collectIDs drains a single-uuid-column result set.
func collectIDs(rows pgx.Rows, err error) ([]ids.UUID, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ids.UUID
	for rows.Next() {
		var id ids.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
