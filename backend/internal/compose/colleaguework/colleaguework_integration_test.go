// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package colleaguework

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/introductions"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// workPerms is a rep who may make introductions and log meetings. Its team
// row scope keeps Team2's contacts out of Rep1's reach.
var workPerms = principal.Permissions{
	RoleKeys: []string{"rep"},
	Objects: map[string]principal.ObjectGrant{
		introductions.Object: {Create: true, Read: true, Update: true},
		"contact":            {Create: true, Read: true, Update: true},
		"activity":           {Create: true, Read: true, Update: true},
	},
	RowScope: principal.RowScopeTeam,
}

type workEnv struct {
	e      *integration.Env
	intros *introductions.Store
}

func setupWork(t *testing.T) workEnv {
	t.Helper()
	e := integration.Setup(t)
	clock := func() time.Time { return time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC) }
	return workEnv{e: e, intros: introductions.NewStore(e.DB(), clock)}
}

func (w workEnv) seat(user ids.UUID) context.Context {
	teams := []ids.UUID{w.e.Team1}
	if user == w.e.Rep3 {
		teams = []ids.UUID{w.e.Team2}
	}
	return w.e.As(user, teams, workPerms)
}

// read runs figure for reader over [from, until), crediting every seat, and
// answers what each seat was credited with and whether a row was withheld.
func (w workEnv) read(reader context.Context, t *testing.T, figure Figure, from, until time.Time) (map[ids.UUID]int, bool) {
	t.Helper()
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	bind := func(v any) string { return fmt.Sprintf("$%d", arg(v)) }
	query, err := figure.SQL(reader, Reading{Credited: "true", Fields: "true", From: bind(from), Until: bind(until), Limit: bind(100), Label: bind("Work")}, arg)
	if err != nil {
		t.Fatal(err)
	}
	credited, withheld := map[ids.UUID]int{}, false
	err = w.e.DB().Tx(reader, func(tx pgx.Tx) error {
		rows, err := tx.Query(reader, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var key, label, name, provenance string
			var source, seat ids.UUID
			var at time.Time
			var precision *string
			var placed *time.Time
			if err := rows.Scan(&key, &source, &label, &seat, &name, &at, &precision, &placed, &provenance); err != nil {
				return err
			}
			if provenance == Withheld {
				withheld = true
				continue
			}
			credited[seat]++
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatalf("reading the figure: %v", err)
	}
	return credited, withheld
}

func (w workEnv) record(recorder context.Context, t *testing.T, contact, introducer, introduced ids.UUID, on time.Time) {
	t.Helper()
	if _, err := w.intros.Record(recorder, introductions.NewRecorded{ContactID: contact, IntroducerUser: introducer, IntroducedUser: introduced, On: on, Channel: "email"}); err != nil {
		t.Fatalf("recording the introduction: %v", err)
	}
}

// A recorded introduction holds only its date, so it counts in the period
// that date falls in where the installation is. Read as a UTC instant, 1 July
// would fall on 30 June in Los Angeles.
func TestAnIntroductionCountsForItsIntroducerOnTheLocalDayItHappened(t *testing.T) {
	w := setupWork(t)
	if _, err := integration.OwnerConn(t).Exec(context.Background(),
		`UPDATE setting SET value = '"America/Los_Angeles"'::jsonb WHERE key = 'installation.timezone'`); err != nil {
		t.Fatal(err)
	}
	rep1, rep2 := w.seat(w.e.Rep1), w.seat(w.e.Rep2)
	for _, intro := range []struct {
		recorder          context.Context
		introducer, other ids.UUID
		on                time.Time
	}{
		{rep1, w.e.Rep1, w.e.Rep2, time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)},
		{rep1, w.e.Rep1, w.e.Rep2, time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)},
		{rep1, w.e.Rep1, w.e.Rep2, time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)},
		{rep2, w.e.Rep2, w.e.Rep1, time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC)},
	} {
		w.record(intro.recorder, t, w.e.SeedContact(t, "Buyer "+intro.on.Format(time.DateOnly), &w.e.Rep1), intro.introducer, intro.other, intro.on)
	}
	july, _ := time.LoadLocation("America/Los_Angeles")
	credited, withheld := w.read(rep1, t, IntroductionsMade, time.Date(2026, 7, 1, 0, 0, 0, 0, july), time.Date(2026, 8, 1, 0, 0, 0, 0, july))
	if credited[w.e.Rep1] != 2 || credited[w.e.Rep2] != 1 || withheld {
		t.Errorf("July credits %v (withheld %t), want two for the introducer of 1 and 31 July and one for the other colleague", credited, withheld)
	}
}

