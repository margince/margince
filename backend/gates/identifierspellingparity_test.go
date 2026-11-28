// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The characters drop_hidden_characters removes in the database are the ones
// values.IdentifierText drops in Go. A character the database refuses and Go
// keeps makes a received address fail its insert, so one crafted mail stalls a
// mailbox sync.

import (
	"regexp"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// hiddenClassSQL reads the bracket class of drop_hidden_characters.
var hiddenClassSQL = regexp.MustCompile(`(?s)FUNCTION drop_hidden_characters\(value text\).*?regexp_replace\(value, '\[([^\]]*)\]'`)

// classRuneOrRange reads one \uXXXX rune, or a range of two, from that class.
var classRuneOrRange = regexp.MustCompile(`\\u([0-9A-Fa-f]{4})(?:-\\u([0-9A-Fa-f]{4}))?`)

func TestEveryRuneTheDatabaseDropsGoDropsToo(t *testing.T) {
	t.Parallel()
	paths, err := gatekit.Glob("migrations/core/*.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	var class string
	for _, path := range paths {
		raw, err := gatekit.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if m := hiddenClassSQL.FindSubmatch(raw); m != nil {
			class = string(m[1])
		}
	}
	if class == "" {
		t.Fatal("no migration defines drop_hidden_characters; the reader no longer matches its shape")
	}
	checked := 0
	for _, part := range classRuneOrRange.FindAllStringSubmatch(class, -1) {
		from := hexRune(t, part[1])
		to := from
		if part[2] != "" {
			to = hexRune(t, part[2])
		}
		for r := from; r <= to; r++ {
			checked++
			if got := values.IdentifierText("a" + string(r) + "b"); got != "ab" {
				t.Errorf("drop_hidden_characters removes %U and values.IdentifierText keeps it: %q", r, got)
			}
		}
	}
	if checked < 70 {
		t.Fatalf("read %d runes from the class; the reader has stopped matching it", checked)
	}
}
