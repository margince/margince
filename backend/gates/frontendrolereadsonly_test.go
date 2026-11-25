// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// A reader whose role only reads is refused in one sentence on every record
// action. The record pages word it from record.roleReadsOnly, and a move the
// server blocks carries the server's own words for it. This gate holds the two
// equal in every shipped language, so a refused button never says two things.

import (
	"regexp"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/compose/momentaction"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

var roleReadsOnlyEntry = regexp.MustCompile(`"record\.roleReadsOnly":\s*("(?:[^"\\]|\\.)*")`)

func TestTheServersRoleRefusalIsTheRecordPagesSentence(t *testing.T) {
	t.Parallel()
	server := momentaction.RoleReadsOnly()
	for language, want := range map[string]string{"en": server.En, "de": server.De, "vi": server.Vi} {
		source, err := gatekit.ReadFile("../frontend/src/i18n/" + language + ".ts")
		if err != nil {
			t.Fatalf("reading the %s copy: %v", language, err)
		}
		match := roleReadsOnlyEntry.FindSubmatch(source)
		if match == nil {
			t.Fatalf("frontend/src/i18n/%s.ts has no record.roleReadsOnly; the gate reads nothing", language)
		}
		got, err := strconv.Unquote(string(match[1]))
		if err != nil {
			t.Fatalf("record.roleReadsOnly in %s.ts is not one string literal: %v", language, err)
		}
		if got != want {
			t.Errorf("%s: the record pages say %q and a blocked move says %q. Change both together", language, got, want)
		}
	}
}
