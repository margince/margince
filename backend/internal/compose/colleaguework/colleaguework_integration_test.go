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
	"github.com/margince/margince/backend/internal/shared/kernel/meetingoutcome"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// workPerms is a rep who may make introductions and log meetings. Its team
// row scope keeps Team2's private contacts out of Rep1's reach.
var workPerms = principal.Permissions{
	RoleKeys: []string{"rep"},
	Objects: map[string]principal.ObjectGrant{
		introductions.Object: {Create: true, Read: true, Update: true},
		"contact":            {Create: true, Read: true, Update: true},
		"activity":           {Create: true, Read: true, Update: true},
	},
	RowScope: principal.RowScopeTeam,
}

var (
	june  = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	july  = time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	ninth = time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC)
	aug   = time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
)

type workEnv struct {
	e      *integration.Env
	intros *introductions.Store
}

func setupWork(t *testing.T) workEnv {
	t.Helper()
	e := integration.Setup(t)
	clock := func() time.Time { return time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC) }
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
// answers what each seat was credited with.
func (w workEnv) read(reader context.Context, t *testing.T, figure Figure, from, until time.Time) map[ids.UUID]int {
	t.Helper()
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	bind := func(v any) string { return fmt.Sprintf("$%d", arg(v)) }
	query, err := figure.SQL(reader, Reading{Credited: "true", Fields: "true", From: bind(from), Until: bind(until), Limit: bind(100), Label: bind("Work")}, arg)
	if err != nil {
		t.Fatal(err)
	}
	credited := map[ids.UUID]int{}
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
			credited[seat]++
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatalf("reading the figure: %v", err)
	}
	return credited
}

// record writes a recorded introduction dated on, and dates its recording at.
func (w workEnv) record(recorder context.Context, t *testing.T, introducer, introduced ids.UUID, on, at time.Time) ids.UUID {
	t.Helper()
	contact := w.e.SeedContact(t, "Buyer "+at.Format(time.RFC3339), &w.e.Rep1)
	id, err := w.intros.Record(recorder, introductions.NewRecorded{ContactID: contact, IntroducerUser: introducer, IntroducedUser: introduced, On: on, Channel: "email"})
	if err != nil {
		t.Fatalf("recording the introduction: %v", err)
	}
	w.e.WsExec(t, `UPDATE intro_request SET created_at = $2 WHERE id = $1`, id, at)
	return id
}

// introduce runs an ask of introducer to its handshake, at the store's clock.
func (w workEnv) introduce(t *testing.T, asker, introducer, contact ids.UUID) {
	t.Helper()
	ask, err := w.intros.Create(w.seat(asker), introductions.NewRequest{
		ContactID: contact, IntroducerUser: introducer, RouteType: "direct",
		InternalReason: "they run procurement", DueAt: time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.intros.Decide(w.seat(introducer), ask, introductions.StatusAccepted, "", nil, 1); err != nil {
		t.Fatal(err)
	}
	if err := w.intros.Complete(w.seat(introducer), ask, nil, 2); err != nil {
		t.Fatal(err)
	}
}

// An ask counts when it was completed, and a recorded introduction when it
// was recorded. The date a recording names cannot fill a past month.
func TestAnIntroductionCountsWhenItWasMadeOrRecorded(t *testing.T) {
	w := setupWork(t)
	rep1 := w.seat(w.e.Rep1)
	w.introduce(t, w.e.Rep2, w.e.Rep1, w.e.SeedContact(t, "Asked Buyer", &w.e.Rep1))
	w.record(rep1, t, w.e.Rep1, w.e.Rep2, time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC))
	w.record(w.seat(w.e.Rep2), t, w.e.Rep2, w.e.Rep1, time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC), aug)
	archived := w.record(rep1, t, w.e.Rep1, w.e.Rep2, ninth, ninth)
	w.e.WsExec(t, `UPDATE intro_request SET archived_at = now() WHERE id = $1`, archived)

	if credited := w.read(rep1, t, IntroductionsMade, july, aug); credited[w.e.Rep1] != 2 || len(credited) != 1 {
		t.Errorf("July credits %v, want Rep1's completed ask and the June introduction recorded in July", credited)
	}
	if credited := w.read(rep1, t, IntroductionsMade, june, july); len(credited) != 0 {
		t.Errorf("June credits %v, want none: an introduction recorded in July is not June's", credited)
	}
}

