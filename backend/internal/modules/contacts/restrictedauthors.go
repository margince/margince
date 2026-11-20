// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Which chat authors a record must not quote. The chat engine names the
// addresses that posted in a channel and asks this before it files a summary.
// A restricted or erased subject's words then never reach the model or the note.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// restrictedAuthorsSQL answers which of the asked addresses belong to a
// contact the chat summary must withhold. A contact is withheld while it is
// archived, since erasure anonymizes in place and stamps `archived_at`, or
// while it holds any live stop. A live stop is one whose `revoked_at` is
// unset, so a new stop kind, such as a processing restriction, needs no edit.
const restrictedAuthorsSQL = `SELECT DISTINCT lower(contact_address.email)
  FROM contact_email contact_address
  JOIN contact subject ON subject.id = contact_address.contact_id
 WHERE lower(contact_address.email) = ANY($1)
   AND (subject.archived_at IS NOT NULL
        OR EXISTS (SELECT 1 FROM communication_suppression live_stop
                    WHERE live_stop.contact_id = contact_address.contact_id
                      AND live_stop.revoked_at IS NULL))`

// RestrictedEmailsTx reports which of emails belong to a contact the summary
// must withhold: one under a live stop, erased, or anonymized in place. The
// answer keys lowercased addresses, so a caller matches an author without
// caring how the chat cased it. An address with no such contact is absent
// from the map rather than false, which reads the same to the caller.
func (s *Store) RestrictedEmailsTx(ctx context.Context, tx pgx.Tx, emails []string) (map[string]bool, error) {
	lowered := make([]string, 0, len(emails))
	for _, email := range emails {
		lowered = append(lowered, strings.ToLower(strings.TrimSpace(email)))
	}
	rows, err := tx.Query(ctx, restrictedAuthorsSQL, lowered)
	if err != nil {
		return nil, fmt.Errorf("contacts: reading restricted chat authors: %w", err)
	}
	defer rows.Close()
	restricted := make(map[string]bool)
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, fmt.Errorf("contacts: scanning a restricted chat author: %w", err)
		}
		restricted[email] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("contacts: reading restricted chat authors: %w", err)
	}
	return restricted, nil
}
