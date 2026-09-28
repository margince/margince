// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// What an Art. 17 erasure does to the subject's TIMELINE and to everything
// derived from it. Kept apart from the subject's own rows (erasure.go) because
// the questions differ: those rows are the subject's record, while these are
// other contacts's records that happen to contain them, plus the machine
// artifacts built on top — a raw capture, an embedding, an interaction edge.
// Nothing here can simply be deleted without asking what else it holds.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// redactSubjectTimeline erases the subject's free text from the activity
// timeline: subject/body of every subject-only activity are wiped (the
// GENERATED search_tsv refreshes from the now-empty text, so the erased
// name is no longer full-text searchable). The subject's attachments are
// purged separately by eraseAttachments (objects first); this handles only
// the timeline text and its field-level provenance. It returns the
// redacted activity ids so the caller can tombstone each record's own
// audit spine.
// subjectTimelineLockSQL selects every row this erasure will judge, FOR UPDATE.
//
// The three id sets come from the same fragments the destroy and the hold
// select from, so a row cannot be judged by one spelling and locked by another.
// Only the NUMBERING is adapted: the fragments are written for the destroy's
// argument list, where the channel keys are $6 behind the floor and the
// tombstone name, and this statement wants none of those. Renumbering one
// placeholder is the smallest adaptation that keeps one definition of the row
// set; TestTheTimelineLockBindsEveryPlaceholderItNames holds it to three.
var subjectTimelineLockSQL = `
	SELECT a.id FROM activity a
	WHERE a.id IN (` + subjectOnlyActivities + `)
	   OR a.id IN (` + unlinkedSubjectMail + `)
	   OR a.id IN (` + strings.ReplaceAll(unlinkedSubjectChannel, "$6", "$3") + `)
	ORDER BY a.id
	FOR UPDATE`

// lockSubjectTimeline takes a row lock on every activity this erasure will
// judge, BEFORE it judges any of them.
//
// The judgement is erase-or-hold, and it reads `retention_class` — which a deal
// win or a sent offer stamps on the same rows, in its own transaction
// (activities.StampCorrespondenceForDeal). Without this lock the two can
// interleave: the erasure reads a NULL class, destroys the correspondence, and
// the qualification commits afterwards. The row is gone either way, so nothing
// readable survives — but A165/ADR-0114 §1 says that correspondence was to be
// HELD, and the obligation was not honoured (issue #1618).
//
// The lock is what makes the race have a winner. Under READ COMMITTED a
// qualification already holding these rows makes this statement wait, and the
// erasure then re-reads the class it just waited for — so an in-flight
// qualification wins and its correspondence is held. One that starts after this
// lock waits for the commit and stamps a row already erased, which is the
// residual and is the honest one: by then there was nothing to hold.
//
// NO FLOOR PREDICATE, deliberately. The floor is what the judgement uses to
// decide, and a lock that pre-applied it would leave exactly the rows whose
// class is about to change unlocked — the ones this exists for.
//
// The same three id sets the destroy and the hold select from, so a row cannot
// be judged by one spelling and locked by another.
func lockSubjectTimeline(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, emails []string, channelKeys []string) error {
	// ORDER BY id is BEST EFFORT against a deadlock between two erasures whose
	// subjects share activities, and is written down as best effort because it
	// is not a guarantee: Postgres does not promise that `FOR UPDATE` acquires
	// in the sort's order, only that the rows come back in it. The plan it
	// usually chooses locks after sorting, which is why this helps at all.
	//
	// When it does not hold, the loser gets 40P01 and EraseContact returns it —
	// the whole transaction rolls back, so no half-erasure commits and the
	// request is safe to re-issue. That is the honest cost, and it is small
	// because the collision needs two erasures overlapping on the same
	// activities at the same moment. Retrying inside the eraser was considered
	// and left alone: a retry loop around a transaction this long belongs to
	// whoever decides the policy for every such write, not to this one.
	_, err := tx.Exec(ctx, subjectTimelineLockSQL, contactID, emails, channelKeys)
	return err
}

