// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// attentionRecorder stands in for the host's notice, a true boundary here: it
// records the episodes the host would be told about.
type attentionRecorder struct{ episodes []ids.UUID }

func (a *attentionRecorder) InvitationAttentionTx(_ context.Context, _ pgx.Tx, _ ids.UserID, _, episode ids.UUID, _ string) error {
	a.episodes = append(a.episodes, episode)
	return nil
}

type parkedRow struct {
	status   string
	reason   *string
	episode  *ids.UUID
	attempts int
}

func (f *invitationFixture) parked(t *testing.T, id crmcontracts.Id) parkedRow {
	t.Helper()
	var row parkedRow
	if err := f.env.owner.QueryRow(f.ctx, `SELECT status, attention_reason, attention_episode, attempts FROM meeting_invitation WHERE activity_id=$1`, id).Scan(&row.status, &row.reason, &row.episode, &row.attempts); err != nil {
		t.Fatal(err)
	}
	return row
}

func (f *invitationFixture) queue(t *testing.T, hours ...int) []crmcontracts.Id {
	t.Helper()
	var out []crmcontracts.Id
	for _, hour := range hours {
		f.request.Start, f.request.End = monday(hour), monday(hour+1)
		meeting, err := f.store.CreateInvitation(f.ctx, f.request)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, meeting.Id)
	}
	return out
}

