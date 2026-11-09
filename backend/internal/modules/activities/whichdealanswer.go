// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// answerWhichDealTx is a colleague's answer to the deal question. Filing one mail
// of a questioned conversation on an open deal files the rest of it there
// too, and settles the question. It runs only for a human, and only while the
// conversation has an open question on that deal's terms. A sibling already on
// a deal, or one the colleague may not change, keeps its filing. Each sibling
// goes through the guarded row write, with its own audit row and event.
func (s *Store) answerWhichDealTx(ctx context.Context, tx pgx.Tx, mail ids.ActivityID, deal ids.UUID) error {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman || !auth.ReadGranted(ctx, linkEntityDeal) {
		return nil
	}
	questions, err := openWhichDealQuestions(ctx, tx, mail)
	if err != nil || len(questions) == 0 {
		return err
	}
	var open bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM deal d
		 WHERE d.id = $1 AND d.status = 'open' AND d.archived_at IS NULL)`, deal).Scan(&open); err != nil || !open {
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
	_, err = s.completeWhichDealQuestions(ctx, tx, questions)
	return err
}

// provenConversationSQL joins mail to the conversation of anchor. Only rows
// whose place in it cannot be forged count: our own attested mail, or mail
// whose sender the receiving server authenticated. The key is the sender's
// References root, so anything else may be a spoof.
var provenConversationSQL = `(` + correspondence.ConversationSQL("mail", "anchor") + ` AND ` + correspondence.ProvenSQL("mail") + `)`

// openWhichDealQuestions is the open deal questions of mail's conversation.
func openWhichDealQuestions(ctx context.Context, tx pgx.Tx, mail ids.ActivityID) ([]ids.UUID, error) {
	return threadIDs(ctx, tx, `SELECT q.id FROM activity anchor
		  JOIN activity mail ON (mail.id = anchor.id OR `+provenConversationSQL+`)
		  JOIN activity q ON q.source_activity_id = mail.id AND q.kind = 'task' AND q.source_system = $2
		   AND q.archived_at IS NULL AND NOT q.is_done
		 WHERE anchor.id = $1 AND mail.restricted_at IS NULL
		 ORDER BY q.id`, mail, provenance.WhichDealSource)
}

// unfiledConversationSiblings is the other proven mail of mail's conversation
// that is on no deal, in id order.
func unfiledConversationSiblings(ctx context.Context, tx pgx.Tx, mail ids.ActivityID) ([]ids.UUID, error) {
	return threadIDs(ctx, tx, `SELECT mail.id FROM activity anchor JOIN activity mail ON `+provenConversationSQL+`
		 WHERE anchor.id = $1 AND mail.restricted_at IS NULL AND mail.id <> anchor.id
		   AND NOT EXISTS (SELECT 1 FROM activity_link l WHERE l.activity_id = mail.id AND l.entity_type = 'deal')
		 ORDER BY mail.id`, mail)
}

// completeWhichDealQuestions marks the questions done, skipping one the
// colleague may not change, and answers how many it closed.
func (s *Store) completeWhichDealQuestions(ctx context.Context, tx pgx.Tx, questions []ids.UUID) (int, error) {
	closed := 0
	for _, question := range questions {
		done, err := s.CompleteTaskTx(ctx, tx, ids.From[ids.ActivityKind](question), s.now())
		switch {
		case err != nil && !notTheirsToFile(err):
			return closed, err
		case err == nil && done:
			closed++
		}
	}
	return closed, nil
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
