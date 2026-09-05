// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A website read reports itself, end to end, the way a document reading does
// (aiactivity_e2e_integration_test.go): every read here is BORN through
// people.Store — StartSiteRead, BeginSiteRead, DeferSiteRead, FinishSiteRead —
// and every envelope is read back out of event_outbox rather than hand-built,
// so what the projection receives is exactly what production staged.
//
// What this proves is the thing the rail needs: a read a person starts is
// their own live work from the moment it is queued, stays live while the
// worker holds it, and settles from the dossier's own outcome — so the orb can
// light for the crawl and rest when it ends.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/aiactivity"
	"github.com/margince/margince/backend/internal/modules/people"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// siteReadLease stands in for the worker's reclaim window, which compose
// computes from the crawl caps. Any positive value proves the shape.
const siteReadLease = 10 * time.Minute

// websiteReadFixture is one real company, the read a rep started on it, and the
// consumer that projects what the dossier announces.
type websiteReadFixture struct {
	env      *Env
	rep      context.Context
	worker   context.Context
	consumer *aiactivity.Consumer
	org      ids.OrganizationID
	readID   ids.UUID
	// delivered is how far the fixture's subscriber has got, so drain hands
	// the consumer only what it has not seen — a replay of the first envelope
	// would paper over a later one the projection got wrong.
	delivered int
}

func newWebsiteReadFixture(t *testing.T) *websiteReadFixture {
	t.Helper()
	e := Setup(t)
	rep := e.As(e.Rep1, nil, AdminPerms)
	org := ids.From[ids.OrganizationKind](e.SeedOrg(t, "Acme Systems", &e.Rep1))
	read, joined, err := e.People.StartSiteRead(rep, org, "https://acme.example", "human:"+e.Rep1.String())
	if err != nil {
		t.Fatalf("StartSiteRead: %v", err)
	}
	if joined {
		t.Fatal("the first start joined — the fixture is not clean")
	}
	// The worker's principal as compose binds it after the claim: a system
	// actor on behalf of the human the dossier names, under the read's own
	// correlation. The projection attributes from the envelope, so this is
	// what makes the worker's transitions land in the rep's feed.
	worker := principal.WithWorkspaceID(context.Background(), e.WS)
	worker = principal.WithCorrelationID(worker, read.ID)
	worker = principal.WithActor(worker, principal.Principal{
		Type: principal.PrincipalSystem, ID: "agent:deepread",
		UserID: e.Rep1, OnBehalfOf: e.Rep1,
	})
	return &websiteReadFixture{
		env:      e,
		rep:      rep,
		worker:   worker,
		consumer: aiactivity.NewConsumer(aiactivity.NewStore(e.DB()), testLogger(t)),
		org:      org,
		readID:   read.ID,
	}
}

