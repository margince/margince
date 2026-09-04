// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind shape H2

package gates

// The LIVENESS obligation as a fitness function: a write that targets one
// standing row of a table which can be archived either REFUSES an archived row,
// DECLARES that it deliberately reaches one, or is ratified with a reason.
//
// It is the fourth of the four obligations a write in this tree owes, and it was
// the only one with no gate. The other three each have theirs — tableownership
// (writes only tables it owns), updateguard (carries a concurrency guard),
// writeauthority + writeshape (probes write authority, audits and emits) — and
// that single gap produced eight defects across organization, person,
// person_consent, attachment, contract, commission_entry and site_read, every
// one of them individually defensible where it sat. What they had in common was
// not a module or a table: it was that nothing asked the question.
//
// ENFORCEMENT IS A GATE AND NOT A TRIGGER, deliberately. A row-level refusal
// looks like the obvious answer and breaks erasure: Art. 17 and the retention
// sweep MUST write archived rows — `archived_at = coalesce(archived_at, now())`
// runs through those statements — so a database that refused every write to an
// archived row would refuse the destruction the archive is a step of. Liveness
// is a property of the CALL SITE, not of the row.
//
// Two spellings satisfy it, and the difference between them is the whole point:
//
//   - REFUSAL — an `archived_at IS NULL` predicate, a `*Live` probe, a LiveOnly
//     lock or patch. The write cannot reach an archived row.
//   - DECLARATION — auth.EnsureRetractable, or storekit.IncludeArchived. The
//     write reaches one ON PURPOSE, and says so where a reader looks.
//
// The declaration half is what makes the gate hold a line a scanner otherwise
// cannot: an archived anchor freezes what its children GRANT and must never
// freeze what they REVOKE, so "no liveness here" is sometimes exactly right.
// Before the pair existed, that case and a forgotten filter were the same text.
//
// Scope: single-row-by-id UPDATE and DELETE. A set-based write is out by
// construction, the way updateguard's is — a cascade over a parent's children
// takes its liveness from the parent it was chosen by, not from a predicate of
// its own. An INSERT is out for a different reason: a row being created has no
// archived_at to violate, and the liveness its ANCHOR owes is a separate
// question (the EnsureLinkTarget one) with a separate answer.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// byIDWriteOf matches a single-row-by-primary-key UPDATE or DELETE inside one
// SQL string literal, capturing the table. The by-id shape is what makes the
// statement a read-modify-write on a row somebody named, which is where the
// liveness question lives.
var byIDWriteOf = regexp.MustCompile(`(?is)\b(?:UPDATE|DELETE\s+FROM)\s+(?:ONLY\s+)?([a-z_]+)\b[\s\S]*?\bWHERE\s+(?:[a-z]+\.)?id\s*=\s*\$`)

// livePredicate is the refusal written into the statement itself.
var livePredicate = regexp.MustCompile(`(?i)archived_at\s+IS\s+NULL`)

var (
	// archivedColumnLine marks a CREATE TABLE block's table as retirable;
	// addedArchivedColumn catches the tables that grew the column later. Both,
	// because a census that read only one of them would judge a smaller tree and
	// still report PASS.
	archivedColumnLine  = regexp.MustCompile(`(?i)^\s*archived_at\s`)
	addedArchivedColumn = regexp.MustCompile(`(?is)ALTER TABLE\s+(?:IF EXISTS\s+)?([a-z_]+).{0,400}?ADD COLUMN\s+(?:IF NOT EXISTS\s+)?archived_at`)
)

