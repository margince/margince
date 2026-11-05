// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// contactRolePrecedence is the order a contact's visible header roles are read
// in. A blind copy is not here: who was blind-copied is not disclosed by a row.
var contactRolePrecedence = []struct {
	stored string
	role   crmcontracts.ActivityVoiceContactRole
}{
	{roleFrom, crmcontracts.ActivityVoiceContactRoleAuthor},
	{roleTo, crmcontracts.ActivityVoiceContactRoleAddressee},
	{roleCc, crmcontracts.ActivityVoiceContactRoleCc},
	{"organizer", crmcontracts.ActivityVoiceContactRoleOrganizer},
	{"attendee", crmcontracts.ActivityVoiceContactRoleAttendee},
}

// WithContactRole sets `voice.contact_role` on a page read for one contact.
// Where that contact owes and is owed nothing, the email move goes back to
// `none`. Run it after WithEmailRowFacts, which fills the voice and the move.
// A row that records no header at all gets no role and keeps its move: mail of
// unknown provenance loses nothing without proof.
func WithContactRole(ctx context.Context, tx pgx.Tx, page []crmcontracts.Activity, contact ids.UUID) error {
	var voiced []ids.UUID
	for i := range page {
		if page[i].Voice != nil {
			voiced = append(voiced, ids.UUID(page[i].Id))
		}
	}
	if len(voiced) == 0 {
		return nil
	}
	rows, err := tx.Query(ctx, `
		SELECT ap.activity_id,
		       coalesce(array_agg(DISTINCT ap.role::text) FILTER (
		           WHERE `+relstrength.ParticipantNamesSQL("$2", "ap")+`), '{}'::text[])
		  FROM activity_participant ap
		 WHERE ap.activity_id = ANY($1)
		   AND ap.role IN ('from', 'to', 'cc', 'organizer', 'attendee')
		   AND NOT `+unstatedContactSQL("ap")+`
		 GROUP BY ap.activity_id`, voiced, contact)
	if err != nil {
		return fmt.Errorf("activities: reading the page contact's part on its rows: %w", err)
	}
	stored, err := pgx.CollectRows(rows, pgx.RowToStructByPos[struct {
		Activity ids.UUID
		Roles    []string
	}])
	if err != nil {
		return fmt.Errorf("activities: reading the page contact's part on its rows: %w", err)
	}
	byActivity := make(map[ids.UUID][]string, len(stored))
	for _, row := range stored {
		byActivity[row.Activity] = row.Roles
	}
	for i := range page {
		roles, recorded := byActivity[ids.UUID(page[i].Id)]
		if page[i].Voice == nil || !recorded {
			continue
		}
		role := contactRoleOf(roles)
		page[i].Voice.ContactRole = &role
		if summary := page[i].EmailSummary; summary != nil {
			summary.Move = moveForContact(role, summary.Move)
		}
	}
	return nil
}

// contactRoleOf is the strongest visible role among a contact's stored ones.
func contactRoleOf(stored []string) crmcontracts.ActivityVoiceContactRole {
	for _, candidate := range contactRolePrecedence {
		if slices.Contains(stored, candidate.stored) {
			return candidate.role
		}
	}
	return crmcontracts.ActivityVoiceContactRoleOther
}

// moveForContact keeps a message's move only on the page of a contact it is
// owed to or awaited from: its author or a direct addressee.
func moveForContact(role crmcontracts.ActivityVoiceContactRole, move crmcontracts.EmailSummaryMove) crmcontracts.EmailSummaryMove {
	switch role {
	case crmcontracts.ActivityVoiceContactRoleAuthor, crmcontracts.ActivityVoiceContactRoleAddressee:
		return move
	default:
		return crmcontracts.EmailSummaryMoveNone
	}
}
