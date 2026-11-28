// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Who a transcript reading acts as. The reading, and the promise it files for
// the colleague who asked, runs as that colleague's agent under their live grants.
// A promise it files for another colleague is the system's, on the asking
// colleague's behalf, so its record says whose meeting it came from. The run row
// is the system's either way.

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Rep2 attends the meeting and speaks the promise, so Rep2 asks for the reading
// here. A confident promise its own speaker made is a task at once, never a
// proposal. That is the path that files as the requester's agent.
func TestTheRequestersOwnPromiseIsFiledByTheirAgent(t *testing.T) {
	e := setupTranscript(t)
	e.WsExec(t, `UPDATE app_user SET display_name = 'Priya Raman' WHERE id = $1`, e.Rep2)
	e.GrantRole(t, e.Rep2, "rep")
	started, _, err := e.Activities.StartTranscriptReadQueued(e.ctx, e.activity, "human:"+e.Rep2.String(), nil)
	if err != nil {
		t.Fatalf("starting the reading: %v", err)
	}
	reader := &transcriptProposeWorker{users: identity.NewService(e.Pool)}
	worker, live, err := reader.readerFor(principal.WithWorkspaceID(context.Background(), e.WS), "human:"+e.Rep2.String(), started.ID)
	if err != nil || !live {
		t.Fatalf("binding the reader for Rep2: live=%t err=%v", live, err)
	}
	proposer := NewTranscriptProposer(e.Pool, cannedBrain{reply: attributedReply(t, "Priya Raman", "seat:"+e.Rep2.String(), 0.9)},
		e.svc, func() time.Time { return clocktest.Now(t) }, slog.New(slog.DiscardHandler))
	if err := proposer.Read(worker, e.Activities, started.ID, e.activity); err != nil {
		t.Fatalf("reading the transcript: %v", err)
	}
	done, err := e.Activities.GetTranscriptRead(e.ctx, e.activity, started.ID)
	if err != nil {
		t.Fatal(err)
	}
	if done.Status != activities.TranscriptReadDone || len(done.ProposalIDs) != 0 {
		t.Fatalf("reading = %s with %d proposals, want done and the promise filed as a task at once", done.Status, len(done.ProposalIDs))
	}
	task := e.wsString(t, `SELECT coalesce(assignee_id::text, '') || ' ' || captured_by
		FROM activity WHERE kind = 'task' ORDER BY created_at DESC LIMIT 1`)
	if want := e.Rep2.String() + " " + transcriptProposalActor; task != want {
		t.Errorf("the task reads %q, want Rep2's own promise filed by the reader agent (%q)", task, want)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'activity' AND action = 'create'
		AND actor_type = 'agent' AND actor_id = $1 AND on_behalf_of = $2`, transcriptProposalActor, e.Rep2); n == 0 {
		t.Error("no activity was written by the reader agent for Rep2")
	}
}

func TestAnotherColleaguesPromiseIsFiledByTheSystemForTheRequestersMeeting(t *testing.T) {
	e := setupTranscript(t)
	e.WsExec(t, `UPDATE app_user SET display_name = 'Priya Raman' WHERE id = $1`, e.Rep2)
	e.read(t, cannedBrain{reply: attributedReply(t, "Priya Raman", "seat:"+e.Rep2.String(), 0.9)})

	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'activity' AND action = 'create'
		AND actor_type = 'system' AND actor_id = $1 AND on_behalf_of = $2`, transcriptReadSystemID, e.Rep1); n == 0 {
		t.Error("Rep2's task was not filed by the system on behalf of Rep1, whose meeting it came from")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE actor_type = 'system' AND actor_id = $1`,
		transcriptProposalActor); n != 0 {
		t.Errorf("%d rows name a system principal by the reader agent's id, want 0", n)
	}
}

func TestAReadingForARequesterWhoMayNoLongerReadTheMeetingFailsWithAReason(t *testing.T) {
	e := setupTranscript(t)
	started, _, err := e.Activities.StartTranscriptReadQueued(e.ctx, e.activity, "human:"+e.Rep1.String(), nil)
	if err != nil {
		t.Fatalf("starting the reading: %v", err)
	}
	proposer := NewTranscriptProposer(e.Pool, cannedBrain{reply: attributedReply(t, "Priya", "seat:"+e.Rep1.String(), 0.9)},
		e.svc, func() time.Time { return clocktest.Now(t) }, slog.New(slog.DiscardHandler))
	worker := &transcriptProposeWorker{
		language: installationLanguage(e.Pool), activities: e.Activities, proposer: proposer,
		users: identity.NewService(e.Pool), log: slog.New(slog.DiscardHandler),
	}
	job := &river.Job[TranscriptProposeArgs]{JobRow: &rivertype.JobRow{}, Args: TranscriptProposeArgs{
		ActivityID: e.activity.UUID, TranscriptReadID: started.ID, RequestedBy: "human:" + e.Rep1.String(),
	}}
	if err := worker.Work(context.Background(), job); err != nil {
		t.Fatalf("working the reading: %v", err)
	}
	done, err := e.Activities.GetTranscriptRead(e.ctx, e.activity, started.ID)
	if err != nil {
		t.Fatal(err)
	}
	if done.Status != activities.TranscriptReadFailed || done.Outcome == nil || done.Outcome.Failure == nil ||
		done.Outcome.Failure.Code != activities.TranscriptFailureDenied {
		t.Fatalf("reading = %+v, want failed as denied, never left running", done)
	}
	if n := e.taskCount(t); n != 0 {
		t.Errorf("%d tasks filed for a requester who may not read the meeting, want 0", n)
	}
}
