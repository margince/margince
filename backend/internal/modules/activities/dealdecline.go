// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// A colleague's "not about this deal", and "none of them" to the deal
// question. The decline is per deal: the mail leaves that deal, and no
// automatic pass files it there again (provenance.DealDeclinedSQL). Other
// deals stay open to it.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// dealDeclineUndoReason is the stated reason of the project-filing undo a
// decline makes, since the decline itself is the member's decision.
const dealDeclineUndoReason = "Not about this deal: the project came from the deal."

// DeclineDeal says the activity is not about deal. A nil deal answers "none
// of them" to the open deal question of the activity's conversation. Every
// deal the question offered is then declined on the activity and on each
// unfiled message of the conversation. The question is done.
func (s *Store) DeclineDeal(ctx context.Context, id ids.ActivityID, deal *ids.UUID) (crmcontracts.ActivityDealDecline, error) {
	out := crmcontracts.ActivityDealDecline{DeclinedDealIds: []openapi_types.UUID{}}
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return out, err
	}
	if err := auth.Require(ctx, linkEntityDeal, principal.ActionRead); err != nil {
		return out, err
	}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		if err := lockForFilingChange(ctx, tx, id); err != nil {
			return err
		}
		if deal != nil {
			if err := auth.EnsureVisibleExisting(ctx, tx, linkEntityDeal, *deal); err != nil {
				return err
			}
			return s.declineOnMessage(ctx, tx, id, []ids.UUID{*deal}, &out)
		}
		return s.declineEveryOfferedDeal(ctx, tx, id, &out)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return out, apperrors.ErrNotFound
	}
	return out, err
}

// declineEveryOfferedDeal is "none of them". A deal the caller cannot see is
// not declined, and the question is done all the same.
func (s *Store) declineEveryOfferedDeal(ctx context.Context, tx pgx.Tx, mail ids.ActivityID, out *crmcontracts.ActivityDealDecline) error {
	questions, err := openWhichDealQuestions(ctx, tx, mail)
	if err != nil {
		return err
	}
	if len(questions) == 0 {
		return httperr.Validation("deal_id", codeRequired,
			"this message has no open deal question, so name the deal it is not about")
	}
	deals, err := offeredDeals(ctx, tx, questions)
	if err != nil {
		return err
	}
	if err := s.declineOnMessage(ctx, tx, mail, deals, out); err != nil {
		return err
	}
	siblings, err := unfiledConversationSiblings(ctx, tx, mail)
	if err != nil {
		return err
	}
	for _, sibling := range siblings {
		if err := declineOnSibling(ctx, tx, ids.From[ids.ActivityKind](sibling), deals); err != nil {
			return err
		}
	}
	out.QuestionsClosed, err = s.completeWhichDealQuestions(ctx, tx, questions)
	return err
}

// declineOnMessage declines each deal on the message the caller named, which
// lockForFilingChange holds. A project link that came from a deal taken off
// goes too, under the project-filing undo's rules.
func (s *Store) declineOnMessage(ctx context.Context, tx pgx.Tx, mail ids.ActivityID, deals []ids.UUID, out *crmcontracts.ActivityDealDecline) error {
	for _, deal := range deals {
		declined, removed, err := declineDealTx(ctx, tx, mail, deal)
		if err != nil {
			return err
		}
		if declined {
			out.DeclinedDealIds = append(out.DeclinedDealIds, openapi_types.UUID(deal))
		}
		if !removed {
			continue
		}
		out.Unlinked = true
		if err := unfileDealProject(ctx, tx, mail, deal, out); err != nil {
			return err
		}
	}
	return nil
}

// declineOnSibling declines the deals on one more message of the
// conversation. One the caller may not change, or one under a retention hold,
// keeps what it has.
func declineOnSibling(ctx context.Context, tx pgx.Tx, mail ids.ActivityID, deals []ids.UUID) error {
	held, err := lockActivityForWrite(ctx, tx, mail.UUID)
	if err == nil && !held {
		err = auth.EnsureActivityWritableIn(ctx, tx, mail.UUID, true)
	}
	if notTheirsToFile(err) || held {
		return nil
	}
	if err != nil {
		return err
	}
	for _, deal := range deals {
		if _, _, err := declineDealTx(ctx, tx, mail, deal); err != nil {
			return err
		}
	}
	return nil
}

// declineDealTx takes the deal link off the locked message and records the
// decline. A deal declined before, and not linked again since, writes nothing.
// It answers whether this call declined the deal, and whether a link came off.
func declineDealTx(ctx context.Context, tx pgx.Tx, mail ids.ActivityID, deal ids.UUID) (declined, removed bool, err error) {
	var args []any
	arg := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	mailArg, dealArg := arg(mail), arg(deal)
	var already, linked bool
	err = tx.QueryRow(ctx, `SELECT `+provenance.DealDeclinedSQL(mailArg, dealArg+"::uuid")+`,
		 EXISTS (SELECT 1 FROM activity_link l
		          WHERE l.activity_id = `+mailArg+` AND l.entity_type = 'deal' AND l.deal_id = `+dealArg+`)`,
		args...).Scan(&already, &linked)
	if err != nil || (already && !linked) {
		return false, false, err
	}
	decision := map[string]any{fieldEntityType: linkEntityDeal, fieldEntityID: deal, provenance.DealDeclinedKey: true}
	removed, err = unlinkRecordedTx(ctx, tx, mail, linkEntityDeal, deal, decision, nil)
	return !already, removed, err
}

// offeredDeals is the open deals the questions offered: those of the
// question's project, else those of its company, as the caller may read them.
func offeredDeals(ctx context.Context, tx pgx.Tx, questions []ids.UUID) ([]ids.UUID, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	questionsArg := arg(questions)
	scope, err := auth.ScopeClauseFor(ctx, linkEntityDeal, "d", arg)
	if err != nil {
		return nil, err
	}
	if scope == "" {
		scope = sqlTrue
	}
	return threadIDs(ctx, tx, `SELECT DISTINCT d.id FROM activity_link ql
		  JOIN deal d ON (ql.entity_type = 'project' AND d.project_id = ql.project_id)
		    OR (ql.entity_type = 'company' AND d.company_id = ql.company_id
		        AND NOT EXISTS (SELECT 1 FROM activity_link qp WHERE qp.activity_id = ql.activity_id AND qp.entity_type = 'project'))
		 WHERE ql.activity_id = ANY($`+fmt.Sprint(questionsArg)+`) AND d.status = 'open' AND d.archived_at IS NULL AND (`+scope+`)
		 ORDER BY d.id`, args...)
}