// drain hands the consumer every ai_task.state_changed this read staged and
// has not yet delivered, oldest first.
func (f *websiteReadFixture) drain(t *testing.T) {
	t.Helper()
	var raws [][]byte
	err := f.env.DB().Tx(context.Background(), func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `
			SELECT envelope FROM event_outbox
			 WHERE envelope->>'type' = 'ai_task.state_changed'
			   AND envelope->'payload'->>'source' = $1
			   AND envelope->'payload'->>'occurrence_key' = $2
			 ORDER BY seq
			 OFFSET $3`, people.SiteReadActivitySource, f.readID.String(), f.delivered)
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
		t.Fatal("the read staged no new ai_task.state_changed — the transition happened and nothing downstream could learn of it")
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

// projectedRead is the occurrence as the projection holds it.
type projectedRead struct {
	Kind          string
	AITask        *string
	State         string
	Attempt       int
	ActorScope    string
	ActorUserID   *ids.UUID
	StartedAt     *time.Time
	FinishedAt    *time.Time
	StaleAfter    *time.Time
	SubjectType   *string
	SubjectID     *ids.UUID
	SubjectLabel  *string
	DegradeReason *string
}

func (f *websiteReadFixture) projection(t *testing.T) projectedRead {
	t.Helper()
	var got projectedRead
	err := f.env.Pool.QueryRow(context.Background(), `
		SELECT kind, ai_task, state, attempt, actor_scope, actor_user_id,
		       started_at, finished_at, stale_after, subject_type, subject_id, subject_label, degrade_reason
		  FROM ai_task_run WHERE source = $1 AND occurrence_key = $2`,
		people.SiteReadActivitySource, f.readID.String()).
		Scan(&got.Kind, &got.AITask, &got.State, &got.Attempt, &got.ActorScope, &got.ActorUserID,
			&got.StartedAt, &got.FinishedAt, &got.StaleAfter, &got.SubjectType, &got.SubjectID,
			&got.SubjectLabel, &got.DegradeReason)
	if err != nil {
		t.Fatalf("reading the projected occurrence: %v", err)
	}
	return got
}

// The button press itself is the first line: a read a rep asked for is that
// rep's live work from the moment it is queued, named for the company, with a
// lease so a queue nobody drains cannot render as live forever.
func TestAQueuedWebsiteReadIsProjectedAsTheRepsOwnLiveWork(t *testing.T) {
	f := newWebsiteReadFixture(t)
	f.drain(t)

	got := f.projection(t)
	if got.State != "queued" || got.Attempt != 1 {
		t.Fatalf("state/attempt = %s/%d, want queued/1", got.State, got.Attempt)
	}
	if got.Kind != people.SiteReadActivityKind || got.AITask != nil {
		t.Fatalf("kind/ai_task = %s/%v, want %s with no task — a read is an occurrence of no single model call",
			got.Kind, got.AITask, people.SiteReadActivityKind)
	}
	if got.ActorScope != "personal" || got.ActorUserID == nil || *got.ActorUserID != f.env.Rep1 {
		t.Fatalf("actor = %s/%v, want personal/%s — the human who pressed the button owns the occurrence",
			got.ActorScope, got.ActorUserID, f.env.Rep1)
	}
	if got.StaleAfter == nil {
		t.Fatal("a queued occurrence carries no stale_after, so a queue nobody drains would render as live forever")
	}
	if got.SubjectType == nil || *got.SubjectType != "organization" || got.SubjectID == nil || *got.SubjectID != f.org.UUID {
		t.Fatalf("subject = %v/%v, want organization/%s", got.SubjectType, got.SubjectID, f.org)
	}
	if got.SubjectLabel == nil || *got.SubjectLabel != "Acme Systems" {
		t.Fatalf("subject_label = %v, want the company's own name, so the rail can say which website it is reading", got.SubjectLabel)
	}
}

// The worker's claim moves the same occurrence to running — the orb's `ingest`
// for the whole crawl — and the crawl's outcome settles it. A finished read
// stops carrying a lease: nothing about a closed occurrence can go stale.
func TestAWebsiteReadRunsAndSettlesInTheProjection(t *testing.T) {
	f := newWebsiteReadFixture(t)
	claim, err := f.env.People.BeginSiteRead(f.worker, f.readID, siteReadLease)
	if err != nil {
		t.Fatalf("BeginSiteRead: %v", err)
	}
	f.drain(t)
	got := f.projection(t)
	if got.State != "running" || got.Attempt != 1 || got.StartedAt == nil {
		t.Fatalf("after the claim, state/attempt/started_at = %s/%d/%v, want running/1/set", got.State, got.Attempt, got.StartedAt)
	}
	if got.StaleAfter == nil || !got.StaleAfter.Equal(claim.ClaimedAt.Add(siteReadLease)) {
		t.Fatalf("stale_after = %v, want the claim's own lease %v — past it a replacement may take the read, so past it the rail stops believing it",
			got.StaleAfter, claim.ClaimedAt.Add(siteReadLease))
	}
	if got.ActorScope != "personal" || got.ActorUserID == nil || *got.ActorUserID != f.env.Rep1 {
		t.Fatalf("the worker's claim re-attributed the read to %s/%v; it stays the rep's", got.ActorScope, got.ActorUserID)
	}

	if err := f.env.People.FinishSiteRead(f.worker, f.readID, people.FinishSiteReadInput{
		Status: "done", ClaimedAt: &claim.ClaimedAt,
		Pages: []people.SiteReadPage{{URL: "https://acme.example", Kind: "home"}},
	}); err != nil {
		t.Fatalf("FinishSiteRead: %v", err)
	}
	f.drain(t)
	got = f.projection(t)
	if got.State != "done" || got.FinishedAt == nil {
		t.Fatalf("after the outcome, state/finished_at = %s/%v, want done/set", got.State, got.FinishedAt)
	}
	if got.StaleAfter != nil {
		t.Fatalf("a settled occurrence carries stale_after %v; it is not claiming to work, so it has nothing to go stale", *got.StaleAfter)
	}
	if got.DegradeReason != nil {
		t.Fatalf("a clean finish carries degrade_reason %q", *got.DegradeReason)
	}
}

// The read's own vocabulary has words the projection's does not, and each maps
// to the honest one: a truncated crawl is `degraded` with the reason it
// stopped, never `done`.
func TestATruncatedWebsiteReadSettlesDegradedWithItsStopReason(t *testing.T) {
	f := newWebsiteReadFixture(t)
	claim, err := f.env.People.BeginSiteRead(f.worker, f.readID, siteReadLease)
	if err != nil {
		t.Fatalf("BeginSiteRead: %v", err)
	}
	stopped := "page_cap"
	if err := f.env.People.FinishSiteRead(f.worker, f.readID, people.FinishSiteReadInput{
		Status: "partial", ClaimedAt: &claim.ClaimedAt, StoppedReason: &stopped,
	}); err != nil {
		t.Fatalf("FinishSiteRead: %v", err)
	}
	f.drain(t)

	got := f.projection(t)
	if got.State != "degraded" {
		t.Fatalf("state = %s, want degraded — a partial read must never read as done", got.State)
	}
	if got.DegradeReason == nil || *got.DegradeReason != "The read stopped at its page limit." {
		t.Fatalf("degrade_reason = %v, want the closed sentence for a page cap", got.DegradeReason)
	}
}

// A budget deferral hands the read back to its carrier for hours. The
// occurrence settles rather than staying live — an orb lit that whole time
// would report a crawl that is not happening — and the claim that takes the
// read up again reopens it under a NEW attempt, which is the reopening the
// projection's guard admits and a state-only guard would refuse.
func TestADeferredWebsiteReadSettlesAndReopensOnTheNextClaim(t *testing.T) {
	f := newWebsiteReadFixture(t)
	if _, err := f.env.People.BeginSiteRead(f.worker, f.readID, siteReadLease); err != nil {
		t.Fatalf("BeginSiteRead: %v", err)
	}
	f.drain(t)
	// Due already, so the next claim's deferred-and-due arm admits it without
	// this test waiting on a clock.
	due := time.Now().Add(-time.Second)
	if err := f.env.People.DeferSiteRead(f.worker, f.readID, due); err != nil {
		t.Fatalf("DeferSiteRead: %v", err)
	}
	f.drain(t)
	got := f.projection(t)
	if got.State != "degraded" || got.Attempt != 1 || got.FinishedAt == nil || got.StaleAfter != nil {
		t.Fatalf("after the deferral, state/attempt/finished_at/stale_after = %s/%d/%v/%v, want degraded/1/set/nil",
			got.State, got.Attempt, got.FinishedAt, got.StaleAfter)
	}
	if got.DegradeReason == nil || *got.DegradeReason == "" {
		t.Fatal("a deferral says why it stopped, and the occurrence carries no reason")
	}

	if _, err := f.env.People.BeginSiteRead(f.worker, f.readID, siteReadLease); err != nil {
		t.Fatalf("BeginSiteRead after the deferral: %v", err)
	}
	f.drain(t)
	got = f.projection(t)
	if got.State != "running" || got.Attempt != 2 {
		t.Fatalf("after the second claim, state/attempt = %s/%d, want running/2 — a row frozen at degraded is a read the UI says stopped while a worker crawls it",
			got.State, got.Attempt)
	}
	if got.FinishedAt != nil || got.DegradeReason != nil {
		t.Fatalf("the reopened occurrence kept the deferral's finished_at %v / reason %v", got.FinishedAt, got.DegradeReason)
	}
}
