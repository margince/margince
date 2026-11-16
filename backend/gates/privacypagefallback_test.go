// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

import (
	"regexp"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	frontendPrivacyPage = "../frontend/src/features/privacy/privacypage.ts"
	serverPrivacyPage   = "internal/modules/consent/boundquestion.go"
)

// TestThePrivacyPageFallbackIsOneOnBothSides holds the subscription question's
// rights link to one order. The server fills the stored sentence and the page
// fills its own copy of it. An order that differed would show one sentence and
// record another.
func TestThePrivacyPageFallbackIsOneOnBothSides(t *testing.T) {
	t.Parallel()
	screen := privacyPageFieldsIn(t, frontendPrivacyPage, `PRIVACY_PAGE_FIELDS = \[([^\]]*)\]`)
	server := privacyPageFieldsIn(t, serverPrivacyPage, `privacyPageFields = \[\]string\{([^}]*)\}`)
	if !slices.Equal(screen, server) || len(server) == 0 {
		t.Errorf("the page falls back through %v and the server through %v", screen, server)
	}
}

// privacyPageFieldsIn reads the field names a declaration lists, out of its
// file's source. Both sides are read this way, so neither is a copy the gate
// keeps itself.
func privacyPageFieldsIn(t *testing.T, path, declaration string) []string {
	t.Helper()
	source, err := gatekit.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	list := regexp.MustCompile(declaration).FindSubmatch(source)
	if list == nil {
		t.Fatalf("%s no longer declares the privacy-page fields; point this gate at what replaced it", path)
	}
	var fields []string
	for _, field := range regexp.MustCompile(`"([a-z_]+)"`).FindAllSubmatch(list[1], -1) {
		fields = append(fields, string(field[1]))
	}
	return fields
}