// livenessMarkers are the spellings that discharge the obligation in Go, by the
// name at the call site. Both halves are here on purpose — a refusal and a
// declaration are equally answers, and the gate's subject is a write that gives
// NEITHER.
//
// Named rather than resolved through types, for the reason every census in this
// package is: a type-checked walk over the whole module costs more than the gate
// is worth, and the names below are unambiguous in this tree. A same-named
// method on some other receiver would over-credit, which is the direction that
// merely lets one write through rather than the direction that goes quiet.
var livenessMarkers = map[string]bool{
	// Refusals: the row is resolved, probed or locked LIVE.
	"EnsureWritableLive": true, "EnsureVisibleLive": true, "HoldWritableLive": true,
	"LockSubjectLive": true, "EnsureActivityVisibleLive": true,
	"EnsureActivityContentVisibleLive": true, "EnsureSignalVisibleLive": true,
	"EnsureLinkTarget": true, "EnsureActivityWritable": true, "EnsureActivityWritableIn": true,
	"LiveOnly": true, "ApplyGuarded": true, "ApplyWithVersion": true,
	// identity's own spelling of the same refusal, rendered into SQL: a seat
	// that is archived, suspended or deactivated is not a live member.
	"LiveMemberSQL": true, "ActivatableMemberSQL": true,
	// Declarations: the write reaches an archived row on purpose.
	"EnsureRetractable": true, "IncludeArchived": true,
}

// byIDWriteFloor is the smallest number of by-id writers of a retirable table
// this census may find and still be believed. It sits well below the real count
// because its job is to catch a reader that has gone quiet — a walk that stops
// finding files, or a statement reader that stops recognising this tree's SQL —
// rather than to track the tree.
//
// It is the coarser of the two under-recognition alarms and covers the whole
// census. The sharper one is livenessUnstated.AssertAllMatched: a ratified
// function the reader stops seeing is named outright, because its waiver then
// matches nothing.
const byIDWriteFloor = 90

// retirableTableFloor guards the other input. The table set is derived from the
// migrations, and a derivation that silently read none of them would exempt
// every write in the tree while reporting a clean run.
const retirableTableFloor = 20

