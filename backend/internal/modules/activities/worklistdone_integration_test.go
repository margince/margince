// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// Done on any Worklist row, written through MarkWorklistRowsDone and read back
// through the lanes and the Done set the Worklist reads.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// doneNow reads one reader's Done set among these rows, the way the Worklist
// asks about the rows a page could draw.
func doneNow(ctx context.Context, t *testing.T, e *loadEnv, among ...WorklistRowRef) map[WorklistRowRef]time.Time {
	t.Helper()
	var out map[WorklistRowRef]time.Time
	if err := database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		got, err := handledStore(e).DoneRows(ctx, tx, among)
		out = got
		return err
	}); err != nil {
		t.Fatalf("reading rows marked done: %v", err)
	}
	return out
}

func doneTrail(t *testing.T, e *loadEnv, row WorklistRowRef) int {
	t.Helper()
	var n int
	if err := e.owner.QueryRow(context.Background(), `SELECT count(*) FROM audit_log
		 WHERE entity_type = 'worklist_done' AND after->>'source' = $1 AND after->>'row_id' = $2`,
		row.Source, row.RowID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// The two follow-up rows keep their Done on the activity, so their own lanes
// leave them out. One Undo for the batch puts both back.
func TestDoneTakesFollowUpsOffTheListAndUndoPutsThemBack(t *testing.T) {
	e := setupLoad(t)
	s := storeAddressing(e, readerAddress)
	sent, _ := e.sentTo(t, e.customerAt(t, "customer"), "", 3*24*time.Hour)
	met := e.metWith(t, e.customerAt(t, "customer"), e.rep, "held", 3*24*time.Hour)
	if !awaited(t, s, e, sent) || !owesMeetingFollowUp(t, s, e, met) {
		t.Fatal("the message and the meeting are not follow-ups before Done, so this proves nothing")
	}
	batch := []WorklistRowRef{
		{Source: "awaiting_reply", RowID: sent.String()},
		{Source: "meeting_follow_up", RowID: met.String()},
	}

	if err := s.MarkWorklistRowsDone(e.followUpReader(), batch); err != nil {
		t.Fatalf("marking the follow-ups done: %v", err)
	}
	if awaited(t, s, e, sent) {
		t.Error("a message marked done is still a follow-up")
	}
	if owesMeetingFollowUp(t, s, e, met) {
		t.Error("a meeting marked done still owes a follow-up")
	}

	if err := s.ReopenWorklistRows(e.followUpReader(), batch); err != nil {
		t.Fatalf("undoing the batch: %v", err)
	}
	if !awaited(t, s, e, sent) || !owesMeetingFollowUp(t, s, e, met) {
		t.Error("one Undo for the batch did not put both follow-ups back")
	}
}

// A follow-up row names the activity it stands for, and a batch naming any
// other kind is refused whole, so nothing of it is stored.
func TestDoneOnAFollowUpRowRefusesAnotherKindOfActivity(t *testing.T) {
	e := setupLoad(t)
	s := storeAddressing(e, readerAddress)
	sent, _ := e.sentTo(t, e.customerAt(t, "customer"), "", 3*24*time.Hour)
	met := e.metWith(t, e.customerAt(t, "customer"), e.rep, "held", 3*24*time.Hour)
	deal := WorklistRowRef{Source: "deal_at_risk", RowID: ids.NewV7().String()}
	cases := map[string]WorklistRowRef{
		"not_an_outbound_message": {Source: "awaiting_reply", RowID: met.String()},
		"not_a_meeting":           {Source: "meeting_follow_up", RowID: sent.String()},
	}
	for want, wrong := range cases {
		err := s.MarkWorklistRowsDone(e.followUpReader(), []WorklistRowRef{deal, wrong})
		var refused *values.ParseError
		if !errors.As(err, &refused) || refused.Code != want {
			t.Errorf("Done on %+v answered %v, want the %s refusal", wrong, err, want)
		}
	}
	if _, stored := doneNow(e.followUpReader(), t, e, deal)[deal]; stored {
		t.Error("a refused batch still stored the row beside the refused one")
	}
}

// Any other row is kept in the reader's own Done set with its moment, audited,
// and taken back by Undo; a second Undo changes nothing.
func TestDoneOnAnyOtherRowIsTheReadersOwnAndUndoable(t *testing.T) {
	e := setupLoad(t)
	s := handledStore(e)
	rep := e.followUpReader()
	deal := WorklistRowRef{Source: "deal_at_risk", RowID: ids.NewV7().String()}
	group := WorklistRowRef{Source: "batch", RowID: "system_incident:rule-7"}

	if err := s.MarkWorklistRowsDone(rep, []WorklistRowRef{deal, deal, group}); err != nil {
		t.Fatalf("marking the rows done: %v", err)
	}
	other := WorklistRowRef{Source: "bounce", RowID: ids.NewV7().String()}
	if err := s.MarkWorklistRowsDone(rep, []WorklistRowRef{other}); err != nil {
		t.Fatalf("marking a third row done: %v", err)
	}
	done := doneNow(rep, t, e, deal)
	if _, read := done[other]; read {
		t.Error("the Done set read a row it was not asked about")
	}
	for _, row := range []WorklistRowRef{deal, group} {
		if at, ok := done[row]; !ok || at.IsZero() {
			t.Errorf("%+v is not read back as done with its moment", row)
		}
	}
	if theirs := doneNow(e.asSeat(e.other), t, e, deal); len(theirs) != 0 {
		t.Errorf("a colleague's Done set carries %d of the rep's rows", len(theirs))
	}
	if n := doneTrail(t, e, deal); n != 1 {
		t.Errorf("Done on the deal wrote %d audit rows, want 1", n)
	}

	for range 2 {
		if err := s.ReopenWorklistRows(rep, []WorklistRowRef{deal}); err != nil {
			t.Fatalf("undoing: %v", err)
		}
	}
	done = doneNow(rep, t, e, deal)
	if _, ok := done[deal]; ok {
		t.Error("the deal is still done after Undo")
	}
	if _, ok := done[group]; !ok {
		t.Error("undoing the deal also took back the group's Done")
	}
	if n := doneTrail(t, e, deal); n != 2 {
		t.Errorf("Done then two Undos wrote %d audit rows, want 2: the second Undo changed nothing", n)
	}
}

// A source the queue does not produce, and a caller with no human behind it,
// are refused before anything is written.
func TestDoneRefusesAnUnknownLaneAndAnAgent(t *testing.T) {
	e := setupLoad(t)
	s := handledStore(e)
	junk := []WorklistRowRef{{Source: "made_up", RowID: "x"}}
	if err := s.MarkWorklistRowsDone(e.followUpReader(), junk); !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Errorf("Done on an unknown lane answered %v, want an invalid argument", err)
	}
	agent := principal.WithActor(principal.WithWorkspaceID(context.Background(), e.ws), principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:x", UserID: e.rep,
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"activity": {Read: true}},
			RowScope: principal.RowScopeAll,
		},
	})
	row := []WorklistRowRef{{Source: "deal_at_risk", RowID: ids.NewV7().String()}}
	if err := s.MarkWorklistRowsDone(agent, row); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("Done from an agent answered %v, want a refusal", err)
	}
}

