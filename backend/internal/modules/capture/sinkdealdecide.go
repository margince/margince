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
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// sqlUnscoped stands in for a scope clause a caller's grant leaves empty.
const sqlUnscoped = "true"

// dealFacts is what the rule reads about one mail, all of it recorded on the
// mail's own rows. outcome is set when the rule has nothing to decide.
type dealFacts struct {
	meeting  bool
	outcome  DealOutcome
	activity ids.ActivityID
	project  ids.UUID
	contacts []ids.UUID
	// provenContacts are the contacts whose part in the mail no header can forge.
	// They are the authenticated sender, and the recipients of mail we sent.
	provenContacts []ids.UUID
	companies      []ids.UUID
	thread         []ids.UUID
	// chosen is the open deals a colleague filed earlier mail of the
	// conversation on, at any company.
	chosen []ids.UUID
	// hooks are the extension changes already made to contacts and companies.
	hooks []HookChange
	// topicMemo is the digest of the topic question last answered for it.
	topicMemo string
}

// readDealFacts reads the facts. refiling reads past the mail's own deal
// links, for the repair re-checking a link the rule wrote.
func readDealFacts(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, refiling bool, hooks MailHooks) (dealFacts, error) {
	facts := dealFacts{activity: activityID}
	var kind string
	var channel *string
	var filed bool
	var project *ids.UUID
	var sender string
	err := tx.QueryRow(ctx, `
		SELECT a.kind, a.channel_provider,
		       EXISTS (SELECT 1 FROM activity_link l WHERE l.activity_id = a.id AND l.entity_type = 'deal'),
		       (SELECT l.project_id FROM activity_link l WHERE l.activity_id = a.id AND l.entity_type = 'project'),
		       coalesce(a.deal_topic_digest, ''), coalesce(a.counterparty_email, '')
		  FROM activity a WHERE a.id = $1`, activityID).Scan(&kind, &channel, &filed, &project, &facts.topicMemo, &sender)
	if err != nil {
		return facts, fmt.Errorf("capture: reading the mail the deal rule files: %w", err)
	}
	facts.meeting = kind == meetingKind
	switch {
	case !dealActivityKind(kind) || channel != nil:
		facts.outcome = DealNotMail
		return facts, nil
	case filed && !refiling:
		facts.outcome = DealAlreadyFiled
		return facts, nil
	case project != nil:
		facts.project = *project
		return facts, nil
	}
	// No one a suppressed mail names files it on a deal.
	if suppressed, err := senderSuppressedTx(ctx, tx, sender); err != nil || suppressed {
		return facts, err
	}
	if facts.contacts, facts.provenContacts, err = mailContacts(ctx, tx, activityID); err != nil {
		return facts, err
	}
	if facts.companies, err = mailCompanies(ctx, tx, activityID, facts.contacts); err != nil {
		return facts, err
	}
	if err = hooks.adjustParties(ctx, tx, &facts); err != nil {
		return facts, err
	}
	if kind != kindEmail {
		return facts, nil
	}
	if facts.chosen, err = chosenThreadDeals(ctx, tx, activityID); err != nil || len(facts.companies) != 1 {
		return facts, err
	}
	facts.thread, err = threadDeals(ctx, tx, activityID, facts.companies[0])
	return facts, err
}

// decideDeal is the rule over the facts, less the deals a colleague declined
// for this mail. Every writer of a machine deal link decides through it, so a
// decline holds for capture, the refile pass and the repair alike. A
// suggestion carries the candidates its topic check weighs. It writes nothing.
// Extension hooks run last, over what the rule and the declines left.
func decideDeal(ctx context.Context, tx pgx.Tx, facts dealFacts, candidates DealCandidates, hooks MailHooks) (DealVerdict, error) {
	verdict, err := decideCore(ctx, tx, facts, candidates)
	if err != nil {
		return verdict, err
	}
	return hooks.adjustVerdict(ctx, tx, facts, candidates, verdict)
}