// livenessUnstated ratifies a by-id write that neither refuses an archived row
// nor declares that it reaches one, keyed by "package-dir:FuncName". Every entry
// says where the liveness actually is, or why the write must reach a retired row;
// a reasonless or stale entry fails.
//
// No entry may read "known defect, see issue". The holes this gate was written
// for were closed before it was armed, so a waiver here is a statement that the
// obligation is MET some other way — never that it is owed and unpaid.
var livenessUnstated = gatekit.Waive(map[string]string{
	// ERASURE AND RETENTION MUST WRITE ARCHIVED ROWS. This is the family that
	// makes a row-level trigger impossible, and the reason each entry gives is
	// the specific destruction it performs rather than a restatement of that
	// sentence: an erasure stamps archived_at itself, so its own subject is
	// archived by the time the statement beside it runs.
	"internal/modules/privacy:anonymizeSubjectRows":    "Art. 17 erasure of the subject row, and it is the statement that SETS archived_at — coalesced, so a subject already archived keeps the instant they were retired. A liveness filter here would make erasure refuse every already-archived subject, which is the population most likely to ask for it",
	"internal/modules/privacy:anonymizeLeadTwins":      "the lead half of the same erasure, reaching the twins a promotion left behind. They are found by promoted_person_id and by address, and an archived twin holds the same personal data a live one does",
	"internal/modules/privacy:archiveActivity":         "the retention sweep retiring a message whose window closed. The write IS the archive transition, chosen by a clock rather than by a caller",
	"internal/modules/privacy:archiveDeal":             "the deal arm of the same sweep, and the same transition: a deal past its window is archived because of when it closed",
	"internal/modules/privacy:anonymizeLead":           "the retention sweep's lead anonymization, which stamps archived_at as part of the destruction and must run on a lead an earlier pass already retired",
	"internal/modules/privacy:eraseActivityContent":    "the retention sweep destroying a message's text. It coalesces archived_at, so refusing an archived row would spare exactly the messages the sweep already took off the timeline and left the words standing",
	"internal/modules/privacy:anonymizePersonRecord":   "the retention sweep's person arm, stamping the tombstone name and archived_at together. Its subject is chosen by a retention policy, and an archived contact still holds every column this nulls",
	"internal/modules/privacy:eraseVoiceSignalContent": "the time-based sweep over voice learning signals, which carry no subject linkage for Art. 17 to find — the clock is the only thing that reaches them, and the row's own state is not a reason to leave the text",
	"internal/modules/privacy:liftAndEraseHeldRecord":  "lifting a statutory hold and destroying what it preserved, in one statement. The record was archived when the hold was placed; requiring it to be live would make the lift unable to reach anything it is ever asked about",
	"internal/modules/privacy:PinToFloor":              "placing a statutory hold, which archives the record as part of placing it. The refusal it does carry is stronger than liveness — restricted_at IS NULL — so a second controller pinning the same record is declined rather than overwriting the first one's window",

	// THE RETIREMENT TRANSITION ITSELF. Each is idempotent by construction: the
	// write sets the retired state absolutely rather than deriving it from a
	// pre-read, so a second attempt converges on the same row instead of racing.
	"internal/modules/activities:ArchiveAttachment":   "the archive transition for a filed document. Refusing an archived row would turn a repeated archive into an error where it is the same answer, and the parent's authority is probed above through resolveAttachmentParent",
	"internal/modules/activities:archiveAbsorbedEcho": "folding a duplicate capture into its survivor, which takes the echo off the timeline. archived_at is coalesced so a row a noise disposition already hid keeps the instant its undo window is measured from — a filter here would refuse exactly that row",
	"internal/modules/capture:withdrawConnection":     "disconnecting a mailbox, and the retry that re-drives the cleanup a previous call left unfinished. The already-withdrawn arm is the one that must reach a retired row, and it is what makes the withdrawal recoverable rather than half-done",
	"internal/modules/customfields:Retire":            "the catalog field's own retirement, which moves `status` and not archived_at; lockField holds the row from before the decision read, and a repeat converges on the same status",
	"internal/modules/identity:UpdateTeam":            "the team's archive and restore arms are this function, so both directions have to reach a row on the far side of the transition. The row is held FOR UPDATE from before the state is read",

	// THE LIVENESS IS ONE FRAME UP, OR ONE HOP ACROSS. Each of these is reached
	// only through a caller or a helper that has already resolved the row live,
	// and each entry names it — the scan reads one function at a time and cannot
	// follow either edge.
	"internal/modules/people:resolveOrCreateAnchor":        "guarded in its helper: anchorOrganization carries `WHERE is_anchor AND archived_at IS NULL FOR UPDATE`, so the row this renames was resolved live and is held for the rest of the transaction",
	"internal/modules/people:recordGeocodeAfter":           "guarded by addressHashInTx, which re-reads the address `WHERE id = $1 AND archived_at IS NULL`: an archived company yields no hash, the comparison fails, and the function returns without writing. The liveness and the address-moved check are one test",
	"internal/modules/people:writeOrgColumn":               "the read-back's column writer, reached only through applyEvidenceFieldsWithOverwrite from three entry points that each take auth.EnsureWritableLive on the organization first: the cold-start accept (gateResolvedColdStartTarget), ApplyDeepReadTx, and Enrich",
	"internal/modules/people:applyUnclaimedOrgColumn":      "the fill arm of writeOrgColumn and reached only through it, so it inherits the same three live-probed entry points",
	"internal/modules/people:writeCompanyFields":           "the company form's writer, reached only after resolveOrCreateAnchor has resolved the anchor through anchorOrganization's `archived_at IS NULL` and locked it for the transaction",
	"internal/modules/people:touchRevertedPerson":          "the aggregate bump after a revert removed a child row. RevertProviderFills holds this contact FOR UPDATE with IncludeArchived from the top of its transaction — deliberately, because the subject of a bought-data revert may be archived — so re-taking liveness here would refuse the case the function exists for",
	"internal/modules/activities:finalizeRelinkedActivity": "the row is already held FOR UPDATE by relinkActivityRow, its only caller, through lockActivityForWrite — two hops past what a per-function scan follows, and the same indirection updateguard ratifies for this function",
	"internal/modules/deals:recomputeOfferTotals":          "every caller holds the offer row lock through visibleOfferLocked, except createOfferTx where the offer was inserted in the same transaction",
	"internal/modules/customfields:Rename":                 "runs under the catalog row lock lockField mints, and the field's own mutability is checked under it. custom_field is retired through `status`; nothing in the tree writes its archived_at",
	"internal/modules/customfields:setOptionsInTx":         "runs under the catalog row lock lockPicklistField mints, plus the per-table advisory lock serializeSchemaChange takes, and the same status-not-archived_at retirement applies",
	"internal/modules/people:recomputeUnderOverrideTx":     "the scoring pass refreshing the machine value beside a human override. Its subject comes from the recompute's own selection of live leads, and the CAS on score_computed is what makes a lost race write nothing",

	// THE WRITE CANNOT REACH A RETIRED ROW, because of what it is rather than
	// because of a predicate. Each says which fact makes that true.
	"internal/modules/capture:limitLinkLessAudience":      "the activity was INSERTed by this same transaction a few statements earlier, so there is no archived row for it to find — the function's own check is that exactly one row moved",
	"internal/modules/capture:commitSyncCursor":           "fenced by the generation, not by liveness: withdrawing a connection bumps it, so a sync that started before the withdrawal finds its generation gone and reports itself superseded instead of writing a watermark onto a retired grant",
	"internal/modules/capture:AdvanceChannelPollOffsetTx": "the poller's watermark for a connection its own live selection handed it, and the statement only ever moves the offset FORWARD (`poll_offset < $3`), so a late write from a retired cycle matches nothing",
	"internal/modules/capture:backfillWorkspace":          "a one-time relocation of credential material out of the `auth` column and into the vault. It MUST reach withdrawn connections: leaving their secrets in a column the vault was introduced to empty is the defect, and the row is claimed with `credential_ref IS NULL` so two boots relocate once",

	// MACHINE ATTRIBUTION AND DERIVED STATE. None of these carries anything a
	// reader is shown as the record's own content, and each is chosen by a pass
	// with its own selection rather than by a caller naming a row.
	"internal/modules/activities:writeDerivedAudienceTx":        "narrowing a message to its participants, pinned on the audience the caller read under FOR UPDATE. Narrowing an archived message is not a change to what anybody may see of a live one, and refusing it would leave the derivation half-applied",
	"internal/modules/activities:reKeyActivity":                 "restating the provider identity of a captured message so a later delivery folds into it rather than duplicating it. An absorbed echo is archived by design and is exactly the row whose identity has to be re-keyed",
	"internal/modules/activities:StampCorrespondenceForProject": "the retention CLASSIFICATION, which decides how long a message must be kept. An archived message still has a statutory window, and the stamp only ever fills a class nobody has set",
	"internal/modules/signals:dropUnattributable":               "the resolver recording that a market signal matched no organization. The row is one the resolution pass selected, and the write moves resolution_state alone — no content, no link",
	"internal/modules/signals:resolveToOrg":                     "the resolver stamping the single-candidate match on a signal its own pass selected; the person link it may write is separately consent-gated and never creates a record",
	"internal/modules/signals:flagAmbiguous":                    "the resolver flagging a signal for review when several organizations are plausible, on a row from the same pass. It links nobody and resolves nothing",
	"internal/modules/people:setDedupeDispositionTx":            "disposing a duplicate-candidate row, not either record it names. `disposition = 'open'` is the guard, and a candidate is closed rather than archived",
	"internal/modules/people:reopenDedupeCandidateTx":           "the undo of the same disposition, read and written under the candidate's own lock; nothing writes dedupe_candidate.archived_at",
	"internal/modules/people:RefreshDisplayNameTx":              "showing the name a contact's own first and last columns already carry. It refuses a name a human set, and an erasure NULLs both halves — so an erased subject fails the both-halves check before any write is attempted",
	"internal/modules/people:completePersonName":                "filling first and last from a confident parse, and only into columns that are both still empty. The predicate is also the concurrency guard, and an erased subject has had them nulled with the row's other identity columns",
	"internal/modules/people:absorbOrgReferences":               "the merge relinking a retired source's references onto its survivor. A merge deliberately reaches the row it is retiring — that is what a merge is — and mergePair resolved the pair under LockPair before this runs",
	"internal/modules/ai:persistBuildVersion":                   "recording the artifact a voice build produced, on the profile that build was raised for. The build row carries the profile id and the build itself is the selection; nothing in the tree writes voice_profile.archived_at",
	"internal/modules/ai:finishBuildTx":                         "closing out a build's own status row. A build is completed or failed, never archived, and the row is the one this pass is executing",
	"internal/modules/ai:RecordSendOutcomeTx":                   "closing a learning signal on the human's judgment of a draft, on a row lockJudgeableSignal has already held and confirmed is still `drafted`. That guard is narrower than liveness",
	"internal/modules/knowledge:DeleteDocument":                 "destroying a corpus document and its passages, under the document's own FOR UPDATE. A destruction that refused a retired document would leave its bytes and its embeddings behind",
	"internal/modules/knowledge:reconcilePages":                 "the boot-time handbook reconcile, which withdraws pages an earlier release shipped and this one does not. The set it acts on is derived from what the binary carries, not from a caller",
	"internal/modules/knowledge:replacePage":                    "the same reconcile replacing a page whose checksum moved, on a document it just resolved as managed by the handbook source",
	"internal/modules/introductions:Decide":                     "an ask's decision, pinned on the version the decider read (`AND version = $6`) and admitted by the state machine May() checks first. An intro_request is closed, never archived",
	"internal/modules/introductions:Complete":                   "the same move, recording that the introduction happened or that a name was dropped instead, under the same version pin",
	"internal/modules/introductions:Cancel":                     "the same move, withdrawing the ask — a retraction, and the one direction an archived anchor must never freeze",
})

