// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// PII reach as a fitness function. tableownership_test.go proves a package
// only writes tables it owns; it says NOTHING about whether Art. 17 erasure
// reaches every table that holds a data subject. Without that guarantee the
// activity timeline and attachments survive an erasure verbatim, still
// full-text searchable. This test closes it: piiTables is the explicit
// registry of PII-bearing tables, and every entry must be a WRITE target of
// privacy/erasure.go (so erasure reaches it) and — unless it is an opaque
// derived artifact — a READ target of privacy/sar.go (so an Art. 15 SAR
// discloses it). A new PII table that skips erasure or SAR fails here instead
// of shipping a silent leak.

import (
	"regexp"
	"sort"
	"strings"
	"testing"
)

// piiHandling declares how erasure and SAR must reach a PII table.
type piiHandling struct {
	// erasureWrite: erasure.go must UPDATE/DELETE this table (redact or purge).
	erasureWrite bool
	// retentionErase: the nightly retention sweep is this table's ONLY
	// eraser. True only where the row carries no linkage back to a subject
	// for the Art. 17 cascade to walk — the cascade starts at a person, so a
	// table it structurally cannot reach must still be erased by SOMETHING,
	// and the sweep is that something. It is a separate field rather than a
	// second way to satisfy erasureWrite because the two are different
	// promises: "erased when the subject asks" and "erased when the clock
	// says", and only the first answers an Art. 17 request.
	retentionErase bool
	// retentionErasures names the SET assignments that CONSTITUTE the sweep's
	// erasure of this table, normalized to single spaces. Required wherever
	// retentionErase is set: "the sweep writes this table" is satisfied by any
	// statement at all, so a version bump or a metadata touch left behind after
	// the plaintext wipe was deleted would keep that claim true while the
	// content survived its window. These are the assignments the erasure IS.
	retentionErasures []string
	// retentionKeeps names columns the sweep deliberately does NOT clear on this
	// table, where a sibling eraser does. retentionErasures can only report that
	// a declared assignment went missing; it says nothing about one that
	// APPEARS, so a decision to keep a column lives in a comment and reverses
	// silently the first time somebody "completes" the statement from the
	// fuller list beside it. That is why the data-layer guard
	// activity_restriction_lift_erases exists at all. Naming
	// the column here makes reversing the decision fail rather than pass, so it
	// has to be an edit to this line and not a quiet widening.
	retentionKeeps []string
	// retentionPurge names the PREDICATES the sweep's deletes of this table
	// carry, one per act. The table is destroyed by the row here rather than by
	// columns of it, so retentionErasures has no SET clauses to name.
	//
	// Predicates rather than a bare "the sweep deletes this table", because
	// several actions delete from one table and a table-level claim is
	// satisfied by whichever of them survives: field_provenance is purged by
	// person/anonymize AND by activity/erase, so a table-level flag would go on
	// passing after either was deleted. Each declared predicate must be carried
	// by a delete of its own.
	retentionPurge []string
	// sarRead: SAR assembly must read this table into the export package.
	// False only for opaque derived artifacts (vectors) that carry no
	// human-readable PII to hand back — they are purged, never exported.
	sarRead bool
	// sarForbidden: SAR assembly must NOT read this table. Its own promise,
	// not the absence of sarRead's: leaving a table merely unregistered for
	// reads means a future AssembleSAR query silently passes this gate, which
	// is the difference between an invariant in a comment and a control. Set
	// it wherever the row holds something an Art. 15 package must not carry —
	// a live credential, say — and the gate fails the moment SAR touches it.
	sarForbidden bool
}