// decideCore is the rule with a colleague's declines applied and no extension.
func decideCore(ctx context.Context, tx pgx.Tx, facts dealFacts, candidates DealCandidates) (DealVerdict, error) {
	verdict, err := decideByRule(ctx, tx, facts, candidates)
	if err != nil {
		return verdict, err
	}
	switch {
	case verdict.Outcome == DealLinked:
		declined, err := dealDeclined(ctx, tx, facts.activity, verdict.Deal)
		if err != nil || declined {
			return DealVerdict{Outcome: DealNone, Basis: verdict.Basis}, err
		}
		verdict.Candidates = []ids.UUID{verdict.Deal}
	case verdict.Outcome == DealAmbiguous && verdict.Basis.NeedsTopicCheck():
		open, err := topicCandidates(ctx, tx, candidates, verdict.Question)
		if err != nil {
			return DealVerdict{}, err
		}
		if verdict.Candidates, err = undeclinedDeals(ctx, tx, facts.activity, open); err != nil {
			return DealVerdict{}, err
		}
	}
	return verdict, nil
}

// topicCandidates is the open deals a topic check weighs for an ambiguous mail.
// They are the company's, or the contacts' seats when no company stands behind
// the question.
func topicCandidates(ctx context.Context, tx pgx.Tx, candidates DealCandidates, question WhichDeal) ([]ids.UUID, error) {
	if len(question.StakeholderIDs) > 0 {
		return candidates.OpenDealsOfStakeholders(ctx, tx, question.StakeholderIDs, DealTopicCandidates)
	}
	return candidates.OpenDealsOfCompanyUpTo(ctx, tx, question.CompanyID, DealTopicCandidates)
}

// undeclinedDeals is deals less those a colleague declined for the mail.
func undeclinedDeals(ctx context.Context, tx pgx.Tx, mail ids.ActivityID, deals []ids.UUID) ([]ids.UUID, error) {
	var args []any
	arg := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	mailArg, dealsArg := arg(mail), arg(deals)
	query := `SELECT d FROM unnest(` + dealsArg + `::uuid[]) d WHERE NOT ` + provenance.DealDeclinedSQL(mailArg, "d") + ` ORDER BY d`
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("capture: reading the deals a colleague declined: %w", err)
	}
	kept, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("capture: reading the deals a colleague declined: %w", err)
	}
	return kept, nil
}

// dealDeclined answers whether a colleague said the mail is not about deal.
func dealDeclined(ctx context.Context, tx pgx.Tx, mail ids.ActivityID, deal ids.UUID) (bool, error) {
	var args []any
	arg := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	query := `SELECT ` + provenance.DealDeclinedSQL(arg(mail), arg(deal)+"::uuid")
	var declined bool
	if err := tx.QueryRow(ctx, query, args...).Scan(&declined); err != nil {
		return false, fmt.Errorf("capture: reading whether a colleague declined the deal: %w", err)
	}
	return declined, nil
}

// decideByRule is the rule's rungs over the facts.
func decideByRule(ctx context.Context, tx pgx.Tx, facts dealFacts, candidates DealCandidates) (DealVerdict, error) {
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
		return withBasis(oneDeal(deals, WhichDeal{ProjectID: facts.project}), DealBasisProject, DealBasisProject), err
	}
	// A colleague's choice for the conversation outranks every company rung,
	// because a partner's mail is often about a deal at the customer. Two
	// choices disagree, and the machine picks neither.
	switch len(facts.chosen) {
	case 1:
		return DealVerdict{Outcome: DealLinked, Deal: facts.chosen[0], Basis: DealBasisChosenThread}, nil
	case 2:
		return DealVerdict{Outcome: DealNone, Basis: DealBasisChosenThread}, nil
	}
	verdict, err := decideByCompany(ctx, tx, facts, candidates)
	if err != nil || verdict.Outcome != DealNone || len(facts.contacts) == 0 {
		return verdict, err
	}
	return decideByStakeholderElsewhere(ctx, tx, facts, candidates)
}