// retirableTables derives the set of tables that can hold an archived row from
// the migration sources — every CREATE TABLE carrying archived_at, plus the
// tables that grew the column in a later ALTER. Derived rather than listed, so
// a table that becomes retirable widens this census on its own.
func retirableTables(t *testing.T) map[string]bool {
	t.Helper()
	tables := map[string]bool{}
	for _, root := range []string{"migrations/core", "migrations/custom"} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".up.sql") {
				return err
			}
			raw, err := os.ReadFile(path) // #nosec G304 G122 -- path is a *.up.sql file from walking the trusted migrations tree
			if err != nil {
				return err
			}
			current := ""
			for _, line := range strings.Split(string(raw), "\n") {
				if m := createTableLine.FindStringSubmatch(line); m != nil {
					current = m[1]
					continue
				}
				if strings.HasPrefix(line, ");") {
					current = ""
					continue
				}
				if current != "" && archivedColumnLine.MatchString(line) {
					tables[current] = true
				}
			}
			for _, m := range addedArchivedColumn.FindAllStringSubmatch(string(raw), -1) {
				tables[m[1]] = true
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(tables) < retirableTableFloor {
		t.Fatalf("derived only %d retirable tables from the migrations and expect at least %d — "+
			"the derivation is broken, not the schema", len(tables), retirableTableFloor)
	}
	return tables
}

