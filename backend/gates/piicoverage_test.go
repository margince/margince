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
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
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
	"internal/modules/privacy/sarmessages.go",
}

// fromJoinRe extracts the table named by a FROM/JOIN clause — SAR reads are
// SELECTs, invisible to sqlWriteTargets.
var fromJoinRe = regexp.MustCompile(`(?is)\b(?:from|join)\s+([a-z_][a-z0-9_]*)`)

// sqlWhitespaceRe collapses the indentation of a raw-string SQL literal so an
// assignment can be matched as the one-line clause it reads as in the registry.
var sqlWhitespaceRe = regexp.MustCompile(`\s+`)

func collapsedSQL(literal string) string {
	return strings.ToLower(strings.TrimSpace(sqlWhitespaceRe.ReplaceAllString(withoutComments(literal), " ")))
}

// withoutComments replaces each comment run with a space, keeping everything
// else — quoted values included — exactly as written.
//
// Before the whitespace collapse, and that ORDER is the whole of it. A line
// comment ends at its newline; collapsing newlines first turns `SET body =
// NULL, -- note\n counterparty_email = NULL` into one line where the comment
// runs to the end of the statement, so every assignment after it disappears and
// the check reports a clean sweep. The comment is not part of the statement, so
// it comes out rather than being scanned around later.
func withoutComments(literal string) string {
	var out strings.Builder
	for i := 0; i < len(literal); i++ {
		skip, _ := gatekit.SQLSpanAt(literal, i)
		if skip == 0 {
			out.WriteByte(literal[i])
			continue
		}
		if literal[i] == '-' || literal[i] == '/' {
			out.WriteByte(' ')
		} else {
			out.WriteString(literal[i : i+skip])
		}
		i += skip - 1
	}
	return out.String()
}

// sqlLiterals returns every Go string literal in one source file. Both the
// write-target and read-target scans run over these.
func sqlLiterals(t *testing.T, path string) []string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	var out []string
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		if s, err := strconv.Unquote(lit.Value); err == nil {
			out = append(out, s)
		}
		return true
	})
	return out
}

// erasureCascadeFiles are the sources that make up the Art. 17 cascade — the
// files ErasePerson's own transaction executes SQL from. It is a LIST because
// the cascade spans more than one file, and a gate pinned to a single path
// silently stops covering a table the moment its scrub is extracted to a
// neighbour. It is deliberately NOT the whole privacy
// package: retention.go also writes subject tables, and letting a retention
// sweep satisfy "Art. 17 reaches this table" is exactly the confusion this test
// exists to prevent.
var erasureCascadeFiles = []string{
	"internal/modules/privacy/erasure.go",
	// The subject's TIMELINE and everything derived from it — split out of
	// erasure.go when that file crossed the size cap. It is the same Art. 17
	// transaction, so it counts here; leaving it off would let a table look
	// uncovered the moment its purge moved file.
	"internal/modules/privacy/erasuretimeline.go",
	// The subject's traces in the relationship graph — the interaction
	// participants, the imported LinkedIn ghosts, and the projection folded out
	// of both. Same Art. 17 transaction, its own file for the same size reason
	// the timeline has one.
	"internal/modules/privacy/erasure_graph.go",
	// Retention’s graph invalidation — same Art. 17/retention transaction.
	"internal/modules/privacy/erasure_attachments.go",
	"internal/modules/privacy/erasure_channels.go",
	// The live capabilities over the subject's consent record — the
	// preference-center token and the double-opt-in token. Split out of
	// erasure.go for the same size reason as the timeline, and named here for
	// the reason this list's own header gives: leaving it off would make
	// preference_token look uncovered the moment its DELETE moved file.
	"internal/modules/privacy/erasure_consent.go",
	"internal/modules/privacy/erasure_rivals.go",
	// What a licensed data provider was PAID to tell us about the subject,
	// and the runs that bought it (ADR-0101). Same Art. 17 transaction, its
	// own file for the same size reason the timeline has one.
	"internal/modules/privacy/erasure_provider.go",
	// The messages nobody has decided yet, and the quotations they carry.
	// Same Art. 17 transaction; its own file because it belongs to neither
	// destructive engine and both reach it.
	"internal/modules/privacy/erasure_approvals.go",
	// The readings of the transcripts the timeline scrub just emptied — same
	// transaction, its own file for the same both-engines reason.
	"internal/modules/privacy/transcriptreadings.go",
	"internal/modules/privacy/deliveries.go",
}