func redactSubjectTimeline(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, emails []string, channelKeys []string, floorInterval string, floorAnchor bool) ([]ids.UUID, error) {
	// Redact the subject's own timeline rows, the unlinked mail about them — in
	// both directions, captured and sent (unlinkedSubjectMail) — and the
	// unlinked CHANNEL messages from them (unlinkedSubjectChannel), but shield
	// commercial correspondence younger than the
	// statutory floor: the floor filters the row being updated (aliased `a`),
	// so it covers all three id sets in one pass. $1 contact, $2 addresses, $3/$4
	// the floor interval + anchor, $5 the tombstone name, $6 the subject's
	// `provider:account` channel keys.
	//
	// source_id and thread_key are cleared for channel rows and ONLY for channel
	// rows, because for those two columns the identifier IS the subject: the
	// capture writes source_id `botID:accountID:messageID` and thread_key
	// `provider:botID:accountID`, and a private chat's id is the human's own
	// Telegram id. Leaving them was a silent Art. 17 hole on the ordinary path —
	// no race required — that emptying subject/body/raw did nothing about. A
	// mail row keeps its natural key: a message-id does not name the subject.
	//
	// Clearing them costs nothing that is still reachable. The pair is the
	// capture idempotency key, but the same erasure arms the suppression list,
	// and Sink.Upsert refuses a suppressed account before it writes — so there
	// is no redelivery left for the key to deduplicate.
	rows, err := tx.Query(ctx, `
		UPDATE activity a SET subject = $5, body = NULL, raw = NULL,
		  counterparty_email = NULL,
		  -- What a classifier concluded the message MEANT goes with the words it
		  -- read. A verdict saying a subject replied negatively is a claim about
		  -- them, derived from text this same statement is emptying, and leaving
		  -- it would keep the conclusion after destroying the evidence.
		  reply_verdict = NULL, reply_verdict_at = NULL, reply_verdict_by = NULL,
		  -- Same reason, one step smaller: the language was READ from the words
		  -- being emptied here, so it is a fact about erased text rather than
		  -- about the row.
		  language = NULL,
		  -- The byline an import carried in from the system this row came from:
		  -- a human's name in free text, written by neither party to the
		  -- exchange. It is content about somebody rather than the record of
		  -- who the exchange was with, so it goes with the words.
		  --
		  -- The repair's own ledger holds a second copy, cleared by the UPDATE
		  -- that follows this statement. A name erased from the activity and
		  -- left standing in the bookkeeping is still readable.
		  source_author_name = NULL,
		  source_id = CASE WHEN a.source_system || ':' || split_part(coalesce(a.thread_key, ''), ':', 3) = ANY($6)
		                   THEN NULL ELSE a.source_id END,
		  thread_key = CASE WHEN a.source_system || ':' || split_part(coalesce(a.thread_key, ''), ':', 3) = ANY($6)
		                    THEN NULL ELSE a.thread_key END,
		  archived_at = coalesce(a.archived_at, now())
		WHERE (a.id IN (`+subjectOnlyActivities+`)
		    OR a.id IN (`+unlinkedSubjectMail+`)
		    OR a.id IN (`+unlinkedSubjectChannel+`))
		  `+correspondenceFloorPredicate(3, 4)+`
		RETURNING a.id`, contactID, emails, floorInterval, floorAnchor, erasedName, channelKeys)
	if err != nil {
		return nil, err
	}
	redacted, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, err
	}
	if err := clearAttributionLedgerNames(ctx, tx, "activity", redacted); err != nil {
		return nil, err
	}
	// The redacted rows' field-level provenance goes with the fields it
	// annotated — origin metadata must not outlive the erased text. A
	// floor-shielded correspondence row is excluded here too: its provenance
	// stays with the evidence it annotates.
	if _, err := tx.Exec(ctx, `
		DELETE FROM field_provenance
		WHERE object_type = 'activity' AND object_id IN (`+subjectOnlyDestroyable+`)`,
		contactID, floorInterval, floorAnchor); err != nil {
		return nil, err
	}
	// And how the reply verdict came to be what it was. DELETED rather than
	// nulled, the way ai_feedback is: a history of judgements about somebody
	// nobody may now assert anything about has nothing left to record. The
	// table's own ON DELETE CASCADE does not reach it — this erasure UPDATES
	// the activity in place and never removes the row.
	//
	// Through the shared helper, which scopes by CONTACT rather than by the
	// floor-bounded activity set this function redacts. The wider scope is
	// deliberate: the floor holds back a message's TEXT for a statutory period,
	// and a conclusion about what that message meant is not the message. Keeping
	// the verdict to satisfy a correspondence floor would preserve our reading
	// of the subject's words on the ground that we must preserve the words.
	if err := deleteReplyVerdictHistoryFor(ctx, tx, contactID); err != nil {
		return nil, err
	}
	// And what we concluded our own replies did about what they asked, which is
	// the same kind of claim over the same emptied words.
	if err := deleteRequestSettlementsFor(ctx, tx, contactID, emails); err != nil {
		return nil, err
	}
	// And what was read out of those conversations as promised, asked or
	// decided — each row carrying the sentence it was read from, so this is the
	// subject's own words and not only a conclusion about them.
	if err := deleteConversationClaimsFor(ctx, tx, contactID); err != nil {
		return nil, err
	}
	// And the handoffs naming them, for the same reason and on the same act.
	if err := deleteSubjectHandoffs(ctx, tx, contactID); err != nil {
		return nil, err
	}
	return redacted, nil
}

