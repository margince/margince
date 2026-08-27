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
)

// reclaimVerdict runs the read-back pass — the one that reports what a rival
// left after this attempt lost the re-claim — against whatever the fixture put
// in the table.
func reclaimVerdict(t *testing.T, e *integration.Env, principalID, key, digest string) claimOutcome {
	t.Helper()
	var got claimOutcome
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		outcome, _, err := resolveClaimRow(context.Background(), tx, principalID, key, "POST /v1/people", digest, false)
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
		VALUES ($1, $2, 'POST /v1/people', 'digest',
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
		VALUES ($1, $2, 'POST /v1/people', 'the-rivals-digest', now())`,
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
		VALUES ($1, $2, 'POST /v1/people', 'digest', now())`,
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
