// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

import (
	"regexp"
	"testing"
	"unicode"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// The SQL mapping is generated from Go rather than a server's locale tables.
func TestIdentifierSQLCaseMappingMatchesEveryGoLowercaseRune(t *testing.T) {
	t.Parallel()
	paths, err := gatekit.Glob("migrations/core/*.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	pattern := regexp.MustCompile(`translate\(value, '([^']+)', '([^']+)'\)`)
	var from, to []rune
	for _, path := range paths {
		raw, err := gatekit.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if m := pattern.FindStringSubmatch(string(raw)); m != nil {
			from, to = []rune(m[1]), []rune(m[2])
		}
	}
	if len(from) < 1400 || len(from) != len(to) {
		t.Fatalf("short SQL lowercase mapping: %d/%d", len(from), len(to))
	}
	index := 0
	for r := rune(0); r <= unicode.MaxRune; r++ {
		want := unicode.ToLower(r)
		if want == r {
			continue
		}
		if index >= len(from) || from[index] != r || to[index] != want {
			t.Fatalf("SQL lowercase mapping differs from Go at %U", r)
		}
		index++
	}
	if index != len(from) {
		t.Fatalf("SQL has %d mappings, Go has %d", len(from), index)
	}
}
