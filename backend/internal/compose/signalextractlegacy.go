// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The commitment signals written before commitments had a rule, read again
// through it.
//
// Such a signal says "a commitment was made" against an account, with the
// model's summary and nothing a claim needs: not the words, not who made it,
// not to whom. Copying it would file an ungrounded claim, which the claim
// writer refuses by design. So its message is read again — the cited message
// and the five before it — and what the reading finds goes through the rule
// like any new commitment. The signal is then settled in the same transaction,
// whatever the reading found, so each is read once and nothing is left open
// that nobody will look at again.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// commitmentDispatchedKey marks, in a commitment signal's audit image, that its
// commitment went through the rule when it was written. A signal without it is
// one the conversion still owes a reading.
const commitmentDispatchedKey = "commitment_dispatched"

// legacyCommitmentsPerPass bounds how many old signals one pass reads, so the
// conversion spends a small share of a pass's model budget and finishes over
// a few passes rather than starving the conversations that are due now.
const legacyCommitmentsPerPass = 10

// legacyCommitment is one signal still owed a reading.
type legacyCommitment struct {
	signal ids.UUID
	cited  ids.UUID
}

// convertLegacyCommitments reads the oldest commitment signals the rule has
// not seen, and reports how many it settled.
func (x *SignalExtractor) convertLegacyCommitments(ctx context.Context) (int, error) {
	var owed []legacyCommitment
	if err := database.WithWorkspaceTx(ctx, x.pool, func(tx pgx.Tx) error {
		var err error
		owed, err = legacyCommitmentsOwed(ctx, tx)
		return err
	}); err != nil {
		return 0, fmt.Errorf("signal extract: reading the commitment signals owed a reading: %w", err)
	}
	settled := 0
	for _, legacy := range owed {
		if stop, _ := outOfTime(ctx); stop {
			break
		}
		if err := x.convertLegacyCommitment(ctx, legacy); err != nil {
			return settled, err
		}
		settled++
	}
	return settled, nil
}

// convertLegacyCommitment reads one signal's message through the rule and
// settles the signal.
func (x *SignalExtractor) convertLegacyCommitment(ctx context.Context, legacy legacyCommitment) error {
	var thread settledThread
	if err := database.WithWorkspaceTx(ctx, x.pool, func(tx pgx.Tx) error {
		var err error
		thread, err = legacyThread(ctx, tx, legacy)
		return err
	}); err != nil {
		return err
	}
	var events []extractedEvent
	if len(thread.Messages) > 0 {
		var err error
		events, err = x.ask(ctx, thread)
		// A refused reading settles the signal like an empty one: the old card
		// held nothing a reader could act on, and asking again every hour for a
		// message the model will not read would cost more than it could find.
		if err != nil && !errors.Is(err, errRefusedReading) {
			return err
		}
	}
	return database.WithWorkspaceTx(ctx, x.pool, func(tx pgx.Tx) error {
		for _, event := range events {
			if event.Kind != extractKindCommitment || event.MessageID != legacy.cited.String() ||
				event.Confidence < extractConfidenceFloor {
				continue
			}
			if _, err := x.dispatchCommitment(ctx, tx, thread, event, legacy.cited); err != nil {
				return err
			}
		}
		_, err := signals.AcknowledgeTx(ctx, tx, legacy.signal, signals.ResolutionSourceCommitmentRule)
		return err
	})
}

// legacyCommitmentsOwed lists the open commitment signals written without the
// rule, oldest first.
func legacyCommitmentsOwed(ctx context.Context, tx pgx.Tx) ([]legacyCommitment, error) {
	rows, err := tx.Query(ctx, `
		SELECT s.id, cite.activity
		  FROM signal s
		  JOIN LATERAL (
		       SELECT `+storekit.CitedActivityID("item")+` AS activity
		         FROM jsonb_array_elements(s.evidence) item
		        WHERE item->>'source_type' = 'activity'
		        LIMIT 1) cite ON true
		 WHERE s.kind = $1 AND s.status = 'open' AND s.archived_at IS NULL
		   AND s.resolved_company_id IS NOT NULL
		   AND NOT EXISTS (
		         SELECT 1 FROM audit_log al
		          WHERE al.entity_type = 'signal' AND al.entity_id = s.id
		            AND al.after @> jsonb_build_object($2::text, true))
		 ORDER BY s.detected_at, s.id
		 LIMIT $3`, extractKindCommitment, commitmentDispatchedKey, legacyCommitmentsPerPass)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []legacyCommitment
	for rows.Next() {
		var legacy legacyCommitment
		if err := rows.Scan(&legacy.signal, &legacy.cited); err != nil {
			return nil, err
		}
		out = append(out, legacy)
	}
	return out, rows.Err()
}

// legacyThreadRuleQuery asks the extractor's own rule about one conversation
// as it stands NOW: the account it reaches, and whether it is private to a
// member.
// The old signal's account and audience were true when it was written; the
// rule is what decides who may read a summary of the conversation today.
var legacyThreadRuleQuery = conversationCTE(" AND a.thread_key = $1") + `
		SELECT c.one_company::uuid, CASE WHEN c.shared THEN NULL ELSE c.private_owner::uuid END
		  FROM conversation c
		 WHERE ` + threadReachesOneAccount + `
		   AND ` + threadIsOneBodyOfWork + `
		   AND ` + threadIsFullyOpen + `
		   AND ` + threadHasANamedReader

// legacyThread is the cited message and the five before it on its thread. A
// message since archived or held, or a thread the extractor would not read
// today, reads as an empty conversation, which settles the signal with
// nothing filed.
func legacyThread(ctx context.Context, tx pgx.Tx, legacy legacyCommitment) (settledThread, error) {
	var thread settledThread
	var key *string
	var at time.Time
	err := tx.QueryRow(ctx, `
		SELECT thread_key, occurred_at FROM activity
		 WHERE id = $1 AND kind = 'email' AND archived_at IS NULL AND restricted_at IS NULL`,
		legacy.cited).Scan(&key, &at)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && key == nil) {
		return thread, nil
	}
	if err != nil {
		return settledThread{}, fmt.Errorf("signal extract: reading a commitment signal's message: %w", err)
	}
	var privateTo *ids.UUID
	err = tx.QueryRow(ctx, legacyThreadRuleQuery, *key).Scan(&thread.CompanyID, &privateTo)
	if errors.Is(err, pgx.ErrNoRows) {
		return settledThread{}, nil
	}
	if err != nil {
		return settledThread{}, fmt.Errorf("signal extract: asking the rule about a commitment signal's thread: %w", err)
	}
	if privateTo != nil {
		thread.PrivateTo = *privateTo
	}
	thread.Key = *key
	messages, err := threadWindow(ctx, tx, *key, &at, &legacy.cited, &legacy.cited)
	if err != nil {
		return settledThread{}, err
	}
	thread.Messages = messages
	return thread, nil
}
