// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

// Two open deals of one account that are one buying motion.
//
// A large account runs several deals at once, so "same account, both open"
// would fire all the time. Two deals are one motion only when they share
// evidence, a mail, a meeting or a stakeholder contact. They must also sit in
// one pipeline with close dates inside DuplicateMotionWindowDays. The finding
// is filed on the later deal.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// PairSubject is two open deals of one account as the rule sees them, already
// assembled by the seam. DealID is the one created later.
type PairSubject struct {
	DealID      string
	OtherDealID string
	Owner       string
	// The later deal's money, which counts twice while both stand.
	AmountMinor *int64
	Currency    string
	PipelineID  string
	// Each deal's own, so the rule compares them.
	OtherPipelineID    string
	ExpectedClose      *time.Time
	OtherExpectedClose *time.Time
	// How many mails and meetings, and how many stakeholder contacts, both
	// deals carry. The seam counts what any reader may see.
	SharedActivities   int
	SharedStakeholders int
}

// PairSubjectsFunc reads the candidate pairs of open deals that share evidence.
// With only set, it reads just the pairs that include one of those deals.
type PairSubjectsFunc func(ctx context.Context, tx pgx.Tx, now time.Time, only []ids.UUID) ([]PairSubject, error)

// WithPairSubjects asks the pass to check pairs of deals too.
func (s *Scanner) WithPairSubjects(subjects PairSubjectsFunc) *Scanner {
	wired := *s
	wired.pairs = subjects
	return &wired
}

// askPairs raises one finding per pair that reads as one buying motion.
func (s *Scanner) askPairs(ctx context.Context, tx pgx.Tx, now time.Time, only []ids.UUID, pass *walked) error {
	if s.pairs == nil {
		return nil
	}
	subjects, err := s.pairs(ctx, tx, now, only)
	if err != nil {
		return err
	}
	for _, subject := range subjects {
		finding := duplicateMotion(subject)
		if finding == nil {
			continue
		}
		pass.raised = append(pass.raised, raised{finding: *finding, owner: subject.Owner})
		pass.seen = append(pass.seen, LogicalKey(*finding))
	}
	return nil
}

// closeClearedPairsOf closes the open pair findings that include deal and that
// a pass over it no longer raises. It closes only those findings, never
// another pair filed on the same deal, and keeps the remind-later carve-out.
func (s *Store) closeClearedPairsOf(ctx context.Context, tx pgx.Tx, deal ids.UUID, seen []string) (int64, error) {
	if err := auth.Require(ctx, "forecast", principal.ActionUpdate); err != nil {
		return 0, err
	}
	if seen == nil {
		seen = []string{}
	}
	tag, err := tx.Exec(ctx, `
		UPDATE assurance_exception
		SET status = $1, updated_at = now()
		WHERE status = 'open' AND type = $2 AND subject_kind = 'deal'
		  AND (subject_id = $3 OR observed ->> $4 = $3::text)
		  AND NOT (logical_key = ANY($5))
		  AND `+notDeferredSQL("$6", "$7"),
		ExceptionConditionCleared, TypeDuplicateMotion, deal, SlotOtherDealID, seen,
		OutcomeRemindLater, MaxSuppressionDays())
	if err != nil {
		return 0, fmt.Errorf("assurance: closing the pair findings of a deal: %w", err)
	}
	return tag.RowsAffected(), nil
}

// duplicateMotion is the rule. A deal with no close date is never flagged. The
// window cannot be measured, and a guess would flag every account that keeps
// its deals undated.
func duplicateMotion(s PairSubject) *Finding {
	if s.PipelineID != s.OtherPipelineID || s.ExpectedClose == nil || s.OtherExpectedClose == nil {
		return nil
	}
	if s.SharedActivities+s.SharedStakeholders == 0 {
		return nil
	}
	gap := closeGapDays(*s.ExpectedClose, *s.OtherExpectedClose)
	if gap > DuplicateMotionWindowDays {
		return nil
	}
	return &Finding{
		Type: TypeDuplicateMotion, SubjectID: s.DealID, OtherID: s.OtherDealID, Severity: SeverityMedium,
		Claim: map[string]any{"expected_close": s.ExpectedClose.Format(time.DateOnly)},
		Observed: map[string]any{
			SlotOtherDealID: s.OtherDealID, "shared_activities": s.SharedActivities,
			"shared_stakeholders": s.SharedStakeholders, SlotCloseGapDays: gap,
		},
		AffectedMinor: s.AmountMinor, Currency: s.Currency,
	}
}

// The observed slots naming the second deal. A reader who may not see it is
// shown the finding without them.
const (
	SlotOtherDealID  = "other_deal_id"
	SlotCloseGapDays = "close_gap_days"
)

// closeGapDays is the whole days between two close dates, which carry no clock.
func closeGapDays(a, b time.Time) int {
	day := func(t time.Time) time.Time { return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC) }
	gap := day(a).Sub(day(b))
	if gap < 0 {
		gap = -gap
	}
	return int(gap / (24 * time.Hour))
}
