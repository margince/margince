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
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// CommitmentTaskSource marks a task written for a commitment one of our users
// made in a captured conversation.
const CommitmentTaskSource = provenance.CommitmentTaskSource

// CommitmentTaskWritten reports whether a commitment's task was ever written,
// archived and completed ones included. An archived task is how a rep says
// "not mine to do", so the answer must outlive the archive or the next reading
// of the same conversation would write it again.
func (s *Store) CommitmentTaskWritten(ctx context.Context, tx pgx.Tx, locator string) (bool, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return false, err
	}
	var written bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM activity
		                WHERE source_system = $1 AND source_id = $2)`,
		CommitmentTaskSource, locator).Scan(&written); err != nil {
		return false, fmt.Errorf("activities: reading whether the commitment's task exists: %w", err)
	}
	return written, nil
}

// VisibleOnlyTo narrows a new activity's audience to one member. A task read
// out of mail only one member may read must not be readable by anybody else
// through the records it is filed against.
func (in *LogActivityInput) VisibleOnlyTo(user ids.UUID) {
	in.audienceMembers = []AudienceMember{{
		SubjectType: string(crmcontracts.AudienceMemberSubjectTypeUser), SubjectID: user,
	}}
}

// MessageParties is who wrote one message and who it went to, as capture
// recorded them.
type MessageParties struct {
	// SenderSeats are the members recorded as its sender. Capture binds a seat
	// only from a party list the mailbox's own connection attested.
	SenderSeats []ids.UUID
	// SenderContacts and RecipientContacts are the contacts on either side.
	SenderContacts    []ids.UUID
	RecipientContacts []ids.UUID
}

// PartiesOf reads who wrote one message and who it went to.
func (s *Store) PartiesOf(ctx context.Context, tx pgx.Tx, message ids.UUID) (MessageParties, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return MessageParties{}, err
	}
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return MessageParties{}, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	messagePos := arg(message)
	// A contact the caller may not see is not named back to them.
	scope, err := auth.ScopeClauseFor(ctx, "contact", "c", arg)
	if err != nil {
		return MessageParties{}, err
	}
	if scope == "" {
		scope = "TRUE"
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT p.role = 'from', p.user_id, p.contact_id FROM activity_participant p
		  LEFT JOIN contact c ON c.id = p.contact_id
		 WHERE p.activity_id = $%d AND p.role IN ('from', 'to', 'cc')
		   AND (p.user_id IS NOT NULL OR (p.contact_id IS NOT NULL AND (%s)))
		 ORDER BY p.id`, messagePos, scope), args...)
	if err != nil {
		return MessageParties{}, fmt.Errorf("activities: reading a message's parties: %w", err)
	}
	defer rows.Close()
	var out MessageParties
	for rows.Next() {
		var sender bool
		var seat, contact *ids.UUID
		if err := rows.Scan(&sender, &seat, &contact); err != nil {
			return MessageParties{}, fmt.Errorf("activities: reading a message's parties: %w", err)
		}
		switch {
		case sender && seat != nil:
			out.SenderSeats = append(out.SenderSeats, *seat)
		case sender && contact != nil:
			out.SenderContacts = append(out.SenderContacts, *contact)
		case contact != nil:
			out.RecipientContacts = append(out.RecipientContacts, *contact)
		}
	}
	return out, rows.Err()
}
