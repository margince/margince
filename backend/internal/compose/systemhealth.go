// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"log/slog"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	healthAvailable     = crmcontracts.HealthSourceAvailabilityAvailable
	healthWithheld      = crmcontracts.HealthSourceAvailabilityWithheld
	healthNotConfigured = crmcontracts.HealthSourceAvailabilityNotConfigured
	healthNotMonitored  = crmcontracts.HealthSourceAvailabilityNotMonitored
	healthUnobserved    = crmcontracts.HealthSourceAvailabilityUnknown
	healthReadFailed    = crmcontracts.HealthSourceAvailabilityFailed
	healthTechnical     = crmcontracts.HealthSourceConditionTechnical
	healthWaitingHuman  = crmcontracts.HealthSourceConditionWaitingHuman
	healthUnverified    = crmcontracts.HealthSourceConditionUnverified
	healthProcessing    = crmcontracts.HealthSourceConditionProcessing
	healthRetrying      = crmcontracts.HealthSourceConditionRetrying
	healthClear         = crmcontracts.HealthSourceConditionClear
	healthUnknown       = crmcontracts.HealthSourceConditionUnknown
	healthWorkspace     = crmcontracts.HealthSourceScopeWorkspace
	healthInstallation  = crmcontracts.HealthSourceScopeInstallation
	healthReadObject    = "job_health"
	healthSearchID      = string(crmcontracts.HealthActionDestinationSearch)
)

type healthRead struct {
	id, object string
	read       func(context.Context) (crmcontracts.HealthSource, []crmcontracts.HealthFinding, error)
}

// GetSystemHealth never starts work or probes while reporting operational state.
func (s Server) GetSystemHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman {
		httperr.Write(w, r, apperrors.ErrPermissionDenied)
		return
	}
	if err := auth.RequireHuman(ctx); err != nil {
		httperr.Write(w, r, err)
		return
	}
	admitted := auth.Require(ctx, healthReadObject, principal.ActionRead) == nil ||
		auth.Require(ctx, "data_task", principal.ActionRead) == nil ||
		auth.Require(ctx, "embedding_reindex", principal.ActionRead) == nil ||
		auth.Require(ctx, "installation_settings", principal.ActionUpdate) == nil
	if !admitted {
		httperr.Write(w, r, apperrors.ErrPermissionDenied)
		return
	}
	ctx, cancel := context.WithTimeout(ctx, healthReadTimeout)
	defer cancel()
	out := composeHealth(ctx, s.healthReads(), utcNow)
	if s.originProbe != nil && auth.Require(ctx, healthReadObject, principal.ActionRead) == nil {
		origin := s.originProbe.Snapshot()
		out.PublicOrigin = &crmcontracts.PublicOriginStatus{Origin: origin.Origin, Reachable: origin.Reachable, CheckedAt: origin.CheckedAt}
		if origin.Detail != "" {
			out.PublicOrigin.Detail = &origin.Detail
		}
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

func composeHealth(ctx context.Context, reads []healthRead, now func() time.Time) crmcontracts.SystemHealth {
	out := crmcontracts.SystemHealth{
		GeneratedAt: now(), Assessment: crmcontracts.SystemHealthAssessmentChecked, Complete: true,
		Sources: []crmcontracts.HealthSource{}, Findings: []crmcontracts.HealthFinding{},
	}
	for _, result := range readHealthSources(ctx, reads) {
		source := result.source
		if source.Availability == healthWithheld {
			out.Sources = append(out.Sources, source)
			continue
		}
		source, findings := gradeHealthRead(ctx, result)
		out.Findings = append(out.Findings, findings...)
		if !healthSourceComplete(source) {
			out.Complete = false
		}
		if source.Condition == healthTechnical || source.Condition == healthUnverified || source.Condition == healthWaitingHuman {
			out.Assessment = crmcontracts.SystemHealthAssessmentAttention
		}
		out.Sources = append(out.Sources, source)
	}
	for _, finding := range out.Findings {
		if finding.RequiresAction {
			out.ActionCount++
		}
	}

	if len(out.Sources) == 0 || allHealthWithheld(out.Sources) {
		out.Complete = false
	}
	if out.ActionCount > 0 {
		out.Assessment = crmcontracts.SystemHealthAssessmentAttention
	}
	if !out.Complete && out.Assessment != crmcontracts.SystemHealthAssessmentAttention {
		out.Assessment = crmcontracts.SystemHealthAssessmentIncomplete
	}
	sort.SliceStable(out.Findings, func(i, j int) bool { return healthFindingLess(out.Findings[i], out.Findings[j]) })
	return out
}

func gradeHealthRead(ctx context.Context, result healthReadResult) (crmcontracts.HealthSource, []crmcontracts.HealthFinding) {
	var source crmcontracts.HealthSource = result.source //nolint:staticcheck // QF1011: the Problem-code census needs the explicit carrier type.
	if result.err != nil {
		slog.ErrorContext(ctx, "system health source failed", "source", source.Id, "err", result.err)
		source.Availability, source.Code = healthReadFailed, "read_failed"
		finding := healthFinding(source, "read_failed", healthUnknown, 0, "items")
		finding.RequiresAction = true
		finding.Count, finding.Unit = nil, nil
		return source, []crmcontracts.HealthFinding{finding}
	}
	for _, finding := range result.findings {
		priority := healthConditionPriority(crmcontracts.HealthSourceCondition(finding.Condition))
		current := healthConditionPriority(source.Condition)
		if priority < current || (priority == current && finding.Code < source.Code) {
			source.Condition = crmcontracts.HealthSourceCondition(finding.Condition)
			source.Code = finding.Code
		}
	}
	return source, result.findings
}

func healthSourceComplete(source crmcontracts.HealthSource) bool {
	switch source.Availability {
	case healthNotConfigured, healthNotMonitored:
		return true
	case healthAvailable:
		return source.Condition != healthUnknown && source.Condition != ""
	default:
		return false
	}
}

func healthSource(id, object string) crmcontracts.HealthSource {
	return crmcontracts.HealthSource{Id: id, Permission: object + ":read", Scope: healthWorkspace, Availability: healthAvailable, Condition: healthUnknown, Code: string(healthUnknown)}
}

func healthFinding(source crmcontracts.HealthSource, code string, condition crmcontracts.HealthSourceCondition, count int, unit string) crmcontracts.HealthFinding {
	u := crmcontracts.HealthFindingUnit(unit)
	return crmcontracts.HealthFinding{
		Id: source.Id + ":" + code, Source: source.Id, Code: code,
		Condition: crmcontracts.HealthFindingCondition(condition), Count: &count, Unit: &u, Scope: crmcontracts.HealthFindingScope(source.Scope),
		ObservedAt: source.ObservedAt, WindowStart: source.WindowStart, WindowEnd: source.WindowEnd,
		Impact: "possible", Responsible: "operator", Recovery: source.Id, Urgency: 2,
		RequiresAction: condition == healthTechnical || condition == healthWaitingHuman || condition == healthUnverified,
		Action:         crmcontracts.HealthAction{Destination: "diagnostics", Available: true},
	}
}

func healthTx[T any](ctx context.Context, pool *pgxpool.Pool, read func(context.Context, pgx.Tx) (T, error)) (T, error) {
	var out T
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error { var err error; out, err = read(ctx, tx); return err })
	return out, err
}

