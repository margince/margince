// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

import (
	"regexp"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const frontendPrivacyPage = "../frontend/src/features/privacy/privacypage.ts"

// TestThePrivacyPageFallbackIsOneOnBothSides holds the subscription question's
// rights link to one order. The server fills the stored sentence and the page
// fills its own copy of it. An order that differed would show one sentence and
// record another.
func TestThePrivacyPageFallbackIsOneOnBothSides(t *testing.T) {
	t.Parallel()
	source, err := gatekit.ReadFile(frontendPrivacyPage)
	if err != nil {
		t.Fatalf("reading the frontend fallback: %v", err)
	}
	list := regexp.MustCompile(`PRIVACY_PAGE_FIELDS = \[([^\]]*)\]`).FindSubmatch(source)
	if list == nil {
		t.Fatalf("%s no longer declares PRIVACY_PAGE_FIELDS; point this gate at what replaced it", frontendPrivacyPage)
	}
	var screen []string
	for _, field := range regexp.MustCompile(`"([a-z_]+)"`).FindAllSubmatch(list[1], -1) {
		screen = append(screen, string(field[1]))
	}
	if server := consent.PrivacyPageFields(); !slices.Equal(screen, server) || len(server) == 0 {
		t.Errorf("the page falls back through %v and the server through %v", screen, server)
	}
}
