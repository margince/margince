// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

import (
	"regexp"
	"sort"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// The is_identifier_text CHECK refuses every rune identifier_spelling removes
// in SQL. A rune Go keeps and SQL removes makes a folded inbound address fail
// its insert, so the whole capture of that message fails. Go may remove more.
func TestEveryRuneTheSpellingCheckRefusesIsDroppedInGo(t *testing.T) {
	t.Parallel()
	refused := identifierSpellingClass(t)
	if len(refused) < 60 {
		t.Fatalf("read %d runes from identifier_spelling's class; the reader no longer matches the migration", len(refused))
	}
	for _, r := range refused {
		if got := values.IdentifierText("a" + string(r) + "b"); got != "ab" {
			t.Errorf("IdentifierText keeps U+%04X, which identifier_spelling removes: %q", r, got)
		}
	}
}

// spellingClass is the bracket expression identifier_spelling hands regexp_replace.
var spellingClass = regexp.MustCompile(`(?s)FUNCTION identifier_spelling\(value text\).*?regexp_replace\(value,\s*'\[([^\]]+)\]'`)

// identifierSpellingClass reads the class from the newest migration defining it.
func identifierSpellingClass(t *testing.T) []rune {
	t.Helper()
	files, err := gatekit.Glob("migrations/core/*.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	var class string
	for _, file := range files {
		body, err := gatekit.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if m := spellingClass.FindSubmatch(body); m != nil {
			class = string(m[1])
		}
	}
	if class == "" {
		t.Fatal("no migration defines identifier_spelling with a regexp_replace class")
	}
	return expandClass(t, class)
}

// expandClass lists the runes of a class written as \uXXXX escapes and ranges.
func expandClass(t *testing.T, class string) []rune {
	t.Helper()
	escapes := regexp.MustCompile(`\\u([0-9A-Fa-f]{4})(?:-\\u([0-9A-Fa-f]{4}))?`)
	var out []rune
	for _, m := range escapes.FindAllStringSubmatch(class, -1) {
		from, err := strconv.ParseInt(m[1], 16, 32)
		if err != nil {
			t.Fatal(err)
		}
		to := from
		if m[2] != "" {
			if to, err = strconv.ParseInt(m[2], 16, 32); err != nil {
				t.Fatal(err)
			}
		}
		for r := from; r <= to; r++ {
			out = append(out, rune(r))
		}
	}
	return out
}
