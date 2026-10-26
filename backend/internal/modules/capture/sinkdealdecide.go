// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// sqlUnscoped stands in for a scope clause a caller's grant leaves empty.
const sqlUnscoped = "true"

// dealFacts is what the rule reads about one mail, all of it recorded on the
// mail's own rows. outcome is set when the rule has nothing to decide.
type dealFacts struct {
	outcome   DealOutcome
	activity  ids.ActivityID
	project   ids.UUID
	contacts  []ids.UUID
	companies []ids.UUID
	thread    []ids.UUID
}

// readDealFacts reads the facts. refiling reads past the mail's own deal
// links, for the repair re-checking a link the rule wrote.
func readDealFacts(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, refiling bool) (dealFacts, error) {
	facts := dealFacts{activity: activityID}
	var kind string
	var channel *string
	var filed bool
	var project *ids.UUID
	err := tx.QueryRow(ctx, `
		SELECT a.kind, a.channel_provider,
		       EXISTS (SELECT 1 FROM activity_link l WHERE l.activity_id = a.id AND l.entity_type = 'deal'),
		       (SELECT l.project_id FROM activity_link l WHERE l.activity_id = a.id AND l.entity_type = 'project')
		  FROM activity a WHERE a.id = $1`, activityID).Scan(&kind, &channel, &filed, &project)
	if err != nil {
		return facts, fmt.Errorf("capture: reading the mail the deal rule files: %w", err)
	}
	switch {
	case kind != kindEmail || channel != nil:
		facts.outcome = DealNotMail
		return facts, nil
	case filed && !refiling:
		facts.outcome = DealAlreadyFiled
		return facts, nil
	case project != nil:
		facts.project = *project
		return facts, nil
	}
	if facts.contacts, err = mailContacts(ctx, tx, activityID); err != nil {
		return facts, err
	}
	if facts.companies, err = mailCompanies(ctx, tx, activityID, facts.contacts); err != nil {
		return facts, err
	}
	if len(facts.companies) == 1 {
		facts.thread, err = threadDeals(ctx, tx, activityID, facts.companies[0])
	}
	return facts, err
}

// decideDeal is the rule over the facts. It writes nothing.
func decideDeal(ctx context.Context, tx pgx.Tx, facts dealFacts, candidates DealCandidates) (DealVerdict, error) {
	// A role that may not read deals files mail under none. That is a
	// configuration, not a fault.
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return DealVerdict{Outcome: DealNone}, nil
		}
		return DealVerdict{}, err
	}
	if !facts.project.IsZero() {
		deals, err := candidates.OpenDealsOfProject(ctx, tx, facts.project)
		return oneDeal(deals, WhichDeal{ProjectID: facts.project}), err
	}
	if len(facts.companies) > 1 {
		return DealVerdict{Outcome: DealConflictingFirms}, nil
	}
	var company ids.UUID
	if len(facts.companies) == 1 {
		company = facts.companies[0]
	}
	question := WhichDeal{CompanyID: company}
	if company.IsZero() {
		return DealVerdict{Outcome: DealNone}, nil
	}
	// The thread a mail claims is typed by its sender, so a sibling's deal
	// counts only when the mail's own company corroborates it. A deal at
	// another company is no clue about this one.
	switch len(facts.thread) {
	case 1:
		return DealVerdict{Outcome: DealLinked, Deal: facts.thread[0]}, nil
	case 2:
		return DealVerdict{Outcome: DealAmbiguous, Question: question}, nil
	}
	deals, err := candidates.OpenDealsOfCompany(ctx, tx, company)
	if err != nil || len(deals) < 2 || len(facts.contacts) == 0 {
		return oneDeal(deals, question), err
	}
	staked, err := candidates.OpenDealsWithStakeholders(ctx, tx, company, facts.contacts)
	if err != nil || len(staked) == 1 {
		return oneDeal(staked, question), err
	}
	return DealVerdict{Outcome: DealAmbiguous, Question: question}, nil
}

// oneDeal turns a candidate read into a verdict: one is the deal, none is no
// deal, several are a question.
func oneDeal(deals []ids.UUID, question WhichDeal) DealVerdict {
	switch len(deals) {
	case 0:
		return DealVerdict{Outcome: DealNone}
	case 1:
		return DealVerdict{Outcome: DealLinked, Deal: deals[0]}
	default:
		return DealVerdict{Outcome: DealAmbiguous, Question: question}
	}
}

