// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The unit a classified chat is judged in. A chat is one long thread per
// customer, so judging the thread once would let one ordinary opening clear
// everything said later. The unit is a BURST: consecutive messages of one chat
// thread whose gaps stay under ChatBurstGap. One model call judges one burst.
// The verdict covers the messages it read. The next burst is born held until
// it has a verdict of its own.
//
// The ledger is capture_thread_verdict as for mail. A burst's question sits
// under the burst id written as text, which no mail thread key can equal. The
// mail rows, their unique key and every reader keyed on a thread are left
// alone. capture_import.burst_id names the burst of one seat's import of one
// message. threadverdict.LedgerKeySQL turns it into the ledger key the
// audience derivation joins on.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/threadverdict"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

const (
	// ChatBurstGap is the longest silence inside one burst: a message that
	// comes less than this after the one before it joins its burst.
	ChatBurstGap = 15 * time.Minute

	// chatBurstMaxMessages bounds a burst, so a conversation that never pauses
	// is still judged in pieces a model call can hold.
	chatBurstMaxMessages = 40
	// chatBurstMessageChars and chatBurstTranscriptChars bound what the model
	// reads. A message that does not fit the transcript is not judged and moves
	// to the next burst (see recordOutcomeOnBurstTx).
	chatBurstMessageChars    = 600
	chatBurstTranscriptChars = 8000

	// chatBurstSubject stands where a mail thread's subject does in the
	// classifier's input.
	chatBurstSubject = "Chat conversation: consecutive messages of one chat"

	instantPrecision = "instant"
)

// burstOfKey returns the burst a ledger key names.
func burstOfKey(key string) (ids.UUID, bool) {
	text, ok := threadverdict.BurstOfKey(key)
	if !ok {
		return ids.Nil, false
	}
	burst, err := ids.Parse(text)
	return burst, err == nil
}

// IsBurst says the question is about a chat burst and names its messages.
func (t PendingThread) IsBurst() bool { return len(t.BurstMembers) > 0 }

// LockKeys are the thread keys a write on this question serializes on. A mail
// question has its thread. A burst has its own key and its chat thread.
func (t PendingThread) LockKeys() []string {
	if t.ChatThread != "" {
		return []string{t.ThreadKey, t.ChatThread}
	}
	return []string{t.ThreadKey}
}

// openChatBurstQuestionTx puts a classified chat message into its burst and
// opens that burst's question. A seat that is not classified has no question:
// a team seat is shared or held and a held member seat stays held.
func openChatBurstQuestionTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID, owner ids.UUID, birth birthDecision) error {
	if birth.posture != PostureClassified || owner.IsZero() {
		return nil
	}
	burst, err := assignChatBurstTx(ctx, tx, id, owner)
	if err != nil || burst.IsZero() {
		return err
	}
	return openBurstQuestionTx(ctx, tx, owner, burst, id.UUID)
}

