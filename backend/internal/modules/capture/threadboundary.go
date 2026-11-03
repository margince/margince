// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/mailsubject"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// subjectThreadKeyTx is the thread an email files under. A reply whose topic differs
// from the message it answers opens its own conversation, keyed by its Message-ID as a
// fresh opener would be; any other reply joins the conversation its parent is in.
func (s *Sink) subjectThreadKeyTx(ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord, subject string) (string, error) {
	seat := actorUserID(ctx)
	if !s.threadJoin.complete() || rec.NaturalKey.SourceSystem != connector.EmailSourceSystem ||
		rec.ThreadKey == "" || len(rec.ReplyTo) == 0 || seat == ids.Nil {
		return rec.ThreadKey, nil
	}
	candidates, err := s.threadJoin.Neighbours(ctx, tx, ids.ActivityID{}, "", rec.ReplyTo[:1])
	if err != nil || len(candidates) == 0 {
		return rec.ThreadKey, err
	}
	var sent []ids.ActivityID
	if s.threadJoin.SentBySeat != nil {
		if sent, err = s.threadJoin.SentBySeat(ctx, tx, seat, candidates); err != nil {
			return "", err
		}
	}
	// The reply headers are the sender's text, so only a parent this seat holds or
	// sent decides; a stranger naming someone else's message steers nothing.
	var parentKey, parentSubject string
	err = tx.QueryRow(ctx, `
		SELECT a.thread_key, coalesce(a.subject, '')
		  FROM activity a
		 WHERE a.id = ANY($1::uuid[]) AND a.archived_at IS NULL AND a.thread_key <> ''
		   AND (a.id = ANY($2::uuid[]) OR EXISTS (SELECT 1 FROM capture_import ci
		                WHERE ci.activity_id = a.id AND ci.user_id = $3))
		 ORDER BY a.occurred_at DESC, a.id
		 LIMIT 1`, candidates, sent, seat).Scan(&parentKey, &parentSubject)
	if errors.Is(err, pgx.ErrNoRows) {
		return rec.ThreadKey, nil
	}
	if err != nil {
		return "", fmt.Errorf("capture: reading the message a reply answers: %w", err)
	}
	if !topicsDiffer(subject, parentSubject) {
		return parentKey, nil
	}
	own := strings.TrimSuffix(strings.TrimPrefix(rec.NaturalKey.SourceID, "<"), ">")
	// The sender chose that Message-ID, so a key some other message already carries
	// would file this reply into a conversation it never belonged to.
	var taken bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM activity
		                WHERE thread_key = $1 AND NOT (source_system = $2 AND source_id = $3))`,
		own, connector.EmailSourceSystem, rec.NaturalKey.SourceID).Scan(&taken); err != nil {
		return "", fmt.Errorf("capture: reading whether a split conversation's key is free: %w", err)
	}
	if taken {
		return parentKey, nil
	}
	return own, carryThreadVerdictsTx(ctx, tx, parentKey, own)
}

// carryThreadVerdictsTx gives a conversation split off another every seat's verdict on
// it, before the reply's audience is decided, so a held exchange stays held.
func carryThreadVerdictsTx(ctx context.Context, tx pgx.Tx, from, to string) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO capture_thread_verdict
		       (thread_key, user_id, status, kind, confidence, disposition_reason, seen_addresses,
		        first_activity_id, resolved_at, next_attempt_at)
		SELECT $2, user_id, status, kind, confidence, disposition_reason, seen_addresses,
		       first_activity_id, resolved_at, next_attempt_at
		  FROM capture_thread_verdict WHERE thread_key = $1
		    ON CONFLICT (thread_key, user_id) DO NOTHING`, from, to); err != nil {
		return fmt.Errorf("capture: carrying thread verdicts to a split conversation: %w", err)
	}
	return nil
}

// topicsDiffer compares subjects as mailsubject.Normalized does. A blank topic, such as
// a bare "Re:", carries no claim about the conversation and never draws a boundary.
func topicsDiffer(subject, other string) bool {
	topic, otherTopic := mailsubject.Normalized(subject), mailsubject.Normalized(other)
	return topic != "" && otherTopic != "" && topic != otherTopic
}

// sameTopicSQL is topicsDiffer negated, over two activity aliases, for the thread join.
func sameTopicSQL(alias, other string) string {
	topic, otherTopic := storekit.NormalisedSubjectSQL(alias+".subject"), storekit.NormalisedSubjectSQL(other+".subject")
	return "(" + topic + " = '' OR " + otherTopic + " IN ('', " + topic + "))"
}
