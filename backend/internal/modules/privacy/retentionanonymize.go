// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// What ANONYMIZING a person under a retention rule actually does: the person
// row's own columns, the satellites that name them outright, the purchase
// history, and the communication record.
//
// Split from retentionactions.go, which is the dispatch — what the engine can do
// to one over-age record. This is the one action big enough to be its own
// concept: it is the time-based twin of the Art. 17 cascade, and it is held
// against that twin table by table (personscrub_test.go). Reading it beside the
// executor table made the widest act in the module look like one row of a map.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// anonymizePersonRecord is the person/anonymize action: it strips the subject's
// own identifying fields and the rows that carry their addresses, so the record
// stops naming them by any key it is resolved on. The subject may lawfully return, so no suppression entry is
// written.
//
// It is NOT what the eraser does minus that entry. Tables the eraser clears are
// untouched here — the raw captures and attachments their messages came from,
// their lead rows and scores, their preference tokens, their deal-room seats.
//
// What survives is written down per table in
// TestErasingAndAnonymizingClearTheSameTables (backend/gates/personscrub_test.go),
// which fails when the gap widens in either direction. That test compares which
// TABLES each act writes and cannot see two acts clearing one table to
// different depths, which is why the custom columns above are nulled here
// deliberately rather than left for it to notice.
//
// Held by: TestErasingAndAnonymizingClearTheSameTables (backend/gates/personscrub_test.go)
func anonymizePersonRecord(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	// The subject's addresses, read BEFORE person_email is deleted
	// below. The graph structures name them by raw address as well as
	// by person id — that is what the address arm of a participant row
	// IS — so a sweep that only matched person_id would leave the
	// address behind, still readable and still re-matchable. Same trap
	// the eraser hit with the subject's NAME, one column over.
	subjectEmails, err := collectStrings(ctx, tx,
		`SELECT lower(email) FROM person_email WHERE person_id = $1`, id)
	if err != nil {
		return err
	}
	// The ACCOUNTS too, and before this sweep's own deletes reach
	// person_channel_identity. A chat roster names the third human in a
	// group by the provider's account and by nothing else, so an account is
	// the third way a graph row holds the subject — the same trap the
	// address arm above describes, in a second vocabulary. Read through the
	// eraser's own query rather than a second one, so the two paths cannot
	// disagree about which accounts are the subject's.
	subjectAccounts, err := personChannelIdentities(ctx, tx, ids.From[ids.PersonKind](id))
	if err != nil {
		return err
	}
	// The NAME too, and before the anonymization below overwrites it —
	// the ghost sweep matches on it, and by then it is the tombstone.
	var subjectName string
	if err := tx.QueryRow(ctx,
		`SELECT coalesce(full_name, '') FROM person WHERE id = $1`, id).Scan(&subjectName); err != nil {
		return err
	}
	// The installation's own columns too. A custom field is where an operator
	// puts what the fixed schema has no room for — a personal note, a private
	// address, a handle — so leaving them would anonymize the name and keep
	// whatever somebody wrote beside it. The eraser nulls them by the same
	// means; a record that still carries them has not stopped naming anyone.
	personCustom, err := subjectCustomColumns(ctx, tx, "person")
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, fmt.Sprintf(`
		UPDATE person SET first_name = NULL, last_name = NULL, full_name = $2,
		  title = NULL, raw = NULL, photo_object_key = NULL, photo_origin = NULL,
		  address_line1 = NULL, address_line2 = NULL, address_city = NULL,
		  address_region = NULL, address_postal_code = NULL, address_country = NULL,
		  archived_at = coalesce(archived_at, now())%s
		WHERE id = $1`, nullColumnAssignments(personCustom)), id, erasedName)
	if err == nil {
		// The double-opt-in token goes with the addresses it was sent to. It is
		// a bearer secret whose only function is to authorise a consent GRANT
		// for this subject, so one left standing after an anonymization is a
		// live invitation to record a lawful basis for somebody the row no
		// longer names. An anonymized subject may lawfully return, which is
		// what the suppression list is for — but they return by being invited
		// again, not by an old token in an old mailbox still working.
		_, err = tx.Exec(ctx, `DELETE FROM consent_doi_token WHERE person_id = $1`, id)
	}
	if err == nil {
		// The confirm-details link goes for the same reason, and a stronger
		// one: it does not merely authorise a grant, it DISPLAYS the record. A
		// link left live would show an old mailbox the fields this statement
		// has just emptied.
		_, err = tx.Exec(ctx, `DELETE FROM confirm_token WHERE person_id = $1`, id)
	}
	if err == nil {
		// And what came back through it, which is the subject's own name and
		// address in plaintext — exactly the content the anonymization above
		// just cleared from the person row.
		_, err = tx.Exec(ctx, `DELETE FROM person_confirm_submission WHERE person_id = $1`, id)
	}
	if err == nil {
		err = clearCommunicationRecord(ctx, tx, id, subjectEmails)
	}
	// Read BEFORE the delete below, for the reason the eraser gives at its own
	// copy of this: the LinkedIn ghost sweep identifies rows by this address,
	// and person_social is about to stop holding it.
	var linkedInHandles []string
	if err == nil {
		linkedInHandles, err = collectStrings(ctx, tx,
			`SELECT handle FROM person_social WHERE person_id = $1 AND platform = 'linkedin'`, id)
	}
	if err == nil {
		err = deleteIdentifyingSatellites(ctx, tx, id)
	}
	// The JUDGEMENTS made about them: what a classifier concluded their replies
	// meant with every human correction of it, and the handoffs naming them.
	// The activity TEXT survives an anonymize — no floor applies — so a verdict
	// or a "rejected: not qualified" left beside those words goes on reading as
	// a live conclusion about somebody the row no longer names.
	if err == nil {
		err = deleteReplyVerdictHistoryFor(ctx, tx, id)
	}
	if err == nil {
		err = deleteSubjectHandoffs(ctx, tx, id)
	}
	if err == nil {
		err = purgeSubjectPurchases(ctx, tx, id)
	}
	if err == nil {
		// What a colleague wrote down to DO about them, on their own weekly
		// plan. The SAME statement the Art. 17 cascade runs, called rather than
		// copied: the two acts differ only by the suppression list, and a
		// second spelling here would be the drift personscrub_test.go exists to
		// catch. Nothing cascades to it — the table holds no person FK and is
		// keyed to the rep who wrote it — so an anonymize that skipped it would
		// leave the subject's name in a commitment beside an "Erased Subject"
		// record.
		err = redactCommitmentsNaming(ctx, tx, ids.From[ids.PersonKind](id))
	}
	if err == nil {
		_, err = tx.Exec(ctx,
			`DELETE FROM embedding WHERE entity_type = 'person' AND entity_id = $1`, id)
	}
	if err == nil {
		// A provenance row names where a field value came from — its source,
		// who captured it, the evidence it was read out of — and it points at
		// the fields the statements above just nulled. There is nothing in it
		// to anonymize: what identifies the subject IS the record of where they
		// were found. The eraser deletes it for that reason and so does this.
		_, err = tx.Exec(ctx,
			`DELETE FROM field_provenance WHERE object_type = 'person' AND object_id = $1`, id)
	}
	if err == nil {
		// Feedback rows name this person as the subject an AI answer was judged
		// about. The judgement is about them and cannot be held without them.
		_, err = tx.Exec(ctx,
			`DELETE FROM ai_feedback WHERE subject_type = 'person' AND subject_id = $1`, id)
	}
	if err == nil {
		// Against the addresses READ AT THE TOP, not a subquery over
		// person_email: those rows are already gone by here, so a subquery
		// would match nothing and this statement would delete nothing while
		// looking like it did.
		//
		// The ledger carries the address a message arrived at and the display
		// name it arrived with, and it is the key a later capture re-matches
		// on — left behind it keeps answering with the person this act just
		// stopped naming.
		_, err = tx.Exec(ctx, `
			DELETE FROM capture_pending_counterparty WHERE email = ANY($1)`, subjectEmails)
	}
	if err == nil {
		err = scrubPersonGraphTraces(ctx, tx, id, subjectEmails, subjectAccounts, subjectName, linkedInHandles)
	}
	return err
}

