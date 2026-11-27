// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/salesprogress"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestTargetAndProgressRefuseAnOwnerOutsideTheReadersPopulation(t *testing.T) {
	e := setupForecast(t)
	at := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	service := newReportingService(e.Pool, func() time.Time { return at })
	ctx := reportingRepSeat(e)
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("missing test actor")
	}
	actor.Permissions.Objects["sales_target"] = principal.ObjectGrant{Read: true, Create: true, Update: true}
	ctx = principal.WithActor(ctx, actor)
	input := crmcontracts.ReportingTargetInput{Metric: "bookings_won", Scope: crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(e.Rep3)}, PipelineId: ptrUUID(e.pipeline), PeriodKind: "month", PeriodStart: openapi_types.Date{Time: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}, Value: 100, Reason: "Outside my population"}
	if _, err := service.CreateTarget(ctx, input); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("target for another owner: %v", err)
	}
	rec := httptest.NewRecorder()
	kind := crmcontracts.GetSalesProgressParamsScopeKindOwner
	(salesprogress.Reading{DB: InstallationDB(e.Pool), Reports: service}).GetSalesProgress(rec, httptest.NewRequest(http.MethodGet, "/analytics/progress", nil).WithContext(ctx), crmcontracts.GetSalesProgressParams{Period: "year", Year: 2026, ScopeKind: &kind, ScopeId: ptrUUID(e.Rep3)})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("progress for another owner: %d %s", rec.Code, rec.Body.String())
	}
	actor.Permissions.RowScope = principal.RowScopeAll
	broader := principal.WithActor(ctx, actor)
	roster, err := service.ListGoalSellers(broader, 2026)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, member := range roster.Data {
		if member.OwnerId == *input.Scope.Id {
			found = true
			if member.CanEdit {
				t.Fatal("non-leader may edit a colleague's target")
			}
		}
	}
	if !found {
		t.Fatal("reader's roster omitted the colleague they may measure")
	}
	if _, err := service.CreateTarget(broader, input); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("outside write reach: %v", err)
	}
	rec = httptest.NewRecorder()
	(salesprogress.Reading{DB: InstallationDB(e.Pool), Reports: service}).GetSalesProgress(rec, httptest.NewRequest(http.MethodGet, "/analytics/progress", nil).WithContext(broader), crmcontracts.GetSalesProgressParams{Period: "year", Year: 2026, ScopeKind: &kind, ScopeId: ptrUUID(e.Rep3)})
	if rec.Code != http.StatusOK {
		t.Fatalf("colleague inside read reach: %d %s", rec.Code, rec.Body.String())
	}
}
