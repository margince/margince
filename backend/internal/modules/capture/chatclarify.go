// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// A colleague's forward that names no author waits for the colleague to say who
// wrote it. The company account asks once for each batch of such forwards and
// files the batch on the customer the colleague names. The state sits on the
// stored original of each held message, so batch, question and message expire
// together. A held forward is a message with no contact
// and no link (pendingforward.Predicate); a batch stays open while any of its
// messages is still one.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/pendingforward"
)

// ClarifyBatchWindow is the longest pause inside one batch. Forwards of one
// colleague on one account that follow each other within it are asked about
// together.
const ClarifyBatchWindow = 2 * time.Minute

// clarifyMaxAttempts bounds the sends of one question that the provider refused
// without delivering. A batch past it stays held until it expires.
const clarifyMaxAttempts = 8

// clarifyRetryBase and clarifyRetryCap pace those retries: the pause doubles
// from the base up to the cap.
const (
	clarifyRetryBase = time.Minute
	clarifyRetryCap  = time.Hour
)

// clarifyScanLimit bounds the held forwards one sweep reads.
const clarifyScanLimit = 2000

// heldForwardMarker is implemented by a Sink that remembers who forwarded a held
// message and through which account, which is what the question is asked from.
type heldForwardMarker interface {
	MarkHeldForward(ctx context.Context, original, member, connection ids.UUID) error
}

