// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

// meetingWithQuery picks, per meeting, the first outside participant by name.
// Outside means no seat, no colleague and no employee of the own company.
// By name, because participant rows carry no reliable order of their own.
var meetingWithQuery = `
	SELECT DISTINCT ON (ap.activity_id) ap.activity_id, ap.contact_id
	  FROM activity_participant ap
	  JOIN activity a ON a.id = ap.activity_id AND a.kind = 'meeting' AND ` + activityLive + ` AND %s
	  JOIN contact c ON c.id = ap.contact_id AND c.archived_at IS NULL
	 WHERE ap.activity_id = ANY($%d) AND ap.user_id IS NULL AND NOT ` + calendarResourceParticipantSQL(scopeUnbounded) + ` AND %s
	   AND NOT %s
	   AND NOT EXISTS (
	       SELECT 1 FROM relationship emp
	         JOIN company co ON co.id = emp.company_id AND co.is_anchor AND %s
	        WHERE emp.contact_id = c.id AND emp.kind = 'employment' AND emp.archived_at IS NULL
	          AND %s AND %s)
	 ORDER BY ap.activity_id, c.full_name, c.id`

// MeetingCounterparties answers who each meeting was with, for this caller:
// the first outside participant, by name, whose contact the caller may see.
// A colleague is never outside (colleagueParticipantSQL). The own-company test
// reads only edges and companies the caller may read. A caller who may not read
// activities or contacts is told about nobody, so the page around them stands.
func (s *Store) MeetingCounterparties(ctx context.Context, meetingIDs []ids.UUID) (map[ids.UUID]ids.UUID, error) {
	out := make(map[ids.UUID]ids.UUID, len(meetingIDs))
	if len(meetingIDs) == 0 {
		return out, nil
	}
	for _, err := range []error{
		auth.Require(ctx, linkEntityActivity, principal.ActionRead),
		auth.Require(ctx, "contact", principal.ActionRead),
	} {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	meetings := arg(meetingIDs)
	activityScope, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	contactScope, err := auth.ScopeClauseFor(ctx, "contact", "c", arg)
	if err != nil {
		return nil, err
	}
	companyScope, edgeScope, err := ownCompanyTestScopes(ctx, arg)
	if err != nil {
		return nil, err
	}
	err = s.tx(ctx, func(tx pgx.Tx) error {
		proved, err := storekit.ProvedMailSeatIdentitiesSQL(ctx, tx, arg)
		if err != nil {
			return fmt.Errorf("activities: reading the seats' proved mailboxes: %w", err)
		}
		query := storekit.SQLf(meetingWithQuery, orUnbounded(activityScope), meetings, orUnbounded(contactScope),
			colleagueParticipantSQL(proved, scopeUnbounded), companyScope, employment.IsCurrentSQL("emp.ended_at"), edgeScope)
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("activities: reading who the meetings were with: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var meeting, contact ids.UUID
			if err := rows.Scan(&meeting, &contact); err != nil {
				return fmt.Errorf("activities: reading who a meeting was with: %w", err)
			}
			out[meeting] = contact
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ownCompanyTestScopes bounds the own-company test by what the caller may read:
// the company grant and row scope, and the employment edge's own admission. A
// caller who may read neither gets a test that finds nothing, so a meeting
// still names its first outside participant and no hidden edge decides which.
func ownCompanyTestScopes(ctx context.Context, arg func(any) int) (company, edge string, err error) {
	// Both grants are asked before either clause is built: a clause registers
	// its arguments, and one built for a test that then finds nothing would
	// leave them unbound in the statement.
	for _, err := range []error{auth.Require(ctx, "company", principal.ActionRead), auth.EdgeReadAdmitted(ctx)} {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return scopeNothing, scopeNothing, nil
		}
		if err != nil {
			return "", "", err
		}
	}
	if company, err = auth.ScopeClauseFor(ctx, "company", "co", arg); err != nil {
		return "", "", err
	}
	if edge, err = auth.EdgeReadScope(ctx, "emp", arg); err != nil {
		return "", "", err
	}
	return orUnbounded(company), orUnbounded(edge), nil
}

// orUnbounded reads an empty scope clause as the unbounded caller it means.
func orUnbounded(clause string) string {
	if clause == "" {
		return scopeUnbounded
	}
	return clause
}

// colleagueParticipantSQL holds when participant row ap is a colleague. The
// invited address decides: it is on a trusted own domain, is a live member's
// login, or is a mailbox proved for a seat. A row with no address falls back
// to its contact's live addresses, read through contactScope on alias cs.
// provedIdentities is the binding storekit.ProvedMailSeatIdentitiesSQL wrote;
// empty leaves that arm out. A row whose domain fold is missing is outside.
func colleagueParticipantSQL(provedIdentities, contactScope string) string {
	const address = "lower(ap.address)"
	ownAddress := owndomain.CoversSQL("ap.address_domain_folded", "own_domain.domain")
	seatAddress := storekit.SeatLoginIsSQL("u", address, "ap.address_domain_folded")
	ownContact := owndomain.CoversSQL("ce.email_domain_folded", "own_domain.domain")
	seatContact := storekit.SeatLoginIsSQL("u", "ce.email", "ce.email_domain_folded")
	if provedIdentities != "" {
		seatAddress = "(" + seatAddress + " OR (" + provedIdentities + " -> u.id::text) ? " + address + ")"
		seatContact = "(" + seatContact + " OR (" + provedIdentities + " -> u.id::text) ? ce.email)"
	}
	return fmt.Sprintf(`((ap.address IS NOT NULL
	     AND (EXISTS (SELECT 1 FROM (%[1]s) own_domain(domain) WHERE %[2]s)
	       OR EXISTS (SELECT 1 FROM app_user u WHERE %[3]s AND %[4]s)))
	   OR (ap.address IS NULL AND EXISTS (SELECT 1 FROM contact_email ce
	               JOIN contact cs ON cs.id = ce.contact_id AND %[7]s
	               WHERE ce.contact_id = ap.contact_id AND ce.archived_at IS NULL
	                 AND (EXISTS (SELECT 1 FROM (%[1]s) own_domain(domain) WHERE %[5]s)
	                   OR EXISTS (SELECT 1 FROM app_user u WHERE %[3]s AND %[6]s)))))`,
		auth.TrustedOwnDomainsFoldedSQL, ownAddress, workforce.LiveMemberSQL("u"), seatAddress, ownContact, seatContact, contactScope)
}

// Unknown guests keep a meeting in the customer lane; known resources cannot establish an outside party.
// Contact evidence is scoped to the reader.
var outsidePartySQL = `((NOT EXISTS (SELECT 1 FROM activity_participant roster
     WHERE roster.activity_id = a.id AND roster.address IS NOT NULL)
 AND NOT EXISTS (SELECT 1 FROM activity_participant ap
     WHERE ap.activity_id = a.id AND %[1]s))
 OR EXISTS (SELECT 1 FROM activity_participant ap
     WHERE ap.activity_id = a.id AND ap.user_id IS NULL AND NOT %[1]s AND NOT %[2]s
       AND NOT EXISTS (SELECT 1 FROM relationship emp
           JOIN company co ON co.id = emp.company_id AND co.is_anchor AND co.archived_at IS NULL AND %[3]s
           JOIN contact cs ON cs.id = emp.contact_id AND %[4]s
          WHERE emp.contact_id = ap.contact_id AND emp.kind = 'employment' AND emp.archived_at IS NULL
            AND %[5]s AND %[6]s)))`

// outsidePartyClause renders outsidePartySQL for this reader, inside the
// transaction that reads the seats' proved mailboxes.
func outsidePartyClause(ctx context.Context, tx pgx.Tx) func(arg func(any) int) (string, error) {
	return func(arg func(any) int) (string, error) {
		proved, err := storekit.ProvedMailSeatIdentitiesSQL(ctx, tx, arg)
		if err != nil {
			return "", fmt.Errorf("activities: reading the seats' proved mailboxes: %w", err)
		}
		contactScope, err := readableContactScope(ctx, "cs", arg)
		if err != nil {
			return "", err
		}
		companyScope, edgeScope, err := ownCompanyTestScopes(ctx, arg)
		if err != nil {
			return "", err
		}
		return storekit.SQLf(outsidePartySQL, calendarResourceParticipantSQL(contactScope), colleagueParticipantSQL(proved, contactScope),
			companyScope, contactScope, employment.IsCurrentSQL("emp.ended_at"), edgeScope), nil
	}
}

// readableContactScope is the contact row scope on alias, or a scope that
// finds nothing for a reader who may not read contacts.
func readableContactScope(ctx context.Context, alias string, arg func(any) int) (string, error) {
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return scopeNothing, nil
		}
		return "", err
	}
	scope, err := auth.ScopeClauseFor(ctx, "contact", alias, arg)
	if err != nil {
		return "", err
	}
	return orUnbounded(scope), nil
}
