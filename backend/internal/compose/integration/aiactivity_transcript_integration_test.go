// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The transcript reading as the rail sees it, end to end: the store moves the
// row, the move announces itself, and the projection holds what a rep's feed
// would draw.
//
// The router used to report this task, which meant the occurrence was settled
// the moment it appeared: a rep pressed the button, saw nothing while the model
// worked, and then found it already done. These are the states that were
// unreachable before — and the re-arm, which is the one that needs the attempt.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/aiactivity"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// transcriptFixture is one real transcript on one real activity, plus the store
// that moves its reading and the consumer that projects what the moves say.
type transcriptFixture struct {
	env      *Env
	ctx      context.Context
	store    *activities.Store
	consumer *aiactivity.Consumer
	activity ids.ActivityID
	readID   ids.UUID
	// delivered is how far this subscriber has got, so drain hands over only
	// what is new. Replaying the whole history would let the original queued
	// event paper over anything a later one got wrong.
	delivered int
}

func newTranscriptFixture(t *testing.T) *transcriptFixture {
	t.Helper()
	e := Setup(t)
	ctx := e.Admin()
	// Through LogActivityInputFrom, exactly as the HTTP handler and the MCP
	// provider both do: a hand-built input would skip the mapping that decides
	// a body is a transcript, and the reading has nothing to read without it.
	raw := "Anna: we will send the quote on Friday.\nBen: thanks."
	sourceSystem := "transcript"
	in, err := activities.LogActivityInputFrom(crmcontracts.CreateActivityRequest{
		Kind: "meeting", Subject: ptr("Acme kickoff"), Body: &raw,
		SourceSystem: &sourceSystem, Source: "ui",
	})
	if err != nil {
		t.Fatalf("LogActivityInputFrom: %v", err)
	}
	store := activities.NewStore(e.DB())
	logged, _, err := store.LogActivity(ctx, in)
	if err != nil {
		t.Fatalf("LogActivity: %v", err)
	}
	activityID := ids.From[ids.ActivityKind](ids.UUID(logged.Id))

	read, _, err := store.StartTranscriptReadQueued(ctx, activityID, "human:"+e.AdminUser.String(), nil)
	if err != nil {
		t.Fatalf("StartTranscriptReadQueued: %v", err)
	}
	return &transcriptFixture{
		env: e, ctx: ctx, store: store,
		consumer: aiactivity.NewConsumer(aiactivity.NewStore(e.DB()), testLogger(t)),
		activity: activityID, readID: read.ID,
	}
}

// drain hands the consumer every ai_task.state_changed this reading staged,
// oldest first — what a subscriber that is keeping up receives.
func (f *transcriptFixture) drain(t *testing.T) {
	t.Helper()
	var raws [][]byte
	err := f.env.DB().Tx(context.Background(), func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `
			SELECT envelope FROM event_outbox
			 WHERE envelope->>'type' = 'ai_task.state_changed'
			   AND envelope->'payload'->>'occurrence_key' = $1
			 ORDER BY seq
			 OFFSET $2`, f.readID.String(), f.delivered)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var raw []byte
			if err := rows.Scan(&raw); err != nil {
				return err
			}
			raws = append(raws, raw)
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatalf("reading the staged envelopes: %v", err)
	}
	if len(raws) == 0 {
		t.Fatal("the reading staged no ai_task.state_changed at all — nothing downstream could ever learn it exists")
	}
	for _, raw := range raws {
		var env kevents.Envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			t.Fatalf("decoding a staged envelope: %v", err)
		}
		if err := f.consumer.HandleEvent(context.Background(), env); err != nil {
			t.Fatalf("the projection refused envelope %s: %v", env.EventID, err)
		}
		f.delivered++
	}
}

func (f *transcriptFixture) projection(t *testing.T) projectedOccurrence {
	t.Helper()
	var got projectedOccurrence
	err := f.env.Pool.QueryRow(context.Background(), `
		SELECT kind, ai_task, state, attempt, actor_scope, actor_user_id,
		       started_at, stale_after, subject_type
		  FROM ai_task_run WHERE source = $1 AND occurrence_key = $2`,
		activities.TranscriptActivitySource, f.readID.String()).
		Scan(&got.Kind, &got.AITask, &got.State, &got.Attempt, &got.ActorScope,
			&got.ActorUserID, &got.StartedAt, &got.StaleAfter, &got.SubjectType)
	if err != nil {
		t.Fatalf("reading the projected occurrence: %v", err)
	}
	return got
}