// An ask is its two colleagues' alone. A recorded introduction is anyone's who
// may open its contact.
func TestAnIntroductionCountsOnlyForAReaderWhoMayReadIt(t *testing.T) {
	w := setupWork(t)
	w.introduce(t, w.e.Rep2, w.e.Rep3, w.e.SeedContact(t, "Asked Buyer", &w.e.Rep2))
	w.record(w.seat(w.e.Rep3), t, w.e.Rep3, w.e.Rep2, ninth, ninth)
	for _, tc := range []struct {
		name   string
		reader ids.UUID
		want   int
	}{{"a colleague in neither", w.e.Rep1, 1}, {"the asker", w.e.Rep2, 2}, {"the introducer", w.e.Rep3, 2}} {
		if credited := w.read(w.seat(tc.reader), t, IntroductionsMade, july, aug); credited[w.e.Rep3] != tc.want {
			t.Errorf("%s reads %v, want %d for the introducer", tc.name, credited, tc.want)
		}
	}
}

// meeting logs one meeting as booker for host, starting a week after at, and
// dates its creation at. shape changes the input before it is logged.
func (w workEnv) meeting(booker context.Context, t *testing.T, host ids.UUID, at time.Time, shape func(*activities.LogActivityInput)) ids.UUID {
	t.Helper()
	subject, status, hostID, start := "Demo", "booked", ids.From[ids.UserKind](host), at.Add(7*24*time.Hour)
	in := activities.LogActivityInput{
		Kind: "meeting", Subject: &subject, OccurredAt: &start, MeetingStatus: &status, HostUserID: &hostID, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: w.e.SeedContact(t, "Guest", &w.e.Rep1)}},
	}
	if shape != nil {
		shape(&in)
	}
	logged, _, err := w.e.Activities.LogActivity(booker, in)
	if err != nil {
		t.Fatalf("logging the meeting: %v", err)
	}
	w.e.WsExec(t, `UPDATE activity SET created_at = $2 WHERE id = $1`, ids.UUID(logged.Id), at)
	return ids.UUID(logged.Id)
}

func sourced(system string) func(*activities.LogActivityInput) {
	return func(in *activities.LogActivityInput) {
		id := "event-" + system
		in.SourceSystem, in.SourceID = &system, &id
	}
}

