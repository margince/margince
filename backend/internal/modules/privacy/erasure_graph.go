// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The subject's traces in the relationship graph (ADR-0078): who they were
// recorded as being in a conversation with, the ghosts of them in a
// colleague's imported network, and the interaction projection folded out of
// both. This is the SAME Art. 17 transaction the rest of the cascade runs in —
// a separate file, not a separate obligation, and reached only from
// anonymizeSubjectRows. erasureCascadeFiles in the PII-coverage gate lists this
// file alongside erasure.go, so moving a scrub here does not take the tables it
// purges out of that gate's sight.
//
// Every clause here reaches the subject by IDENTIFIER as well as by contact id,
// because the graph structures exist precisely to hold a party who never
// became a record: that is what the address arm of a participant row and a
// LinkedIn ghost both ARE. A contact-keyed sweep alone leaves the subject
// named, reachable and re-matchable.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// scrubSubjectFromGraph removes the subject from the relationship graph.
// subjectName and linkedInHandles are the caller's, read before the
// anonymization that overwrote them: the ghost sweep identifies rows by both,
// and by the time it runs neither the contact row nor contact_social still holds
// them.
func scrubSubjectFromGraph(
	ctx context.Context, tx pgx.Tx, contactID ids.ContactID,
	emails []string, identities []channelIdentity, subjectName string, linkedInHandles []string,
) error {
	if err := scrubSubjectFromParticipants(ctx, tx, contactID, emails, identities); err != nil {
		return err
	}
	if err := deleteSubjectLinkedInGhosts(ctx, tx, contactID, emails, subjectName, linkedInHandles); err != nil {
		return err
	}
	return deleteSubjectInteractionEdges(ctx, tx, contactID)
}

// scrubSubjectFromParticipants clears the subject off the interaction
// participants (ACT-DDL-3), which name them three times over: by contact_id, by
// the raw ADDRESS a message carried, and by the ACCOUNT a chat roster named
// them with. The last two exist precisely for the party who never became a
// record, so they survive the contact_email purge and would keep the erased
// subject readable and re-matchable.
//
// The account arm is paired with the TRANSPORT, read off the activity the row
// hangs from, because an account id is only the subject's against the provider
// that issued it — a bare numeric id matched across every provider is the
// untyped over-deletion this module refuses everywhere else. The participant
// row carries no provider of its own, so the transport a scrub matches on is
// the transport the message actually rode.
func scrubSubjectFromParticipants(
	ctx context.Context, tx pgx.Tx, contactID ids.ContactID,
	emails []string, identities []channelIdentity,
) error {
	providers, accounts := channelIdentityPairs(identities)
	// Delete first, then null. A participant row must name SOMEBODY (the
	// ACT-DDL-3 identity CHECK), so a row whose only identity is the subject
	// cannot be blanked — it has to go. A row that also names one of our
	// users is a different matter: the colleague was in that conversation and
	// that is not the subject's data to erase, so the subject's arms are
	// nulled and the row stands.
	if _, err := tx.Exec(ctx, subjectParticipantsDelete, contactID, emails, providers, accounts); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, subjectParticipantsBlank, contactID, emails, providers, accounts)
	return err
}

// subjectNamedOnAParticipantRow is the three ways one row can name the subject,
// shared by the delete and the blank below so both reach the same rows: a row
// either goes or is stripped, and a key one statement recognised while the
// other did not would leave the subject on the row that stayed.
//
// The statements it builds stay package-level VARS because
// legalholdarms_test.go enumerates them by name — a statement assembled inside
// a function is one that census cannot see, and a hold arm it stops checking
// reads exactly like a hold arm that passes.
func subjectNamedOnAParticipantRow() string {
	return `
		   AND (ap.contact_id = $1
		     OR (ap.address IS NOT NULL AND ap.address = ANY($2))
		     OR (ap.channel_user_id IS NOT NULL AND EXISTS (
		           SELECT 1 FROM activity a
		            WHERE a.id = ap.activity_id
		              AND (a.channel_provider, ap.channel_user_id) IN (
		                    SELECT provider, account
		                      FROM unnest($3::text[], $4::text[]) AS t(provider, account)))))`
}

