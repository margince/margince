// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"maps"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/continuity"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func (s Server) jobsSummary(ctx context.Context) (crmcontracts.HealthSource, []crmcontracts.HealthFinding, error) {
	source := healthSource("jobs", healthReadObject)
	ws, ok := principal.WorkspaceID(ctx)
	if !ok {
		return source, nil, apperrors.ErrPermissionDenied
	}
	// The compact read omits the historical failure sample.
	rows, err := jobs.WorkspaceKindHealth(ctx, s.jobHealthHandlers.pool, ws.String(), dispatcherKinds())
	if err != nil {
		return source, nil, err
	}
	now := utcNow()
	source.ObservedAt = &now
	source.Condition, source.Code = healthUnknown, "queue_checked"
	source.Availability = healthUnobserved
	groups := map[jobHealthGroupKey]*jobHealthSummaryGroup{}
	for _, row := range rows {
		assessment := jobs.AssessHealth(row)
		if row.OldestWaitingAgeSeconds != nil && (source.OldestWaitSeconds == nil || int(*row.OldestWaitingAgeSeconds) > *source.OldestWaitSeconds) {
			source.OldestWaitSeconds = secondsOrAbsent(row.OldestWaitingAgeSeconds)
		}
		if assessment.Count == 0 || assessment.Condition == string(healthClear) {
			continue
		}
		switch assessment.Condition {
		case string(healthUnknown):
			source.Code = assessment.Code
		case string(healthProcessing):
			source.Condition, source.Code = healthProcessing, assessment.Code
			continue
		}
		scope := healthWorkspace
		if row.FleetWide {
			scope = "shared"
		}
		key := jobHealthGroupKey{assessment.Code, scope}
		if groups[key] == nil {
			groups[key] = &jobHealthSummaryGroup{kinds: map[string]bool{}}
		}
		groups[key].add(row, assessment)
	}
	source, findings := groupedJobHealth(source, groups)
	return source, findings, nil
}

type jobHealthGroupKey struct {
	code  string
	scope crmcontracts.HealthSourceScope
}

type jobHealthSummaryGroup struct {
	count      int
	assessment jobs.HealthAssessment
	retry      *time.Time
	kinds      map[string]bool
}

func (group *jobHealthSummaryGroup) add(row jobs.KindHealth, assessment jobs.HealthAssessment) {
	group.count += int(assessment.Count)
	group.kinds[row.Kind] = true
	group.assessment = assessment
	if assessment.Condition == string(healthRetrying) && row.NextAttemptAt != nil && (group.retry == nil || row.NextAttemptAt.Before(*group.retry)) {
		group.retry = row.NextAttemptAt
	}
}

func groupedJobHealth(source crmcontracts.HealthSource, groups map[jobHealthGroupKey]*jobHealthSummaryGroup) (crmcontracts.HealthSource, []crmcontracts.HealthFinding) {
	findings := []crmcontracts.HealthFinding{}
	for key, group := range groups {
		f := healthFinding(source, key.code, crmcontracts.HealthSourceCondition(group.assessment.Condition), group.count, "runs")
		f.Id += ":" + string(key.scope)
		f.Scope = crmcontracts.HealthFindingScope(key.scope)
		f.Urgency = group.assessment.Urgency
		f.NextAttemptAt = group.retry
		names := slices.Sorted(maps.Keys(group.kinds))
		truncated := len(names) > 8
		if truncated {
			names = names[:8]
		}
		f.JobKinds, f.JobKindsTruncated = &names, &truncated
		findings = append(findings, f)
	}
	if len(findings) > 0 {
		source.Availability = healthAvailable
		for _, finding := range findings {
			if finding.Condition == crmcontracts.HealthFindingConditionUnknown {
				source.Availability = healthUnobserved
				break
			}
		}
	}
	return source, findings
}

