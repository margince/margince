// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Two threads that turn out to be one conversation become one.
//
// A captured email's thread_key is the FIRST id of its References header, and
// mail programs shorten that header: iPhone Mail sends only the direct parent,
// so every reply it makes starts a new root and one conversation lands under
// several keys. An import states its own key (the importer's thread), which a
// mailbox capturing the same messages never derives. Capture notices when a
// message links two keys (capture/threadjoin.go) and compose merges them
// (compose/threadmerge.go); this file owns the two tables of this module that
// carry a thread key, and the reply links that make the join possible.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// RecordMailReferencesTx stores the Message-IDs one email says it answers.
//
// Idempotent: a re-sync of the same message states the same links. The ids are
// the sender's text; nothing here trusts them — MailNeighboursTx only reports
// candidates, and capture acts on one only when the same seat holds both ends.
func RecordMailReferencesTx(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, referenced []string) error {
	if len(referenced) == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO activity_mail_reference (activity_id, referenced_id)
		SELECT $1, r FROM unnest($2::text[]) AS r
		ON CONFLICT (activity_id, referenced_id) DO NOTHING`,
		activityID, referenced); err != nil {
		return fmt.Errorf("activities: recording what a message replies to: %w", err)
	}
	return nil
}

// MailNeighboursTx answers the live messages directly linked to one email by a
// reply: the ones it names (its In-Reply-To and References, resolved through
// activity_identity, which holds every door's Message-ID) and the ones that
// name it.
//
// Both directions, because messages arrive in any order. A backfill walks a
// mailbox newest first, so the reply is routinely stored before the message it
// answers, and only the reverse read can find it.
func MailNeighboursTx(
	ctx context.Context, tx pgx.Tx, self ids.ActivityID, messageID string, referenced []string,
) ([]ids.ActivityID, error) {
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT a.id
		  FROM activity a
		 WHERE a.id <> $1
		   AND a.archived_at IS NULL
		   AND a.thread_key IS NOT NULL
		   AND (a.id IN (SELECT activity_id FROM activity_identity
		                  WHERE identity_kind = $4 AND identity_key = ANY($3::text[]))
		        -- A message sent from here is filed under its minted Message-ID
		        -- as its natural key and claims no identity row, so a reply to
		        -- it is found by that key.
		        OR (a.source_system = $5 AND a.source_id = ANY($3::text[]))
		        OR ($2 <> '' AND a.id IN (SELECT activity_id FROM activity_mail_reference
		                                   WHERE referenced_id = $2)))`,
		self, messageID, referenced, IdentityKindMail, connector.EmailSourceSystem)
	if err != nil {
		return nil, fmt.Errorf("activities: reading the messages a reply links: %w", err)
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (ids.ActivityID, error) {
		var id ids.ActivityID
		err := r.Scan(&id)
		return id, err
	})
}

// EarliestThreadKeyTx orders candidate thread keys by the earliest live message
// each holds, and answers the first; ties go to the smaller key so two
// concurrent joins agree.
//
// The earliest message is the conversation's opener as far as this workspace
// knows it, which is the root a capture with an unshortened References chain
// would have chosen — so the surviving key is the one later mail most likely
// derives by itself.
func EarliestThreadKeyTx(ctx context.Context, tx pgx.Tx, keys []string) (string, error) {
	var key string
	err := tx.QueryRow(ctx, `
		SELECT thread_key
		  FROM activity
		 WHERE thread_key = ANY($1::text[]) AND archived_at IS NULL
		 GROUP BY thread_key
		 ORDER BY min(occurred_at), thread_key
		 LIMIT 1`, keys).Scan(&key)
	if err != nil {
		return "", fmt.Errorf("activities: choosing the thread two keys merge into: %w", err)
	}
	return key, nil
}

// ThreadMergeTx moves every message of thread `from` into thread `to`, and the
// thread-scoped "not sales" judgement with it.
//
// The judgement survives only where BOTH threads carried it. It hides a thread
// from the worklist, so carrying one half's judgement onto the other half would
// take messages nobody judged off the list without a word. Dropping it puts
// them back, where a rep sees them and can judge again.
//
// Archived rows move too: a thread key is a grouping, and an erased or
// restricted message still belongs to the conversation it was in.
func ThreadMergeTx(ctx context.Context, tx pgx.Tx, from, to string) error {
	if from == "" || to == "" || from == to {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		UPDATE activity SET thread_key = $2
		 WHERE thread_key = $1`, from, to); err != nil {
		return fmt.Errorf("activities: moving a thread's messages into the one it joins: %w", err)
	}
	// Both halves' judgements, kept only where they agree.
	if _, err := tx.Exec(ctx, `
		DELETE FROM activity_sales_state s
		 WHERE s.thread_key IN ($1, $2)
		   AND NOT EXISTS (SELECT 1 FROM activity_sales_state o
		                    WHERE o.thread_key = CASE WHEN s.thread_key = $1 THEN $2 ELSE $1 END
		                      AND o.kind = s.kind AND o.channel_provider = s.channel_provider)`,
		from, to); err != nil {
		return fmt.Errorf("activities: reconciling the merged threads' judgements: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM activity_sales_state WHERE thread_key = $1`, from); err != nil {
		return fmt.Errorf("activities: retiring the merged thread's judgements: %w", err)
	}
	return nil
}
