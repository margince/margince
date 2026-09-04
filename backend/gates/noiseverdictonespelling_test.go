// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// "An already-settled answer disowns this contact" has ONE spelling, and this
// is what fails when a second appears.
//
// The question is asked twice by design and in two transactions: a scan selects
// the contacts a noise verdict already covered, and the retraction re-asks on
// its own transaction whether that answer still stands. Both build their
// predicate from capture.noiseJudgedStandsSQL, which is what makes a keep_out
// withdrawn — or a verdict corrected — between the two cost the contact nothing.
//
// A second spelling would not fail anything. It would select a contact the
// recheck then spares, or spare one the scan had already archived, and the two
// halves would disagree about a contact nobody looked at again. That is why the
// function's doc comment claims one spelling, and why the claim needs a test
// rather than a reader's goodwill.
//
// It matches the PREDICATE, not the function name. Two markers, because the
// question has two halves and a second spelling of either is the drift:
//
//   - the OVERRIDE half — capture_sender_override tested for `keep_out`;
//   - the VERDICT half — a noise verdict together with the outranking test that
//     a live or `real` answer displaces it.
//
// Held against the whole module tier rather than the capture package: a second
// spelling written in a neighbouring module is the same defect, and a census
// that looked only where the first one lives would not see it.

import (
	"go/ast"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	// keepOutOverride is the override half: the standing "keep this sender out"
	// a mailbox owner set for themselves.
	keepOutOverride = regexp.MustCompile(`(?is)capture_sender_override.{0,400}?decision\s*=\s*'keep_out'`)
	// outrankedNoiseVerdict is the verdict half: a noise answer AND the test
	// that no live or `real` answer displaces it. Both, because a statement
	// asking only "is this noise" is a different and legitimate question —
	// the sweeps and the senders list each ask it — while the pair is this
	// one.
	outrankedNoiseVerdict = regexp.MustCompile(`(?is)status\s*=\s*'noise'.{0,600}?status\s+IN\s*\(\s*'pending',\s*'unsure',\s*'real'\s*\)`)
)

// theOneSpelling is the function whose doc comment claims to be it. Named
// rather than derived, because the claim is a sentence in that comment and this
// is the test it names.
const theOneSpelling = "noiseJudgedStandsSQL"

func TestTheSettledAnswerThatDisownsAContactHasOneSpelling(t *testing.T) {
	t.Parallel()
	heldCache := map[string]map[string][]string{}
	spellings := map[string]bool{}
	for _, src := range tierFiles(t, modulesDir) {
		dir := filepath.ToSlash(filepath.Dir(src.Path))
		held := heldStatements(t, heldCache, dir)
		var imported []map[string][]string
		for _, importDir := range inModuleImportDirs(src.File) {
			imported = append(imported, heldStatements(t, heldCache, importDir))
		}
		for _, decl := range src.File.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			for _, statement := range statementsJudged(fn, held, imported) {
				if keepOutOverride.MatchString(statement) || outrankedNoiseVerdict.MatchString(statement) {
					spellings[src.Path+":"+fn.Name.Name] = true
				}
			}
		}
	}

	// A census that found nothing certifies nothing, and this one goes quiet the
	// moment the statement reader stops recognising the predicate — which reads
	// exactly like a tree with one spelling in it.
	var found []string
	for where := range spellings {
		found = append(found, where)
	}
	if len(found) == 0 {
		t.Fatalf("this census found no spelling of the settled-answer predicate at all, and %s holds one — "+
			"the reader has stopped seeing it rather than the tree having lost it", theOneSpelling)
	}
	for _, where := range found {
		if strings.HasSuffix(where, ":"+theOneSpelling) {
			continue
		}
		t.Errorf("%s spells the settled-answer predicate, and %s claims to be its one spelling — "+
			"build from that function instead, so the scan and the retraction cannot ask different "+
			"questions about the same contact; if this genuinely asks something else, say so where the "+
			"claim is made and give it a name of its own",
			where, theOneSpelling)
	}
}
