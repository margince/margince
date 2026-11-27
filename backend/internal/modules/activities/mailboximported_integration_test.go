// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// chatRow seeds a connector-captured chat message in a thread and returns its id.
func (e *loadEnv) chatRow(t *testing.T, audience string, importedBy ...ids.UUID) ids.UUID {
	t.Helper()
	row := ids.NewV7()
	e.exec(t, `INSERT INTO activity (id, kind, channel_provider, direction, subject, occurred_at, audience, thread_key, source, captured_by)
		VALUES ($1, 'message', 'whatsapp', 'inbound', 'hello', now(), $2, 'chat-thread', 'seed', 'connector:whatsapp')`, row, audience)
	for _, seat := range importedBy {
		e.exec(t, `INSERT INTO capture_import (activity_id, user_id) VALUES ($1, $2)`, row, seat)
	}
	return row
}

// chatSeat is a seat reading every activity, with the update grant and seat
// type the case names.
func (e *loadEnv) chatSeat(seat ids.UUID, update bool, seatType principal.SeatType) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + seat.String(), UserID: seat, SeatType: seatType,
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"activity": {Read: true, Update: update}},
			RowScope: principal.RowScopeAll,
		},
	})
}

type importFacts struct{ imported, yours *bool }

func (e *loadEnv) factsAs(ctx context.Context, t *testing.T) map[ids.UUID]importFacts {
	t.Helper()
	rows, _, err := handledStore(e).ListActivities(ctx, ListActivitiesInput{})
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	out := map[ids.UUID]importFacts{}
	for _, r := range rows {
		out[ids.UUID(r.Id)] = importFacts{r.MailboxImported, r.ThreadAudienceYours}
	}
	return out
}

func (f importFacts) reads(facts [2]bool) bool {
	return f.imported != nil && *f.imported == facts[0] && f.yours != nil && *f.yours == facts[1]
}

var (
	importedAndYours = [2]bool{true, true}
	importedNotYours = [2]bool{true, false}
	notImported      = [2]bool{false, false}
)

func TestAListedChatRowSaysWhetherTheCallerMayChangeItsThreadAudience(t *testing.T) {
	e := setupLoad(t)
	mine := e.chatRow(t, "workspace", e.rep)
	theirs := e.chatRow(t, "workspace", e.other)
	orphan := e.chatRow(t, "workspace")
	hidden := e.chatRow(t, "participants", e.other)

	asRep := e.factsAs(e.chatSeat(e.rep, true, principal.SeatFull), t)
	if !asRep[mine].reads(importedAndYours) {
		t.Errorf("the importer's own row reads %+v, want imported and yours", asRep[mine])
	}
	if !asRep[theirs].reads(importedNotYours) {
		t.Errorf("a colleague's import reads %+v, want imported and not yours", asRep[theirs])
	}
	if !asRep[orphan].reads(notImported) {
		t.Errorf("a row whose importer left reads %+v, want neither", asRep[orphan])
	}
	if got, listed := asRep[hidden]; !listed || got.imported != nil || got.yours != nil {
		t.Errorf("a withheld row reads %+v (listed %v), want neither field", got, listed)
	}
	if !e.factsAs(e.chatSeat(e.other, true, principal.SeatFull), t)[theirs].reads(importedAndYours) {
		t.Error("the colleague who imported the row is not told it is theirs")
	}
}

func TestTheThreadControlIsOfferedOnlyWhereTheWriteWouldRun(t *testing.T) {
	e := setupLoad(t)
	mine := e.chatRow(t, "workspace", e.rep)
	archived := e.chatRow(t, "workspace", e.rep)
	e.exec(t, `UPDATE activity SET archived_at = now() WHERE id = $1`, archived)
	restricted := e.chatRow(t, "workspace", e.rep)
	e.exec(t, `INSERT INTO activity_retention_evidence (activity_id, basis, qualified_at, decided_by_name, reason)
		VALUES ($1, 'controller_pin', now(), 'Controller', 'Statutory hold')`, restricted)
	e.exec(t, `UPDATE activity SET retention_class = 'commercial_correspondence', retention_class_at = now(), archived_at = now(),
		restricted_at = now(), restricted_reason = 'statutory retention', restricted_until = now() + interval '6 years'
		WHERE id = $1`, restricted)
	unthreaded := e.chatRow(t, "workspace", e.rep)
	e.exec(t, `UPDATE activity SET thread_key = NULL WHERE id = $1`, unthreaded)

	sharedBox := e.chatRow(t, "workspace")
	team := ids.NewV7()
	e.exec(t, `INSERT INTO team (id, name) VALUES ($1, 'Shared box readers')`, team)
	connection := ids.NewV7()
	e.exec(t, `INSERT INTO capture_connection (id, provider, status, reader_team_id, account_label, mail_posture)
		VALUES ($1, 'imap', 'connected', $2, 'ops@example.test', 'held')`, connection, team)
	e.exec(t, `INSERT INTO capture_import (activity_id, connection_id) VALUES ($1, $2)`, sharedBox, connection)

	full := e.factsAs(e.chatSeat(e.rep, true, principal.SeatFull), t)
	if !full[mine].reads(importedAndYours) {
		t.Fatalf("the importer's live threaded row reads %+v, want yours", full[mine])
	}
	if f := full[sharedBox]; f.yours != nil && *f.yours {
		t.Errorf("a shared mailbox's import reads %+v: it holds no seat's thread hold, so the write finds nothing", f)
	}
	for name, id := range map[string]ids.UUID{"archived": archived, "restricted": restricted, "unthreaded": unthreaded} {
		if f, listed := full[id]; listed && f.yours != nil && *f.yours {
			t.Errorf("a %s row is offered the thread write, which finds no row", name)
		}
	}
	for name, ctx := range map[string]context.Context{
		"a read-only seat":                 e.chatSeat(e.rep, true, principal.SeatRead),
		"a caller without activity:update": e.chatSeat(e.rep, false, principal.SeatFull),
	} {
		if f := e.factsAs(ctx, t)[mine]; !f.reads(importedNotYours) {
			t.Errorf("%s reads %+v, want imported and not yours", name, f)
		}
	}
}

func TestTheDrawerOffersTheThreadWriteOnlyToAnImporter(t *testing.T) {
	e := setupLoad(t)
	imported := e.chatRow(t, "workspace", e.rep)
	sentNotImported := e.chatRow(t, "workspace", e.other)
	e.exec(t, `UPDATE activity SET captured_by = 'connector:gmail:' || $2::text WHERE id = $1`, sentNotImported, e.rep)
	for name, c := range map[string]struct {
		row  ids.UUID
		want bool
	}{"an importer": {imported, true}, "a seat the row was only stamped with": {sentNotImported, false}} {
		var got bool
		err := storeKnowing(e).db.Tx(e.chatSeat(e.rep, true, principal.SeatFull), func(tx pgx.Tx) error {
			var err error
			got, err = callerHoldsThreadWrite(e.chatSeat(e.rep, true, principal.SeatFull), tx, ids.ActivityID{UUID: c.row})
			return err
		})
		if err != nil || got != c.want {
			t.Errorf("%s: drawer holds the write = %v (err %v), want %v", name, got, err, c.want)
		}
	}
}
