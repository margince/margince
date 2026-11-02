// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// The rules that keep a mail file off a Documents list are spelled once. The
// deal's Files area and its deal room each spelled the inline-image rule, and
// the copies drifted. One lacked a COALESCE and kept a file with no content
// type out of a room.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// listedFileTests are each rule's type test, in any case and however the SQL
// around it is laid out. A second copy has to write one again, in Go or SQL.
var listedFileTests = map[string]*regexp.Regexp{
	"inline image":     regexp.MustCompile(`(?i)like\s+'image/`),
	"calendar payload": regexp.MustCompile(`(?i)like\s+'text/calendar`),
}

func TestTheListedMailFileRulesAreSpelledOnce(t *testing.T) {
	t.Parallel()
	const owner = "internal/platform/database/storekit/dealfiles.go"
	for rule, test := range listedFileTests {
		spellings := spellingsOf(t, test)
		if len(spellings) != 1 || spellings[0] != owner {
			t.Errorf("the %s rule is spelled in %v, want only %s. "+
				"Apply storekit.ListedMailFileSQL instead, so every Documents list keeps one answer",
				rule, spellings, owner)
		}
	}
}

func spellingsOf(t *testing.T, test *regexp.Regexp) []string {
	t.Helper()
	var spellings []string
	err := filepath.Walk("internal", func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || strings.HasSuffix(path, "_test.go") ||
			!strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, ".sql") {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if test.Match(body) {
			spellings = append(spellings, filepath.ToSlash(path))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the module tree: %v", err)
	}
	return spellings
}
