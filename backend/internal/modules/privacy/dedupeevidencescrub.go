// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// emptyDedupeEvidence is the snapshot's own empty shape. The column is NOT NULL
// and the detector writes a JSON ARRAY of comparison rows, so a pair whose
// evidence has been destroyed carries no rows rather than no value.
const emptyDedupeEvidence = `[]`

// scrubDedupeEvidence destroys the naming snapshot on every duplicate pair that
// holds a subject the caller has just anonymized.
//
// dedupe_candidate.evidence is what made two records look alike, and
// contacts.DedupeEvidenceFields is the list of what that can be: full name,
// display name, email address, phone number, channel identity. Both privacy
// acts ANONYMIZE — the contact and lead rows survive with their columns nulled
// — so the ON DELETE CASCADE on the candidate's six subject columns never
// fires and the snapshot outlives the act unless it is named here.
//
// It is reachable and not merely resident. The queue's open lane requires both
// subjects live and so hides the pair, but the decided lanes do not, and
// GetDedupeCandidate filters on the candidate's OWN archived_at alone — so a
// pair disposed before the erasure keeps serving the subject's address and
// phone number to anyone who can name it.
//
// Contacts and leads in ONE statement because one erasure is both: the subject
// and the segregated lead twins they were promoted from, and a pair naming
// either end is the same snapshot of the same human.
//
// The ROW survives with its evidence emptied rather than being deleted:
// uq_dedupe_candidate_pair spans every disposition, so the row is what stops the
// detector proposing this pair again.
func scrubDedupeEvidence(ctx context.Context, tx pgx.Tx, contacts, leads []ids.UUID) error {
	if len(contacts) == 0 && len(leads) == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		UPDATE dedupe_candidate SET evidence = $3::jsonb
		 WHERE left_contact_id = ANY($1) OR right_contact_id = ANY($1)
		    OR left_lead_id = ANY($2) OR right_lead_id = ANY($2)`,
		contacts, leads, emptyDedupeEvidence); err != nil {
		return fmt.Errorf("privacy: destroying the subject's duplicate-pair evidence: %w", err)
	}
	return nil
}
