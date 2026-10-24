// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
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

func errUnsupportedCloseWindow() error {
	return fmt.Errorf("choose a supported expected-close window, all_open or fiscal_quarter: %w", apperrors.ErrInvalidArgument)
}

func reportingContext(ctx context.Context, tx pgx.Tx, selection crmcontracts.ReportingSelection, framework crmcontracts.ReportingFramework, at time.Time) (crmcontracts.ReportingContext, error) {
	out := crmcontracts.ReportingContext{EvaluatedAt: at, StateAt: at, FrameworkRevision: framework.Revision, DefinitionVersion: "1", PeriodKind: string(selection.Period), PipelineId: selection.PipelineId}
	calendar, err := reportingCalendar(ctx, tx)
	if err != nil {
		return out, err
	}
	language := string(identity.BaseLanguageForRecord(ctx, tx))
	out.ProseLanguage = &language
	out.Timezone = calendar.Timezone
	out.Currency = calendar.Currency
	out.Scope, err = (reportingAuthority{}).Scope(ctx, tx, selection.Scope, false)
	if err != nil {
		return out, err
	}
	holds, err := scopeHoldsDeals(ctx, tx, reportingRequested(out.Scope), unownedIsExcluded)
	if err != nil {
		return out, err
	}
	out.ScopeHoldsDeals = &holds
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
		return out, errUnsupportedCloseWindow()
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
	if err != nil {
		return out, err
	}
	out.MemberIds, err = measurableMemberIDs(ctx, tx, out.MemberIds)
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
	return reporting.NewService(db, metricEvaluator{forecast: newForecastStore(db)}, reportingAuthority{users: identity.NewService(pool)}, reportingCalendar, now).
		WithBaseLanguage(identity.BaseLanguageForRecord).
		WithQuestions(reportingQuestions{floor: analyticsquery.DefaultFloor, names: newAttentionNames(db)})
}

// measurableMemberIDs narrows a scope's members to the owners the reader may
// measure one by one, so owner targets and owner lists name no one else.
func measurableMemberIDs(ctx context.Context, tx pgx.Tx, scopeMembers []openapi_types.UUID) ([]openapi_types.UUID, error) {
	actor, ok := principal.Actor(ctx)
	if !ok {
		return nil, apperrors.ErrPermissionDenied
	}
	if auth.Unbounded(actor) || actor.Permissions.RowScope == principal.RowScopeAll {
		return scopeMembers, nil
	}
	measurable, err := (reportingAuthority{}).Members(ctx, tx)
	if err != nil {
		return nil, err
	}
	out := make([]openapi_types.UUID, 0, len(scopeMembers))
	for _, id := range scopeMembers {
		if slices.Contains(measurable, ids.UUID(id)) {
			out = append(out, id)
		}
	}
	return out, nil
}