// byIDWritesIn names the retirable tables these statements write one row of by
// id. A table outside the set answers nothing about liveness because it cannot
// hold an archived row.
func byIDWritesIn(statements []string, retirable map[string]bool) map[string]bool {
	written := map[string]bool{}
	for _, lit := range statements {
		for _, m := range byIDWriteOf.FindAllStringSubmatch(lit, -1) {
			if retirable[m[1]] {
				written[m[1]] = true
			}
		}
	}
	return written
}

// statesLiveness reports whether this function answers the liveness question at
// all, either way. It reads the function's own frame: the statements it runs
// and the names it calls.
//
// The marker is deliberately NOT attributed to the table being written, which is
// where updateguard's lock credit is stricter. It cannot be: the liveness that
// governs a child write is frequently its ANCHOR's — a deal room's deal, a
// contract's organization — so requiring the probe and the statement to name one
// table would refuse the shape this rule is mostly about. What that costs is a
// function whose marker belongs to some other row; what insisting would cost is
// a waiver on every correct anchor-gated write in the tree.
func statesLiveness(fn *ast.FuncDecl, statements []string) bool {
	for _, lit := range statements {
		if livePredicate.MatchString(lit) {
			return true
		}
	}
	stated := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.SelectorExpr:
			if livenessMarkers[node.Sel.Name] {
				stated = true
			}
		case *ast.Ident:
			// A marker this package spells without a qualifier — identity's
			// LiveMemberSQL is called from inside its own package.
			if livenessMarkers[node.Name] {
				stated = true
			}
		}
		return true
	})
	return stated
}

