// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"context"
	"crypto/subtle"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/snapshotreads"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Catalog returns the metric vocabulary available to the current caller.
func (s *Service) Catalog(ctx context.Context) (crmcontracts.ReportingCatalog, error) {
	if err := auth.Require(ctx, "report_definition", principal.ActionRead); err != nil {
		return crmcontracts.ReportingCatalog{}, err
	}
	var out crmcontracts.ReportingCatalog
	err := s.store.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = s.evaluator.Catalog(ctx, s.readerCopy(ctx, tx).Lang())
		if err != nil {
			return err
		}
		canSchedule := auth.Allows(ctx, "report_schedule", principal.ActionCreate) || auth.Allows(ctx, "report_schedule", principal.ActionUpdate)
		if !canSchedule {
			return nil
		}
		ready, err := scheduleReady(ctx, tx)
		out.ScheduleReady = &ready
		if err != nil || !ready {
			return err
		}
		days, err := editionRetentionDays(ctx, tx)
		out.EditionRetentionDays = &days
		return err
	})
	return out, err
}

// Evaluate uses a repeatable-read snapshot for values and their evidence.
func (s *Service) Evaluate(ctx context.Context, selection crmcontracts.ReportingSelection) (crmcontracts.ReportingEvaluation, error) {
	var out crmcontracts.ReportingEvaluation
	err := s.store.db.TxIsolated(ctx, pgx.RepeatableRead, func(tx pgx.Tx) error { var err error; out, err = s.EvaluateTx(ctx, tx, selection); return err })
	return out, err
}

// EvaluateTx shares the caller’s transaction for cross-surface metric consistency.
func (s *Service) EvaluateTx(ctx context.Context, tx pgx.Tx, selection crmcontracts.ReportingSelection) (crmcontracts.ReportingEvaluation, error) {
	if err := auth.Require(ctx, "report_definition", principal.ActionRead); err != nil {
		return crmcontracts.ReportingEvaluation{}, err
	}
	framework, err := s.framework(ctx, tx)
	if err != nil {
		return crmcontracts.ReportingEvaluation{}, err
	}
	out, err := s.evaluateSelection(ctx, tx, selection, framework)
	return out.Result, err
}

func (s *Service) evaluateSelection(ctx context.Context, tx pgx.Tx, selection crmcontracts.ReportingSelection, framework crmcontracts.ReportingFramework) (Evaluation, error) {
	return s.evaluateAt(ctx, tx, selection, framework, s.now())
}

func (s *Service) evaluateAt(ctx context.Context, tx pgx.Tx, selection crmcontracts.ReportingSelection, framework crmcontracts.ReportingFramework, at time.Time) (Evaluation, error) {
	// Every door derives the blocks from a layout, so the charts drawn are the
	// ones the layout places.
	deriveLayoutBlocks(&selection)
	withChartList(&selection)
	if err := s.validateSelection(ctx, selection); err != nil {
		return Evaluation{}, err
	}
	// The metrics, the targets and the owner names resolve the same named
	// subjects; their live-subject reads are answered once in this snapshot.
	ctx, release := snapshotreads.Open(ctx, tx)
	defer release()
	out, err := s.evaluator.Evaluate(ctx, tx, selection, framework, at)
	if err != nil {
		return Evaluation{}, err
	}
	if err := s.applyTargets(ctx, tx, &out); err != nil {
		return Evaluation{}, err
	}
	if err := distinguishOwnerNames(ctx, tx, out.Result.Charts); err != nil {
		return Evaluation{}, err
	}
	out.Result.EvaluationKey, err = evaluationKey(out)
	return out, err
}

// EvaluateReport pins the requested definition revision before evaluation.
func (s *Service) EvaluateReport(ctx context.Context, id ids.UUID, revision *int64) (crmcontracts.ReportingEvaluation, error) {
	if err := auth.Require(ctx, "report_definition", principal.ActionRead); err != nil {
		return crmcontracts.ReportingEvaluation{}, err
	}
	var out Evaluation
	err := s.store.db.TxIsolated(ctx, pgx.RepeatableRead, func(tx pgx.Tx) error {
		report, err := s.report(ctx, tx, id, false)
		if err != nil {
			return err
		}
		chosen := report.Revision
		if revision != nil {
			chosen = *revision
		}
		selection, frameworkID, err := s.evaluableRevision(ctx, tx, report, chosen)
		if err != nil {
			return err
		}
		framework, err := s.frameworkRevision(ctx, tx, frameworkID)
		if err != nil {
			return err
		}
		out, err = s.evaluateSelection(ctx, tx, selection, framework)
		if err != nil {
			return err
		}
		recorded, err := s.revisionMetricVersions(ctx, tx, id, chosen)
		if err != nil {
			return err
		}
		return withdrawChangedDefinitions(&out, recorded)
	})
	return out.Result, err
}

