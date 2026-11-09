// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/freemail"
	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// domainQuestionContactLimit bounds how many senders a question names.
const domainQuestionContactLimit = 3

// The reasons a company is suggested for an open question, strongest first.
const (
	SuggestionNameTwin        = "name_twin"
	SuggestionRedirectTarget  = "redirect_target"
	SuggestionContactEmployer = "contact_employer"
)

// Whether an unevidenced question's site was read.
const (
	QuestionSiteRead   = "read"
	QuestionSiteUnread = "unread"
)

// DomainQuestionContext says whose mail raised a question and which existing
// company, if any, the evidence points at. Every name in it passed the
// reader's own row scope.
type DomainQuestionContext struct {
	Contacts      []DomainQuestionContact
	ContactCount  int
	HiddenSenders bool
	SenderAddress string
	OriginID      *ids.UUID
	Suggestion    *DomainQuestionSuggestion
	SiteRead      string
	// LinkedContactCount is how many contacts the reader may see that a
	// confirm would employ there: the planter's own set.
	LinkedContactCount int

	// visibleSenders holds the senders the reader may see, past the display
	// cap, so a suggestion is judged over all of them.
	visibleSenders []ids.ContactID
}

// DomainQuestionContact is one sender the reader may see.
type DomainQuestionContact struct {
	ID   ids.ContactID
	Name string
}

// DomainQuestionSuggestion is the existing company the evidence points at,
// and which evidence that is.
type DomainQuestionSuggestion struct {
	CompanyID ids.CompanyID
	Name      string
	Basis     string
}

// domainQuestionEvidence is what the list row already knows and the context
// read needs: the redirect target and the stored name twin.
type domainQuestionEvidence struct {
	redirectTarget string
	nameTwin       *ids.CompanyID
	siteUnreadable bool
	reasonCode     string
}

func redirectTargetOf(forward *DomainForwardEvidence) string {
	if forward == nil {
		return ""
	}
	parsed, err := url.Parse(forward.EffectiveURL)
	if err != nil {
		return ""
	}
	target, ok := freemail.Hostname(parsed.Hostname())
	if !ok {
		return ""
	}
	return target
}

// domainQuestionContextTx reads the context of every listed question in a
// fixed number of queries, whatever the page size.
func domainQuestionContextTx(ctx context.Context, tx pgx.Tx, evidence map[string]domainQuestionEvidence, asOf time.Time) (map[string]DomainQuestionContext, error) {
	out := make(map[string]DomainQuestionContext, len(evidence))
	if len(evidence) == 0 {
		return out, nil
	}
	domains := make([]string, 0, len(evidence))
	for domain, known := range evidence {
		domains = append(domains, domain)
		one := DomainQuestionContext{}
		if known.reasonCode == PendingUnevidenced {
			one.SiteRead = QuestionSiteRead
			if known.siteUnreadable {
				one.SiteRead = QuestionSiteUnread
			}
		}
		out[domain] = one
	}
	if err := readDomainQuestionSendersTx(ctx, tx, domains, asOf, out); err != nil {
		return nil, err
	}
	if err := suggestDomainQuestionCompaniesTx(ctx, tx, evidence, out); err != nil {
		return nil, err
	}
	if err := countLinkedContactsTx(ctx, tx, domains, out); err != nil {
		return nil, err
	}
	return out, nil
}

