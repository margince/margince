// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Choosing whose signature to read next.
//
// Its own file rather than a tail on enrichsignature.go, which holds the APPLY
// half. This one answers "who is owed a read", and its predicates are the
// pass's policy: who wrote the message, whose mailbox may be mined, and what a
// narrowed or held row costs. The two halves change for different reasons.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// signatureCandidateSQL selects the contacts whose latest mail this pass has not
// read. Its own declaration rather than a literal inside the query call: the
// predicates it carries are the pass's whole policy — who wrote the message,
// whose mailbox may be mined, what a narrowed or held row costs — and a
// function that holds them inline reads as plumbing rather than as the rules.
var signatureCandidateSQL = `
			SELECT p.id, p.full_name, coalesce(pe.email, ''), a.id, coalesce(a.body, '')
			FROM contact p
			LEFT JOIN contact_email pe ON pe.contact_id = p.id AND pe.is_primary AND pe.archived_at IS NULL
			JOIN LATERAL (
				SELECT a.id, a.body, a.occurred_at, a.captured_by
				FROM activity_link al
				JOIN activity a ON a.id = al.activity_id
				WHERE al.contact_id = p.id AND al.entity_type = 'contact'
				  AND a.kind = 'email' AND a.direction = 'inbound' AND a.archived_at IS NULL
				  -- THIS CONTACT WROTE IT. Reaching them is not writing it, and a
				  -- signature is only theirs to be read off a message they sent.
				  AND ` + SenderPredicate("p.id", "a") + `
				  -- A limited message is not signature material. What this pass
				  -- extracts — a title, a phone, an employer — is written onto a
				  -- contact every seat can read, so mining a message whose
				  -- audience excludes those seats republishes its content in
				  -- field form, and narrowing the mail afterwards does not take
				  -- the field back. The candidate simply waits for open mail.
				  AND a.audience = 'workspace'
				  -- A row held under a statutory obligation is out of reach of
				  -- every ordinary read, and the model call this feeds is
				  -- processing — which is exactly what such a hold bars.
				  AND a.restricted_at IS NULL
				ORDER BY a.occurred_at DESC
				LIMIT 1
			) a ON true
			LEFT JOIN contact_signature_enrich_state st ON st.contact_id = p.id
			-- Anyone the workspace still corresponds with, and no longer only
			-- the contacts whose details are missing. A contact changes jobs and
			-- numbers, so a filled field is a question this pass must keep
			-- asking; the emptiness test that used to live here would answer it
			-- once and never again. The watermark below is what keeps that
			-- affordable: it is arrival of NEW mail, not absence of an answer,
			-- that makes somebody a candidate.
			WHERE p.archived_at IS NULL AND p.merged_into_id IS NULL
			  AND (st.last_activity_at IS NULL OR a.occurred_at > st.last_activity_at)
			  -- The mailbox that captured THIS mail decides whether it may be
			  -- read for a signature, and the test is here rather than in the
			  -- Go loop so a switched-off mailbox never consumes a slot of the
			  -- pass's own limit. Gated on the activity, not on p.captured_by:
			  -- a contact captured by one mailbox is regularly last written to
			  -- from another, and it is the mail being read that matters.
			  --
			  -- The join is the provenance string capture stamps
			  -- (connector:<provider>:<user id>); there is no foreign key. A row
			  -- stamped with the bare connector:<name> form — no granting user
			  -- bound — matches no connection and follows the workspace default,
			  -- which is the same answer it had before this switch existed.
			  AND COALESCE((
				SELECT cc.signature_enrich_enabled
				  FROM capture_connection cc
				 WHERE ('connector:' || cc.provider || ':' || cc.user_id::text) = a.captured_by
				   AND cc.archived_at IS NULL
			  ), $2)
			-- Freshest mail first. The pass is capped, and it now runs within
			-- minutes of a message arriving, so ordering by contact age would
			-- put a contact who just wrote behind every older one still waiting
			-- — the contact whose details a rep is about to look at is the one
			-- who would wait longest.
			ORDER BY a.occurred_at DESC
			LIMIT $1`

// SignatureCandidates lists the contacts whose latest inbound mail this pass has
// not read yet, freshest first and capped at limit.
//
// The watermark, not the emptiness of a field, is what retires somebody: a
// contact changes jobs and numbers, so a filled title is a question worth
// asking again when they write. defaultEnabled is the workspace answer for a
// mailbox that never made its own choice.
func (s *Store) SignatureCandidates(ctx context.Context, limit int, defaultEnabled bool) ([]SignatureCandidate, error) {
	var out []SignatureCandidate
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, signatureCandidateSQL, limit, defaultEnabled)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c SignatureCandidate
			if err := rows.Scan(&c.ContactID, &c.FullName, &c.Email, &c.ActivityID, &c.Body); err != nil {
				return err
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("contacts: listing signature candidates: %w", err)
	}
	return out, nil
}