// mailContacts answers the live contacts among the mail's participants. They
// come from activity_participant: the participant link arm caps how many get
// a link, so a copied contact can be missing from activity_link.
func mailContacts(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID) ([]ids.UUID, error) {
	if !auth.ReadGranted(ctx, "contact") {
		return nil, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT p.contact_id
		  FROM activity_participant p
		  JOIN contact c ON c.id = p.contact_id AND c.archived_at IS NULL
		 WHERE p.activity_id = $1
		 ORDER BY p.contact_id`, activityID)
	if err != nil {
		return nil, fmt.Errorf("capture: reading the mail's participants: %w", err)
	}
	contacts, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("capture: reading the mail's participants: %w", err)
	}
	return contacts, nil
}

// mailCompanies is the customer companies the mail names, at most two: its
// company links and every current employer of its participants. A contact
// with two employers is conflicting evidence, not a guess. The installation's
// own company is never a customer. Without the edge grant, only links count.
func mailCompanies(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, contacts []ids.UUID) ([]ids.UUID, error) {
	if !auth.ReadGranted(ctx, "company") {
		return nil, nil
	}
	args := []any{activityID}
	arg := func(v any) int { args = append(args, v); return len(args) }
	employers := ""
	edgeScope, err := auth.EdgeReadScope(ctx, "r", arg)
	switch {
	case errors.Is(err, apperrors.ErrPermissionDenied):
	case err != nil:
		return nil, err
	default:
		if edgeScope == "" {
			edgeScope = sqlUnscoped
		}
		employers = fmt.Sprintf(`
		UNION
		SELECT r.company_id FROM relationship r
		 WHERE r.contact_id = ANY($%d) AND r.kind = 'employment' AND r.company_id IS NOT NULL`, arg(contacts)) + `
		   AND r.archived_at IS NULL AND ` + employment.IsCurrentSQL("r.ended_at") + `
		   AND (` + edgeScope + `)`
	}
	rows, err := tx.Query(ctx, `
		SELECT named.company_id
		  FROM (SELECT l.company_id FROM activity_link l
		         WHERE l.activity_id = $1 AND l.entity_type = 'company'`+employers+`) AS named
		  JOIN company c ON c.id = named.company_id AND c.archived_at IS NULL AND NOT c.is_anchor
		 ORDER BY named.company_id
		 LIMIT 2`, args...)
	if err != nil {
		return nil, fmt.Errorf("capture: reading the mail's companies: %w", err)
	}
	companies, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("capture: reading the mail's companies: %w", err)
	}
	return companies, nil
}

// threadDeals is the open deals of company that proven mail of this mail's
// conversation is filed under. It answers at most two, after the whole
// population is filtered. The conversation and the proof are correspondence's, as the deal
// answer and the request settlement read them. The mail itself must be proven
// too: a reply anyone could have typed follows no deal. Only siblings and
// deals the capturing seat may read count.
func threadDeals(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, company ids.UUID) ([]ids.UUID, error) {
	if !auth.ReadGranted(ctx, "deal") {
		return nil, nil
	}
	args := []any{activityID, company}
	arg := func(v any) int { args = append(args, v); return len(args) }
	siblingScope, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	dealScope, err := auth.ScopeClauseFor(ctx, "deal", "d", arg)
	if err != nil {
		return nil, err
	}
	for _, clause := range []*string{&siblingScope, &dealScope} {
		if *clause == "" {
			*clause = sqlUnscoped
		}
	}
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT d.id
		  FROM activity self
		  JOIN activity a ON a.id <> self.id AND `+correspondence.ConversationSQL("a", "self")+`
		  JOIN activity_link l ON l.activity_id = a.id AND l.entity_type = 'deal'
		  JOIN deal d ON d.id = l.deal_id
		 WHERE self.id = $1 AND `+correspondence.ProvenSQL("self")+`
		   AND `+correspondence.ProvenSQL("a")+`
		   AND a.restricted_at IS NULL AND (`+siblingScope+`)
		   AND d.company_id = $2 AND d.status = 'open' AND d.archived_at IS NULL AND (`+dealScope+`)
		 ORDER BY d.id
		 LIMIT 2`, args...)
	if err != nil {
		return nil, fmt.Errorf("capture: reading the conversation's deals: %w", err)
	}
	deals, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("capture: reading the conversation's deals: %w", err)
	}
	return deals, nil
}