// The line the router could never draw: a reading is live and the rep's own
// from the moment they ask for it, not from the moment it finishes.
func TestAQueuedTranscriptReadingIsProjectedAsTheContactsOwnLiveWork(t *testing.T) {
	f := newTranscriptFixture(t)
	f.drain(t)

	got := f.projection(t)
	if got.State != "queued" || got.Attempt != 1 {
		t.Fatalf("state/attempt = %s/%d, want queued/1", got.State, got.Attempt)
	}
	if got.ActorScope != "personal" || got.ActorUserID == nil || *got.ActorUserID != f.env.AdminUser {
		t.Fatalf("actor = %s/%v, want personal/%s — the human who asked owns the occurrence",
			got.ActorScope, got.ActorUserID, f.env.AdminUser)
	}
	if got.Kind != activities.TranscriptAITask || got.AITask == nil || *got.AITask != activities.TranscriptAITask {
		t.Fatalf("kind/ai_task = %s/%v, want %s", got.Kind, got.AITask, activities.TranscriptAITask)
	}
	if got.StaleAfter == nil {
		t.Fatal("a queued occurrence carries no stale_after, so a queue nobody drains would render as live forever")
	}
	if got.SubjectType == nil || *got.SubjectType != "activity" {
		t.Fatalf("subject_type = %v, want activity", got.SubjectType)
	}
}

// The claim moves it to running under a start time, which is what the rail ages
// the live line from.
func TestAClaimedTranscriptReadingIsProjectedAsRunning(t *testing.T) {
	f := newTranscriptFixture(t)
	if _, err := f.store.BeginTranscriptRead(f.ctx, f.readID, activities.TranscriptReadLease); err != nil {
		t.Fatalf("BeginTranscriptRead: %v", err)
	}
	f.drain(t)

	got := f.projection(t)
	if got.State != "running" || got.Attempt != 1 {
		t.Fatalf("state/attempt = %s/%d, want running/1", got.State, got.Attempt)
	}
	if got.StartedAt == nil {
		t.Fatal("a running occurrence carries no start time, so the rail cannot age it")
	}
}

// A settled reading stops carrying a lease: nothing about a closed occurrence
// can go stale.
func TestAFinishedTranscriptReadingSettlesInTheProjection(t *testing.T) {
	f := newTranscriptFixture(t)
	if _, err := f.store.BeginTranscriptRead(f.ctx, f.readID, activities.TranscriptReadLease); err != nil {
		t.Fatalf("BeginTranscriptRead: %v", err)
	}
	if err := f.store.FinishTranscriptRead(f.ctx, f.readID, activities.TranscriptReadOutcome{
		Status: activities.TranscriptReadDone, Detail: "No next steps were stated.",
	}); err != nil {
		t.Fatalf("FinishTranscriptRead: %v", err)
	}
	f.drain(t)

	got := f.projection(t)
	if got.State != "done" {
		t.Fatalf("state = %s, want done", got.State)
	}
	if got.StaleAfter != nil {
		t.Fatalf("a settled occurrence still carries stale_after %v — a closed reading cannot go stale", got.StaleAfter)
	}
}

// The re-arm is what the attempt column exists for. A reading whose worker died
// is handed back, and the queued state it announces has to BEAT the running one
// it replaces — otherwise the rail shows a dead reading as live until its lease
// runs out, which is the failure the projection exists to prevent.
func TestARearmedTranscriptReadingOutranksTheDeadClaimItReplaces(t *testing.T) {
	f := newTranscriptFixture(t)
	if _, err := f.store.BeginTranscriptRead(f.ctx, f.readID, activities.TranscriptReadLease); err != nil {
		t.Fatalf("BeginTranscriptRead: %v", err)
	}
	f.drain(t)
	if got := f.projection(t); got.State != "running" || got.Attempt != 1 {
		t.Fatalf("after the claim, state/attempt = %s/%d, want running/1", got.State, got.Attempt)
	}

	// Age the claim past its lease, which is what makes the reading abandoned.
	// The clock is moved rather than waited on: the lease is minutes long and a
	// test that slept for it would be the flake this suite forbids.
	if _, err := f.env.Pool.Exec(context.Background(), `
		UPDATE transcript_read
		   SET started_at = now() - ($2 * interval '1 microsecond')
		 WHERE id = $1`, f.readID, (activities.TranscriptReadLease * 2).Microseconds()); err != nil {
		t.Fatalf("ageing the claim past its lease: %v", err)
	}
	// Pressing the button again is the recovery path, and the one a rep would
	// try unprompted.
	if _, _, err := f.store.StartTranscriptReadQueued(f.ctx, f.activity, "human:"+f.env.AdminUser.String(), nil); err != nil {
		t.Fatalf("re-arming through a second press: %v", err)
	}
	f.drain(t)

	got := f.projection(t)
	if got.State != "queued" || got.Attempt != 2 {
		t.Fatalf("after the re-arm, state/attempt = %s/%d, want queued/2 — a row frozen at running is a reading the rail says is working and nobody holds",
			got.State, got.Attempt)
	}
	if got.StaleAfter == nil || !got.StaleAfter.After(time.Now()) {
		t.Fatalf("the re-armed occurrence's stale_after is %v — it must age from THIS attempt, not the dead one's start", got.StaleAfter)
	}
}