func (s *Service) frameworkRevision(ctx context.Context, tx pgx.Tx, revision int64) (crmcontracts.ReportingFramework, error) {
	out := crmcontracts.ReportingFramework{Revision: revision, Version: revision, Definition: crmcontracts.ReportingFrameworkInput{Template: reportingSales, Qualification: []crmcontracts.ReportingQualification{}, CaptureContexts: []crmcontracts.ReportingCaptureContext{}}}
	if revision == 0 {
		return out, nil
	}
	var b bindings
	query := "SELECT definition,effective_at FROM reporting_framework_revision WHERE revision=" + b.add(revision)
	err := tx.QueryRow(ctx, query, b.values...).Scan(&out.Definition, &out.EffectiveAt)
	return out, storedError(err)
}

// EvidenceExpectation binds live evidence to the evaluation the reader actually saw.
type EvidenceExpectation struct {
	Key               string
	At                time.Time
	FrameworkRevision int64
}

// Evidence refuses stale evaluation receipts instead of mixing observations.
func (s *Service) Evidence(ctx context.Context, selection crmcontracts.ReportingSelection, metric crmcontracts.ReportingMetricID, contextID, group string, through *time.Time, cursor *string, limit int, expectation EvidenceExpectation) (crmcontracts.ReportingEvidence, error) {
	if err := auth.Require(ctx, "report_definition", principal.ActionRead); err != nil {
		return crmcontracts.ReportingEvidence{}, err
	}
	if expectation.Key == "" || expectation.At.IsZero() || expectation.At.After(s.now()) {
		return crmcontracts.ReportingEvidence{}, invalid("open evidence from a current evaluated reading")
	}
	var out crmcontracts.ReportingEvidence
	err := s.store.db.TxIsolated(ctx, pgx.RepeatableRead, func(tx pgx.Tx) error {
		evaluation, err := s.checkedEvaluation(ctx, tx, selection, expectation)
		if err != nil {
			return err
		}
		out, err = evidencePage(evaluation.Result.Context, evaluation.Facts, metric, contextID, group, through, cursor, limit)
		return err
	})
	return out, err
}

func evidencePage(frame crmcontracts.ReportingContext, facts []Fact, metric crmcontracts.ReportingMetricID, contextID, group string, through *time.Time, cursor *string, limit int) (crmcontracts.ReportingEvidence, error) {
	out := crmcontracts.ReportingEvidence{Context: frame, Metric: metric, Rows: []crmcontracts.ReportingEvidenceRow{}}
	offset := 0
	if cursor != nil {
		parsed, err := strconv.Atoi(*cursor)
		if err != nil || parsed < 0 {
			return out, invalid("the continuation token is invalid")
		}
		offset = parsed
	}
	limit = max(1, min(limit, 100))
	week := evidenceWeek(frame, group)
	matched := 0
	for _, fact := range facts {
		if fact.Metric != metric || fact.ContextID != contextID || !evidenceGroup(fact, group, week) {
			continue
		}
		if through != nil && (fact.At() == nil || !fact.At().Before(*through)) {
			continue
		}
		matched++
		if matched <= offset {
			continue
		}
		if len(out.Rows) == limit {
			next := strconv.Itoa(offset + limit)
			out.NextCursor = &next
			out.Truncated = true
			break
		}
		out.Rows = append(out.Rows, evidenceRow(fact))
	}
	return out, nil
}

// evidenceRow shows one fact as a row, withholding what the reader may not
// see. It copies the labels, so the scan over the facts allocates nothing.
func evidenceRow(fact Fact) crmcontracts.ReportingEvidenceRow {
	row := fact.Row
	if fact.EvidenceAt != nil {
		row.OccurredAt = fact.EvidenceAt
	}
	if fact.OwnerWithheld || row.Restricted {
		row.OwnerId, row.OwnerLabel = nil, nil
	} else if owner := fact.OwnerLabel; owner != "" {
		row.OwnerLabel = &owner
	}
	if row.Restricted {
		row.StageLabel = nil
	} else if stage := fact.StageLabel; stage != "" {
		row.StageLabel = &stage
	}
	return row
}

