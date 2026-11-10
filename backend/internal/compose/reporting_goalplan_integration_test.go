// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func goalMonths(value int64) crmcontracts.SalesGoalMonths {
	months := make(crmcontracts.SalesGoalMonths, 12)
	for index := range months {
		amount := value
		months[index] = &amount
	}
	return months
}

func ownerMonthTargets(t *testing.T, service *reporting.Service, f *forecastEnv) []crmcontracts.ReportingTarget {
	t.Helper()
	live := false
	list, err := service.ListTargets(reportingActor(f), nil, 100, reporting.TargetFilter{Retired: &live})
	if err != nil {
		t.Fatal(err)
	}
	var out []crmcontracts.ReportingTarget
	for _, target := range list.Data {
		scope := target.Definition.Scope
		if scope.Kind == "owner" && scope.Id != nil && *scope.Id == openapi_types.UUID(f.Rep3) {
			out = append(out, target)
		}
	}
	return out
}

func TestAPublishedGoalPlanOwnsItsTwelveMonthsUntilWithdrawn(t *testing.T) {
	e := setupForecast(t)
	service := newReportingService(e.Pool, func() time.Time { return time.Date(2026, 2, 10, 12, 0, 0, 0, time.UTC) })
	ctx := reportingActor(e)
	declared := int64(12000000)
	draft, err := service.SaveGoalDraft(ctx, e.Rep3, 2026, 0, crmcontracts.SalesGoalDraftInput{Months: goalMonths(1000000), DeclaredTotalMinor: &declared, Reason: "Agreed in the yearly review"})
	if err != nil {
		t.Fatal(err)
	}
	if got := ownerMonthTargets(t, service, e); len(got) != 0 {
		t.Fatalf("a draft wrote %d targets, want none", len(got))
	}
	published, err := service.PublishGoalPlan(ctx, e.Rep3, 2026, draft.Version, crmcontracts.SalesGoalPublishInput{ConfirmAgreed: true, Reason: "Agreed in the yearly review"})
	if err != nil {
		t.Fatal(err)
	}
	if published.State != "published" || len(ownerMonthTargets(t, service, e)) != 12 {
		t.Fatalf("published plan = %s with %d monthly targets, want published with 12", published.State, len(ownerMonthTargets(t, service, e)))
	}
	march := crmcontracts.ReportingTargetInput{Metric: "bookings_won", Scope: crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(e.Rep3)}, PeriodKind: "month", PeriodStart: openapi_types.Date{Time: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)}, Value: 1, Reason: "Side change"}
	if _, err := service.CreateTarget(ctx, march); !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Fatalf("a target write to a plan-owned month = %v, want a refusal", err)
	}
	withdrawn, err := service.WithdrawGoalPlan(ctx, e.Rep3, 2026, published.Version, "Seller moved to another team")
	if err != nil {
		t.Fatal(err)
	}
	if withdrawn.State != "withdrawn" || len(ownerMonthTargets(t, service, e)) != 0 {
		t.Fatalf("withdrawn plan = %s with %d live targets, want withdrawn with none", withdrawn.State, len(ownerMonthTargets(t, service, e)))
	}
	retired := true
	list, err := service.ListTargets(ctx, nil, 100, reporting.TargetFilter{Retired: &retired, PeriodStart: &march.PeriodStart})
	if err != nil || len(list.Data) != 1 {
		t.Fatalf("retired March targets = %d (%v), want the one the plan wrote", len(list.Data), err)
	}
	if _, err := service.UpdateTarget(ctx, ids.UUID(list.Data[0].Id), list.Data[0].Version, march); err != nil {
		t.Fatalf("a withdrawn plan still holds its months: %v", err)
	}
}

func TestADraftThatDoesNotAddUpToTheYearIsNotPublished(t *testing.T) {
	e := setupForecast(t)
	service := newReportingService(e.Pool, func() time.Time { return time.Date(2026, 2, 10, 12, 0, 0, 0, time.UTC) })
	ctx := reportingActor(e)
	declared := int64(13000000)
	draft, err := service.SaveGoalDraft(ctx, e.Rep3, 2026, 0, crmcontracts.SalesGoalDraftInput{Months: goalMonths(1000000), DeclaredTotalMinor: &declared, Reason: "First proposal"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.PublishGoalPlan(ctx, e.Rep3, 2026, draft.Version, crmcontracts.SalesGoalPublishInput{ConfirmAgreed: true, Reason: "Agreed"}); !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Fatalf("publishing a draft off its yearly total = %v, want a refusal", err)
	}
	if got := ownerMonthTargets(t, service, e); len(got) != 0 {
		t.Fatalf("a refused publication wrote %d targets", len(got))
	}
	discarded, err := service.DiscardGoalDraft(ctx, e.Rep3, 2026, draft.Version)
	if err != nil || discarded.Draft != nil || discarded.History[len(discarded.History)-1].Kind != "discarded" {
		t.Fatalf("discarded plan = %+v (%v), want no draft and a discard in the history", discarded, err)
	}
}
