// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The committed schema catalog, which the migration lane regenerates. It holds
// the checks as Postgres renders them, so later widening shows up here.
const headCatalogPath = "../../../migrations/testdata/head_catalog.txt"

// TestTheScopeChecksAdmitTheScopeVocabulary holds the database's copy of the
// scope vocabulary to the Go one, both ways. A scope only Go knows is refused
// at mint time by the row; a scope only the check knows is one no mint path
// admits.
func TestTheScopeChecksAdmitTheScopeVocabulary(t *testing.T) {
	catalog, err := os.ReadFile(headCatalogPath)
	if err != nil {
		t.Fatalf("reading the head catalog: %v", err)
	}
	vocabulary := make([]string, 0, len(passportScopeVocabulary))
	for _, scope := range passportScopeVocabulary {
		vocabulary = append(vocabulary, string(scope))
	}
	for table, want := range map[string][]string{
		"passport":                 vocabulary,
		"oauth_grant":              vocabulary,
		"oauth_authorization_code": append(slices.Clone(vocabulary), scopeOfflineAccess),
	} {
		got := scopeCheckWords(t, catalog, table)
		slices.Sort(got)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("%s_scopes_vocabulary admits %v, and the identity module grants %v.\n"+
				"\tChange the vocabulary and the check in one change, through a new migration.",
				table, got, want)
		}
	}
}

// scopeCheckWords reads the scope words a table's vocabulary check admits. It
// refuses a check that has lost its non-empty clause, since a credential with
// no scope is one no mint path writes either.
func scopeCheckWords(t *testing.T, catalog []byte, table string) []string {
	t.Helper()
	re := regexp.MustCompile(`(?m)^public\.` + table + `\.` + table +
		`_scopes_vocabulary CHECK (.*)$`)
	match := re.FindSubmatch(catalog)
	if match == nil {
		t.Fatalf("no %s_scopes_vocabulary check in %s", table, headCatalogPath)
	}
	check := string(match[1])
	if !strings.Contains(check, "cardinality(scopes) > 0") {
		t.Errorf("%s_scopes_vocabulary no longer refuses an empty scope list: %s", table, check)
	}
	words := regexp.MustCompile(`scopes <@ ARRAY\[([^\]]*)\]`).FindStringSubmatch(check)
	if words == nil {
		t.Fatalf("%s_scopes_vocabulary names no ARRAY of scopes: %s", table, check)
	}
	var out []string
	for part := range strings.SplitSeq(words[1], ",") {
		word := strings.TrimSuffix(strings.TrimSpace(part), "::text")
		out = append(out, strings.Trim(word, "'"))
	}
	return out
}
