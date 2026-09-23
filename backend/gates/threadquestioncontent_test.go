// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind claim H2

package gates

// One spelling of "has this message anything to read", held by a test rather
// than by a comment.
//
// TWO statements must agree: the claim that ADMITS a thread question
// (ThreadVerdictStore.ClaimDue) and the retirement that ENDS one
// (ThreadVerdictStore.RetireExhausted). They are each other's complement — a
// row is claimable while the rule holds and retirable once it does not — so a
// second, diverging spelling opens a gap rather than a disagreement: a row
// neither side will touch, pending forever.
//
// That gap is invisible from either statement alone, and invisible in
// production too. Nothing surfaces a question nobody asks, because it looks
// exactly like one nobody has got to yet. Which is why the obligation is a gate
// and not a review note.
//
// The subject is DERIVED from the table the rule is about: every non-test file
// that names `capture_thread_verdict` is read, and any emptiness test on an
// activity's content columns in one of them must reach the rule through
// threadHasSomethingToRead. A hand-listed pair of files would go stale the
// moment a third reader appears.
//
// `restricted_at IS NULL` is deliberately NOT a marker, and that is measured
// rather than assumed: eight files in this corpus carry it as the ordinary
// "do not serve held mail" guard, so matching it would accuse every one of them
// of a rule they are not spelling. The emptiness test is what is distinctive.
//
// THREE arms, because two of them can each pass over the defect alone:
//
//   - No second spelling anywhere in the corpus.
//   - Each named statement still composes the rule. A count of composers would
//     pass a third consumer appearing while one of the two dropped out, which
//     is the gap itself rather than a variation on it.
//   - The detector still recognises the rule it protects. The only instance in
//     the tree is the exempt one, so an edit to the SQL's shape — wrapping the
//     columns in a trim, say — would otherwise leave this gate matching nothing
//     and reporting PASS for the rest of its life.
//
// WHAT THIS DOES NOT CATCH, deliberately: a rule rebuilt in Go rather than in
// SQL. The defect this exists over was SQL, as is the whole engine.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// activityContentEmptinessTest matches a SQL test of an activity's own content
// for presence or absence. It matches the TEST and not a projection: a bare
// coalesce over subject in a SELECT list is the module's most common idiom and
// says nothing about this rule, so a comparison against the empty literal is
// what the match requires.
//
// The gap between the column and the comparison is bounded rather than
// forbidden, because the rule wraps its columns — today in a trim, tomorrow in
// whatever the next whitespace question needs — and a detector that insisted on
// the column sitting flush against the comparison would stop seeing its own
// subject the first time that happened. The self-check arm proves it still does.
var activityContentEmptinessTest = regexp.MustCompile(
	`(?i)\b(subject|body)\b[^;]{0,120}?(<>|!=|=)\s*''` +
		`|(\w+\.)?(subject|body)\s+IS\s+(NOT\s+)?NULL`)

// threadReadableRuleOwner is the file allowed to spell the rule out, and only
// inside the fragment's own declaration. Every other reader composes it.
const threadReadableRuleOwner = "internal/modules/capture/threadverdict.go"

// threadReadableRuleFragment is the identifier the statements compose.
const threadReadableRuleFragment = "threadHasSomethingToRead"

// threadReadableRuleReceiver is the store both statements hang off, so a
// same-named method on another type in this file is not mistaken for one.
const threadReadableRuleReceiver = "ThreadVerdictStore"

// threadReadableRuleComposers are the two statements the claim is ABOUT, named
// rather than counted. They are complements, and the gap exists only between
// these two: a third consumer may appear freely, but if either of these stops
// asking, a row becomes neither claimable nor retirable. A count cannot tell
// those apart once a third exists.
var threadReadableRuleComposers = []string{"ClaimDue", "RetireExhausted"}

// threadVerdictTable is the table the claim is about, and the corpus is every
// file that names it. The rule is meaningless away from these rows.
const threadVerdictTable = "capture_thread_verdict"

// fragmentSpan locates threadHasSomethingToRead in the owner file: where its
// declaration starts, and the SQL inside it. The constant is one
// backtick-quoted literal, so the body ends at the next backtick.
func fragmentSpan(text string) (decl, body, bodyEnd int, ok bool) {
	prefix := "const " + threadReadableRuleFragment + " = `"
	decl = strings.Index(text, prefix)
	if decl < 0 {
		return 0, 0, 0, false
	}
	body = decl + len(prefix)
	rel := strings.Index(text[body:], "`")
	if rel < 0 {
		return 0, 0, 0, false
	}
	return decl, body, body + rel, true
}

