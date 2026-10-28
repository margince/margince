// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The deal card's "Not now" and its undo, over a real database.
//
// The set-aside is one reader's, held against the move's fingerprint. The
// card stops naming that move for them and keeps naming it for others. After
// the undo it names the move again.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/dealstatus"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/dealrooms"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestSettingTheDealMoveAsideHidesItForThatReaderUntilTheUndo(t *testing.T) {
	e := Setup(t)
	pipelineID, stage, _ := DealFixture(t, e)
	deal := e.SeedDeal(t, "Turbinenbau Renewal", pipelineID, stage, &e.Rep1)
	subject := "Send the signed contract"
	now := clocktest.Now(t)
	due := now.Add(48 * time.Hour)
	if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "task", Subject: &subject, DueAt: &due, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: deal}},
	}); err != nil {
		t.Fatalf("logging the task: %v", err)
	}
	svc := dealstatus.NewService(e.Pool, e.Deals, e.Activities, dealrooms.NewStore(e.DB()), func() time.Time { return now })
	dealID := ids.From[ids.DealKind](deal)
	next := func(who string) *crmcontracts.DealStatusCardMove {
		t.Helper()
		ctx := e.Admin()
		if who == "rep" {
			ctx = e.As(e.Rep1, []ids.UUID{e.Team1}, AccountRepPerms)
		}
		card, err := svc.ReadFacts(ctx, dealID)
		if err != nil {
			t.Fatalf("reading the card as %s: %v", who, err)
		}
		return card.Next
	}

	before := next("rep")
	if before == nil || before.Action != dealstatus.ActionOpenTask || before.Fingerprint == nil {
		t.Fatalf("move = %+v, want the open task with a fingerprint to set it aside by", before)
	}
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, AccountRepPerms)
	if err := svc.DismissMove(rep, dealID, *before.Fingerprint); err != nil {
		t.Fatalf("setting the move aside: %v", err)
	}
	if got := next("rep"); got != nil {
		t.Fatalf("the set-aside move is still on the card: %+v", got)
	}
	if got := next("admin"); got == nil || got.Fingerprint == nil || *got.Fingerprint != *before.Fingerprint {
		t.Errorf("another reader's move = %+v, want the same move untouched", got)
	}
	// The Worklist's deal rows ask the same question of the cached move.
	queued := func(ctx context.Context) bool {
		t.Helper()
		moves, err := svc.CurrentMoves(ctx, []ids.UUID{deal})
		if err != nil {
			t.Fatalf("reading the cached moves: %v", err)
		}
		_, ok := moves[deal]
		return ok
	}
	if queued(rep) {
		t.Error("the set-aside move still reaches this reader's Worklist row")
	}
	if !queued(e.Admin()) {
		t.Error("another reader's Worklist row lost the move this reader set aside")
	}
	if err := svc.RestoreMove(rep, dealID, *before.Fingerprint); err != nil {
		t.Fatalf("undoing the set-aside: %v", err)
	}
	if got := next("rep"); got == nil || got.Fingerprint == nil || *got.Fingerprint != *before.Fingerprint {
		t.Errorf("after the undo the move = %+v, want it back", got)
	}
	if !queued(rep) {
		t.Error("after the undo the move is still off this reader's Worklist row")
	}

	// A digest of the wrong shape is the client's mistake, said as one.
	err := svc.DismissMove(rep, dealID, "not-a-digest")
	var problem *httperr.DetailedError
	if !errors.As(err, &problem) || problem.Status != 422 {
		t.Errorf("a malformed fingerprint answered %v, want a 422", err)
	}
}
