// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// heldThreadPartiesShown is how many parties a held thread names; the rest are
// only counted.
const heldThreadPartiesShown = 5

// HeldThreadParticipant is one party outside the workspace on a held thread's
// opening message. ContactID is set only for a contact the reader may open.
type HeldThreadParticipant struct {
	Address   string
	ContactID *ids.UUID
}

// heldThreadPartiesTx names who each readable held thread is with.
//
// The parties are message content, so they are read through the content
// clause again. Not the ledger's seen addresses or the existence join: neither
// carries the gate. Colleagues are left out, because the question is who
// outside the workspace a held thread is with.
func heldThreadPartiesTx(ctx context.Context, tx pgx.Tx, threads []HeldThread) error {
	var messages []ids.UUID
	for _, t := range threads {
		if t.ActivityID != nil {
			messages = append(messages, *t.ActivityID)
		}
	}
	if len(messages) == 0 {
		return nil
	}
	var args []any
	arg := func(v any) int {
		args = append(args, v)
		return len(args)
	}
	content, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return err
	}
	opening := arg(messages)
	rows, err := tx.Query(ctx, `
		SELECT p.activity_id, p.address, p.contact_id
		  FROM activity_participant p
		  JOIN activity a ON a.id = p.activity_id AND a.archived_at IS NULL AND `+content+`
		 WHERE p.activity_id = ANY($`+fmt.Sprint(opening)+`)
		   AND p.address IS NOT NULL AND p.user_id IS NULL
		   AND NOT EXISTS (SELECT 1 FROM app_user u WHERE lower(u.email) = p.address)
		 ORDER BY p.activity_id,
		          CASE p.role WHEN 'from' THEN 0 WHEN 'to' THEN 1 WHEN 'cc' THEN 2 ELSE 3 END,
		          p.address`, args...)
	if err != nil {
		return fmt.Errorf("capture: reading who a seat's held threads are with: %w", err)
	}
	defer rows.Close()
	parties := map[ids.UUID][]HeldThreadParticipant{}
	for rows.Next() {
		var message ids.UUID
		var party HeldThreadParticipant
		if err := rows.Scan(&message, &party.Address, &party.ContactID); err != nil {
			return fmt.Errorf("capture: reading who a seat's held threads are with: %w", err)
		}
		parties[message] = appendParty(parties[message], party)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("capture: reading who a seat's held threads are with: %w", err)
	}
	rows.Close()
	for i, t := range threads {
		if t.ActivityID == nil {
			continue
		}
		all := parties[*t.ActivityID]
		threads[i].ParticipantCount = len(all)
		threads[i].Participants = all[:min(len(all), heldThreadPartiesShown)]
	}
	return withholdUnseenPartyContactsTx(ctx, tx, threads)
}

// appendParty keeps one entry per address: a party named as both To and Cc is
// one party on the thread.
func appendParty(parties []HeldThreadParticipant, party HeldThreadParticipant) []HeldThreadParticipant {
	for i, known := range parties {
		if known.Address == party.Address {
			if known.ContactID == nil {
				parties[i].ContactID = party.ContactID
			}
			return parties
		}
	}
	return append(parties, party)
}

// withholdUnseenPartyContactsTx drops every shown party's contact the reader
// may not open, one probe for the whole list.
func withholdUnseenPartyContactsTx(ctx context.Context, tx pgx.Tx, threads []HeldThread) error {
	var contacts []ids.UUID
	for _, t := range threads {
		for _, party := range t.Participants {
			if party.ContactID != nil {
				contacts = append(contacts, *party.ContactID)
			}
		}
	}
	if len(contacts) == 0 {
		return nil
	}
	visible, err := auth.VisibleSubset(ctx, tx, "contact", contacts)
	if err != nil {
		return err
	}
	for _, t := range threads {
		for j, party := range t.Participants {
			if party.ContactID != nil && !visible[*party.ContactID] {
				t.Participants[j].ContactID = nil
			}
		}
	}
	return nil
}