// retentionSweepFiles are the nightly time-based evaluator — the only eraser a
// subject-unlinked PII table has. Kept apart from the cascade above so a
// retention sweep can never be mistaken for an answer to an Art. 17 request.
//
// A LIST rather than one path, because the evaluator has already outgrown one
// file twice. Splitting the AI-store sweeps out made three tables look
// unswept; then the per-action executors moved to retentionactions.go and the
// list was not extended, so every assignment the sweep's OWN actions make —
// activity/erase among them — was invisible to this gate. A census keyed to a
// filename reports a refactor as a compliance regression, and worse reports
// nothing at all when the refactor moves code OUT of the names it knows.
//
// It is still a list and not a glob over retention*.go, and that is the part
// worth reading before "fixing" it. retentionrestricted.go is the RESTRICTION
// LIFT — a different trigger — and it clears `counterparty_email`, which the
// sweep deliberately keeps. Folding it in would let the lift's assignments
// satisfy the sweep's declarations below, so a retentionErasures entry would
// pass whether or not the sweep still made it: the gate would go quietly green
// over exactly the divergence it exists to catch. Over-recognition is the
// failure mode a glob buys here, and it is the one with no failing assertion
// to notice it.
var retentionSweepFiles = []string{
	"internal/modules/privacy/retention.go",
	"internal/modules/privacy/retentionai.go",
	"internal/modules/privacy/retention_graph.go",
	"internal/modules/privacy/retentionactions.go",
}

// sqlStatements splits one Go string literal into the statements it holds. A
// literal is not a statement: several in this tree carry two, and a check that
// read the literal whole would let a second statement's SET clause hide behind
// the first one's.
//
// The split skips SEMICOLONS INSIDE STRINGS, which is the case gate-patterns.md
// §D names. `SET body = 'a;b', counterparty_email = NULL` splits naively into
// two fragments, and the second names no table — so sqlWriteTargets drops it and
// the destruction of a retained column in it goes unseen. Quoting is the only
// escape the split has to understand: a doubled ” inside a string is still
// inside it, which falls out of the scan without a case of its own.
//
// The scan is gatekit's, shared with the trigger-written-column census, because
// "what here is not SQL" is one question: a quoted value, a dollar-quoted body,
// a line comment and a block comment all carry semicolons that are not
// separators, and a split landing inside one leaves a fragment naming no table.
// Comments matter as much as quotes here — `SET body = NULL /* ; */ ,
// counterparty_email = NULL` splits at the comment's semicolon and the
// destruction rides out in a fragment sqlWriteTargets drops.
//
// ONE form it still does not understand, and does not guess at: the E” escape,
// where a backslashed apostrophe leaves the scan believing it has closed a
// string it has not. quotingBeyondTheSplit answers for it at the collection
// site, so its arrival is a failure naming the literal rather than a green run
// over a split nobody can trust — and it looks for it OUTSIDE the spans the
// scan does understand, or an E-string MENTIONED inside a value or a comment
// would refuse a statement that reads perfectly well.
func sqlStatements(literal string) []string {
	var out []string
	start := 0
	for i := 0; i < len(literal); i++ {
		if skip, _ := gatekit.SQLSpanAt(literal, i); skip > 0 {
			i += skip - 1
			continue
		}
		if literal[i] != ';' {
			continue
		}
		if trimmed := strings.TrimSpace(literal[start:i]); trimmed != "" {
			out = append(out, collapsedSQL(trimmed))
		}
		start = i + 1
	}
	if trimmed := strings.TrimSpace(literal[start:]); trimmed != "" {
		out = append(out, collapsedSQL(trimmed))
	}
	return out
}

// escapeStringRe is the one form the split cannot track: E'…' takes a
// BACKSLASH escape, so `E'a\';b'` leaves the scan believing it has closed a
// string it has not, and an inverted scan splits inside one.
var escapeStringRe = regexp.MustCompile(`(?i)\bE'`)

