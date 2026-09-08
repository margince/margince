// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// A record held under a statutory retention obligation is unavailable in
// EVERY ordinary read path (A165/ADR-0114 §2): lists, timelines, search,
// exports, embeddings, agent grounding. This gate derives the readers of the
// activity table from the tree and asks each how it excludes a held row,
// because a reader that forgets is indistinguishable from one that never
// existed — until a supervisory authority asks why an erased subject's
// correspondence is on a sales rep's screen.
//
// A file satisfies the gate by ONE of three means, each a real exclusion:
//
//   - it carries the shared row scope (auth.ActivityDiscoverClause / ActivityContentClause or one of the
//     probes built on it), which always composes ActivityAvailableClause;
//   - it names restricted_at itself, or the privacy floor fragments that do;
//   - it filters `archived_at IS NULL`, which excludes held rows because the
//     schema makes restricted imply archived (activity_restricted_is_archived).
//
// A file that reads activity by none of those is waived here with the reason
// it may — and each waiver names the cost.
//
// WHAT A GREEN RUN HERE DOES NOT BUY, because the third marker is weaker than
// the sentence at the top and a reader could take this census for more than it
// is. Filtering `archived_at IS NULL` satisfies this gate while disclosing every
// AUDIENCE-limited subject there is. That is correct for the question actually
// asked — a statutory hold implies archived, so excluding archived excludes held
// — and it means this gate was never going to catch an audience defect, which is
// a different rule with its own gate.
//
// And it sees ONE door. An activity's content is also reachable through
// audit_log.before/.after, keyed by entity_id, so a reader of the trail never
// names the activity table and is never a subject here — the verdict this
// census gives such a file is the verdict it would give one that gated nothing.
// audittraildoor_test.go is the census of that door (#2138).

