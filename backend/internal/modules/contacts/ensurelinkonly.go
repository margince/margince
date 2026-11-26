// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
)

// MemberAuthority reads one member's authority inside a transaction.
type MemberAuthority func(context.Context, pgx.Tx, ids.UUID) (authz.RBAC, principal.SeatType, error)

// linkExistingCounterparty files a captured activity under the contact or lead
// an address already names, and writes no record of its own. A mailbox no
// seat owns takes this path. named reports that a record holds the address, so
// a backup owner's mint must not make a second one.
func (s *Store) linkExistingCounterparty(ctx context.Context, tx pgx.Tx, in EnsureCounterpartyInput) (res EnsureCounterpartyResult, named bool, err error) {
	if err := auth.Require(ctx, entityContact, principal.ActionRead); err != nil {
		return EnsureCounterpartyResult{}, false, err
	}
	match, lead, err := resolveCapturedContact(ctx, tx, ContactCandidate{Emails: []string{in.Email}}, in.Email)
	if err != nil {
		return EnsureCounterpartyResult{}, false, err
	}
	// The mailbox's own principal reads every row, so each link asks the
	// readers instead: a record none of them may read gets no shared mail.
	if !lead.IsZero() {
		// Filed on the lead, without the qualify task a personal reply files.
		readable, err := readableByATeamMemberTx(ctx, tx, in, leadEntity, lead.UUID)
		if err != nil || !readable {
			return EnsureCounterpartyResult{}, true, err
		}
		return EnsureCounterpartyResult{LeadID: lead}, true, linkActivityToLead(ctx, tx, in.ActivityID, lead)
	}
	if match.Decision != DecisionExactCollision {
		return EnsureCounterpartyResult{}, false, nil
	}
	readable, err := readableByATeamMemberTx(ctx, tx, in, entityContact, match.ContactID.UUID)
	if err != nil || !readable {
		return EnsureCounterpartyResult{}, true, err
	}
	res = EnsureCounterpartyResult{ContactID: match.ContactID}
	return res, true, s.linkActivityToContact(ctx, tx, in.ActivityID, res.ContactID)
}

// readableByATeamMemberTx answers whether a live member of in's reader team
// may read the record under that member's own role and the table's visibility
// rule. entity names both the object and its table: contact or lead.
func readableByATeamMemberTx(ctx context.Context, tx pgx.Tx, in EnsureCounterpartyInput, entity string, record ids.UUID) (bool, error) {
	if in.ReaderTeam.IsZero() || in.ReaderAuthority == nil {
		return false, nil
	}
	rows, err := tx.Query(ctx, `SELECT m.user_id FROM team_membership m
	  JOIN app_user u ON u.id = m.user_id AND `+workforce.LiveMemberSQL("u")+`
	 WHERE m.team_id = $1 ORDER BY m.user_id`, in.ReaderTeam)
	if err != nil {
		return false, fmt.Errorf("contacts: reading the reader team: %w", err)
	}
	members, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return false, fmt.Errorf("contacts: reading the reader team: %w", err)
	}
	for _, member := range members {
		visible, err := memberMayReadTx(ctx, tx, in.ReaderAuthority, member, entity, record)
		if err != nil || visible {
			return visible, err
		}
	}
	return false, nil
}

// memberReadSQL asks whether the row is visible, spelled per table so each
// read stays a literal its gates can see.
var memberReadSQL = map[string]string{
	entityContact: `SELECT EXISTS (SELECT 1 FROM contact r WHERE r.id = $1 AND %s)`,
	leadEntity:    `SELECT EXISTS (SELECT 1 FROM lead r WHERE r.id = $1 AND %s)`,
}

// memberMayReadTx asks one member's role for read on entity, then the
// table's visibility rule for the row.
func memberMayReadTx(ctx context.Context, tx pgx.Tx, authority MemberAuthority, member ids.UUID, entity string, record ids.UUID) (bool, error) {
	rbac, seat, err := authority(ctx, tx, member)
	if errors.Is(err, apperrors.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !rbac.Permissions.Allows(entity, principal.ActionRead) {
		return false, nil
	}
	reader := authz.HumanPrincipal(member, rbac, seat)
	statement, known := memberReadSQL[entity]
	if !known {
		return false, fmt.Errorf("contacts: no reader check for %s", entity)
	}
	args := []any{record}
	clause := auth.VisiblePredicate(reader, entity, func(v any) int { args = append(args, v); return len(args) })("r")
	var visible bool
	if err := tx.QueryRow(ctx, fmt.Sprintf(statement, clause), args...).Scan(&visible); err != nil {
		return false, fmt.Errorf("contacts: asking whether a reader may read the %s: %w", entity, err)
	}
	return visible, nil
}