// openBurstQuestionTx opens the question of one burst and leaves an existing
// one as it is.
func openBurstQuestionTx(ctx context.Context, tx pgx.Tx, user, burst, first ids.UUID) error {
	key := threadverdict.BurstKey(burst.String())
	if err := LockThreadHoldTx(ctx, tx, key); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO capture_thread_verdict (thread_key, user_id, first_activity_id, status, next_attempt_at)
		VALUES ($1, $2, $3, 'pending', now())
		ON CONFLICT (thread_key, user_id) DO NOTHING`,
		key, user, first); err != nil {
		return fmt.Errorf("capture: opening the confidentiality question for a chat burst: %w", err)
	}
	return nil
}

// assignChatBurstTx names the burst of one chat message, and returns Nil when
// the seat has no import row of it. A message that already has a burst keeps
// it, so a redelivery moves nothing.
//
// A message with an instant time joins the burst of the newest earlier message
// of its thread. That needs a gap under ChatBurstGap, a burst not yet judged
// and room in it. Anything else starts a burst. A judged burst is closed: its
// verdict covers the messages it read, so a later message must not join it
// unread.
//
// A message with a day or unknown time cannot be placed by a gap. The messages
// of one operation on one thread form one burst.
func assignChatBurstTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID, owner ids.UUID) (ids.UUID, error) {
	var current *ids.UUID
	var thread string
	var at time.Time
	var precision string
	err := tx.QueryRow(ctx, `
		SELECT i.burst_id, a.thread_key, a.occurred_at, a.occurred_precision
		  FROM capture_import i JOIN activity a ON a.id = i.activity_id
		 WHERE i.activity_id = $1 AND i.user_id = $2 AND a.restricted_at IS NULL`, id, owner).Scan(&current, &thread, &at, &precision)
	if err == pgx.ErrNoRows {
		return ids.Nil, nil
	}
	if err != nil {
		return ids.Nil, fmt.Errorf("capture: reading the burst of a chat message: %w", err)
	}
	if current != nil {
		return *current, nil
	}
	// Two messages of one thread are placed one after the other, or the second
	// would not see the burst the first started.
	if err := LockThreadHoldTx(ctx, tx, thread); err != nil {
		return ids.Nil, err
	}
	burst := id.UUID
	if precision == instantPrecision && datasource.InstantInRange(at) {
		if burst, err = instantBurstTx(ctx, tx, id, owner, thread, at); err != nil {
			return ids.Nil, err
		}
	} else if op, ok := principal.CorrelationID(ctx); ok {
		if err := tx.QueryRow(ctx, `SELECT md5($1 || '|' || $2 || '|' || $3)::uuid`,
			owner.String(), thread, op.String()).Scan(&burst); err != nil {
			return ids.Nil, fmt.Errorf("capture: naming the burst of an undated chat message: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE capture_import SET burst_id = $3 WHERE activity_id = $1 AND user_id = $2 AND burst_id IS NULL`,
		id, owner, burst); err != nil {
		return ids.Nil, fmt.Errorf("capture: placing a chat message in its burst: %w", err)
	}
	return burst, nil
}

// instantBurstTx answers the burst an instant message joins, or its own id when
// it starts one.
func instantBurstTx(
	ctx context.Context, tx pgx.Tx, id ids.ActivityID, owner ids.UUID, thread string, at time.Time,
) (ids.UUID, error) {
	var previous ids.UUID
	var previousAt time.Time
	var previousPrecision, status string
	var size int
	err := tx.QueryRow(ctx, `
		SELECT p.burst_id, pa.occurred_at, pa.occurred_precision, coalesce(v.status, 'pending'),
		       (SELECT count(*) FROM capture_import c WHERE c.user_id = p.user_id AND c.burst_id = p.burst_id)
		  FROM capture_import p
		  JOIN activity pa ON pa.id = p.activity_id
		  LEFT JOIN capture_thread_verdict v ON v.user_id = p.user_id AND v.thread_key = `+threadverdict.BurstKeySQL("p.burst_id")+`
		 WHERE p.user_id = $1 AND p.burst_id IS NOT NULL AND pa.thread_key = $2
		   AND p.activity_id <> $3 AND pa.restricted_at IS NULL
		   AND pa.occurred_at <= $4 -- occurred_at raw: the keyset bound of the message before this one
		 ORDER BY pa.occurred_at DESC, pa.id DESC
		 LIMIT 1`, owner, thread, id, at).Scan(&previous, &previousAt, &previousPrecision, &status, &size)
	if err == pgx.ErrNoRows {
		return id.UUID, nil
	}
	if err != nil {
		return ids.Nil, fmt.Errorf("capture: reading the chat message before this one: %w", err)
	}
	joins := previousPrecision == instantPrecision && at.Sub(previousAt) < ChatBurstGap &&
		status == VerdictPending && size < chatBurstMaxMessages
	if !joins {
		return id.UUID, nil
	}
	return previous, nil
}

