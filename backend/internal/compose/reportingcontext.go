// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

//craft:ignore naked-any pgx binds heterogeneous scalar types at the SQL boundary.
type reportingBindings struct{ values []any }

//craft:ignore naked-any pgx binds heterogeneous scalar types at the SQL boundary.
func (b *reportingBindings) arg(value any) int {
	b.values = append(b.values, value)
	return len(b.values)
}

//craft:ignore naked-any pgx binds heterogeneous scalar types at the SQL boundary.
func (b *reportingBindings) add(value any) string { return fmt.Sprintf("$%d", b.arg(value)) }

func reportingCalendar(ctx context.Context, tx pgx.Tx) (reporting.Calendar, error) {
	var out reporting.Calendar
	var err error
	out.Timezone, err = identity.TimezoneOf(ctx, tx)
	if err != nil {
		return out, err
	}
	out.Currency, err = identity.BaseCurrencyOf(ctx, tx)
	if err != nil {
		return out, err
	}
	out.FiscalStartMonth, err = identity.FiscalYearStartMonthOf(ctx, tx)
	return out, err
}

func reportingContext(ctx context.Context, tx pgx.Tx, selection crmcontracts.ReportingSelection, framework crmcontracts.ReportingFramework, at time.Time) (crmcontracts.ReportingContext, error) {
	out := crmcontracts.ReportingContext{EvaluatedAt: at, StateAt: at, FrameworkRevision: framework.Revision, DefinitionVersion: "1", PeriodKind: string(selection.Period), PipelineId: selection.PipelineId}
	calendar, err := reportingCalendar(ctx, tx)
	if err != nil {
		return out, err
	}
	out.Timezone = calendar.Timezone
	out.Currency = calendar.Currency
	out.Scope, err = (reportingAuthority{}).Scope(ctx, tx, selection.Scope, false)
	if err != nil {
		return out, err
	}
	if err := reportingPipeline(ctx, tx, selection.PipelineId); err != nil {
		return out, err
	}
	out.Interval, err = reporting.Interval(selection, calendar, at)
	if err != nil {
		return out, err
	}
	target, err := reporting.TargetWindow(out.Interval, string(selection.TargetBasis), calendar)
	if err != nil {
		return out, err
	}
	out.TargetInterval = &target
	switch selection.CloseWindow {
	case "all_open":
	case "fiscal_quarter":
		closeWindow, err := reporting.TargetWindow(crmcontracts.ReportingWindow{StartAt: at, EndAt: at.Add(time.Nanosecond)}, "fiscal_quarter", calendar)
		if err != nil {
			return out, err
		}
		out.CloseInterval = &closeWindow
	default:
		return out, fmt.Errorf("choose a supported expected-close window, all_open or fiscal_quarter: %w", apperrors.ErrInvalidArgument)
	}
	members, memberErr := reportingMembers(ctx, tx, out.Scope)
	err = memberErr
	for _, id := range members {
		out.MemberIds = append(out.MemberIds, openapi_types.UUID(id))
	}
	if err != nil {
		return out, err
	}
	out.PopulationFingerprint, err = reportingPopulationFingerprint(out.Scope, reportingMemberStrings(out))
	return out, err
}

func reportingPopulationFingerprint(scope crmcontracts.ReportingScope, members []string) (string, error) {
	raw, err := json.Marshal(struct {
		Kind    crmcontracts.ReportingScopeKind
		ID      *openapi_types.UUID
		Members []string
	}{Kind: scope.Kind, ID: scope.Id, Members: members})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

func reportingMemberStrings(frame crmcontracts.ReportingContext) []string {
	members := make([]string, len(frame.MemberIds))
	for i, id := range frame.MemberIds {
		members[i] = id.String()
	}
	return members
}

func newReportingService(pool *pgxpool.Pool, now func() time.Time) *reporting.Service {
	db := InstallationDB(pool)
	return reporting.NewService(db, metricEvaluator{forecast: forecasting.NewStore(db)}, reportingAuthority{users: identity.NewService(pool)}, reportingCalendar, now)
}
