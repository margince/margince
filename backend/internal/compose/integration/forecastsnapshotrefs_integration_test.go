// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// forecast_readings hands out the snapshot ids forecast_movement takes. The
// listing rides the reading's resolved scope, so the interesting cases are the
// ones where that scope refuses the caller: a rep must not learn that the
// workspace was frozen, or when, by asking for it.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// readingSnapshots is the tool's sealed answer, narrowed to the listing.
type readingSnapshots struct {
	Data struct {
		Snapshots []struct {
			ID      string `json:"id"`
			TakenAt string `json:"taken_at"`
			Trigger string `json:"trigger"`
		} `json:"snapshots"`
	} `json:"data"`
}

// freezeWorkspace takes one workspace snapshot of the quarter holding `at`.
func freezeWorkspace(ctx context.Context, t *testing.T, e *Env, at time.Time) ids.UUID {
	t.Helper()
	store := forecasting.NewStore(compose.InstallationDB(e.Pool))
	var id ids.UUID
	if err := store.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		period, base, err := compose.ForecastPeriodAt(ctx, tx, forecasting.PeriodQuarter, at)
		if err != nil {
			return err
		}
		id, err = store.TakeSnapshot(ctx, tx, forecasting.NewSnapshot{
			Period: period, Scope: forecasting.Scope{Kind: forecasting.ScopeWorkspace},
			Trigger: forecasting.TriggerRecheck, BaseCurrency: base,
			Readings: forecasting.Readings{}, TakenAt: at,
		})
		return err
	}); err != nil {
		t.Fatalf("freezing a workspace forecast: %v", err)
	}
	return id
}

func TestForecastReadingsListsTheSnapshotsForecastMovementTakes(t *testing.T) {
	e := Setup(t)
	registry := compose.NewRegistry(e.Pool, compose.SendPath{})
	admin := e.Admin()
	args := json.RawMessage(`{"as_of":"2036-02-14"}`)

	read := func(ctx context.Context) readingSnapshots {
		t.Helper()
		out, err := registry.Invoke(ctx, "forecast_readings", args)
		if err != nil {
			t.Fatalf("forecast_readings: %v", err)
		}
		var body readingSnapshots
		if err := json.Unmarshal(out, &body); err != nil {
			t.Fatal(err)
		}
		return body
	}

	raw, err := registry.Invoke(admin, "forecast_readings", args)
	if err != nil {
		t.Fatalf("forecast_readings: %v", err)
	}
	var generic struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Fatal(err)
	}
	if string(generic.Data["snapshots"]) != "[]" {
		t.Fatalf("a period nobody froze answered snapshots=%s, want []", generic.Data["snapshots"])
	}

	opening := freezeWorkspace(admin, t, e, time.Date(2036, 2, 1, 9, 0, 0, 0, time.UTC))
	closing := freezeWorkspace(admin, t, e, time.Date(2036, 2, 10, 9, 0, 0, 0, time.UTC))

	listed := read(admin).Data.Snapshots
	if len(listed) != 2 || listed[0].ID != closing.String() || listed[1].ID != opening.String() {
		t.Fatalf("listed %+v, want the closing then the opening snapshot", listed)
	}
	if listed[0].Trigger != forecasting.TriggerRecheck || listed[0].TakenAt != "2036-02-10T09:00:00Z" {
		t.Errorf("the reference carries %+v, want its trigger and RFC 3339 instant", listed[0])
	}

	// The listed ids are what the movement tool accepts, in one period.
	if _, err := registry.Invoke(admin, "forecast_movement",
		json.RawMessage(`{"from":"`+opening.String()+`","to":"`+closing.String()+`"}`)); err != nil {
		t.Fatalf("forecast_movement refused the ids forecast_readings listed: %v", err)
	}
}

func TestForecastReadingsListsNoSnapshotsToASeatThatCannotReadTheScope(t *testing.T) {
	e := Setup(t)
	registry := compose.NewRegistry(e.Pool, compose.SendPath{})
	freezeWorkspace(e.Admin(), t, e, time.Date(2037, 2, 1, 9, 0, 0, 0, time.UTC))

	rep := RepPerms
	rep.Objects = map[string]principal.ObjectGrant{
		"forecast": {Read: true}, "deal": {Read: true}, "pipeline": {Read: true},
		"installation_settings": {Read: true},
	}
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, rep)

	// The rep's own default population is answered, and the workspace freeze is
	// not theirs: it must not be listed under it.
	own, err := registry.Invoke(ctx, "forecast_readings", json.RawMessage(`{"as_of":"2037-02-14"}`))
	if err != nil {
		t.Fatalf("a rep's own default forecast was refused: %v", err)
	}
	var body readingSnapshots
	if err := json.Unmarshal(own, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data.Snapshots) != 0 {
		t.Errorf("a rep's default population listed the workspace's snapshots: %+v", body.Data.Snapshots)
	}

	// Naming the workspace is a wider population than the seat may measure: the
	// reading is refused before anything is listed.
	out, err := registry.Invoke(ctx, "forecast_readings",
		json.RawMessage(`{"as_of":"2037-02-14","scope_kind":"workspace"}`))
	if !errors.Is(err, apperrors.ErrPermissionDenied) && !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("a rep naming the workspace got (%s, %v), want a scope refusal", out, err)
	}
}

func TestForecastReadingsRefusesASeatWithoutTheForecastGrant(t *testing.T) {
	e := Setup(t)
	registry := compose.NewRegistry(e.Pool, compose.SendPath{})
	freezeWorkspace(e.Admin(), t, e, time.Date(2038, 2, 1, 9, 0, 0, 0, time.UTC))

	noForecast := ReadOnlyPerms
	noForecast.Objects = map[string]principal.ObjectGrant{
		"deal": {Read: true}, "pipeline": {Read: true}, "installation_settings": {Read: true},
	}
	out, err := registry.Invoke(e.As(e.Rep1, []ids.UUID{e.Team1}, noForecast), "forecast_readings",
		json.RawMessage(`{"as_of":"2038-02-14"}`))
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a seat without forecast.read got (%s, %v), want ErrPermissionDenied", out, err)
	}
}