// withoutFragmentConstant returns the owner file with the rule's own
// declaration removed, so what remains is every OTHER statement in it.
func withoutFragmentConstant(text string) string {
	decl, _, bodyEnd, ok := fragmentSpan(text)
	if !ok {
		return text
	}
	return text[:decl] + text[bodyEnd:]
}

// threadVerdictCorpus reads every non-test Go file under internal/ that names
// the thread verdict table, keyed by repo-relative path.
func threadVerdictCorpus(t *testing.T) map[string]string {
	t.Helper()
	corpus := map[string]string{}
	err := filepath.WalkDir("internal", func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(body), threadVerdictTable) {
			corpus[filepath.ToSlash(path)] = string(body)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("sweeping internal/ for %s: %v", threadVerdictTable, err)
	}
	return corpus
}

// assertEachComposerAsksTheRule fails for a named statement that no longer
// reaches threadHasSomethingToRead, and for one that has gone missing entirely.
func assertEachComposerAsksTheRule(t *testing.T, src string) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, threadReadableRuleOwner, src, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", threadReadableRuleOwner, err)
	}
	asks := map[string]bool{}
	for _, decl := range file.Decls {
		fn, isFunc := decl.(*ast.FuncDecl)
		if !isFunc || fn.Recv == nil || fn.Body == nil {
			continue
		}
		if !slices.Contains(threadReadableRuleComposers, fn.Name.Name) ||
			receiverTypeName(fn) != threadReadableRuleReceiver {
			continue
		}
		body := src[fset.Position(fn.Body.Pos()).Offset:fset.Position(fn.Body.End()).Offset]
		asks[fn.Name.Name] = strings.Contains(body, threadReadableRuleFragment)
	}
	for _, name := range threadReadableRuleComposers {
		reaches, found := asks[name]
		if !found {
			t.Errorf("%s.%s is gone from %s, and this gate is named in the rule's own doc comment "+
				"as one of the two that ask it.\n"+
				"  Point %s at whatever replaced it, or say in the rule why the pair is now one.",
				threadReadableRuleReceiver, name, threadReadableRuleOwner, "threadReadableRuleComposers")
			continue
		}
		if !reaches {
			t.Errorf("%s.%s no longer asks %s.\n"+
				"  The claim and the retirement are complements: a row is claimable while the rule\n"+
				"  holds and retirable once it does not. A side that stops asking leaves a row\n"+
				"  neither side touches, and nothing surfaces it — a question nobody asks looks\n"+
				"  exactly like one nobody has got to yet.",
				threadReadableRuleReceiver, name, threadReadableRuleFragment)
		}
	}
}

func TestTheThreadQuestionsReadableRuleHasOneSpelling(t *testing.T) {
	t.Parallel()
	corpus := threadVerdictCorpus(t)
	owner, held := corpus[threadReadableRuleOwner]
	if !held {
		t.Fatalf("%s is not in the corpus — the rule moved, and this census is now reading past it", threadReadableRuleOwner)
	}
	// A census that reads nothing reports PASS. Twelve files name this table
	// today; a count this low means the sweep lost its footing.
	if len(corpus) < 6 {
		t.Fatalf("only %d file(s) name %s — the census is looking in the wrong place", len(corpus), threadVerdictTable)
	}

	// The rule's own SQL is the one text the detector is guaranteed to have to
	// see. Everywhere else it is meant to find nothing, so this is the only arm
	// that fails when the detector has gone blind.
	_, body, bodyEnd, spanned := fragmentSpan(owner)
	if !spanned {
		t.Fatalf("%s no longer declares %s as one backtick literal", threadReadableRuleOwner, threadReadableRuleFragment)
	}
	if !activityContentEmptinessTest.MatchString(owner[body:bodyEnd]) {
		t.Errorf("the detector no longer matches %s's own SQL, so it would report PASS over every "+
			"second spelling of it.\n"+
			"  Widen activityContentEmptinessTest to the shape the rule now uses.",
			threadReadableRuleFragment)
	}

	assertEachComposerAsksTheRule(t, owner)

	for path, text := range corpus {
		scanned := withoutGoLineComments(text)
		if path == threadReadableRuleOwner {
			// The owner file is not skipped wholesale: a second, diverging
			// spelling could then hide inside the very file that declares the
			// fragment. Only the fragment's OWN declaration is exempt.
			scanned = withoutFragmentConstant(scanned)
		}
		for _, hit := range activityContentEmptinessTest.FindAllString(scanned, -1) {
			t.Errorf("%s spells the readable-message rule itself (%q) instead of asking %s.\n"+
				"  Two spellings drift, and the drift is silent: a row the claim refuses and the\n"+
				"  retirement will not end sits pending with nothing that will ever answer it.\n"+
				"  Compose the fragment, or state here why this statement is not that rule.",
				path, hit, threadReadableRuleFragment)
		}
	}
}
