// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A lane the Worklist may lose, lost against a real database.
//
// The lane tests in attention/ drive fakes, and a fake that returns an error
// leaves no aborted transaction behind. The page reads every lane on ONE
// connection, so what a slow statement does to the lanes after it is a fact
// about Postgres and pgx that only a real snapshot can show.

import (
	"context"
	"maps"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// stuckWaiting is the real who-is-waiting reader whose read first spends longer
// than any lane budget on the page's snapshot, as the horizon measurement did.
type stuckWaiting struct {
	attention.Waiting
	pool *pgxpool.Pool
}

func (w stuckWaiting) Unanswered(ctx context.Context, asOf time.Time) ([]attention.WaitingCustomer, bool, error) {
	err := database.WithWorkspaceTx(ctx, w.pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `SELECT pg_sleep(30)`)
		return err
	})
	if err != nil {
		return nil, false, err
	}
	return w.Waiting.Unanswered(ctx, asOf)
}

// shortLaneBudget keeps the seam's real snapshot and shortens only the wait,
// so the test proves the isolation without spending the production budget.
type shortLaneBudget struct{ attentionSnapshots }

func (s shortLaneBudget) Degradable(ctx context.Context, _ time.Duration, fn func(context.Context) error) error {
	return s.attentionSnapshots.Degradable(ctx, 200*time.Millisecond, fn)
}

func TestAStuckWaitingLaneIsNamedUnavailableAndTheRestOfTheDayStillLoads(t *testing.T) {
	e := integration.Setup(t)
	seedLeadTask(t, e)

	db := InstallationDB(e.Pool)
	svc := newAttentionService(e.Pool, approvals.NewService(e.DB()), time.Now).
		WithWaiting(stuckWaiting{
			Waiting: attentionWaiting{
				store: activities.NewStore(db),
				deals: deals.NewStore(db, DealsInstallation()),
				now:   time.Now,
			},
			pool: e.Pool,
		}).
		WithSnapshots(shortLaneBudget{attentionSnapshots{pool: e.Pool}})

	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, leadRepPerms)
	started := time.Now()
	page, err := svc.Worklist(ctx, "mine", "", ids.Nil, 50, "")
	if err != nil {
		t.Fatalf("the worklist failed with its waiting lane: %v — a lane the page reports as unavailable took the page down instead", err)
	}
	if took := time.Since(started); took > 10*time.Second {
		t.Errorf("the page took %s, want the lane cut at its own budget", took)
	}

	var named bool
	for _, missing := range page.SourcesUnavailable {
		if string(missing.Source) == "customer_waiting" && missing.Reason == crmcontracts.WorklistSourceUnavailableReasonFailed {
			named = true
		}
	}
	if !named {
		t.Errorf("sources_unavailable is %v, want the waiting lane named as failed", page.SourcesUnavailable)
	}
	if got := taskTitles(page); len(got) != 1 || got[0] != "Call selected prospect" {
		t.Errorf("the task lane carries %v after the waiting lane failed, want the planned task", got)
	}
}

// stuckSuggestions is the real suggestion reader whose count first stalls on
// the page's snapshot, as the visibility clause did over production's evidence.
type stuckSuggestions struct {
	attentionDealSuggestions
	pool *pgxpool.Pool
}

func (s stuckSuggestions) CountOpen(ctx context.Context) (int, error) {
	err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `SELECT pg_sleep(30)`)
		return err
	})
	if err != nil {
		return 0, err
	}
	return s.attentionDealSuggestions.CountOpen(ctx)
}

func TestAStuckSuggestionReadIsNamedUnavailableAndTheRestOfTheDayStillLoads(t *testing.T) {
	e := integration.Setup(t)
	seedLeadTask(t, e)

	db := InstallationDB(e.Pool)
	svc := newAttentionService(e.Pool, approvals.NewService(e.DB()), time.Now).
		WithDealSuggestions(stuckSuggestions{
			attentionDealSuggestions: attentionDealSuggestions{store: deals.NewStore(db, DealsInstallation())},
			pool:                     e.Pool,
		}).
		WithSnapshots(shortLaneBudget{attentionSnapshots{pool: e.Pool}})

	// The suggestion read needs company as well as deal, or it is withheld
	// before it runs and the stall is never reached.
	perms := leadRepPerms
	perms.Objects = maps.Clone(leadRepPerms.Objects)
	perms.Objects["company"] = principal.ObjectGrant{Read: true}
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, perms)
	started := time.Now()
	page, err := svc.Worklist(ctx, "mine", "", ids.Nil, 50, "")
	if err != nil {
		t.Fatalf("the worklist failed with its suggestion read: %v — a source the page reports as unavailable took the page down instead", err)
	}
	if took := time.Since(started); took > 10*time.Second {
		t.Errorf("the page took %s, want the read cut at its lane budget", took)
	}

	var named bool
	for _, missing := range page.SourcesUnavailable {
		if missing.Source == "deal_suggestion" && missing.Reason == crmcontracts.WorklistSourceUnavailableReasonFailed {
			named = true
		}
	}
	if !named {
		t.Errorf("sources_unavailable is %v, want deal_suggestion named as failed", page.SourcesUnavailable)
	}
	for _, item := range page.Queue {
		if string(item.Source) == "deal_suggestion" {
			t.Errorf("the queue carries suggestion %s from a read that failed", item.Id)
		}
	}
	if got := taskTitles(page); len(got) != 1 || got[0] != "Call selected prospect" {
		t.Errorf("the task lane carries %v after the suggestion read failed, want the planned task", got)
	}
}