// subjectActivityEmbeddingsDelete drops the vectors of the subject's own
// timeline rows. A held activity keeps its embedding along with its text: the
// vector is derived from evidence the hold freezes, and destroying it while
// the text stands would be a partial spoliation with nothing to show for it.
// clearAttributionLedgerNames removes the byline the author repair wrote into
// its own bookkeeping for these activities.
//
// The repair's ledger holds a SECOND copy of the name the caller has just
// cleared off the message. It is the same free text about the same human, so an
// erasure that stopped at the activity would leave the erased name sitting in
// the ledger beside it, readable by anything that reads that table.
//
// THE DIGEST GOES WITH THE NAME. `payload_hash` is an unkeyed SHA-256 over the
// author id and that same name, and a hash of a human name is not anonymous: the
// candidate set is a staff list, so anyone holding the ledger can hash a few
// hundred names and match one. Leaving it would keep a re-identifiable copy of
// the very string the statement above just cleared. Emptied rather than nulled —
// the column is NOT NULL, and ” is what this package's other erasures write to
// a NOT NULL text column.
//
// THE BATCH LABEL GOES TOO, and this is the second answer to that question.
// The first was a wire pattern meant to stop a label naming anybody, and it
// does not: `alice-smith` and `A.Smith` both satisfy it. The label is free text
// an operator types, so no cheap syntax makes it safe, and a PII declaration
// resting on one would be false however carefully it were worded. Nothing reads
// this column — it is written with the row and never queried — so clearing it
// on an erased record costs an operator one label on a row whose content is
// already destroyed, and buys a declaration that is simply true.
//
// The ROW stays and the REVISION with it: the ledger's job is to say this
// record has already been reached, and at which revision, so a resumed run
// neither redoes it nor loses its place.
//
// Stated exactly, because a looser version of this sentence stood here and
// overclaimed: keeping the revision refuses an offer at an EQUAL OR LOWER one.
// It does not by itself refuse a higher one. What stops a later batch writing a
// name back onto an erased message is the erasure ARCHIVING the activity — the
// repair's store refuses an archived row outright, before any comparison.
//
// `source_author_id` stays, and the honest statement of why is narrower than
// any version of this comment has yet managed. It is NOT "what a re-run
// compares against" — that was wrong, and review caught it: the repair compares
// the activity's own two author columns, under that row's lock, and never reads
// this one. What it actually is: a seat id, ordinarily a colleague's account
// rather than the erased subject's, kept because an erasure of somebody's
// correspondence is not obviously an instruction to forget which colleague
// wrote it.
//
// Nothing here PROVES the two are different. An author who is also a contact
// being erased would leave their own seat id standing. Whether that satisfies
// Art. 17 is a controller's ruling, not this function's, and it is open.
//
// ONE spelling for all three paths that clear it: the Art. 17 erasure, the
// retention sweep's activity/erase, and the restriction lift. The neighbours
// here already carry the scar of the alternative — purgeContentDerivedFrom is
// shared for exactly this reason, after two copies of a content list went out of
// step and the shorter one missed the provider original.
// `objectType` names which kind of record these ids are, because the ledger is
// keyed on (object_type, object_id) and two tables may hold the same uuid. It
// was the literal 'activity' while the repair wrote nothing else; the record
// repair reaches five more types, and a clear that still named only activities
// would leave a contact's byline standing under its own key.
func clearAttributionLedgerNames(ctx context.Context, tx pgx.Tx, objectType string, objectIDs []ids.UUID) error {
	if len(objectIDs) == 0 {
		return nil
	}
	_, err := tx.Exec(ctx, `
		UPDATE source_attribution_repair SET source_author_name = NULL, payload_hash = '', batch_ref = ''
		 WHERE object_type = $1 AND object_id = ANY($2)`, objectType, objectIDs)
	return err
}