// quotingBeyondTheSplit names the form in a literal that sqlStatements would
// mis-scan, or "" when there is none. Nothing in the swept files uses it today;
// this is the tripwire for the day one does, and it fails CLOSED — the caller
// reports the literal — because the alternative is a split landing inside a
// string and a destructive assignment riding out in a fragment that names no
// table.
//
// Looked for OUTSIDE the spans the scan does understand, which is the half a
// bare regex gets wrong in the expensive direction: `SET body = 'E”s note'` or
// a comment mentioning one would refuse a statement that is read perfectly
// well, and a tripwire that fires on correct SQL is one somebody turns off.
func quotingBeyondTheSplit(literal string) string {
	for i := 0; i < len(literal); i++ {
		if skip, _ := gatekit.SQLSpanAt(literal, i); skip > 0 {
			i += skip - 1
			continue
		}
		if m := escapeStringRe.FindString(literal[i:]); m != "" && strings.HasPrefix(strings.ToLower(literal[i:]), "e'") {
			return m
		}
	}
	return ""
}

// setAssignments returns the assignment list of one UPDATE — what sits between
// SET and the clause that ends it — or "" when the statement has none.
//
// Scanned at PAREN DEPTH ZERO rather than matched with a lazy regex, and that
// is the whole point of it being a scan. `SET redacted_fields = ARRAY(SELECT c
// FROM unnest(…) WHERE c IS NOT NULL), counterparty_email = NULL` is a shape
// this tree already writes (retentionrestricted.go), and a regex ending at the
// first WHERE stops inside that subquery — so every assignment after it becomes
// invisible and reordering the list, which nobody reviews as a compliance
// change, turns the check off.
func setAssignments(statement string) string {
	low := strings.ToLower(statement)
	i := strings.Index(low, " set ")
	if i < 0 {
		return ""
	}
	rest := statement[i+len(" set "):]
	depth := 0
	for pos := range rest {
		switch rest[pos] {
		case '(':
			depth++
		case ')':
			depth--
		}
		if depth != 0 {
			continue
		}
		for _, end := range []string{" where ", " from ", " returning "} {
			if strings.HasPrefix(strings.ToLower(rest[pos:]), end) {
				return rest[:pos]
			}
		}
	}
	return rest
}

// oneStatementCarries reports whether any single statement makes every declared
// assignment. Declared assignments describe ONE erasure, so they are proven
// against one statement — see the call site for why the union is not enough.
func oneStatementCarries(statements []string, assignments []string) bool {
	for _, stmt := range statements {
		all := true
		for _, assignment := range assignments {
			all = all && strings.Contains(stmt, strings.ToLower(assignment))
		}
		if all {
			return true
		}
	}
	return false
}

