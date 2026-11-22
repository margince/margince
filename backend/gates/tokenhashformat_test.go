// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// A column that stores the hash of a token states its format with a CHECK, or
// formatNotHeld names it with what else it holds. The format is 64 lower-case
// hex characters as text and 32 bytes as bytea. Without it a writer can store
// base64 or a prefixed value, and a lookup by hash never matches.
//
// The corpus derives from the head catalog: each column named token_hash, and
// the columns hashColumnsByAnotherName lists.

import (
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// catalogColumnLine reads a column: its table, its name and its type.
var catalogColumnLine = regexp.MustCompile(`^public\.(\w+)\.(\w+) (.+?)(?: NOT NULL)? gen=`)

// hashColumnsByAnotherName hold a SHA-256 under a name the corpus rule cannot
// derive. One is the code an OAuth client trades for its tokens. The other is
// a draft reference that is looked up by its hash.
var hashColumnsByAnotherName = []string{
	"oauth_authorization_code.code_hash",
	"voice_learning_signal.draft_ref_hash",
}

// formatNotHeld records each hash column that carries no format CHECK, with
// why. An unlisted column must carry it.
var formatNotHeld = gatekit.Waive(map[string]string{
	"meeting_proposal.token_hash": "erasure replaces the hash with a mark that names the activity, so a row holds one of two shapes",
})

// tokenHashFormat is the CHECK definition the catalog prints for a hash column
// of this type, and false for a type that has no stated format.
func tokenHashFormat(column, columnType string) (string, bool) {
	switch columnType {
	case "text":
		return "((" + column + " ~ '^[0-9a-f]{64}$'::text))", true
	case "bytea":
		return "((octet_length(" + column + ") = 32))", true
	}
	return "", false
}

// hashColumnsWithoutFormat answers each hash column of a catalog that no
// validated CHECK holds to its format, and every hash column the catalog has.
func hashColumnsWithoutFormat(catalog string) (off map[string]string, seen []string) {
	checks := map[string][]string{}
	types := map[string]string{}
	for line := range strings.SplitSeq(catalog, "\n") {
		line = strings.TrimSpace(line)
		// An unvalidated check keeps a suffix in its definition, so it never equals the format.
		if m := checkLine.FindStringSubmatch(line); m != nil {
			checks[m[1]] = append(checks[m[1]], m[2])
		}
		m := catalogColumnLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		key := m[1] + "." + m[2]
		if m[2] == "token_hash" || slices.Contains(hashColumnsByAnotherName, key) {
			types[key] = m[3]
		}
	}
	off = map[string]string{}
	for key, columnType := range types {
		seen = append(seen, key)
		table, column, _ := strings.Cut(key, ".")
		want, known := tokenHashFormat(column, columnType)
		switch {
		case !known:
			off[key] = "is " + columnType + ", and a token hash is stored as bytea"
		case !slices.Contains(checks[table], want):
			off[key] = "has no validated CHECK " + want
		}
	}
	sort.Strings(seen)
	return off, seen
}

func TestEveryTokenHashColumnStatesItsFormat(t *testing.T) {
	t.Parallel()
	defer formatNotHeld.AssertAllMatched(t)
	raw, err := gatekit.ReadFile("migrations/testdata/head_catalog.txt")
	if err != nil {
		t.Fatalf("reading the head catalog: %v", err)
	}
	off, seen := hashColumnsWithoutFormat(string(raw))
	// A short census would pass on a catalog whose line shape had moved.
	if len(seen) < 17 {
		t.Fatalf("read %d hash columns from the catalog, and the schema has at least 17: %v", len(seen), seen)
	}
	for _, key := range hashColumnsByAnotherName {
		if !slices.Contains(seen, key) {
			t.Errorf("hashColumnsByAnotherName lists %s, and the catalog has no such column", key)
		}
	}
	for _, key := range seen {
		fault, bad := off[key]
		if !bad || formatNotHeld.Waived(t, key) {
			continue
		}
		t.Errorf("%s %s. A reader cannot tell what the column holds, and a writer may store "+
			"another form. Add the CHECK in a migration, NOT VALID first and validated in the next file. "+
			"A new table stores a token hash as bytea: docs/how-to/apply-migrations.md.", key, fault)
	}
}

// The reader is held to each way a hash column can miss its format.
func TestTheTokenHashReaderSeesEachMissingFormat(t *testing.T) {
	t.Parallel()
	catalog := strings.Join([]string{
		"public.held_text.held_text_token_hash_check CHECK ((token_hash ~ '^[0-9a-f]{64}$'::text))",
		"public.held_text.token_hash text NOT NULL gen=- def=-",
		"public.held_bytes.held_bytes_token_hash_check CHECK ((octet_length(token_hash) = 32))",
		"public.held_bytes.token_hash bytea NOT NULL gen=- def=-",
		"public.bare.token_hash text gen=- def=-",
		"public.short.short_token_hash_check CHECK ((octet_length(token_hash) = 20))",
		"public.short.token_hash bytea NOT NULL gen=- def=-",
		"public.pending.pending_token_hash_check CHECK ((token_hash ~ '^[0-9a-f]{64}$'::text)) NOT VALID",
		"public.pending.token_hash text NOT NULL gen=- def=-",
		"public.swapped.swapped_token_hash_check CHECK ((octet_length(token_hash) = 32))",
		"public.swapped.token_hash text NOT NULL gen=- def=-",
		"public.typed.token_hash character varying(64) NOT NULL gen=- def=-",
		"public.oauth_authorization_code.code_hash text NOT NULL gen=- def=-",
		"public.other.content_hash text NOT NULL gen=- def=-",
	}, "\n")
	off, seen := hashColumnsWithoutFormat(catalog)
	if len(seen) != 8 {
		t.Fatalf("the reader saw %d hash columns, want 8: %v", len(seen), seen)
	}
	for _, key := range []string{
		"bare.token_hash", "short.token_hash", "pending.token_hash", "swapped.token_hash",
		"typed.token_hash", "oauth_authorization_code.code_hash",
	} {
		if _, bad := off[key]; !bad {
			t.Errorf("the reader passed %s, which does not state its format", key)
		}
	}
	for _, key := range []string{"held_text.token_hash", "held_bytes.token_hash"} {
		if fault, bad := off[key]; bad {
			t.Errorf("the reader refused %s, which states its format: %s", key, fault)
		}
	}
}
