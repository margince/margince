// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Who raised an open company question and which company it may be about, as
// the reader of the question may see them. Every name here passes the row
// scope of its own record, so a question never names a contact or company the
// reader could not open.

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/freemail"
	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// domainQuestionContactLimit bounds the names one question shows; the count
// says how many more there are.
const domainQuestionContactLimit = 3

// The evidence a suggested company rests on, spelled as the contract does.
const (
	SuggestionRedirectTarget  = string(crmcontracts.DomainCompanySuggestionBasisRedirectTarget)
	SuggestionContactEmployer = string(crmcontracts.DomainCompanySuggestionBasisContactEmployer)
	SuggestionNameTwin        = string(crmcontracts.DomainCompanySuggestionBasisNameTwin)
)

// DomainQuestionContact is one sender of the question's mail the reader may see.
type DomainQuestionContact struct {
	ID   ids.UUID
	Name string
}

// DomainCompanySuggestion is the one existing company the evidence points to.
type DomainCompanySuggestion struct {
	CompanyID ids.UUID
	Name      string
	Basis     string
}

// DomainQuestionContext explains a question to its reader.
type DomainQuestionContext struct {
	// Senders of the newest mail first, at most domainQuestionContactLimit.
	Contacts []DomainQuestionContact
	// Live contacts on the domain the reader may see, which a confirmed
	// company also employs.
	ContactCount int
	// The mail came from contacts the reader may not see, and none they may.
	HiddenSenders bool
	// The From address of the newest mail, which the reader may read.
	SenderAddress    string
	OriginActivityID *ids.UUID
	Suggestion       *DomainCompanySuggestion
	// No read of the site succeeded or was allowed (unevidenced only).
	SiteUnread bool
}

type domainSuggestionCandidate struct {
	company ids.UUID
	basis   string
}

// attachDomainQuestionContextTx fills Context on every question in one read
// per kind of evidence, so a page of questions costs a fixed number of queries.
func attachDomainQuestionContextTx(ctx context.Context, tx pgx.Tx, questions []DomainQuestion, candidates map[string]ids.UUID, asOf time.Time) error {
	if len(questions) == 0 {
		return nil
	}
	domains := make([]string, 0, len(questions))
	for _, q := range questions {
		domains = append(domains, q.Domain)
	}
	senders, err := readDomainSendersTx(ctx, tx, domains, asOf)
	if err != nil {
		return err
	}
	counts, err := countVisibleDomainContactsTx(ctx, tx, domains)
	if err != nil {
		return err
	}
	offered := map[string][]domainSuggestionCandidate{}
	if err := redirectTargetCandidatesTx(ctx, tx, questions, offered); err != nil {
		return err
	}
	if err := employerCandidatesTx(ctx, tx, senders, offered); err != nil {
		return err
	}
	for domain, company := range candidates {
		offered[domain] = append(offered[domain], domainSuggestionCandidate{company: company, basis: SuggestionNameTwin})
	}
	names, err := visibleCompanyNamesTx(ctx, tx, offered)
	if err != nil {
		return err
	}
	for i := range questions {
		q := &questions[i]
		q.Context.ContactCount = counts[q.Domain]
		senders[q.Domain].fill(&q.Context)
		q.Context.Suggestion = soleSuggestion(offered[q.Domain], names)
	}
	return nil
}

// soleSuggestion names a company only when every source agrees on one and the
// reader may see it. Two candidates are a choice for the reader, never a guess.
func soleSuggestion(offered []domainSuggestionCandidate, names map[ids.UUID]string) *DomainCompanySuggestion {
	if len(offered) == 0 {
		return nil
	}
	first := offered[0]
	for _, c := range offered[1:] {
		if c.company != first.company {
			return nil
		}
	}
	name, visible := names[first.company]
	if !visible {
		return nil
	}
	return &DomainCompanySuggestion{CompanyID: first.company, Name: name, Basis: first.basis}
}

type domainSender struct {
	activity ids.UUID
	address  string
	contact  *ids.UUID
	name     string
	visible  bool
}

type domainSenders []domainSender

func (s domainSenders) fill(out *DomainQuestionContext) {
	if len(s) == 0 {
		return
	}
	origin := s[0].activity
	out.OriginActivityID, out.SenderAddress = &origin, s[0].address
	seen := map[ids.UUID]bool{}
	hidden := false
	for _, sender := range s {
		if sender.contact == nil || seen[*sender.contact] {
			continue
		}
		seen[*sender.contact] = true
		if !sender.visible {
			hidden = true
			continue
		}
		if len(out.Contacts) < domainQuestionContactLimit {
			out.Contacts = append(out.Contacts, DomainQuestionContact{ID: *sender.contact, Name: sender.name})
		}
	}
	out.HiddenSenders = hidden && len(out.Contacts) == 0
}

