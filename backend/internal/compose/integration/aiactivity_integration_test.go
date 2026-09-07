// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The projection's guard, against a real database.
//
// The guard IS SQL — a tuple comparison inside an ON CONFLICT predicate — so a
// unit test with a fake store would prove nothing about it. Every case here
// goes through aiactivity.Store.ApplyStateChange, which is the only writer
// ai_task_run has.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/aiactivity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// aiActivityEnv is the shared harness plus the projection under test and one
// occurrence key, so each test owns its own row.
type aiActivityEnv struct {
	env           *Env
	store         *aiactivity.Store
	occurrenceKey string
	// queuedAt comes from the DATABASE clock: the CHECKs this suite exercises
	// compare timestamps the database stamped against ones it is handed, and a
	// "now" computed on the test host names a different instant.
	queuedAt time.Time
}

func newAIActivityEnv(t *testing.T) *aiActivityEnv {
	t.Helper()
	env := Setup(t)
	owner := OwnerConn(t)

	var now time.Time
	if err := owner.QueryRow(context.Background(), `SELECT now()`).Scan(&now); err != nil {
		t.Fatalf("reading the database clock: %v", err)
	}
	return &aiActivityEnv{
		env:           env,
		store:         aiactivity.NewStore(env.DB()),
		occurrenceKey: "attachment_extraction:" + ids.NewV7().String(),
		queuedAt:      now.UTC(),
	}
}

// change is the per-test shorthand: everything a state needs is derived from
// the state itself, so no case has to restate the CHECKs it is not about.
type change struct {
	attempt       int
	state         string
	degradeReason string
	// staleAfter is the lease's end as the source declared it; nil for a
	// source that declares none, which is what most cases here are about.
	staleAfter *time.Time
}

func (a *aiActivityEnv) build(c change) aiactivity.Change {
	out := aiactivity.Change{
		Source:        "attachment_extraction",
		OccurrenceKey: a.occurrenceKey,
		Kind:          "document_extract",
		AITask:        "document_extract",
		Attempt:       c.attempt,
		ActorScope:    aiactivity.ScopePersonal,
		ActorUserID:   a.env.Rep1,
		State:         c.state,
		QueuedAt:      a.queuedAt,
		StaleAfter:    c.staleAfter,
		DegradeReason: c.degradeReason,
		EventID:       ids.NewV7(),
	}
	started := a.queuedAt.Add(time.Second)
	finished := a.queuedAt.Add(2 * time.Second)
	if c.state != "queued" {
		out.StartedAt = &started
	}
	if c.state == "done" || c.state == "degraded" || c.state == "failed" {
		out.FinishedAt = &finished
	}
	return out
}

// applyChange projects one change and returns whether the guard admitted it.
func (a *aiActivityEnv) applyChange(t *testing.T, c change) bool {
	t.Helper()
	applied, err := a.store.ApplyStateChange(a.env.Admin(), a.build(c))
	if err != nil {
		t.Fatalf("ApplyStateChange(%+v): %v", c, err)
	}
	return applied
}

// apply projects a change that MUST land — the setup steps, where a refusal
// would silently leave a later assertion testing a state it never reached.
func (a *aiActivityEnv) apply(t *testing.T, c change) {
	t.Helper()
	if !a.applyChange(t, c) {
		t.Fatalf("setup change %+v was refused by the guard", c)
	}
}

// projected is the row as the table holds it.
type projected struct {
	State         string
	Attempt       int
	StartedAt     *time.Time
	FinishedAt    *time.Time
	StaleAfter    *time.Time
	DegradeReason *string
	Seq           int64
}

func (a *aiActivityEnv) read(t *testing.T) projected {
	t.Helper()
	var row projected
	err := a.env.Pool.QueryRow(context.Background(),
		`SELECT state, attempt, started_at, finished_at, stale_after, degrade_reason, seq
		   FROM ai_task_run WHERE source = $1 AND occurrence_key = $2`,
		"attachment_extraction", a.occurrenceKey).
		Scan(&row.State, &row.Attempt, &row.StartedAt, &row.FinishedAt, &row.StaleAfter, &row.DegradeReason, &row.Seq)
	if err != nil {
		t.Fatalf("reading the projected occurrence: %v", err)
	}
	return row
}

