// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/buildinfo"
)

func (s Server) searchSummary(ctx context.Context) (crmcontracts.HealthSource, []crmcontracts.HealthFinding, error) {
	source := healthSource(healthSearchID, "embedding_reindex")
	source.Scope = healthInstallation
	if !s.serving() {
		source.Availability, source.Code = healthNotConfigured, "search_not_configured"
		return source, nil, nil
	}
	report, err := s.embedReindexHandlers.engine.statusEvidence(ctx)
	if err != nil {
		return source, nil, err
	}
	now := utcNow()
	source.ObservedAt = &now
	source.Availability, source.Condition, source.Code = healthUnobserved, healthUnknown, "search_unverified"
	if report.Status == "reembedding" {
		source.Condition, source.Code = healthProcessing, "search_processing"
	} else if report.ReindexNeeded {
		source.Condition, source.Code = healthWaitingHuman, "search_reindex"
	}
	if source.Condition == healthUnknown {
		return source, nil, nil
	}
	f := healthFinding(source, source.Code, source.Condition, report.EntitiesPending, "records")
	f.Count, f.Unit = nil, nil
	f.Responsible = crmcontracts.HealthFindingResponsibleAdministrator
	f.Urgency = 3
	if source.Condition == healthWaitingHuman {
		f.Urgency = 1
	}
	f.Action.Destination = crmcontracts.HealthActionDestinationSearch
	return source, []crmcontracts.HealthFinding{f}, nil
}

func (s Server) updatesSummary(ctx context.Context) (crmcontracts.HealthSource, []crmcontracts.HealthFinding, error) {
	source := healthSource("updates", "data_task")
	engine, err := newDataTaskEngine(s.dataTaskHandlers.pool, buildinfo.ReleaseVersion, s.dataTaskHandlers.now)
	if err != nil {
		return source, nil, err
	}
	rows, err := engine.List(ctx)
	if err != nil {
		return source, nil, err
	}
	now := s.dataTaskHandlers.now()
	source.ObservedAt = &now
	source.Condition, source.Code = healthClear, "updates_checked"
	counts := map[string]int{}
	conditions := map[string]crmcontracts.HealthSourceCondition{}
	attempts := map[string]*time.Time{}
	for _, row := range rows {
		code, condition, included := dataTaskHealth(row, now)
		if !included {
			continue
		}

		if code == "updates_retrying" && (attempts[code] == nil || row.NextAttemptAt.Before(*attempts[code])) {
			attempts[code] = row.NextAttemptAt
		}
		counts[code]++
		conditions[code] = condition
	}
	findings := []crmcontracts.HealthFinding{}
	for code, n := range counts {
		f := healthFinding(source, code, conditions[code], n, "updates")
		f.Action.Destination = "updates"
		f.NextAttemptAt = attempts[code]
		f.Responsible = crmcontracts.HealthFindingResponsibleAdministrator
		f.Urgency = 3
		if f.Condition == crmcontracts.HealthFindingConditionWaitingHuman {
			f.Urgency = 1
		}
		if f.Condition == crmcontracts.HealthFindingConditionTechnical {
			f.Urgency = 0
		}
		findings = append(findings, f)
		if source.Condition == healthClear || source.Condition == healthProcessing || f.Condition == crmcontracts.HealthFindingConditionTechnical {
			source.Condition, source.Code = crmcontracts.HealthSourceCondition(f.Condition), code
		}
	}
	return source, findings, nil
}

func (s Server) providersSummary(ctx context.Context) (crmcontracts.HealthSource, []crmcontracts.HealthFinding, error) {
	source := healthSource("providers", "ai_diagnostics")
	source.Scope = healthInstallation
	report, err := s.ReadProviderHealth(ctx)
	if err != nil {
		return source, nil, err
	}
	source.Availability, source.Condition, source.Code = healthUnobserved, healthUnknown, "providers_unobserved"
	if len(report.Configured) == 0 {
		source.Availability, source.Code = healthNotConfigured, "providers_not_configured"
		return source, nil, nil
	}
	if len(report.Providers) == 0 {
		return source, nil, nil
	}
	source.Code, source.Condition = "providers_blocked", healthTechnical
	f := healthFinding(source, source.Code, healthTechnical, len(report.Providers), "items")
	f.Count = nil
	f.Unit = nil
	f.Urgency = 0
	f.Responsible = crmcontracts.HealthFindingResponsibleAdministrator
	f.Action.Destination = "models"
	// The tracker dates the fault, not a successful availability observation.
	for _, entry := range report.Providers {
		if f.FirstObservedAt == nil || entry.Since.Before(*f.FirstObservedAt) {
			at := entry.Since
			f.FirstObservedAt = &at
		}
	}
	return source, []crmcontracts.HealthFinding{f}, nil
}

func dataTaskHealth(row datatask.Progress, now time.Time) (string, crmcontracts.HealthSourceCondition, bool) {
	if row.Version == 0 && row.BlockedReason == "review_required" {
		return "", "", false
	}
	switch {
	case row.Status == string(crmcontracts.DataTaskProgressStatusDone):
		return "", "", false
	case row.Status == string(crmcontracts.DataTaskProgressStatusFailed):
		return "updates_failed", healthTechnical, true
	case row.Status == "paused" || row.BlockedReason == "review_required" || row.BlockedReason == "missing_prerequisite" || row.BlockedReason == "newer_release_required":
		return "updates_review", healthWaitingHuman, true
	case row.NextAttemptAt != nil && row.NextAttemptAt.After(now):
		return "updates_retrying", healthRetrying, true
	default:
		return "updates_processing", healthProcessing, true
	}
}
