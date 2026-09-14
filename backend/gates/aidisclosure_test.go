// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H1

package gates

// The Art. 50 disclosure has ONE spelling, and it is draftfloor.AIDisclosure.
//
// The sentence a model-written message carries to say a model wrote it is a
// legal obligation, and the tree carried five copies of it that had drifted:
// some named the article and some did not, two of the five were English
// whatever language the draft was in, and which sentence a customer received
// depended on which surface wrote their message. That is the shape of defect
// nobody notices until two emails are compared side by side — and the one that
// is hardest to argue was an accident afterwards.
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

// disclosureHome is the file this gate exempts: its literals are the subject
// rather than a violation.
//
// Held by: TestTheAIDisclosureHasOneSpelling (backend/gates/aidisclosure_test.go)
const disclosureHome = "backend/internal/shared/kernel/draftfloor/contacttext.go"

// disclosureStems are the parts of each sentence a copy would carry, short of
// the citation clause: a drafter that wrote its own would spell the opening and
// might well leave the article off, which is precisely the drift that happened.
//
// Matched anywhere in the file rather than only at the start of a literal. A
// second copy does not have to be a whole string — the first one this gate was
// tried against was appended to the real line — and a comment quoting the
// sentence is a copy too: it is what the next reader edits when the wording
// changes and the code does not.
func disclosureStems(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, lang := range textlang.Shipped {
		line := draftfloor.AIDisclosure(lang)
		stem, _, found := strings.Cut(line, " (")
		if !found {
			t.Fatalf("the %s disclosure carries no citation clause to cut at: %q", lang, line)
		}
		out = append(out, stem)
	}
	if len(out) == 0 {
		t.Fatal("no shipped language answered a disclosure — this gate is reading nothing")
	}
	return out
}

func TestTheAIDisclosureHasOneSpelling(t *testing.T) {
	t.Parallel()

	stems := disclosureStems(t)
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
		if filepath.ToSlash(rel) == disclosureHome {
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
			t.Errorf("%s writes the Art. 50 disclosure itself:\n\n\t%s…\n\n"+
				"It is a legal line with three translations, and a second copy is how the tree "+
				"came to send one sentence from one surface and a different one from another. "+
				"Call draftfloor.AIDisclosure(lang) with the draft's own language.", rel, stem)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
}