// A released reading goes BACKWARDS — running to queued — and the projection
// must follow it. This is the case a state-rank-only guard gets wrong: it
// refuses the re-queue and pins the row at a running nobody holds, which is
// precisely the stale state the design exists to prevent.
func TestAIActivityAReleasedAttemptReopensTheOccurrence(t *testing.T) {
	env := newAIActivityEnv(t)
	env.apply(t, change{attempt: 1, state: "queued"})
	env.apply(t, change{attempt: 1, state: "running"})

	if !env.applyChange(t, change{attempt: 2, state: "queued"}) {
		t.Fatal("attempt 2 queued must apply: a release legitimately reopens the occurrence")
	}
	row := env.read(t)
	if row.State != "queued" || row.Attempt != 2 {
		t.Fatalf("state/attempt = %s/%d, want queued/2", row.State, row.Attempt)
	}
	if row.StartedAt != nil {
		t.Fatal("a re-queued occurrence has no start; a left-over started_at would age into a false stale_after")
	}
}

// Within one attempt, settled is terminal. A redelivered running for the same
// attempt must not resurrect a finished occurrence.
func TestAIActivityALateRunningCannotResurrectASettledAttempt(t *testing.T) {
	env := newAIActivityEnv(t)
	env.apply(t, change{attempt: 1, state: "queued"})
	env.apply(t, change{attempt: 1, state: "running"})
	env.apply(t, change{attempt: 1, state: "done"})

	if env.applyChange(t, change{attempt: 1, state: "running"}) {
		t.Fatal("a late running for a settled attempt must be refused")
	}
	if got := env.read(t).State; got != "done" {
		t.Fatalf("state = %s, want done", got)
	}
}

// A higher attempt outranks a settled lower one: the document was read again.
func TestAIActivityAHigherAttemptSupersedesASettledLowerOne(t *testing.T) {
	env := newAIActivityEnv(t)
	env.apply(t, change{attempt: 1, state: "queued"})
	env.apply(t, change{attempt: 1, state: "failed", degradeReason: "extraction_timeout"})

	if !env.applyChange(t, change{attempt: 2, state: "queued"}) {
		t.Fatal("attempt 2 must supersede a settled attempt 1")
	}
	row := env.read(t)
	if row.FinishedAt != nil || row.DegradeReason != nil {
		t.Fatalf("reopening must clear the previous attempt's terminal facts, got finished_at=%v degrade_reason=%v — the row would read as both live and failed", row.FinishedAt, row.DegradeReason)
	}
}

// An exact redelivery — same attempt, same state — is a no-op. The bus is
// at-least-once, so this is the ordinary case, not the exotic one.
func TestAIActivityAnExactRedeliveryChangesNothing(t *testing.T) {
	env := newAIActivityEnv(t)
	env.apply(t, change{attempt: 1, state: "running"})
	before := env.read(t)

	if env.applyChange(t, change{attempt: 1, state: "running"}) {
		t.Fatal("an identical redelivery must be refused, not reapplied")
	}
	if after := env.read(t); after.Seq != before.Seq {
		t.Fatalf("a redelivery moved seq from %d to %d, so every streaming client would see a change that did not happen", before.Seq, after.Seq)
	}
}

// Every applied write takes a strictly higher seq, and a refused one takes
// none. Without that the delta cursor either misses a change or replays one.
func TestAIActivitySeqAdvancesOnlyOnAnAppliedWrite(t *testing.T) {
	env := newAIActivityEnv(t)
	env.apply(t, change{attempt: 1, state: "queued"})
	first := env.read(t).Seq
	env.apply(t, change{attempt: 1, state: "running"})
	second := env.read(t).Seq
	if second <= first {
		t.Fatalf("seq did not advance: %d then %d", first, second)
	}
	if env.applyChange(t, change{attempt: 1, state: "queued"}) {
		t.Fatal("a re-queue at the SAME attempt is a redelivery of a state already passed, and must be refused")
	}
	if got := env.read(t).Seq; got != second {
		t.Fatalf("a refused change moved seq from %d to %d", second, got)
	}
}

// leasedUntil is a stale_after for the cases about renewal, an offset from the
// same database instant every other timestamp here is derived from.
func (a *aiActivityEnv) leasedUntil(d time.Duration) *time.Time {
	until := a.queuedAt.Add(d)
	return &until
}

