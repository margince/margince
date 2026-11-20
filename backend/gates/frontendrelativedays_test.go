// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H1

package gates

// The filter builder bounds a "days ago" count where the engine does.
//
// The builder holds an entry back while it is past MAX_DAYS_AGO, and storekit
// refuses a count past MaxRelativeDays. Apart, the builder either counts what
// the server refuses or refuses what the server would count.

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const frontendSegmentPredicate = "../frontend/src/lib/segmentpredicate.ts"

// tsMaxDaysAgo reads the declaration only: a mention in a comment or a test
// would keep this gate green after the constant itself had moved.
var tsMaxDaysAgo = regexp.MustCompile(`(?m)^export const MAX_DAYS_AGO = ([0-9_]+);`)

func TestTheFrontendDayCountBoundMatchesTheEngine(t *testing.T) {
	t.Parallel()
	source, err := gatekit.ReadFile(frontendSegmentPredicate)
	if err != nil {
		t.Fatalf("reading the frontend filter tree: %v", err)
	}
	found := tsMaxDaysAgo.FindAllStringSubmatch(string(source), -1)
	if len(found) != 1 {
		t.Fatalf("%s declares `export const MAX_DAYS_AGO = N;` %d times, want once: a gate that reads nothing agrees with everything",
			frontendSegmentPredicate, len(found))
	}
	bound, err := strconv.Atoi(strings.ReplaceAll(found[0][1], "_", ""))
	if err != nil {
		t.Fatalf("%s: MAX_DAYS_AGO %q is not a whole number", frontendSegmentPredicate, found[0][1])
	}
	if bound != storekit.MaxRelativeDays {
		t.Errorf("the builder bounds days ago at %d and storekit.MaxRelativeDays at %d: move both together",
			bound, storekit.MaxRelativeDays)
	}
}