// readDomainSendersTx reads the senders of each domain's newest question mail
// the reader may read, newest first. The mail set is vendorMailSQL's, the one
// the decider of the question is chosen from.
func readDomainSendersTx(ctx context.Context, tx pgx.Tx, domains []string, asOf time.Time) (map[string]domainSenders, error) {
	out := map[string]domainSenders{}
	if err := auth.Require(ctx, "activity", principal.ActionRead); errors.Is(err, apperrors.ErrPermissionDenied) {
		return out, nil
	} else if err != nil {
		return nil, err
	}
	var args []any
	bind := func(value any) int { args = append(args, value); return len(args) }
	domainList := fmt.Sprintf("$%d::text[]", bind(domains))
	readable, err := auth.ActivityContentClause(ctx, "question_mail", bind)
	if err != nil {
		return nil, err
	}
	visible, err := visibleContactSQL(ctx, "c", bind)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT q.domain, m.id, sender.address, c.id, COALESCE(c.full_name, ''), COALESCE(`+visible+`, false)
 FROM unnest(`+domainList+`) AS q(domain)
 CROSS JOIN LATERAL (SELECT question_mail.id, question_mail.occurred_at FROM activity question_mail
  WHERE `+vendorMailSQL("question_mail", "q.domain", asOf, bind)+` AND `+readable+`
  ORDER BY question_mail.occurred_at DESC, question_mail.id DESC LIMIT 20) m
 JOIN activity_participant sender ON sender.activity_id = m.id AND sender.role = 'from'
  AND sender.address_domain_folded IS NOT NULL AND `+owndomain.CoversSQL("sender.address_domain_folded", "q.domain")+`
 LEFT JOIN contact_email ce ON ce.email = lower(sender.address) AND ce.archived_at IS NULL
 LEFT JOIN contact c ON c.id = ce.contact_id AND c.archived_at IS NULL AND c.merged_into_id IS NULL
 ORDER BY q.domain, m.occurred_at DESC, m.id DESC`, args...)
	if err != nil {
		return nil, fmt.Errorf("contacts: reading who raised the domain questions: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var domain string
		var s domainSender
		if err := rows.Scan(&domain, &s.activity, &s.address, &s.contact, &s.name, &s.visible); err != nil {
			return nil, fmt.Errorf("contacts: reading who raised the domain questions: %w", err)
		}
		out[domain] = append(out[domain], s)
	}
	return out, rows.Err()
}

// visibleContactSQL is the row scope of a contact aliased alias, or false
// when the reader may not read contacts at all.
func visibleContactSQL(ctx context.Context, alias string, bind func(any) int) (string, error) {
	if err := auth.Require(ctx, contactEntity, principal.ActionRead); errors.Is(err, apperrors.ErrPermissionDenied) {
		return "false", nil
	} else if err != nil {
		return "", err
	}
	scope, err := auth.ScopeClauseFor(ctx, contactEntity, alias, bind)
	if err != nil || scope == "" {
		return scopeAllRows, err
	}
	return "(" + scope + ")", nil
}

func countVisibleDomainContactsTx(ctx context.Context, tx pgx.Tx, domains []string) (map[string]int, error) {
	var args []any
	bind := func(value any) int { args = append(args, value); return len(args) }
	domainList := fmt.Sprintf("$%d::text[]", bind(domains))
	visible, err := visibleContactSQL(ctx, "p", bind)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT q.domain, count(DISTINCT p.id)
 FROM unnest(`+domainList+`) AS q(domain)
 JOIN contact_email pe ON pe.archived_at IS NULL AND `+owndomain.CoversIndexedSQL("pe.email_domain_folded", "q.domain")+`
 JOIN contact p ON p.id = pe.contact_id AND p.archived_at IS NULL AND p.merged_into_id IS NULL
 WHERE `+visible+`
 GROUP BY q.domain`, args...)
	if err != nil {
		return nil, fmt.Errorf("contacts: counting the contacts on the domain questions: %w", err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var domain string
		var n int
		if err := rows.Scan(&domain, &n); err != nil {
			return nil, fmt.Errorf("contacts: counting the contacts on the domain questions: %w", err)
		}
		out[domain] = n
	}
	return out, rows.Err()
}