import (
	"go/ast"
	"go/token"
	"path"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// activityReadLiteral matches a SQL string literal that reads the activity
// table by name. `activity_link`, `activity_participant` and the other
// activity_* tables are deliberately not matched: they carry no content.
//
// The pattern is gatekit's, shared with the sibling censuses: a matcher that
// stops seeing this tree's SQL judges nothing and reads exactly like a clean
// tree, so it has one place to be right and one place to be tested.
var activityReadLiteral = gatekit.TableReadPattern("activity")

// auditImageRead matches a statement that reads the audit trail's before/after
// images.
//
// That trail is a SECOND DOOR onto an activity's content, reached through
// audit_log.entity_id rather than through the activity table: `before` and
// `after` carry the activity's subject verbatim, so a reader projecting them is
// reading activity content while naming no activity table. Three files did
// exactly that and were never subjects of this gate — one had a real audience
// defect and two gated correctly by luck, so the verdict on all three was the
// verdict a file that gates nothing gets.
//
// BOTH halves in one pattern, within a bounded window, rather than "mentions
// audit_log" AND "mentions before" anywhere in the declaration. A declaration
// carries its error strings too, and this tree writes sentences with the words
// "before" and "after" in them: asked separately, the two conditions pulled in
// a person-name repair and a JSON-decode error message as audit-image readers,
// which is a corpus that has to be ratified reader by reader for reasons that
// are not true.
//
// Both orders, because a projection may sit either side of its FROM.
var auditImageRead = regexp.MustCompile(
	`(?is)\b(?:from|join)\s+audit_log\b.{0,300}?\b(?:before|after)\b` +
		`|\b(?:before|after)\b.{0,300}?\b(?:from|join)\s+audit_log\b`)

// auditImageEntity is the record type a statement binds its audit rows to,
// when it binds it to a LITERAL at all.
//
// The trail is one table for every record the product keeps, so an image read
// is only a second door onto ACTIVITY content when an activity can be behind
// it. A statement pinned to `entity_type = 'deal'` cannot reach one — no
// activity row is in its range whatever the caller asks — and demanding the
// activity gate of it would be asking a deal reader to prove something about a
// table it never touches. That is how a census earns a waiver list of readers
// exempt for reasons that are not true, which is a list nobody can audit.
var auditImageEntity = regexp.MustCompile(`(?is)\bentity_type\s*=\s*'([a-z_]+)'`)

// readsAnActivitysAuditImage reports whether a statement reads an audit image
// an activity can be behind.
//
// UNBOUND MEANS YES. A parameterized entity_type, or one spelled in a form this
// does not read — `IN ('deal','lead')`, a value off a variable — leaves an
// activity in range, so the reader stays a subject. Only a literal naming
// something else takes it out, which is the one direction where being wrong
// costs a false PASS rather than a false finding.
func readsAnActivitysAuditImage(text string) bool {
	for _, at := range auditImageRead.FindAllStringIndex(text, -1) {
		if !boundToAnotherRecordType(text[at[0]:min(at[1]+auditImageBindingReach, len(text))]) {
			return true
		}
	}
	return false
}

// auditImageBindingReach is how far past the matched read the record-type
// binding is looked for.
//
// The match itself is not enough, and the reason is asymmetric: when the
// projection sits BEFORE its FROM — `SELECT before FROM audit_log WHERE ...` —
// the window ends at the table name and the WHERE that binds the record type
// is entirely outside it. So the lookup runs on from the match rather than
// within it.
//
// Forward only. A binding written before the projection is not read, which
// leaves the reader in the census: that is the safe direction, and reaching
// backwards would let an `entity_type = 'person'` belonging to some earlier
// query in the same declaration answer for this one.
const auditImageBindingReach = 300

// boundToAnotherRecordType reports whether every record type this statement
// names is one an activity cannot be.
func boundToAnotherRecordType(statement string) bool {
	bound := auditImageEntity.FindAllStringSubmatch(statement, -1)
	if len(bound) == 0 {
		return false
	}
	for _, match := range bound {
		if match[1] == "activity" {
			return false
		}
	}
	return true
}

// scopeMarkers are the shared gates that carry the availability test: a reader
// reaching activity through one of them cannot see a held row. They are Go
// calls rather than SQL, so they are matched on the names a reader reaches
// rather than inside its statements.
//
// The list is every activity gate auth exports plus the two privacy floor
// predicates, and it is stated rather than derived because each entry is a
// claim that THAT function composes ActivityAvailableClause. EnsureActivityWritable
// does so through EnsureActivityContentVisibleLive; it is here because a write
// path probes with it and nothing else, and a reader inside such a transaction
// is guarded by a call this gate could not otherwise see.
var heldScopeMarkers = []string{
	"ActivityDiscoverClause", "ActivityContentClause", "ActivityAvailableClause",
	"EnsureActivityVisible", "EnsureActivityVisibleLive",
	"EnsureActivityContentVisible", "EnsureActivityContentVisibleLive",
	"EnsureActivityWritable",
	"correspondenceFloorPredicate", "handelsbriefShielded",
}

// literalMarkers exclude a held row in the FUNCTION that carries them, not
// merely somewhere in the file. That is the granularity the tree actually
// supports: this SQL is assembled from concatenated fragments and shared
// constants, so the `FROM activity` and its `archived_at IS NULL` are
// routinely different string literals in one query, and matching per literal
// would report green code as red. Per function is still far tighter than per
// file — an `archived_at IS NULL` belonging to a different query in the same
// file no longer answers for the activity read beside it, which is the false
// negative both reviews of this gate found.
//
// `archived_at IS NULL` counts because the schema makes restricted imply
// archived (activity_restricted_is_archived), and the guard refuses a write
// that would un-archive a held row.
//
// The column may be aliased (`a.archived_at`) or bare, so the match is on the
// column and its test rather than on one spelling of the pair — a gate that
// only recognised the unaliased form would report green code as red, which
// costs its own credibility faster than a miss does.
var heldLiteralMarkers = []*regexp.Regexp{
	regexp.MustCompile(`(?i)restricted_at`),
	regexp.MustCompile(`(?i)\barchived_at\s+IS\s+NULL`),
}

// activityDimension is one obligation an activity reader owes, as the two
// things the walk needs to recognise it being met: the shared gates that carry
// it (Go names) and the predicates that spell it inline (SQL). The walk itself
// — call graph, fragments, per-function granularity, waivers — is one piece of
// machinery asked the same question about two different exclusions, because a
// second copy of it would drift from this one and the drift would show up as a
// census that quietly stopped seeing half the tree.
type activityDimension struct {
	scopeMarkers   []string
	literalMarkers []*regexp.Regexp
	// subject narrows WHICH activity reads the dimension is owed by. Nil means
	// every read of the table, which is the availability rule. The audience
	// rule is owed only by a read that projects content, and asking the whole
	// corpus for it produces a waiver list long enough to hide a real finding.
	subject *regexp.Regexp
}

// owes answers whether one activity read is in this dimension's corpus.
//
// Comments are stripped first, on both sides of the census: a statement is
// pulled in by what it PROJECTS, never by a sentence about what it projects, in
// the same way an exclusion is recognised by a predicate and never by a comment
// describing one.
func (d activityDimension) owes(sql string) bool {
	return d.subject == nil || d.subject.MatchString(withoutComments(sql))
}

// heldDimension is the statutory hold: a restricted row is out of every
// ordinary read path (A165/ADR-0114 §2).
var heldDimension = activityDimension{scopeMarkers: heldScopeMarkers, literalMarkers: heldLiteralMarkers}

// restrictedReadersAdmitted ratifies the readers that reach a held row and may.
//
// Keyed by FILE AND FUNCTION. It used to be keyed by file, which meant a
// waiver written for one reader silently covered every reader added to that
// file afterwards — the same looseness the file-scope exemption had, in the
// ratification list rather than in the walk. A key names one reader, and
// AssertAllMatched reports one whose reader has gone.
//
// Two entries that were here are gone rather than fixed: the replay lookup in
// activities/activity.go and the classify backlog in activities/capturelabel.go
// both stated in prose what the call graph now derives — the replay hands its
// id to readActivity, which carries the scope, and the backlog composes
// ClassifyBacklogPredicate, which filters archived rows. A reason a gate can
// work out is better computed than written down.
var restrictedReadersAdmitted = gatekit.Waive(map[string]string{
	"internal/compose/audiencerescope.go:AudienceRescopeGen.rescope":          "the audience-change consumer reads the thread key (content by the activity policy) and the capture owner of the ONE activity whose audience just moved — deliberately, as a system principal, because both are exactly what narrowing the derived models needs, to NARROW what other readers may see — excluding a held row here would leave a legal-hold conversation's derived signals workspace-visible, the exact disclosure the consumer exists to remove. The cost is that a held activity's thread key and owner id reach this system principal",
	"internal/modules/privacy/auditaudienceboundary.go:ListAuditLog":          "the compliance read joins activity to evaluate ONE predicate — the audience arm the row's author set — and projects a single boolean from it. No activity column reaches the caller: the join's whole output is content_readable, which can only ever WITHHOLD an audit image, never reveal an activity. A held activity is therefore no more readable through this join than without it. The cost is that the audit IMAGE of a held activity stays readable to the admin, which is a pre-existing property of audit_log rather than of this join — audit_log is append-only and the hold is on the activity — and is filed rather than settled here, because making the compliance trail skip held rows is a decision about A165 and not a fix to the audience gap this join closes",
	"internal/modules/privacy/erasure_graph.go:subjectNamedOnAParticipantRow": "the identity predicate BOTH participant scrubs share — the Art. 17 eraser's and the retention sweep's — and both are WRITERS. The one thing it reads from activity is channel_provider, a registry key naming a transport: never a subject's content, never projected, and read only because a chat roster names the third human in a group by an account id alone, which is meaningful only against the provider that issued it. Excluding a held activity here would do the opposite of what the hold protects — it would leave the erased subject's account standing on that roster row forever, readable and matchable back to them by the next roster naming it, while every other arm of the same statement removed them. The four statements built on it each carry notTransitivelyHeld, which is the hold exclusion that belongs to this path; the cost is that a held activity's transport decides whether a participant row on it is scrubbed, a fact about the row's own erasability whose effect is always toward removing the subject",

	// The four below read the trail with entity_type as a PARAMETER, so an
	// activity's audit image is within their range when a caller names one.
	// Each is ratified on the same ground the audience boundary above already
	// states, and the ground is a decision somebody owes rather than a property
	// of these readers: audit_log is append-only and the statutory hold is on
	// the ACTIVITY, not on the ledger row that recorded it. Making the
	// compliance trail skip held rows is an A165 question — a trail with holes
	// in it is its own defect — and it is filed rather than settled here.
	//
	// What this widening buys is that the door is now VISIBLE: before it, these
	// five readers were not subjects of this gate at all, and the verdict it
	// gave them was the verdict it gives a file that gates nothing.
	"internal/compose/magicseam.go:magicUndoJudge.judgeOne":                  "reads ONE audit row to decide whether the change it records can be taken back, and projects a VERDICT: magicOffer carries the audit id the caller already holds, magicRefusal a reason from a closed vocabulary. No image leaves it. The entry is one the caller can already see in that record's history — privacy.HistoryServesEntry is asked before the evaluation, and the target record goes through the seam's own visibility check with the row-scope miss and the object denial kept apart. Cost: an activity's before/after decides whether an undo control is offered beside an entry that reader was already being shown",
	"internal/compose/recordrestore.go:RestoreSeam.readRow":                  "reads ONE audit row by its own id to decide what restoring it means, entity_type parameterized. The image it reads is the image the caller is already looking at on that record's history — this read reveals nothing the history did not — and the restore it drives writes to the target row rather than disclosing the trail. Cost: an activity's before/after passes through this seam when a caller restores one",
	"internal/compose/humanprecedence.go:fieldOwnership.HumanOwnedConflicts": "asks which fields of ONE record a human last set, by looking for the field's key in an after-image. It projects no image: the statement's output is the set of field KEYS a human owns, so an activity's content cannot leave through it. Cost: an activity's after-image decides which of its own field names are reported as human-owned",
	"internal/compose/superseded.go:moneyMovedUnderIt":                       "asks whether a later audit row moved money under the row being judged, reading the after-image to compare one amount. It projects a boolean, never the image. Cost: an activity's after-image is read to answer a question about the row it belongs to",
	"internal/modules/people/ensurenamefill.go:displayNameSetByHumanTx":      "asks whether a human ever set this record's display name, by looking for the key in an after-image, and projects EXISTS. No image leaves it. Cost: an activity's after-image is read to answer a question about that same record's naming",

	"internal/modules/capture/tracestore.go:TraceStore.readRungs": "the capture trace ladder LEFT JOINs activity to reach one thing — the counterparty email a stored trace row was raised about — and uses it only inside the lateral's WHERE, to pick which disposition verdict applies. Every column it PROJECTS comes from capture_trace and from capture_pending_counterparty; no activity column is scanned, so a held activity is no more readable through this join than without it. Excluding held rows here would instead blank the disposition on a trace row whose message is under hold, which tells an operator the connector did nothing when it did. The cost is that a held activity's counterparty_email decides which verdict a trace row shows — a fact about the trace, never content of the activity",
})

// activityReaderScope is every non-test, non-generated file under internal/
// that reads an activity's content — through the activity table by name, or
// through the audit trail's before/after images, which carry the same subject
// and name no activity table.
var activityReaderScope = gatekit.Scope{
	Roots:   []string{"internal"},
	Subject: readsActivityTable,
	Exempt:  gatekit.Waive(map[string]string{}),
}

func readsActivityTable(path string, file *ast.File) bool {
	return gatekit.FileReadsTable(path, file, activityReadLiteral) ||
		readsTheAuditImage(path, file)
}

// readsTheAuditImage reports whether the file reads an activity's content
// through the audit trail rather than through the activity table.
//
// A file that reads audit_log and projects no image discloses
// nothing about an activity, and one that mentions `before` outside a trail
// read is prose or another table's column.
func readsTheAuditImage(path string, file *ast.File) bool {
	return gatekit.FileReadsTable(path, file, auditImageRead)
}

// readsActivityContent reports whether one declaration's text reads an
// activity's content, by EITHER door.
//
// The file-level subject above and this per-declaration filter have to agree,
// or widening one alone buys nothing: a file admitted to the corpus for its
// audit read, whose declarations are then judged only against the activity
// table, has no declaration that matches and reports no offenders. That is a
// PASS with nothing examined — precisely the vacuous verdict this widening was
// written to end, moved one level in.
func readsAnActivitysContent(text string) bool {
	if activityReadLiteral.MatchString(text) {
		return true
	}
	return readsAnActivitysAuditImage(text)
}

// unguardedActivityReaders names each reader in the file that reads the
// activity table and reaches no exclusion.
//
// A reader's exclusion is often not in the reader. This tree routinely splits
// one query across a function and the helper that builds its WHERE —
// ListActivitiesTx + listActivitiesFilter, listOpenTasks + openTasksFilter —
// so a function judged on its own body alone reports red while the scope it
// applies sits ten lines below, which teaches the next engineer to distrust
// the gate rather than to fix anything.
//
// That used to be answered by exempting the whole FILE the moment it mentioned
// one of the shared gates anywhere, and that is too much: a new unguarded
// reader added to such a file inherits an exemption earned by a different
// function. The exemption is call-graph-scoped now — a reader is guarded by
// what IT reaches, one level into its own package — which admits the split
// queries and admits nothing else.
//
// The graph is packageCallGraph, shared with the privacy and rename censuses,
// so what "reaches" means has one place to be right. Its own doc states the
// limit that matters here: an edge through an interface, a stored field or a
// closure is not followed, and an unfollowed edge is a route this gate cannot
// see rather than a route that carries nothing. A reader guarded only through
// such a call reports red and is ratified by name, which is the direction that
// asks a human instead of assuming one.
func unguardedActivityReaders(graph map[string]*graphFunc, file *ast.File, dim activityDimension) []string {
	var offenders []string
	for _, decl := range file.Decls {
		// The read is looked for in the declaration's whole text, not literal by
		// literal, because this tree concatenates a query out of several: a
		// function whose `FROM activity` sits in one constant and whose
		// `SELECT subject, body` sits in another has no single literal that
		// matches, and a per-literal walk skips it entirely — the shape that
		// reports PASS over a content reader.
		whole := declText(decl)
		if !readsAnActivitysContent(whole) {
			continue
		}
		reads := gatekit.DeclReads(decl, activityReadLiteral)
		// Asked of the DECLARATION's whole text, not of one literal. This tree
		// assembles a query from several string constants — a fragment holding
		// the WHERE, the caller holding the SELECT — so a reader whose
		// `FROM activity` and whose `subject` sit in different literals owes
		// the content rule just as surely, and a per-literal test would skip it
		// and report PASS.
		if !dim.owes(whole) {
			continue
		}
		fn, isFunc := decl.(*ast.FuncDecl)
		if !isFunc {
			// A package-level SQL fragment is half a query by construction, so
			// it is judged from the other end: at the functions that NAME it,
			// each of which owes the exclusion for the query it assembles.
			// Reporting the fragment itself would name a declaration nobody can
			// fix in place — the WHERE belongs to whoever composes it.
			//
			// Per BINDING, not per declaration. One `const (…)` block binds
			// several names, and only one of them may hold the activity SQL, so
			// a block judged whole would answer for a function that names its
			// unrelated sibling — reporting a reader that reads nothing, and
			// counting a sibling's namer as evidence that the fragment is
			// composed somewhere.
			for _, fragment := range activityFragmentsIn(decl) {
				for _, namer := range unguardedNamers(graph, fragment.names, dim) {
					offenders = append(offenders, namer+": "+gatekit.FirstLineOf(fragment.sql))
				}
			}
			continue
		}
		key := scrubKey(receiverTypeName(fn), fn.Name.Name)
		if guardedInItsPackage(graph, key, dim) {
			continue
		}
		offenders = append(offenders, key+": "+gatekit.FirstLineOf(quotableRead(reads, whole)))
	}
	return offenders
}

// guardedInItsPackage answers whether a reader is excluded from held rows by
// anything in its own package — what it reaches, or what reaches it.
//
// Both directions, because this tree splits a query BOTH ways. A reader calls
// the helper that writes its WHERE (ListActivitiesTx + listActivitiesFilter),
// and a reader is called by the transaction that probed and locked the row
// first (readAudienceImage under SetActivityAudience, which holds the row
// LiveOnly and passes EnsureActivityWritable before reading it). Judging only
// downwards reports the second kind red while its exclusion sits in the frame
// above.
//
// The caller arm demands EVERY caller, not any: a helper reached by one
// guarded transaction and one bare one is exactly the case worth a sentence,
// and it is the bare caller that gets reported by its own name as well. A
// function nothing calls is not guarded by vacuous truth — an unreferenced
// reader has no transaction to inherit from.
//
// What neither arm can check is whether the exclusion a neighbour carries
// actually constrains THIS query. That is the same assumption the downward arm
// has always made, and it is the price of a gate that does not report the
// composition this tree is built out of.
func guardedInItsPackage(graph map[string]*graphFunc, key string, dim activityDimension) bool {
	if reachesAnExclusion(graph, key, true, dim) {
		return true
	}
	called := false
	for callerKey, entry := range graph {
		if callerKey == key || !entry.calls[key] {
			continue
		}
		called = true
		if !reachesAnExclusion(graph, callerKey, true, dim) {
			return false
		}
	}
	return called
}

// reachesAnExclusion answers whether one function excludes a held row — in its
// own statements and identifiers, or through a function of its own package.
//
// followCalls is spent on the first hop and not renewed. One level is the
// shape the tree has: a reader and the helper that writes its WHERE. Following
// further would start admitting readers whose exclusion is three unrelated
// frames away, which is the file-scope looseness this replaced, spelled
// differently.
func reachesAnExclusion(graph map[string]*graphFunc, key string, followCalls bool, dim activityDimension) bool {
	entry, known := graph[key]
	if !known {
		return false
	}
	for _, statement := range entry.statements {
		// Comments are stripped first. A census that matched inside them would
		// read a sentence ABOUT the obligation as the obligation being met —
		// and this file's own explanations are exactly such sentences.
		if matchesAny(withoutComments(statement), dim.literalMarkers) {
			return true
		}
	}
	// The shared gates are Go calls rather than SQL, so they are looked for
	// among the names the function reaches. `calls` alone is not enough: this
	// tree passes a clause builder by name as often as it calls it.
	for _, names := range []map[string]bool{entry.calls, entry.reads} {
		for name := range names {
			if matchesMarker(name, dim.scopeMarkers) {
				return true
			}
		}
	}
	if !followCalls {
		return false
	}
	for callee := range entry.calls {
		if reachesAnExclusion(graph, callee, false, dim) {
			return true
		}
	}
	return false
}

// fragmentBinding is ONE package-level binding whose value reads the activity
// table: the names it binds, and the statement that made it a subject.
type fragmentBinding struct {
	names []string
	sql   string
}

// activityFragmentsIn splits a package-level declaration into the individual
// bindings that read the activity table.
//
// A `const (…)` block is one declaration binding many names, and the activity
// SQL usually belongs to exactly one of them. Collecting every name in the
// block would let a function that names an unrelated sibling answer for the
// fragment — as evidence that it is composed somewhere, and, when that
// function reaches no exclusion, as a reported reader of SQL it never touches.
func activityFragmentsIn(decl ast.Decl) []fragmentBinding {
	gen, isGen := decl.(*ast.GenDecl)
	if !isGen {
		return nil
	}
	// A const spec with no values REPEATS the previous one's — `const ( a =
	// "…"; b )` binds b to a's expression — so the statement has to be carried
	// forward or a function naming only the repeated name escapes entirely.
	// Only for const: a var has no implicit repetition, and carrying there
	// would attribute a statement to a name that was merely declared.
	var carried []ast.Expr
	var bindings []fragmentBinding
	for _, spec := range gen.Specs {
		value, isValue := spec.(*ast.ValueSpec)
		if !isValue {
			continue
		}
		values := value.Values
		switch {
		case len(values) > 0:
			carried = values
		case gen.Tok == token.CONST:
			values = carried
		}
		bindings = append(bindings, activityBindingsInSpec(value, values)...)
	}
	return bindings
}

// activityBindingsInSpec pairs each of this spec's values with the name it is
// bound to. values is passed in rather than read off the spec because a const
// spec may be repeating the previous one's, and only the caller walking the
// block knows what that was.
//
// One spec can bind several at once — `const activitySQL, personSQL = "…", "…"`
// — so taking every name in the spec would let the person statement's name
// answer for the activity one, in both directions: as evidence the activity
// fragment is composed somewhere, and, when the function naming it reaches no
// exclusion, as a reported reader of SQL it never touches.
//
// Names and values pair by index while their counts agree, which is what a
// spec of literals always looks like. When they do not — `var a, b =
// twoResults()` binds two names to one expression — the value is attributed to
// every name in the spec, because there is no index to pair on and the
// alternative is attributing it to nobody. That direction over-reports, which
// is the safe one: it can turn a clean function into a finding somebody looks
// at, never an activity reader into a clean one.
func activityBindingsInSpec(spec *ast.ValueSpec, values []ast.Expr) []fragmentBinding {
	names := make([]string, 0, len(spec.Names))
	for _, name := range spec.Names {
		if name.Name != "_" {
			names = append(names, name.Name)
		}
	}
	paired := len(spec.Names) == len(values)
	var bindings []fragmentBinding
	for i, value := range values {
		sql, reads := activitySQLIn(value)
		if !reads {
			continue
		}
		bound := names
		if paired {
			bound = nil
			if spec.Names[i].Name != "_" {
				bound = []string{spec.Names[i].Name}
			}
		}
		bindings = append(bindings, fragmentBinding{names: bound, sql: sql})
	}
	return bindings
}

// activitySQLIn is the first statement in this value that reads the activity
// table, and whether there is one.
func activitySQLIn(value ast.Expr) (string, bool) {
	found := ""
	ast.Inspect(value, func(node ast.Node) bool {
		if found != "" {
			return false
		}
		expr, isExpr := node.(ast.Expr)
		if !isExpr {
			return true
		}
		text, isText := gatekit.LiteralText(expr)
		if isText && activityReadLiteral.MatchString(text) {
			found = text
			return false
		}
		return true
	})
	return found, found != ""
}

// unguardedNamers are the functions that assemble a query from this fragment
// and reach no exclusion, sorted so a failure reads the same on every run.
//
// A fragment nothing names answers with the fragment itself, because unused
// SQL that reads activity is either dead or reached by a spelling this gate
// cannot see, and both deserve a sentence. nil means every namer is guarded,
// which is the only way a fragment passes.
func unguardedNamers(graph map[string]*graphFunc, names []string, dim activityDimension) []string {
	var unguarded []string
	named := false
	for key, entry := range graph {
		mentions := false
		for _, name := range names {
			if entry.reads[name] || entry.calls[name] {
				mentions = true
			}
		}
		if !mentions {
			continue
		}
		named = true
		if !guardedInItsPackage(graph, key, dim) {
			unguarded = append(unguarded, key)
		}
	}
	if !named {
		return []string{"a package-level SQL fragment no function names"}
	}
	sort.Strings(unguarded)
	return unguarded
}

// matchesMarker is CallsAny's substring test over a name already extracted,
// kept identical because this tree reaches a shared gate through a
// package-local wrapper as often as directly.
func matchesMarker(name string, markers []string) bool {
	for _, marker := range markers {
		if strings.Contains(name, marker) {
			return true
		}
	}
	return false
}

func TestEveryReaderOfTheActivityTableExcludesRestrictedRows(t *testing.T) {
	t.Parallel()
	defer restrictedReadersAdmitted.AssertAllMatched(t)
	subjects := activityReaderScope.Files(t)
	graphs := map[string]map[string]*graphFunc{}
	for _, src := range subjects {
		dir := path.Dir(src.Path)
		if _, done := graphs[dir]; !done {
			// The whole package, not the reading file: a helper that builds the
			// WHERE need not read the activity table itself, so it often sits in
			// a file this gate's Scope never selects. Resolving against subjects
			// alone would follow the call, find no body, and report exactly like
			// a helper that excludes nothing.
			graphs[dir] = packageCallGraph(t, dir)
		}
		for _, offender := range unguardedActivityReaders(graphs[dir], src.File, heldDimension) {
			if restrictedReadersAdmitted.Waived(t, src.Path+":"+strings.SplitN(offender, ":", 2)[0]) {
				continue
			}
			t.Errorf("%s: %s reads the activity table and excludes no held row — compose auth.ActivityContentClause / ActivityDiscoverClause / ActivityAvailableClause, filter `restricted_at IS NULL` or `archived_at IS NULL`, or ratify the reader in restrictedReadersAdmitted with the cost stated (A165/ADR-0114 §2)", src.Path, offender)
		}
	}
}

// quotableRead is what the failure message shows: the reader's own matching
// literal when it has one, and otherwise the assembled text — a split query has
// no single literal to quote, and quoting nothing would name a reader without
// showing what it reads.
func quotableRead(reads []gatekit.TableRead, whole string) string {
	if len(reads) > 0 {
		return reads[0].SQL
	}
	return whole
}

// declText joins a declaration's string literals into the statement they
// assemble into, which is what the dimension's subject and the table-read
// pattern are both matched against.
func declText(decl ast.Decl) string {
	var parts []string
	ast.Inspect(decl, func(node ast.Node) bool {
		expr, isExpr := node.(ast.Expr)
		if !isExpr {
			return true
		}
		if text, isText := gatekit.LiteralText(expr); isText {
			parts = append(parts, text)
		}
		return true
	})
	return strings.Join(parts, "\n")
}

func matchesAny(text string, markers []*regexp.Regexp) bool {
	for _, marker := range markers {
		if marker.MatchString(text) {
			return true
		}
	}
	return false
}

// The audit door's subject filter, as a spec.
//
// It is asked of every declaration in the corpus, so both directions cost: too
// narrow and a content reader is never examined; too wide and the census fills
// with readers of other records' trails, each needing a waiver written for a
// reason that is not true. A waiver list nobody can audit is how a census stops
// being read.
func TestTheAuditDoorAdmitsOnlyReadsAnActivityCanBeBehind(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name string
		sql  string
		want bool
	}{
		{
			name: "an image read pinned to activity",
			sql:  `SELECT after ->> 'subject' FROM audit_log WHERE entity_type = 'activity'`,
			want: true,
		},
		{
			name: "an image read that binds no record type at all",
			sql:  `SELECT before, after FROM audit_log WHERE entity_id = $1`,
			want: true,
		},
		{
			// The safe direction. A value off a variable could be 'activity'
			// on the next call, so it stays a subject.
			name: "an image read whose record type is parameterized",
			sql:  `SELECT before FROM audit_log WHERE entity_type = $1 AND entity_id = $2`,
			want: true,
		},
		{
			// No activity row is in this statement's range whatever the caller
			// asks, so demanding the activity gate of it would be asking a deal
			// reader to prove something about a table it never touches.
			name: "an image read pinned to another record type",
			sql:  `SELECT count(*) FROM audit_log au WHERE au.entity_type = 'deal' AND (au.after ->> 'expected_close_date') IS NOT NULL`,
			want: false,
		},
		{
			name: "a projection sitting before its FROM still counts",
			sql:  `SELECT a.before ->> 'body' FROM audit_log a WHERE a.entity_id = $1`,
			want: true,
		},
		{
			// The false positives that made the window bounded in the first
			// place: prose and error strings carrying the words on their own.
			name: "a sentence using the words apart from any audit read",
			sql:  `"the person's name was repaired before the export and after the merge"`,
			want: false,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := readsAnActivitysAuditImage(c.sql); got != c.want {
				t.Errorf("readsAnActivitysAuditImage = %v, want %v for:\n%s", got, c.want, c.sql)
			}
		})
	}
}
