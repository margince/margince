// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
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
}

// pickEmailTarget is the one rule for who an email about a row goes to:
//
//   - exactly one linked contact;
//   - else exactly one live linked lead (two live leads name nobody);
//   - else exactly one linked company;
//   - else a closed lead, refused, so the reader is told why.
//
// Leads and companies are weighed for a task only. A call, meeting or note is
// written about its contact, and its rule is unchanged.
func pickEmailTarget(kind crmcontracts.ActivityKind, found emailCandidates) (EmailTarget, bool) {
	if len(found.contacts) == 1 {
		contact := found.contacts[0]
		target := EmailTarget{EntityType: crmcontracts.ActivityEmailTargetEntityTypeContact, EntityID: contact.id, Address: contact.address}
		switch {
		case contact.closed:
			target.Address, target.Refusal = "", crmcontracts.ActivityEmailTargetRefusalArchived
		case contact.address == "":
			target.Refusal = crmcontracts.ActivityEmailTargetRefusalNoAddress
		}
		return target, true
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
		return EmailTarget{EntityType: crmcontracts.ActivityEmailTargetEntityTypeCompany, EntityID: found.companies[0].id}, true
	case len(found.companies) == 0 && len(closed) == 1:
		return EmailTarget{
			EntityType: crmcontracts.ActivityEmailTargetEntityTypeLead, EntityID: closed[0].id,
			Refusal: crmcontracts.ActivityEmailTargetRefusalLeadClosed,
		}, true
	}
	return EmailTarget{}, false
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

// linkedCandidates reads one kind of linked record per row, under that object's
// grant and row scope. A caller without the grant is told about none.
func linkedCandidates(
	ctx context.Context, tx pgx.Tx, object, alias, query string, rows []ids.UUID,
) (map[ids.UUID][]emailCandidate, error) {
	out := map[ids.UUID][]emailCandidate{}
	if len(rows) == 0 || !auth.ReadGranted(ctx, object) {
		return out, nil
	}
	args := []any{rows}
	scope, err := auth.ScopeClauseFor(ctx, object, alias, func(v any) int {
		args = append(args, v)
		return len(args)
	})
	if err != nil {
		return nil, err
	}
	if scope != "" {
		scope = `AND (` + scope + `)`
	}
	found, err := tx.Query(ctx, fmt.Sprintf(query, scope), args...)
	if err != nil {
		return nil, fmt.Errorf("activities: reading the %s each row is filed on: %w", object, err)
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
