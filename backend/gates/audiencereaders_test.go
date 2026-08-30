// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// A message's AUDIENCE says who may read its content. Every reader that serves
// subject, body, attachments, participants or anything derived from them owes
// the audience test, and the obligation is invisible in the code that forgets
// it: a reader that never composed the clause looks exactly like one that
// never needed to. This census asks every reader of the activity table which
// way it discharges the obligation, and makes an exemption a sentence somebody
// wrote rather than an omission nobody noticed.
//
// It shares its walk with the statutory-hold census next door
// (restrictedreaders_test.go): same call graph, same per-function granularity,
// same waiver discipline, a different pair of markers. Two copies of that
// machinery would drift, and a census that has stopped seeing half the tree
// reports PASS.
//
// The CORPUS is narrower than that census's, and deliberately so. The hold is
// an availability rule: a restricted row is out of every read, including a
// count and a timestamp. The audience is a CONTENT rule: it says who may read
// what the message says, and a reader taking `max(occurred_at)` off an
// activity owes it nothing. So the subject here is a reader whose activity SQL
// projects content — subject, body, raw, an attachment filename, an evidence
// snippet, a provider payload — and a reader that takes only markers is not in
// the corpus rather than waived out of it. Widening the corpus to every reader
// would have produced sixty-eight waivers, and a list that long is where a
// real finding hides.
//
// A file satisfies this gate by ONE of three means:
//
//   - it carries auth.ActivityContentClause (or a probe built on it), which IS
//     the audience test;
//   - it names the audience column itself, which is how the capture sink, the
//     recompute and the narrowing paths spell it;
//   - it is waived here, by name, with the cost of the exemption stated.
//
// ActivityDiscoverClause is deliberately NOT a discharge. Discovery is the
// weaker gate — it answers whether the caller may learn the activity exists —
// and a reader that serves content under it is precisely the defect this
// census exists to find.