// A refused calendar parks the host's whole queue under one episode. A row a
// worker holds parks itself when its own attempt ends, and joins that episode.
// Once the calendar can take bookings again, the next pass sends them all on.
func TestARefusedCalendarParksTheQueueOnceAndResumesIt(t *testing.T) {
	f := newInvitationFixture(t)
	told := &attentionRecorder{}
	f.store = f.store.WithInvitationAttention(told)
	queued := f.queue(t, 10, 12, 14)
	if _, err := f.env.owner.Exec(f.ctx, `UPDATE meeting_invitation SET lease_until=$2 WHERE activity_id=$1`, queued[2], f.now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	f.calendar.checkErr = connector.ErrAuthRejected
	if err := f.store.DeliverInvitations(f.ctx); err != nil {
		t.Fatal(err)
	}
	first, second := f.parked(t, queued[0]), f.parked(t, queued[1])
	if first.status != invitationNeedsAttention || second.status != invitationNeedsAttention || first.episode == nil || second.episode == nil || *first.episode != *second.episode {
		t.Fatalf("the unleased queue did not park under one episode: %+v %+v", first, second)
	}
	if tried := first.attempts + second.attempts; tried != 1 {
		t.Errorf("the queue was tried %d time(s), want once: the first refusal parks the rest untried", tried)
	}
	if held := f.parked(t, queued[2]); held.status != invitationPending {
		t.Fatalf("a row a worker holds was parked under its lease: %+v", held)
	}

	if _, err := f.env.owner.Exec(f.ctx, `UPDATE meeting_invitation SET lease_until=NULL WHERE activity_id=$1`, queued[2]); err != nil {
		t.Fatal(err)
	}
	if err := f.store.DeliverInvitations(f.ctx); err != nil {
		t.Fatal(err)
	}
	held := f.parked(t, queued[2])
	if held.reason == nil || *held.reason != attentionCalendarDisconnected || held.episode == nil || *held.episode != *first.episode {
		t.Fatalf("the held row did not join the episode when its attempt ended: %+v", held)
	}
	for _, episode := range told.episodes {
		if episode != *first.episode {
			t.Fatalf("the host was told about episodes %v, want only %s", told.episodes, *first.episode)
		}
	}

	f.reconnect()
	if err := f.store.DeliverInvitations(f.ctx); err != nil {
		t.Fatal(err)
	}
	for _, id := range queued {
		if row := f.parked(t, id); row.status != invitationConfirmed || row.reason != nil || row.episode != nil {
			t.Errorf("a reconnected calendar did not send %s on: %+v", id, row)
		}
	}
}

// An ordinary failure waits on its own after the fifth attempt, says why, and
// a retry clears the reason.
func TestTheFifthOrdinaryFailureSaysWhyAndARetryClearsIt(t *testing.T) {
	f := newInvitationFixture(t)
	told := &attentionRecorder{}
	f.store = f.store.WithInvitationAttention(told)
	queued := f.queue(t, 10)
	f.calendar.fail = true
	for range 5 {
		if err := f.store.DeliverInvitations(f.ctx); err != nil {
			t.Fatal(err)
		}
		f.now = f.now.Add(10 * time.Minute)
	}
	row := f.parked(t, queued[0])
	if row.status != invitationNeedsAttention || row.reason == nil || *row.reason != attentionDeliveryFailed {
		t.Fatalf("the fifth failure reads %+v, want delivery_failed", row)
	}
	if len(told.episodes) != 1 || told.episodes[0] != ids.UUID(queued[0]) {
		t.Errorf("the host was told %v, want once for this invitation", told.episodes)
	}
	meeting, err := f.store.Invitation(f.ctx, ids.UUID(queued[0]))
	if err != nil {
		t.Fatal(err)
	}
	if meeting.AttentionReason == nil || *meeting.AttentionReason != crmcontracts.MeetingInvitationAttentionReasonDeliveryFailed {
		t.Errorf("the invitation view reads reason %v", meeting.AttentionReason)
	}
	if _, err := f.store.ChangeInvitation(f.ctx, ids.UUID(queued[0]), crmcontracts.MeetingInvitationChange{Action: "retry", Version: meeting.Version}); err != nil {
		t.Fatal(err)
	}
	if row := f.parked(t, queued[0]); row.status != invitationPending || row.reason != nil || row.episode != nil {
		t.Errorf("a retry left %+v, want pending with no reason", row)
	}
}

// reconnect is the host granting the calendar again a minute later.
func (f *invitationFixture) reconnect() {
	f.now = f.now.Add(time.Minute)
	f.calendar.checkErr, f.calendar.reconnectedAt = nil, f.now
}

func (f *invitationFixture) deliver(t *testing.T) {
	t.Helper()
	if err := f.store.DeliverInvitations(f.ctx); err != nil {
		t.Fatal(err)
	}
}

// A rejection that was in flight when the queue resumed is about the calendar
// the host has since reconnected. It parks nothing and tells nobody; the row is
// retried. A refusal after the resume is a new disconnect, told once more.
func TestAStaleRefusalParksNothingAndANewOneIsANewEpisode(t *testing.T) {
	f := newInvitationFixture(t)
	told := &attentionRecorder{}
	f.store = f.store.WithInvitationAttention(told)
	f.queue(t, 10, 12)
	held, found, err := f.store.claimInvitation(f.ctx)
	if err != nil || !found {
		t.Fatalf("claiming the held row: %v %t", err, found)
	}
	f.calendar.checkErr = connector.ErrAuthRejected
	f.deliver(t)
	f.reconnect()
	f.deliver(t)
	if err := f.store.finishInvitation(f.ctx, held, nil, connector.ErrAuthRejected); err != nil {
		t.Fatal(err)
	}
	if row := f.parked(t, crmcontracts.Id(held.ID)); row.status != invitationPending || row.reason != nil {
		t.Errorf("a refusal from before the reconnect parked its row: %+v", row)
	}
	if len(told.episodes) != 1 {
		t.Fatalf("the host was told %d times about one disconnect", len(told.episodes))
	}

	f.now = f.now.Add(time.Hour)
	f.calendar.checkErr = connector.ErrAuthRejected
	f.deliver(t)
	if len(told.episodes) != 2 || told.episodes[1] == told.episodes[0] {
		t.Errorf("a disconnect after the resume told the host %v, want one more, new episode", told.episodes)
	}
}

// A refusal while a meeting moves or is canceled parks the change, and the
// reconnect sends the same change on.
func TestARefusedMoveAndCancelParkAndResumeAsThemselves(t *testing.T) {
	for _, action := range []string{"reschedule", invitationCancel} {
		t.Run(action, func(t *testing.T) {
			f := newInvitationFixture(t)
			meeting := f.book(t)
			change := crmcontracts.MeetingInvitationChange{Action: crmcontracts.MeetingInvitationChangeAction(action), Version: meeting.Version}
			if action == "reschedule" {
				start, end := monday(14), monday(15)
				change.Start, change.End = &start, &end
			}
			if _, err := f.store.ChangeInvitation(f.ctx, ids.UUID(meeting.Id), change); err != nil {
				t.Fatal(err)
			}
			f.calendar.checkErr = connector.ErrAuthRejected
			f.deliver(t)
			if row := f.parked(t, meeting.Id); row.reason == nil || *row.reason != attentionCalendarDisconnected {
				t.Fatalf("a refused %s reads %+v", action, row)
			}
			f.reconnect()
			f.deliver(t)
			want := map[string]string{"reschedule": invitationConfirmed, invitationCancel: invitationCanceled}[action]
			if row := f.parked(t, meeting.Id); row.status != want || row.reason != nil {
				t.Errorf("the reconnected %s reads %+v, want %s", action, row, want)
			}
		})
	}
}

// A reconnect resumes the disconnect's rows only: an invitation that failed on
// its own keeps waiting for its retry.
func TestAReconnectLeavesADeliveryFailureWaiting(t *testing.T) {
	f := newInvitationFixture(t)
	failed := f.queue(t, 10)[0]
	f.calendar.fail = true
	for range 5 {
		f.deliver(t)
		f.now = f.now.Add(10 * time.Minute)
	}
	f.calendar.fail = false
	parked := f.queue(t, 14)[0]
	f.calendar.checkErr = connector.ErrAuthRejected
	f.deliver(t)
	f.reconnect()
	f.deliver(t)
	if row := f.parked(t, failed); row.status != invitationNeedsAttention || row.reason == nil || *row.reason != attentionDeliveryFailed {
		t.Errorf("the delivery failure reads %+v after a reconnect, want it still waiting", row)
	}
	if row := f.parked(t, parked); row.status != invitationConfirmed {
		t.Errorf("the disconnected row reads %+v after a reconnect", row)
	}
}

// The calendar is judged under the host's lock, so a disconnect cannot park
// rows between the judgement and the resume it allows.
func TestAResumeJudgesTheCalendarUnderTheHostsLock(t *testing.T) {
	f := newInvitationFixture(t)
	f.queue(t, 10)
	f.calendar.checkErr = connector.ErrAuthRejected
	f.deliver(t)
	f.reconnect()
	held := true
	f.calendar.judging = func() {
		ctx, cancel := context.WithTimeout(f.ctx, 300*time.Millisecond)
		defer cancel()
		held = f.store.tx(ctx, func(tx pgx.Tx) error {
			return storekit.LockWriteIdentity(ctx, tx, "meeting_host", ids.From[ids.UserKind](f.env.rep).String())
		}) != nil
	}
	f.deliver(t)
	if !held {
		t.Error("the calendar was judged while another writer could take the host's lock")
	}
}

// A retry answers the row as stored, without the reason it no longer has.
func TestARetryAnswersWithoutTheReason(t *testing.T) {
	f := newInvitationFixture(t)
	id := f.queue(t, 10)[0]
	f.calendar.fail = true
	for range 5 {
		f.deliver(t)
		f.now = f.now.Add(10 * time.Minute)
	}
	meeting, err := f.store.Invitation(f.ctx, ids.UUID(id))
	if err != nil {
		t.Fatal(err)
	}
	retried, err := f.store.ChangeInvitation(f.ctx, ids.UUID(id), crmcontracts.MeetingInvitationChange{Action: "retry", Version: meeting.Version})
	if err != nil {
		t.Fatal(err)
	}
	if retried.AttentionReason != nil || retried.Status != invitationPending {
		t.Errorf("the retry answered %s with reason %v", retried.Status, retried.AttentionReason)
	}
}
