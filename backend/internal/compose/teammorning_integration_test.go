// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestTeamDecisionResponsibilityTracksLiveMembership(t *testing.T) {
	e := integration.Setup(t)
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	pipeline, open, _ := integration.DealFixture(t, e)
	deal := e.SeedDeal(t, "Example team renewal", pipeline, open, &e.Rep1)
	queue := approvalsServiceWithEffects(e.Pool)
	decision, err := queue.Stage(onBehalfOf(e.AgentCtx(), e.Rep1), approvals.StageInput{
		Kind: "advance_deal", ProposedChange: json.RawMessage(`{"stage":"won"}`),
		DiffHash: "team-owned-stage", TargetType: "deal", TargetID: deal, Summary: "Review renewal stage",
	})
	if err != nil {
		t.Fatal(err)
	}
	feed := newAttentionService(e.Pool, queue, func() time.Time { return now })
	perms := integration.AdminPerms
	perms.RowScope = principal.RowScopeTeam
	readCtx := e.As(e.Rep2, []ids.UUID{e.Team1}, perms)
	assertVisible := func(scope crmcontracts.GetWorklistParamsScope, want bool) {
		t.Helper()
		ctx := readCtx
		if scope == "all" {
			ctx = e.As(e.AdminUser, nil, integration.AdminPerms)
		}
		page := readWorklistHTTP(ctx, t, feed, crmcontracts.GetWorklistParams{Scope: &scope})
		found := false
		for _, row := range page.Queue {
			if row.Id != decision.String() {
				continue
			}
			found = true
			if row.Owner == nil || row.Owner.Id == nil || ids.UUID(*row.Owner.Id) != e.Rep1 {
				t.Fatalf("decision lost its responsible owner: %+v", row.Owner)
			}
		}
		if found != want {
			t.Fatalf("scope %s decision present=%v, want %v", scope, found, want)
		}
	}
	assertVisible("mine", false)
	assertVisible("team", true)
	assertVisible("all", true)
	named := openapi_types.UUID(e.Rep1)
	namedPage := readWorklistHTTP(readCtx, t, feed, crmcontracts.GetWorklistParams{Owner: &named})
	found := false
	for _, row := range namedPage.Queue {
		if row.Id == decision.String() {
			found = true
		}
	}
	if !found {
		t.Fatal("responsible teammate's named queue omitted their decision")
	}
	svc := identity.NewServiceFor(e.DB())
	admin := identity.Identity{UserID: ids.UserID{UUID: e.AdminUser}, WorkspaceID: ids.WorkspaceID{UUID: e.WS}, Permissions: integration.AdminPerms, Roles: []string{"admin"}, SeatType: "full"}
	if err := svc.SetTeamMember(e.Admin(), admin, e.Team1, e.Rep1, false); err != nil {
		t.Fatal(err)
	}
	assertVisible("team", false)
	assertVisible("all", true)
	if err := svc.SetTeamMember(e.Admin(), admin, e.Team1, e.Rep1, true); err != nil {
		t.Fatal(err)
	}
	assertVisible("team", true)
	if err := svc.DeactivateUser(e.Admin(), admin, identity.DeactivateUserInput{UserID: ids.UserID{UUID: e.Rep1}}); err != nil {
		t.Fatal(err)
	}
	assertVisible("team", false)
	assertVisible("all", true)
}

