// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The deal page names the seat a single-threaded deal is missing. Below the
// floor in seats it asks for a second contact, and past it for a second engaged
// one. The server fires the finding below that same floor. If the two disagree,
// the card asks for a contact the deal already has.

import (
	"os"
	"regexp"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/compose/network"
)

const frontendThreadingFloor = "../frontend/src/features/deals/deal360/dealoverviewrows.ts"

// Matched by name, so an unrelated number in the file cannot pass for it.
var tsThreadingFloor = regexp.MustCompile(`const THREADING_FLOOR\s*=\s*(\d+)\b`)

func TestFrontendMirrorsTheThreadingFloor(t *testing.T) {
	t.Parallel()

	source, err := os.ReadFile(frontendThreadingFloor)
	if err != nil {
		t.Fatalf("read %s: %v", frontendThreadingFloor, err)
	}
	found := tsThreadingFloor.FindSubmatch(source)
	if found == nil {
		t.Fatalf("%s declares no THREADING_FLOOR: the committee card no longer mirrors the server's single-threading floor", frontendThreadingFloor)
	}
	mirrored, err := strconv.Atoi(string(found[1]))
	if err != nil {
		t.Fatalf("THREADING_FLOOR in %s is not a number: %v", frontendThreadingFloor, err)
	}
	if mirrored != network.ReportThreadingFloor {
		t.Fatalf("single-threading floor disagrees: server %d, %s %d. Change both together, or the card names the wrong missing seat",
			network.ReportThreadingFloor, frontendThreadingFloor, mirrored)
	}
}
