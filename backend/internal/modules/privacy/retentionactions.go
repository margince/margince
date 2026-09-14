// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// What the engine can DO to one over-age record: the executor table, the two
// questions the authoring surface asks of it, and the dispatch.
//
// Split from retention.go because the table is two things at once — the dispatch
// AND the authorable set — and both the nightly pass and the write path consult
// it. Keeping it beside the pass made it read as an implementation detail of the
// loop, which is exactly the reading that let scope and action be validated
// independently.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// retentionExecutor applies one action to one record inside the pass's audited
// transaction.
type retentionExecutor func(s *RetentionService, ctx context.Context, tx pgx.Tx, id ids.UUID) error

// retentionActions is every `object_type/action` pair the engine can perform —
// the dispatch table AND the authorable set, deliberately one map.
//
// Scope and action are chosen INDEPENDENTLY by whoever authors a policy, but
// only some of their combinations have an executor: there is no way to archive
// an ai_call_payload or to anonymize an activity. A write path that validated
// each half separately would admit `deal/won` + `erase`, and the pass would then
// abort on the first due record and stay red every night until somebody deleted
// the row — taking every LATER policy with it, because policies are ordered.
// Storage limitation would stop installation-wide, silently. So membership here
// is what ParseRetentionScope is for scopes: the one gate, consulted by the
// validator and by the pass.
//
// contact/erase is registered with a NIL executor: it owns its own transaction
// (the Art. 17 cascade is ~30 statements plus object-store deletes), so apply
// dispatches it before opening one. Nil means "runs outside the transaction",
// never "unsupported" — membership is the key, not the value.
var retentionActions = map[string]retentionExecutor{
	"contact/erase":         nil,
	"activity/archive":      (*RetentionService).archiveActivity,
	"activity/erase":        (*RetentionService).eraseActivityContent,
	"deal/archive":          (*RetentionService).archiveDeal,
	"ai_call_payload/erase": (*RetentionService).erasePayload,
	"lead/anonymize":        (*RetentionService).anonymizeLead,
	"contact/anonymize":     (*RetentionService).anonymizeContact,
}

// The executors. Named methods rather than closures in the table above, because
// each runs a by-id UPDATE and updateguard_test.go walks named functions to
// assert every one of them either carries a concurrency guard or is a ratified
// exception — an anonymous function is invisible to it, and the retention sweep's
// deliberately unguarded absolute writes would stop being checked at all.

func (s *RetentionService) archiveActivity(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	if _, err := tx.Exec(ctx, `UPDATE activity SET archived_at = now() WHERE id = $1`, id); err != nil {
		return err
	}
	return s.invalidateGraph(ctx, tx, id)
}

func (*RetentionService) archiveDeal(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	_, err := tx.Exec(ctx, `UPDATE deal SET archived_at = now() WHERE id = $1`, id)
	return err
}

// erasePayload deletes the row outright rather than scrubbing it in place —
// unlike activity/erase there is no metadata half of this record left to keep:
// ai_call_payload IS the special-category-adjacent content, and ai_call (the
// metadata row it FK-cascades from) survives untouched. The retention audit entry
// carries no payload bytes, only policy metadata.
func (*RetentionService) erasePayload(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	_, err := tx.Exec(ctx, `DELETE FROM ai_call_payload WHERE id = $1`, id)
	return err
}

func (*RetentionService) anonymizeContact(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	return anonymizeContactRecord(ctx, tx, id)
}

// SupportsRetentionAction reports whether the engine can perform this action on
// this object type. The authoring surface refuses a pair it answers false for.
func SupportsRetentionAction(objectType, action string) bool {
	_, ok := retentionActions[objectType+"/"+action]
	return ok
}

// ActionsForScope is every action a given scope may be authored with, sorted, so
// a refusal can name the alternatives instead of leaving the caller to guess at
// a set the contract's two independent enums do not express.
func ActionsForScope(objectType string) []string {
	out := make([]string, 0, 3)
	for _, action := range []string{actionArchive, actionAnonymize, actionErase} {
		if SupportsRetentionAction(objectType, action) {
			out = append(out, action)
		}
	}
	return out
}

