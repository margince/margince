// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H1

//go:build !integration

package gates

// Every module names its entity-model page as its table list, and no Go comment
// copies that list.
//
// The page is rendered from tableOwners, so it cannot drift; a typed copy in a
// doc.go did, by up to seventeen tables. It reads the phrase, so a list written
// as "this module keeps a, b and c" passes.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// tableListPointer is the one legal form of the phrase.
var tableListPointer = regexp.MustCompile(`Tables owned: docs/reference/entity-model/([a-z0-9]+)\.md\.`)

// tableListPhrase is how a comment opens a list of a package's tables: the
// label with its colon, or a doc heading. "tables owned by three modules" is
// prose about other packages and stays legal.
var tableListPhrase = regexp.MustCompile(`(?i)\btables owned\s*:|^\s*#\s*tables owned\b|\balso owned\s*:`)

// tableListRoots are the Go trees whose comments describe table ownership:
// the backend's own packages and the extension units beside it.
var tableListRoots = []string{"internal", "../extensions"}

func TestTableOwnershipCommentsPointAtTheEntityModel(t *testing.T) {
	t.Parallel()
	owningPackages := map[string]bool{}
	for _, owner := range tableOwners {
		owningPackages[owner] = true
	}

	pointed := map[string]bool{}
	for _, root := range tableListRoots {
		for _, path := range goSourceFiles(t, root) {
			if strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "_gen.go") {
				continue
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			pkg := filepath.ToSlash(filepath.Dir(path))
			for i, line := range strings.Split(string(raw), "\n") {
				comment, isComment := strings.CutPrefix(strings.TrimSpace(line), "//")
				if !isComment || !tableListPhrase.MatchString(comment) {
					continue
				}
				checkTableListPointer(t, path, i+1, pkg, comment, owningPackages)
				pointed[pkg] = true
			}
		}
	}

	for owner := range owningPackages {
		if strings.HasPrefix(owner, catalogModulePrefix) && !pointed[owner] {
			t.Errorf("%s owns tables and none of its files carries the line "+
				"`// Tables owned: docs/reference/entity-model/%s.md.` — put it in the package comment, "+
				"so a reader of the module finds the list without searching for it", owner, areaOf(owner))
		}
	}
}

// checkTableListPointer judges one comment line that talks about owned tables.
func checkTableListPointer(t *testing.T, path string, line int, pkg, comment string, owningPackages map[string]bool) {
	t.Helper()
	match := tableListPointer.FindStringSubmatch(comment)
	switch {
	case match == nil:
		t.Errorf("%s:%d lists tables in a comment (%q). The list is "+
			"docs/reference/entity-model/, rendered from tableOwners; write "+
			"`Tables owned: docs/reference/entity-model/<page>.md.` and keep only the why of a table",
			path, line, strings.TrimSpace(comment))
	case !owningPackages[pkg]:
		t.Errorf("%s:%d points at entity-model/%s.md, and tableOwners gives %s no table — "+
			"the page lists nothing of this package's", path, line, match[1], pkg)
	case match[1] != areaOf(pkg):
		t.Errorf("%s:%d points at entity-model/%s.md, and the renderer files %s's tables under %s.md",
			path, line, match[1], pkg, areaOf(pkg))
	}
}

func TestTheTableListPhraseTellsALabelFromProse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		comment string
		isList  bool
	}{
		{" Tables owned: deal, pipeline, stage.", true},
		{" Tables owned: docs/reference/entity-model/deals.md.", true},
		{" # Tables owned", true},
		{" Also owned: lead_origin.", true},
		{" The flag lives on five tables owned by three modules.", false},
		{" Tables owned by different modules are relinked here.", false},
	}
	for _, tc := range tests {
		if got := tableListPhrase.MatchString(tc.comment); got != tc.isList {
			t.Errorf("tableListPhrase.MatchString(%q) = %v, want %v", tc.comment, got, tc.isList)
		}
	}
}