// piiTables is the registry of every table holding data about a subject.
// "Holds a subject's PII" is a domain judgment, not a schema property —
// attachment/raw_capture/embedding carry it with no person FK, while
// person-referencing tables like relationship and the consent proof logs
// deliberately do not qualify (kept under Art. 5 accountability). So, like
// tableOwners in the ownership gate, this map IS the hand-maintained
// artifact: a table is registered here as the one act that declares it
// PII-bearing, and the test then proves erasure and SAR reach it. Keep it
// in step with the subject data in data-model §3.
var piiTables = map[string]piiHandling{
	"person":        {erasureWrite: true, sarRead: true},
	"person_email":  {erasureWrite: true, sarRead: true},
	"person_social": {erasureWrite: true, sarRead: true},
	"person_phone":  {erasureWrite: true, sarRead: true},
	// The channel identity binds a human to their Telegram account: the
	// provider's user id for them plus the @username they message under. Both
	// identify the subject as directly as an address does, and the id is the
	// key a re-capture would resurrect them by — so erasure purges it, the
	// suppression list keeps holding it, and Art. 15 hands it back.
	"person_channel_identity": {erasureWrite: true, sarRead: true},
	"lead":                    {erasureWrite: true, sarRead: true},
	// Who was IN each interaction (ACT-DDL-3). It names the subject twice —
	// by person_id and by the raw address of a party who never became a
	// record — so erasure nulls both and Art. 15 hands back the fact that
	// they were a party to those conversations.
	"activity_participant": {erasureWrite: true, sarRead: true},
	// LinkedIn ghosts (CG-DDL-2) hold a third party's name, employer and
	// sometimes address, imported from a colleague's export without that
	// person being asked. Erasure deletes them; Art. 15 hands them back,
	// because "you appear in someone's imported address book" is exactly the
	// kind of holding a subject would not otherwise discover.
	"linkedin_connection": {erasureWrite: true, sarRead: true},
	// The interaction projection (CG-DDL-1): derived, but derived from data an
	// erasure removes, and it holds who corresponded with the subject and how
	// often. Purged, never exported — like the embedding, it is a machine
	// artifact rather than anything the subject supplied.
	"graph_interaction_edge": {erasureWrite: true, sarRead: false},
	// The contact↔contact projection: derived like its sibling above, and it
	// names the subject on EITHER end of every row — the erasure delete must
	// cover both endpoint columns, or the subject survives on the far side of
	// somebody else's edge. Purged, never exported, for the same reason.
	"graph_contact_edge": {erasureWrite: true, sarRead: false},
	// The Art. 17 cascade reaches activity, so erasureWrite is what this table
	// promises. retentionErasures is declared ANYWAY, because the nightly
	// sweep's activity/erase action is a SECOND eraser of the same content and
	// the two have already drifted once: the sweep cleared `body` and left
	// `raw`, the re-parseable original, which is the same content one parse
	// away. Nothing populates the column today, so nothing would have reported
	// it — the first connector to store an original is what would have made it
	// a leak.
	//
	// `counterparty_email` is absent from this list ON PURPOSE. Both sibling
	// erasers clear it; the sweep keeps it, because the retention action's
	// contract is that the RECORD of the meeting survives and its content goes,
	// and who it was with is the record. Declared here so the difference is a
	// decision somebody can find rather than an omission nobody can date.
	"activity": {
		erasureWrite: true,
		sarRead:      true,
		retentionErasures: []string{
			"body = NULL",
			"raw = NULL",
			// The tombstone is part of the erasure, not decoration beside it:
			// the row keeps saying a meeting happened while saying nothing
			// about what was in it, and a sweep that stopped writing it would
			// leave the old subject line standing.
			"subject = $2",
		},
		// Columns a sibling eraser clears and the sweep deliberately does not.
		// counterparty_email and the channel identity are the same ruling: the
		// retention action keeps the RECORD of the event, and who it was with
		// is the record rather than its content. field_provenance is not here
		// because it is a different table — see its own entry.
		retentionKeeps: []string{"counterparty_email", "source_id", "thread_key"},
	},
	"attachment": {erasureWrite: true, sarRead: true},
	// The verbatim provider payload — full headers and body. The sweep's
	// activity/erase destroys it along with the parsed copy it duplicates,
	// because the two are joined on (source_system, source_id) and the erase
	// deliberately KEEPS that pair on the activity row: clearing activity.body
	// while this survived erased nothing, and sar.go exports this table by
	// email match.
	//
	// Not registered retentionPurge, and the reason is a module boundary rather
	// than a decision: this is capture's table, so the delete lives in
	// capture.PendingStore.PurgeRawCaptureTx and reaches the sweep through the
	// seam compose injects. It is therefore outside retentionSweepFiles, and
	// adding capture's file to that list would let capture's own noise
	// redaction — which also writes activity — satisfy the sweep's declarations
	// for the table beside this one. Over-recognition is the failure that list
	// exists to avoid, so the obligation is held where it can be seen whole:
	//
	// Held by: TestTheRetentionSweepDestroysTheProviderOriginalToo and
	// TestTheEraseActionRefusesWithoutItsPurger
	// (backend/internal/compose/integration/retentionrawcapture_integration_test.go)
	"raw_capture": {erasureWrite: true, sarRead: true},
	"embedding":   {erasureWrite: true, sarRead: false}, // opaque vector: purged, never exported
	// The 24-hour capture trace names its counterparty — by address for mail,
	// by DISPLAY NAME for a channel where there is no address to write — and
	// keeps a clamped subject line. It was absent from this registry for as
	// long as capture.trace_payloads defaulted off, which is exactly how the
	// channel erasure lane came to be missing with nothing failing: an
	// unregistered table is one this gate never asks about.
	//
	// sarRead false, like embedding above and for a different reason. The row
	// is an operational diagnostic that deletes itself within the day and says
	// nothing about the subject a message of theirs does not already say — the
	// activity IS the Art. 15 answer, and the trace is the record of what the
	// pipeline decided about it. Erased when they ask, never exported.
	"capture_trace": {erasureWrite: true, sarRead: false},
	// Field-level provenance names who captured which of the subject's
	// fields from where — subject-linked metadata (B-E02.12).
	//
	// The sweep destroys these by the ROW, under two predicates that select
	// different rows: person/anonymize takes the SUBJECT's field origins,
	// activity/erase takes one erased message's. Both are declared, so deleting
	// either act fails rather than being covered by the other — provenance
	// naming who captured a value and from where outlives the value itself
	// otherwise, and this table is SAR-exported.
	"field_provenance": {
		erasureWrite:   true,
		sarRead:        true,
		retentionPurge: []string{"object_type = 'person'", "object_type = 'activity'"},
	},
	// The enrichment sidecar holds the subject's title, phone, employer and
	// public profile URL, each with the verbatim sentence it was read from —
	// their data twice over, the value and the quote naming them. Nothing
	// cascades to it, because anonymize-in-place leaves the person row
	// standing, so erasure has to reach it by statement.
	"person_profile_field": {erasureWrite: true, sarRead: true},
	// What a licensed data provider asserted about the subject and this
	// installation retained (ADR-0101). Bought from a third party rather than
	// given by them, which makes it disclosable twice over — the values and
	// the fact that they were purchased.
	"person_provider_claim": {erasureWrite: true, sarRead: true},
	// What a purchase FILLED on the record. It keeps the value it wrote for a
	// plain column — the title, the profile URL — because the revert has
	// nothing else to recognise the value by, and a hash of an address or a
	// handle is a reversible fingerprint rather than a safer store. So the row
	// carries the subject's own data and is erased with the claims beside it.
	// Art. 15 discloses it: a subject asking what we hold is owed the fact that
	// a purchase put something on their record, not only that we bought it.
	"provider_applied_field": {erasureWrite: true, sarRead: true},
	// The run that bought it. Erasure SCRUBS rather than deletes: the row
	// stops naming anybody (person_id, fingerprint, job id, requester,
	// snapshot) while the spend it records survives, because what the
	// installation paid is an accounting fact about the installation once it
	// names no one (PI-AC-8). Art. 15 hands back the purpose and the
	// categories, since a values-only export would say what we hold while
	// hiding that we went out and bought it.
	"provider_run": {erasureWrite: true, sarRead: true},
	// The correction ledger holds what a human typed OVER what the system
	// inferred — a title, a phone number, a free-text note about the subject.
	// The verdict is a decision a person made about them, so Art. 15 hands it
	// back, and Art. 17 deletes rather than nulls it: a verdict with no value
	// is not a verdict, and suppressions about a person nobody may now assert
	// anything about have nothing left to suppress.
	"ai_feedback": {erasureWrite: true, sarRead: true},
	// The capture disposition ledger keys on the subject's own address and
	// keeps the display name their mail arrived with (CAP-DDL-8).
	"capture_pending_counterparty": {erasureWrite: true, sarRead: true},
	// The send log keeps a second copy of an outbound message's recipient
	// addresses, subject line and body, scrubbed with the activity it
	// transmitted and exported alongside it.
	"comms_outbound": {erasureWrite: true, sarRead: true},
	// The voice learning signal keeps the model's drafted text
	// (generated_original) in plaintext, which is correspondence about a
	// subject. It names no person, activity or subject, deliberately: the row
	// exists to say whether the owner sent the machine's words or reworded
	// them, and linking it to the recipient would put a second copy of their
	// mail behind a join Art. 17 would have to find. So the time-based sweep
	// (privacy/retention.go, 180 days) is its eraser, not the cascade.
	//
	// The SAR exclusion holds for a draft that was SENT: the subject's copy of
	// that correspondence is the activity, which AssembleSAR already exports,
	// and the sent body itself is classified and discarded, never stored. It
	// does NOT hold for a draft that was rejected or abandoned — no activity
	// exists, so generated_original is the only copy of words written about the
	// subject, and it is held for up to 180 days with no Art. 15 export path.
	// That bound is a property of storing the drafted text at all
	// (RecordDraftedSignal), not of any one outcome recorded against it, and
	// whether Art. 15 must reach an unsent draft is open against the spec.
	"voice_learning_signal": {
		retentionErase: true,
		retentionErasures: []string{
			"generated_original = NULL",
			"final_text = NULL",
			"content_erased_at = now()",
		},
		sarRead: false,
	},
	// A staged proposal holds a whole composed message before any activity
	// exists — for a held draft, the addressee and the body — and since core
	// 0244 it also holds EVIDENCE: the verbatim lines a claim was read out of,
	// which for a meeting transcript are the subject's own words. That quotation
	// is a second copy of a body the timeline scrub nulls, kept in a row the
	// activity-keyed scrubs cannot see, so Art. 17 has to reach it by statement
	// and Art. 15 has to hand it back.
	"approval": {erasureWrite: true, sarRead: true},
	// One reading of one transcript (core 0245). It quotes nothing itself — a
	// status, a line count, and which proposals it produced — but every one of
	// those is an answer about a body the erasure destroys, and its schema's
	// ON DELETE CASCADE never fires because neither engine ever deletes an
	// activity. Purged with the body rather than exported, like the embedding:
	// a machine artifact of the subject's text, not anything they supplied.
	"transcript_read": {erasureWrite: true, sarRead: false},
	// The preference-center token (0048) is a live capability over the
	// subject's consent record — held by whoever has the emailed
	// List-Unsubscribe URL, honoured with no session at all. Registered so
	// this gate proves erasure retires it: the person row survives
	// anonymize-in-place, so the schema's ON DELETE CASCADE never fires.
	// The export side is sarFORBIDDEN, not merely not-read, and for the
	// opposite reason to embedding's: not "nothing human-readable to hand
	// back" but "a working credential", which an Art. 15 package assembled by
	// an admin must not carry into an export file — the subject already holds
	// their own copy, in the mail that delivered it. Declared so a future SAR
	// section over this table fails the gate instead of shipping.
	// Why each outbound message to this person was permitted, per recipient
	// and per phase. It holds their address and the ids of the records the
	// decision rested on, so erasure must reach it: the person row survives
	// anonymize-in-place and the schema's cascade fires off the DELIVERY, not
	// off the subject.
	//
	// sarREAD, and not a close call. Art. 15(1)(a)-(c) asks the controller to
	// say what it did with somebody's data and why; this table IS that answer
	// for every message sent to them. Withholding it would mean holding the
	// clearest record of the processing and declining to disclose it.
	"communication_decision": {erasureWrite: true, sarRead: true},
	// The non-consent basis a message stood on — the thing that happened, its
	// scope and its window. Same reasoning: erased with the subject, and
	// disclosed, because "we wrote to you because you wrote to us on 2 May" is
	// exactly what a subject access request is asking for.
	"communication_basis": {erasureWrite: true, sarRead: true},
	// Objections, restrictions and dead addresses. Disclosed for the same
	// reason: a person asking what is held about them is owed the record that
	// they said stop, and when.
	"communication_suppression": {erasureWrite: true, sarRead: true},

	"preference_token": {erasureWrite: true, sarForbidden: true},

	// The confirm-details link: a live bearer credential that opens the
	// subject's own record. Registered for the same two reasons preference_token
	// is. Erasure must retire it explicitly, because anonymize-in-place leaves
	// the person row standing so the schema's ON DELETE CASCADE never fires. The
	// export side is sarFORBIDDEN because the row holds a working credential and
	// the address it went to, and an Art. 15 package assembled by an admin must
	// carry neither — the subject already has their own copy, in the mail.
	"confirm_token": {erasureWrite: true, sarForbidden: true},
	// And what came back through it: a correction the subject typed, or their
	// request to be removed. sarREAD rather than forbidden, and it is the one
	// part of the package the subject authored — an export that handed back
	// their archived addresses and their whole timeline while omitting the
	// correction they themselves sent would be answering the wrong question.
	"person_confirm_submission": {erasureWrite: true, sarRead: true},
}