// apply runs ONE action on ONE record in one audited transaction.
func (s *RetentionService) apply(ctx context.Context, pol retentionPolicy, id ids.UUID) error {
	pair := pol.ObjectType + "/" + pol.Action
	executor, supported := retentionActions[pair]
	if !supported {
		// Unreachable through the authoring surface, which refuses an
		// unsupported pair, and through the pass, which skips the policy before
		// selecting a record. Kept because an unsupported pair must never be
		// mistaken for a completed action.
		return fmt.Errorf("retention: no executor for %s", pair)
	}
	if executor == nil {
		return s.eraser.EraseContact(ctx, id, "retention")
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := executor(s, ctx, tx, id); err != nil {
			return err
		}
		// Retention audits under the verb of the action it ran —
		// archive, anonymize and erase are all in the closed audit
		// vocabulary (0053) — so a governance read can tell a retention
		// anonymize from a user edit, and the field-history projection
		// can treat anonymize/erase as its scrub boundary instead of
		// parsing payload shapes. The policy metadata rides the evidence
		// column, and before/after stay nil: this row records that a
		// policy acted, not a field diff, so a projectable verb like
		// archive must carry no payload the field-history diff could
		// mistake for record fields.
		auditID, err := storekit.AuditWithEvidence(ctx, tx, pol.Action, pol.ObjectType, id, nil, nil, map[string]any{
			evidenceKeyRetentionAction: pol.Action, "policy": pol.ID, "retain_days": pol.RetainDays,
		})
		if err != nil {
			return err
		}
		policyID := pol.ID
		action, err := publishedRetentionAction(pol.Action, pol.ID)
		if err != nil {
			return err
		}
		return storekit.EmitEventForEntity(ctx, tx, auditID, pol.ObjectType, id, retentionAppliedPayload(action, &policyID, nil))
	})
}

// eraseActivityContent is the activity/erase action. Transcript free-text is
// the special-category risk; the record of the meeting stays, its content goes
// — including any attached recording/transcript file (objects first, so the
// purge shares the contact-erase durability guarantee).
//
// `raw` goes with `body`. It is the re-parseable original the schema names, so
// clearing the parsed copy and leaving the source erases nothing — the content
// is one parse away. Nothing in this tree populates the column, which is why
// only a gate will ever notice if this stops: piicoverage_test.go declares the
// assignments this statement IS.
//
// `counterparty_email` and the channel identity (`source_id`, `thread_key`)
// deliberately stay, and that is where this statement parts company with its
// two siblings. The retention action's contract is that the RECORD of the
// meeting survives and its content goes, and who it was with is the record.
// The difference is declared in piicoverage_test.go's retentionKeeps for
// `activity`, so reversing it fails the gate rather than passing silently — the
// data-layer guard `activity_restriction_lift_erases` exists because the same
// kind of difference was once carried in prose and went short.
func (s *RetentionService) eraseActivityContent(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	_, err := tx.Exec(ctx,
		// language goes with the text: it was read from the body this statement
		// is emptying, so keeping it would answer one question about content
		// that no longer exists.
		`UPDATE activity SET body = NULL, raw = NULL, subject = $2, language = NULL, archived_at = coalesce(archived_at, now()) WHERE id = $1`,
		id, erasedActivitySubject)
	if err == nil {
		// Everything the text left behind — the verbatim provider original, the
		// vectors, the provenance of fields that are now gone, the transcript
		// readings, the proposals quoting it, the attachments and the
		// transmitted copy.
		//
		// The SAME call the restriction lift and the controller's release make.
		// This list used to live here and a shorter one lived there, and the
		// shorter one was missing the provider original and the quoting
		// proposals — which is what a second list does, whatever the comment
		// beside it promises.
		err = s.eraser.purgeContentDerivedFrom(ctx, tx, id, theClockRanOut)
	}
	if err == nil {
		// Not in the shared helper, and that is the difference between the two
		// arms rather than an omission from one. This arm removes an
		// interaction the relationship aggregates counted, so it re-folds them
		// in the same transaction; the lift paths emit their own events and the
		// bus consumer that handles them is the backstop there. Folding it in
		// would make the shared helper need a seam two of its three callers do
		// not have.
		err = s.invalidateGraph(ctx, tx, id)
	}
	return err
}

