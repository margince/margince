// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// A person's time zone has one writer after their row exists: writeTimezoneTx
// in identity/usertimezone.go. It is the one place that keeps a chosen zone
// from being replaced by a browser guess, and that sets timezone_chosen with
// the zone. A second writer would store a zone without saying who picked it.
//
// What this gate can see: it counts `SET timezone` in string literals under
// internal/. A writer that sets the column later in a SET list is not seen.

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// TestOneWriterSetsAPersonsTimezone is what writeTimezoneTx's doc comment
// names.
func TestOneWriterSetsAPersonsTimezone(t *testing.T) {
	t.Parallel()
	const key = "SET timezone"
	scope := gatekit.Scope{
		Roots:   []string{"internal"},
		Subject: fileContains(key),
		Exempt:  gatekit.Waive(map[string]string{}),
	}
	total, where := countAcross(t, scope, key)
	if total != 1 {
		t.Errorf("a person's time zone is set from %d place(s), want exactly 1 (writeTimezoneTx): %s",
			total, strings.Join(where, ", "))
	}
}
