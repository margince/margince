// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/mailsubject"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// readMailForTheSeatTx settles what the seat's own addresses decide, then the thread the
// subject files the mail under, so every later step reads the same record.
func (s *Sink) readMailForTheSeatTx(
	ctx context.Context, tx pgx.Tx, seat ids.UUID, rec connector.NormalizedRecord, fields ActivityFields,
) (connector.NormalizedRecord, ActivityFields, error) {
	rec, fields, err := s.readAgainstTheSeatsAddressesTx(ctx, tx, seat, rec, fields)
	if err != nil {
		return rec, fields, err
	}
	rec.ThreadKey, err = s.subjectThreadKeyTx(ctx, tx, rec, fields.Subject)
	return rec, fields, err
}

// subjectThreadKeyTx is the thread a new email files under. A reply whose topic differs
// from its parent's opens a conversation keyed by its own Message-ID. Any other reply
// joins its parent's conversation. A message already stored keeps the key it has.
func (s *Sink) subjectThreadKeyTx(ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord, subject string) (string, error) {
	seat := actorUserID(ctx)
	if !s.threadJoin.complete() || rec.NaturalKey.SourceSystem != connector.EmailSourceSystem ||
		rec.ThreadKey == "" || len(rec.ReplyTo) == 0 || seat == ids.Nil {
		return rec.ThreadKey, nil
	}
	own := strings.TrimSuffix(strings.TrimPrefix(rec.NaturalKey.SourceID, "<"), ">")
	var stored, taken bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM activity WHERE source_system = $1 AND source_id = $2)
		    OR EXISTS (SELECT 1 FROM activity_identity WHERE identity_kind = $3 AND identity_key = $4),
		       EXISTS (SELECT 1 FROM activity WHERE thread_key = $4)`,
		connector.EmailSourceSystem, rec.NaturalKey.SourceID, s.mailIdentityKind, own).Scan(&stored, &taken); err != nil {
		return "", fmt.Errorf("capture: reading whether a reply is already stored: %w", err)
	}
	if stored {
		return rec.ThreadKey, nil
	}
	parentKey, differs, found, err := s.heldParentTx(ctx, tx, seat, rec.ReplyTo[0], threadTopic(subject))
	if err != nil || !found {
		return rec.ThreadKey, err
	}
	// The sender chose that Message-ID, so a key another message already carries
	// would file this reply into a conversation it never belonged to.
	if !differs || taken {
		return parentKey, nil
	}
	return own, carryThreadVerdictsTx(ctx, tx, parentKey, own)
}

// heldParentTx answers the conversation of the message a reply names as its parent, and
// whether their topics differ. The header is the sender's text, so only a parent this
// seat holds or sent counts. Only the key and a boolean leave the query.
func (s *Sink) heldParentTx(
	ctx context.Context, tx pgx.Tx, seat ids.UUID, parentID, topic string,
) (key string, differs, found bool, err error) {
	candidates, err := s.threadJoin.Neighbours(ctx, tx, ids.ActivityID{}, "", []string{parentID})
	if err != nil || len(candidates) == 0 {
		return "", false, false, err
	}
	var sent []ids.ActivityID
	if s.threadJoin.SentBySeat != nil {
		if sent, err = s.threadJoin.SentBySeat(ctx, tx, seat, candidates); err != nil {
			return "", false, false, err
		}
	}
	parentTopic := threadTopicSQL("a.subject")
	err = tx.QueryRow(ctx, `
		SELECT a.thread_key, `+parentTopic+` <> '' AND $4 <> '' AND `+parentTopic+` <> $4
		  FROM activity a
		 WHERE a.id = ANY($1::uuid[]) AND a.archived_at IS NULL AND a.thread_key <> ''
		   AND (a.id = ANY($2::uuid[]) OR EXISTS (SELECT 1 FROM capture_import ci
		                WHERE ci.activity_id = a.id AND ci.user_id = $3))
		 ORDER BY a.occurred_at DESC, a.id
		 LIMIT 1`, candidates, sent, seat, topic).Scan(&key, &differs)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, false, nil
	}
	if err != nil {
		return "", false, false, fmt.Errorf("capture: reading the message a reply answers: %w", err)
	}
	return key, differs, true, nil
}

// carryThreadVerdictsTx gives a split conversation every seat's settled verdict on the
// one it left, before the reply's audience is decided. A pending question is not
// carried: it names the old conversation's message, and the new one opens its own.
func carryThreadVerdictsTx(ctx context.Context, tx pgx.Tx, from, to string) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO capture_thread_verdict
		       (thread_key, user_id, status, kind, confidence, disposition_reason, seen_addresses,
		        first_activity_id, resolved_at)
		SELECT $2, user_id, status, kind, confidence, disposition_reason, seen_addresses,
		       first_activity_id, resolved_at
		  FROM capture_thread_verdict WHERE thread_key = $1 AND status <> 'pending'
		    ON CONFLICT (thread_key, user_id) DO NOTHING`, from, to); err != nil {
		return fmt.Errorf("capture: carrying thread verdicts to a split conversation: %w", err)
	}
	return nil
}

// sameTopicSQL is true unless both activities have a topic and the two differ. The
// thread join reads it to keep a split apart.
func sameTopicSQL(alias, other string) string {
	topic, otherTopic := threadTopicSQL(alias+".subject"), threadTopicSQL(other+".subject")
	return "(" + topic + " = '' OR " + otherTopic + " IN ('', " + topic + "))"
}

// projectKeyTag is a bracketed project key in a normalised subject, the shape
// projectKeyShaped admits. A reply that keeps or drops one is the same topic.
const projectKeyTag = `\[[a-z][a-z0-9_-]{1,23}\]`

var projectKeyTags = regexp.MustCompile(projectKeyTag)

// threadTopic is a subject as a conversation boundary compares it: normalised as
// the answer check reads it, without its project-key tags. threadTopicSQL spells
// the same steps.
func threadTopic(subject string) string {
	return strings.Join(strings.Fields(projectKeyTags.ReplaceAllString(mailsubject.Normalized(subject), " ")), " ")
}

// threadTopicSQL is threadTopic over a subject column.
func threadTopicSQL(subject string) string {
	return `btrim(regexp_replace(regexp_replace(` + storekit.NormalisedSubjectSQL(subject) +
		`, '` + projectKeyTag + `', ' ', 'g'), ' +', ' ', 'g'))`
}