func evidenceGroup(fact Fact, group string, week evidenceWeekSpan) bool {
	if group == "" {
		return true
	}
	if after, ok := strings.CutPrefix(group, "owner:"); ok {
		return !fact.OwnerWithheld && fact.OwnerID.String() == after
	}
	// A stage cohort's outcome group starts like its stage's and names the
	// outcome after it, so the fact's own key answers it.
	if after, ok := strings.CutPrefix(group, "stage:"); ok {
		return fact.StageID == after || fact.GroupKey == group
	}
	// A source's group names its leads and, by one more component, each outcome
	// under it. The origin is escaped, so a further colon is another source.
	if strings.HasPrefix(group, "source:") {
		outcome, under := strings.CutPrefix(fact.GroupKey, group+":")
		return fact.GroupKey == group || (under && !strings.Contains(outcome, ":"))
	}
	if strings.HasPrefix(group, "week:") && fact.At() != nil {
		return week.valid && !fact.At().Before(week.start) && fact.At().Before(week.end)
	}
	return fact.GroupKey == group
}

// evidenceWeekSpan is the local calendar span a "week:" group names. A group
// that is not a week, or names a bad date or zone, has no valid span.
type evidenceWeekSpan struct {
	start, end time.Time
	valid      bool
}

// evidenceWeek reads the week a group names once per page, not once per fact:
// loading the zone reads its rules from disk.
func evidenceWeek(frame crmcontracts.ReportingContext, group string) evidenceWeekSpan {
	date, weekly := strings.CutPrefix(group, "week:")
	if !weekly {
		return evidenceWeekSpan{}
	}
	zone, err := time.LoadLocation(frame.Timezone)
	if err != nil {
		return evidenceWeekSpan{}
	}
	start, err := time.ParseInLocation(time.DateOnly, date, zone)
	if err != nil {
		return evidenceWeekSpan{}
	}
	return evidenceWeekSpan{start: start, end: WeekEnd(start), valid: true}
}

// WeekEnd is the local midnight of the Monday after start, so a week a period
// opens midweek is short. Weekly charts and their evidence both cut weeks here.
func WeekEnd(start time.Time) time.Time {
	return start.AddDate(0, 0, 7-(int(start.Weekday())+6)%7)
}

func (s *Service) checkedEvaluation(ctx context.Context, tx pgx.Tx, selection crmcontracts.ReportingSelection, expectation EvidenceExpectation) (Evaluation, error) {
	if expectation.Key == "" || expectation.At.IsZero() || expectation.At.After(s.now()) {
		return Evaluation{}, invalid("refresh the evaluated reading")
	}
	framework, err := s.frameworkRevision(ctx, tx, expectation.FrameworkRevision)
	if err != nil {
		return Evaluation{}, err
	}
	evaluation, err := s.evaluateAt(ctx, tx, selection, framework, expectation.At)
	if err != nil {
		return Evaluation{}, err
	}
	if subtle.ConstantTimeCompare([]byte(expectation.Key), []byte(evaluation.Result.EvaluationKey)) != 1 {
		return Evaluation{}, errEvidenceStale
	}
	return evaluation, nil
}

// errEvidenceStale refuses evidence or an export asked of figures that changed since
// the report was read. The generic skew sentence speaks of a lost write, and this is a read.
var errEvidenceStale = &apperrors.VersionSkewError{
	Message: "The figures changed after this report was read. Reload the report, then try again.",
}

// EvaluatedReceipt binds an evaluation to its context and captured source values.
func (s *Service) EvaluatedReceipt(ctx context.Context, selection crmcontracts.ReportingSelection, expectation EvidenceExpectation) (crmcontracts.ReportingEvaluation, error) {
	if err := auth.Require(ctx, "report_definition", principal.ActionRead); err != nil {
		return crmcontracts.ReportingEvaluation{}, err
	}
	var out crmcontracts.ReportingEvaluation
	err := s.store.db.TxIsolated(ctx, pgx.RepeatableRead, func(tx pgx.Tx) error {
		evaluation, err := s.checkedEvaluation(ctx, tx, selection, expectation)
		out = evaluation.Result
		return err
	})
	return out, err
}

// EvaluateFactsTx evaluates a selection and keeps the facts behind it. A
// reading that splits the figure by month, owner or category then adds up the
// rows Analytics counted.
func (s *Service) EvaluateFactsTx(ctx context.Context, tx pgx.Tx, selection crmcontracts.ReportingSelection) (Evaluation, error) {
	if err := auth.Require(ctx, "report_definition", principal.ActionRead); err != nil {
		return Evaluation{}, err
	}
	framework, err := s.framework(ctx, tx)
	if err != nil {
		return Evaluation{}, err
	}
	return s.evaluateSelection(ctx, tx, selection, framework)
}

// Calendar reads the installation's reporting zone, currency and fiscal start.
func (s *Service) Calendar(ctx context.Context, tx pgx.Tx) (Calendar, error) {
	return s.calendar(ctx, tx)
}

// Now is the service clock, so a reading built beside it cuts off where its
// evaluation does.
func (s *Service) Now() time.Time { return s.now() }