func (s Server) healthReads() []healthRead {
	return []healthRead{
		{"jobs", healthReadObject, s.jobsSummary},
		{"capture", healthReadObject, s.captureSummary},
		{"intake", healthReadObject, s.intakeSummary},
		{"recovery", healthReadObject, s.recoverySummary},
		{"automatic", healthReadObject, s.automaticSummary},
		{healthSearchID, "embedding_reindex", s.searchSummary},
		{"updates", "data_task", s.updatesSummary},
		{"providers", "ai_diagnostics", s.providersSummary},
	}
}

func healthConditionPriority(condition crmcontracts.HealthSourceCondition) int {
	switch condition {
	case healthTechnical:
		return 0
	case healthUnverified:
		return 1
	case healthWaitingHuman:
		return 2
	case healthRetrying:
		return 3
	case healthUnknown:
		return 4
	case healthProcessing:
		return 5
	default:
		return 6
	}
}

func healthFindingLess(a, b crmcontracts.HealthFinding) bool {
	if a.Urgency != b.Urgency {
		return a.Urgency < b.Urgency
	}
	if a.Source != b.Source {
		return a.Source < b.Source
	}
	if a.Scope != b.Scope {
		return a.Scope < b.Scope
	}
	unit := func(f crmcontracts.HealthFinding) string {
		if f.Unit == nil {
			return ""
		}
		return string(*f.Unit)
	}
	if unit(a) != unit(b) {
		return unit(a) < unit(b)
	}
	if (a.Count == nil) != (b.Count == nil) {
		return a.Count != nil
	}
	if a.Count != nil && b.Count != nil && *a.Count != *b.Count {
		return *a.Count > *b.Count
	}
	if a.FirstObservedAt != nil && b.FirstObservedAt != nil && !a.FirstObservedAt.Equal(*b.FirstObservedAt) {
		return a.FirstObservedAt.Before(*b.FirstObservedAt)
	}
	if (a.FirstObservedAt == nil) != (b.FirstObservedAt == nil) {
		return a.FirstObservedAt != nil
	}
	return a.Id < b.Id
}

type healthReadResult struct {
	source   crmcontracts.HealthSource
	findings []crmcontracts.HealthFinding
	err      error
}

func readHealthSources(ctx context.Context, reads []healthRead) []healthReadResult {
	results := make([]healthReadResult, len(reads))
	var pending sync.WaitGroup
	for i, read := range reads {
		source := healthSource(read.id, read.object)
		results[i].source = source
		if !auth.Allows(ctx, read.object, principal.ActionRead) {
			source.Availability, source.Code = healthWithheld, string(healthWithheld)
			results[i].source = source
			continue
		}
		pending.Go(func() {
			source, findings, err := read.read(ctx)
			if err == nil {
				results[i].source = source
			}
			results[i].findings, results[i].err = findings, err
		})
	}
	pending.Wait()
	return results
}

func allHealthWithheld(sources []crmcontracts.HealthSource) bool {
	for _, source := range sources {
		if source.Availability != healthWithheld {
			return false
		}
	}
	return true
}
