// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H1

package gates

// The AI provenance notice has ONE spelling, and it is
// draftfloor.AIProvenanceNotice.
//
// The sentence a model-written draft carries to say a model wrote it is what
// asks the reviewing human to read before sending, and the tree carried five
// copies of it that had drifted: some named the article and some did not, two
// of the five were English whatever language the draft was in, and which
// sentence appeared depended on which surface wrote the draft. A reviewer shown
// a different sentence by each surface learns to skim past all of them.
//
// The notice discharges no disclosure duty — AIProvenanceNotice's own comment
// says why, and that is the place to read it. This gate holds the spelling.
//
// So the prohibition is on the SENTENCE, not on a symbol: a second copy arrives
// as a string literal, which is exactly what a new drafting surface reaches for
// when it does not know the helper exists. The check reads the shipped wording
// from the helper itself rather than restating it, so the day the wording
// changes this gate follows rather than failing.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/draftfloor"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// noticeHome is the file this gate exempts: its literals are the subject
// rather than a violation.
//
// Held by: TestTheAIProvenanceNoticeHasOneSpelling
// (backend/gates/aiprovenancenotice_test.go)
const noticeHome = "backend/internal/shared/kernel/draftfloor/contacttext.go"

// noticeStems are each notice's opening sentence: a drafter that wrote its own
// would spell the opening and might well word the review prompt differently.
//
// Matched anywhere in the file rather than only at the start of a literal. A
// second copy does not have to be a whole string — the first one this gate was
// tried against was appended to the real line — and a comment quoting the
// sentence is a copy too: it is what the next reader edits when the wording
// changes and the code does not.
func noticeStems(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, lang := range textlang.Shipped {
		line := draftfloor.AIProvenanceNotice(lang)
		stem, _, found := strings.Cut(line, ". ")
		if !found {
			t.Fatalf("the %s provenance notice has no second sentence to cut at: %q", lang, line)
		}
		out = append(out, stem)
	}
	if len(out) == 0 {
		t.Fatal("no shipped language answered a notice — this gate is reading nothing")
	}
	return out
}

func TestTheAIProvenanceNoticeHasOneSpelling(t *testing.T) {
	t.Parallel()

	stems := noticeStems(t)
	root := filepath.Join(repoRoot, "backend")
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		rel, relErr := filepath.Rel(repoRoot, path)
		if relErr != nil {
			return relErr
		}
		if filepath.ToSlash(rel) == noticeHome {
			return nil
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		source := string(raw)
		for _, stem := range stems {
			if !strings.Contains(source, stem) {
				continue
			}
			t.Errorf("%s writes the AI provenance notice itself:\n\n\t%s…\n\n"+
				"It is one line with three translations, and a second copy is how the tree "+
				"came to show one sentence from one surface and a different one from another. "+
				"Call draftfloor.AIProvenanceNotice(lang) with the draft's own language.", rel, stem)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
}
