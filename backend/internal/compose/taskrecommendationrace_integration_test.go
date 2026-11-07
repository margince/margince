// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestConcurrentAcceptanceCreatesOneSuggestedTask(t *testing.T) {
	e := setupVerdicts(t)
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	at := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	cards := newDealStatusService(e.Pool, func() time.Time { return at })
	card, err := cards.ReadFacts(reader, ids.From[ids.DealKind](e.calm))
	if err != nil || card.Next == nil {
		t.Fatalf("current recommendation = %+v, %v", card.Next, err)
	}
	in := suggestedTaskInput(t, card.Next)
	guard := taskRecommendationFrom(cards)
	ready, release := make(chan struct{}, 2), make(chan struct{})
	defer close(release)
	var arrivals atomic.Int32
	store := activities.NewStore(e.DB()).WithClock(func() time.Time { return at }).WithTaskRecommendation(
		func(ctx context.Context, tx pgx.Tx, receipt crmcontracts.RecommendationReceipt) (activities.LogActivityInput, error) {
			current, err := guard(ctx, tx, receipt)
			if err == nil && arrivals.Add(1) <= 2 {
				ready <- struct{}{}
				<-release
			}
			return current, err
		})
	type acceptance struct {
		id      crmcontracts.Id
		created bool
		err     error
	}
	results := make(chan acceptance, 2)
	before := e.WsCount(t, "SELECT count(*) FROM activity")
	for range 2 {
		go func() {
			out, created, err := store.LogActivity(reader, in)
			results <- acceptance{out.Id, created, err}
		}()
	}
	for range 2 {
		select {
		case <-ready:
		case result := <-results:
			t.Fatalf("acceptance stopped before both real validations: %v", result.err)
		}
	}
	// Both real guards read the same basis before either writer proceeds.
	release <- struct{}{}
	release <- struct{}{}
	created := 0
	var committed crmcontracts.Id
	for range 2 {
		result := <-results
		if result.err != nil {
			t.Fatalf("concurrent acceptance: %v", result.err)
		}
		if result.created {
			created++
		}
		if committed != (crmcontracts.Id{}) && committed != result.id {
			t.Fatal("the same receipt returned different tasks")
		}
		committed = result.id
	}
	if created != 1 || e.WsCount(t, "SELECT count(*) FROM activity") != before+1 {
		t.Fatalf("created=%d; expected one committed suggestion and one replay", created)
	}
}

func TestSuggestedTaskRefusesAWeakBorrowedTransaction(t *testing.T) {
	e := setupVerdicts(t)
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	at := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	cards := newDealStatusService(e.Pool, func() time.Time { return at })
	card, err := cards.ReadFacts(reader, ids.From[ids.DealKind](e.calm))
	if err != nil || card.Next == nil {
		t.Fatalf("current recommendation = %+v, %v", card.Next, err)
	}
	in := suggestedTaskInput(t, card.Next)
	store := activities.NewStore(e.DB()).WithTaskRecommendation(taskRecommendationFrom(cards))
	before := e.WsCount(t, "SELECT count(*) FROM activity")
	err = database.WithWorkspaceSnapshot(reader, e.Pool, func(ctx context.Context) error {
		return database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			_, _, err := store.LogActivityTx(ctx, tx, in)
			return err
		})
	})
	if !errors.Is(err, apperrors.ErrInvalidArgument) || e.WsCount(t, "SELECT count(*) FROM activity") != before {
		t.Fatalf("a weak transaction accepted suggested work: %v", err)
	}
}
