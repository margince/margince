// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The time-based sweep's reach into the relationship graph (ADR-0078). It runs
// inside the retention engine's per-record transaction and is reached only from
// the person/anonymize action — a separate file, not a separate obligation.
//
// retentionSweepFiles in the PII-coverage gate lists this file alongside
// retention.go, so a table swept only here still counts as swept — and it stays
// OUT of erasureCascadeFiles, because a nightly sweep is never an answer to an
// Art. 17 request.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The sweep's two participant statements, package-level for the reason the
// eraser's twins are: legalholdarms_test.go enumerates them BY NAME, and a
// statement assembled inside a function is one that census cannot see — a hold
// arm it stops checking reads exactly like a hold arm that passes.
//
// They carry the transitive hold exclusion the eraser's carry, which this sweep
// did NOT have before. A legal hold on a linked deal, company, lead or
// project freezes the evidence about it, and a clock has less claim on that
// evidence than a subject's own Art. 17 request does — so the path nobody asks
// for may not be the one that deletes what the requested path refuses to.
var sweptParticipantsDelete = `
		DELETE FROM activity_participant ap
		 WHERE ap.user_id IS NULL` + subjectNamedOnAParticipantRow() +
	notTransitivelyHeld("ap.activity_id")

var sweptParticipantsBlank = `
		UPDATE activity_participant ap
		   SET person_id = NULL, address = NULL, display_name = NULL, channel_user_id = NULL
		 WHERE ap.user_id IS NOT NULL` + subjectNamedOnAParticipantRow() +
	notTransitivelyHeld("ap.activity_id")

// subjectGraphIdentifiers reads the two identifiers the graph holds the subject
// by, and it is called before the anonymization destroys either.
//
// The graph structures exist precisely to hold a party who never became a
// record, so a sweep matching person_id alone leaves the subject named,
// readable and re-matchable. There are two such identifiers and they arrive
// from different tables: the raw ADDRESS a message carried — what the address
// arm of a participant row IS — and the ACCOUNT a chat roster named them with,
// which is how the third human in a group is identified and the only way they
// are. Both source tables are deleted further down the same act.
//
// LIVE bindings only, which is where this path parts from the eraser's
// personChannelIdentities — and the difference is a precondition rather than a
// preference. The eraser reads archived bindings too, because it is DELETING
// them and because refuseRivalIdentifierHolders has already refused the whole
// request if another live person holds one of the same identifiers. This sweep
// has no such refusal: nobody asked for it, so there is nobody to refuse to.
//
// Archiving a Person archives their bindings, so an account that was theirs can
// be a LIVE binding of somebody else by the time a clock reaches the archived
// record — the same customer writing in again and resolving to a second person.
// Matching it here would delete that live third party's rows on a request that
// never named them. uq_person_channel_identity is unique among live rows, so a
// live binding names exactly one person and the case cannot arise.
func subjectGraphIdentifiers(ctx context.Context, tx pgx.Tx, id ids.UUID) ([]string, []channelIdentity, error) {
	emails, err := collectStrings(ctx, tx,
		`SELECT lower(email) FROM person_email WHERE person_id = $1`, id)
	if err != nil {
		return nil, nil, err
	}
	rows, err := tx.Query(ctx,
		`SELECT provider, channel_user_id FROM person_channel_identity
		  WHERE person_id = $1 AND archived_at IS NULL`, id)
	if err != nil {
		return nil, nil, err
	}
	accounts, err := pgx.CollectRows(rows, pgx.RowToStructByPos[channelIdentity])
	if err != nil {
		return nil, nil, err
	}
	return emails, accounts, nil
}

// scrubPersonGraphTraces removes the anonymized subject from the relationship
// graph. Those structures hold the subject as surely as the person columns do,
// and the time-based sweep reaches them for the same reason the request-driven
// eraser does: an anonymized person who is still named on a participant row,
// still counted in an interaction edge, or still listed in an imported address
// book is not anonymized. This sweep is the path nobody asks for, which is
// exactly why it must not be the thinner one.
//
// subjectEmails, subjectAccounts and subjectName are the caller's, read before
// the anonymization overwrote them.
func scrubPersonGraphTraces(
	ctx context.Context, tx pgx.Tx, id ids.UUID,
	subjectEmails []string, subjectAccounts []channelIdentity,
	subjectName string, linkedInHandles []string,
) error {
	// The eraser's own identity predicate, called rather than restated — the
	// lesson the LinkedIn arm below already records, applied before this copy
	// could drift too. It is what carries the third identity here: a chat
	// roster names a party by account alone, and a sweep matching only
	// person_id and address would leave that account standing.
	//
	// Delete then null, in that order and for the reason the eraser documents:
	// a participant row must name somebody, so a row whose only identity is the
	// subject cannot be blanked, while one that also names a colleague is not
	// the subject's to remove.
	providers, accounts := channelIdentityPairs(subjectAccounts)
	_, err := tx.Exec(ctx, sweptParticipantsDelete, id, subjectEmails, providers, accounts)
	if err == nil {
		_, err = tx.Exec(ctx, sweptParticipantsBlank, id, subjectEmails, providers, accounts)
	}
	if err == nil {
		_, err = tx.Exec(ctx,
			`DELETE FROM graph_interaction_edge WHERE person_id = $1`, id)
	}
	if err == nil {
		// Both endpoint columns: the swept party can stand on either end of a
		// contact↔contact edge, and the row re-identifies them from the graph
		// alone whichever side they are on.
		_, err = tx.Exec(ctx,
			`DELETE FROM graph_contact_edge WHERE person_a = $1 OR person_b = $1`, id)
	}
	if err == nil {
		// The SAME reach the request-driven eraser uses, by calling it rather
		// than by restating it. This path used to carry its own copy of the
		// predicate and the copy had drifted: no profile-URL arm, and an
		// employer compared only for equality where the eraser also matches a
		// longer name. Both gaps left a named third party's row standing after
		// the clock said it was gone, and the comment here claimed the reaches
		// were identical the whole time.
		//
		// This is the path nobody asks for, which is exactly why it must not be
		// the thinner one.
		err = deleteSubjectLinkedInGhosts(ctx, tx, ids.From[ids.PersonKind](id), subjectEmails, subjectName, linkedInHandles)
	}
	return err
}
