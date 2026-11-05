// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

// A manager's standing call whose pipeline moved since it was made.
//
// The call is a number somebody stood behind on a day. A deal it was drawn
// from may change in a way the forecast counts. The number then no longer
// describes the pipeline beside it, and somebody should look again. The
// forecast engine compares the call with its baseline in compose. This pass
// turns its answer into one finding per call.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// SubjectKindForecastCall names a finding about a manager's call.
const SubjectKindForecastCall = "forecast_call"

// TypeForecastCallMoved is a standing call whose baseline the pipeline moved
// away from, or that can no longer be compared with it.
const TypeForecastCallMoved = "forecast_call_moved"

// CallSubject is one standing call and what its comparison came to. A call
// with no baseline is not a subject: nothing can be said about what moved.
type CallSubject struct {
	CallID      string
	Owner       string
	PeriodStart time.Time
	PeriodEnd   time.Time
	ScopeKind   string
	ScopeID     string
	CalledMinor int64
	Currency    string
	SubmittedAt time.Time
	// Limitation names why the baseline cannot be compared with today, such
	// as a changed team or base currency. The counts are then empty.
	Limitation string
	// DealIDs are the deals that moved materially, sorted.
	DealIDs []string
	// Net is each reading's change since the call, keyed by reading. The
	// readings are different measures and are never added up.
	Net                  map[string]int64
	QualificationChanges int
	BaselineSnapshotID   string
}

// CallSubjectsFunc reads the standing calls and compares each with its
// baseline through the forecast engine.
type CallSubjectsFunc func(ctx context.Context, tx pgx.Tx, now time.Time) ([]CallSubject, error)

// WithCallSubjects asks the pass to check standing calls too.
func (s *Scanner) WithCallSubjects(subjects CallSubjectsFunc) *Scanner {
	wired := *s
	wired.calls = subjects
	return &wired
}

// askCalls raises one finding per call that needs review. A call nothing
// material moved under raises none, however old it is.
func (s *Scanner) askCalls(ctx context.Context, tx pgx.Tx, now time.Time, pass *walked) error {
	if s.calls == nil {
		return nil
	}
	subjects, err := s.calls(ctx, tx, now)
	if err != nil {
		return err
	}
	for _, subject := range subjects {
		finding := callFinding(subject)
		if finding == nil {
			continue
		}
		pass.raised = append(pass.raised, raised{finding: *finding, owner: subject.Owner})
		pass.seen = append(pass.seen, LogicalKey(*finding))
	}
	return nil
}

// callFinding is the finding one compared call raises, nil when it is current.
func callFinding(c CallSubject) *Finding {
	if c.Limitation == "" && len(c.DealIDs) == 0 {
		return nil
	}
	claim := map[string]any{
		"called_minor": c.CalledMinor, "currency": c.Currency,
		"period_start": c.PeriodStart.Format(time.DateOnly), "period_end": c.PeriodEnd.Format(time.DateOnly),
		"scope_kind": c.ScopeKind, "submitted_at": c.SubmittedAt, "baseline_snapshot_id": c.BaselineSnapshotID,
	}
	if c.ScopeID != "" {
		claim["scope_id"] = c.ScopeID
	}
	if c.Limitation != "" {
		return &Finding{
			Type: TypeForecastCallMoved, SubjectKind: SubjectKindForecastCall, SubjectID: c.CallID,
			Severity: SeverityLow, Claim: claim,
			Observed: map[string]any{"comparison": "limited", "limitation": c.Limitation},
		}
	}
	// Every moved deal is stored, so a reader's own scope can be applied to
	// the list on the way out (compose/assurancecallread.go).
	observed := map[string]any{
		"comparison": "moved", "changed_deals": len(c.DealIDs), "deal_ids": c.DealIDs,
		"qualification_changes": c.QualificationChanges,
	}
	for reading, net := range c.Net {
		observed[reading+"_change_minor"] = net
	}
	return &Finding{
		Type: TypeForecastCallMoved, SubjectKind: SubjectKindForecastCall, SubjectID: c.CallID,
		Severity: SeverityMedium, Claim: claim, Observed: observed,
	}
}

// closeAbsentCalls closes every open call finding this pass did not raise
// again, because the pipeline came back to the call. It also closes the
// finding of a call a later call replaced, even one raised this pass. A call
// recorded while the pass ran is visible to this statement.
func (s *Store) closeAbsentCalls(ctx context.Context, tx pgx.Tx, seen []string) (int64, error) {
	if err := auth.Require(ctx, "forecast", principal.ActionUpdate); err != nil {
		return 0, err
	}
	if seen == nil {
		seen = []string{}
	}
	tag, err := tx.Exec(ctx, `UPDATE assurance_exception SET status = $1, updated_at = now()
		WHERE subject_kind = $2 AND type = $3 AND status = 'open'
		  AND (NOT (logical_key = ANY($4))
		       OR EXISTS (SELECT 1 FROM forecast_call later WHERE later.supersedes_id = subject_id))`,
		ExceptionConditionCleared, SubjectKindForecastCall, TypeForecastCallMoved, seen)
	if err != nil {
		return 0, fmt.Errorf("assurance: clearing call findings nothing raised again: %w", err)
	}
	return tag.RowsAffected(), nil
}