func TestTeamWorklistExcludesUnlinkedOutsideTasksAndRefusesTheirOwner(t *testing.T) {
	e := integration.Setup(t)
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	tasks := make(map[ids.UUID]string)
	for _, owner := range []ids.UUID{e.Rep1, e.Rep2, e.Rep3} {
		assignee := ids.From[ids.UserKind](owner)
		subject := "Follow up " + owner.String()
		row, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
			Kind: "task", Source: "manual", Subject: &subject, AssigneeID: &assignee, DueAt: &now, OccurredAt: &now,
		})
		if err != nil {
			t.Fatal(err)
		}
		tasks[owner] = ids.UUID(row.Id).String()
	}
	feed := newAttentionService(e.Pool, approvalsServiceWithEffects(e.Pool), func() time.Time { return now })
	for _, tc := range []struct {
		name   string
		reader ids.UUID
		team   ids.UUID
		scope  principal.RowScope
		want   []ids.UUID
	}{
		{"team A", e.Rep2, e.Team1, principal.RowScopeTeam, []ids.UUID{e.Rep1, e.Rep2}},
		{"team B", e.Rep3, e.Team2, principal.RowScopeTeam, []ids.UUID{e.Rep3}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			perms := integration.AdminPerms
			perms.RowScope = tc.scope
			ctx := e.As(tc.reader, []ids.UUID{tc.team}, perms)
			scope := crmcontracts.GetWorklistParamsScopeTeam
			page := readWorklistHTTP(ctx, t, feed, crmcontracts.GetWorklistParams{Scope: &scope})
			listed := make(map[string]bool)
			for _, row := range page.Queue {
				listed[row.Id] = true
			}
			for owner, task := range tasks {
				want := false
				for _, member := range tc.want {
					if member == owner {
						want = true
					}
				}
				if listed[task] != want {
					t.Fatalf("task %s present=%v, want=%v", task, listed[task], want)
				}
			}
			outside := e.Rep3
			if tc.reader == e.Rep3 {
				outside = e.Rep1
			}
			response := httptest.NewRecorder()
			named := openapi_types.UUID(outside)
			attention.NewHandlers(feed).GetWorklist(response, httptest.NewRequest(http.MethodGet, "/v1/worklist", nil).WithContext(ctx), crmcontracts.GetWorklistParams{Owner: &named})
			if response.Code != http.StatusForbidden || strings.Contains(response.Body.String(), tasks[outside]) {
				t.Fatalf("outside owner returned HTTP %d: %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestTeamBoardCountsOnlyReadableWaitingMail(t *testing.T) {
	e := integration.Setup(t)
	var recipient string
	if err := integration.OwnerConn(t).QueryRow(t.Context(), `SELECT email FROM app_user WHERE id=$1`, e.Rep1).Scan(&recipient); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	pipeline, open, _ := integration.DealFixture(t, e)
	deal := e.SeedDeal(t, "Example mail renewal", pipeline, open, &e.Rep1)
	for _, private := range []bool{false, true} {
		subject, direction := "Please confirm the quote", "inbound"
		thread := "shared-request"
		if private {
			thread = "private-request"
		}
		at := now.Add(-time.Hour)
		input := activities.LogActivityInput{
			Kind: "email", Source: "manual", Subject: &subject, Direction: &direction, OccurredAt: &at, ThreadKey: thread,
			EmailTo: []string{recipient},
			Links:   []activities.ActivityLinkInput{{EntityType: "deal", EntityID: deal}},
		}
		if private {
			input.VisibleOnlyTo(e.Rep1)
		}
		message, _, err := e.Activities.LogActivity(e.Admin(), input)
		if err != nil {
			t.Fatal(err)
		}
		judge := principal.SystemActing(e.Admin(), "system:owed_verdict")
		id := ids.UUID(message.Id)
		if applied, err := e.Activities.SetCaptureLabel(judge, id, "commitment"); err != nil || applied == private {
			t.Fatalf("capture label on private=%v: %v %v", private, applied, err)
		}
		if private {
			continue
		}
		if applied, err := e.Activities.SetOwedVerdict(judge, id, activities.OwedVerdictAsksUs, "test-rules", at); err != nil || !applied {
			t.Fatalf("owed verdict: %v %v", applied, err)
		}
	}
	feed := newAttentionService(e.Pool, approvalsServiceWithEffects(e.Pool), func() time.Time { return now })
	for _, tc := range []struct {
		reader ids.UUID
		want   int
	}{{e.Rep1, 1}, {e.Rep2, 1}} {
		perms := integration.AdminPerms
		perms.RowScope = principal.RowScopeTeam
		ctx := e.As(tc.reader, []ids.UUID{e.Team1}, perms)
		response := httptest.NewRecorder()
		attention.NewHandlers(feed).GetTeamBoard(response, httptest.NewRequest(http.MethodGet, "/v1/worklist/team", nil).WithContext(ctx), crmcontracts.GetTeamBoardParams{})
		if response.Code != http.StatusOK {
			t.Fatalf("board HTTP %d: %s", response.Code, response.Body.String())
		}
		var board crmcontracts.TeamBoard
		if err := json.Unmarshal(response.Body.Bytes(), &board); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, member := range board.Members {
			if ids.UUID(member.UserId) == e.Rep1 {
				found = true
				if member.Counts.Waiting != tc.want {
					t.Fatalf("reader %s sees %d waits, want %d", tc.reader, member.Counts.Waiting, tc.want)
				}
			}
		}
		if !found {
			t.Fatal("board omitted the responsible teammate")
		}
	}
}
