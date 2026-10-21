// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The browser and the server must read a unit's spelling as the same key.
//
// The server stores a known spelling as its key. The browser shows older rows
// through its own copy of the table. If the two part, a unit sent today and an
// older row spelled alike show as two different units. The TypeScript table is
// a declared mirror of deals.UnitSpellings, compared both ways, and UNIT_KEYS
// lists the keys it maps to.

import (
	"maps"
	"os"
	"regexp"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/modules/deals"
)

const frontendUnitTable = "../frontend/src/format/productunit.ts"

var (
	// One `spelling: "key",` entry, bare or quoted, on its own line.
	tsUnitSpelling = regexp.MustCompile(`(?m)^\s*(?:"([^"]+)"|([^\s":]+)):\s*"([a-z_]+)",?\s*$`)
	tsUnitKeys     = regexp.MustCompile(`UNIT_KEYS = \[([^\]]*)\]`)
	tsQuotedKey    = regexp.MustCompile(`"([a-z_]+)"`)
)

func readFrontendUnitTable(t *testing.T) (spellings map[string]string, keys []string) {
	t.Helper()
	source, err := os.ReadFile(frontendUnitTable)
	if err != nil {
		t.Fatalf("reading the frontend unit table: %v", err)
	}
	text := string(source)
	start := indexAfter(text, "UNIT_SPELLINGS: Readonly<Record<string, UnitKey>> = {")
	if start < 0 {
		t.Fatalf("%s no longer declares UNIT_SPELLINGS as an object literal; this gate reads a shape that is gone", frontendUnitTable)
	}
	end := indexAfter(text[start:], "};")
	if end < 0 {
		t.Fatalf("%s's UNIT_SPELLINGS literal is unterminated", frontendUnitTable)
	}
	table := tsComment.ReplaceAllString(text[start:start+end], " ")
	spellings = map[string]string{}
	for _, m := range tsUnitSpelling.FindAllStringSubmatch(table, -1) {
		spelling := m[1]
		if spelling == "" {
			spelling = m[2]
		}
		spellings[spelling] = m[3]
	}
	if len(spellings) == 0 {
		t.Fatal("no entries parsed out of UNIT_SPELLINGS; a gate that reads nothing agrees with everything")
	}
	list := tsUnitKeys.FindStringSubmatch(text)
	if list == nil {
		t.Fatalf("%s no longer declares UNIT_KEYS as an array literal", frontendUnitTable)
	}
	for _, m := range tsQuotedKey.FindAllStringSubmatch(list[1], -1) {
		keys = append(keys, m[1])
	}
	return spellings, keys
}

func TestTheFrontendUnitSpellingsMatchTheGoOnes(t *testing.T) {
	t.Parallel()
	inTS, keys := readFrontendUnitTable(t)
	inGo := deals.UnitSpellings()

	for spelling, key := range inGo {
		got, ok := inTS[spelling]
		switch {
		case !ok:
			t.Errorf("%q maps to %q in Go but is missing from %s", spelling, key, frontendUnitTable)
		case got != key:
			t.Errorf("%q maps to %q in Go but to %q in %s", spelling, key, got, frontendUnitTable)
		}
	}
	for spelling, key := range inTS {
		if _, ok := inGo[spelling]; !ok {
			t.Errorf("%q maps to %q in %s but is missing from deals.UnitSpellings", spelling, key, frontendUnitTable)
		}
	}

	targets := slices.Compact(slices.Sorted(maps.Values(inGo)))
	listed := slices.Sorted(slices.Values(keys))
	if !slices.Equal(targets, listed) {
		t.Errorf("UNIT_KEYS lists %v, but the spellings map to %v", listed, targets)
	}
	for _, key := range targets {
		if inGo[key] != key {
			t.Errorf("key %q does not spell itself in deals.UnitSpellings", key)
		}
	}
}
