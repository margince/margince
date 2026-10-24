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
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// signatureCandidateSQL selects the contacts whose newest readable mail this
// pass has not read. Every test on the mail itself is in signatureSourceSQL, so
// this query and OlderSignatureSources cannot read different mail.
var signatureCandidateSQL = `
			SELECT p.id, p.full_name, coalesce(pe.email, ''), ` + olderMailMayFillTitleSQL("p") + `, a.id, coalesce(a.body, '')
			FROM contact p
			LEFT JOIN contact_email pe ON pe.contact_id = p.id AND pe.is_primary AND pe.archived_at IS NULL
			JOIN LATERAL (
				SELECT a.id, a.body, a.occurred_at
				FROM activity_link al
				JOIN activity a ON a.id = al.activity_id
				WHERE ` + signatureSourceSQL("$2") + `
				ORDER BY a.occurred_at DESC, a.id DESC
				LIMIT 1
			) a ON true
			LEFT JOIN contact_signature_enrich_state st ON st.contact_id = p.id
			-- Anyone the workspace still corresponds with, and no longer only
			-- the contacts whose details are missing. A contact changes jobs and
			-- numbers, so a filled field is a question this pass must keep
			-- asking. The watermark keeps that affordable: NEW mail, not a
			-- missing answer, makes somebody a candidate.
			WHERE p.archived_at IS NULL AND p.merged_into_id IS NULL
			  AND (st.last_activity_at IS NULL OR a.occurred_at > st.last_activity_at)
			-- Freshest mail first. The pass is capped and runs within minutes of
			-- a message arriving, so the contact who just wrote is the one a rep
			-- is about to look at.
			ORDER BY a.occurred_at DESC
			LIMIT $1`

// olderSignatureSourcesSQL lists a live contact's readable mails strictly older
// than the one named, in the candidate query's order.
var olderSignatureSourcesSQL = `
			SELECT a.id
			FROM contact p
			JOIN activity newest ON newest.id = $4
			JOIN activity_link al ON al.contact_id = p.id
			JOIN activity a ON a.id = al.activity_id
			WHERE p.id = $3 AND p.archived_at IS NULL AND p.merged_into_id IS NULL
			  AND ` + signatureSourceSQL("$2") + `
			  AND (a.occurred_at, a.id) < (newest.occurred_at, newest.id)
			ORDER BY a.occurred_at DESC, a.id DESC
			LIMIT $1`

// readableSignatureSourceSQL returns the body of one named mail only while the
// pass may still read it for this live contact. The pass asks it right before
// each model call, so a mailbox switched off mid-walk stops the walk.
var readableSignatureSourceSQL = `
			SELECT coalesce(a.body, '')
			FROM contact p
			JOIN activity_link al ON al.contact_id = p.id
			JOIN activity a ON a.id = al.activity_id
			WHERE p.id = $1 AND a.id = $3 AND p.archived_at IS NULL AND p.merged_into_id IS NULL
			  AND ` + signatureSourceSQL("$2")

// signatureSourceSQL holds the tests one mail must pass before this pass reads
// it for the contact's signature. The caller joins `activity_link al` and
// `activity a`; defaultEnabled is the placeholder of the workspace's answer for
// a mailbox that never chose.
func signatureSourceSQL(defaultEnabled string) string {
	return `al.contact_id = p.id AND al.entity_type = 'contact'
				  -- THIS CONTACT WROTE IT. A signature is only theirs to be read
				  -- off a message they sent, never one that merely reached them.
				  AND ` + SenderPredicate("p.id", "a") + `
				  AND ` + SignatureSourceEligible("p", "a") + `
				  AND ` + signatureMailSQL(defaultEnabled)
}

// signatureMailSQL holds the mail-wide conditions; signatureSourceSQL also
// binds the mail to its contact. The final apply rechecks the complete rule.
//
// The mailbox switch is a test on each mail. A newer mail from a switched-off or
// disconnected mailbox therefore does not hide an older mail from one that is on.
// A bare connector:<name> stamp matches no connection and follows the workspace
// default.
func signatureMailSQL(defaultEnabled string) string {
	return `a.kind = 'email' AND a.direction = 'inbound' AND a.archived_at IS NULL
				  -- A row held under a statutory obligation is out of reach of
				  -- the model call this feeds, which is processing.
				  AND a.restricted_at IS NULL
				  -- A body the parser did not decode in full may have lost the
				  -- signature, and newsletter mail states the list's footer.
				  AND ` + provenance.MailBodyCompleteSQL("a") + `
				  AND NOT a.bulk_mail_attested
				  AND COALESCE((
					SELECT cc.signature_enrich_enabled AND cc.status = 'connected'
					  FROM capture_connection cc WHERE ` + capturingMailboxSQL("a") + `
				  ), ` + defaultEnabled + `)`
}

// capturingMailboxSQL matches the live connection whose mailbox captured the
// activity, by the provenance string capture stamps (connector:<provider>:<user
// id>). There is no foreign key.
func capturingMailboxSQL(activity string) string {
	return `('connector:' || cc.provider || ':' || cc.user_id::text) = ` + activity + `.captured_by
					   AND cc.archived_at IS NULL`
}

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

// SignatureCandidates lists the contacts whose newest readable mail this pass
// has not read yet, freshest first and capped at limit.
//
// The watermark, not the emptiness of a field, is what retires somebody. A
// contact changes jobs and numbers, so a filled title is worth asking about
// again when they write. A contact returns only when mail newer than the
// watermark arrives. Each new mail then costs at most signatureHistoryDepth
// model calls, however often the pass runs. The comparison is on occurred_at:
// an activity id check would reopen the contact once the mail it names is
// archived. defaultEnabled is the workspace answer for a
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
			if err := rows.Scan(&c.ContactID, &c.FullName, &c.Email, &c.OlderMailMayFillTitle, &c.ActivityID, &c.Body); err != nil {
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

// OlderSignatureSources lists up to limit of the contact's readable mails older
// than before, newest first. It returns ids only: the pass reads each body
// through ReadableSignatureBody right before it asks the model.
func (s *Store) OlderSignatureSources(ctx context.Context, contactID ids.ContactID, before ids.UUID, limit int, defaultEnabled bool) ([]ids.UUID, error) {
	var out []ids.UUID
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, olderSignatureSourcesSQL, limit, defaultEnabled, contactID, before)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("contacts: listing older signature sources: %w", err)
	}
	return out, nil
}

// ReadableSignatureBody returns the mail's body, and false when the pass may no
// longer read it for this contact. It applies the candidate query's tests.
func (s *Store) ReadableSignatureBody(ctx context.Context, contactID ids.ContactID, activityID ids.UUID, defaultEnabled bool) (string, bool, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return "", false, err
	}
	var body string
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, readableSignatureSourceSQL, contactID, defaultEnabled, activityID).Scan(&body)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("contacts: reading a signature source: %w", err)
	}
	return body, true, nil
}
