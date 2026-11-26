// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Margince's own "Which deal is this about?" question. When several open deals
// fit a mail, it suggests each of them in the link suggestion queue. It asks
// the one colleague the mailbox confirms is responsible. The row names that
// colleague (for_user), and the queue hides it from everyone else. Filing the mail on a
// deal answers the question for its whole conversation.

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// WhichDealQuestion asks Seat which of Deals the mail is about. Reason is the
// question in the seat's language, shown in the queue.
type WhichDealQuestion struct {
	Mail   ids.ActivityID
	Seat   ids.UUID
	Deals  []ids.UUID
	Reason string
}

// AskWhichDealTx suggests each deal to the seat, at most once per conversation
// and seat. Any earlier question of the seat in the conversation counts, a
// decided one too. So a second mail adds none and an answered one stays
// answered. The seat keys the question with the thread: the thread key is the
// sender's References root, and a spoofed root cannot use up another seat's.
func AskWhichDealTx(ctx context.Context, tx pgx.Tx, q WhichDealQuestion) error {
	if len(q.Deals) == 0 {
		return nil
	}
	// The lock holds the read and the proposals as one step, so two mails of
	// one conversation captured at once ask the seat once.
	var asked bool
	err := tx.QueryRow(ctx, `
		SELECT pg_advisory_xact_lock(hashtextextended('margince:which-deal:' || $2::text || ':'
		       || coalesce(nullif(mail.thread_key, ''), mail.id::text), 0)) IS NULL
		  FROM activity mail WHERE mail.id = $1`, q.Mail, q.Seat).Scan(&asked)
	if err != nil {
		return fmt.Errorf("activities: holding the deal question of a conversation: %w", err)
	}
	err = tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM activity_link_suggestion s
		  JOIN activity asked ON asked.id = s.activity_id
		  JOIN activity mail ON mail.id = $1
		 WHERE s.for_user = $2
		   AND coalesce(nullif(asked.thread_key, ''), asked.id::text) = coalesce(nullif(mail.thread_key, ''), mail.id::text))`,
		q.Mail, q.Seat).Scan(&asked)
	if err != nil {
		return fmt.Errorf("activities: reading whether the deal question was asked: %w", err)
	}
	if asked {
		return nil
	}
	score := math.Round(10000/float64(len(q.Deals))) / 10000
	for _, deal := range q.Deals {
		_, err := ProposeLinkSuggestionTx(ctx, tx, LinkSuggestionInput{
			ActivityID: q.Mail, EntityType: linkEntityDeal, EntityID: deal,
			ForUser: q.Seat, Reason: q.Reason, Score: score,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// askedOfCaller is the clause that hides a core question from everyone but the
// colleague it asks. An agent on a colleague's passport answers as them. The
// system and a passportless agent see only units' suggestions.
func askedOfCaller(ctx context.Context, alias string, arg func(any) int) string {
	reader := ids.Nil
	if actor, ok := principal.Actor(ctx); ok {
		switch {
		case actor.Type == principal.PrincipalHuman:
			reader = actor.UserID
		case actor.ActsAsItsUser():
			reader = actor.OnBehalfOf
		}
	}
	if reader.IsZero() {
		return alias + ".for_user IS NULL"
	}
	return fmt.Sprintf("(%s.for_user IS NULL OR %s.for_user = $%d)", alias, alias, arg(reader))
}

// answerWhichDealTx is a colleague's answer to the deal question: filing one
// mail of a questioned conversation on an open deal. A human asked in the
// conversation files the rest of it there too. A sibling already on a deal, or
// one they may not change, keeps its filing. Then every question of the
// conversation is settled, whoever filed.
func answerWhichDealTx(ctx context.Context, tx pgx.Tx, mail ids.ActivityID, deal ids.UUID) error {
	if err := fileQuestionedConversation(ctx, tx, mail, deal); err != nil {
		return err
	}
	return settleWhichDealQuestions(ctx, tx, mail, deal)
}

func fileQuestionedConversation(ctx context.Context, tx pgx.Tx, mail ids.ActivityID, deal ids.UUID) error {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman || !auth.ReadGranted(ctx, linkEntityDeal) {
		return nil
	}
	questions, err := openWhichDealQuestions(ctx, tx, mail, true)
	if err != nil || len(questions) == 0 {
		return err
	}
	open, err := dealOpen(ctx, tx, deal)
	if err != nil || !open {
		return err
	}
	siblings, err := unfiledConversationSiblings(ctx, tx, mail)
	if err != nil {
		return err
	}
	for _, sibling := range siblings {
		// The sibling is re-checked for a deal under its own lock: one a
		// colleague filed while this waited keeps its deal alone.
		in := RelinkActivityInput{EntityType: linkEntityDeal, EntityID: deal, unfiledOnly: true}
		if _, _, err := relinkActivityRow(ctx, tx, ids.From[ids.ActivityKind](sibling), in, linkColumn(linkEntityDeal)); err != nil && !notTheirsToFile(err) {
			return err
		}
	}
	return nil
}

// settleOnFiling settles the question after a deal link a relink wrote, unless
// the door answers the question itself.
func settleOnFiling(ctx context.Context, tx pgx.Tx, mail ids.ActivityID, in RelinkActivityInput) error {
	if in.EntityType != linkEntityDeal || in.answers {
		return nil
	}
	return settleWhichDealQuestions(ctx, tx, mail, in.EntityID)
}

// settleWhichDealQuestions closes every pending question of mail's
// conversation once a mail of it is on deal.
func settleWhichDealQuestions(ctx context.Context, tx pgx.Tx, mail ids.ActivityID, deal ids.UUID) error {
	questions, err := openWhichDealQuestions(ctx, tx, mail, false)
	if err != nil {
		return err
	}
	_, err = closeWhichDealQuestions(ctx, tx, questions, &deal)
	return err
}

// dealOpen reports whether the deal is open and live.
func dealOpen(ctx context.Context, tx pgx.Tx, deal ids.UUID) (bool, error) {
	var open bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM deal d
		 WHERE d.id = $1 AND d.status = 'open' AND d.archived_at IS NULL)`, deal).Scan(&open)
	return open, err
}