// anonymizeContactRecord is the contact/anonymize action: it strips the subject's
// own identifying fields and the rows that carry their addresses, so the record
// stops naming them by any key it is resolved on. The subject may lawfully return, so no suppression entry is
// written.
//
// It is NOT what the eraser does minus that entry. Tables the eraser clears are
// untouched here — the raw captures and attachments their messages came from,
// their lead rows and scores, their preference tokens, their deal-room seats.
//
// What survives is written down per table in
// TestErasingAndAnonymizingClearTheSameTables (backend/gates/contactscrub_test.go),
// which fails when the gap widens in either direction. That test compares which
// TABLES each act writes and cannot see two acts clearing one table to
// different depths, which is why the custom columns above are nulled here
// deliberately rather than left for it to notice.
//
// Held by: TestErasingAndAnonymizingClearTheSameTables (backend/gates/contactscrub_test.go)
//
//nolint:cyclop // the rename added no branch: this body is what it was under the old noun.
func anonymizeContactRecord(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	// The identifiers the graph holds the subject by, read BEFORE the deletes
	// below destroy the rows they come from. subjectGraphIdentifiers says which
	// they are and why the order matters.
	subjectEmails, subjectAccounts, err := subjectGraphIdentifiers(ctx, tx, id)
	if err != nil {
		return err
	}
	// The NAME too, and before the anonymization below overwrites it —
	// the ghost sweep matches on it, and by then it is the tombstone.
	var subjectName string
	if err := tx.QueryRow(ctx,
		`SELECT coalesce(full_name, '') FROM contact WHERE id = $1`, id).Scan(&subjectName); err != nil {
		return err
	}
	// The installation's own columns too. A custom field is where an operator
	// puts what the fixed schema has no room for — a personal note, a private
	// address, a handle — so leaving them would anonymize the name and keep
	// whatever somebody wrote beside it. The eraser nulls them by the same
	// means; a record that still carries them has not stopped naming anyone.
	contactCustom, err := subjectCustomColumns(ctx, tx, "contact")
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, fmt.Sprintf(`
		UPDATE contact SET first_name = NULL, last_name = NULL, full_name = $2,
		  title = NULL, raw = NULL, photo_object_key = NULL, photo_origin = NULL,
		  address_line1 = NULL, address_line2 = NULL, address_city = NULL,
		  address_region = NULL, address_postal_code = NULL, address_country = NULL,
		  archived_at = coalesce(archived_at, now())%s
		WHERE id = $1`, nullColumnAssignments(contactCustom)), id, erasedName)
	if err == nil {
		// The double-opt-in token goes with the addresses it was sent to. It is
		// a bearer secret whose only function is to authorise a consent GRANT
		// for this subject, so one left standing after an anonymization is a
		// live invitation to record a lawful basis for somebody the row no
		// longer names. An anonymized subject may lawfully return, which is
		// what the suppression list is for — but they return by being invited
		// again, not by an old token in an old mailbox still working.
		_, err = tx.Exec(ctx, `DELETE FROM consent_doi_token WHERE contact_id = $1`, id)
	}
	if err == nil {
		// The confirm-details link goes for the same reason, and a stronger
		// one: it does not merely authorise a grant, it DISPLAYS the record. A
		// link left live would show an old mailbox the fields this statement
		// has just emptied.
		_, err = tx.Exec(ctx, `DELETE FROM confirm_token WHERE contact_id = $1`, id)
	}
	if err == nil {
		// The withdrawal link goes too, and it is the one that would linger
		// longest: 24 months against the double-opt-in token's weeks. It also
		// HOLDS THE ADDRESS the link was written to, in its own column, which
		// is precisely the content the statement above just cleared from the
		// contact row — so leaving it would keep an anonymized subject's mailbox
		// legible in a table the anonymization did not touch.
		_, err = tx.Exec(ctx, `DELETE FROM withdrawal_credential WHERE contact_id = $1`, id)
	}
	if err == nil {
		// And what came back through it, which is the subject's own name and
		// address in plaintext — exactly the content the anonymization above
		// just cleared from the contact row.
		_, err = tx.Exec(ctx, `DELETE FROM contact_confirm_submission WHERE contact_id = $1`, id)
	}
	if err == nil {
		err = clearCommunicationRecord(ctx, tx, id, subjectEmails)
	}
	// Read BEFORE the delete below, for the reason the eraser gives at its own
	// copy of this: the LinkedIn ghost sweep identifies rows by this address,
	// and contact_social is about to stop holding it.
	var linkedInHandles []string
	if err == nil {
		linkedInHandles, err = collectStrings(ctx, tx,
			`SELECT handle FROM contact_social WHERE contact_id = $1 AND platform = 'linkedin'`, id)
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
		// second spelling here would be the drift contactscrub_test.go exists to
		// catch. Nothing cascades to it — the table holds no contact FK and is
		// keyed to the rep who wrote it — so an anonymize that skipped it would
		// leave the subject's name in a commitment beside an "Erased Subject"
		// record.
		err = redactCommitmentsNaming(ctx, tx, ids.From[ids.ContactKind](id))
	}
	if err == nil {
		_, err = tx.Exec(ctx,
			`DELETE FROM embedding WHERE entity_type = 'contact' AND entity_id = $1`, id)
	}
	if err == nil {
		// A provenance row names where a field value came from — its source,
		// who captured it, the evidence it was read out of — and it points at
		// the fields the statements above just nulled. There is nothing in it
		// to anonymize: what identifies the subject IS the record of where they
		// were found. The eraser deletes it for that reason and so does this.
		_, err = tx.Exec(ctx,
			`DELETE FROM field_provenance WHERE object_type = 'contact' AND object_id = $1`, id)
	}
	if err == nil {
		// Feedback rows name this contact as the subject an AI answer was judged
		// about. The judgement is about them and cannot be held without them.
		_, err = tx.Exec(ctx,
			`DELETE FROM ai_feedback WHERE subject_type = 'contact' AND subject_id = $1`, id)
	}
	if err == nil {
		// Against the addresses READ AT THE TOP, not a subquery over
		// contact_email: those rows are already gone by here, so a subquery
		// would match nothing and this statement would delete nothing while
		// looking like it did.
		//
		// The ledger carries the address a message arrived at and the display
		// name it arrived with, and it is the key a later capture re-matches
		// on — left behind it keeps answering with the contact this act just
		// stopped naming.
		_, err = tx.Exec(ctx, `
			DELETE FROM capture_pending_counterparty WHERE email = ANY($1)`, subjectEmails)
	}
	if err == nil {
		err = scrubContactGraphTraces(ctx, tx, id, subjectEmails, subjectAccounts, subjectName, linkedInHandles)
	}
	return err
}

