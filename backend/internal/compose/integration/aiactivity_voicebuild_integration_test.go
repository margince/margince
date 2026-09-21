// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A voice build as the rail sees it, end to end: the store moves the row, the
// move announces itself, and the projection holds what a rep's feed would draw.
//
// The router used to report this task, so the occurrence was settled the moment
// it appeared — a rep asked the product to learn their writing voice, saw
// nothing while the model worked, and then found it already done. These are the
// states that were unreachable before, and the deferral, which is the one that
// needs the attempt.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/aiactivity"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// voiceBuildReclaim stands in for the worker's reclaim window, which compose
// computes from the job's own timeout. Any positive value proves the shape.
const voiceBuildReclaim = 10 * time.Minute

// voiceBuildFixture is one owner's profile with a real corpus, the build they
// asked for, and the consumer that projects what the build announces.
type voiceBuildFixture struct {
	env      *Env
	owner    context.Context
	store    *ai.VoiceStore
	consumer *aiactivity.Consumer
	profile  ids.UUID
	buildID  ids.UUID
	// delivered is how far this subscriber has got, so drain hands over only
	// what is new — replaying the queued event would paper over a later one.
	delivered int
}

func newVoiceBuildFixture(t *testing.T) *voiceBuildFixture {
	t.Helper()
	e := Setup(t)
	store := ai.NewVoiceStore(e.DB())
	owner := e.As(e.Rep1, []ids.UUID{e.Team1}, voiceRepPerms)
	profile, err := store.CreateProfile(owner, ai.CreateVoiceProfileInput{})
	if err != nil {
		t.Fatalf("creating the profile: %v", err)
	}
	if _, _, _, err := store.IngestSource(owner, profile.ID, ai.IngestSourceInput{
		Kind: "document", SourceLabel: "seed", SourceRef: "seed", Content: strings.Repeat("word ", 800),
	}); err != nil {
		t.Fatalf("seeding the corpus: %v", err)
	}
	build, err := store.CreateBuild(owner, profile.ID, ai.CreateVoiceBuildInput{Reason: "manual"})
	if err != nil {
		t.Fatalf("asking for the build: %v", err)
	}
	return &voiceBuildFixture{
		env: e, owner: owner, store: store,
		consumer: aiactivity.NewConsumer(aiactivity.NewStore(e.DB()), testLogger(t)),
		profile:  profile.ID, buildID: build.ID,
	}
}