// redirectTargetCandidatesTx offers the company that already owns the domain a
// redirected site answers on. The redirect alone never links anything: the
// reader still confirms.
func redirectTargetCandidatesTx(ctx context.Context, tx pgx.Tx, questions []DomainQuestion, offered map[string][]domainSuggestionCandidate) error {
	byTarget := map[string][]string{}
	for _, q := range questions {
		if q.Forward == nil {
			continue
		}
		effective, err := url.Parse(q.Forward.EffectiveURL)
		if err != nil {
			continue
		}
		target, ok := freemail.Hostname(effective.Hostname())
		if !ok || target == q.Domain {
			continue
		}
		byTarget[target] = append(byTarget[target], q.Domain)
	}
	if len(byTarget) == 0 {
		return nil
	}
	targets := make([]string, 0, len(byTarget))
	for target := range byTarget {
		targets = append(targets, target)
	}
	rows, err := tx.Query(ctx, `SELECT DISTINCT cd.domain_folded, cd.company_id FROM company_domain cd
 WHERE cd.domain_folded = ANY($1) AND cd.archived_at IS NULL`, targets)
	if err != nil {
		return fmt.Errorf("contacts: reading the companies on redirect targets: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var target string
		var company ids.UUID
		if err := rows.Scan(&target, &company); err != nil {
			return fmt.Errorf("contacts: reading the companies on redirect targets: %w", err)
		}
		for _, domain := range byTarget[target] {
			offered[domain] = append(offered[domain], domainSuggestionCandidate{company: company, basis: SuggestionRedirectTarget})
		}
	}
	return rows.Err()
}

// employerCandidatesTx offers the current employer of each sender the reader
// may see, through the same gated read every contact page names an employer by.
func employerCandidatesTx(ctx context.Context, tx pgx.Tx, senders map[string]domainSenders, offered map[string][]domainSuggestionCandidate) error {
	var contactIDs []ids.UUID
	for _, list := range senders {
		for _, s := range list {
			if s.contact != nil && s.visible {
				contactIDs = append(contactIDs, *s.contact)
			}
		}
	}
	employers, err := readContactEmployers(ctx, tx, contactIDs)
	if err != nil {
		return err
	}
	for domain, list := range senders {
		for _, s := range list {
			if s.contact == nil || !s.visible {
				continue
			}
			if employer, ok := employers[*s.contact]; ok {
				offered[domain] = append(offered[domain], domainSuggestionCandidate{company: ids.UUID(employer.CompanyId), basis: SuggestionContactEmployer})
			}
		}
	}
	return nil
}

// visibleCompanyNamesTx names the offered companies the reader may see.
func visibleCompanyNamesTx(ctx context.Context, tx pgx.Tx, offered map[string][]domainSuggestionCandidate) (map[ids.UUID]string, error) {
	var companyIDs []ids.UUID
	for _, list := range offered {
		for _, c := range list {
			companyIDs = append(companyIDs, c.company)
		}
	}
	out := map[ids.UUID]string{}
	if len(companyIDs) == 0 {
		return out, nil
	}
	var args []any
	bind := func(value any) int { args = append(args, value); return len(args) }
	idList := fmt.Sprintf("$%d", bind(companyIDs))
	scope, err := auth.ScopeClauseFor(ctx, companyEntity, "company", bind)
	if err != nil {
		return nil, err
	}
	if scope == "" {
		scope = scopeAllRows
	}
	rows, err := tx.Query(ctx, `SELECT company.id, company.display_name FROM company company
 WHERE company.id = ANY(`+idList+`) AND company.archived_at IS NULL AND company.merged_into_id IS NULL AND `+scope, args...)
	if err != nil {
		return nil, fmt.Errorf("contacts: naming the suggested companies: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id ids.UUID
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("contacts: naming the suggested companies: %w", err)
		}
		out[id] = name
	}
	return out, rows.Err()
}

// Contract spells the context on the wire, for the review list and the
// worklist row alike.
func (c DomainQuestionContext) Contract() crmcontracts.DomainQuestionContext {
	out := crmcontracts.DomainQuestionContext{
		Contacts:      make([]crmcontracts.DomainQuestionContact, 0, len(c.Contacts)),
		ContactCount:  c.ContactCount,
		HiddenSenders: c.HiddenSenders,
		SiteUnread:    c.SiteUnread,
	}
	for _, contact := range c.Contacts {
		out.Contacts = append(out.Contacts, crmcontracts.DomainQuestionContact{Id: openapi_types.UUID(contact.ID), Name: contact.Name})
	}
	if c.SenderAddress != "" {
		out.SenderAddress = &c.SenderAddress
	}
	if c.OriginActivityID != nil {
		origin := openapi_types.UUID(*c.OriginActivityID)
		out.OriginActivityId = &origin
	}
	if c.Suggestion != nil {
		out.SuggestedCompany = &crmcontracts.DomainCompanySuggestion{
			Id:    openapi_types.UUID(c.Suggestion.CompanyID),
			Name:  c.Suggestion.Name,
			Basis: crmcontracts.DomainCompanySuggestionBasis(c.Suggestion.Basis),
		}
	}
	return out
}