import (
	"go/ast"
	"path"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// audienceScopeMarkers are the shared gates that carry the audience test.
// Each entry is a claim that THAT function composes the audience arm, which is
// why the list is stated rather than derived: ActivityContentClause composes it
// directly, and the Ensure* probes reach it through
// EnsureActivityContentVisible.
var audienceScopeMarkers = []string{
	"ActivityContentClause", "ActivityAudienceArm",
	"EnsureActivityContentVisible", "EnsureActivityContentVisibleLive",
	"EnsureActivityWritable",
	// The documents library reaches the clause two hops down —
	// visibleParentClause assembles the parent-kind arms and delegates the
	// activity one to activityParentClause, which composes it. The walk follows
	// one hop, so the wrapper is named here rather than the walk widened: a
	// second hop everywhere would start admitting readers whose gate is three
	// unrelated frames away, which is the file-scope looseness both censuses
	// were tightened away from.
	"visibleParentClause",
}

// audienceLiteralMarkers are the inline spellings that COMPARE the column, not
// merely mention it. A reader satisfies the census by testing the audience —
// `audience = 'workspace'`, `a.audience <> 'workspace'`, `audience IN (…)`, or
// stamping it on a write — and a query that projects `a.audience` beside an
// ungated body enforces nothing.
//
// The earlier spelling was a bare `\baudience\b`, which a SQL comment
// satisfied: the census would have read this file's own explanatory comments as
// enforcement. Comments are stripped before matching (matchesAny is fed through
// withoutComments for this dimension) and the word alone no longer counts, so
// the two ways to pass a census without doing anything are both closed.
var audienceLiteralMarkers = []*regexp.Regexp{
	// `IS` is deliberately absent. `audience IS NOT NULL` is a predicate that
	// constrains nothing — the column is NOT NULL — so admitting it would let a
	// reader satisfy the census with a test every row passes. Only a
	// comparison against a VALUE counts.
	regexp.MustCompile(`(?i)\baudience\b\s*(=|<>|!=|\bIN\b)`),
	// The write side: a sink or a recompute naming the column in what it SETS
	// or INSERTS is deciding the audience rather than reading past it.
	regexp.MustCompile(`(?i)(SET|,)\s*audience\s*=`),
	regexp.MustCompile(`(?i)INSERT\s+INTO\s+activity\s*\([^)]*\baudience\b`),
}

// audienceReadersAdmitted ratifies the readers that serve activity rows
// without the audience test and may. Keyed by FILE AND FUNCTION, so a waiver
// covers one reader and not every reader added to that file afterwards.
var audienceReadersAdmitted = gatekit.Waive(map[string]string{
	"internal/compose/participantreplay.go:selectReplayCandidates":            "the replay pass re-parses a stored provider original to fill in the participant rows an earlier capture missed, as the system principal, and every column it reads dies inside the transaction: the payload is handed to the mail parser and what SURVIVES is activity_participant rows — addresses and roles, never text. It cannot compose the audience because it is the pass that establishes who the participants ARE, which is one of the inputs the audience arm reads; gating it on the answer it computes would leave a message whose participants were missed permanently unreadable by the very people who were on it. The cost is that a limited message's stored original is parsed in-process by a system pass",
	"internal/modules/activities/capturenoise.go:Store.RedactCapturedNoiseTx": "the noise redaction reads subject, body and raw to answer ONE question — is there content left to destroy — and its only effect is to null them. A gate here would exempt limited mail from destruction, which is the opposite of what the audience protects: it would leave a colleague's held message stored forever because nobody may read it. The cost is nil; no column it reads reaches a caller",
	"internal/modules/capture/pending.go:PendingStore.ClaimDue":               "the ledger claim joins activity to build the verdict prompt for ONE pending sender, inside the capture module, on behalf of the mailbox that captured the mail. The audience is not yet decided for these rows — the verdict is what decides it — so composing it would be circular: no thread would ever be judged, and every first-time sender would stay pending forever. The cost is that the sender-verdict model reads the message text of a not-yet-judged message, which is why capture_counterparty_verdict is local-only and no_payload",
	"internal/modules/capture/sinkmailgates.go:Sink.correspondencePositiveTx": "the T1 correspondence gate asks whether THIS WORKSPACE has written to an address, and reads the subject and body of the attested outbound mail to answer it — the text is scanned for a decline and then dropped, so what survives the function is one boolean about the address. It is deliberately not per-mailbox: the question is the workspace's intent toward a counterparty, and the answer must be the same whichever seat is syncing when the address next appears, or the same address mints a record for one colleague and not another. The COST is real and stated rather than argued away: mailbox B's sink reads text from mailbox A's limited outbound, and A's limit is observable in B's result as a record that does or does not get created. Nothing of A's text reaches B — no subject, no body, no id — and the boolean would read the same for any decline-free outbound. Narrowing it to the acting mailbox is a product decision about what corresponded MEANS, not a fix to apply inside a waiver",
})

// activityContentColumn matches a projection of an activity's own content. The
// alternation is the columns activity and its attachment/raw siblings hold text
// in, and it is matched against the SQL that also reads the activity table, so
// a `body` belonging to some other table's query in the same function is not by
// itself enough — the function has to be reading activity too.
//
// It over-recognises rather than under-recognises: `payload` and `filename`
// belong to raw_capture and attachment, and a function joining those to
// activity is exactly a content reader. Over-recognition costs a waiver with a
// sentence; under-recognition costs a census that reports PASS over a leak.
var activityContentColumn = regexp.MustCompile(`(?i)\b(subject|body|raw|filename|evidence_snippet|payload)\b`)

// audienceReaderScope selects the files this census judges: non-test,
// non-generated, under internal/, reading the activity table and projecting
// content from it.
var audienceReaderScope = gatekit.Scope{
	Roots:   []string{"internal"},
	Subject: readsActivityContent,
	Exempt:  gatekit.Waive(map[string]string{}),
}

func readsActivityContent(path string, file *ast.File) bool {
	if !gatekit.FileReadsTable(path, file, activityReadLiteral) {
		return false
	}
	for _, read := range gatekit.TableReads(file, activityReadLiteral) {
		if activityContentColumn.MatchString(read.SQL) {
			return true
		}
	}
	return false
}

var audienceDimension = activityDimension{
	scopeMarkers:   audienceScopeMarkers,
	literalMarkers: audienceLiteralMarkers,
	subject:        activityContentColumn,
}

func TestEveryReaderOfTheActivityTableComposesTheAudience(t *testing.T) {
	t.Parallel()
	defer audienceReadersAdmitted.AssertAllMatched(t)
	subjects := audienceReaderScope.Files(t)
	graphs := map[string]map[string]*graphFunc{}
	for _, src := range subjects {
		dir := path.Dir(src.Path)
		if _, done := graphs[dir]; !done {
			graphs[dir] = packageCallGraph(t, dir)
		}
		for _, offender := range unguardedActivityReaders(graphs[dir], src.File, audienceDimension) {
			if audienceReadersAdmitted.Waived(t, src.Path+":"+strings.SplitN(offender, ":", 2)[0]) {
				continue
			}
			t.Errorf("%s: %s reads the activity table and composes no audience test — compose auth.ActivityContentClause, name the audience column, or ratify the reader in audienceReadersAdmitted with the cost stated", src.Path, offender)
		}
	}
}