// deleteIdentifyingSatellites removes the rows that name the subject outright.
//
// In this file, one statement per table: gates/satellite_lifecycle_test.go
// reads this FILE's SQL literals to prove every satellite is handled, so a
// helper elsewhere or a loop over identifiers is invisible to it.
func deleteIdentifyingSatellites(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	// The anonymize UPDATES the contact row rather than deleting it, so none of
	// these cascades — one skipped leaves the subject readable beside an
	// "Erased Subject" record.
	_, err := tx.Exec(ctx, `DELETE FROM contact_social WHERE contact_id = $1`, id)
	if err == nil {
		_, err = tx.Exec(ctx, `DELETE FROM contact_email WHERE contact_id = $1`, id)
	}
	if err == nil {
		_, err = tx.Exec(ctx, `DELETE FROM contact_phone WHERE contact_id = $1`, id)
	}
	if err == nil {
		// The sidecar holds their title and employer verbatim.
		_, err = tx.Exec(ctx, `DELETE FROM contact_profile_field WHERE contact_id = $1`, id)
	}
	if err == nil {
		// A resolution key: left behind it keeps binding inbound mail here.
		_, err = tx.Exec(ctx, `DELETE FROM contact_channel_identity WHERE contact_id = $1`, id)
	}
	return err
}