func TestEveryByIDWriteOfARetirableRowAnswersForLiveness(t *testing.T) {
	t.Parallel()
	defer livenessUnstated.AssertAllMatched(t)
	retirable := retirableTables(t)
	fset := token.NewFileSet()
	heldCache := map[string]map[string][]string{}
	judged := 0
	for _, root := range []string{"internal/modules", "internal/compose", "internal/platform"} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") ||
				isIntegrationTagged(path) {
				return err
			}
			path = filepath.ToSlash(path)
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return err
			}
			// Read once per FILE rather than per function: the imports are the
			// file's, and heldStatements caches per package anyway.
			var imported []map[string][]string
			for _, dir := range inModuleImportDirs(file) {
				imported = append(imported, heldStatements(t, heldCache, dir))
			}
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				statements := statementsJudged(fn, heldStatements(t, heldCache, filepath.Dir(path)), imported)
				written := byIDWritesIn(statements, retirable)
				if len(written) == 0 {
					continue
				}
				judged++
				if statesLiveness(fn, statements) {
					continue
				}
				key := filepath.ToSlash(filepath.Dir(path)) + ":" + fn.Name.Name
				if livenessUnstated.Waived(t, key) {
					continue
				}
				t.Errorf("%s: %s writes one row of %s by id and answers nothing about liveness — "+
					"refuse an archived row (an archived_at IS NULL predicate, a *Live probe, a LiveOnly lock "+
					"or patch), DECLARE that this write reaches one on purpose (auth.EnsureRetractable, "+
					"storekit.IncludeArchived), or ratify it in livenessUnstated with where the liveness is",
					path, fn.Name.Name, strings.Join(sortedTables(written), ", "))
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	// A census that judged nothing certifies nothing, and this one has two ways
	// to go quiet: the walk can stop finding files, and the statement reader can
	// stop finding writes inside them. Neither produces a finding on its own —
	// they produce a smaller silence, which is what the floor is for.
	if judged < byIDWriteFloor {
		t.Fatalf("this census judged %d function(s) writing a retirable row by id and expects at least %d — "+
			"the reader has stopped seeing statements rather than the tree having lost them",
			judged, byIDWriteFloor)
	}
}

// sortedTables renders a finding's tables in a stable order, so one function's
// message does not change between runs over map iteration.
func sortedTables(written map[string]bool) []string {
	out := make([]string, 0, len(written))
	for table := range written {
		out = append(out, table)
	}
	sort.Strings(out)
	return out
}
