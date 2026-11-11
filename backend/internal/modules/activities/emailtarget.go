// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/contactaddress"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// EmailTarget is the record an email about a written-about row goes to, and
// why it is refused when it is.
type EmailTarget struct {
	EntityType crmcontracts.ActivityEmailTargetEntityType
	EntityID   ids.UUID
	// Address fills To. Empty for a company, whose composer asks which of its
	// contacts the mail is for, and for a refused target.
	Address string
	// Name greets a lead, which has no contact record to read a name from.
	Name    string
	Refusal crmcontracts.ActivityEmailTargetRefusal
}

// offered says whether the target takes a draft.
func (t EmailTarget) offered() bool { return t.Refusal == "" }

// emailCandidate is one record a row is filed under, as the target rule reads it.
type emailCandidate struct {
	id      ids.UUID
	name    string
	address string
	// closed is an archived contact, or a lead that was promoted or disqualified.
	closed bool
}

// emailCandidates is everything one row is filed under that the rule weighs.
type emailCandidates struct {
	contacts, leads, companies []emailCandidate
	// dealContacts and dealCompanies are the live stakeholders and the
	// company of the deals a task is filed on. employers are the companies
	// its linked contacts work at now.
	dealContacts, dealCompanies, employers []emailCandidate
}

// pickEmailTarget is the one rule for who an email about a row goes to:
//
//   - exactly one linked contact;
//   - else exactly one live linked lead (two live leads name nobody);
//   - else exactly one linked company;
//   - else a closed lead, refused, so the reader is told why;
//   - else the one stakeholder of the task's deal, else that deal's company;
//   - else the one company the task's contacts work at.
//
// Leads, companies and deals are weighed for a task only. A call, meeting or
// note is written about its contact, and its rule is unchanged.
func pickEmailTarget(kind crmcontracts.ActivityKind, found emailCandidates) (EmailTarget, bool) {
	if len(found.contacts) == 1 {
		return contactTarget(found.contacts[0]), true
	}
	if kind != crmcontracts.ActivityKindTask {
		return EmailTarget{}, false
	}
	var live, closed []emailCandidate
	for _, lead := range found.leads {
		if lead.closed {
			closed = append(closed, lead)
		} else {
			live = append(live, lead)
		}
	}
	switch {
	case len(live) == 1:
		lead := live[0]
		target := EmailTarget{EntityType: crmcontracts.ActivityEmailTargetEntityTypeLead, EntityID: lead.id, Address: lead.address, Name: lead.name}
		if lead.address == "" {
			target.Refusal = crmcontracts.ActivityEmailTargetRefusalNoAddress
		}
		return target, true
	case len(live) > 1:
		return EmailTarget{}, false
	case len(found.companies) == 1:
		return companyTarget(found.companies[0]), true
	case len(found.companies) == 0 && len(closed) == 1:
		return EmailTarget{
			EntityType: crmcontracts.ActivityEmailTargetEntityTypeLead, EntityID: closed[0].id,
			Refusal: crmcontracts.ActivityEmailTargetRefusalLeadClosed,
		}, true
	}
	return inferredTarget(found)
}

// inferredTarget names a recipient for a task filed on no one it can write
// to. That is its deal's one stakeholder, else the deal's company, else the
// one company its contacts share. A company's composer asks which contact.
func inferredTarget(found emailCandidates) (EmailTarget, bool) {
	if len(found.companies) > 0 || len(found.leads) > 0 {
		return EmailTarget{}, false
	}
	switch {
	case len(found.contacts) == 0 && len(found.dealContacts) == 1:
		return contactTarget(found.dealContacts[0]), true
	case len(found.contacts) == 0 && len(found.dealCompanies) == 1:
		return companyTarget(found.dealCompanies[0]), true
	case len(found.contacts) > 1 && len(found.employers) == 1:
		return companyTarget(found.employers[0]), true
	}
	return EmailTarget{}, false
}

func contactTarget(contact emailCandidate) EmailTarget {
	target := EmailTarget{EntityType: crmcontracts.ActivityEmailTargetEntityTypeContact, EntityID: contact.id, Address: contact.address}
	switch {
	case contact.closed:
		target.Address, target.Refusal = "", crmcontracts.ActivityEmailTargetRefusalArchived
	case contact.address == "":
		target.Refusal = crmcontracts.ActivityEmailTargetRefusalNoAddress
	}
	return target
}

func companyTarget(company emailCandidate) EmailTarget {
	return EmailTarget{EntityType: crmcontracts.ActivityEmailTargetEntityTypeCompany, EntityID: company.id}
}

// leadCandidatesSQL reads the leads a task is filed under, under the lead scope.
const leadCandidatesSQL = `
	SELECT l.activity_id, ld.id, coalesce(ld.full_name, ''), coalesce(ld.email, ''), ld.archived_at IS NOT NULL
	  FROM activity_link l
	  JOIN lead ld ON ld.id = l.lead_id
	 WHERE l.activity_id = ANY($1) AND l.lead_id IS NOT NULL %s`

