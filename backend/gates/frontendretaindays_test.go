// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The retention editor refuses a window past its ceiling before it sends one.
// That ceiling must be the server's. A lower one refuses windows the server
// stores. A higher one lets a window through that only a 422 answers.

import (
	"regexp"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const frontendRetentionLogic = "../frontend/src/features/privacy/retention.logic.ts"

// retainDaysDeclaration reads the frontend's one declaration of the bounds.
var retainDaysDeclaration = regexp.MustCompile(
	`(?m)^export const RETAIN_DAYS = \{\s*min:\s*(\d+),\s*max:\s*(\d+)\s*\} as const;`)

func TestTheFrontendRetentionCeilingMatchesTheServers(t *testing.T) {
	t.Parallel()
	source, err := gatekit.ReadFile(frontendRetentionLogic)
	if err != nil {
		t.Fatalf("reading the frontend retention bounds: %v", err)
	}
	found := retainDaysDeclaration.FindAllStringSubmatch(string(source), -1)
	if len(found) != 1 {
		t.Fatalf("%s declares RETAIN_DAYS %d times, want once in the shape this gate reads", frontendRetentionLogic, len(found))
	}
	ceiling, err := strconv.Atoi(found[0][2])
	if err != nil {
		t.Fatalf("%s: RETAIN_DAYS.max %q is not a number", frontendRetentionLogic, found[0][2])
	}
	if ceiling != privacy.RetainDaysCeiling {
		t.Errorf("the frontend refuses a window past %d days; the server's RetainDaysCeiling is %d", ceiling, privacy.RetainDaysCeiling)
	}
}
