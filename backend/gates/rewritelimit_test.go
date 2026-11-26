// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The composer bars a rewrite verb past the server's own limit, so the two
// numbers must be one. The TypeScript constant is a declared mirror of the
// limit the rewrite path refuses past, and this holds it.

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/rewrite"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const frontendRewrites = "../frontend/src/lib/rewrites.ts"

var rewriteMaxConstant = regexp.MustCompile(`export const REWRITE_MAX_CHARACTERS = ([0-9_]+);`)

func TestTheComposersRewriteLimitIsTheServersOwn(t *testing.T) {
	t.Parallel()
	source, err := gatekit.ReadFile(frontendRewrites)
	if err != nil {
		t.Fatalf("reading %s: %v", frontendRewrites, err)
	}
	found := rewriteMaxConstant.FindAllStringSubmatch(string(source), -1)
	if len(found) != 1 {
		t.Fatalf("%s declares REWRITE_MAX_CHARACTERS %d times, want once", frontendRewrites, len(found))
	}
	limit, err := strconv.Atoi(strings.ReplaceAll(found[0][1], "_", ""))
	if err != nil {
		t.Fatalf("REWRITE_MAX_CHARACTERS is not a number: %v", err)
	}
	if limit != rewrite.MaxSourceRunes {
		t.Errorf("the composer bars a rewrite past %d characters and the server past %d; set REWRITE_MAX_CHARACTERS to %d",
			limit, rewrite.MaxSourceRunes, rewrite.MaxSourceRunes)
	}
}
