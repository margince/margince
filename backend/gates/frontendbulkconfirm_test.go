// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The server refuses a bulk change above its confirm line without a token. The
// browser shows the Large change warning above its own copy of that line.
// If they differ, a change the server treats as large runs with no warning.
// Both numbers are read from their source text and compared.

import (
	"regexp"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	serverBulkConfirm   = "internal/compose/bulkconfirm.go"
	frontendBulkConfirm = "../frontend/src/components/bulkchangeapi.ts"
)

var (
	goBulkConfirmAbove = regexp.MustCompile(`(?m)^const bulkConfirmAbove\s*=\s*(\d+)\s*$`)
	tsBulkConfirmAbove = regexp.MustCompile(`(?m)^export const BULK_CONFIRM_ABOVE\s*=\s*(\d+);`)
)

func readBulkConfirmLine(t *testing.T, path string, pattern *regexp.Regexp) int {
	t.Helper()
	source, err := gatekit.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	m := pattern.FindSubmatch(source)
	if m == nil {
		t.Fatalf("%s no longer declares the bulk confirm line in the shape this gate reads", path)
	}
	n, err := strconv.Atoi(string(m[1]))
	if err != nil {
		t.Fatalf("%s: %q is not a number", path, m[1])
	}
	return n
}

func TestTheFrontendBulkConfirmLineMatchesTheServerOne(t *testing.T) {
	t.Parallel()
	server := readBulkConfirmLine(t, serverBulkConfirm, goBulkConfirmAbove)
	browser := readBulkConfirmLine(t, frontendBulkConfirm, tsBulkConfirmAbove)
	if server != browser {
		t.Errorf("the server confirms above %d rows and the browser warns above %d: change both together", server, browser)
	}
}
