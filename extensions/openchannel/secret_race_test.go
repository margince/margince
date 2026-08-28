// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package openchannel

// What a mint answers when another mint got there first.
//
// The seal cannot be done inside this unit's transaction — Secrets().PutUser
// opens a pool transaction of its own, and nesting one inside another takes a
// second connection while holding the first, which on a small pool hangs rather
// than failing. So the mint claims a version under the row lock, closes that
// transaction, seals, and records under the version it claimed.
//
// That leaves exactly one window worth asserting about: another mint claiming
// and recording in between. The loser must be TOLD, because the alternative is
// a caller holding a secret it believes is live while the endpoint verifies
// with somebody else's.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
)

// TestAMintWhoseClaimWasSupersededAnswersConflict scripts the recording
// transaction finding a version other than the one this mint reserved, which is
// what a second mint completing in the window looks like from here.
func TestAMintWhoseClaimWasSupersededAnswersConflict(t *testing.T) {
	t.Parallel()
	rt := newRuntime()
	superseded := endpointRow(endpointID, ownerUserID, "", true)
	// The version column, last in the projection: the endpoint has moved past
	// the version this mint's claim reserved.
	superseded[len(superseded)-1] = 9
	rt.tx.singleRows = [][]any{
		endpointRow(endpointID, ownerUserID, "", true),
		// The secret_version this mint reserved.
		{1},
		superseded,
		// What the endpoint's secret_version actually is by the time this mint
		// records: another mint reserved after it.
		{9},
	}

	_, err := mintSecret(context.Background(), rt, json.RawMessage(ownArgs))
	if err == nil {
		t.Fatal("a mint superseded by another answered success, so its caller holds a secret the endpoint does not verify with")
	}
	if !errors.Is(err, extension.ErrConflict) {
		t.Fatalf("a superseded mint answered %v, which does not carry the conflict a caller retries on", err)
	}
}

// And the ordinary path still answers a secret: a claim nobody superseded
// records and returns, or the guard above has simply broken minting.
func TestAMintWhoseClaimStandsAnswersTheSecret(t *testing.T) {
	t.Parallel()
	rt := newRuntime()
	rt.tx.singleRows = [][]any{
		endpointRow(endpointID, ownerUserID, "", true),
		// The secret_version the claim reserved.
		{1},
		endpointRow(endpointID, ownerUserID, "", true),
		// The endpoint's live secret_version: unchanged, so the claim stands.
		{1},
		endpointRow(endpointID, ownerUserID, "", true),
	}

	out, err := mintSecret(context.Background(), rt, json.RawMessage(ownArgs))
	if err != nil {
		t.Fatalf("an unopposed mint failed: %v", err)
	}
	if jsonOf[mintedSecret](t, out).SigningSecret == "" {
		t.Fatal("an unopposed mint answered no secret")
	}
}
