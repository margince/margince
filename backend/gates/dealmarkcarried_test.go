// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// A statement that copies deal links onto another activity carries each link's
// mark with it. Without deal_relevance and relevance_source, a
// colleague's "not about this deal" is lost on the copy and the deal clock
// moves again. The census is every hand-written INSERT into activity_link.

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

var linkInsertRe = regexp.MustCompile(`(?i)INSERT\s+INTO\s+activity_link\b[^_]`)

// The census floors sit below what the tree holds today, so a walk of the
// wrong place fails instead of passing on nothing.
const (
	linkInsertCensusFloor = 10
	linkCopyCensusFloor   = 1
)

func TestEveryCopyOfAnActivityLinkCarriesTheDealMark(t *testing.T) {
	t.Parallel()
	inserts, copies := 0, 0
	err := gatekit.WalkDir("internal", func(rel string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(rel, ".go") ||
			strings.HasSuffix(rel, "_test.go") || strings.HasSuffix(rel, "_gen.go") {
			return err
		}
		body, err := gatekit.ReadFile(rel) // #nosec G304 -- a *.go file from walking the trusted backend tree
		if err != nil {
			return err
		}
		src := string(body)
		for _, at := range linkInsertRe.FindAllStringIndex(src, -1) {
			inserts++
			statement := src[at[0]:]
			if end := strings.IndexByte(statement, '`'); end >= 0 {
				statement = statement[:end]
			}
			if !copiesDealLinks(statement) {
				continue
			}
			copies++
			if !strings.Contains(statement, "deal_relevance") || !strings.Contains(statement, "relevance_source") {
				t.Errorf("%s copies activity_link rows without deal_relevance and relevance_source, "+
					"so a deal link's mark is lost on the copy. Copy both columns", rel)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking internal: %v", err)
	}
	if inserts < linkInsertCensusFloor || copies < linkCopyCensusFloor {
		t.Fatalf("the census found %d inserts and %d copies, under its floors of %d and %d: the walk read the wrong place",
			inserts, copies, linkInsertCensusFloor, linkCopyCensusFloor)
	}
}

var linkSourceRe = regexp.MustCompile(`(?i)FROM\s+activity_link\b`)

// copiesDealLinks holds for an insert whose column list writes deal_id and
// whose rows come from activity_link: a copy, not a fresh link.
func copiesDealLinks(statement string) bool {
	open, closing := strings.IndexByte(statement, '('), strings.IndexByte(statement, ')')
	if open < 0 || closing < open {
		return false
	}
	return strings.Contains(statement[open:closing], "deal_id") && linkSourceRe.MatchString(statement)
}
