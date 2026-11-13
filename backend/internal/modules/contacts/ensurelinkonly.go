// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

// linkExistingCounterparty files a captured activity under the contact or lead
// an address already names, and writes no record of its own. A mailbox no
// seat owns takes this path: it may file mail, never mint a contact, an
// employer or a task.
func (s *Store) linkExistingCounterparty(ctx context.Context, tx pgx.Tx, in EnsureCounterpartyInput) (EnsureCounterpartyResult, error) {
	if err := auth.Require(ctx, entityContact, principal.ActionRead); err != nil {
		return EnsureCounterpartyResult{}, err
	}
	match, lead, err := resolveCapturedContact(ctx, tx, ContactCandidate{Emails: []string{in.Email}}, in.Email)
	if err != nil {
		return EnsureCounterpartyResult{}, err
	}
	if !lead.IsZero() {
		// Filed on the lead, without the qualify task a personal reply files.
		allowed, err := mayReadLead(ctx)
		if err != nil || !allowed {
			return EnsureCounterpartyResult{}, err
		}
		return EnsureCounterpartyResult{LeadID: lead}, linkActivityToLead(ctx, tx, in.ActivityID, lead)
	}
	if match.Decision != DecisionExactCollision {
		return EnsureCounterpartyResult{}, nil
	}
	// The mailbox's own principal reads every row, so the link asks the
	// readers instead: a contact none of them may read gets no shared mail.
	readable, err := readableByATeamMemberTx(ctx, tx, in.ReaderTeam, match.ContactID)
	if err != nil || !readable {
		return EnsureCounterpartyResult{}, err
	}
	res := EnsureCounterpartyResult{ContactID: match.ContactID}
	return res, s.linkActivityToContact(ctx, tx, in.ActivityID, res.ContactID)
}

// readableByATeamMemberTx answers whether a live member of team may read the
// contact under the contact table's own visibility rule. Contacts are read by
// every seat, so only the member's id and teams decide it: capture privacy and
// a manual grant.
func readableByATeamMemberTx(ctx context.Context, tx pgx.Tx, team ids.UUID, contact ids.ContactID) (bool, error) {
	if team.IsZero() {
		return false, nil
	}
	rows, err := tx.Query(ctx, `SELECT m.user_id, array_agg(other.team_id)
	  FROM team_membership m
	  JOIN app_user u ON u.id = m.user_id AND `+workforce.LiveMemberSQL("u")+`
	  JOIN team_membership other ON other.user_id = m.user_id
	 WHERE m.team_id = $1 GROUP BY m.user_id ORDER BY m.user_id`, team)
	if err != nil {
		return false, fmt.Errorf("contacts: reading the reader team: %w", err)
	}
	type member struct {
		user  ids.UUID
		teams []ids.UUID
	}
	members, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (member, error) {
		var m member
		return m, row.Scan(&m.user, &m.teams)
	})
	if err != nil {
		return false, fmt.Errorf("contacts: reading the reader team: %w", err)
	}
	for _, m := range members {
		reader := principal.Principal{Type: principal.PrincipalHuman, ID: "human:" + m.user.String(), UserID: m.user, TeamIDs: m.teams}
		args := []any{contact.UUID}
		clause := auth.VisiblePredicate(reader, entityContact, func(v any) int { args = append(args, v); return len(args) })("c")
		var visible bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM contact c WHERE c.id = $1 AND `+clause+`)`, args...).Scan(&visible); err != nil {
			return false, fmt.Errorf("contacts: asking whether a reader may read the contact: %w", err)
		}
		if visible {
			return true, nil
		}
	}
	return false, nil
}
