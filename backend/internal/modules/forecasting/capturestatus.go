// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// CaptureKey separates pipeline captures from whole-scope captures.
func CaptureKey(scope Scope, pipeline *ids.UUID) string {
	owner, pipe := "", ""
	if scope.ID != nil {
		owner = scope.ID.String()
	}
	if pipeline != nil {
		pipe = pipeline.String()
	}
	return scope.Kind + ":" + owner + ":" + pipe
}

// CaptureStatusTx reads capture availability within the caller’s transaction.
func (s *Store) CaptureStatusTx(ctx context.Context, tx pgx.Tx, scope Scope, pipeline *ids.UUID) (*crmcontracts.ReportingCaptureStatus, error) {
	if err := auth.Require(ctx, "forecast", principal.ActionRead); err != nil {
		return nil, err
	}
	if err := checkScope(scope); err != nil {
		return nil, err
	}
	if err := requireScopeAuthority(ctx, scope); err != nil {
		return nil, err
	}
	_, out, err := readCaptureStatus(ctx, tx, CaptureKey(scope, pipeline), false)
	return out, err
}

func readCaptureStatus(ctx context.Context, tx pgx.Tx, key string, lock bool) (ids.UUID, *crmcontracts.ReportingCaptureStatus, error) {
	var id ids.UUID
	var out crmcontracts.ReportingCaptureStatus
	query := "SELECT id,last_attempt_at,last_success_at,failure,next_capture_at FROM forecast_capture_status WHERE context_key=" + storekit.Placeholders([]string{key})
	if lock {
		query += " FOR UPDATE"
	}
	err := tx.QueryRow(ctx, query, key).Scan(&id, &out.LastAttemptAt, &out.LastSuccessAt, &out.Failure, &out.NextCaptureAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.Nil, nil, nil
	}
	return id, &out, err
}

// RecordCaptureStatus records attempts independently of successful snapshot creation.
func (s *Store) RecordCaptureStatus(ctx context.Context, scope Scope, pipeline *ids.UUID, at time.Time, success bool) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	if err := requireForecastScope(ctx, scope); err != nil {
		return err
	}
	return s.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		key := CaptureKey(scope, pipeline)
		// The key lock also serializes the first insert, where no row exists to lock.
		lockKey := "forecast_capture_status:" + key
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended("+storekit.Placeholders([]string{lockKey})+",0))", lockKey); err != nil {
			return err
		}
		id, before, err := readCaptureStatus(ctx, tx, key, true)
		if err != nil {
			return err
		}
		after := crmcontracts.ReportingCaptureStatus{LastAttemptAt: at, NextCaptureAt: at.Add(24 * time.Hour)}
		action := "update"
		if before == nil {
			id = ids.NewV7()
			action = "create"
		} else {
			after.LastSuccessAt = before.LastSuccessAt
		}
		if success {
			after.LastSuccessAt, err = latestDailyCapture(ctx, tx, scope, pipeline, at)
			if err != nil {
				return err
			}
		} else {
			reason := "Capture failed. Check the configured team and pipeline, then retry the forecast capture job."
			after.Failure = &reason
		}
		//craft:ignore naked-any pgx arguments bind the typed capture metadata.
		args := []any{id, key, after.LastAttemptAt, after.LastSuccessAt, after.Failure, after.NextCaptureAt}
		p := strings.Split(storekit.Placeholders(args), ",")
		if _, err := tx.Exec(ctx, "INSERT INTO forecast_capture_status(id,context_key,last_attempt_at,last_success_at,failure,next_capture_at) VALUES ("+strings.Join(p, ",")+") ON CONFLICT(context_key) DO UPDATE SET last_attempt_at=EXCLUDED.last_attempt_at,last_success_at=EXCLUDED.last_success_at,failure=EXCLUDED.failure,next_capture_at=EXCLUDED.next_capture_at", args...); err != nil {
			return err
		}
		audit, err := storekit.Audit(ctx, tx, action, "forecast_capture_status", id, before, after)
		if err != nil {
			return err
		}
		return storekit.Emit(ctx, tx, audit, "reporting.changed", "forecast_capture_status", id, crmcontracts.InternalEventReportingChanged{Object: "forecast_capture_status", Action: action})
	})
}

func latestDailyCapture(ctx context.Context, tx pgx.Tx, scope Scope, pipeline *ids.UUID, at time.Time) (*time.Time, error) {
	//craft:ignore naked-any pgx binds nullable UUIDs and timestamps at the SQL boundary.
	args := []any{scope.Kind, scope.ID, pipeline, at, TriggerDaily}
	p := strings.Split(storekit.Placeholders(args), ",")
	var captured *time.Time
	err := tx.QueryRow(ctx, "SELECT max(taken_at) FROM forecast_snapshot WHERE scope_kind="+p[0]+" AND scope_id IS NOT DISTINCT FROM "+p[1]+"::uuid AND pipeline_id IS NOT DISTINCT FROM "+p[2]+"::uuid AND taken_at<="+p[3]+" AND trigger="+p[4], args...).Scan(&captured)
	return captured, err
}
