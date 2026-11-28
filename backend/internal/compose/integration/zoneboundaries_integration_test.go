// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	losAngeles = "America/Los_Angeles"
	auckland   = "Pacific/Auckland"
)

func useInstallationZone(t *testing.T, e *Env, zone string) {
	t.Helper()
	e.WsExec(t, `UPDATE setting SET value = to_jsonb($1::text) WHERE key = 'installation.timezone'`, zone)
}

func inWorkspaceTx(t *testing.T, e *Env, fn func(ctx context.Context, tx pgx.Tx) error) {
	t.Helper()
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	if err := database.WithTx(ctx, e.Pool, func(tx pgx.Tx) error { return fn(ctx, tx) }); err != nil {
		t.Fatal(err)
	}
}

// A pause ends at the start of the picked day in the owner's own zone, and in
// the installation's zone when the owner has none.
func TestAPauseEndsAtTheStartOfItsDayInTheOwnersZone(t *testing.T) {
	e := Setup(t)
	useInstallationZone(t, e, losAngeles)
	pipeline, open, _ := DealFixture(t, e)
	deal := e.SeedDeal(t, "Waiting", pipeline, open, &e.Rep1)
	owner := OwnerConn(t)
	ctx := context.Background()

	for _, tc := range []struct {
		name, ownerZone string
		want            time.Time
	}{
		{"west of UTC", losAngeles, time.Date(2026, 8, 20, 7, 0, 0, 0, time.UTC)},
		{"east of UTC", auckland, time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)},
		{"no zone of its own, the installation's applies", "", time.Date(2026, 8, 20, 7, 0, 0, 0, time.UTC)},
	} {
		if _, err := owner.Exec(ctx, `UPDATE app_user SET timezone = NULLIF($1, '') WHERE id = $2`, tc.ownerZone, e.Rep1); err != nil {
			t.Fatal(err)
		}
		if _, err := owner.Exec(ctx, `UPDATE deal SET wait_until = '2026-08-20' WHERE id = $1`, deal); err != nil {
			t.Fatal(err)
		}
		var got *time.Time
		inWorkspaceTx(t, e, func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT `+deals.PauseEndsSQL("d")+` FROM deal d WHERE d.id = $1`, deal).Scan(&got)
		})
		if got == nil || !got.Equal(tc.want) {
			t.Errorf("%s: the pause ends at %v, want %v", tc.name, got, tc.want)
		}
		read, err := e.Deals.GetDeal(e.Admin(), ids.From[ids.DealKind](deal), storekit.LiveOnly)
		if err != nil {
			t.Fatal(err)
		}
		if read.WaitEndsAt == nil || !read.WaitEndsAt.Equal(tc.want) {
			t.Errorf("%s: the deal read carries the pause end %v, want %v", tc.name, read.WaitEndsAt, tc.want)
		}
	}

	if _, err := owner.Exec(ctx, `UPDATE deal SET wait_until = NULL WHERE id = $1`, deal); err != nil {
		t.Fatal(err)
	}
	var none *time.Time
	inWorkspaceTx(t, e, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT `+deals.PauseEndsSQL("d")+` FROM deal d WHERE d.id = $1`, deal).Scan(&none)
	})
	if none != nil {
		t.Errorf("a deal with no pause ends one at %v", none)
	}
}

// A departure day is over when the installation's day reaches it. The database
// clock cannot be pinned, so the zone is chosen to sit on another day than UTC.
// Kiritimati is ahead of UTC and Pago Pago behind it. One of the two always
// sits on another day, whatever time the suite runs at.
func TestADepartureDayIsJudgedInTheInstallationZone(t *testing.T) {
	e := Setup(t)
	// The predicate reads the database clock. So the zone and its day come from
	// that clock, with plain AT TIME ZONE.
	var zone string
	var today time.Time
	inWorkspaceTx(t, e, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			WITH pick AS (
			  SELECT CASE WHEN (now() AT TIME ZONE 'Pacific/Kiritimati')::date <> (now() AT TIME ZONE 'UTC')::date
			              THEN 'Pacific/Kiritimati' ELSE 'Pacific/Pago_Pago' END AS zone)
			SELECT zone, (now() AT TIME ZONE zone)::date FROM pick`).Scan(&zone, &today)
	})
	useInstallationZone(t, e, zone)

	current := func(end time.Time) bool {
		var held bool
		inWorkspaceTx(t, e, func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT `+employment.IsCurrentSQL("$1::date"), end).Scan(&held)
		})
		return held
	}
	if current(today) {
		t.Errorf("a job ending on the installation's day %s still counts as current in %s", today.Format(time.DateOnly), zone)
	}
	if !current(today.AddDate(0, 0, 1)) {
		t.Errorf("a job ending the day after the installation's day %s is already over in %s", today.Format(time.DateOnly), zone)
	}
}

