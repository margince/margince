// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// OwnSentMessage is one mail the signed-in user sent from a mailbox they
// connected. CapturedBy names the connector that stored it, so the caller can
// still prove the mailbox is the seat's own and not one it shares.
type OwnSentMessage struct {
	ActivityID ids.ActivityID
	Subject    string
	Body       string
	CapturedBy string
	SentAt     time.Time
}

// voiceEligibleSQL is the state a sent mail must be in for its words to feed
// a voice. It is attested own outbound mail, open to the workspace, with a
// body, neither archived nor restricted. The reader and the recheck both ask it.
func voiceEligibleSQL(alias string) string {
	return sprintf(`%[1]s.kind = 'email' AND %[2]s
	   AND %[1]s.counterparty_outbound_attested AND %[1]s.sent_by_colleague IS NULL
	   AND %[1]s.archived_at IS NULL AND %[1]s.restricted_at IS NULL AND %[1]s.body IS NOT NULL
	   AND %[1]s.audience = 'workspace'`, alias, relstrength.WeWroteSQL(alias))
}

// OwnSentMailTx reads the newest limit mails the signed-in human sent
// themselves, share-locked until the caller's transaction ends. It takes no
// user argument, so no caller can ask for another seat's mail. A mail counts
// only when voiceEligibleSQL holds and it names the seat as its sender. The
// lock holds a narrowing back until a caller filing these mails commits.
func (s *Store) OwnSentMailTx(ctx context.Context, tx pgx.Tx, limit int) ([]OwnSentMessage, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman || actor.UserID.IsZero() {
		return nil, apperrors.ErrPermissionDenied
	}
	if limit <= 0 {
		return nil, nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	seat := arg(actor.UserID)
	readable, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	bound := arg(limit)
	query := sprintf(`
		SELECT a.id, coalesce(a.subject, ''), a.body, a.captured_by, a.occurred_at
		  FROM activity a
		 WHERE EXISTS (SELECT 1 FROM activity_participant p
		                WHERE p.activity_id = a.id AND p.role = 'from' AND p.user_id = $%d AND %s)
		   AND %s AND (%s)
		 ORDER BY a.occurred_at DESC, a.id DESC
		 LIMIT $%d
		 FOR SHARE OF a`, seat, auth.ParticipantStandsSQL("p", "a"), voiceEligibleSQL("a"), readable, bound)
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("activities: reading the signed-in user's own sent mail: %w", err)
	}
	var sent []OwnSentMessage
	var mail OwnSentMessage
	_, err = pgx.ForEachRow(rows, []any{&mail.ActivityID, &mail.Subject, &mail.Body, &mail.CapturedBy, &mail.SentAt}, func() error {
		sent = append(sent, mail)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("activities: reading the signed-in user's own sent mail: %w", err)
	}
	return sent, nil
}

// VoiceEligibleTx reports, under a share lock, whether a mail may still feed
// a voice, and its subject for the caller's confidentiality check. A mail that
// no longer exists answers not eligible.
func VoiceEligibleTx(ctx context.Context, tx pgx.Tx, activityID ids.UUID) (eligible bool, subject string, err error) {
	err = tx.QueryRow(ctx, sprintf(`
		SELECT %s, coalesce(a.subject, '') FROM activity a WHERE a.id = $1 FOR SHARE OF a`,
		voiceEligibleSQL("a")), activityID).Scan(&eligible, &subject)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, "", nil
	}
	if err != nil {
		return false, "", fmt.Errorf("activities: reading whether %s may feed a voice: %w", activityID, err)
	}
	return eligible, subject, nil
}
