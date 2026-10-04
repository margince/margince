// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A snapshot freezes per-deal figures, and listing its id makes it reachable:
// forecast_movement must show a caller no more than the live forecast would.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func frozenDeal(deal, owner ids.UUID, base int64) forecasting.Contribution {
	return forecasting.Contribution{
		DealID: deal.String(), Owner: owner.String(), AmountMinor: &base, Currency: "EUR",
		BaseMinor: &base, Category: forecasting.CategoryCommit, StageProbability: 50,
		WeightedMinor: base / 2, InOpen: true, InEvidence: true, InBestCase: true,
	}
}

type movementAnswer struct {
	Data struct {
		OpeningMinor int64 `json:"opening_minor"`
		ClosingMinor int64 `json:"closing_minor"`
		Deals        []struct {
			DealID string `json:"deal_id"`
		} `json:"deals"`
	} `json:"data"`
}

func ownSeat(masked bool) principal.Permissions {
	perms := RepPerms
	perms.RowScope = principal.RowScopeOwn
	perms.Objects = map[string]principal.ObjectGrant{
		"forecast": {Read: true}, "deal": {Read: true}, "pipeline": {Read: true},
		"installation_settings": {Read: true},
	}
	if masked {
		perms.FieldMasks = []principal.FieldMask{
			{Object: "deal", Field: "amount_minor", Condition: principal.MaskAlways}}
	}
	return perms
}

func TestForecastMovementShowsACallerOnlyWhatTheLiveForecastWould(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	registry := compose.NewRegistry(e.Pool, compose.SendPath{})
	admin := e.Admin()

	seed := func(owner ids.UUID, name string) ids.UUID {
		id := ids.NewV7()
		e.WsExec(t, `INSERT INTO deal (id, name, amount_minor, currency, expected_close_date, pipeline_id,
			stage_id, owner_id, source, captured_by)
			VALUES ($1, $2, 100000, 'EUR', '2038-02-20', $3, $4, $5, 'manual', 'human:test')`,
			id, name, pipeline, open, owner)
		return id
	}
	mine := seed(e.Rep1, "Mine")
	// A frozen row naming a deal no longer readable (here, gone): deals are
	// workspace-readable, so the table has nothing else that withholds one.
	theirs := ids.NewV7()

	// One owner-scoped pair holding BOTH deals, so the row filter is what
	// separates them, and a workspace pair a team-lens seat may not measure.
	owner := forecasting.Scope{Kind: forecasting.ScopeOwner, ID: &e.Rep1}
	at := time.Date(2038, 2, 1, 9, 0, 0, 0, time.UTC)
	opening := freezeScope(admin, t, e, at, owner, []forecasting.Contribution{frozenDeal(mine, e.Rep1, 100_000)})
	closing := freezeScope(admin, t, e, at.Add(time.Hour), owner, []forecasting.Contribution{
		frozenDeal(mine, e.Rep1, 140_000), frozenDeal(theirs, e.Rep3, 70_000)})
	workspace := forecasting.Scope{Kind: forecasting.ScopeWorkspace}
	wsOpening := freezeScope(admin, t, e, at.Add(2*time.Hour), workspace, nil)
	wsClosing := freezeScope(admin, t, e, at.Add(3*time.Hour), workspace, nil)

	move := func(ctx context.Context, from, to ids.UUID) (movementAnswer, error) {
		out, err := registry.Invoke(ctx, "forecast_movement",
			json.RawMessage(`{"from":"`+from.String()+`","to":"`+to.String()+`"}`))
		var answer movementAnswer
		if err == nil {
			err = json.Unmarshal(out, &answer)
		}
		return answer, err
	}

	t.Run("an unmasked seat sees its own deal and none it cannot read", func(t *testing.T) {
		got, err := move(e.As(e.Rep1, nil, ownSeat(false)), opening, closing)
		if err != nil {
			t.Fatal(err)
		}
		if got.Data.OpeningMinor != 100_000 || got.Data.ClosingMinor != 140_000 {
			t.Errorf("movement read %d to %d, want 100000 to 140000 over the seat's own deal only",
				got.Data.OpeningMinor, got.Data.ClosingMinor)
		}
		for _, deal := range got.Data.Deals {
			if deal.DealID == theirs.String() {
				t.Errorf("movement listed %s, a deal this seat cannot read", theirs)
			}
		}
	})

	t.Run("a masked seat gets the figure the live forecast withholds", func(t *testing.T) {
		got, err := move(e.As(e.Rep1, nil, ownSeat(true)), opening, closing)
		if err != nil {
			t.Fatal(err)
		}
		if got.Data.OpeningMinor != 0 || got.Data.ClosingMinor != 0 || len(got.Data.Deals) != 0 {
			t.Errorf("a masked seat read %+v, want nothing: the live reading counts the masked deal as unpriced", got.Data)
		}
		live, err := registry.Invoke(e.As(e.Rep1, nil, ownSeat(true)), "forecast_readings",
			json.RawMessage(`{"as_of":"2038-02-14","scope_kind":"owner","scope_id":"`+e.Rep1.String()+`"}`))
		if err != nil {
			t.Fatal(err)
		}
		var reading struct {
			Data struct {
				OpenMinor int64 `json:"open_minor"`
			} `json:"data"`
		}
		if err := json.Unmarshal(live, &reading); err != nil || reading.Data.OpenMinor != 0 {
			t.Fatalf("the live reading showed open %d (%v) to the same masked seat, so the movement comparison proves nothing",
				reading.Data.OpenMinor, err)
		}
	})

	t.Run("a snapshot of a population the seat may not measure is not found", func(t *testing.T) {
		_, err := move(e.As(e.Rep1, nil, ownSeat(false)), wsOpening, wsClosing)
		if !errors.Is(err, apperrors.ErrNotFound) {
			t.Fatalf("an own-lens seat read a workspace snapshot pair: err = %v, want ErrNotFound", err)
		}
	})

	t.Run("a seat measuring the workspace still reads it", func(t *testing.T) {
		if _, err := move(admin, wsOpening, wsClosing); err != nil {
			t.Fatalf("an unbounded seat was refused its own workspace snapshots: %v", err)
		}
	})
}
