// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// closeDayEnv is a closed JPY deal with two stored rates. The older one is what
// a close on the corrected day must freeze. The newer one is what the real close
// froze.
type closeDayEnv struct {
	*configEnv
	deal     ids.DealID
	today    time.Time
	oldDay   time.Time
	oldTotal int64
}

func setupClosedDealEnv(t *testing.T) closeDayEnv {
	t.Helper()
	e := setupConfigEnv(t)
	now := clocktest.Now(t).UTC()
	// Winning stamps correspondence and starts delivery through seams the
	// composition injects; neither is under test here.
	e.store = NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)), Installation{
		Timezone:                func(context.Context, pgx.Tx) (string, error) { return "UTC", nil },
		BaseCurrency:            func(context.Context, pgx.Tx) (string, error) { return "EUR", nil },
		StampCorrespondence:     func(context.Context, pgx.Tx, ids.DealID, string) error { return nil },
		StartDeliveryForWonDeal: func(context.Context, pgx.Tx, ids.DealID, string) error { return nil },
		ContractVisible:         func(context.Context, string, func(any) int) (string, error) { return "FALSE", nil },
	}).WithClock(func() time.Time { return now })

	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	env := closeDayEnv{configEnv: e, today: today, oldDay: today.AddDate(0, 0, -10), oldTotal: 5000}
	e.seedRate(t, "JPY", "0.005", today.AddDate(0, 0, -20))
	e.seedRate(t, "JPY", "0.006", today.AddDate(0, 0, -2))

	open := openStage(t, e, SemanticOpen)
	var pipelineID ids.PipelineID
	if err := e.store.Tx(e.as(), func(tx pgx.Tx) error {
		return tx.QueryRow(e.as(), `SELECT pipeline_id FROM stage WHERE id = $1`, open).Scan(&pipelineID)
	}); err != nil {
		t.Fatalf("reading the pipeline: %v", err)
	}
	hundred := 100
	wonStage, err := e.store.CreateStage(e.as(), CreateStageInput{
		PipelineID: pipelineID, Name: "Won", Position: 1,
		Semantic: string(SemanticWon), WinProbability: &hundred,
	})
	if err != nil {
		t.Fatalf("seeding the won stage: %v", err)
	}
	won := ids.From[ids.StageKind](ids.UUID(wonStage.Id))
	owner := ids.From[ids.UserKind](e.admin)
	amount, currency := int64(10000), "JPY"
	deal, err := e.store.CreateDeal(e.asDealWriter(), CreateDealInput{
		Name: "Imported win", PipelineID: pipelineID, StageID: open, OwnerID: &owner, OwnerExact: true,
		AmountMinor: &amount, Currency: &currency,
	})
	if err != nil {
		t.Fatalf("seeding the deal: %v", err)
	}
	env.deal = ids.From[ids.DealKind](ids.UUID(deal.Id))
	reason := "imported"
	if _, err := e.store.AdvanceDeal(e.asDealWriter(), env.deal,
		AdvanceDealInput{ToStageID: won, WonWithoutContractReason: &reason}); err != nil {
		t.Fatalf("closing the deal: %v", err)
	}
	return env
}

type storedClose struct {
	closedAt time.Time
	rateDate time.Time
	base     int64
}

func (e closeDayEnv) stored(t *testing.T) storedClose {
	t.Helper()
	var s storedClose
	if err := e.store.Tx(e.as(), func(tx pgx.Tx) error {
		return tx.QueryRow(e.as(),
			`SELECT closed_at, fx_rate_date, amount_minor_base FROM deal WHERE id = $1`,
			e.deal).Scan(&s.closedAt, &s.rateDate, &s.base)
	}); err != nil {
		t.Fatalf("reading the stored close: %v", err)
	}
	return s
}

