// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The drain every pass over stored provider originals shares.
//
// A provider original runs to tens of megabytes, and a batch is up to a few
// hundred of them. So the offer names each candidate's original by id and
// never carries its bytes: the drain reads one payload immediately before the
// candidate it belongs to is settled, and lets it go before reading the next.
// What a batch holds is therefore one original at a time rather than the whole
// batch's, and the offer query streams identities rather than TOASTed
// payloads, which keeps it inside the statement ceiling however large the
// originals are. capture's part-slimming sweep reads its payloads the same
// way, for the same reason.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// storedOriginalPass is one sweep over stored provider originals: which rows to
// offer, what to do with each, and where the outcome is recorded.
//
// The passes share the DRAIN rather than each spelling it. The parts they have
// in common are the parts that are easy to get subtly wrong: one bounded
// transaction, one correlation id for the whole batch (an audited write is
// refused without one, which would fail the batch and re-select the same rows
// forever), a marker written for every row the pass touched so a settled row
// is never offered twice, and reading each original only when its turn comes.
// It is generic in the CANDIDATE because each pass needs a different set of
// facts about the row it is judging, and reading the extra ones back per row
// would be another read of `activity` per pass — another place to gate, and
// another chance to forget one. The harness itself needs only the two ids,
// which is what storedOriginalCandidate asks for.
type storedOriginalPass[C storedOriginalCandidate] struct {
	// name and unit are what the debug line says: which pass ran, and what its
	// count is counting.
	name string
	unit string
	// offer answers which rows this pass still owes work on. It names each
	// candidate's original and must not read its payload.
	offer func(ctx context.Context, tx pgx.Tx, limit int) ([]C, error)
	// settle judges one candidate against the payload the drain just read for
	// it. The payload is only valid for the duration of the call.
	settle func(ctx context.Context, tx pgx.Tx, c C, payload []byte) (string, error)
	mark   func(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, outcome string) error
}

// storedOriginalCandidate is what every pass's candidate must answer: which
// activity this is, and which stored original it is read from. The marker is
// written against the first, and the payload is read by the second.
type storedOriginalCandidate interface {
	activity() ids.ActivityID
	original() ids.UUID
}

// unitMeetings is what a meeting-shaped pass counts. Both meeting passes report
// through it, so their debug lines describe the same kind of work in the same
// word.
const unitMeetings = "meetings"

// drainStoredOriginals runs one bounded batch of a stored-original pass and
// answers how many rows it settled — written, empty or refused alike, because
// every one of them is progress the next pass will not repeat.
//
// A candidate whose original is gone by the time its turn comes is passed over
// unmarked and uncounted. It is no longer a row this pass can offer, since
// every offer joins on the original, so leaving it unmarked cannot make the
// loop re-select it; marking it would record a verdict about bytes nobody
// read.
func drainStoredOriginals[C storedOriginalCandidate](
	ctx context.Context, pool *pgxpool.Pool, limit int, log *slog.Logger, pass storedOriginalPass[C],
) (int, error) {
	if limit <= 0 {
		return 0, fmt.Errorf("compose: the %s needs a positive batch limit, got %d", pass.name, limit)
	}
	// One correlation id per batch. Naming an attendee is an audited write, and
	// storekit refuses to emit its event without one — a refusal that would
	// take the whole batch down with it and re-select the same rows forever.
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	var settled int
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		candidates, err := pass.offer(ctx, tx, limit)
		if err != nil {
			return err
		}
		for _, c := range candidates {
			done, err := settleStoredOriginal(ctx, tx, pass, c)
			if err != nil {
				return err
			}
			if done {
				settled++
			}
		}
		if settled > 0 {
			log.DebugContext(ctx, "compose: settled a batch of stored originals",
				"pass", pass.name, pass.unit, settled)
		}
		return nil
	})
	return settled, err
}

// settleStoredOriginal reads one candidate's original, judges it and records
// the outcome, answering whether the candidate was settled. It is its own
// function so the payload is unreachable the moment it returns.
func settleStoredOriginal[C storedOriginalCandidate](
	ctx context.Context, tx pgx.Tx, pass storedOriginalPass[C], c C,
) (bool, error) {
	payload, found, err := readStoredOriginal(ctx, tx, c.original())
	if err != nil || !found {
		return false, err
	}
	outcome, err := pass.settle(ctx, tx, c, payload)
	if err != nil {
		return false, err
	}
	if err := pass.mark(ctx, tx, c.activity(), outcome); err != nil {
		return false, err
	}
	return true, nil
}

// readStoredOriginal reads one raw_capture row's payload inside the pass's own
// transaction. A row that no longer exists is reported as not found rather than
// as a fault: an erasure or a retention sweep may remove an original between
// the offer and its turn, and that is not a reason to fail the batch.
//
// It composes no audience, and the passes that call it may not. They run as the
// system principal, and what survives their transaction is participant rows,
// a meeting status or a display name, never the original's text. Several of
// them establish the participant rows the audience test itself reads, so
// gating them on that answer would leave a limited activity permanently
// unreadable by the very contacts and colleagues who were on it.
func readStoredOriginal(ctx context.Context, tx pgx.Tx, id ids.UUID) ([]byte, bool, error) {
	var payload []byte
	err := tx.QueryRow(ctx, `SELECT payload FROM raw_capture WHERE id = $1`, id).Scan(&payload)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("compose: reading a stored original: %w", err)
	}
	return payload, true, nil
}