// sarAssemblyFiles are the files whose SQL literals make up the Art. 15
// package. Listed rather than globbed: the gate asks what the EXPORT reads, and
// a glob over the package would read erasure's DELETE statements as disclosures.
// A new file carrying SAR sections joins this list; the section-count assertion
// below is what fails if one is forgotten.
var sarAssemblyFiles = []string{
	"internal/modules/privacy/sar.go",
	"internal/modules/privacy/sarsections.go",
	"internal/modules/privacy/sarmessages.go",
}

// fromJoinRe extracts the table named by a FROM/JOIN clause — SAR reads are
// SELECTs, invisible to sqlWriteTargets.
var fromJoinRe = regexp.MustCompile(`(?is)\b(?:from|join)\s+([a-z_][a-z0-9_]*)`)

func TestErasureAndSARReachEveryPIITable(t *testing.T) {
	t.Parallel()
	writes := map[string]bool{}
	for _, path := range erasureCascadeFiles {
		for _, lit := range sqlLiterals(t, path) {
			for _, table := range sqlWriteTargets(lit) {
				writes[table] = true
			}
		}
	}
	// Each swept table keeps the text of the sweep statements that write it, so
	// the declared erasure assignments are checked against the statement that
	// erases THIS table rather than against retention.go as a whole.
	sweeps := map[string]string{}
	// Kept per STATEMENT as well, because the two checks below ask different
	// questions of them. "Is this assignment still made" is answered by the
	// concatenation; "does the sweep touch a column it promised not to" has to
	// look inside one statement's SET clause, or a WHERE predicate naming the
	// column in a neighbouring statement would answer for it.
	sweepStatements := map[string][]string{}
	// The sweep statements this gate can only read HALF of, keyed by the table
	// the readable half names. Collected from the syntax tree rather than the
	// text, for the reason assembledSweepTargets gives.
	assembled := map[string]string{}
	for _, path := range retentionSweepFiles {
		for table, fragment := range assembledSweepTargets(t, path) {
			assembled[table] = fragment
		}
		for _, lit := range sqlLiterals(t, path) {
			// Before the split, whether the split can be trusted. A quoting
			// form the scan cannot track leaves it inverted, and an inverted
			// scan cuts inside a string — which is the fragment naming no
			// table that sqlStatements was written to prevent.
			if form := quotingBeyondTheSplit(lit); form != "" {
				t.Fatalf("%s carries a `%s` literal, which the statement split cannot track: %q\n"+
					"the split would cut inside the string and the fragment carrying the assignment would name no table. "+
					"Teach sqlStatements this form, or keep the statement out of the swept files", path, form, collapsedSQL(lit))
			}
			// SPLIT, because the unit both checks below name is the statement
			// and a literal is not one. Read whole, a literal carrying two
			// would let the first statement's SET clause answer for the second
			// — which is what sqlStatements exists to prevent, and it was
			// reached only by the falsification case: the gate itself passed
			// the literal, so the shape that case plants was not one the sweep
			// was actually checked against.
			for _, stmt := range sqlStatements(lit) {
				for _, table := range sqlWriteTargets(stmt) {
					sweeps[table] += " " + stmt
					sweepStatements[table] = append(sweepStatements[table], stmt)
				}
			}
		}
	}
	// The files the SAR assembly is SPLIT ACROSS, not one named file and not
	// the whole package. One filename reports a section missing the moment
	// somebody moves it — a rename passing itself off as an Art. 15 gap — and
	// the whole package sweeps in erasure's own DELETE ... FROM statements,
	// which fromJoinRe cannot tell from a SELECT.
	reads := map[string]bool{}
	for _, path := range sarAssemblyFiles {
		for _, lit := range sqlLiterals(t, path) {
			for _, m := range fromJoinRe.FindAllStringSubmatch(lit, -1) {
				reads[m[1]] = true
			}
		}
	}

	var missing []string
	for table, h := range piiTables {
		if !h.erasureWrite && !h.retentionErase {
			missing = append(missing, "PII table "+table+
				" is registered with no eraser at all — declare erasureWrite (the Art. 17 cascade) or retentionErase (the time-based sweep)")
		}
		if h.erasureWrite && !writes[table] {
			missing = append(missing, "erasure never writes PII table "+table+
				" — Art. 17 leaves it intact; redact/purge it in ErasePerson")
		}
		if h.retentionErase && sweeps[table] == "" {
			missing = append(missing, "the retention sweep never writes PII table "+table+
				" — its only eraser is gone; erase it in the nightly evaluator or move it onto the Art. 17 cascade")
		}
		if h.retentionErase && len(h.retentionErasures) == 0 {
			missing = append(missing, "PII table "+table+
				" names the retention sweep as its eraser but declares no erasure assignments — list the SET clauses that ARE the wipe, so a metadata-only write cannot satisfy this gate")
		}
		// Checked against ONE statement rather than the union of every sweep
		// statement that writes this table, because a declaration describes one
		// act: the union would let two statements each making half of it pass,
		// which is how an erasure gets split and stops being one.
		//
		// What this still cannot see, stated rather than implied: a statement
		// satisfies a declaration by EXISTING. A second statement carrying the
		// whole erasure — a helper nobody calls — would answer for an action
		// that had stopped making it. Closing that needs call-graph
		// reachability, which no SQL gate in this tree has and which the
		// sweep's two kinds of entry point (the retentionActions executors and
		// the evaluate*Retention passes) make more than a one-line derivation.
		// The realistic drift — somebody deletes the assignment — is caught,
		// because nothing else in the swept files carries these together.
		if len(h.retentionPurge) > 0 && !sweepPurges(sweepStatements[table], table, h.retentionPurge) {
			missing = append(missing, "PII table "+table+
				" is registered as one the retention sweep PURGES under "+strings.Join(h.retentionPurge, ", ")+
				", and no sweep delete carries every one of them — the rows one of those acts was written to destroy now"+
				" outlive the content they describe; restore the delete or amend the declared purge")
		}
		if len(h.retentionErasures) > 0 && !oneStatementCarries(sweepStatements[table], h.retentionErasures) {
			missing = append(missing, "no single retention-sweep statement on PII table "+table+
				" carries all of "+strings.Join(h.retentionErasures, ", ")+
				" — the content they were written to erase now outlives its window, or the erasure has been split across statements and the declaration no longer describes one act; restore the assignments or amend the declared erasure")
		}
		// A table that declares only retentionKeeps has no other tripwire: with
		// no statements to read, statementDestroying answers "nothing destroys
		// it" and the check turns itself off. That is the same regression the
		// file list above has now had twice, arriving where nothing fails loud.
		if len(h.retentionKeeps) > 0 && len(sweepStatements[table]) == 0 {
			missing = append(missing, "PII table "+table+
				" registers a column the sweep deliberately keeps, but the sweep no longer writes this table at all — the check has nothing to read and is passing vacuously; add the file that holds the sweep's statements to retentionSweepFiles, or drop the registration")
		}
		// Before asking what the statements destroy, whether they can be read
		// at all. A SET clause with no column in it is one the gate cannot
		// judge, and answering "keeps everything" for it is the quiet pass.
		if len(h.retentionKeeps) > 0 {
			if unreadable := unreadableWriteOn(sweepStatements[table], assembled[table]); unreadable != "" {
				missing = append(missing, "the retention sweep writes PII table "+table+
					" with a statement this gate can only half read (`"+unreadable+"`) — the rest of it is assembled at runtime, so the"+
					" columns registered as deliberately KEPT are no longer checked against it. Name the columns in the"+
					" statement, or move the write out of the swept files and register what it does")
			}
		}
		for _, column := range h.retentionKeeps {
			if destroyer := statementDestroying(sweepStatements[table], table, column); destroyer != "" {
				missing = append(missing, "the retention sweep now destroys `"+column+"` on PII table "+table+
					" (`"+destroyer+"`), which is registered as a column it deliberately KEEPS — the retention action's contract is that the record of the event survives and its content goes. If that ruling has changed, move the column into retentionErasures in the same commit so the change is the declaration and not a side effect of it")
			}
		}
		if h.sarRead && !reads[table] {
			missing = append(missing, "SAR never reads PII table "+table+
				" — Art. 15 export is incomplete; add a section in AssembleSAR")
		}
		if h.sarForbidden && reads[table] {
			missing = append(missing, "SAR reads PII table "+table+
				" — it is registered sarForbidden because its rows must never leave in an Art. 15 package; drop the section in AssembleSAR")
		}
		if h.sarRead && h.sarForbidden {
			missing = append(missing, "PII table "+table+
				" is registered both sarRead and sarForbidden — the export cannot both require and refuse it")
		}
	}
	sort.Strings(missing)
	for _, m := range missing {
		t.Error(m)
	}
}
