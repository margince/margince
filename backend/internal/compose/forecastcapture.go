// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func (w *forecastSnapshotSweepWorker) freeze(ctx context.Context, ws ids.UUID) error {
	contexts := []crmcontracts.ReportingCaptureContext{{Scope: crmcontracts.ReportingScope{Kind: ScopeKindWorkspace}}}
	framework, frameworkErr := newReportingService(w.pool, w.now).GetFramework(ctx)
	if frameworkErr == nil {
		contexts = append(contexts, framework.Definition.CaptureContexts...)
	}
	if len(contexts) > 21 {
		return errors.New("forecast capture context limit exceeded")
	}
	store := newForecastStoreFor(w.pool)
	attempts, err := reportingCaptureAttempts(ctx, store, contexts)
	if err != nil {
		return err
	}
	sort.SliceStable(contexts, func(i, j int) bool {
		return attempts[reportingCaptureKey(contexts[i])].Before(attempts[reportingCaptureKey(contexts[j])])
	})
	language := textlang.Lang(identity.BaseLanguageForPrompt(ctx, w.pool))
	failures := []error{frameworkErr}
	seen := map[string]bool{}
	for _, capture := range contexts {
		if err := ctx.Err(); err != nil {
			return err
		}
		key := reportingCaptureKey(capture)
		if seen[key] {
			continue
		}
		seen[key] = true
		if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < 20*time.Second {
			return errors.Join(append(failures, errors.New("forecast capture paused at its time budget; the next pass resumes least-recently attempted contexts"))...)
		}
		captureCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		err := w.freezeContext(captureCtx, capture)
		cancel()
		statusErr := store.RecordCaptureStatus(ctx, forecasting.Scope{Kind: string(capture.Scope.Kind), ID: (*ids.UUID)(capture.Scope.Id)}, (*ids.UUID)(capture.PipelineId), w.now(), err == nil || alreadyFrozenToday(err), language)
		if statusErr != nil {
			return errors.Join(err, statusErr)
		}
		if alreadyFrozenToday(err) {
			w.log.DebugContext(ctx, "forecast context already captured today", "workspace_id", ws, "context", key)
			continue
		}
		if err != nil {
			failures = append(failures, fmt.Errorf("capture %s: %w", key, err))
		}
	}
	return errors.Join(failures...)
}

func reportingCaptureKey(capture crmcontracts.ReportingCaptureContext) string {
	return forecasting.CaptureKey(forecasting.Scope{Kind: string(capture.Scope.Kind), ID: (*ids.UUID)(capture.Scope.Id)}, (*ids.UUID)(capture.PipelineId))
}

func (w *forecastSnapshotSweepWorker) freezeContext(ctx context.Context, capture crmcontracts.ReportingCaptureContext) error {
	store := newForecastStoreFor(w.pool)
	at := w.now()
	return store.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		scope, err := (reportingAuthority{}).Scope(ctx, tx, capture.Scope, false)
		if err != nil {
			return err
		}
		if err := reportingPipeline(ctx, tx, capture.PipelineId); err != nil {
			return err
		}
		period, currency, err := ForecastPeriodAt(ctx, tx, forecasting.PeriodQuarter, at)
		if err != nil {
			return err
		}
		captured, err := captureReadings(ctx, tx, scope, (*ids.UUID)(capture.PipelineId), period, at, currency)
		if err != nil {
			return err
		}
		snapshot := forecasting.NewSnapshot{
			Period: period, Scope: captured.Scope,
			Trigger: forecasting.TriggerDaily, BaseCurrency: currency, Readings: captured.Readings, TakenAt: at,
			PipelineID: (*ids.UUID)(capture.PipelineId), PopulationFingerprint: captured.PopulationFingerprint,
		}
		if err := stampForecastBasis(ctx, tx, &snapshot); err != nil {
			return err
		}
		_, err = store.TakeSnapshot(ctx, tx, snapshot)
		return err
	})
}

// captureReadings is what a capture freezes: the readings over the scope's
// deals, the scope they resolved to, and the fingerprint of its members. The
// daily capture, a call's baseline and the comparison with it all read here.
// So a call is compared with today on one definition.
func captureReadings(
	ctx context.Context, tx pgx.Tx, scope crmcontracts.ReportingScope, pipeline *ids.UUID,
	period forecasting.Period, at time.Time, currency string,
) (forecasting.Baseline, error) {
	out := forecasting.Baseline{BaseCurrency: currency}
	requested := forecasting.Scope{Kind: string(scope.Kind), ID: (*ids.UUID)(scope.Id)}
	deals, resolved, _, err := forecastDealsForPipeline(ctx, tx, period, requested, at, currency, pipeline)
	if err != nil {
		return out, err
	}
	out.Scope = resolved
	if out.Readings, err = forecasting.Compute(period, period.LocalDay(at), deals); err != nil {
		return out, err
	}
	if out.ForwardMeasure, out.QualifiedPolicy, err = forecastBasis(ctx, tx); err != nil {
		return out, err
	}
	members, err := reportingMembers(ctx, tx, scope)
	if err != nil {
		return out, err
	}
	names := make([]string, len(members))
	for i, id := range members {
		names[i] = id.String()
	}
	out.PopulationFingerprint, err = reportingPopulationFingerprint(scope, names)
	return out, err
}

func reportingCaptureAttempts(ctx context.Context, store *forecasting.Store, contexts []crmcontracts.ReportingCaptureContext) (map[string]time.Time, error) {
	attempts := map[string]time.Time{}
	if err := store.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		for _, capture := range contexts {
			status, err := store.CaptureStatusTx(ctx, tx, forecasting.Scope{Kind: string(capture.Scope.Kind), ID: (*ids.UUID)(capture.Scope.Id)}, (*ids.UUID)(capture.PipelineId))
			if err != nil {
				return err
			}
			if status != nil {
				attempts[reportingCaptureKey(capture)] = status.LastAttemptAt
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return attempts, nil
}

// stampForecastBasis records on a snapshot the two settings that shaped what
// its reader saw. Both snapshot writers call it, so a vintage cannot be
// stamped one way by the daily pass and another way by the week's close.
func stampForecastBasis(ctx context.Context, tx pgx.Tx, snapshot *forecasting.NewSnapshot) error {
	measure, policy, err := forecastBasis(ctx, tx)
	snapshot.ForwardMeasure, snapshot.QualifiedPolicyVersion = measure, policy
	return err
}

// forecastBasis reads the forward measure and the qualified policy in force.
// A call's baseline carries the same two through forecasting.Baseline.
func forecastBasis(ctx context.Context, tx pgx.Tx) (forecasting.ForwardMeasure, string, error) {
	measure, err := ForecastForwardMeasure(ctx, tx)
	if err != nil {
		return "", "", err
	}
	policy, err := forecastQualifiedPolicy(ctx, tx)
	return measure, policy, err
}