// decideByStakeholderElsewhere ties the mail to an open deal at another company
// through a seat, as for a consultant on a client's deal. A seat of a proven
// participant files the mail. A seat of a contact only named in a header asks a
// colleague instead, because a sender could name anyone there.
func decideByStakeholderElsewhere(ctx context.Context, tx pgx.Tx, facts dealFacts, candidates DealCandidates) (DealVerdict, error) {
	if len(facts.provenContacts) > 0 {
		staked, err := candidates.OpenDealsOfStakeholders(ctx, tx, facts.provenContacts, 2)
		if err != nil || len(staked) > 0 {
			return withBasis(oneDeal(staked, WhichDeal{StakeholderIDs: facts.provenContacts}), DealBasisStakeholder, DealBasisNamedStakeholder), err
		}
	}
	named, err := candidates.OpenDealsOfStakeholders(ctx, tx, facts.contacts, 1)
	if err != nil || len(named) == 0 {
		return DealVerdict{Outcome: DealNone}, err
	}
	return DealVerdict{Outcome: DealAmbiguous, Question: WhichDeal{StakeholderIDs: facts.contacts}, Basis: DealBasisNamedStakeholder}, nil
}

// decideByCompany is the rungs that start from the mail's company: its thread,
// its only open deal, and the stakeholders among several.
func decideByCompany(ctx context.Context, tx pgx.Tx, facts dealFacts, candidates DealCandidates) (DealVerdict, error) {
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
	// The thread a mail claims is typed by its sender. A sibling's deal the
	// machine chose counts only when the mail's own company corroborates it.
	// Only a colleague's choice crosses companies (decideByRule).
	switch len(facts.thread) {
	case 1:
		return DealVerdict{Outcome: DealLinked, Deal: facts.thread[0], Basis: DealBasisThread}, nil
	case 2:
		return DealVerdict{Outcome: DealAmbiguous, Question: question, Basis: DealBasisThread}, nil
	}
	deals, err := candidates.OpenDealsOfCompany(ctx, tx, company)
	if err != nil || len(deals) < 2 || len(facts.contacts) == 0 {
		return withBasis(oneDeal(deals, question), DealBasisSoleOpenDeal, DealBasisAmbiguous), err
	}
	staked, err := candidates.OpenDealsWithStakeholders(ctx, tx, company, facts.contacts)
	if err != nil || len(staked) == 1 {
		return withBasis(oneDeal(staked, question), DealBasisStakeholder, DealBasisAmbiguous), err
	}
	return DealVerdict{Outcome: DealAmbiguous, Question: question, Basis: DealBasisAmbiguous}, nil
}

