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
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestReportingTeamTargetRemainsIndependentOfOwnerAllocations(t *testing.T) {
	e := setupForecast(t)
	at := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	service := newReportingService(e.Pool, func() time.Time { return at })
	ctx := reportingActor(e)
	input := crmcontracts.ReportingTargetInput{Metric: "bookings_won", Scope: crmcontracts.ReportingScope{Kind: "team", Id: ptrUUID(e.Team1)}, PeriodKind: "month", PeriodStart: openapi_types.Date{Time: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}, Value: 30000000, Reason: "Team commitment"}
	team, err := service.CreateTarget(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	input.Scope = crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(e.Rep1)}
	input.Value = 22000000
	owner, err := service.CreateTarget(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateTarget(ctx, input); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("duplicate target: %v", err)
	}
	target, err := service.GetTarget(ctx, ids.UUID(team.Id))
	if err != nil {
		t.Fatal(err)
	}
	if target.Definition.Value != 30000000 || target.AllocatedValue == nil || *target.AllocatedValue != 22000000 || target.AllocationDifference == nil || *target.AllocationDifference != 8000000 {
		t.Fatalf("independent allocation: %+v", target)
	}
	input.Value = 35000000
	input.Reason = "Revised owner commitment"
	if _, err := service.UpdateTarget(ctx, ids.UUID(owner.Id), owner.Version, input); err != nil {
		t.Fatal(err)
	}
	list, err := service.ListTargets(ctx, nil, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range list.Data {
		if target.Id != team.Id {
			continue
		}
		if target.Definition.Value != 30000000 || target.AllocationDifference == nil || *target.AllocationDifference != -5000000 {
			t.Fatalf("team target changed with allocation: %+v", target)
		}
		return
	}
	t.Fatal("team target missing from list")
}
