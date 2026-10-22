// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/briefs"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestStoredBriefRankingNeedsCurrentInterventionBeforeWorklistAdmission(t *testing.T) {
	e := integration.Setup(t)
	// Postgres keeps microseconds; a Linux clock reads nanoseconds, so the
	// instant read back would never equal one with finer precision.
	now := clocktest.Now(t).Truncate(time.Microsecond)
	clock := func() time.Time { return now }
	store := e.Deals.WithClock(clock)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	pipeline, open, _ := integration.DealFixture(t, e)
	owner := ids.From[ids.UserKind](e.Rep1)
	amount, currency := int64(6000000), "EUR"
	create := func(name string, closes *time.Time) ids.DealID {
		t.Helper()
		row, err := store.CreateDeal(ctx, deals.CreateDealInput{Name: name, PipelineID: pipeline, StageID: open, OwnerID: &owner, AmountMinor: &amount, Currency: &currency, ExpectedClose: closes, Source: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		return ids.From[ids.DealKind](ids.UUID(row.Id))
	}
	unsupported := create("Valuable but no current intervention", nil)
	closeDate := now.AddDate(0, 0, 5)
	supported := create("Current closing window", &closeDate)
	engine := briefs.NewBriefEngine(e.Pool, e.Contacts)
	run, err := engine.SnapshotRun(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Items) != 2 {
		t.Fatalf("native ranking produced %d entries, want both candidates", len(run.Items))
	}
	for _, item := range run.Items {
		if item.DealID != supported.UUID {
			continue
		}
		if err := engine.AnnotateCurrentRun(ctx, briefs.Annotation{Items: []briefs.ItemAnnotation{{
			ItemID: item.ID, Finding: "The current close window needs review.", CitedEvidence: item.EvidenceIDs,
		}}}, now); err != nil {
			t.Fatal(err)
		}
	}
	subject := "Prepare the agreed proposal"
	if _, _, err := e.Activities.LogActivity(ctx, activities.LogActivityInput{Kind: "task", Subject: &subject, DueAt: &now, Source: "manual"}); err != nil {
		t.Fatal(err)
	}
	svc := newAttentionService(e.Pool, approvals.NewService(e.DB()), clock).WithDealFacts(attentionDealFacts{store: store})
	read := func() crmcontracts.Worklist {
		t.Helper()
		page, err := svc.Worklist(ctx, "mine", "", ids.Nil, 100, "")
		if err != nil {
			t.Fatal(err)
		}
		return page
	}
	page := read()
	var supportedRow, task bool
	for _, row := range page.Queue {
		if row.Source == "task" {
			task = true
		}
		if row.Subject == nil {
			continue
		}
		if ids.UUID(row.Subject.Id) == unsupported.UUID {
			t.Fatal("ranking alone created actionable work")
		}
		if ids.UUID(row.Subject.Id) == supported.UUID {
			supportedRow = true
			if row.Verdict == nil || row.Verdict.AsOf == nil || !row.Verdict.AsOf.Equal(run.AsOf) {
				t.Fatal("the composed row lost the cached finding's source cutoff")
			}
			found := false
			for _, reason := range row.Because {
				if reason.Kind == "closing_soon" {
					found = true
				}
			}
			if !found {
				t.Fatal("admitted suggestion lost its current close-date reason")
			}
		}
	}
	if !supportedRow || !task {
		t.Fatalf("supported suggestion/task missing: suggestion=%t task=%t", supportedRow, task)
	}
	later := now.AddDate(0, 0, 90)
	if _, err := store.UpdateDeal(ctx, supported, deals.UpdateDealInput{ExpectedClose: &later}); err != nil {
		t.Fatal(err)
	}
	page = read()
	for _, row := range page.Queue {
		if row.Source == "brief_item" {
			t.Fatal("stored suggestion survived the removal of its current basis")
		}
	}
	for _, count := range page.Counts {
		if count.Category == "deals_at_risk" && count.Considered != 0 {
			t.Fatalf("unsupported suggestions counted: %+v", count)
		}
	}
	if page.Focus != nil {
		for _, row := range page.Focus.Items {
			if row.Source == "brief_item" {
				t.Fatal("unsupported suggestion survived in Focus")
			}
		}
	}
	review, err := engine.LatestRun(ctx, now)
	if err != nil || len(review.Items) != 2 {
		t.Fatalf("existing review source lost its suggestions: items=%d err=%v", len(review.Items), err)
	}
}

func TestStoredBriefDoesNotReviveAClosedDeal(t *testing.T) {
	e := integration.Setup(t)
	now := clocktest.Now(t)
	clock := func() time.Time { return now }
	store := e.Deals.WithClock(clock)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	pipeline, open, won := integration.DealFixture(t, e)
	amount, currency, closes := int64(6000000), "EUR", now.AddDate(0, 0, 5)
	owner := ids.From[ids.UserKind](e.Rep1)
	deal, err := store.CreateDeal(ctx, deals.CreateDealInput{Name: "Deal with a completed decision", PipelineID: pipeline, StageID: open, OwnerID: &owner, AmountMinor: &amount, Currency: &currency, ExpectedClose: &closes, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	engine := briefs.NewBriefEngine(e.Pool, e.Contacts)
	run, err := engine.SnapshotRun(ctx, now)
	if err != nil || len(run.Items) != 1 {
		t.Fatalf("native snapshot: items=%d err=%v", len(run.Items), err)
	}
	lane := attentionBriefing{engine: engine, figures: attentionDealFacts{store: store}, now: clock}
	before, _, _, err := lane.Queue(ctx)
	if err != nil || len(before) != 1 {
		t.Fatalf("open brief entry: items=%d err=%v", len(before), err)
	}
	if _, err := store.AdvanceDeal(ctx, ids.From[ids.DealKind](ids.UUID(deal.Id)), deals.AdvanceDealInput{ToStageID: won, WonWithoutContractReason: new("verbal")}); err != nil {
		t.Fatal(err)
	}
	after, _, _, err := lane.Queue(ctx)
	if err != nil || len(after) != 0 {
		t.Fatalf("closed brief entry: items=%d err=%v", len(after), err)
	}
}

func TestCurrentQuietDealObservationsExplainTheStoredSuggestion(t *testing.T) {
	e := integration.Setup(t)
	now := clocktest.Now(t)
	clock := func() time.Time { return now }
	store := e.Deals.WithClock(clock)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	pipeline, open, _ := integration.DealFixture(t, e)
	owner := ids.From[ids.UserKind](e.Rep1)
	amount, currency := int64(6000000), "EUR"
	deal, err := store.CreateDeal(ctx, deals.CreateDealInput{Name: "Proposal without a recorded next step", PipelineID: pipeline, StageID: open, OwnerID: &owner, AmountMinor: &amount, Currency: &currency, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	old, title := now.AddDate(0, 0, -30), "Discussed rollout scope"
	link := activities.ActivityLinkInput{EntityType: "deal", EntityID: ids.UUID(deal.Id)}
	if _, _, err := e.Activities.LogActivity(ctx, activities.LogActivityInput{Kind: "call", Subject: &title, OccurredAt: &old, Source: "manual", Links: []activities.ActivityLinkInput{link}}); err != nil {
		t.Fatal(err)
	}
	engine := briefs.NewBriefEngine(e.Pool, e.Contacts)
	run, err := engine.SnapshotRun(ctx, now)
	if err != nil || len(run.Items) != 1 {
		t.Fatalf("native snapshot: items=%d err=%v", len(run.Items), err)
	}
	svc := newAttentionService(e.Pool, approvals.NewService(e.DB()), clock)
	page, err := svc.Worklist(ctx, "mine", "", ids.Nil, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range page.Queue {
		if row.Subject == nil || row.Subject.Id != deal.Id {
			continue
		}
		reasons := map[string]bool{}
		for _, reason := range row.Because {
			reasons[string(reason.Kind)] = true
		}
		if !reasons["quiet_days"] || !reasons["no_next_step"] {
			t.Fatalf("native current observations missing: %+v", row.Because)
		}
		return
	}
	t.Fatal("the current quiet deal never reached the composed queue")
}
