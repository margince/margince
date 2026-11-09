// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"maps"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// decisionCensusForTest is attention's batchScanDepth, the approvals the ranked
// queue reads before the tail takes over.
const decisionCensusForTest = 200

// stageCloseDates has seat stage one close-date correction on each of count new
// deals it owns, through the approvals writer, and answers their ids. The
// stager is the decider the request names.
func stageCloseDates(
	t *testing.T, e *integration.Env, svc *approvals.Service, pipeline ids.PipelineID, open ids.StageID, seat, team ids.UUID, count int,
) map[string]bool {
	t.Helper()
	stager := e.As(seat, []ids.UUID{team}, integration.AdminPerms)
	staged := map[string]bool{}
	for range count {
		deal := e.SeedDeal(t, "Review the date", pipeline, open, &seat)
		proposal, err := json.Marshal(deals.CloseDateCorrection{DealID: ids.From[ids.DealKind](deal), ExpectedCloseDate: "2026-12-01", Basis: "Review the date"})
		if err != nil {
			t.Fatal(err)
		}
		id, err := svc.Stage(stager, approvals.StageInput{Kind: deals.CloseDateCorrectionKind, TargetType: "deal", TargetID: deal, ProposedChange: proposal, DiffHash: deal.String()})
		if err != nil {
			t.Fatal(err)
		}
		staged[id.String()] = true
	}
	return staged
}

// decisionReader is a team-scope seat holding every grant the Decisions
// view reads, so each of its sources answers and the view can state a total.
func decisionReader(e *integration.Env, seat, team ids.UUID) context.Context {
	perms := integration.AdminPerms
	perms.Objects = maps.Clone(perms.Objects)
	perms.Objects["introduction"] = principal.ObjectGrant{Read: true}
	perms.RowScope = principal.RowScopeTeam
	return e.As(seat, []ids.UUID{team}, perms)
}

// walkDecisions follows next_cursor through the Decisions view to its end and
// answers how often each approval was served, and the first page.
func walkDecisions(
	ctx context.Context, t *testing.T, e *integration.Env, svc *approvals.Service, scope string,
) (map[string]int, crmcontracts.Worklist) {
	t.Helper()
	now := time.Date(2026, 9, 15, 8, 0, 0, 0, time.UTC)
	feed := newAttentionService(e.Pool, svc, func() time.Time { return now })
	served := map[string]int{}
	var first crmcontracts.Worklist
	cursor := ""
	for page := 0; ; page++ {
		if page > 30 {
			t.Fatalf("%s: the Decisions walk did not end in 30 pages", scope)
		}
		day, err := feed.WorklistView(ctx, "decisions", scope, "", ids.Nil, 25, cursor)
		if err != nil {
			t.Fatalf("%s page %d: %v", scope, page, err)
		}
		if page == 0 {
			first = day
		}
		for _, row := range day.Queue {
			if row.Source == crmcontracts.WorklistItemSourceApproval {
				served[row.Id]++
			}
		}
		if day.NextCursor == nil {
			return served, first
		}
		cursor = *day.NextCursor
	}
}

// Past the ranked queue's census depth, the Decisions view continues through
// next_cursor until every approval its reader may decide has been served once.
// The stated total is the number walked, and nobody else's approval is served.
func TestTheDecisionsViewPagesThroughEveryApprovalPastTheCensus(t *testing.T) {
	e := integration.Setup(t)
	svc := NewApprovalsService(e.DB())
	pipeline, open, _ := integration.DealFixture(t, e)
	rep1 := stageCloseDates(t, e, svc, pipeline, open, e.Rep1, e.Team1, decisionCensusForTest+30)
	rep3 := stageCloseDates(t, e, svc, pipeline, open, e.Rep3, e.Team2, 5)

	for _, check := range []struct {
		name, scope string
		ctx         context.Context
		want, never map[string]bool
		exact       bool
	}{
		{"Rep1's own", "mine", decisionReader(e, e.Rep1, e.Team1), rep1, rep3, true},
		{"Rep1's team", "team", decisionReader(e, e.Rep1, e.Team1), rep1, rep3, false},
		{"Rep3's own", "mine", decisionReader(e, e.Rep3, e.Team2), rep3, rep1, true},
	} {
		t.Run(check.name, func(t *testing.T) {
			served, first := walkDecisions(check.ctx, t, e, svc, check.scope)
			for id, times := range served {
				switch {
				case check.never[id]:
					t.Errorf("approval %s, which this reader may not decide, was served", id)
				case times != 1:
					t.Errorf("approval %s was served %d times, want once", id, times)
				}
			}
			for id := range check.want {
				if served[id] == 0 {
					t.Errorf("approval %s was never served", id)
				}
			}
			if !check.exact {
				return
			}
			total, stated := totalOf(first, crmcontracts.WorklistCountCategoryDecisions)
			if !stated || total != len(served) || total != len(check.want) {
				t.Errorf("decisions total = %d (stated %v); the walk served %d of %d", total, stated, len(served), len(check.want))
			}
		})
	}
}
