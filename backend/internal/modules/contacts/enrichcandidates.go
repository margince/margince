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
				  -- Whose mail may be mined for whom: SignatureSourceEligible
				  -- states the rule, and the apply asks the same fragment.
				  AND ` + SignatureSourceEligible("p", "a") + `
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

// SignatureSourceEligible is the ONE spelling of "may this message be mined for
// this contact's signature", given the contact's alias and the activity's.
//
// It exists as a fragment because TWO statements must agree: the candidate
// query that SELECTS the work, and the apply in enrichsignature.go that lands
// the fields. They ran on different rules once — selection widened to an
// owner's own mail while the apply still demanded audience='workspace' — and
// the pass then marked every such message read without writing anything, so
// the mail was consumed and never reconsidered. A predicate spelled twice is
// how that happens; spelled once, the two cannot drift.
//
// Held by: TestSignatureEligibilityHasOneSpelling
// (backend/gates/signatureeligibilityonespelling_test.go), which fails when any
// other signature reader in this module tests the audience itself.
//
// The rule, in two parts.
//
// WORKSPACE MAIL is always minable, whatever the contact's visibility. Every
// seat can already open it, so writing its content into a field discloses it to
// nobody new. This arm is unconditional on purpose: an owner-scoped contact with
// open mail is the ordinary case, and hanging it off the visibility would refuse
// mail that nothing protects.
//
// NARROWED MAIL is minable only when the record is no wider than the message.
// A `participants` message may be read for an OWNER-SCOPED contact, because
// their fields are readable by their owner — the same audience the mail already
// has. Three conditions bound it:
//
//   - the owner must actually be ON the message, delivered to their mailbox or
//     stamped as a participant, the two facts auth.activityMembershipArm uses
//     to decide a seat was present;
//   - the record must carry NO live grant. contact is a shareable table, so a
//     record_grant lets another seat read these fields with no tie to the source
//     message's audience — mining narrowed mail would put its content in front
//     of somebody who cannot open it;
//   - a WORKSPACE-VISIBLE contact never qualifies: their fields land on a record
//     every seat reads, and narrowing the message afterwards does not take the
//     field back.
//
// audience='selected' is deliberately absent: it admits named users and teams
// beyond the participants, so it is not bounded by "the owner was there".
func SignatureSourceEligible(contact, activity string) string {
	return fmt.Sprintf(`(
		    %[2]s.audience = 'workspace'
		    OR (%[1]s.visibility = 'owner' AND %[2]s.audience = 'participants'
		        AND (EXISTS (SELECT 1 FROM capture_import ci
		                      WHERE ci.activity_id = %[2]s.id AND ci.user_id = %[1]s.owner_id)
		             OR EXISTS (SELECT 1 FROM activity_participant ap
		                         WHERE ap.activity_id = %[2]s.id AND ap.user_id = %[1]s.owner_id))
		        AND NOT EXISTS (SELECT 1 FROM record_grant rg
		                         WHERE rg.record_type = 'contact' AND rg.record_id = %[1]s.id
		                           AND (rg.expires_at IS NULL OR rg.expires_at > now())))
		  )`, contact, activity)
}

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