// The ONE write admitted at an equal (attempt, rank): the same live state,
// believable for longer. A source whose work spans several model calls leases
// one call at a time and renews before each, so the projection has to take a
// later stale_after on the row it already holds — refusing it is what forces a
// lease to be sized for the longest work it could cover, which is exactly how
// long a dead process then goes on being displayed as working.
func TestAIActivityARenewalExtendsALiveLease(t *testing.T) {
	env := newAIActivityEnv(t)
	env.apply(t, change{attempt: 1, state: "running", staleAfter: env.leasedUntil(5 * time.Minute)})
	before := env.read(t)

	if !env.applyChange(t, change{attempt: 1, state: "running", staleAfter: env.leasedUntil(10 * time.Minute)}) {
		t.Fatal("a renewal at the same attempt with a later lease must apply")
	}
	after := env.read(t)
	if after.StaleAfter == nil || !after.StaleAfter.Equal(*env.leasedUntil(10 * time.Minute)) {
		t.Errorf("stale_after after the renewal = %v, want the renewed %v", after.StaleAfter, env.leasedUntil(10*time.Minute))
	}
	if after.State != "running" || after.Attempt != 1 {
		t.Errorf("state/attempt after the renewal = %s/%d, want running/1 — a renewal extends the row, it does not move it", after.State, after.Attempt)
	}
	if after.Seq <= before.Seq {
		t.Errorf("seq did not advance on an applied renewal: %d then %d", before.Seq, after.Seq)
	}
}

// The renewal branch must stay as harmless under the at-least-once bus as the
// tuple guard is. A renewal delivered late carries an EARLIER lease than the
// row holds, and an exact redelivery carries an equal one; both must leave the
// row and its seq alone, or a redelivery would shorten a lease a later renewal
// had already extended.
func TestAIActivityARenewalDeliveredLateChangesNothing(t *testing.T) {
	env := newAIActivityEnv(t)
	env.apply(t, change{attempt: 1, state: "running", staleAfter: env.leasedUntil(10 * time.Minute)})
	before := env.read(t)

	if env.applyChange(t, change{attempt: 1, state: "running", staleAfter: env.leasedUntil(5 * time.Minute)}) {
		t.Fatal("a renewal carrying an earlier lease than the row holds must be refused")
	}
	if env.applyChange(t, change{attempt: 1, state: "running", staleAfter: env.leasedUntil(10 * time.Minute)}) {
		t.Fatal("a redelivered renewal carrying the lease the row already holds must be refused")
	}
	after := env.read(t)
	if after.StaleAfter == nil || !after.StaleAfter.Equal(*before.StaleAfter) {
		t.Errorf("stale_after moved from %v to %v on a refused renewal", before.StaleAfter, after.StaleAfter)
	}
	if after.Seq != before.Seq {
		t.Errorf("a refused renewal moved seq from %d to %d", before.Seq, after.Seq)
	}
}

// A settled occurrence has no lease to renew, and the branch that admits a
// renewal must not become the way a late running reopens it: the settled row's
// stale_after is NULL, and nothing compares later than that.
func TestAIActivityASettledOccurrenceCannotBeRenewedBackToLife(t *testing.T) {
	env := newAIActivityEnv(t)
	env.apply(t, change{attempt: 1, state: "running", staleAfter: env.leasedUntil(5 * time.Minute)})
	env.apply(t, change{attempt: 1, state: "done"})

	if env.applyChange(t, change{attempt: 1, state: "running", staleAfter: env.leasedUntil(time.Hour)}) {
		t.Fatal("a renewal for a settled attempt must be refused")
	}
	row := env.read(t)
	if row.State != "done" {
		t.Errorf("state = %s, want done", row.State)
	}
	if row.StaleAfter != nil {
		t.Errorf("a settled row took a lease until %v", row.StaleAfter)
	}
}

// Settled rows age out; live ones never do. A purge that took a live row would
// erase an occurrence its source still holds a claim on, and the reconciler
// would keep putting it back.
func TestAIActivityThePurgeTakesSettledRowsOnly(t *testing.T) {
	env := newAIActivityEnv(t)
	env.apply(t, change{attempt: 1, state: "running"})

	deleted, err := env.store.PurgeSettledBefore(env.env.Admin(), env.queuedAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("PurgeSettledBefore: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("the purge took %d live row(s); a live occurrence has no finish to age past", deleted)
	}

	env.apply(t, change{attempt: 1, state: "done"})
	deleted, err = env.store.PurgeSettledBefore(env.env.Admin(), env.queuedAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("PurgeSettledBefore: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("the purge took %d settled row(s), want 1", deleted)
	}
}