var subjectActivityEmbeddingsDelete = `
		DELETE FROM embedding e USING activity_link l
		WHERE e.entity_type = 'activity' AND l.contact_id = $1 AND e.entity_id = l.activity_id` +
	notTransitivelyHeld("l.activity_id")

// purgeDerivedTraces removes what the system DERIVED from the subject and
// arms the suppression list. Raw capture is purged two ways: by email here
// (crude on purpose — over-deleting evidence is recoverable, under-deleting
// PII is not) and by channel identity in purgeChannelRawCapture
// (erasure_channels.go, kept apart for file length). Embeddings of
// activities on the subject's timeline embed text ABOUT them; the vector
// store must not keep what a similarity probe could partially reconstruct.
func purgeDerivedTraces(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, displayName string, emails []string, identities []channelIdentity, erased erasedCitations) (rawPurged, aiPayloadsPurged int64, err error) {
	for _, email := range emails {
		tag, execErr := tx.Exec(ctx,
			`DELETE FROM raw_capture WHERE payload::text ILIKE '%' || $1 || '%' ESCAPE '\'`,
			storekit.EscapeLike(email))
		if execErr != nil {
			return 0, 0, execErr
		}
		rawPurged += tag.RowsAffected()
	}
	// The 24-hour capture trace, when the deployment enabled payload capture.
	// The sweep bounds exposure to a day; it does not ANSWER an erasure made
	// inside that day, and a request honoured everywhere except one diagnostic
	// table is not honoured.
	//
	// Exact equality rather than the ILIKE the lanes around it use: this column
	// is written normalized (lower-cased, trimmed) and indexed, so the crude
	// content match those need — they search whole payloads — buys nothing here
	// and would scan.
	// TWO lanes, because two columns can name the subject. The address column is
	// exact equality — it is written normalized and indexed. The SUBJECT is free
	// text from the provider's header, and it routinely carries somebody's
	// address or name ("Re: intro — alice@acme.test"): a message FROM another
	// sender can name the subject in its own subject line, so an address-only
	// purge leaves that behind, and a trace written after the erasure would even
	// add it back. Same ILIKE shape as the raw_capture lane above, and crude for
	// the same stated reason — over-deleting a diagnostic row is recoverable,
	// under-deleting personal data is not.
	for _, email := range emails {
		if _, execErr := tx.Exec(ctx, `
			DELETE FROM capture_trace
			 WHERE counterparty = lower($1)
			    OR subject ILIKE '%' || $2 || '%' ESCAPE '\'`,
			email, storekit.EscapeLike(email)); execErr != nil {
			return 0, 0, execErr
		}
	}
	channelRawPurged, err := purgeChannelRawCapture(ctx, tx, identities)
	if err != nil {
		return 0, 0, err
	}
	rawPurged += channelRawPurged
	// The trace's OTHER lane. The loop above matches an address; a counterparty
	// the pipeline knew by a provider account is named there by their display
	// name instead, which no address can reach.
	if _, err := purgeChannelCaptureTrace(ctx, tx, displayName, identities); err != nil {
		return 0, 0, err
	}
	if _, err := tx.Exec(ctx, subjectActivityEmbeddingsDelete, contactID); err != nil {
		return 0, 0, err
	}
	// Captured AI payloads (Layer 3) go two ways, and the first is the one
	// that reaches a transcript.
	//
	// BY CITATION: a call that said what record it was about names it on
	// ai_call, so every payload of a call made about this contact — or about an
	// activity or lead this erasure just wiped with them — is deleted whatever
	// its text says.
	// That is the difference between destroying what mentioned their address
	// and destroying what was about them, and it is the only lane that reaches
	// a meeting transcript, which names its speakers rather than addressing
	// them and may never spell an address at all.
	//
	// BY CONTENT, unchanged: any opt-in body naming one of the subject's
	// addresses. The citation does NOT replace it and is not the boundary — a
	// call whose input spans several records names none, by design, and is
	// reached by this match or not at all. The residual is exactly what it was
	// for every task that names no record.
	//
	// Either way the ai_call metadata row survives (the FK is ON DELETE CASCADE
	// from ai_call, never the reverse), and both are crude on purpose:
	// over-deleting captured telemetry is recoverable, under-deleting personal
	// data is a violation.
	//
	// No channel-identity lane here, unlike raw_capture above — see
	// purgeChannelRawCapture's comment (erasure_channels.go) for why
	// ai_call_payload cannot safely take the same match.
	citedTag, err := tx.Exec(ctx, `
		DELETE FROM ai_call_payload p
		 USING ai_call c
		 WHERE p.ai_call_id = c.id
		   AND ((c.subject_type = 'contact' AND c.subject_id = $1)
		     OR (c.subject_type = 'activity' AND c.subject_id = ANY($2))
		     OR (c.subject_type = 'lead' AND c.subject_id = ANY($3)))`,
		contactID, erased.activities, erased.leads)
	if err != nil {
		return 0, 0, err
	}
	aiPayloadsPurged += citedTag.RowsAffected()
	for _, email := range emails {
		tag, execErr := tx.Exec(ctx, `
			DELETE FROM ai_call_payload
			WHERE request_payload::text ILIKE '%' || $1 || '%' ESCAPE '\'
			   OR response_payload::text ILIKE '%' || $1 || '%' ESCAPE '\'`,
			storekit.EscapeLike(email))
		if execErr != nil {
			return 0, 0, execErr
		}
		aiPayloadsPurged += tag.RowsAffected()
	}
	for _, email := range emails {
		if _, err := tx.Exec(ctx, `
			INSERT INTO erasure_suppression (kind, value_hash)
			VALUES ('email', $1)
			ON CONFLICT DO NOTHING`, storekit.SuppressionHash(email)); err != nil {
			return 0, 0, err
		}
	}
	return rawPurged, aiPayloadsPurged, nil
}