// purgeSubjectPurchases removes everything a licensed data provider left on one
// subject: what it asserted, what those assertions FILLED on the record, and
// the identifying half of the runs that bought them.
//
// The same three statements the Art. 17 path runs, for the same reason:
// anonymize-in-place leaves the contact row standing, so nothing cascades, and
// without these the page would show a bought email and employer beside an
// "Erased Subject" name. The runs are scrubbed rather than deleted — what the
// installation paid is an accounting fact about the installation once the row
// names nobody.
func purgeSubjectPurchases(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	if _, err := tx.Exec(ctx, `DELETE FROM contact_provider_claim WHERE contact_id = $1`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM provider_applied_field WHERE contact_id = $1`, id); err != nil {
		return err
	}
	_, err := tx.Exec(ctx,
		`UPDATE provider_run SET`+storekit.ScrubProviderRunColumns+` WHERE contact_id = $1`, id)
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
	// contact and has no meaning after them, and both acts must clear the same
	// tables — one that cleared it and one that did not would leave the
	// subject's acquisition record standing after an operator was told the
	// contact had been anonymized.
	if _, err := tx.Exec(ctx, `DELETE FROM contact_acquisition_evidence WHERE contact_id = $1`, id); err != nil {
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
	if _, err := tx.Exec(ctx, `DELETE FROM privacy_notice_case WHERE contact_id = $1`, id); err != nil {
		return fmt.Errorf("clear the contact's notice cases: %w", err)
	}
	// Excluding no case: a sweep is nobody's fulfilment.
	if err := retireRightsCases(ctx, tx, id, ids.UUID{}); err != nil {
		return err
	}
	// The same writer the eraser runs, for the reason the acquisition evidence
	// above carries: both acts must clear the same tables, and a review left
	// holding an anonymized subject's addresses is the record still naming
	// somebody an operator was told had been anonymized.
	if err := clearRefusedSendReviews(ctx, tx, id, addresses); err != nil {
		return err
	}
	// The same writer the eraser runs: both acts must clear the same tables, or
	// an anonymized subject keeps a director's sentence about them.
	if err := tombstoneExceptionExplanations(ctx, tx, id, addresses); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM communication_basis WHERE contact_id = $1`, id); err != nil {
		return err
	}
	// The objection SURVIVES an anonymization, re-pinned to the address.
	//
	// This is where the anonymizer parts company with the eraser. The eraser
	// hashes every address onto erasure_suppression, so an erased subject
	// cannot be re-captured at all and a per-contact objection has nothing left
	// to protect. The anonymizer deliberately writes no such row, because an
	// anonymized subject may lawfully return — and if their objection were
	// deleted here they would return unsuppressed, having never withdrawn it.
	// So the contact link is cut and the address kept, which is exactly what the
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
		   SET contact_id = NULL, address = coalesce(address, u.addr)
		  FROM unnest($2::text[]) AS u(addr)
		 WHERE contact_id = $1`, id, addresses); err != nil {
		return err
	}
	// Whatever the detach could not reach — a row whose address is not among
	// the subject's — names a contact who is going, so it goes with them.
	_, err := tx.Exec(ctx, `DELETE FROM communication_suppression WHERE contact_id = $1`, id)
	return err
}