// MarkHeldForward records on a held forward's stored original the colleague who
// sent it and the account it arrived through. A forward with no stored original
// cannot be asked about; the ingest workers always store one.
func (s *Sink) MarkHeldForward(ctx context.Context, original, member, connection ids.UUID) error {
	if original.IsZero() || member.IsZero() {
		return nil
	}
	var conn *ids.UUID
	if !connection.IsZero() {
		conn = &connection
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE raw_capture SET held_by = coalesce(held_by, $2), held_connection = coalesce(held_connection, $3)
			 WHERE id = $1`, original, member, conn)
		if err != nil {
			return fmt.Errorf("capture: marking a held forward: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("capture: the stored original of a held forward is gone: %w", apperrors.ErrNotFound)
		}
		return nil
	})
}

// ClarifyBatch is one batch of held forwards, asked about in one question.
type ClarifyBatch struct {
	ID ids.UUID
	// Member is the colleague who forwarded the messages.
	Member ids.UUID
	// Connection is the company account's channel_connection; zero for the
	// workspace's own bot.
	Connection ids.UUID
	// Provider and ThreadKey are the channel and conversation of the
	// colleague's chat with the account.
	Provider  string
	ThreadKey string
	Count     int
	// LastWrote is when the colleague wrote the newest message of the batch,
	// which is where the provider's reply window starts.
	LastWrote time.Time
	// Attempts is the number of sends started, this one included once claimed.
	Attempts int
}

// heldGroup is the key forwards are batched under.
type heldGroup struct {
	member     ids.UUID
	connection ids.UUID
}

// FormClarifyBatchesTx gives every settled run of unbatched held forwards its
// batch id. A run is consecutive forwards of one colleague on one account, each
// within ClarifyBatchWindow of the one before. It settles when that long has
// passed since its newest. The database clock decides, so no worker's
// clock does. It reports how many batches it formed.
func FormClarifyBatchesTx(ctx context.Context, tx pgx.Tx) (int, error) {
	rows, err := tx.Query(ctx, `
		SELECT r.id, r.held_by, r.held_connection, a.created_at, now()
		  FROM raw_capture r JOIN activity a ON a.raw_capture_id = r.id
		 WHERE r.held_by IS NOT NULL AND r.ask_batch_id IS NULL AND `+pendingforward.Predicate("a")+`
		 ORDER BY r.held_by, r.held_connection NULLS FIRST, a.created_at, r.id
		 LIMIT $1 FOR UPDATE OF r SKIP LOCKED`, clarifyScanLimit)
	if err != nil {
		return 0, fmt.Errorf("capture: reading unbatched held forwards: %w", err)
	}
	type held struct {
		raw        ids.UUID
		group      heldGroup
		created    time.Time
		serverTime time.Time
	}
	all, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (held, error) {
		var h held
		var conn *ids.UUID
		if err := row.Scan(&h.raw, &h.group.member, &conn, &h.created, &h.serverTime); err != nil {
			return h, err
		}
		if conn != nil {
			h.group.connection = *conn
		}
		return h, nil
	})
	if err != nil {
		return 0, fmt.Errorf("capture: reading unbatched held forwards: %w", err)
	}
	formed := 0
	for start := 0; start < len(all); {
		end := start + 1
		for end < len(all) && all[end].group == all[start].group && all[end].created.Sub(all[end-1].created) <= ClarifyBatchWindow {
			end++
		}
		last := all[end-1]
		if last.serverTime.Sub(last.created) > ClarifyBatchWindow {
			members := make([]ids.UUID, 0, end-start)
			for _, h := range all[start:end] {
				members = append(members, h.raw)
			}
			if _, err := tx.Exec(ctx, `UPDATE raw_capture SET ask_batch_id = $1 WHERE id = ANY($2)`, ids.NewV7(), members); err != nil {
				return formed, fmt.Errorf("capture: forming a batch of held forwards: %w", err)
			}
			formed++
		}
		start = end
	}
	return formed, nil
}

// dueBatchSQL selects the batches that still have a held message and have not
// been asked, or whose question is to be retried. $1 is the attempt cap.
func dueBatchSQL() string {
	return `
	SELECT r.ask_batch_id, r.held_by, r.held_connection, a.channel_provider, a.thread_key, count(*),
	       max(a.occurred_at), max(r.ask_attempts)
	  FROM raw_capture r JOIN activity a ON a.raw_capture_id = r.id
	 WHERE r.ask_batch_id IS NOT NULL AND r.ask_sent_at IS NULL AND r.ask_claimed_at IS NULL
	   AND r.ask_attempts < $1 AND (r.ask_next_at IS NULL OR r.ask_next_at <= now()) AND ` + pendingforward.Predicate("a") + `
	 GROUP BY r.ask_batch_id, r.held_by, r.held_connection, a.channel_provider, a.thread_key
	 ORDER BY min(a.created_at)`
}

// DueClarifyBatchesTx lists the batches whose question has not been sent.
func DueClarifyBatchesTx(ctx context.Context, tx pgx.Tx) ([]ClarifyBatch, error) {
	rows, err := tx.Query(ctx, dueBatchSQL(), clarifyMaxAttempts)
	if err != nil {
		return nil, fmt.Errorf("capture: reading the batches to ask about: %w", err)
	}
	return pgx.CollectRows(rows, scanBatch)
}

func scanBatch(row pgx.CollectableRow) (ClarifyBatch, error) {
	var b ClarifyBatch
	var conn *ids.UUID
	var attempts int16
	if err := row.Scan(&b.ID, &b.Member, &conn, &b.Provider, &b.ThreadKey, &b.Count, &b.LastWrote, &attempts); err != nil {
		return b, err
	}
	if conn != nil {
		b.Connection = *conn
	}
	b.Attempts = int(attempts)
	return b, nil
}

// ClaimClarifyBatchTx marks a batch's send as started, and reports whether this
// caller got it. The claim is written before the provider is called. So a send
// that ends in doubt is never made again, and two sweeps never both ask.
func ClaimClarifyBatchTx(ctx context.Context, tx pgx.Tx, batch ids.UUID) (bool, error) {
	tag, err := tx.Exec(ctx, `
		UPDATE raw_capture r SET ask_claimed_at = now(), ask_attempts = ask_attempts + 1
		  FROM activity a
		 WHERE a.raw_capture_id = r.id AND r.ask_batch_id = $1 AND r.ask_sent_at IS NULL AND r.ask_claimed_at IS NULL
		   AND `+pendingforward.Predicate("a"), batch)
	if err != nil {
		return false, fmt.Errorf("capture: claiming a batch to ask about: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// SettleClarifyBatchTx records how a claimed send ended. A message id means the
// provider accepted the question. An empty id with retry true means the provider
// refused without delivering, and the batch is tried again after a pause. An
// empty id with retry false is a refusal no retry can cure, and the batch stays
// held until it expires.
func SettleClarifyBatchTx(ctx context.Context, tx pgx.Tx, batch ids.UUID, messageID string, retry bool) error {
	var err error
	switch {
	case messageID != "":
		_, err = tx.Exec(ctx, `UPDATE raw_capture SET ask_sent_at = now(), ask_message_ids = array_append(ask_message_ids, $2)
			 WHERE ask_batch_id = $1 AND ask_claimed_at IS NOT NULL AND ask_sent_at IS NULL`, batch, messageID)
	case retry:
		_, err = tx.Exec(ctx, `UPDATE raw_capture SET ask_claimed_at = NULL,
			ask_next_at = now() + make_interval(secs => least($2::float8, $3::float8 * power(2, ask_attempts - 1)))
			 WHERE ask_batch_id = $1 AND ask_sent_at IS NULL`, batch, clarifyRetryCap.Seconds(), clarifyRetryBase.Seconds())
	default:
		_, err = tx.Exec(ctx, `UPDATE raw_capture SET ask_claimed_at = NULL, ask_attempts = $2
			 WHERE ask_batch_id = $1 AND ask_sent_at IS NULL`, batch, clarifyMaxAttempts)
	}
	if err != nil {
		return fmt.Errorf("capture: settling a batch's question: %w", err)
	}
	return nil
}