// A meeting counts for the seat that booked it, or whose agent did, when a
// colleague hosts it. Hosting it oneself is no booking for a colleague, and
// neither is logging it once it happened. Imports, transcripts, calendar
// syncs, system work and an agent acting for nobody book nothing.
func TestAMeetingCountsForTheSeatThatBookedItForAColleague(t *testing.T) {
	w := setupWork(t)
	rep1, rep2 := w.seat(w.e.Rep1), w.e.Rep2
	agent := w.e.AgentOnPassport(t, w.e.Rep1, []ids.UUID{w.e.Team1}, workPerms, ids.From[ids.PassportKind](w.e.AgentPassport))
	nobodys := principal.WithActor(agent, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:" + w.e.AgentPassport.String(), PassportID: w.e.AgentPassport,
		SeatType: principal.SeatFull, Permissions: workPerms,
	})
	w.meeting(rep1, t, rep2, july, nil)
	w.meeting(agent, t, rep2, ninth, nil)
	w.meeting(rep1, t, w.e.Rep1, ninth, nil)
	w.meeting(rep1, t, rep2, ninth, func(in *activities.LogActivityInput) { in.MeetingStatus = new("held") })
	w.meeting(rep1, t, rep2, ninth, func(in *activities.LogActivityInput) {
		in.MeetingStatus, in.OccurredAt = nil, new(ninth.Add(-24*time.Hour))
	})
	w.meeting(rep1, t, rep2, aug, nil)
	w.meeting(rep1, t, rep2, ninth, sourced("mirror:hubspot"))
	w.meeting(rep1, t, rep2, ninth, sourced(meetingoutcome.TranscriptSourceSystem))
	w.meeting(w.e.ConnectorOwnerCtx(w.e.Rep1, "google"), t, rep2, july, nil)
	w.meeting(w.e.AutomationCtx(w.e.Rep1), t, rep2, ninth, nil)
	w.meeting(nobodys, t, rep2, ninth, nil)
	archived := w.meeting(rep1, t, rep2, ninth, nil)
	if _, err := w.e.Activities.ArchiveActivity(w.e.Admin(), ids.From[ids.ActivityKind](archived), nil); err != nil {
		t.Fatal(err)
	}
	if credited := w.read(rep1, t, MeetingsBookedForColleagues, july, aug); credited[w.e.Rep1] != 2 || len(credited) != 1 {
		t.Errorf("credits %v, want the two bookings for Rep2 in July, by Rep1 and by Rep1's agent", credited)
	}
}

// The host is the one the booking named. A later host repair or handover
// moves no credit, for or against the booker.
func TestAHostChangeAfterBookingMovesNoCredit(t *testing.T) {
	w := setupWork(t)
	rep1 := w.seat(w.e.Rep1)
	repaired := w.meeting(rep1, t, w.e.Rep2, ninth, nil)
	handedOver := w.meeting(rep1, t, w.e.Rep2, ninth, nil)
	self := w.meeting(rep1, t, w.e.Rep1, ninth, nil)
	w.e.WsExec(t, `UPDATE activity SET host_user_id = $2 WHERE id = ANY($1)`, []ids.UUID{repaired, handedOver}, w.e.Rep1)
	w.e.WsExec(t, `UPDATE activity SET host_user_id = $2 WHERE id = $1`, self, w.e.Rep2)
	if credited := w.read(rep1, t, MeetingsBookedForColleagues, july, aug); credited[w.e.Rep1] != 2 || len(credited) != 1 {
		t.Errorf("credits %v, want the two bookings Rep1 made for Rep2", credited)
	}
}

// A meeting the reader may not open counts nowhere for them: one limited to
// its participants, and one about a contact private to another colleague.
func TestABookingTheReaderMayNotOpenIsNotCounted(t *testing.T) {
	w := setupWork(t)
	rep2 := w.seat(w.e.Rep2)
	limited := w.meeting(rep2, t, w.e.Rep3, ninth, nil)
	if _, err := w.e.Activities.SetAudience(rep2, ids.From[ids.ActivityKind](limited), activities.SetAudienceInput{Audience: "participants"}); err != nil {
		t.Fatal(err)
	}
	private := w.e.SeedContact(t, "Private Guest", &w.e.Rep3)
	w.e.MakeCapturePrivate(t, "contact", private, w.e.Rep3)
	w.meeting(w.seat(w.e.Rep3), t, w.e.Rep2, ninth, func(in *activities.LogActivityInput) {
		in.Links = []activities.ActivityLinkInput{{EntityType: "contact", EntityID: private}}
	})
	if credited := w.read(w.seat(w.e.Rep1), t, MeetingsBookedForColleagues, july, aug); len(credited) != 0 {
		t.Errorf("a reader who may open neither meeting was credited %v", credited)
	}
	if credited := w.read(rep2, t, MeetingsBookedForColleagues, july, aug); credited[w.e.Rep2] != 1 || credited[w.e.Rep3] != 0 {
		t.Errorf("Rep2 reads %v, want their own limited booking and not Rep3's private one", credited)
	}
}
