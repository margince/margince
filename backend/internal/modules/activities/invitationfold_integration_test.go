// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// invitedHistory is one customer's History: a meeting and the mail about its
// event from both sides, plus mail that is not about a captured meeting.
type invitedHistory struct {
	contact, meeting ids.UUID
	// starts is the meeting's occurrence, as its identity key spells it.
	starts                              string
	sent, received, update, answer      ids.UUID
	counter, written, otherEvent, plain ids.UUID
}

func (e *loadEnv) invitedHistory(t *testing.T) invitedHistory {
	t.Helper()
	now := clocktest.Now(t)
	h := invitedHistory{contact: e.buyer(t), meeting: ids.NewV7(), starts: now.Add(24 * time.Hour).UTC().Format(time.RFC3339)}
	uid := "kickoff-" + ids.NewV7().String() + "@google.com"
	e.exec(t, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'meeting', 'Kickoff', $2, 'seed', 'system')`, h.meeting, now.Add(24*time.Hour))
	e.exec(t, `INSERT INTO activity_identity (identity_kind, identity_key, activity_id, attested_by)
		VALUES ('meeting', $1, $2, 'seed')`, MeetingIdentityKey(uid, h.starts), h.meeting)
	e.exec(t, `INSERT INTO activity_participant (activity_id, role, user_id, response) VALUES ($1, 'organizer', $2, 'accepted')`, h.meeting, e.rep)
	e.exec(t, `INSERT INTO activity_participant (activity_id, role, contact_id, response) VALUES ($1, 'attendee', $2, 'declined')`, h.meeting, h.contact)
	e.exec(t, `INSERT INTO activity_participant (activity_id, role, user_id) VALUES ($1, 'attendee', $2)`, h.meeting, e.other)
	e.linkContact(t, h.meeting, h.contact)
	mail := func(direction, subject string, method, eventUID any, ago time.Duration) ids.UUID {
		id := ids.NewV7()
		e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, source, captured_by, calendar_method, calendar_uid)
			VALUES ($1, 'email', $2, $3, $4, 'seed', 'system', $5, $6)`, id, direction, subject, now.Add(-ago), method, eventUID)
		e.linkContact(t, id, h.contact)
		return id
	}
	h.sent = mail("outbound", "Invitation: Kickoff", "REQUEST", uid, 5*time.Hour)
	h.received = mail("inbound", "Invitation: Kickoff", "REQUEST", uid, 5*time.Hour)
	h.update = mail("outbound", "Updated invitation: Kickoff", "REQUEST", uid, 4*time.Hour)
	h.answer = mail("inbound", "Declined: Kickoff", "REPLY", uid, 3*time.Hour)
	h.counter = mail("inbound", "New time proposed: Kickoff", "COUNTER", uid, 2*time.Hour)
	h.written = mail("inbound", "Re: Invitation: Kickoff", nil, nil, time.Hour)
	h.otherEvent = mail("inbound", "Invitation: Webinar", "REQUEST", "webinar-"+ids.NewV7().String(), 30*time.Minute)
	h.plain = mail("outbound", "the proposal", nil, nil, 10*time.Minute)
	return h
}

func (e *loadEnv) linkContact(t *testing.T, activity, contact ids.UUID) {
	t.Helper()
	e.exec(t, `INSERT INTO activity_link (id, activity_id, entity_type, contact_id)
		VALUES ($1, $2, 'contact', $3)`, ids.NewV7(), activity, contact)
}

func (e *loadEnv) history(t *testing.T, in ListActivitiesInput) map[ids.UUID]crmcontracts.Activity {
	t.Helper()
	rows, _, err := handledStore(e).ListActivities(e.followUpReader(), in)
	if err != nil {
		t.Fatalf("listing the history: %v", err)
	}
	out := map[ids.UUID]crmcontracts.Activity{}
	for _, row := range rows {
		out[ids.UUID(row.Id)] = row
	}
	return out
}