// Both participant statements carry the transitive hold exclusion: a
// participant row is part of the held activity's record of who was in the
// conversation, and a hold that keeps the message but loses its parties is
// not a hold.
var subjectParticipantsDelete = `
		DELETE FROM activity_participant ap
		 WHERE ap.user_id IS NULL` + subjectNamedOnAParticipantRow() +
	notTransitivelyHeld("ap.activity_id")

var subjectParticipantsBlank = `
		UPDATE activity_participant ap
		   SET contact_id = NULL, address = NULL, display_name = NULL, channel_user_id = NULL
		 WHERE ap.user_id IS NOT NULL` + subjectNamedOnAParticipantRow() +
	notTransitivelyHeld("ap.activity_id")

// deleteSubjectLinkedInGhosts drops the subject's LinkedIn ghosts (CG-DDL-2).
// A ghost can BE the subject: it holds their name, employer and — on CSV rows
// — their address, imported from a colleague's export without the subject ever
// being asked. That is exactly the data an Art. 17 request is about, and it is
// invisible to every other clause in the cascade because a ghost is not a
// contact row.
//
// It deletes on SUGGESTION-GRADE evidence, not just on a confirmed match, and
// that asymmetry is deliberate. Matching errs toward caution because a wrong
// link attaches a stranger to a customer record. Deletion errs the other way,
// because the two mistakes do not cost the same: deleting one ghost too many
// costs a re-import of a file the colleague still has, while keeping one too
// few leaves a named contact's data behind after we certified it destroyed.
//
// So: matched to them, or carrying their address, or bearing their name at an
// employer they actually work for — the same evidence that would have produced
// a suggestion.
func deleteSubjectLinkedInGhosts(
	ctx context.Context, tx pgx.Tx, contactID ids.ContactID,
	emails []string, subjectName string, linkedInHandles []string,
) error {
	_, err := tx.Exec(ctx, `
		DELETE FROM linkedin_connection g
		 WHERE g.matched_contact_id = $1
		    OR (g.email IS NOT NULL AND g.email = ANY($2))
		    -- The LinkedIn address, passed in rather than joined: contact_social
		    -- is cleared earlier in this same transaction, so a join here would
		    -- read an empty table and miss every ghost identified only by URL.
		    OR (g.profile_url IS NOT NULL AND g.profile_url = ANY($4))
		    -- Name + employer, matched on the NAME the ghost carries rather
		    -- than on its derived matched_company_id. That column is set by a
		    -- matcher that runs on upload: a ghost imported before its account
		    -- existed, or one whose matcher pass failed, has it NULL and would
		    -- survive an erasure it plainly belongs in. The employer is
		    -- compared as text, which is the evidence the ghost actually holds.
		    OR (g.normalized_company IS NOT NULL
		        AND lower(f_unaccent($3)) = g.normalized_name
		        AND EXISTS (
		            SELECT 1 FROM relationship r
		              JOIN company o ON o.id = r.company_id
		             WHERE r.contact_id = $1 AND r.kind = 'employment'
		               AND r.archived_at IS NULL
		               AND (r.company_id = g.matched_company_id
		                    OR lower(f_unaccent(o.display_name)) = g.normalized_company
		                    OR lower(f_unaccent(o.display_name)) LIKE g.normalized_company || ' %')))`,
		contactID, emails, subjectName, linkedInHandles)
	return err
}

// deleteSubjectInteractionEdges drops the subject's rows in the interaction
// projection (CG-DDL-1). It is derived, but derived from data that is now gone
// — and it holds who corresponded with the subject, how often and how
// recently. It is dropped HERE, in the erasure transaction, rather than left
// to the cg:graph-edge consumer: an erasure obligation that depends on an
// event being delivered is an obligation that fails silently when the bus is
// behind or the handler is wrong. It was in fact wrong — the consumer listened
// for a `contact.erased` event this path has never emitted, so the edges
// outlived every erasure.
func deleteSubjectInteractionEdges(ctx context.Context, tx pgx.Tx, contactID ids.ContactID) error {
	if _, err := tx.Exec(ctx,
		`DELETE FROM graph_interaction_edge WHERE contact_id = $1`, contactID); err != nil {
		return err
	}
	// The contact↔contact projection names the subject on EITHER end, so both
	// endpoint columns are matched — a contact_a-only delete leaves the subject
	// standing on the far side of everyone else's edges.
	_, err := tx.Exec(ctx,
		`DELETE FROM graph_contact_edge WHERE contact_a = $1 OR contact_b = $1`, contactID)
	return err
}