// One instant names one rate day, the installation's, in the lookup that
// converts a figure and in the SQL that converts a list.
func TestARateIsReadAtTheInstallationDayOfAnInstant(t *testing.T) {
	west := time.Date(2026, 8, 20, 6, 30, 0, 0, time.UTC)
	east := time.Date(2026, 8, 19, 18, 30, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, zone string
		at         time.Time
		rate       string
		day        string
	}{
		{"west of UTC, 23:30 on the 19th local", losAngeles, west, "2.0000000000", "2026-08-19"},
		{"east of UTC, 06:30 on the 20th local", auckland, east, "5.0000000000", "2026-08-20"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := Setup(t)
			useInstallationZone(t, e, tc.zone)
			for day, rate := range map[string]string{"2026-08-18": "1", "2026-08-19": "2", "2026-08-20": "5", "2026-08-21": "9"} {
				e.WsExec(t, `INSERT INTO fx_rate (from_currency, to_currency, rate, rate_date) VALUES ('USD', 'EUR', $1::numeric, $2::date)`, rate, day)
			}
			pipeline, open, _ := DealFixture(t, e)
			deal := e.SeedDeal(t, "Priced", pipeline, open, &e.Rep1)
			e.WsExec(t, `UPDATE deal SET amount_minor = 10000, currency = 'USD' WHERE id = $1`, deal)

			inWorkspaceTx(t, e, func(ctx context.Context, tx pgx.Tx) error {
				got, found, err := deals.NewFXRates("EUR", tc.at).For(ctx, tx, "USD")
				if err != nil || !found {
					t.Fatalf("the lookup found %v, %v", found, err)
				}
				if on := got.On.Format(time.DateOnly); on != tc.day {
					t.Errorf("the lookup reads the rate dated %s, want %s", on, tc.day)
				}
				identity, _, err := deals.NewFXRates("EUR", tc.at).For(ctx, tx, "EUR")
				if err != nil || identity.On.Format(time.DateOnly) != tc.day {
					t.Errorf("the identity rate is dated %v, want %s (%v)", identity.On, tc.day, err)
				}
				var converted int64
				err = tx.QueryRow(ctx, `SELECT `+deals.BaseValueSQL("$1::timestamptz", "'EUR'", "d")+` FROM deal d WHERE d.id = $2`,
					tc.at, deal).Scan(&converted)
				if err != nil {
					t.Fatal(err)
				}
				if want := map[string]int64{"2.0000000000": 20000, "5.0000000000": 50000}[tc.rate]; converted != want {
					t.Errorf("the SQL converts 10000 to %d, want %d", converted, want)
				}
				return nil
			})
		})
	}
}

// A price takes effect on the installation's day, so one entered "today" is
// accepted and yesterday's is refused, wherever that day sits against UTC.
func TestAModelRateIsEffectiveOnTheInstallationDay(t *testing.T) {
	for _, tc := range []struct {
		name, zone string
		at         time.Time
		today      string
		yesterday  string
	}{
		{"west of UTC", losAngeles, time.Date(2026, 8, 20, 6, 30, 0, 0, time.UTC), "2026-08-19", "2026-08-18"},
		{"east of UTC", auckland, time.Date(2026, 8, 19, 18, 30, 0, 0, time.UTC), "2026-08-20", "2026-08-19"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := Setup(t)
			useInstallationZone(t, e, tc.zone)
			store := ai.NewRateStore(e.DB()).SetClock(func() time.Time { return tc.at })
			in := ai.SetModelRateInput{
				Provider: "anthropic", ModelID: "claude-zone-check",
				InputUsd: "5", OutputUsd: "25", CacheReadUsd: "0.5", CacheWriteUsd: "6.25",
			}
			row, err := store.SetModelRate(e.Admin(), in)
			if err != nil {
				t.Fatalf("a price with no day: %v", err)
			}
			if got := row.EffectiveDate.Format(time.DateOnly); got != tc.today {
				t.Errorf("a price with no day takes effect on %s, want %s", got, tc.today)
			}
			past, perr := time.Parse(time.DateOnly, tc.yesterday)
			if perr != nil {
				t.Fatal(perr)
			}
			in.EffectiveDate = past
			if _, err := store.SetModelRate(e.Admin(), in); err == nil {
				t.Errorf("the installation's yesterday %s was accepted: %v", tc.yesterday, err)
			}
			rate, err := store.RateFor(e.Admin(), "anthropic", "claude-zone-check", tc.at)
			if err != nil || rate == nil {
				t.Errorf("a price effective today is not in force at %v: %v, %v", tc.at, rate, err)
			}
		})
	}
}

// The day an instant falls on is cut by one function in SQL, the installation's.
func TestTheInstallationDayOfAnInstantWestAndEastOfUTC(t *testing.T) {
	e := Setup(t)
	for _, tc := range []struct{ zone, at, day string }{
		{losAngeles, "2026-08-20T06:30:00Z", "2026-08-19"},
		{auckland, "2026-08-19T18:30:00Z", "2026-08-20"},
	} {
		useInstallationZone(t, e, tc.zone)
		var got time.Time
		inWorkspaceTx(t, e, func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT `+occurredday.TodaySQL("$1::timestamptz"), tc.at).Scan(&got)
		})
		if got.Format(time.DateOnly) != tc.day {
			t.Errorf("%s at %s is the day %s, want %s", tc.zone, tc.at, got.Format(time.DateOnly), tc.day)
		}
	}
}