// provenConversationSQL joins mail to the conversation of anchor. Only rows
// whose place in it cannot be forged count: our own attested mail, or mail
// whose sender the receiving server authenticated. The key is the sender's
// References root, so anything else may be a spoof.
var provenConversationSQL = `(` + correspondence.ConversationSQL("mail", "anchor") + ` AND ` + correspondence.ProvenSQL("mail") + `)`

// whichDealQuestion is one pending deal Margince suggested.
type whichDealQuestion struct {
	id, mail, deal ids.UUID
}

// openWhichDealQuestions is the pending deal questions of mail's
// conversation. callerOnly keeps those asked of the caller, on messages they
// may change.
func openWhichDealQuestions(ctx context.Context, tx pgx.Tx, mail ids.ActivityID, callerOnly bool) ([]whichDealQuestion, error) {
	args := []any{mail}
	asked := sqlTrue
	if callerOnly {
		asked = askedOfCaller(ctx, "s", func(v any) int { args = append(args, v); return len(args) })
	}
	rows, err := tx.Query(ctx, `SELECT s.id, s.activity_id, s.entity_id FROM activity anchor
		  JOIN activity mail ON (mail.id = anchor.id OR `+provenConversationSQL+`)
		  JOIN activity_link_suggestion s ON s.activity_id = mail.id AND s.unit IS NULL
		   AND s.entity_type = 'deal' AND s.status = 'pending'
		 WHERE anchor.id = $1 AND mail.restricted_at IS NULL AND `+asked+`
		 ORDER BY s.id`, args...)
	if err != nil {
		return nil, fmt.Errorf("activities: reading the deal question's conversation: %w", err)
	}
	questions, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (whichDealQuestion, error) {
		var q whichDealQuestion
		return q, r.Scan(&q.id, &q.mail, &q.deal)
	})
	if err != nil || len(questions) == 0 || !callerOnly {
		return questions, err
	}
	messages := make([]ids.UUID, len(questions))
	for i, q := range questions {
		messages[i] = q.mail
	}
	writable, err := auth.ActivityWritableSubset(ctx, tx, messages)
	if err != nil {
		return nil, err
	}
	mine := questions[:0]
	for _, q := range questions {
		if writable[q.mail] {
			mine = append(mine, q)
		}
	}
	return mine, nil
}

// unfiledConversationSiblings is the other proven mail of mail's conversation
// that is on no deal, in id order.
func unfiledConversationSiblings(ctx context.Context, tx pgx.Tx, mail ids.ActivityID) ([]ids.UUID, error) {
	return threadIDs(ctx, tx, `SELECT mail.id FROM activity anchor JOIN activity mail ON `+provenConversationSQL+`
		 WHERE anchor.id = $1 AND mail.restricted_at IS NULL AND mail.id <> anchor.id
		   AND NOT EXISTS (SELECT 1 FROM activity_link l WHERE l.activity_id = mail.id AND l.entity_type = 'deal')
		 ORDER BY mail.id`, mail)
}

// closeWhichDealQuestions closes the questions and answers on how many
// messages it closed one. The chosen deal's suggestion is accepted, whichever
// mail of the conversation went on it; every other one is rejected. Each
// change has its audit row and event, as a decision in the queue does.
func closeWhichDealQuestions(ctx context.Context, tx pgx.Tx, questions []whichDealQuestion, chosen *ids.UUID) (int, error) {
	decidedBy, err := storekit.CapturedBy(ctx)
	if err != nil {
		return 0, err
	}
	closed := map[ids.UUID]bool{}
	for _, q := range questions {
		answer := suggestionRejected
		if chosen != nil && q.deal == *chosen {
			answer = suggestionAccepted
		}
		var status string
		err := tx.QueryRow(ctx, `
			UPDATE activity_link_suggestion SET status = $2, decided_by = $3, decided_at = now()
			 WHERE id = $1 AND status = 'pending'
			 RETURNING status`, q.id, answer, decidedBy).Scan(&status)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return 0, fmt.Errorf("activities: closing a deal question: %w", err)
		}
		if err := recordLinkSuggestion(ctx, tx, ids.From[ids.ActivityKind](q.mail), q.id, status, linkEntityDeal, q.deal); err != nil {
			return 0, err
		}
		closed[q.mail] = true
	}
	return len(closed), nil
}

// notTheirsToFile is a refusal that leaves one row as it is: the colleague
// may not change it, or it is gone.
func notTheirsToFile(err error) bool {
	return errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrNotFound)
}

func threadIDs(ctx context.Context, tx pgx.Tx, query string, args ...any) ([]ids.UUID, error) {
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("activities: reading the deal question's conversation: %w", err)
	}
	found, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("activities: reading the deal question's conversation: %w", err)
	}
	return found, nil
}