// claimBurstTx reads the messages of a claimed chat burst question. It fills
// the row with what the model is to read. It also answers the
// wait that is left while the burst is still being written to. A verdict on
// half a conversation would close it. The rest would then be judged without
// what came before. A question that is not about a burst is returned
// as it was.
func claimBurstTx(ctx context.Context, tx pgx.Tx, row PendingThread) (PendingThread, time.Duration, error) {
	burst, ok := burstOfKey(row.ThreadKey)
	if !ok {
		return row, 0, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT a.id, a.thread_key, coalesce(a.direction, ''), a.occurred_at, a.occurred_precision,
		       left(coalesce(a.body, ''), $3), extract(epoch FROM now() - a.occurred_at)
		  FROM capture_import i JOIN activity a ON a.id = i.activity_id
		 WHERE i.user_id = $1 AND i.burst_id = $2 AND a.archived_at IS NULL AND a.restricted_at IS NULL
		 ORDER BY a.occurred_at, a.id
		 LIMIT $4`, row.UserID, burst, chatBurstMessageChars, chatBurstMaxMessages)
	if err != nil {
		return row, 0, fmt.Errorf("capture: reading the messages of a chat burst: %w", err)
	}
	defer rows.Close()
	var transcript strings.Builder
	var members []ids.UUID
	var quietIn time.Duration
	for rows.Next() {
		var member ids.UUID
		var thread, direction, body, precision string
		var at time.Time
		var ageSeconds float64
		if err := rows.Scan(&member, &thread, &direction, &at, &precision, &body, &ageSeconds); err != nil {
			return row, 0, fmt.Errorf("capture: reading a message of a chat burst: %w", err)
		}
		line := fmt.Sprintf("[%s] %s: %s\n", at.UTC().Format("2006-01-02 15:04"), chatSpeaker(direction), body)
		if len(members) > 0 && transcript.Len()+len(line) > chatBurstTranscriptChars {
			break
		}
		transcript.WriteString(line)
		members = append(members, member)
		row.ChatThread = thread
		if precision == instantPrecision {
			quietIn = max(quietIn, ChatBurstGap-time.Duration(ageSeconds*float64(time.Second)))
		}
	}
	if err := rows.Err(); err != nil {
		return row, 0, fmt.Errorf("capture: reading the messages of a chat burst: %w", err)
	}
	if len(members) == 0 {
		return row, 0, nil
	}
	row.BurstMembers = members
	row.Subject = chatBurstSubject
	row.Body = transcript.String()
	return row, quietIn, nil
}

// chatSpeaker names who spoke, by the side of the connection the message came
// from.
func chatSpeaker(direction string) string {
	if direction == "outbound" {
		return "Member"
	}
	return "Customer"
}

// recordOutcomeOnBurstTx stamps a burst's answer on the messages the model
// read, and moves the burst's other messages into a burst of their own. Those
// arrived after the claim or did not fit the transcript, so the answer says
// nothing about them.
func (s *ThreadVerdictStore) recordOutcomeOnBurstTx(
	ctx context.Context, tx pgx.Tx, row PendingThread, status, kind string,
) (ThreadOutcome, error) {
	burst, ok := burstOfKey(row.ThreadKey)
	if !ok {
		return ThreadOutcome{}, fmt.Errorf("capture: a chat burst question is keyed by %q", row.ThreadKey)
	}
	read, err := stillInBurstTx(ctx, tx, row.UserID, burst, row.BurstMembers)
	if err != nil {
		return ThreadOutcome{}, err
	}
	if err := stampSiblingsTx(ctx, tx, row.UserID, read, status, kind, siblingStatusesTaken(status)); err != nil {
		return ThreadOutcome{}, err
	}
	out := ThreadOutcome{Stamped: read}
	rows, err := tx.Query(ctx, `
		SELECT i.activity_id
		  FROM capture_import i JOIN activity a ON a.id = i.activity_id
		 WHERE i.user_id = $1 AND i.burst_id = $2 AND NOT (i.activity_id = ANY($3)) AND a.restricted_at IS NULL
		 ORDER BY a.occurred_at, a.id`, row.UserID, burst, row.BurstMembers)
	if err != nil {
		return ThreadOutcome{}, fmt.Errorf("capture: reading the messages a burst verdict did not read: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var unread ids.UUID
		if err := rows.Scan(&unread); err != nil {
			return ThreadOutcome{}, fmt.Errorf("capture: reading a message a burst verdict did not read: %w", err)
		}
		out.Moved = append(out.Moved, unread)
	}
	if err := rows.Err(); err != nil || len(out.Moved) == 0 {
		return out, err
	}
	rows.Close()
	next := ids.NewV7()
	if _, err := tx.Exec(ctx, `
		UPDATE capture_import SET burst_id = $3 WHERE user_id = $1 AND activity_id = ANY($2)`,
		row.UserID, out.Moved, next); err != nil {
		return ThreadOutcome{}, fmt.Errorf("capture: moving unread chat messages to their own burst: %w", err)
	}
	return out, openBurstQuestionTx(ctx, tx, row.UserID, next, out.Moved[0])
}

// readBursts fills in the chat bursts among the claimed questions. A burst that
// is still being written to goes back to the queue. It is due once it has been
// quiet for ChatBurstGap, and the attempt it was charged is refunded.
func (s *ThreadVerdictStore) readBursts(ctx context.Context, claimed []PendingThread) ([]PendingThread, error) {
	var ready []PendingThread
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		ready = ready[:0]
		for _, row := range claimed {
			row, wait, err := claimBurstTx(ctx, tx, row)
			if err != nil {
				return err
			}
			if wait <= 0 {
				ready = append(ready, row)
				continue
			}
			tag, err := tx.Exec(ctx, `
				UPDATE capture_thread_verdict
				   SET attempts = greatest(attempts - 1, 0), next_attempt_at = now() + make_interval(secs => $2),
				       claimed_until = NULL, claimed_by = NULL, updated_at = now()
				 WHERE id = $1 AND claimed_by = $3`, row.ID, wait.Seconds(), row.Claim)
			if err != nil {
				return err
			}
			if tag.RowsAffected() != 1 {
				// The lease expired and another worker holds the question now.
				continue
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("capture: reading the chat bursts awaiting a confidentiality verdict: %w", err)
	}
	return ready, nil
}

// stillInBurstTx keeps the claimed messages that are still in the burst. An
// edit that landed after the claim moved its message out, and the answer says
// nothing about the new text.
func stillInBurstTx(ctx context.Context, tx pgx.Tx, user, burst ids.UUID, members []ids.UUID) ([]ids.UUID, error) {
	var kept []ids.UUID
	rows, err := tx.Query(ctx, `
		SELECT activity_id FROM capture_import WHERE user_id = $1 AND burst_id = $2 AND activity_id = ANY($3)`,
		user, burst, members)
	if err != nil {
		return nil, fmt.Errorf("capture: reading which claimed chat messages are still in their burst: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id ids.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("capture: reading a claimed chat message: %w", err)
		}
		kept = append(kept, id)
	}
	return kept, rows.Err()
}

// reopenEditedChatTx takes an edited chat message out of a burst whose verdict
// was about the old text. That is a burst already cleared, or one the engine
// claimed and is reading. The message goes into a burst of its own, born held
// and with its own question, and its audience is derived again.
func (s *Sink) reopenEditedChatTx(ctx context.Context, tx pgx.Tx, key connector.NaturalKey, user ids.UUID) error {
	var activity, burst ids.UUID
	var thread string
	err := tx.QueryRow(ctx, `
		SELECT a.id, i.burst_id, a.thread_key
		  FROM activity a
		  JOIN capture_import i ON i.activity_id = a.id AND i.user_id = $3 AND i.burst_id IS NOT NULL
		  JOIN capture_thread_verdict v ON v.user_id = i.user_id AND v.thread_key = `+threadverdict.BurstKeySQL("i.burst_id")+`
		 WHERE a.source_system = $1 AND a.source_id = $2 AND a.restricted_at IS NULL
		   AND (v.status = $4 OR (v.status = $5 AND v.claimed_by IS NOT NULL))`,
		key.SourceSystem, key.SourceID, user, VerdictCleared, VerdictPending).Scan(&activity, &burst, &thread)
	if err == pgx.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("capture: reading the burst of an edited chat message: %w", err)
	}
	if err := LockThreadHoldTx(ctx, tx, thread); err != nil {
		return err
	}
	moved := ids.NewV7()
	if _, err := tx.Exec(ctx, `
		UPDATE capture_import SET burst_id = $3, verdict_status = NULL, verdict_reason = NULL
		 WHERE activity_id = $1 AND user_id = $2`, activity, user, moved); err != nil {
		return fmt.Errorf("capture: moving an edited chat message to a burst of its own: %w", err)
	}
	if err := openBurstQuestionTx(ctx, tx, user, moved, activity); err != nil {
		return err
	}
	if s.recomputeAudience == nil {
		return nil
	}
	return s.recomputeAudience(ctx, tx, ids.From[ids.ActivityKind](activity))
}
