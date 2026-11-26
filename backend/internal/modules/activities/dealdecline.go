// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// A colleague's "not about this deal", and "none of them" to the deal
// question. The decline is per deal: the mail's link to that deal is marked
// unrelated (dealrelevance.go), and no automatic pass files it there again
// (provenance.DealDeclinedSQL).

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
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
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
		if deal != nil {
			if err := lockMarkedLink(ctx, tx, id, *deal); err != nil {
				return err
			}
			return s.declineOnMessage(ctx, tx, id, []ids.UUID{*deal}, &out)
		}
		if err := lockForFilingChange(ctx, tx, id); err != nil {
			return err
		}
		return s.declineEveryOfferedDeal(ctx, tx, id, &out)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return out, apperrors.ErrNotFound
	}
	return out, err
}

// declineEveryOfferedDeal is "none of them" to the questions the caller may
// read and mark done. A deal the caller cannot see is not declined, and the
// question is done all the same.
func (s *Store) declineEveryOfferedDeal(ctx context.Context, tx pgx.Tx, mail ids.ActivityID, out *crmcontracts.ActivityDealDecline) error {
	questions, err := answerableWhichDealQuestions(ctx, tx, mail)
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
// lockForFilingChange holds. A project link that came from a deal marked so
// goes too, under the project-filing undo's rules.
func (s *Store) declineOnMessage(ctx context.Context, tx pgx.Tx, mail ids.ActivityID, deals []ids.UUID, out *crmcontracts.ActivityDealDecline) error {
	for _, deal := range deals {
		decline, err := declineDealTx(ctx, tx, mail, deal)
		if err != nil {
			return err
		}
		if decline.declined {
			out.DeclinedDealIds = append(out.DeclinedDealIds, openapi_types.UUID(deal))
		}
		out.MarkedUnrelated = out.MarkedUnrelated || decline.marked
		out.ProjectUnfiled = out.ProjectUnfiled || decline.project.unfiled
		out.ProjectKept = out.ProjectKept || decline.project.kept
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
		if _, err := declineDealTx(ctx, tx, mail, deal); err != nil {
			return err
		}
	}
	return nil
}

// dealDecline is what declineDealTx did. linked says the message carries a
// link to the deal, which the decline marks rather than takes off.
type dealDecline struct {
	declined, linked, marked bool
	project                  projectUnfiling
}

// declineDealTx marks the locked message's link to deal unrelated, or records
// the decline when there is no link. A deal declined before writes nothing,
// and so does a linked deal the caller may not change.
func declineDealTx(ctx context.Context, tx pgx.Tx, mail ids.ActivityID, deal ids.UUID) (dealDecline, error) {
	var args []any
	arg := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	mailArg, dealArg := arg(mail), arg(deal)
	var already, linked bool
	err := tx.QueryRow(ctx, `SELECT `+provenance.DealDeclinedSQL(mailArg, dealArg+"::uuid")+`,
		 EXISTS (SELECT 1 FROM activity_link l
		          WHERE l.activity_id = `+mailArg+` AND l.entity_type = 'deal' AND l.deal_id = `+dealArg+`)`,
		args...).Scan(&already, &linked)
	if err != nil {
		return dealDecline{}, err
	}
	decision := map[string]any{fieldEntityType: linkEntityDeal, fieldEntityID: deal, provenance.DealDeclinedKey: true}
	if !linked {
		if already {
			return dealDecline{}, nil
		}
		_, err := unlinkRecordedTx(ctx, tx, mail, linkEntityDeal, deal, decision, nil)
		return dealDecline{declined: true}, err
	}
	if err := auth.EnsureChangeable(ctx, tx, linkEntityDeal, deal); notTheirsToFile(err) {
		return dealDecline{linked: true}, nil
	} else if err != nil {
		return dealDecline{}, err
	}
	marked, err := writeDealMarkTx(ctx, tx, mail, deal, dealMark{
		relevance: relstrength.DealUnrelated, source: relstrength.RelevanceByHuman, decision: decision,
	})
	if err != nil || !marked {
		return dealDecline{linked: true}, err
	}
	project, err := unfileDealProject(ctx, tx, mail, deal)
	return dealDecline{declined: !already, linked: true, marked: true, project: project}, err
}

// answerableWhichDealQuestions is the open deal questions of mail's
// conversation whose content the caller may read and which they may mark done.
func answerableWhichDealQuestions(ctx context.Context, tx pgx.Tx, mail ids.ActivityID) ([]ids.UUID, error) {
	questions, err := openWhichDealQuestions(ctx, tx, mail)
	if err != nil || len(questions) == 0 {
		return nil, err
	}
	visible, err := VisibleTasks(ctx, tx, questions)
	if err != nil {
		return nil, err
	}
	writable, err := auth.ActivityWritableSubset(ctx, tx, questions)
	if err != nil {
		return nil, err
	}
	answerable := questions[:0]
	for _, question := range questions {
		if visible[question] && writable[question] {
			answerable = append(answerable, question)
		}
	}
	return answerable, nil
}

// offeredDeals is the deals the questions recorded as offered, as the caller
// may read them. A question raised before questions recorded their deals
// offers the open deals of its project, else of its company, read now.
func offeredDeals(ctx context.Context, tx pgx.Tx, questions []ids.UUID) ([]ids.UUID, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	questionsArg, keyArg := arg(questions), arg(provenance.WhichDealOfferedKey)
	scope, err := auth.ScopeClauseFor(ctx, linkEntityDeal, "d", arg)
	if err != nil {
		return nil, err
	}
	if scope == "" {
		scope = sqlTrue
	}
	recorded := fmt.Sprintf(`q.raw->$%d`, keyArg)
	return threadIDs(ctx, tx, `SELECT DISTINCT d.id FROM activity q JOIN deal d ON CASE
		   WHEN jsonb_typeof(`+recorded+`) = 'array'
		     THEN d.id::text IN (SELECT jsonb_array_elements_text(`+recorded+`))
		   ELSE d.status = 'open' AND d.archived_at IS NULL AND EXISTS (SELECT 1 FROM activity_link ql
		     WHERE ql.activity_id = q.id AND ((ql.entity_type = 'project' AND d.project_id = ql.project_id)
		       OR (ql.entity_type = 'company' AND d.company_id = ql.company_id
		           AND NOT EXISTS (SELECT 1 FROM activity_link qp WHERE qp.activity_id = q.id AND qp.entity_type = 'project'))))
		   END
		 WHERE q.id = ANY($`+fmt.Sprint(questionsArg)+`) AND (`+scope+`)
		 ORDER BY d.id`, args...)
}
