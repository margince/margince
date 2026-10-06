// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H3

//go:build !integration

package gates

// A uuid this tree mints is a v7.
//
// ids.NewV7 states the rule and the reason: ids minted by one process sort by
// creation order, so a B-tree insert lands at the index's right edge instead of
// a random point in it. A v4 under a unique index pays the whole index as its
// working set on every insert.
//
// Two places can break it and they fail differently, so both are read here. A
// column DEFAULT is visible in the catalog and was where two primary keys sat.
// An inline gen_random_uuid() in a Go statement is not: no query over pg_attrdef
// reaches it, and provider_run's unique-indexed external_correlation_id was
// minted that way from two different statements.
//
// A gen_random_uuid() CAST TO TEXT is left alone. Those are deliberate random
// tokens — `'skipped:' || gen_random_uuid()::text` fills a text fingerprint
// column whose whole job is to not collide — and ordering means nothing to a
// value that is never a key. The cast is the distinction the rule turns on.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// v4Default matches a column whose default mints a v4. Scoped to a column line,
// so uuidv7()'s own body — which correctly takes its random bits from
// gen_random_uuid() — is not read as a violation of the rule it implements.
var v4Default = regexp.MustCompile(`^(?:public|ext)\.([a-z0-9_]+)\.([a-z0-9_]+) uuid .*def=gen_random_uuid\(\)(?:::uuid)?\s*$`)

// v4Mint is the inline call. Whether it lands in a uuid column or in a text
// token is decided by what FOLLOWS it, which RE2 cannot express as a lookahead,
// so mintsAUUIDValue reads that itself.
const v4Mint = "gen_random_uuid()"

// mintsAUUIDValue reports whether line mints a uuid VALUE — a call not cast to
// text. A cast one is a random token by intent, and ordering means nothing to a
// value that is never a key.
//
// The closing parens and spaces between the call and the cast are stepped over
// first, because `(gen_random_uuid())::text` is as legal as the bare form and a
// reader that demanded the cast touch the call would report that token as a key.
func mintsAUUIDValue(line string) bool {
	for rest := line; ; {
		at := strings.Index(rest, v4Mint)
		if at < 0 {
			return false
		}
		rest = strings.TrimLeft(rest[at+len(v4Mint):], ") \t")
		if !strings.HasPrefix(rest, "::text") {
			return true
		}
	}
}

// uuidColumnFloor is the smallest number of uuid columns this gate may read and
// still be believed: a reader that stopped reaching the catalog finds no v4
// default and reports the same clean tree as a schema that has none.
const uuidColumnFloor = 150

// mintedV7FileFloor is the same guard for the inline half: a walk that returned no
// files scans nothing and reports the same clean tree as a tree with no v4 in
// it.
const mintedV7FileFloor = 500

func TestEveryMintedUUIDIsAV7(t *testing.T) {
	t.Parallel()
	uuidColumns := 0
	for _, record := range catalogRecords(t) {
		record = strings.TrimSpace(record)
		if !strings.Contains(record, " uuid ") || strings.Contains(record, "CREATE ") {
			continue
		}
		uuidColumns++
		if m := v4Default.FindStringSubmatch(record); m != nil {
			t.Errorf("%s.%s defaults to gen_random_uuid(). uuidv7() is the rule: a v4 key lands at a "+
				"random point in the index on every insert, so the working set is the whole index "+
				"rather than its right edge.", m[1], m[2])
		}
	}
	if uuidColumns < uuidColumnFloor {
		t.Fatalf("read only %d uuid column(s) and expects at least %d — the catalog reader is broken, "+
			"not the schema", uuidColumns, uuidColumnFloor)
	}

	root := filepath.Join(moduleRoot(t), "internal")
	paths, err := goFilesUnder(root)
	if err != nil {
		t.Fatalf("walking the module: %v", err)
	}
	if len(paths) < mintedV7FileFloor {
		t.Fatalf("the walk found %d Go file(s) under internal/ and expects at least %d — it is reading a "+
			"smaller tree than the one this gate is about", len(paths), mintedV7FileFloor)
	}
	for _, path := range paths {
		body, readErr := os.ReadFile(path) // #nosec G304 -- a repo-relative path from the walk above
		if readErr != nil {
			t.Fatalf("reading %s: %v", path, readErr)
		}
		rel := filepath.ToSlash(strings.TrimPrefix(path, root+string(filepath.Separator)))
		for i, line := range strings.Split(string(body), "\n") {
			if mintsAUUIDValue(line) {
				t.Errorf("internal/%s:%d mints a uuid with gen_random_uuid(). Use uuidv7() — or cast it "+
					"to text if it is a random token rather than a key, which is what the cast says.",
					rel, i+1)
			}
		}
	}
}

// Both detectors, driven by cases that MUST fire.
//
// The gate above only ever asserts absence, so a catalog-dump drift that killed
// v4Default, or a spelling mintsAUUIDValue stopped seeing, would leave it green
// with nothing caught — and the floors cannot tell that apart, because they count
// the corpus rather than the detector.
func TestTheV4DetectorsFireOnWhatTheyAreFor(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		line  string
		mints bool
	}{
		{"a bare mint lands in a uuid column", "VALUES (gen_random_uuid(), $1)", true},
		{"a cast touching the call is a text token", "'skipped:' || gen_random_uuid()::text", false},
		{"a parenthesized cast is the same token", "'skipped:' || (gen_random_uuid())::text", false},
		{"a uuid mint beside a text one is still a mint", "gen_random_uuid(), gen_random_uuid()::text", true},
		{"no call at all", "VALUES (uuidv7(), $1)", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := mintsAUUIDValue(tc.line); got != tc.mints {
				t.Errorf("mintsAUUIDValue(%q) = %v, want %v", tc.line, got, tc.mints)
			}
		})
	}

	for _, record := range []string{
		"public.planted.id uuid NOT NULL gen=- def=gen_random_uuid()",
		"public.planted.id uuid NOT NULL gen=- def=gen_random_uuid()::uuid",
	} {
		if !v4Default.MatchString(record) {
			t.Errorf("v4Default misses %q, so a column defaulting to a v4 would pass the census", record)
		}
	}
	for _, record := range []string{
		"public.planted.id uuid NOT NULL gen=- def=uuidv7()",
		"public.planted.name text NOT NULL gen=- def=gen_random_uuid()",
	} {
		if v4Default.MatchString(record) {
			t.Errorf("v4Default matches %q, which is not a uuid column defaulting to a v4", record)
		}
	}
}