func TestEveryInvitationMailOfAMeetingFoldsOntoItInHistory(t *testing.T) {
	e := setupLoad(t)
	h := e.invitedHistory(t)
	contactType := "contact"
	rows := e.history(t, ListActivitiesInput{EntityType: &contactType, EntityID: &h.contact, CollapseSeatCopies: true})

	for name, folded := range map[string]ids.UUID{"organizer's copy": h.sent, "guest's copy": h.received, "update": h.update, "answer": h.answer} {
		if _, shown := rows[folded]; shown {
			t.Errorf("the %s of the meeting's invitation is still its own History row", name)
		}
	}
	for name, kept := range map[string]ids.UUID{
		"counter-proposal": h.counter, "written reply": h.written, "invitation to an event with no meeting": h.otherEvent, "plain mail": h.plain,
	} {
		if _, shown := rows[kept]; !shown {
			t.Errorf("the %s is gone from History", name)
		}
	}
	meeting, ok := rows[h.meeting]
	want := crmcontracts.MeetingInvitationSummary{Invited: 3, Accepted: 1, Declined: 1}
	if !ok || meeting.Invitation == nil || *meeting.Invitation != want {
		t.Errorf("the meeting reads invitation %+v (shown %v), want %+v", meeting.Invitation, ok, want)
	}
	if rows[h.written].Invitation != nil {
		t.Error("a mail row carries a meeting's invitation summary")
	}
}

func TestInvitationMailStaysWhereItsMeetingIsNotListed(t *testing.T) {
	e := setupLoad(t)
	h := e.invitedHistory(t)
	contactType := "contact"
	mailOnly := e.history(t, ListActivitiesInput{EntityType: &contactType, EntityID: &h.contact, CollapseSeatCopies: true, Kinds: []string{"email"}})
	if _, shown := mailOnly[h.received]; !shown {
		t.Error("a mail-only History hides an invitation whose meeting it does not show")
	}
	every := e.history(t, ListActivitiesInput{EntityType: &contactType, EntityID: &h.contact})
	if _, shown := every[h.sent]; !shown || every[h.meeting].Invitation != nil {
		t.Error("a read without collapse_copies folds the invitation; an export must read every row")
	}
	e.exec(t, `UPDATE activity SET archived_at = now() WHERE id = $1`, h.meeting)
	archived := e.history(t, ListActivitiesInput{EntityType: &contactType, EntityID: &h.contact, CollapseSeatCopies: true})
	if _, shown := archived[h.sent]; !shown {
		t.Error("the invitation of an archived meeting is hidden with it")
	}
}

func TestInvitationMailStaysWhenTheReaderCannotOpenItsMeeting(t *testing.T) {
	e := setupLoad(t)
	h := e.invitedHistory(t)
	e.exec(t, `DELETE FROM activity_participant WHERE activity_id = $1 AND user_id = $2`, h.meeting, e.rep)
	e.exec(t, `UPDATE activity SET audience = 'participants' WHERE id = $1`, h.meeting)
	contactType := "contact"
	rows := e.history(t, ListActivitiesInput{EntityType: &contactType, EntityID: &h.contact, CollapseSeatCopies: true})
	if _, shown := rows[h.sent]; !shown {
		t.Error("an invitation the reader may open is hidden behind a meeting they may not")
	}
	if meeting, shown := rows[h.meeting]; shown && meeting.Invitation != nil {
		t.Error("a withheld meeting says who it invited")
	}
}

func TestMailForOneOccurrenceFoldsOnlyOntoThatOccurrence(t *testing.T) {
	e := setupLoad(t)
	h := e.invitedHistory(t)
	var uid string
	if err := e.owner.QueryRow(context.Background(), `SELECT calendar_uid FROM activity WHERE id = $1`, h.sent).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	listed := h.starts
	other := clocktest.Now(t).Add(8 * 24 * time.Hour).UTC().Format(time.RFC3339)
	e.exec(t, `UPDATE activity SET calendar_event = jsonb_build_object('occurrence', $2::text) WHERE id = $1`, h.update, other)
	e.exec(t, `UPDATE activity SET calendar_event = jsonb_build_object('occurrence', $2::text) WHERE id = $1`, h.answer, listed)
	contactType := "contact"
	rows := e.history(t, ListActivitiesInput{EntityType: &contactType, EntityID: &h.contact, CollapseSeatCopies: true})
	if _, shown := rows[h.update]; !shown {
		t.Errorf("mail about the occurrence on %s folds onto the one on %s (series %s)", other, listed, uid)
	}
	if _, shown := rows[h.answer]; shown {
		t.Error("mail about the listed occurrence is still its own row")
	}
}
