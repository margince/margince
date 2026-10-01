// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Catalog returns the metric vocabulary available to the current caller.
func (s *Service) Catalog(ctx context.Context) (crmcontracts.ReportingCatalog, error) {
	if err := auth.Require(ctx, "report_definition", principal.ActionRead); err != nil {
		return crmcontracts.ReportingCatalog{}, err
	}
	out, err := s.evaluator.Catalog(ctx)
	canSchedule := auth.Allows(ctx, "report_schedule", principal.ActionCreate) || auth.Allows(ctx, "report_schedule", principal.ActionUpdate)
	if err != nil || !canSchedule {
		return out, err
	}
	err = s.store.db.Tx(ctx, func(tx pgx.Tx) error {
		ready, err := scheduleReady(ctx, tx)
		out.ScheduleReady = &ready
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
	if err := s.validateSelection(ctx, selection); err != nil {
		return Evaluation{}, err
	}
	out, err := s.evaluator.Evaluate(ctx, tx, selection, framework, at)
	if err != nil {
		return Evaluation{}, err
	}
	if err := s.applyTargets(ctx, tx, &out.Result); err != nil {
		return Evaluation{}, err
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return Evaluation{}, err
	}
	digest := sha256.Sum256(raw)
	out.Result.EvaluationKey = hex.EncodeToString(digest[:])
	return out, nil
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
		selection, frameworkID, err := s.reportRevision(ctx, tx, report, chosen)
		if err != nil {
			return err
		}
		framework, err := s.frameworkRevision(ctx, tx, frameworkID)
		if err != nil {
			return err
		}
		out, err = s.evaluateSelection(ctx, tx, selection, framework)
		return err
	})
	return out.Result, err
}

func (s *Service) frameworkRevision(ctx context.Context, tx pgx.Tx, revision int64) (crmcontracts.ReportingFramework, error) {
	out := crmcontracts.ReportingFramework{Revision: revision, Version: revision, Definition: crmcontracts.ReportingFrameworkInput{Template: reportingSales, Qualification: []crmcontracts.ReportingQualification{}, CaptureContexts: []crmcontracts.ReportingCaptureContext{}}}
	if revision == 0 {
		return out, nil
	}
	var b bindings
	err := tx.QueryRow(ctx, "SELECT definition,effective_at FROM reporting_framework_revision WHERE revision="+b.add(revision), b.values...).Scan(&out.Definition, &out.EffectiveAt)
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
	matched := 0
	for _, fact := range facts {
		if fact.Metric != metric || fact.ContextID != contextID || !evidenceGroup(frame, fact, group) {
			continue
		}
		if through != nil && (fact.Row.OccurredAt == nil || !fact.Row.OccurredAt.Before(*through)) {
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
		out.Rows = append(out.Rows, fact.Row)
	}
	return out, nil
}

func evidenceGroup(frame crmcontracts.ReportingContext, fact Fact, group string) bool {
	if group == "" {
		return true
	}
	if strings.HasPrefix(group, "owner:") {
		return fact.OwnerID.String() == strings.TrimPrefix(group, "owner:")
	}
	if strings.HasPrefix(group, "stage:") {
		return fact.StageID == strings.TrimPrefix(group, "stage:")
	}
	if strings.HasPrefix(group, "week:") && fact.Row.OccurredAt != nil {
		zone, err := time.LoadLocation(frame.Timezone)
		if err != nil {
			return false
		}
		start, err := time.ParseInLocation(time.DateOnly, strings.TrimPrefix(group, "week:"), zone)
		if err != nil {
			return false
		}
		days := 7 - (int(start.Weekday())+6)%7
		return !fact.Row.OccurredAt.Before(start) && fact.Row.OccurredAt.Before(start.AddDate(0, 0, days))
	}
	return fact.GroupKey == group
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
		return Evaluation{}, apperrors.ErrVersionSkew
	}
	return evaluation, nil
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