// erasedCitations are the OTHER records this erasure destroyed alongside the
// contact, named so a model call that cited one of them is purged with it. A
// draft written for a lead and a reading of a meeting hold the subject's words
// as surely as a call that named the contact.
type erasedCitations struct {
	activities []ids.UUID
	leads      []ids.UUID
}

// deleteSubjectHandoffs drops every SDR handoff naming the subject, and with it
// the transitions that cascade from each one.
//
// A handoff carries the subject's id, a free-text note one seat wrote about
// them, and a judgement — accepted, or refused for this reason — that contacts
// made about this contact. Erasure UPDATEs the contact in place and never removes
// the row, so the ON DELETE CASCADE on lead_id and contact_id never fires for an
// Art. 17 request: this statement is what actually reaches them.
//
// DELETED rather than nulled, on the ground ai_feedback records: a decision
// about somebody nobody may now assert anything about has nothing left to say.
// sdr_handoff_event goes with it through its own cascade, which DOES fire here
// because this is a real delete.
func deleteSubjectHandoffs[ID ids.UUID | ids.ContactID](ctx context.Context, tx pgx.Tx, contactID ID) error {
	// The transitions first, by name. sdr_handoff_event cascades from the delete
	// below and would go anyway — but a cascade is invisible to the census that
	// asks whether Art. 17 reaches a table, and invisible to the next reader
	// auditing what this erasure destroys. Naming it costs one statement and
	// makes both able to see it.
	if _, err := tx.Exec(ctx, `
		DELETE FROM sdr_handoff_event
		WHERE handoff_id IN (
		      SELECT id FROM sdr_handoff
		       WHERE contact_id = $1
		          OR lead_id IN (SELECT id FROM lead WHERE promoted_contact_id = $1))`,
		contactID); err != nil {
		return fmt.Errorf("privacy: clearing the subject's handoff transitions: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM sdr_handoff
		WHERE contact_id = $1
		   OR lead_id IN (SELECT id FROM lead WHERE promoted_contact_id = $1)`,
		contactID); err != nil {
		return fmt.Errorf("privacy: clearing the subject's handoffs: %w", err)
	}
	return nil
}

// deleteReplyVerdictHistoryFor drops every judgement this installation recorded
// about what one contact's replies meant — the classifier's own and every human
// correction after it.
//
// ONE spelling, called by both acts. The Art. 17 timeline redaction reaches
// these rows through the activities it empties; the anonymize reaches them
// through the contact, because it empties no activity at all. Two statements
// would be two answers to "which rows belong to this subject", and the anonymize
// parity gate exists precisely because those two answers drift.
//
// Scoped through activity_link, the same join the SAR export uses to decide
// which activities are this contact's.
//
// The type parameter is there because the two callers hold the subject id in
// different types — the erasure spine in ids.ContactID, the retention sweep in
// ids.UUID — and a second function per type would be a second answer to "which
// rows belong to this subject". The constraint keeps it to those two rather
// than admitting any id at all.
func deleteReplyVerdictHistoryFor[ID ids.UUID | ids.ContactID](ctx context.Context, tx pgx.Tx, contactID ID) error {
	if _, err := tx.Exec(ctx, `
		DELETE FROM activity_reply_verdict_history
		WHERE activity_id IN (SELECT l.activity_id FROM activity_link l WHERE l.contact_id = $1)`,
		contactID); err != nil {
		return fmt.Errorf("privacy: clearing the subject's reply verdicts: %w", err)
	}
	return nil
}

// deleteRequestSettlementsFor drops what this installation concluded about
// whether our replies settled what one contact asked of us.
//
// What it destroys is a reading OF the subject's correspondence — a verdict,
// and in the still_owed case a sentence the model wrote naming what we still
// owe them — so it goes with the words it was read from rather than outliving
// them. One spelling for both acts, typed over the two id forms the callers
// hold, like the reply verdicts above.
//
// BOTH ARMS, and the second is the one that matters. A link walk alone would
// leave every settlement about mail linked to nobody — and since ADR-0072
// stopped creating a counterparty for every captured message, that class is
// ordinary rather than exotic: a deferred or still-unsure sender produces
// activities with no contact link at all, and the settlement pass is happy to
// judge them because its candidate read requires no link either. It selects
// through the same two selectors redactSubjectTimeline empties the timeline
// through, so the rows this destroys are the rows whose words are destroyed
// beside it.
//
// Keyed on the REQUEST, which is the row the settlement hangs from. The
// judged-through reply is an activity of ours on the same thread, and deleting
// by the request alone is enough: the table holds one row per request, so no
// settlement survives its own subject.
func deleteRequestSettlementsFor[ID ids.UUID | ids.ContactID](ctx context.Context, tx pgx.Tx, contactID ID, emails []string) error {
	if _, err := tx.Exec(ctx, `
		DELETE FROM activity_request_settlement
		WHERE request_activity_id IN (`+subjectOnlyActivities+`)
		   OR request_activity_id IN (`+unlinkedSubjectMail+`)`,
		contactID, emails); err != nil {
		return fmt.Errorf("privacy: clearing the subject's request settlements: %w", err)
	}
	return nil
}
