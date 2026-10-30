// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAnOwnedDealBeyondOtherOwnersScanBudgetStillReachesHome(t *testing.T) {
	e := integration.Setup(t)
	pipeline, stage, _ := integration.DealFixture(t, e)
	at := clocktest.Now(t)
	own := e.SeedDeal(t, "A recorded recovery decision", pipeline, stage, &e.Rep1)
	unownedDeal, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "Work without an owner", PipelineID: pipeline, StageID: stage, OwnerExact: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	unowned := ids.UUID(unownedDeal.Id)
	former := e.SeedDeal(t, "Recovery needing a new owner", pipeline, stage, &e.Rep2)
	if err := identity.NewService(e.Pool).DeactivateUser(e.Admin(), adminIdentity(e), identity.DeactivateUserInput{UserID: ids.From[ids.UserKind](e.Rep2)}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []ids.UUID{own, unowned, former} {
		e.WsExec(t, `UPDATE deal SET created_at = $2, last_activity_at = $2 WHERE id = $1`, id, at.AddDate(0, 0, -90))
	}
	for i := range slippingScanLimit + 1 {
		id := e.SeedDeal(t, fmt.Sprintf("Another owner's recent candidate %d", i), pipeline, stage, &e.Rep3)
		e.WsExec(t, `UPDATE deal SET created_at = $2, last_activity_at = $2 WHERE id = $1`, id, at.AddDate(0, 0, -21))
	}
	unscoped, cut, err := quietDealScanWithClock(e.Pool, 19, func() time.Time { return at })(e.Admin(), nil)
	if err != nil || !cut {
		t.Fatalf("unscoped flood fixture was not bounded: cut=%v, err=%v", cut, err)
	}
	for _, row := range unscoped {
		if row.DealID == own || row.DealID == unowned {
			t.Fatal("fixture did not place older work beyond the workspace scan")
		}
	}
	service := newAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return at })
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	ownPerms := integration.AdminPerms
	ownPerms.RoleKeys = []string{"custom"}
	ownPerms.RowScope = principal.RowScopeOwn
	teamPerms := ownPerms
	teamPerms.RowScope = principal.RowScopeTeam
	checks := []struct {
		name, scope string
		ctx         context.Context
		owner       ids.UUID
		want        ids.UUID
		count       int
	}{
		{"mine", "mine", reader, ids.Nil, own, 1},
		{"own row scope", "mine", e.As(e.Rep1, []ids.UUID{e.Team1}, ownPerms), ids.Nil, own, 1},
		{"team row scope", "team", e.As(e.Rep1, []ids.UUID{e.Team1}, teamPerms), ids.Nil, own, 3},
		{"named owner", "mine", e.Admin(), e.Rep1, own, 1},
		{"team", "team", reader, ids.Nil, own, 3},
		{"unassigned", "unassigned", reader, ids.Nil, unowned, 1},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			scope := crmcontracts.GetWorklistParamsScope(check.scope)
			params := crmcontracts.GetWorklistParams{Scope: &scope}
			if !check.owner.IsZero() {
				owner := openapi_types.UUID(check.owner)
				params.Owner = &owner
			}
			response := httptest.NewRecorder()
			attention.NewHandlers(service).GetWorklist(response, httptest.NewRequest(http.MethodGet, "/v1/worklist", nil).WithContext(check.ctx), params)
			if response.Code != http.StatusOK {
				t.Fatalf("worklist status %d: %s", response.Code, response.Body.String())
			}
			var day crmcontracts.Worklist
			if err := json.Unmarshal(response.Body.Bytes(), &day); err != nil {
				t.Fatal(err)
			}
			found := false
			for _, row := range day.Queue {
				found = found || row.Source == "deal_at_risk" && row.Id == check.want.String()
				if row.Source == "deal_at_risk" && row.Id != check.want.String() && (check.scope != "team" || row.Id != unowned.String() && row.Id != former.String()) {
					t.Fatalf("another owner's work leaked into %s: %s", check.name, row.Id)
				}
			}
			if !found || len(day.Queue) != check.count || day.Focus == nil || day.Focus.Total != check.count || day.Summary.Total != check.count {
				t.Fatalf("owned recovery work missing from queue/counts/Focus: %+v", day)
			}
			for _, reach := range day.Reach {
				if reach.Source == "deal_at_risk" && (reach.MoreAvailable || reach.Considered != check.count || reach.Shown != check.count) {
					t.Fatal("other owners exhausted this scope's scan budget")
				}
			}
		})
	}
	board, err := service.TeamBoard(reader)
	if err != nil || board.Truncated || board.Unassigned.AtRisk != 1 {
		t.Fatalf("board risk scope: %+v, %v", board, err)
	}
	for _, member := range board.Members {
		if ids.UUID(member.UserId) == e.Rep1 && member.Counts.AtRisk != 1 {
			t.Fatalf("board lost the reader's recovery: %+v", member)
		}
	}
	if _, err := service.Worklist(context.Background(), "mine", "", ids.Nil, 10, ""); err == nil {
		t.Fatal("a missing principal was served an empty worklist")
	}
	restricted := e.As(e.Rep1, nil, principal.Permissions{RoleKeys: []string{"custom"}, RowScope: principal.RowScopeOwn})
	_, _, err = quietDealScanWithClock(e.Pool, 19, func() time.Time { return at })(restricted, &storekit.OwnerSet{IDs: []ids.UUID{e.Rep1}})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("candidate selection bypassed the read grant: %v", err)
	}
}

func TestOwnershipScopesTheOverdueScanAtEveryQuietWindow(t *testing.T) {
	e := integration.Setup(t)
	pipeline, stage, _ := integration.DealFixture(t, e)
	at := clocktest.Now(t)
	own := e.SeedDeal(t, "Own dated recovery", pipeline, stage, &e.Rep1)
	for i := range slippingScanLimit + 1 {
		e.SeedDeal(t, fmt.Sprintf("Other dated recovery %d", i), pipeline, stage, &e.Rep3)
	}
	e.WsExec(t, `UPDATE deal SET expected_close_date = $1 WHERE pipeline_id = $2`, at.AddDate(0, 0, -20), pipeline)
	e.WsExec(t, `UPDATE deal SET expected_close_date = $1 WHERE id = $2`, at.AddDate(0, 0, -1), own)
	for _, days := range []int{19, 60} {
		rows, cut, err := quietDealScanWithClock(e.Pool, days, func() time.Time { return at })(e.Admin(), &storekit.OwnerSet{IDs: []ids.UUID{e.Rep1}})
		if err != nil {
			t.Fatal(err)
		}
		if cut || len(rows) != 1 || rows[0].DealID != own || !rows[0].CloseOverdue {
			t.Fatalf("owner's overdue scan at %d days: %+v, cut=%v", days, rows, cut)
		}
	}
	rows, _, err := quietDealScanWithClock(e.Pool, 19, func() time.Time { return at })(e.Admin(), &storekit.OwnerSet{})
	if err != nil || len(rows) != 0 {
		t.Fatalf("an empty owner set widened: %d rows, %v", len(rows), err)
	}
}