// Undo of a Done takes back the Done alone. A snooze the reader set after it
// stays, so a late Undo cannot bring back what they put off.
func TestUndoOfDoneLeavesALaterSnooze(t *testing.T) {
	e := setupLoad(t)
	s := storeAddressing(e, readerAddress)
	sent, _ := e.sentTo(t, e.customerAt(t, "customer"), "", 3*24*time.Hour)
	row := []WorklistRowRef{{Source: "awaiting_reply", RowID: sent.String()}}
	if err := s.MarkWorklistRowsDone(e.followUpReader(), row); err != nil {
		t.Fatalf("marking done: %v", err)
	}
	e.exec(t, `UPDATE activity_reader_state SET state = 'snoozed', reopen_on = 'time',
		snoozed_until = now() + interval '1 day' WHERE activity_id = $1`, sent)

	if err := s.ReopenWorklistRows(e.followUpReader(), row); err != nil {
		t.Fatalf("undoing: %v", err)
	}
	var states int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM activity_reader_state WHERE activity_id = $1 AND state = 'snoozed'`,
		sent).Scan(&states); err != nil {
		t.Fatal(err)
	}
	if states != 1 {
		t.Error("the Undo of a Done also took back the snooze set after it")
	}
}

// A row named twice, or a message already done, writes nothing more: one
// audit row and one event in all.
func TestDoneTwiceOnOneMessageWritesItOnce(t *testing.T) {
	e := setupLoad(t)
	_, message := handledFixture(t, e, "Answered on the phone")
	row := WorklistRowRef{Source: "customer_waiting", RowID: message.String()}
	for range 2 {
		if err := handledStore(e).MarkWorklistRowsDone(e.asSeat(e.rep), []WorklistRowRef{row, row}); err != nil {
			t.Fatalf("marking done: %v", err)
		}
	}
	if audited, announced := handledTrail(t, e, message); audited != 1 || announced != 1 {
		t.Errorf("two presses naming the message twice wrote %d audit rows and %d events, want 1 and 1",
			audited, announced)
	}
}