// An ask is its two colleagues' alone, so a third reads it as left out. A
// recorded introduction is anyone's who may open its contact.
func TestAnIntroductionCountsOnlyForAReaderWhoMayReadIt(t *testing.T) {
	w := setupWork(t)
	rep1, rep2, rep3 := w.seat(w.e.Rep1), w.seat(w.e.Rep2), w.seat(w.e.Rep3)
	ask, err := w.intros.Create(rep2, introductions.NewRequest{
		ContactID: w.e.SeedContact(t, "Asked Buyer", &w.e.Rep2), IntroducerUser: w.e.Rep3, RouteType: "direct",
		InternalReason: "they run procurement", DueAt: time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.intros.Decide(rep3, ask, introductions.StatusAccepted, "", nil, 1); err != nil {
		t.Fatal(err)
	}
	if err := w.intros.Complete(rep3, ask, nil, 2); err != nil {
		t.Fatal(err)
	}
	w.record(rep3, t, w.e.SeedContact(t, "Team2 Buyer", &w.e.Rep3), w.e.Rep3, w.e.Rep2, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))
	from, until := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name     string
		reader   context.Context
		want     int
		withheld bool
	}{{"a colleague in neither", rep1, 1, true}, {"the asker", rep2, 2, false}, {"the introducer", rep3, 2, false}} {
		credited, withheld := w.read(tc.reader, t, IntroductionsMade, from, until)
		if credited[w.e.Rep3] != tc.want || withheld != tc.withheld {
			t.Errorf("%s reads %v (withheld %t), want %d for the introducer (withheld %t)", tc.name, credited, withheld, tc.want, tc.withheld)
		}
	}
}

// meeting logs one meeting as booker for host, and dates its creation at.
func (w workEnv) meeting(booker context.Context, t *testing.T, host ids.UUID, status string, at time.Time) ids.UUID {
	t.Helper()
	subject, hostID, start := "Demo", ids.From[ids.UserKind](host), at.Add(7*24*time.Hour)
	logged, _, err := w.e.Activities.LogActivity(booker, activities.LogActivityInput{
		Kind: "meeting", Subject: &subject, OccurredAt: &start, MeetingStatus: &status, HostUserID: &hostID, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: w.e.SeedContact(t, "Guest", &w.e.Rep1)}},
	})
	if err != nil {
		t.Fatalf("logging the meeting: %v", err)
	}
	w.e.WsExec(t, `UPDATE activity SET created_at = $2 WHERE id = $1`, ids.UUID(logged.Id), at)
	return ids.UUID(logged.Id)
}

// A meeting counts for the seat that booked it, or whose agent did, when a
// colleague hosts it. Hosting it oneself, logging it once it was held, or a
// calendar sync on one's behalf is not booking for a colleague.
func TestAMeetingCountsForTheSeatThatBookedItForAColleague(t *testing.T) {
	w := setupWork(t)
	rep1 := w.seat(w.e.Rep1)
	agent := w.e.AgentOnPassport(t, w.e.Rep1, []ids.UUID{w.e.Team1}, workPerms, ids.From[ids.PassportKind](w.e.AgentPassport))
	from, until := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	w.meeting(rep1, t, w.e.Rep2, "booked", from)
	w.meeting(agent, t, w.e.Rep2, "booked", time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC))
	w.meeting(rep1, t, w.e.Rep1, "booked", time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC))
	w.meeting(rep1, t, w.e.Rep2, "held", time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC))
	w.meeting(rep1, t, w.e.Rep2, "booked", until)
	w.meeting(w.e.ConnectorOwnerCtx(w.e.Rep1, "google"), t, w.e.Rep2, "booked", from)
	credited, withheld := w.read(rep1, t, MeetingsBookedForColleagues, from, until)
	if credited[w.e.Rep1] != 2 || len(credited) != 1 || withheld {
		t.Errorf("credits %v (withheld %t), want the two bookings for Rep2 in July, by Rep1 and by Rep1's agent", credited, withheld)
	}
}

// A meeting whose audience leaves a reader out counts nowhere for them.
func TestABookingOutsideTheReadersAudienceIsWithheld(t *testing.T) {
	w := setupWork(t)
	rep2 := w.seat(w.e.Rep2)
	at := time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC)
	limited := w.meeting(rep2, t, w.e.Rep3, "booked", at)
	if _, err := w.e.Activities.SetAudience(rep2, ids.From[ids.ActivityKind](limited), activities.SetAudienceInput{Audience: "participants"}); err != nil {
		t.Fatal(err)
	}
	from, until := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	if credited, withheld := w.read(w.seat(w.e.Rep1), t, MeetingsBookedForColleagues, from, until); len(credited) != 0 || !withheld {
		t.Errorf("a reader outside the audience was credited %v (withheld %t), want nothing and withheld", credited, withheld)
	}
	if credited, withheld := w.read(rep2, t, MeetingsBookedForColleagues, from, until); credited[w.e.Rep2] != 1 || withheld {
		t.Errorf("the booker reads %v (withheld %t), want their one booking", credited, withheld)
	}
}