// companyCandidatesSQL reads the live companies a task is filed under, under
// the company scope.
const companyCandidatesSQL = `
	SELECT l.activity_id, co.id, '', '', false
	  FROM activity_link l
	  JOIN company co ON co.id = l.company_id AND co.archived_at IS NULL
	 WHERE l.activity_id = ANY($1) AND l.company_id IS NOT NULL %s`

// dealContactCandidatesSQL reads the live stakeholders of the deals a task is
// filed on, under the contact, deal and edge scopes.
const dealContactCandidatesSQL = `
	SELECT DISTINCT l.activity_id, p.id, p.full_name,
	       coalesce((SELECT pe.email FROM contact_email pe
	                  WHERE pe.contact_id = p.id AND pe.archived_at IS NULL` + contactaddress.ReachableOrder + `
	                  LIMIT 1), ''),
	       false
	  FROM activity_link l
	  JOIN deal d ON d.id = l.deal_id AND d.archived_at IS NULL
	  JOIN relationship r ON r.deal_id = d.id AND r.kind = 'deal_stakeholder' AND r.archived_at IS NULL
	  JOIN contact p ON p.id = r.contact_id AND p.archived_at IS NULL
	 WHERE l.activity_id = ANY($1) AND l.deal_id IS NOT NULL %s`

// dealCompanyCandidatesSQL reads the live companies of the deals a task is
// filed on, under the company and deal scopes.
const dealCompanyCandidatesSQL = `
	SELECT DISTINCT l.activity_id, co.id, ''::text, ''::text, false
	  FROM activity_link l
	  JOIN deal d ON d.id = l.deal_id AND d.archived_at IS NULL
	  JOIN company co ON co.id = d.company_id AND co.archived_at IS NULL
	 WHERE l.activity_id = ANY($1) AND l.deal_id IS NOT NULL %s`

// employerCandidatesSQL reads the live company every contact a task is filed
// on works at now, under the company and edge scopes. One contact with no
// visible job there leaves the task with no shared employer.
var employerCandidatesSQL = `
	SELECT l.activity_id, co.id, ''::text, ''::text, false
	  FROM activity_link l
	  JOIN relationship job ON job.contact_id = l.contact_id AND job.kind = 'employment'
	   AND job.archived_at IS NULL AND ` + employment.IsCurrentSQL("job.ended_at") + `
	  JOIN company co ON co.id = job.company_id AND co.archived_at IS NULL
	 WHERE l.activity_id = ANY($1) AND l.contact_id IS NOT NULL %s
	 GROUP BY l.activity_id, co.id
	HAVING count(DISTINCT l.contact_id) = (SELECT count(DISTINCT every.contact_id) FROM activity_link every
	                                         WHERE every.activity_id = l.activity_id AND every.contact_id IS NOT NULL)`

// candidateQuery is one read of the records a row may be written to. Every
// table it names is read under that object's grant and row scope.
type candidateQuery struct {
	query  string
	scopes []scopedTable
	// edge is the alias of the relationship table it reads, if any.
	edge string
}

type scopedTable struct{ object, alias string }

// linkedCandidates reads one kind of record per row. A caller without a grant
// for any table the read names is told about none.
func linkedCandidates(ctx context.Context, tx pgx.Tx, read candidateQuery, rows []ids.UUID) (map[ids.UUID][]emailCandidate, error) {
	out := map[ids.UUID][]emailCandidate{}
	if len(rows) == 0 {
		return out, nil
	}
	args := []any{rows}
	arg := func(v any) int {
		args = append(args, v)
		return len(args)
	}
	var clauses []string
	for _, table := range read.scopes {
		if !auth.ReadGranted(ctx, table.object) {
			return out, nil
		}
		scope, err := auth.ScopeClauseFor(ctx, table.object, table.alias, arg)
		if err != nil {
			return nil, err
		}
		clauses = append(clauses, scope)
	}
	if read.edge != "" {
		scope, err := auth.EdgeReadScope(ctx, read.edge, arg)
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		clauses = append(clauses, scope)
	}
	var where strings.Builder
	for _, clause := range clauses {
		if clause != "" {
			where.WriteString(` AND (` + clause + `)`)
		}
	}
	found, err := tx.Query(ctx, fmt.Sprintf(read.query, where.String()), args...)
	if err != nil {
		return nil, fmt.Errorf("activities: reading the %s each row is filed on: %w", read.scopes[0].object, err)
	}
	defer found.Close()
	for found.Next() {
		var row ids.UUID
		var candidate emailCandidate
		if err := found.Scan(&row, &candidate.id, &candidate.name, &candidate.address, &candidate.closed); err != nil {
			return nil, err
		}
		out[row] = append(out[row], candidate)
	}
	return out, found.Err()
}
