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
const meetingWithQuery = `
	SELECT DISTINCT ON (ap.activity_id) ap.activity_id, ap.contact_id
	  FROM activity_participant ap
	  JOIN activity a ON a.id = ap.activity_id AND a.kind = 'meeting' AND ` + activityLive + ` AND %s
	  JOIN contact c ON c.id = ap.contact_id AND c.archived_at IS NULL
	 WHERE ap.activity_id = ANY($%d) AND ap.user_id IS NULL AND %s
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
			colleagueParticipantSQL(proved), companyScope, employment.IsCurrentSQL("emp.ended_at"), edgeScope)
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

// colleagueParticipantSQL holds when participant row ap is a colleague. Its
// address or a contact address is on a trusted own domain, is a live member's
// login, or is a mailbox proved for a seat. provedIdentities is the binding
// storekit.ProvedMailSeatIdentitiesSQL wrote; empty leaves that arm out.
// A row whose domain fold is missing counts as outside.
func colleagueParticipantSQL(provedIdentities string) string {
	const address = "lower(ap.address)"
	ownAddress := owndomain.CoversSQL("ap.address_domain_folded", "own_domain.domain")
	seatAddress := storekit.SeatLoginIsSQL("u", address, "ap.address_domain_folded")
	ownContact := owndomain.CoversSQL("ce.email_domain_folded", "own_domain.domain")
	seatContact := storekit.SeatLoginIsSQL("u", "ce.email", "ce.email_domain_folded")
	if provedIdentities != "" {
		seatAddress = "(" + seatAddress + " OR (" + provedIdentities + " -> u.id::text) ? " + address + ")"
		seatContact = "(" + seatContact + " OR (" + provedIdentities + " -> u.id::text) ? ce.email)"
	}
	return fmt.Sprintf(`(EXISTS (SELECT 1 FROM (%[1]s) own_domain(domain) WHERE %[2]s)
	   OR EXISTS (SELECT 1 FROM app_user u WHERE %[3]s AND %[4]s)
	   OR EXISTS (SELECT 1 FROM contact_email ce
	               WHERE ce.contact_id = ap.contact_id AND ce.archived_at IS NULL
	                 AND (EXISTS (SELECT 1 FROM (%[1]s) own_domain(domain) WHERE %[5]s)
	                   OR EXISTS (SELECT 1 FROM app_user u WHERE %[3]s AND %[6]s))))`,
		auth.TrustedOwnDomainsFoldedSQL, ownAddress, workforce.LiveMemberSQL("u"), seatAddress, ownContact, seatContact)
}

// meetingsWithoutOutsidePartyQuery selects the meetings with no outside
// participant. A participant with no contact counts as outside.
const meetingsWithoutOutsidePartyQuery = `
	SELECT a.id FROM activity a
	 WHERE a.id = ANY($%d) AND a.kind = 'meeting' AND ` + activityLive + ` AND %s
	   AND NOT EXISTS (
	       SELECT 1 FROM activity_participant ap
	        WHERE ap.activity_id = a.id AND ap.user_id IS NULL
	          AND NOT %s
	          AND NOT EXISTS (
	              SELECT 1 FROM relationship emp
	                JOIN company co ON co.id = emp.company_id AND co.is_anchor AND co.archived_at IS NULL
	               WHERE emp.contact_id = ap.contact_id AND emp.kind = 'employment' AND emp.archived_at IS NULL
	                 AND %s))`

// MeetingsWithoutOutsideParty answers which meetings have nobody from outside.
// Every participant is a seat, a colleague or an own-company employee.
// Contacts and edges are read without row scope, so a hidden guest never
// hides a customer meeting. Only the activity content gate applies.
// A reader who may not read activities is told nothing.
func (s *Store) MeetingsWithoutOutsideParty(ctx context.Context, meetingIDs []ids.UUID) (map[ids.UUID]bool, error) {
	out := make(map[ids.UUID]bool, len(meetingIDs))
	if len(meetingIDs) == 0 {
		return out, nil
	}
	if err := auth.Require(ctx, linkEntityActivity, principal.ActionRead); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return out, nil
		}
		return nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	meetings := arg(meetingIDs)
	activityScope, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	err = s.tx(ctx, func(tx pgx.Tx) error {
		proved, err := storekit.ProvedMailSeatIdentitiesSQL(ctx, tx, arg)
		if err != nil {
			return fmt.Errorf("activities: reading the seats' proved mailboxes: %w", err)
		}
		query := storekit.SQLf(meetingsWithoutOutsidePartyQuery, meetings, orUnbounded(activityScope),
			colleagueParticipantSQL(proved), employment.IsCurrentSQL("emp.ended_at"))
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("activities: reading which meetings have an outside party: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var meeting ids.UUID
			if err := rows.Scan(&meeting); err != nil {
				return fmt.Errorf("activities: reading a meeting without an outside party: %w", err)
			}
			out[meeting] = true
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
