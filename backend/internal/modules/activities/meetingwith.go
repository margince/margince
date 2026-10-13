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
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// meetingWithQuery picks, per meeting, the first participant by name who holds
// no seat and is not employed by the installation's own company: the same
// "outside" HeldMeetingCounterparties means, without needing an employer. By
// name, because participant rows carry no reliable order of their own, and two
// reads of an unchanged meeting must name the same contact.
const meetingWithQuery = `
	SELECT DISTINCT ON (ap.activity_id) ap.activity_id, ap.contact_id
	  FROM activity_participant ap
	  JOIN activity a ON a.id = ap.activity_id AND a.kind = 'meeting' AND ` + activityLive + ` AND %s
	  JOIN contact c ON c.id = ap.contact_id AND c.archived_at IS NULL
	 WHERE ap.activity_id = ANY($%d) AND ap.user_id IS NULL AND %s
	   AND NOT EXISTS (
	       SELECT 1 FROM relationship emp
	         JOIN company co ON co.id = emp.company_id AND co.is_anchor AND %s
	        WHERE emp.contact_id = c.id AND emp.kind = 'employment' AND emp.archived_at IS NULL
	          AND %s AND %s)
	 ORDER BY ap.activity_id, c.full_name, c.id`

// MeetingCounterparties answers who each of these meetings was with, for THIS
// caller: the first outside participant, by name, who is a contact the caller
// may see. A meeting with no such participant is absent from the map. The
// own-company test reads only employment edges and companies the caller may
// read, so which contact is named discloses no edge they may not see.
//
// Participants and not activity links: a link says what a meeting is filed
// under, and a meeting filed under a deal or a colleague's record is still with
// the customer who sat in it. A caller who may not read activities or contacts
// is told about nobody rather than refused, so the page around them stands.
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
	query := storekit.SQLf(meetingWithQuery, orUnbounded(activityScope), meetings, orUnbounded(contactScope),
		companyScope, employment.IsCurrentSQL("emp.ended_at"), edgeScope)
	err = s.tx(ctx, func(tx pgx.Tx) error {
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