// readDomainQuestionSendersTx names the contacts who sent the question's mail.
// It reads only mail the reader may read, the same mail that opened the
// question for them, and names only contacts inside their scope.
func readDomainQuestionSendersTx(ctx context.Context, tx pgx.Tx, domains []string, asOf time.Time, out map[string]DomainQuestionContext) error {
	var args []any
	bind := func(value any) int { args = append(args, value); return len(args) }
	domainsArg := fmt.Sprintf("$%d::text[]", bind(domains))
	readable, err := auth.ActivityContentClause(ctx, "m", bind)
	if err != nil {
		return err
	}
	visible, err := contactVisibleSQL(ctx, bind)
	if err != nil {
		return err
	}
	query := `
 WITH mail AS (
  SELECT q.domain, m.id, m.occurred_at, s.address, ce.contact_id
    FROM unnest(` + domainsArg + `) AS q(domain)
    JOIN activity m ON ` + vendorMailSQL("m", "q.domain", asOf, bind) + ` AND ` + readable + `
    JOIN activity_participant s ON s.activity_id = m.id AND s.role = 'from'
     AND s.address_domain_folded IS NOT NULL AND ` + owndomain.CoversIndexedSQL("s.address_domain_folded", "q.domain") + `
    LEFT JOIN contact_email ce ON ce.email = lower(s.address) AND ce.archived_at IS NULL)
 SELECT origin.domain, origin.id, origin.address, c.id, ` + visible + `, c.full_name, sender.last_at
   FROM (SELECT DISTINCT ON (domain) domain, id, address FROM mail ORDER BY domain, occurred_at DESC, id DESC) origin
   LEFT JOIN (SELECT domain, contact_id, max(occurred_at) AS last_at FROM mail WHERE contact_id IS NOT NULL GROUP BY domain, contact_id) sender
     ON sender.domain = origin.domain
   LEFT JOIN contact c ON c.id = sender.contact_id AND c.archived_at IS NULL AND c.merged_into_id IS NULL
  ORDER BY origin.domain, sender.last_at DESC NULLS LAST, c.id`
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("contacts: reading who raised the open company questions: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var domain, address string
		var origin ids.UUID
		var contactID *ids.ContactID
		var visible bool
		var name *string
		var lastAt *time.Time
		if err := rows.Scan(&domain, &origin, &address, &contactID, &visible, &name, &lastAt); err != nil {
			return fmt.Errorf("contacts: reading who raised the open company questions: %w", err)
		}
		one := out[domain]
		one.OriginID, one.SenderAddress = &origin, address
		if contactID != nil {
			one.ContactCount++
			if visible && name != nil {
				one.visibleSenders = append(one.visibleSenders, *contactID)
				if len(one.Contacts) < domainQuestionContactLimit {
					one.Contacts = append(one.Contacts, DomainQuestionContact{ID: *contactID, Name: *name})
				}
			}
		}
		one.HiddenSenders = one.ContactCount > 0 && len(one.Contacts) == 0
		out[domain] = one
	}
	return rows.Err()
}

// contactVisibleSQL is a boolean column saying whether the reader may see
// contact c. A seat without contact read sees none of them.
func contactVisibleSQL(ctx context.Context, bind func(any) int) (string, error) {
	if err := auth.Require(ctx, entityContact, principal.ActionRead); errors.Is(err, apperrors.ErrPermissionDenied) {
		return "false", nil
	} else if err != nil {
		return "", err
	}
	scope, err := auth.ScopeClauseFor(ctx, entityContact, "c", bind)
	if err != nil || scope == "" {
		return "(c.id IS NOT NULL)", err
	}
	return "(c.id IS NOT NULL AND " + scope + ")", nil
}

