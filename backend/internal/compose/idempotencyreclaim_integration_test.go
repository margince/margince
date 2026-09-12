// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The claim transaction's re-claim arm, over a real Postgres: when the
// retention sweep removes a row between this attempt's INSERT and its read,
// the attempt claims the key again — and when a rival wins that race, what
// the rival left decides the answer.
//
// It used to be hardcoded: whatever the winner had recorded, the loser was
// told "a request with this idempotency key is still in progress". A settled
// claim and a claim under a different request body are different things to be
// told, and a client that branches on the two 409 details was told the wrong
// one.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// reclaimVerdict runs the read-back pass — the one that reports what a rival
// left after this attempt lost the re-claim — against whatever the fixture put
// in the table.
func reclaimVerdict(t *testing.T, e *integration.Env, principalID, key, digest string) claimOutcome {
	t.Helper()
	var got claimOutcome
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		outcome, _, _, err := resolveClaimRow(context.Background(), tx, principalID, key, "POST /v1/contacts", digest, false)
		got = outcome
		return err
	}); err != nil {
		t.Fatalf("reading back the rival's claim: %v", err)
	}
	return got
}

func TestALostReclaimReportsWhatTheRivalActuallyLeft(t *testing.T) {
	e := integration.Setup(t)
	principalID := "human:" + ids.NewV7().String()

	// The rival settled: its response is recorded and this attempt's own
	// digest matches, so the honest answer is a replay of that response —
	// not "still in progress", which is what the hardcoded branch said.
	settled := ids.NewV7().String()
	e.WsExec(t, `
		INSERT INTO idempotency_key
		  (principal_id, key, endpoint, request_digest,
		   response_status, response_body, response_content_type, created_at)
		VALUES ($1, $2, 'POST /v1/contacts', 'digest',
		        201, '{"full_name":"Ada"}', 'application/json', now())`,
		principalID, settled)
	if got := reclaimVerdict(t, e, principalID, settled, "digest"); got != claimReplay {
		t.Errorf("a settled rival reports %v, want claimReplay — its response is recorded and this request is the same one", got)
	}

	// The rival holds the same key for a DIFFERENT body. "Already used with a
	// different request body" is the answer; telling this caller their own
	// retry is in flight sends them to retry a key that will never accept it.
	mismatched := ids.NewV7().String()
	e.WsExec(t, `
		INSERT INTO idempotency_key
		  (principal_id, key, endpoint, request_digest, created_at)
		VALUES ($1, $2, 'POST /v1/contacts', 'the-rivals-digest', now())`,
		principalID, mismatched)
	if got := reclaimVerdict(t, e, principalID, mismatched, "our-digest"); got != claimMismatch {
		t.Errorf("a rival holding a different body reports %v, want claimMismatch", got)
	}

	// And an unsettled rival under the SAME body genuinely is in flight —
	// the verdict the old branch always gave, now given for a reason.
	inFlight := ids.NewV7().String()
	e.WsExec(t, `
		INSERT INTO idempotency_key
		  (principal_id, key, endpoint, request_digest, created_at)
		VALUES ($1, $2, 'POST /v1/contacts', 'digest', now())`,
		principalID, inFlight)
	if got := reclaimVerdict(t, e, principalID, inFlight, "digest"); got != claimInProgress {
		t.Errorf("an unsettled rival under one body reports %v, want claimInProgress", got)
	}
}

// A row that vanishes TWICE inside one transaction has nothing left to read.
// The pass stops rather than looping, and answers in-flight — the verdict that
// costs least if it is wrong, because it tells the caller to retry rather than
// inventing a result.
func TestAClaimThatVanishesTwiceStopsRatherThanLooping(t *testing.T) {
	e := integration.Setup(t)
	if got := reclaimVerdict(t, e, "human:"+ids.NewV7().String(), ids.NewV7().String(), "digest"); got != claimInProgress {
		t.Errorf("a claim with no row at all reports %v, want claimInProgress", got)
	}
}

// A settlement from an attempt whose claim EXPIRED writes nothing.
//
// The claim row is identified by (principal, key, endpoint), and that was the
// whole identity a settle or a release matched. But the row is re-claimed in
// place once past the replay window — new digest, cleared response, same three
// columns — so a first attempt still in flight when its claim expired could
// return and settle the replacement's row. Two harms, and both are asserted
// here: it would overwrite a result a later replay serves for the wrong call,
// and it would delete a live claim, letting a third attempt run beside the
// second.
//
// The window is narrow — an attempt has to outlive the replay window, and both
// transports are bounded far below it — which is why the fix is a predicate
// rather than a lock. attempt_id is stamped fresh on the claim and on the
// re-claim, so a settlement names the attempt that made it or names nothing.
func TestASettlementFromAnExpiredAttemptWritesNothing(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	actor, _ := principal.Actor(ctx)
	const key, endpoint, digest = "k-expired", "POST /v1/contacts", "digest-first"

	// The first attempt takes the key and is still running.
	_, stale, _, err := claimKey(ctx, e.Pool, actor.ID, key, endpoint, digest)
	if err != nil {
		t.Fatalf("the first claim failed: %v", err)
	}

	// Its claim ages past the replay window while it runs.
	e.WsExec(t, `UPDATE idempotency_key SET created_at = now() - interval '48 hours' WHERE key = $1`, key)

	// A second attempt re-claims the key in place, under a new attempt id.
	outcome, live, _, err := claimKey(ctx, e.Pool, actor.ID, key, endpoint, "digest-second")
	if err != nil {
		t.Fatalf("the re-claim failed: %v", err)
	}
	if outcome != claimFresh {
		t.Fatalf("re-claiming an expired key = %v, want fresh", outcome)
	}
	if live.attempt == stale.attempt {
		t.Fatal("the re-claim kept the previous attempt's id, so the two are indistinguishable")
	}

	// Now the FIRST attempt returns and settles. It must write nothing.
	if err := settleClaim(ctx, e.Pool, stale, 200, `{"stale":true}`, "application/json", 0, true); err != nil {
		t.Fatalf("the stale settlement errored: %v", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM idempotency_key WHERE key = $1 AND response_status IS NULL`, key); n != 1 {
		t.Error("a stale settlement recorded a result on the replacement's claim — a later replay would " +
			"answer the wrong call's document under a key that looks settled")
	}

	// And its release must not free the live claim either.
	if err := settleClaim(ctx, e.Pool, stale, 409, `{"code":"conflict"}`, "application/json", 0, false); err != nil {
		t.Fatalf("the stale release errored: %v", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM idempotency_key WHERE key = $1`, key); n != 1 {
		t.Error("a stale release deleted the replacement's live claim — the key is free while the " +
			"attempt holding it is still running, so a third attempt executes beside it")
	}

	// The live attempt still settles, which is what makes the above a
	// predicate rather than a lock.
	if err := settleClaim(ctx, e.Pool, live, 200, `{"live":true}`, "application/json", 0, true); err != nil {
		t.Fatalf("the live settlement errored: %v", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM idempotency_key WHERE key = $1 AND response_status = 200`, key); n != 1 {
		t.Error("the attempt that holds the claim could not settle it")
	}
}