// drain hands the consumer every ai_task.state_changed this build staged and
// has not yet delivered, oldest first.
func (f *voiceBuildFixture) drain(t *testing.T) {
	t.Helper()
	var raws [][]byte
	err := f.env.DB().Tx(context.Background(), func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `
			SELECT envelope FROM event_outbox
			 WHERE envelope->>'type' = 'ai_task.state_changed'
			   AND envelope->'payload'->>'occurrence_key' = $1
			 ORDER BY seq
			 OFFSET $2`, f.buildID.String(), f.delivered)
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
		t.Fatal("the build staged no new ai_task.state_changed — the transition happened and nothing downstream could learn of it")
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

func (f *voiceBuildFixture) projection(t *testing.T) projectedOccurrence {
	t.Helper()
	var got projectedOccurrence
	err := f.env.Pool.QueryRow(context.Background(), `
		SELECT kind, ai_task, state, attempt, actor_scope, actor_user_id,
		       started_at, stale_after, subject_type
		  FROM ai_task_run WHERE source = $1 AND occurrence_key = $2`,
		ai.VoiceBuildActivitySource, f.buildID.String()).
		Scan(&got.Kind, &got.AITask, &got.State, &got.Attempt, &got.ActorScope,
			&got.ActorUserID, &got.StartedAt, &got.StaleAfter, &got.SubjectType)
	if err != nil {
		t.Fatalf("reading the projected occurrence: %v", err)
	}
	return got
}

// claim takes the build the way the worker does, and reports the instant the
// claim was stamped — the fence every terminal transition is written against.
func (f *voiceBuildFixture) claim(t *testing.T) time.Time {
	t.Helper()
	input, claimed, err := f.store.ClaimBuild(f.owner, f.profile, f.buildID, voiceBuildReclaim)
	if err != nil || !claimed {
		t.Fatalf("claiming the build: claimed=%v err=%v", claimed, err)
	}
	if input.Build.StartedAt == nil {
		t.Fatal("a claimed build carries no started_at, so no terminal write could fence against it")
	}
	return *input.Build.StartedAt
}

// The line the router could never draw: a build is live and the rep's own from
// the moment they ask for it, not from the moment it finishes.
func TestAQueuedVoiceBuildIsProjectedAsTheContactsOwnLiveWork(t *testing.T) {
	f := newVoiceBuildFixture(t)
	f.drain(t)

	got := f.projection(t)
	if got.State != "queued" || got.Attempt != 1 {
		t.Fatalf("state/attempt = %s/%d, want queued/1", got.State, got.Attempt)
	}
	if got.ActorScope != "personal" || got.ActorUserID == nil || *got.ActorUserID != f.env.Rep1 {
		t.Fatalf("actor = %s/%v, want personal/%s — the human who asked owns the occurrence",
			got.ActorScope, got.ActorUserID, f.env.Rep1)
	}
	if got.Kind != ai.VoiceBuildAITask || got.AITask == nil || *got.AITask != ai.VoiceBuildAITask {
		t.Fatalf("kind/ai_task = %s/%v, want %s", got.Kind, got.AITask, ai.VoiceBuildAITask)
	}
	if got.StaleAfter == nil {
		t.Fatal("a queued occurrence carries no stale_after, so a queue nobody drains would render as live forever")
	}
	// NO subject: a build is about the reader's own writing voice, and there is
	// no second party to name.
	if got.SubjectType != nil {
		t.Errorf("the build named subject %q; it is about the reader's own voice", *got.SubjectType)
	}
}

// The claim moves the same occurrence to running, carrying the window a
// replacement worker may take it in — and the outcome settles it.
func TestAVoiceBuildRunsAndSettlesInTheProjection(t *testing.T) {
	f := newVoiceBuildFixture(t)
	claimedAt := f.claim(t)
	f.drain(t)

	running := f.projection(t)
	if running.State != "running" || running.Attempt != 1 {
		t.Fatalf("after the claim, state/attempt = %s/%d, want running/1", running.State, running.Attempt)
	}
	if running.StartedAt == nil || running.StaleAfter == nil {
		t.Fatalf("a claimed build carries started_at %v and stale_after %v; a worker that dies must become stalled",
			running.StartedAt, running.StaleAfter)
	}

	if err := f.store.FailBuild(f.owner, f.buildID, claimedAt,
		"model_unavailable", "The model was unavailable. Try again shortly."); err != nil {
		t.Fatalf("failing the build: %v", err)
	}
	f.drain(t)

	settled := f.projection(t)
	if settled.State != "failed" {
		t.Fatalf("state = %s, want failed", settled.State)
	}
	if settled.StaleAfter != nil {
		t.Error("a settled occurrence still carries a lease; a closed line has nothing to go stale")
	}
}

// A build parked for budget SETTLES rather than staying live, and the next
// claim reopens it under a new attempt.
//
// The attempt is what the migration is for: the projection orders two events
// for one occurrence by it, and a `running` that supersedes a settled deferral
// is indistinguishable from a stale redelivery without one.
func TestADeferredVoiceBuildSettlesAndTheNextClaimReopensIt(t *testing.T) {
	f := newVoiceBuildFixture(t)
	claimedAt := f.claim(t)
	f.drain(t)

	due := time.Now().Add(-time.Second)
	if err := f.store.DeferBuild(f.owner, f.buildID, claimedAt,
		"The workspace is out of AI credit until the window reopens.", due); err != nil {
		t.Fatalf("deferring the build: %v", err)
	}
	f.drain(t)

	parked := f.projection(t)
	if parked.State != "degraded" || parked.Attempt != 1 {
		t.Fatalf("after the deferral, state/attempt = %s/%d, want degraded/1 — the build is not being worked",
			parked.State, parked.Attempt)
	}
	if parked.StaleAfter != nil {
		t.Error("a deferred build still carries a lease, so an orb would report work that is not happening")
	}

	f.claim(t)
	f.drain(t)

	resumed := f.projection(t)
	if resumed.State != "running" || resumed.Attempt != 2 {
		t.Fatalf("after the next claim, state/attempt = %s/%d, want running/2 — the same build on a new attempt",
			resumed.State, resumed.Attempt)
	}
	if resumed.StaleAfter == nil {
		t.Error("the resumed build carries no lease, so a worker that dies on the second attempt renders as live forever")
	}
}
