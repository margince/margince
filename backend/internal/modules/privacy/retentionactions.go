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
// person/erase is registered with a NIL executor: it owns its own transaction
// (the Art. 17 cascade is ~30 statements plus object-store deletes), so apply
// dispatches it before opening one. Nil means "runs outside the transaction",
// never "unsupported" — membership is the key, not the value.
var retentionActions = map[string]retentionExecutor{
	"person/erase":          nil,
	"activity/archive":      (*RetentionService).archiveActivity,
	"activity/erase":        (*RetentionService).eraseActivityContent,
	"deal/archive":          (*RetentionService).archiveDeal,
	"ai_call_payload/erase": (*RetentionService).erasePayload,
	"lead/anonymize":        (*RetentionService).anonymizeLead,
	"person/anonymize":      (*RetentionService).anonymizePerson,
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

func (*RetentionService) anonymizeLead(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	if _, err := tx.Exec(ctx, `
		UPDATE lead SET full_name = 'Anonymized Lead', email = NULL, title = NULL,
		  company_name = NULL, candidate_org_key = NULL, raw = NULL,
		  archived_at = coalesce(archived_at, now())
		WHERE id = $1`, id); err != nil {
		return err
	}
	// The score's explanation goes with the lead it explains. Both tables
	// hold personal data the UPDATE above cannot reach: the retained series
	// embeds activity ids inside its factors JSON, and a manual signal names
	// the colleague who entered it and carries their written reason. This is
	// an ANONYMIZE, not a delete, so the lead row survives and fires no
	// ON DELETE cascade — the FKs on those tables do nothing here, which is
	// why each has to be named (ADR-0105).
	if _, err := tx.Exec(ctx, `DELETE FROM lead_score_history WHERE lead_id = $1`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM lead_manual_signal WHERE lead_id = $1`, id); err != nil {
		return err
	}
	_, err := tx.Exec(ctx,
		`DELETE FROM embedding WHERE entity_type = 'lead' AND entity_id = $1`, id)
	return err
}

func (*RetentionService) anonymizePerson(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	return anonymizePersonRecord(ctx, tx, id)
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
		return s.eraser.ErasePerson(ctx, id, "retention")
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
		return storekit.EmitEventForEntity(ctx, tx, auditID, pol.ObjectType, id, retentionAppliedPayload(pol.Action, &policyID, nil))
	})
}

// eraseActivityContent is the activity/erase action. Transcript free-text is
// the special-category risk; the record of the meeting stays, its content goes
// — including any attached recording/transcript file (objects first, so the
// purge shares the person-erase durability guarantee).
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
		`UPDATE activity SET body = NULL, raw = NULL, subject = $2, archived_at = coalesce(archived_at, now()) WHERE id = $1`,
		id, erasedActivitySubject)
	if err == nil {
		// The verbatim provider payload the parsed copy was made from. It is
		// keyed on (source_system, source_id) — the pair the statement above
		// deliberately keeps — so the join that makes the record replayable is
		// the same join that serves the original back through an Art. 15
		// export. Clearing the parsed copy alone erases nothing.
		//
		// This reaches the captures written under the DOMAIN natural key, which
		// is the mail lane. A channel poller stores its original under the
		// provider's redelivery key instead, and no join from the activity
		// finds it: #2802 carries that lane.
		err = s.purgeRawCaptures(ctx, tx, []ids.UUID{id})
	}
	if err == nil {
		// Provenance outlives the value it describes otherwise: the row names
		// who captured the erased body and from where, and it is registered
		// PII-bearing and SAR-exported. Both sibling erasers delete it
		// (erasuretimeline.go, retentionrestricted.go).
		_, err = tx.Exec(ctx,
			`DELETE FROM field_provenance WHERE object_type = 'activity' AND object_id = $1`, id)
	}
	if err == nil {
		_, err = tx.Exec(ctx,
			`DELETE FROM embedding WHERE entity_type = 'activity' AND entity_id = $1`, id)
	}
	if err == nil {
		err = s.invalidateGraph(ctx, tx, id)
	}
	if err == nil {
		err = s.eraser.eraseAttachments(ctx, tx, "retention: the policy erased this record", causeRetention, `entity_type = 'activity' AND entity_id = $1`, id)
	}
	if err == nil {
		// A proposal read out of this body quotes it verbatim, so it ages out
		// on the body's schedule too. The transcript window (365 days) is the
		// one this bites: the sweep visits a transcript ONCE — its selector
		// requires a body, and this statement removes it — so a quotation left
		// behind here is never revisited by anything.
		err = redactApprovalsCitingActivities(ctx, tx, []ids.UUID{id}, AgedOutSourceWithdrawal)
	}
	if err == nil {
		err = purgeTranscriptReadings(ctx, tx, []ids.UUID{id})
	}
	if err == nil {
		// An outbound message ages out on the schedule of the activity
		// it belongs to: the send log holds the same recipients,
		// subject and body, and a policy that emptied one while the
		// other kept serving them would age out nothing.
		err = redactDeliveries(ctx, tx, []ids.UUID{id}, erasedActivitySubject)
	}
	return err
}

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
		  title = NULL, raw = NULL,
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
	// Read BEFORE the delete below, for the reason the eraser gives at its own
	// copy of this: the LinkedIn ghost sweep identifies rows by this address,
	// and person_social is about to stop holding it.
	var linkedInHandles []string
	if err == nil {
		linkedInHandles, err = collectStrings(ctx, tx,
			`SELECT handle FROM person_social WHERE person_id = $1 AND platform = 'linkedin'`, id)
	}
	if err == nil {
		_, err = tx.Exec(ctx, `DELETE FROM person_social WHERE person_id = $1`, id)
	}
	if err == nil {
		_, err = tx.Exec(ctx, `DELETE FROM person_email WHERE person_id = $1`, id)
	}
	if err == nil {
		_, err = tx.Exec(ctx, `DELETE FROM person_phone WHERE person_id = $1`, id)
	}
	if err == nil {
		// The enrichment sidecar holds the subject's title and employer with
		// the verbatim sentence naming them. Anonymizing the person row above
		// cascades to nothing, so a sweep that skipped this would leave the
		// quote standing beside an "Erased Subject" record.
		_, err = tx.Exec(ctx, `DELETE FROM person_profile_field WHERE person_id = $1`, id)
	}
	if err == nil {
		// Purchased provider values, and the runs that bought them. Same
		// reasoning as the sidecar above and the same statements the erasure
		// path uses: anonymize-in-place cascades to nothing, so without these
		// the person page would show a bought email and employer beside an
		// "Erased Subject" name.
		_, err = tx.Exec(ctx, `DELETE FROM person_provider_claim WHERE person_id = $1`, id)
	}
	if err == nil {
		_, err = tx.Exec(ctx,
			`UPDATE provider_run SET`+storekit.ScrubProviderRunColumns+` WHERE person_id = $1`, id)
	}
	if err == nil {
		// The channel identity is a resolution key on the subject as
		// much as their address: left behind, it would keep binding
		// inbound messages to the row this sweep just anonymized.
		_, err = tx.Exec(ctx,
			`DELETE FROM person_channel_identity WHERE person_id = $1`, id)
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
		err = scrubPersonGraphTraces(ctx, tx, id, subjectEmails, subjectName, linkedInHandles)
	}
	return err
}