// withBasis names the rung of a verdict: linked when it found one deal,
// ambiguous when it found several.
func withBasis(verdict DealVerdict, linked, ambiguous DealBasis) DealVerdict {
	switch verdict.Outcome {
	case DealLinked:
		verdict.Basis = linked
	case DealAmbiguous:
		verdict.Basis = ambiguous
	}
	return verdict
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

// Participant evidence survives the link cap; explicit contact filings also count.
// The second answer is the contacts no header can forge. They are the From of
// authenticated inbound mail, and the To and Cc of mail a colleague sent. The
// sender check covers the domain, not the address, so a consumer-mail From
// proves nothing.
func mailContacts(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID) (all, proven []ids.UUID, err error) {
	if !auth.ReadGranted(ctx, "contact") {
		return nil, nil, nil
	}
	consumer, err := MatcherTx(ctx, tx)
	if err != nil {
		return nil, nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT named.contact_id, named.signed, named.sent, named.domain FROM (
		 SELECT p.contact_id, (`+correspondence.AuthenticatedInboundSQL("a")+` AND p.role = 'from') AS signed,
		        (`+correspondence.OurOutboundSQL("a")+` AND p.role IN ('to', 'cc')) AS sent,
		        coalesce(p.address_domain_folded, '') AS domain
		   FROM activity_participant p JOIN activity a ON a.id = p.activity_id WHERE p.activity_id=$1
		 UNION ALL SELECT l.contact_id, false, false, '' FROM activity_link l WHERE l.activity_id=$1 AND l.entity_type='contact'
		) named JOIN contact c ON c.id=named.contact_id AND c.archived_at IS NULL
		ORDER BY named.contact_id`, activityID)
	if err != nil {
		return nil, nil, fmt.Errorf("capture: reading the mail's participants: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var contact ids.UUID
		var signed, sent bool
		var domain string
		if err := rows.Scan(&contact, &signed, &sent, &domain); err != nil {
			return nil, nil, fmt.Errorf("capture: reading the mail's participants: %w", err)
		}
		if len(all) == 0 || all[len(all)-1] != contact {
			all = append(all, contact)
		}
		if (sent || signed && !consumer.IsConsumer(domain)) && (len(proven) == 0 || proven[len(proven)-1] != contact) {
			proven = append(proven, contact)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("capture: reading the mail's participants: %w", err)
	}
	return all, proven, nil
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
		 WHERE r.contact_id = ANY($%d) AND r.company_id IS NOT NULL`, arg(contacts)) + `
		   AND ` + employment.CurrentMembershipSQL("r") + `
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
// conversation is filed under. The conversation and the proof are
// correspondence's, as the deal answer and the request settlement read them.
func threadDeals(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, company ids.UUID) ([]ids.UUID, error) {
	return conversationDeals(ctx, tx, activityID, func(arg func(any) int) string {
		return correspondence.ProvenSQL("a") + fmt.Sprintf(` AND d.company_id = $%d`, arg(company))
	})
}

// chosenThreadDeals is the open deals at any company that a human or an agent
// filed earlier mail of this mail's conversation on. Their choice is the
// proof, so the sibling need not be proven itself. A later choice never
// reroutes older mail.
func chosenThreadDeals(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID) ([]ids.UUID, error) {
	return conversationDeals(ctx, tx, activityID, func(func(any) int) string {
		return `(a.occurred_at, a.id) < (self.occurred_at, self.id) AND ` + provenance.HumanChoseDealSQL("a.id", "d.id")
	})
}

// conversationDeals is the open deals that other mail of this mail's
// conversation is filed under and that sibling passes. It answers at most two,
// after the whole population is filtered. The mail itself must be proven: a
// reply anyone could have typed follows no deal. Only unrestricted siblings
// and deals the capturing seat may read count.
func conversationDeals(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, sibling func(arg func(any) int) string) ([]ids.UUID, error) {
	if !auth.ReadGranted(ctx, "deal") {
		return nil, nil
	}
	args := []any{activityID}
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
	query := `
		SELECT DISTINCT d.id
		  FROM activity self
		  JOIN activity a ON a.id <> self.id AND ` + correspondence.ConversationSQL("a", "self") + `
		  JOIN activity_link l ON l.activity_id = a.id AND l.entity_type = 'deal' AND ` + relstrength.DealLinkStandsSQL("l") + `
		  JOIN deal d ON d.id = l.deal_id
		 WHERE self.id = $1 AND ` + correspondence.ProvenSQL("self") + `
		   AND a.restricted_at IS NULL AND (` + siblingScope + `)
		   AND d.status = 'open' AND d.archived_at IS NULL AND (` + dealScope + `)
		   AND ` + sibling(arg) + `
		 ORDER BY d.id
		 LIMIT 2`
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("capture: reading the conversation's deals: %w", err)
	}
	deals, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("capture: reading the conversation's deals: %w", err)
	}
	return deals, nil
}

func dealActivityKind(kind string) bool {
	return kind == kindEmail || kind == meetingKind
}