// deleteIdentifyingSatellites removes the rows that name the subject outright.
//
// In this file, one statement per table: gates/satellite_lifecycle_test.go
// reads this FILE's SQL literals to prove every satellite is handled, so a
// helper elsewhere or a loop over identifiers is invisible to it.
func deleteIdentifyingSatellites(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	// The anonymize UPDATES the person row rather than deleting it, so none of
	// these cascades — one skipped leaves the subject readable beside an
	// "Erased Subject" record.
	_, err := tx.Exec(ctx, `DELETE FROM person_social WHERE person_id = $1`, id)
	if err == nil {
		_, err = tx.Exec(ctx, `DELETE FROM person_email WHERE person_id = $1`, id)
	}
	if err == nil {
		_, err = tx.Exec(ctx, `DELETE FROM person_phone WHERE person_id = $1`, id)
	}
	if err == nil {
		// The sidecar holds their title and employer verbatim.
		_, err = tx.Exec(ctx, `DELETE FROM person_profile_field WHERE person_id = $1`, id)
	}
	if err == nil {
		// A resolution key: left behind it keeps binding inbound mail here.
		_, err = tx.Exec(ctx, `DELETE FROM person_channel_identity WHERE person_id = $1`, id)
	}
	return err
}

// purgeSubjectPurchases removes everything a licensed data provider left on one
// subject: what it asserted, what those assertions FILLED on the record, and
// the identifying half of the runs that bought them.
//
// The same three statements the Art. 17 path runs, for the same reason:
// anonymize-in-place leaves the person row standing, so nothing cascades, and
// without these the page would show a bought email and employer beside an
// "Erased Subject" name. The runs are scrubbed rather than deleted — what the
// installation paid is an accounting fact about the installation once the row
// names nobody.
func purgeSubjectPurchases(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	if _, err := tx.Exec(ctx, `DELETE FROM person_provider_claim WHERE person_id = $1`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM provider_applied_field WHERE person_id = $1`, id); err != nil {
		return err
	}
	_, err := tx.Exec(ctx,
		`UPDATE provider_run SET`+storekit.ScrubProviderRunColumns+` WHERE person_id = $1`, id)
	return err
}

// clearCommunicationRecord gives the anonymizer the same treatment of the
// outbound authorization record that the eraser applies, because both acts must
// clear the same tables: one that cleared a table the other left would hold the
// subject's data after an operator had been told the record was dealt with.
//
// The decision row is tombstoned rather than deleted — it is the controller's
// own Art. 5(2) evidence that a send already made was lawful, and losing that
// is not what anonymization is for. A basis IS deleted, because it names a
// thread and a date and an anonymized subject who returns arrives as a NEW
// record, so an old basis would authorize writing to them on the strength of a
// conversation nobody can now identify.
//
// A live objection is neither: it is re-pinned to the address. The eraser can
// delete one because it also hashes every address onto erasure_suppression, so
// an erased subject cannot be re-captured at all. The anonymizer writes no such
// row by design, so deleting the objection here would let a returning subject
// be marketed to having never withdrawn it.
func clearCommunicationRecord(ctx context.Context, tx pgx.Tx, id ids.UUID, addresses []string) error {
	// Why this contact existed, cleared alongside the rest. It names one
	// person and has no meaning after them, and both acts must clear the same
	// tables — one that cleared it and one that did not would leave the
	// subject's acquisition record standing after an operator was told the
	// person had been anonymized.
	if _, err := tx.Exec(ctx, `DELETE FROM person_acquisition_evidence WHERE person_id = $1`, id); err != nil {
		return err
	}
	// Per row, not one constant. A single delivery can carry two decisions for
	// the same subject — one message To and Cc'ing two of their addresses — and
	// collapsing both to one tombstone collides on
	// communication_decision_one_per_attempt, which aborts the whole
	// transaction and leaves the subject unerasable on every retry.
	if _, err := tx.Exec(ctx, `
		UPDATE communication_decision
		   SET recipient_address = 'erased+' || id || '@example.invalid',
		       subject_id = NULL, subject_kind = NULL
		 WHERE subject_id = $1`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM privacy_notice_case WHERE person_id = $1`, id); err != nil {
		return fmt.Errorf("clear the person's notice cases: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM communication_basis WHERE person_id = $1`, id); err != nil {
		return err
	}
	// The objection SURVIVES an anonymization, re-pinned to the address.
	//
	// This is where the anonymizer parts company with the eraser. The eraser
	// hashes every address onto erasure_suppression, so an erased subject
	// cannot be re-captured at all and a per-person objection has nothing left
	// to protect. The anonymizer deliberately writes no such row, because an
	// anonymized subject may lawfully return — and if their objection were
	// deleted here they would return unsuppressed, having never withdrawn it.
	// So the person link is cut and the address kept, which is exactly what the
	// address-only row shape exists for.
	// EVERY row is detached, revoked or not, and the revoked ones keep their
	// revoked_at. Filtering on `revoked_at IS NULL` here was safe while nothing
	// could set that column; consent's lift verb now can, and a lift committing
	// between this statement and the DELETE below would leave the row unmatched
	// here and then deleted there — losing an address the anonymizer is about to
	// orphan. Detaching a revoked row costs nothing: it stays revoked, so it
	// suppresses nothing, and it carries the address forward for the record
	// rather than vanishing mid-transaction.
	if _, err := tx.Exec(ctx, `
		UPDATE communication_suppression
		   SET person_id = NULL, address = coalesce(address, u.addr)
		  FROM unnest($2::text[]) AS u(addr)
		 WHERE person_id = $1`, id, addresses); err != nil {
		return err
	}
	// Whatever the detach could not reach — a row whose address is not among
	// the subject's — names a person who is going, so it goes with them.
	_, err := tx.Exec(ctx, `DELETE FROM communication_suppression WHERE person_id = $1`, id)
	return err
}