// suggestDomainQuestionCompaniesTx offers an existing company only when one
// visible, live company qualifies, and no second one does. Nothing is linked here: the
// reader still confirms it.
func suggestDomainQuestionCompaniesTx(ctx context.Context, tx pgx.Tx, evidence map[string]domainQuestionEvidence, out map[string]DomainQuestionContext) error {
	query, args, err := suggestionCandidatesSQL(ctx, evidence, out)
	if err != nil || query == "" {
		return err
	}
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("contacts: reading the companies an open question could suggest: %w", err)
	}
	defer rows.Close()
	candidates := map[string][]DomainQuestionSuggestion{}
	for rows.Next() {
		var domain string
		var one DomainQuestionSuggestion
		if err := rows.Scan(&domain, &one.CompanyID, &one.Name, &one.Basis); err != nil {
			return fmt.Errorf("contacts: reading the companies an open question could suggest: %w", err)
		}
		if !slices.ContainsFunc(candidates[domain], func(seen DomainQuestionSuggestion) bool { return seen.CompanyID == one.CompanyID }) {
			candidates[domain] = append(candidates[domain], one)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for domain, companies := range candidates {
		if len(companies) == 1 {
			one := out[domain]
			one.Suggestion = &companies[0]
			out[domain] = one
		}
	}
	return nil
}

// suggestionCandidatesSQL unions the three sources a suggestion may come
// from, strongest first, and keeps only live companies inside the reader's
// scope. A name twin offers every company carrying that name, so two of them
// leave no suggestion. An employer counts only while the employment is current. A hidden sender's employer is never a source: the suggestion would
// say where a contact the reader may not see works.
func suggestionCandidatesSQL(ctx context.Context, evidence map[string]domainQuestionEvidence, out map[string]DomainQuestionContext) (string, []any, error) {
	var twinDomains, targetDomains, targets, senderDomains []string
	var twins []ids.CompanyID
	var senders []ids.ContactID
	for domain, known := range evidence {
		if known.nameTwin != nil && known.reasonCode == PendingNearDuplicate {
			twinDomains, twins = append(twinDomains, domain), append(twins, *known.nameTwin)
		}
		if known.redirectTarget != "" && known.redirectTarget != domain {
			targetDomains, targets = append(targetDomains, domain), append(targets, owndomain.Fold(known.redirectTarget))
		}
	}
	if err := auth.Require(ctx, entityCompany, principal.ActionRead); err != nil {
		return "", nil, err
	}
	employers, err := mayReadEmployers(ctx)
	if err != nil {
		return "", nil, err
	}
	for domain, one := range out {
		for _, contact := range one.visibleSenders {
			if employers {
				senderDomains, senders = append(senderDomains, domain), append(senders, contact)
			}
		}
	}
	if len(twins)+len(targets)+len(senders) == 0 {
		return "", nil, nil
	}
	var args []any
	bind := func(value any) int { args = append(args, value); return len(args) }
	arg := func(value any, cast string) string { return fmt.Sprintf("$%d::%s", bind(value), cast) }
	twinName := exactNameKeySQL("twin.display_name")
	candidates := `SELECT q.domain, same.id AS company_id, '` + SuggestionNameTwin + `' AS basis, 1 AS strength
   FROM unnest(` + arg(twinDomains, "text[]") + `, ` + arg(twins, "uuid[]") + `) AS q(domain, company_id)
   JOIN company twin ON twin.id = q.company_id
   JOIN company same ON (` + exactNameKeySQL("same.display_name") + ` = ` + twinName + `
     OR ` + exactNameKeySQL("same.legal_name") + ` = ` + twinName + `)
 UNION ALL SELECT q.domain, cd.company_id, '` + SuggestionRedirectTarget + `', 2
   FROM unnest(` + arg(targetDomains, "text[]") + `, ` + arg(targets, "text[]") + `) AS q(domain, target)
   JOIN company_domain cd ON cd.domain_folded = q.target AND cd.archived_at IS NULL
 UNION ALL SELECT q.domain, r.company_id, '` + SuggestionContactEmployer + `', 3
   FROM unnest(` + arg(senderDomains, "text[]") + `, ` + arg(senders, "uuid[]") + `) AS q(domain, contact_id)
   JOIN relationship r ON r.contact_id = q.contact_id AND r.company_id IS NOT NULL
    AND r.kind = 'employment' AND r.archived_at IS NULL AND ` + employment.CurrentPrimarySQL("r")
	query := `SELECT cand.domain, co.id, co.display_name, cand.basis FROM (` + candidates + `) cand
 JOIN company co ON co.id = cand.company_id AND co.archived_at IS NULL AND co.merged_into_id IS NULL`
	scope, err := auth.ScopeClauseFor(ctx, entityCompany, "co", bind)
	if err != nil {
		return "", nil, err
	}
	if scope != "" {
		query += ` WHERE ` + scope
	}
	return query + ` ORDER BY cand.domain, cand.strength, co.id`, args, nil
}

// mayReadEmployers says whether the reader may read employment edges; without
// that grant an employer is not a source of suggestions.
func mayReadEmployers(ctx context.Context) (bool, error) {
	err := auth.Require(ctx, "relationship", principal.ActionRead)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return false, nil
	}
	return err == nil, err
}