// sweepPurges reports whether the sweep deletes from table under every declared
// predicate — one delete per predicate, so a declaration names the acts rather
// than the table.
//
// Through deleteRe, the matcher tableownership_test.go already derives its own
// answer from, rather than a second spelling here. The hand-written one this
// replaced looked for "delete from <table> " and its end-of-string twin, and
// went blind on a trailing `;` or `)` — both of which end a statement in this
// tree, and both of which would have reported a purge that exists as missing.
func sweepPurges(statements []string, table string, predicates []string) bool {
	for _, predicate := range predicates {
		found := false
		for _, stmt := range statements {
			if !strings.Contains(stmt, strings.ToLower(predicate)) {
				continue
			}
			for _, m := range deleteRe.FindAllStringSubmatch(stmt, -1) {
				if m[1] == table {
					found = true
				}
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// statementDestroying returns the sweep statement that would destroy column on
// table, or "" if none does.
//
// It asks whether the column is ASSIGNED AT ALL rather than whether it is
// assigned NULL: `col = NULL`, `col=NULL`, `col = CAST(NULL AS text)` and
// `col = ”` are one act in four spellings, and a check that recognised one
// would go quietly green on the other three. A retained column is one the sweep
// does not write, so any write to it is the finding, and a DELETE of the row
// counts because it takes the column with it.
//
// WHAT IT CANNOT SEE, and the tripwire that makes that loud. The column is
// matched as a LITERAL name, so a statement whose SET target is assembled —
// `"UPDATE activity SET " + column + " = NULL"` — carries no name to match and
// would pass silently. gate-patterns.md §D names that as the way a shape gate
// goes green. It cannot be read from a string literal at all, so it is not
// matched but REPORTED: assembledSetTarget below answers for a swept UPDATE
// whose SET clause names nothing, and the caller fails on it rather than
// reading it as a statement that destroys no column.
//
// Held by TestTheRetainedColumnCheckSeesEveryDestructiveShape, which plants the
// shapes an earlier version of this missed rather than trusting that the one
// statement in the tree passes.
func statementDestroying(statements []string, table, column string) string {
	needle := regexp.MustCompile(`(?is)(?:\bSET\b|,)\s*` + regexp.QuoteMeta(strings.ToLower(column)) + `\s*=`)
	for _, stmt := range statements {
		// The statement has to write THIS table. The caller already groups
		// statements by write target, but a helper that only answered
		// correctly because its caller filtered first is one the next caller
		// gets wrong — and `activity` and `activity_participant` both carry a
		// counterparty column, so the confusion is available here today.
		writesTable := false
		for _, target := range sqlWriteTargets(stmt) {
			writesTable = writesTable || target == table
		}
		if !writesTable {
			continue
		}
		if deleteRe.MatchString(stmt) {
			return stmt
		}
		if sets := setAssignments(stmt); sets != "" && needle.MatchString("SET "+strings.ToLower(sets)) {
			return stmt
		}
	}
	return ""
}

// assembledSetTarget answers the swept UPDATE whose SET clause names no column,
// or "" when every one of them does.
//
// A statement built by concatenation reaches a gate that reads string literals
// as its literal half alone — `UPDATE activity SET ` — and the column it writes
// is never in the text. statementDestroying would answer "destroys nothing",
// which is indistinguishable from a statement that really destroys nothing, and
// that is the shape gate-patterns.md §D says a shape check silently passes.
//
// A SET clause is unreadable in two shapes, and reading only the first was the
// same quiet pass one level in. `UPDATE provider_run SET` — which the sweep
// really does assemble today, in retentionactions.go, joined to
// storekit.ScrubProviderRunColumns — leaves NO assignments.
// `UPDATE activity SET body = NULL, ` + column + ` = NULL` leaves one, so a
// check asking only "is it empty" reads that as an ordinary statement and the
// assembled column goes unseen exactly as before. The trailing comma is the
// tell: an assignment list ending on a separator has an assignment after it
// that is not in the text.
//
// Both answers are read off the literal alone, which is why
// assembledSweepTargets below reads the concatenation instead: a fragment can
// also be a COMPLETE statement — `"UPDATE activity SET body = NULL" + more +
// " WHERE id = $1"` — and no amount of looking at that text says more follows.
//
// Neither reports provider_run, and that is the scoping and not an oversight.
// The caller asks these only of a table registering columns the sweep KEEPS,
// because the question they serve — "is the KEEPS registration still being
// checked against this statement" — has no meaning for a table with no such
// registration. provider_run declares none; its assembly is a named constant a
// reader can follow, not a column hidden from one.
func assembledSetTarget(statements []string) string {
	for _, stmt := range statements {
		if !updateRe.MatchString(stmt) {
			continue
		}
		assignments := strings.TrimSpace(setAssignments(stmt))
		if assignments == "" || strings.HasSuffix(assignments, ",") || formatVerbTargetRe.MatchString(assignments) {
			return stmt
		}
	}
	return ""
}

// formatVerbTargetRe is a fmt verb standing where a COLUMN should be — at the
// head of the assignment list or just after a comma, and followed by the `=`
// that makes it a target.
//
// `fmt.Sprintf("UPDATE activity SET %s = NULL", column)` reaches the reader as
// a literal that looks complete and names a column that is not a column, so
// neither the empty-clause nor the dangling-comma tell fires — and no `+` node
// exists for assembledSweepTargets to read either. The verb is the tell.
//
// The POSITION is what makes it a tell rather than a nuisance. A bare `%` is
// ordinary SQL text: `SET body = 'template %s'`, `SET query = 'foo%bar'`, a
// LIKE pattern `'100%'`. Matching those would report a statement that is
// entirely readable, and this gate's report is an instruction to go rewrite it
// — a tripwire that fires on correct code is one somebody turns off.
//
// What it still reads wrongly, stated rather than implied: a comma AND a verb
// AND an equals inside one quoted value — `SET body = 'a, %s = b'` — puts a
// target position inside a string. That is over-recognition on a shape nothing
// writes, which costs a false finding rather than a missed destruction.
var formatVerbTargetRe = regexp.MustCompile(`(?:^|,)\s*%[-+# 0-9.*]*[a-z]\s*=`)

// unreadableWriteOn answers the swept statement on one table that this gate
// can only half read, or "" when it can read them all.
//
// It is a function rather than two lines at the call site because the ANSWER is
// two answers and dropping either is silent. The text says a fragment is
// unfinished (assembledSetTarget); the syntax says a finished-looking fragment
// has more joined onto it (the caller's assembledSweepTargets). A regression
// that kept only the first would keep passing every case written for it.
func unreadableWriteOn(statements []string, assembledFragment string) string {
	if unreadable := assembledSetTarget(statements); unreadable != "" {
		return unreadable
	}
	return assembledFragment
}

// assembledSweepTargets returns, per table, a swept SQL literal that is joined
// to a runtime value — `"UPDATE provider_run SET" + storekit.Scrub…`. The table
// comes from the literal's own text, which is where the write target is even
// when the assignments are not.
//
// This reads the CONCATENATION, not the string, and that is the whole point.
// assembledSetTarget can only judge the text it is handed, and a fragment that
// reads as a finished statement is indistinguishable from one that is; the `+`
// node is the only place the fact that more follows is written down.
func assembledSweepTargets(t *testing.T, path string) map[string]string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	// Parentheses are not part of the expression, and reading them as if they
	// were fails in BOTH directions: a parenthesized literal on the joined side
	// hides the fragment, and one on the other side reads as a runtime value
	// and reports a statement that is entirely in the text.
	text := func(e ast.Expr) (string, bool) {
		for {
			paren, ok := e.(*ast.ParenExpr)
			if !ok {
				break
			}
			e = paren.X
		}
		lit, ok := e.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return "", false
		}
		unquoted, err := strconv.Unquote(lit.Value)
		return unquoted, err == nil
	}
	out := map[string]string{}
	ast.Inspect(file, func(n ast.Node) bool {
		// A SQL fragment handed to an ASSEMBLER — fmt.Sprintf, a
		// strings.Builder's WriteString, strings.Replace — is half a statement
		// with no `+` node to say so. tx.Exec's own literal is not: the whole
		// statement is the argument, which is why the call names are listed
		// rather than "any call".
		if call, isCall := n.(*ast.CallExpr); isCall && assemblesSQL(call) {
			// The whole SUBTREE, not the direct arguments: strings.Join takes
			// its pieces inside a slice literal, and a Sprintf format string
			// can itself be a join of literals.
			ast.Inspect(call, func(inner ast.Node) bool {
				expr, isExpr := inner.(ast.Expr)
				if !isExpr {
					return true
				}
				fragment, ok := text(expr)
				if !ok {
					return true
				}
				for _, table := range sqlWriteTargets(collapsedSQL(fragment)) {
					out[table] = collapsedSQL(fragment)
				}
				return true
			})
			return true
		}
		join, ok := n.(*ast.BinaryExpr)
		if !ok || join.Op != token.ADD {
			return true
		}
		// One side a literal and the other not. Two literals joined are still
		// one literal as far as the text is concerned — sqlLiterals reads both
		// halves — and reporting those would fire on every wrapped statement in
		// the tree.
		for _, side := range [2][2]ast.Expr{{join.X, join.Y}, {join.Y, join.X}} {
			fragment, ok := text(side[0])
			if !ok {
				continue
			}
			if _, alsoLiteral := text(side[1]); alsoLiteral {
				continue
			}
			for _, table := range sqlWriteTargets(collapsedSQL(fragment)) {
				out[table] = collapsedSQL(fragment)
			}
		}
		return true
	})
	return out
}

// sqlAssemblers are the calls that build a statement out of pieces. Named
// rather than inferred, because "a literal inside a call" is every statement in
// the tree: tx.Exec takes the whole thing.
var sqlAssemblers = map[string]bool{
	"Sprintf": true, "Sprint": true, "Sprintln": true,
	"Fprintf": true, "WriteString": true, "Replace": true, "ReplaceAll": true,
	"Join": true,
}

func assemblesSQL(call *ast.CallExpr) bool {
	switch fn := call.Fun.(type) {
	case *ast.SelectorExpr:
		return sqlAssemblers[fn.Sel.Name]
	case *ast.Ident:
		return sqlAssemblers[fn.Name]
	}
	return false
}

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