func TestCorrectingTheCloseDayMovesClosedAtAndFreezesTheRateOfThatDay(t *testing.T) {
	e := setupClosedDealEnv(t)
	if got := e.stored(t); got.base != 6000 {
		t.Fatalf("precondition: the real close froze %d base units, want 6000 at the newer rate", got.base)
	}

	if _, err := e.store.CorrectCloseDay(e.asDealWriter(), e.deal, CorrectCloseDayInput{ClosedOn: e.oldDay}); err != nil {
		t.Fatalf("correcting the close day: %v", err)
	}

	got := e.stored(t)
	if !got.closedAt.Equal(e.oldDay.Add(12 * time.Hour)) {
		t.Errorf("closed_at = %s, want midday of %s", got.closedAt, e.oldDay.Format(time.DateOnly))
	}
	if got.base != e.oldTotal {
		t.Errorf("amount_minor_base = %d, want %d: the rate of the corrected day was not frozen again", got.base, e.oldTotal)
	}
	if !got.rateDate.Equal(e.oldDay) {
		t.Errorf("fx_rate_date = %s, want the corrected day %s", got.rateDate.Format(time.DateOnly), e.oldDay.Format(time.DateOnly))
	}
	before, after := e.auditImages(t, "deal", e.deal.UUID)
	if _, moved := after["closed_at"]; !moved || before["closed_at"] == nil {
		t.Errorf("the audit row carries before %v and after %v, want closed_at in both", before, after)
	}
}

func TestCorrectingToTheDayItClosedOnChangesNothing(t *testing.T) {
	e := setupClosedDealEnv(t)
	before := e.stored(t)
	if _, err := e.store.CorrectCloseDay(e.asDealWriter(), e.deal, CorrectCloseDayInput{ClosedOn: e.today}); err != nil {
		t.Fatalf("correcting to the same day: %v", err)
	}
	if after := e.stored(t); after != before {
		t.Errorf("the same day rewrote the close: before %+v, after %+v", before, after)
	}
}

func TestACloseDayCorrectionRefusesWhatItCannotHold(t *testing.T) {
	e := setupClosedDealEnv(t)
	agent := principal.WithActor(e.asDealWriter(), principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:x", UserID: e.admin,
		Permissions: principal.Permissions{
			RoleKeys: []string{"admin"}, RowScope: principal.RowScopeAll,
			Objects: map[string]principal.ObjectGrant{"deal": {Read: true, Update: true}},
		},
	})
	cases := []struct {
		name string
		ctx  context.Context
		day  time.Time
		want func(error) bool
	}{
		{"an agent", agent, e.oldDay, func(err error) bool { return errors.Is(err, apperrors.ErrPermissionDenied) }},
		{"a day after today", e.asDealWriter(), e.today.AddDate(0, 0, 1), func(err error) bool {
			var f *CloseDayFutureError
			return errors.As(err, &f)
		}},
		{"a day before any stored rate", e.asDealWriter(), e.today.AddDate(0, 0, -40), func(err error) bool {
			var f *MissingFxRateError
			return errors.As(err, &f)
		}},
	}
	for _, c := range cases {
		before := e.stored(t)
		_, err := e.store.CorrectCloseDay(c.ctx, e.deal, CorrectCloseDayInput{ClosedOn: c.day})
		if err == nil || !c.want(err) {
			t.Errorf("%s: got %v", c.name, err)
		}
		if after := e.stored(t); after != before {
			t.Errorf("%s: a refused correction still changed the close", c.name)
		}
	}
}

func TestAnOpenDealHasNoCloseDayToCorrect(t *testing.T) {
	e := setupConfigEnv(t)
	e.store = NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)), Installation{
		Timezone: func(context.Context, pgx.Tx) (string, error) { return "UTC", nil },
	})
	deal := seedDealInStage(t, e, openStage(t, e, SemanticOpen))
	_, err := e.store.CorrectCloseDay(e.asDealWriter(), deal, CorrectCloseDayInput{ClosedOn: clocktest.Now(t).UTC()})
	if _, ok := errors.AsType[*CloseDayOpenDealError](err); !ok {
		t.Fatalf("an open deal answered %v, want deal_not_closed", err)
	}
}
