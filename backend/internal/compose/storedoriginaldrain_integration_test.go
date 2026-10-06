// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The drain's two promises beyond the marker: it holds one stored original at
// a time rather than the whole batch's, and an original removed between the
// offer and its turn is passed over rather than failing or mis-marking the
// batch.

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// liveHeap answers the bytes still reachable after a full collection, which is
// what the drain is holding rather than what it has merely allocated.
func liveHeap() uint64 {
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc
}

// A batch's peak live heap must not scale with batch × payload. The settle hook
// samples the live heap as each candidate's turn comes; an offer that carried
// every payload would have all of them reachable at the first sample.
func TestTheDrainHoldsOneStoredOriginalAtATime(t *testing.T) {
	e := integration.Setup(t)
	const (
		candidates  = 12
		payloadSize = 4 << 20
	)
	body := strings.Repeat(strings.Repeat("x", 76)+"\r\n", payloadSize/78)
	for i := range candidates {
		seedReplayableMail(t, e, fmt.Sprintf("msg-large-%02d", i), ccMessage+body)
	}

	baseline := liveHeap()
	var peak uint64
	var seen int
	pass := storedOriginalPass[replayCandidate]{
		name: "participant replay", unit: "activities",
		offer: selectReplayCandidates,
		settle: func(ctx context.Context, tx pgx.Tx, c replayCandidate, payload []byte) (string, error) {
			if len(payload) < payloadSize {
				t.Errorf("the settle step saw a %d-byte original, want the stored %d", len(payload), payloadSize)
			}
			seen++
			if live := liveHeap(); live > baseline && live-baseline > peak {
				peak = live - baseline
			}
			return replayOne(ctx, tx, c, payload)
		},
		mark: markReplayed,
	}
	settled, err := drainStoredOriginals(replayCtx(e), e.Pool, candidates, slog.Default(), pass)
	if err != nil {
		t.Fatalf("draining the large originals: %v", err)
	}
	if settled != candidates || seen != candidates {
		t.Fatalf("settled %d and saw %d, want %d of each", settled, seen, candidates)
	}
	// Every original together is twelve payloads. One at a time is the current
	// payload plus the driver's read buffer for it, which is rounded up to a
	// power of two; four payloads' worth leaves room for that and still fails
	// loudly on anything that holds the batch.
	if limit := uint64(4 * payloadSize); peak > limit {
		t.Errorf("peak live heap during the drain was %d bytes, over %d — the batch's originals are held together", peak, limit)
	}
}

// An original that goes before its turn — an erasure or a retention sweep
// running beside the pass — is passed over: no marker, no count, no error, and
// the candidates on either side of it are settled as usual.
func TestTheDrainPassesOverAnOriginalRemovedBeforeItsTurn(t *testing.T) {
	e := integration.Setup(t)
	first := seedReplayableMail(t, e, "msg-vanish-a", ccMessage)
	gone := seedReplayableMail(t, e, "msg-vanish-b", ccMessage)
	last := seedReplayableMail(t, e, "msg-vanish-c", ccMessage)
	owner := integration.OwnerConn(t)

	pass := storedOriginalPass[replayCandidate]{
		name: "participant replay", unit: "activities",
		offer: func(ctx context.Context, tx pgx.Tx, limit int) ([]replayCandidate, error) {
			out, err := selectReplayCandidates(ctx, tx, limit)
			if err != nil {
				return nil, err
			}
			if _, err := owner.Exec(context.Background(),
				`DELETE FROM raw_capture WHERE source_id = 'msg-vanish-b'`); err != nil {
				t.Fatalf("removing the original mid-batch: %v", err)
			}
			return out, nil
		},
		settle: replayOne,
		mark:   markReplayed,
	}
	settled, err := drainStoredOriginals(replayCtx(e), e.Pool, 10, slog.Default(), pass)
	if err != nil {
		t.Fatalf("draining with an original removed mid-batch: %v", err)
	}
	if settled != 2 {
		t.Errorf("settled = %d, want the two originals still on file", settled)
	}
	for _, id := range []ids.UUID{first, last} {
		if _, found := replayOutcome(t, id); !found {
			t.Errorf("activity %s beside the removed original was not settled", id)
		}
	}
	if outcome, found := replayOutcome(t, gone); found {
		t.Errorf("the activity whose original was removed was marked %q from bytes nobody read", outcome)
	}
	again, err := replayParticipantsBatch(replayCtx(e), e.Pool, 10, slog.Default())
	if err != nil {
		t.Fatalf("the pass after the removal: %v", err)
	}
	if again != 0 {
		t.Errorf("the next pass settled %d, want 0 — nothing is left on file to offer", again)
	}
}

// readStoredOriginal answers not-found for a row that does not exist, and the
// bytes for one that does.
func TestReadStoredOriginalAnswersWhetherTheRowIsOnFile(t *testing.T) {
	e := integration.Setup(t)
	seedReplayableMail(t, e, "msg-read", ccMessage)
	var rawID ids.UUID
	if err := integration.OwnerConn(t).QueryRow(context.Background(),
		`SELECT id FROM raw_capture WHERE source_id = 'msg-read'`).Scan(&rawID); err != nil {
		t.Fatalf("reading the seeded original's id: %v", err)
	}
	if err := database.WithWorkspaceTx(replayCtx(e), e.Pool, func(tx pgx.Tx) error {
		payload, found, err := readStoredOriginal(context.Background(), tx, rawID)
		if err != nil || !found || len(payload) == 0 {
			t.Errorf("the seeded original: found=%v len=%d err=%v", found, len(payload), err)
		}
		payload, found, err = readStoredOriginal(context.Background(), tx, ids.NewV7())
		if err != nil || found || payload != nil {
			t.Errorf("a missing original: found=%v len=%d err=%v", found, len(payload), err)
		}
		return nil
	}); err != nil {
		t.Fatalf("the read transaction: %v", err)
	}
}