func (s Server) captureSummary(ctx context.Context) (crmcontracts.HealthSource, []crmcontracts.HealthFinding, error) {
	source := healthSource("capture", healthReadObject)
	now := utcNow()
	var categories capture.OperationalCategories
	var classifier crmcontracts.CaptureClassifierHealth
	var held crmcontracts.CaptureHeldMeetings
	var sweeps []crmcontracts.CaptureSweepHealth
	_, err := healthTx(ctx, s.captureHealthHandlers.pool, func(ctx context.Context, tx pgx.Tx) (bool, error) {
		var err error
		categories, err = capture.ReadOperationalCategoriesTx(ctx, tx)
		if err != nil {
			return false, err
		}
		classifier, err = captureClassifierHealth(ctx, tx, now)
		if err != nil {
			return false, err
		}
		held, err = captureHeldMeetings(ctx, tx, now)
		if err != nil {
			return false, err
		}
		sweeps, err = captureSweepHealth(ctx, tx)
		return err == nil, err
	})
	if err != nil {
		return source, nil, err
	}
	source.ObservedAt = &now
	source.Condition, source.Code = healthClear, "capture_checked"
	findings := []crmcontracts.HealthFinding{}
	add := func(code string, condition crmcontracts.HealthSourceCondition, n int, unit, responsible string) {
		if n == 0 {
			return
		}
		f := healthFinding(source, code, condition, n, unit)
		f.Responsible = crmcontracts.HealthFindingResponsible(responsible)
		if condition == healthProcessing {
			f.Urgency = 3
		}
		if condition == healthTechnical {
			f.Urgency = 0
		}
		if condition == healthWaitingHuman {
			f.Urgency = 1
		}
		findings = append(findings, f)
		source.Condition, source.Code = crmcontracts.HealthSourceCondition(condition), code
	}
	add("capture_processing", healthProcessing, classifier.Pending, "records", "operator")
	add("capture_uncertain", healthWaitingHuman, classifier.Unsure, "records", "mailbox_owner")
	add("capture_exhausted", healthTechnical, classifier.Exhausted, "records", "operator")
	add("capture_thread_processing", healthProcessing, categories.MachineThreads, "threads", "operator")
	add("capture_thread_uncertain", healthWaitingHuman, categories.UncertainThreads, "threads", "mailbox_owner")
	add("capture_owner_review", healthWaitingHuman, categories.OwnerReviewMailboxes, "mailboxes", "mailbox_owner")
	add("capture_unread", healthUnverified, categories.UnreadMailboxes, "mailboxes", "mailbox_owner")
	add("capture_held", healthProcessing, held.Count, "records", "operator")
	for _, sweep := range sweeps {
		if sweep.LastRun == nil {
			source.Availability = healthUnobserved
			continue
		}
		if sweep.LastRun.Outcome == crmcontracts.CaptureSweepRunOutcomeFailed || sweep.LastRun.Outcome == crmcontracts.CaptureSweepRunOutcomeSkipped {
			add("capture_repair_unverified", healthUnverified, 1, "runs", "operator")
			break
		}
	}
	if classifier.Exhausted > 0 {
		source.Condition, source.Code = healthTechnical, "capture_exhausted"
	}
	return source, findings, nil
}

func (s Server) intakeSummary(ctx context.Context) (crmcontracts.HealthSource, []crmcontracts.HealthFinding, error) {
	source := healthSource("intake", healthReadObject)
	now := utcNow()
	report, err := healthTx(ctx, s.extensionIngestHealthHandlers.pool, func(ctx context.Context, tx pgx.Tx) (crmcontracts.ExtensionIngestHealth, error) {
		return readExtensionIngestHealth(ctx, tx, now)
	})
	if err != nil {
		return source, nil, err
	}
	source.ObservedAt = &report.GeneratedAt
	source.Condition, source.Code = healthClear, "intake_no_refusals"
	start := extensionIngestWindowStart(now)
	source.WindowStart, source.WindowEnd = &start, &now
	n := 0
	for _, u := range report.Units {
		n += u.Refused
	}
	source.Count = &n
	unit := crmcontracts.HealthSourceUnit("records")
	source.Unit = &unit
	if n == 0 {
		return source, nil, nil
	}
	source.Condition, source.Code = healthUnverified, "intake_refused"
	f := healthFinding(source, source.Code, healthUnverified, n, "records")
	return source, []crmcontracts.HealthFinding{f}, nil
}

func (s Server) recoverySummary(ctx context.Context) (crmcontracts.HealthSource, []crmcontracts.HealthFinding, error) {
	source := healthSource("recovery", healthReadObject)
	now := utcNow()
	source.ObservedAt = &now
	drill, ever, err := s.ledger.Latest(ctx)
	if err != nil {
		return source, nil, err
	}
	source.Scope = healthInstallation
	source.Condition, source.Code = healthUnknown, "recovery_missing"
	source.Availability = healthUnobserved
	var evidence *continuity.Drill
	if ever {
		evidence = &drill
		source.ObservedAt = drill.FinishedAt
		if source.ObservedAt == nil {
			source.ObservedAt = &drill.StartedAt
		}
	}
	if !ever {
		return source, nil, nil
	}
	assessment := continuity.AssessDrill(evidence)
	source.Code = assessment.Code
	if assessment.Code == "recovery_verified" {
		source.Condition, source.Availability = healthClear, healthAvailable
		return source, nil, nil
	}
	if ever {
		source.Condition, source.Availability = healthUnverified, healthAvailable
	}
	f := healthFinding(source, source.Code, healthUnverified, 1, "runs")
	f.Count = nil
	f.Unit = nil
	return source, []crmcontracts.HealthFinding{f}, nil
}

func (s Server) automaticSummary(ctx context.Context) (crmcontracts.HealthSource, []crmcontracts.HealthFinding, error) {
	source := healthSource("automatic", healthReadObject)
	now := utcNow()
	var counts crmcontracts.AutomaticWorkCounts
	var n int
	var from time.Time
	_, err := healthTx(ctx, s.automaticWorkHandlers.pool, func(ctx context.Context, tx pgx.Tx) (bool, error) {
		var err error
		counts, n, from, err = readAutomaticWorkSummary(ctx, tx, now)
		return err == nil, err
	})
	if err != nil {
		return source, nil, err
	}
	source.ObservedAt = &now
	source.Condition, source.Code = healthClear, "automatic_checked"
	source.WindowStart, source.WindowEnd = &from, &now
	source.AutomaticWork = &counts
	if n == 0 {
		return source, nil, nil
	}
	source.Condition, source.Code = healthWaitingHuman, "automatic_routing"
	f := healthFinding(source, source.Code, healthWaitingHuman, n, "items")
	f.Responsible = crmcontracts.HealthFindingResponsibleAdministrator
	f.Urgency = 1
	return source, []crmcontracts.HealthFinding{f}, nil
}
