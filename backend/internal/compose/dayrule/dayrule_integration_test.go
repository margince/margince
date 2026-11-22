// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package dayrule_test

// A person in Ho Chi Minh City on an installation in Berlin. At 18:30 UTC on
// 15 June it is the 15th in Berlin and the 16th for the person.

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/dayrule"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

func TestThePersonalSurfaceFollowsThePersonAndTheCombinedOneTheInstallation(t *testing.T) {
	e := integration.Setup(t)
	e.WsExec(t, `UPDATE setting SET value = '"Europe/Berlin"'::jsonb WHERE key = 'installation.timezone'`)
	e.WsExec(t, `UPDATE app_user SET timezone = 'Asia/Ho_Chi_Minh' WHERE id = $1`, e.Rep1)
	e.WsExec(t, `UPDATE app_user SET timezone = NULL WHERE id = $1`, e.Rep2)
	at := time.Date(2026, 6, 15, 18, 30, 0, 0, time.UTC)

	for _, c := range []struct {
		name    string
		surface occurredday.Surface
		user    ids.UUID
		want    string
	}{
		{"own list of a person with a zone", occurredday.Personal, e.Rep1, "2026-06-16"},
		{"own list of a person without a zone", occurredday.Personal, e.Rep2, "2026-06-15"},
		{"combined screen, same person", occurredday.Combined, e.Rep1, "2026-06-15"},
		{"own list with no person", occurredday.Personal, ids.Nil, "2026-06-15"},
	} {
		t.Run(c.name, func(t *testing.T) {
			var goDay time.Time
			err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
				var err error
				goDay, _, err = dayrule.DayAt(e.Admin(), tx, c.surface, c.user, at)
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			if got := goDay.Format(time.DateOnly); got != c.want {
				t.Errorf("Go day = %s, want %s", got, c.want)
			}
			sqlDay := e.WsScalar(t, `SELECT `+occurredday.DaySQL(c.surface, "$1::timestamptz", "$2::uuid")+`::text`, at, c.user)
			if sqlDay != c.want {
				t.Errorf("SQL day = %s, want %s: the SQL twin must agree with the Go rule", sqlDay, c.want)
			}
		})
	}
}

func TestTheActingReadersZoneIsTheirOwnWithTheInstallationAsFallback(t *testing.T) {
	e := integration.Setup(t)
	e.WsExec(t, `UPDATE setting SET value = '"Europe/Berlin"'::jsonb WHERE key = 'installation.timezone'`)
	e.WsExec(t, `UPDATE app_user SET timezone = 'Asia/Ho_Chi_Minh' WHERE id = $1`, e.Rep1)
	e.WsExec(t, `UPDATE app_user SET timezone = NULL WHERE id = $1`, e.Rep2)
	zoneOf := dayrule.ActingReaderZone(e.Pool)
	for user, want := range map[ids.UUID]string{e.Rep1: "Asia/Ho_Chi_Minh", e.Rep2: "Europe/Berlin"} {
		ctx := e.As(user, nil, integration.RepPerms)
		loc, err := zoneOf(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if loc.String() != want {
			t.Errorf("zone for %s = %s, want %s", user, loc, want)
		}
	}
}
